package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// AuthRealm is the realm advertised in WWW-Authenticate.
const AuthRealm = "agentsview"

// httpShutdownGrace bounds how long in-flight requests get to finish
// after the context is cancelled. A local tool server has no long
// uploads to protect, so this is short enough that Ctrl-C feels
// immediate and long enough that a response in flight is not truncated.
const httpShutdownGrace = 5 * time.Second

// HTTPOptions is the caller's requested HTTP listener configuration,
// straight from the command line and config.
type HTTPOptions struct {
	// Addr is the requested bind address: a bare port ("8085"), a
	// port with a leading colon (":8085"), or host:port.
	Addr string

	// AllowInsecure opts in to binding somewhere other than loopback.
	AllowInsecure bool

	// Token is the bearer token clients must present. Empty means no
	// token was configured.
	Token string

	// RequireAuth demands a token even on loopback, provisioning one
	// when none was configured.
	RequireAuth bool
}

// HTTPListener is a validated listener configuration. Build it with
// ResolveHTTPListener; the zero value is not usable.
type HTTPListener struct {
	// Addr is the normalized host:port to bind.
	Addr string

	// Token is the bearer token to enforce, or "" for no auth. It is
	// only ever empty for a loopback listener that did not ask for
	// auth.
	Token string

	// Loopback reports whether Addr is a loopback address.
	Loopback bool

	// TokenProvisioned reports that Token was generated here rather
	// than supplied, so the caller can print it once for the client
	// config.
	TokenProvisioned bool
}

// ResolveHTTPListener validates and normalizes an HTTP listener
// request, failing closed.
//
// The threat model is a local MCP server holding every transcript on
// the machine, reachable by anything that can open a TCP connection to
// it. Three rules follow, and all three are enforced here rather than
// at the handler, so a misconfiguration cannot start listening at all:
//
//   - A bare port binds loopback. "8085" or ":8085" in a config file
//     reads as "port 8085", not as "every interface on this host",
//     and Go's default interpretation of ":8085" is the latter.
//   - Leaving loopback is an explicit opt-in.
//   - Off loopback, a token is mandatory even with that opt-in. The
//     insecure flag says where to listen, never who may talk.
func ResolveHTTPListener(opts HTTPOptions) (HTTPListener, error) {
	addr, loopback, err := normalizeBindAddr(opts.Addr)
	if err != nil {
		return HTTPListener{}, err
	}
	out := HTTPListener{
		Addr:     addr,
		Token:    strings.TrimSpace(opts.Token),
		Loopback: loopback,
	}
	if !loopback && !opts.AllowInsecure {
		return HTTPListener{}, fmt.Errorf(
			"refusing to bind %s: it is not a loopback address; "+
				"pass --http-allow-insecure to expose the archive "+
				"beyond this machine", addr,
		)
	}
	if !loopback && out.Token == "" {
		return HTTPListener{}, fmt.Errorf(
			"refusing to bind %s without an auth token: "+
				"--http-allow-insecure controls where to listen, "+
				"not who may read", addr,
		)
	}
	if out.Token == "" && opts.RequireAuth {
		token, err := generateToken()
		if err != nil {
			return HTTPListener{}, err
		}
		out.Token = token
		out.TokenProvisioned = true
	}
	return out, nil
}

// normalizeBindAddr turns a caller-supplied bind string into a
// host:port pair and reports whether the host is loopback.
func normalizeBindAddr(addr string) (string, bool, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", false, errors.New(
			"http address is required: pass a port or host:port",
		)
	}
	// A bare port, with or without the leading colon, means loopback.
	if port, err := strconv.Atoi(strings.TrimPrefix(addr, ":")); err == nil {
		if port < 0 || port > 65535 {
			return "", false, fmt.Errorf("invalid port %d", port)
		}
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), true, nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", false, fmt.Errorf("invalid http address %q: %w", addr, err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", false, fmt.Errorf("invalid port %q in %q", port, addr)
	}
	if strings.TrimSpace(host) == "" {
		return net.JoinHostPort("127.0.0.1", port), true, nil
	}
	return net.JoinHostPort(host, port), isLoopbackHost(host), nil
}

// isLoopbackHost reports whether host names only this machine.
//
// A name other than localhost is never treated as loopback even if it
// currently resolves there: resolution is attacker-influenceable and
// can change after the check, which is the same reason the SDK's
// rebinding guard inspects the Host header on every request.
func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// generateToken returns a fresh random bearer token.
func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating auth token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// NewHTTPHandler wraps the MCP streamable HTTP handler in bearer auth.
//
// The SDK's localhost protection is deliberately left enabled: it
// rejects requests that arrive on a loopback socket carrying a
// non-loopback Host header, which is exactly the shape of a DNS
// rebinding attack from a browser page against a local tool server.
func NewHTTPHandler(srv *mcpsdk.Server, listener HTTPListener) http.Handler {
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return srv }, nil,
	)
	return WithBearerAuth(listener.Token, handler)
}

// WithBearerAuth enforces a bearer token. An empty token disables the
// check, which ResolveHTTPListener only permits on loopback.
func WithBearerAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, ok := bearerToken(r)
		if !ok {
			unauthorized(w, "")
			return
		}
		// Digest both sides before comparing: subtle.ConstantTimeCompare
		// returns early for unequal lengths, so comparing raw tokens
		// would leak the token length through response timing. The
		// digests are always 32 bytes.
		got := sha256.Sum256([]byte(presented))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			unauthorized(w, "invalid_token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the credentials from an Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// unauthorized writes a 401 with the challenge a client needs to know
// what to send next.
func unauthorized(w http.ResponseWriter, errCode string) {
	challenge := `Bearer realm="` + AuthRealm + `"`
	if errCode != "" {
		challenge += `, error="` + errCode + `"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// ServeHTTP serves handler on ln until ctx is cancelled, then drains
// in-flight requests within httpShutdownGrace.
//
// A cancelled context is the normal way this server stops, so it
// returns nil: the caller's exit code should not say "failed" because
// the user pressed Ctrl-C.
func ServeHTTP(
	ctx context.Context, ln net.Listener, handler http.Handler,
) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	idle := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), httpShutdownGrace,
		)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
		}
		close(idle)
	}()
	err := srv.Serve(ln)
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-idle
	return nil
}

// RunStdio serves the MCP server over newline-delimited JSON on the
// given streams, defaulting to this process's stdin and stdout.
//
// Both ways this ends are normal: the client disconnects (EOF) or the
// user interrupts (context cancelled). Neither is reported as an
// error, because an MCP client that closes the pipe on exit would
// otherwise make every clean session look like a crash in the logs.
func RunStdio(
	ctx context.Context, srv *mcpsdk.Server,
	in io.ReadCloser, out io.WriteCloser,
) error {
	var transport mcpsdk.Transport = &mcpsdk.StdioTransport{}
	if in != nil && out != nil {
		transport = &mcpsdk.IOTransport{Reader: in, Writer: out}
	}
	err := srv.Run(ctx, transport)
	if isCleanDisconnect(err) {
		return nil
	}
	return err
}

// isCleanDisconnect reports whether err is one of the ordinary ways a
// transport ends.
func isCleanDisconnect(err error) bool {
	switch {
	case err == nil,
		errors.Is(err, context.Canceled),
		errors.Is(err, io.EOF),
		errors.Is(err, io.ErrClosedPipe),
		errors.Is(err, net.ErrClosed):
		return true
	}
	return false
}
