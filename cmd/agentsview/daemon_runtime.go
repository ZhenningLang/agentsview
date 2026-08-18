// ABOUTME: Adapts kit daemon runtime records for agentsview CLI transport.
// ABOUTME: Keeps daemon discovery metadata close to commands that use it.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/shirou/gopsutil/v4/process"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/kit/daemon"
)

const (
	daemonService           = "agentsview"
	runtimeAPIVersion       = "api_version"
	runtimeAPIVersionValue  = "1"
	runtimeCaddyCreateTime  = "caddy_create_time_ms"
	runtimeCaddyPID         = "caddy_pid"
	runtimeCreateTime       = "create_time_ms"
	runtimeDataVersion      = "data_version"
	runtimeHost             = "host"
	runtimeNoSync           = "no_sync"
	runtimePort             = "port"
	runtimeReadOnly         = "read_only"
	runtimeRequireAuth      = "require_auth"
	startProbeTick          = 250 * time.Millisecond
)

// DaemonRuntime is the agentsview-specific view of a kit daemon runtime record.
type DaemonRuntime struct {
	Record                  daemon.RuntimeRecord
	Host                    string
	Port                    int
	ReadOnly                bool
	RequireAuth             bool
	NoSync                  bool
	APIVersion              string
	DataVersion             int
	ProcessCreateTimeMillis int64
	CaddyPID                int
	CaddyCreateTimeMillis   int64
}

type daemonRuntimeOptions struct {
	RequireAuth bool
	NoSync      bool
	CaddyPID    int
}

func runtimeStore(dataDir string) daemon.RuntimeStore {
	return daemon.RuntimeStore{Dir: dataDir}
}

// WriteDaemonRuntime writes a shared kit daemon runtime record for the running
// server. It returns the path written.
func WriteDaemonRuntime(
	dataDir string, host string, port int, version string,
	readOnly bool, opts ...daemonRuntimeOptions,
) (string, error) {
	var options daemonRuntimeOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	ep := daemon.Endpoint{
		Network: daemon.NetworkTCP,
		Address: net.JoinHostPort(probeHostForDial(host), strconv.Itoa(port)),
	}
	rec := daemon.NewRuntimeRecord(daemonService, version, ep)
	rec.Metadata = map[string]string{
		runtimeAPIVersion:  runtimeAPIVersionValue,
		runtimeDataVersion: strconv.Itoa(db.CurrentDataVersion()),
		runtimeHost:        host,
		runtimeNoSync:      strconv.FormatBool(options.NoSync),
		runtimePort:        strconv.Itoa(port),
		runtimeReadOnly:    strconv.FormatBool(readOnly),
		runtimeRequireAuth: strconv.FormatBool(options.RequireAuth),
	}
	if ct, ok := processCreateTimeMillis(os.Getpid()); ok {
		rec.Metadata[runtimeCreateTime] = strconv.FormatInt(ct, 10)
	}
	if options.CaddyPID > 0 {
		rec.Metadata[runtimeCaddyPID] = strconv.Itoa(options.CaddyPID)
		if ct, ok := processCreateTimeMillis(options.CaddyPID); ok {
			rec.Metadata[runtimeCaddyCreateTime] = strconv.FormatInt(ct, 10)
		}
	}
	return runtimeStore(dataDir).Write(rec)
}

func processCreateTimeMillis(pid int) (int64, bool) {
	if pid <= 0 {
		return 0, false
	}
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, false
	}
	created, err := proc.CreateTime()
	if err != nil {
		return 0, false
	}
	return created, true
}

// RemoveDaemonRuntime removes the current process's kit daemon runtime record.
func RemoveDaemonRuntime(dataDir string) {
	path, err := runtimeStore(dataDir).Path(os.Getpid())
	if err == nil {
		_ = os.Remove(path)
	}
}

// FindDaemonRuntime returns a live agentsview daemon whose kit runtime record
// passes the ping probe. Writable daemons are preferred over read-only pg serve
// daemons when both are discoverable. When authToken is non-empty, it is sent
// as a bearer token so require_auth daemons remain discoverable.
func FindDaemonRuntime(dataDir string, authToken ...string) *DaemonRuntime {
	token := firstAuthToken(authToken)
	var readOnly *DaemonRuntime
	for _, target := range listDaemonRuntimeTargets(dataDir, token) {
		if target.CompatibilityErr != nil || !target.Confirmed {
			continue
		}
		rt := target.Runtime
		if !rt.ReadOnly {
			return rt
		}
		if readOnly == nil {
			readOnly = rt
		}
	}
	return readOnly
}

func firstAuthToken(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

func probeRuntime(
	ctx context.Context,
	rec daemon.RuntimeRecord,
	authToken string,
	opts daemon.ProbeOptions,
) (daemon.PingInfo, error) {
	ep := rec.Endpoint()
	if authToken == "" {
		return daemon.Probe(ctx, ep, opts)
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := ep.HTTPClient(daemon.HTTPClientOptions{
		Timeout:           timeout,
		DisableKeepAlives: true,
	})
	client.Transport = bearerAuthTransport{
		token: authToken,
		base:  client.Transport,
	}
	return daemon.ProbeHTTP(ctx, client, ep.BaseURL(), opts)
}

type bearerAuthTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerAuthTransport) RoundTrip(
	req *http.Request,
) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func daemonRuntimeFromRecord(rec daemon.RuntimeRecord) *DaemonRuntime {
	ep := rec.Endpoint()
	host, portText, _ := net.SplitHostPort(ep.Address)
	port, _ := strconv.Atoi(portText)
	if rec.Metadata != nil {
		if h := rec.Metadata[runtimeHost]; h != "" {
			host = h
		}
		if p := rec.Metadata[runtimePort]; p != "" {
			if parsed, err := strconv.Atoi(p); err == nil {
				port = parsed
			}
		}
	}
	readOnly := false
	requireAuth := false
	noSync := false
	apiVersion := ""
	dataVersion := 0
	processCreateTime := int64(0)
	caddyPID := 0
	caddyCreateTime := int64(0)
	if rec.Metadata != nil {
		readOnly, _ = strconv.ParseBool(rec.Metadata[runtimeReadOnly])
		requireAuth, _ = strconv.ParseBool(rec.Metadata[runtimeRequireAuth])
		noSync, _ = strconv.ParseBool(rec.Metadata[runtimeNoSync])
		apiVersion = rec.Metadata[runtimeAPIVersion]
		dataVersion, _ = strconv.Atoi(rec.Metadata[runtimeDataVersion])
		processCreateTime, _ = strconv.ParseInt(rec.Metadata[runtimeCreateTime], 10, 64)
		caddyPID, _ = strconv.Atoi(rec.Metadata[runtimeCaddyPID])
		caddyCreateTime, _ = strconv.ParseInt(rec.Metadata[runtimeCaddyCreateTime], 10, 64)
	}
	return &DaemonRuntime{
		Record:                  rec,
		Port:                    port,
		Host:                    host,
		ReadOnly:                readOnly,
		RequireAuth:             requireAuth,
		NoSync:                  noSync,
		APIVersion:              apiVersion,
		DataVersion:             dataVersion,
		ProcessCreateTimeMillis: processCreateTime,
		CaddyPID:                caddyPID,
		CaddyCreateTimeMillis:   caddyCreateTime,
	}
}

type daemonRuntimeTarget struct {
	Record           daemon.RuntimeRecord
	Runtime          *DaemonRuntime
	Confirmed        bool
	CompatibilityErr error
}

func listDaemonRuntimeTargets(dataDir, authToken string) []daemonRuntimeTarget {
	migrateLegacyDaemonRuntimes(dataDir, authToken)
	cleanupMismatchedDaemonRuntimes(dataDir)

	store := runtimeStore(dataDir)
	_, _ = store.CleanupDead()
	records, err := store.List()
	if err != nil {
		return nil
	}
	var targets []daemonRuntimeTarget
	for _, rec := range records {
		if rec.Service != "" && rec.Service != daemonService {
			continue
		}
		if !daemon.ProcessAlive(rec.PID) {
			continue
		}
		target := daemonRuntimeTarget{
			Record:           rec,
			Runtime:          daemonRuntimeFromRecord(rec),
			CompatibilityErr: daemonRuntimeCompatibilityError(rec),
		}
		info, err := probeRuntime(context.Background(), rec, authToken, daemon.ProbeOptions{
			ExpectedService: daemonService,
			Timeout:         500 * time.Millisecond,
		})
		target.Confirmed = err == nil && info.PID == rec.PID
		targets = append(targets, target)
	}
	return targets
}

func daemonRuntimeCompatibilityError(rec daemon.RuntimeRecord) error {
	if rec.Metadata == nil {
		return nil
	}
	if api := rec.Metadata[runtimeAPIVersion]; api != "" && api != runtimeAPIVersionValue {
		return fmt.Errorf("runtime API version %s is incompatible with %s", api, runtimeAPIVersionValue)
	}
	if raw := rec.Metadata[runtimeDataVersion]; raw != "" {
		got, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("runtime data version %q is invalid", raw)
		}
		if got != db.CurrentDataVersion() {
			return fmt.Errorf("runtime data version %d is incompatible with %d", got, db.CurrentDataVersion())
		}
	}
	return nil
}

func cleanupMismatchedDaemonRuntimes(dataDir string) {
	store := runtimeStore(dataDir)
	records, err := store.List()
	if err != nil {
		return
	}
	for _, rec := range records {
		if rec.SourcePath == "" || rec.Metadata == nil {
			continue
		}
		raw := rec.Metadata[runtimeCreateTime]
		if raw == "" {
			continue
		}
		if daemon.ProcessAlive(rec.PID) && !processCreateTimeMatches(rec.PID, raw) {
			_ = os.Remove(rec.SourcePath)
		}
	}
}

func liveDaemonRecords(dataDir string) []daemon.RuntimeRecord {
	targets := listDaemonRuntimeTargets(dataDir, "")
	records := make([]daemon.RuntimeRecord, 0, len(targets))
	for _, target := range targets {
		records = append(records, target.Record)
	}
	return records
}

func hasLiveDaemonRuntime(dataDir string, authToken ...string) bool {
	for _, target := range listDaemonRuntimeTargets(dataDir, firstAuthToken(authToken)) {
		if target.CompatibilityErr == nil {
			return true
		}
	}
	return false
}

type legacyStateFile struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port,omitempty"`
	Host      string `json:"host,omitempty"`
	Version   string `json:"version,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

func isLegacyStateFileName(name string) bool {
	return strings.HasPrefix(name, "server.") &&
		strings.HasSuffix(name, ".json")
}

func migrateLegacyDaemonRuntimes(dataDir string, authToken ...string) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return
	}
	token := firstAuthToken(authToken)
	for _, entry := range entries {
		if !isLegacyStateFileName(entry.Name()) {
			continue
		}
		path := filepath.Join(dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sf legacyStateFile
		if err := json.Unmarshal(data, &sf); err != nil {
			continue
		}
		if !daemon.ProcessAlive(sf.PID) {
			_ = os.Remove(path)
			continue
		}
		if sf.Port <= 0 {
			sf.Port = legacyPortFromStateFileName(entry.Name())
		}
		if sf.Port <= 0 {
			continue
		}
		rec := legacyRuntimeRecord(sf)
		info, err := probeRuntime(context.Background(), rec, token, daemon.ProbeOptions{
			ExpectedService: daemonService,
			Timeout:         500 * time.Millisecond,
		})
		if err != nil || info.PID != sf.PID {
			continue
		}
		if rec.Version == "" {
			rec.Version = info.Version
		}
		if _, err := runtimeStore(dataDir).Write(rec); err != nil {
			continue
		}
		_ = os.Remove(path)
	}
}

func legacyPortFromStateFileName(name string) int {
	portText := strings.TrimSuffix(strings.TrimPrefix(name, "server."), ".json")
	port, _ := strconv.Atoi(portText)
	return port
}

func legacyRuntimeRecord(sf legacyStateFile) daemon.RuntimeRecord {
	host := sf.Host
	if host == "" {
		host = "127.0.0.1"
	}
	rec := daemon.RuntimeRecord{
		PID:     sf.PID,
		Network: daemon.NetworkTCP,
		Address: net.JoinHostPort(probeHostForDial(host), strconv.Itoa(sf.Port)),
		Service: daemonService,
		Version: sf.Version,
		Metadata: map[string]string{
			runtimeHost:     host,
			runtimePort:     strconv.Itoa(sf.Port),
			runtimeReadOnly: strconv.FormatBool(sf.ReadOnly),
		},
	}
	if sf.StartedAt != "" {
		if startedAt, err := time.Parse(time.RFC3339Nano, sf.StartedAt); err == nil {
			rec.StartedAt = startedAt.UTC()
		}
	}
	return rec
}

func hasLiveWritableDaemonRuntime(dataDir string, authToken ...string) bool {
	for _, target := range listDaemonRuntimeTargets(dataDir, firstAuthToken(authToken)) {
		if target.CompatibilityErr == nil && !target.Runtime.ReadOnly {
			return true
		}
	}
	return false
}

type heldStartLock struct {
	path string
	lock *flock.Flock
}

var startLocks sync.Map

// MarkDaemonStarting acquires the kit daemon start lock for this data dir while
// the server is starting. The lock file itself is advisory; lock ownership is
// what other processes observe.
func MarkDaemonStarting(dataDir string) {
	path, err := runtimeStore(dataDir).LockPath()
	if err != nil {
		return
	}
	if _, ok := startLocks.Load(path); ok {
		return
	}
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil || !locked {
		return
	}
	startLocks.Store(path, heldStartLock{path: path, lock: lock})
}

// UnmarkDaemonStarting releases the kit daemon start lock for this data dir.
func UnmarkDaemonStarting(dataDir string) {
	path, err := runtimeStore(dataDir).LockPath()
	if err != nil {
		return
	}
	value, ok := startLocks.LoadAndDelete(path)
	if !ok {
		return
	}
	held := value.(heldStartLock)
	_ = held.lock.Unlock()
}

func isDaemonStarting(dataDir string) bool {
	path, err := runtimeStore(dataDir).LockPath()
	if err != nil {
		return false
	}
	if _, ok := startLocks.Load(path); ok {
		return true
	}
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil {
		return false
	}
	if locked {
		_ = lock.Unlock()
		return false
	}
	return true
}

const legacyStartupLockPrefix = "server.starting."

func isLegacyDaemonStarting(dataDir string) bool {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), legacyStartupLockPrefix) {
			continue
		}
		path := filepath.Join(dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
			continue
		}
		if !daemon.ProcessAlive(pid) {
			_ = os.Remove(path)
			continue
		}
		return true
	}
	return false
}

// IsDaemonStarting reports whether the shared kit daemon start lock is held.
func IsDaemonStarting(dataDir string) bool {
	return isDaemonStarting(dataDir) || isLegacyDaemonStarting(dataDir)
}

// IsDaemonActive reports whether a server process is managing dataDir.
func IsDaemonActive(dataDir string, authToken ...string) bool {
	return hasLiveDaemonRuntime(dataDir, authToken...) ||
		IsDaemonStarting(dataDir)
}

// IsLocalDaemonActive reports whether a writable local daemon is managing the
// SQLite archive in dataDir.
func IsLocalDaemonActive(dataDir string, authToken ...string) bool {
	return hasLiveWritableDaemonRuntime(dataDir, authToken...) ||
		IsDaemonStarting(dataDir)
}

// WaitForDaemonStartup polls until the daemon start lock clears or a running daemon is
// detected, up to the given timeout.
func WaitForDaemonStartup(
	dataDir string, timeout time.Duration, authToken ...string,
) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if FindDaemonRuntime(dataDir, authToken...) != nil {
			return true
		}
		if !IsDaemonStarting(dataDir) {
			return false
		}
		time.Sleep(startProbeTick)
	}
	return false
}

// probeHostForDial converts a bind-all address to a loopback address suitable
// for TCP readiness probes and daemon runtime endpoints.
func probeHostForDial(host string) string {
	switch host {
	case "", "0.0.0.0":
		return "127.0.0.1"
	case "::":
		return "::1"
	default:
		return host
	}
}
