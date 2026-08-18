package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/kit/daemon"
)

const serveStopGraceTimeout = 10 * time.Second

func runServeStatus(out io.Writer, cfg config.Config) {
	for _, target := range listDaemonRuntimeTargets(cfg.DataDir, cfg.AuthToken) {
		if target.CompatibilityErr != nil {
			fmt.Fprintf(out, "agentsview runtime pid %d is incompatible: %v\n", target.Record.PID, target.CompatibilityErr)
			continue
		}
		if target.Confirmed {
			for _, line := range serveStatusLines(target.Runtime) {
				fmt.Fprintln(out, line)
			}
			return
		}
		fmt.Fprintf(out, "agentsview process running (pid %d) but not responding to health checks.\n", target.Record.PID)
		return
	}
	if state, ok := readStartupState(cfg.DataDir); ok {
		fmt.Fprintf(out, "agentsview is starting (pid %d, phase %s).\n", state.PID, state.Phase)
		if state.LogPath != "" {
			fmt.Fprintf(out, "Logs: %s\n", state.LogPath)
		}
		return
	}
	if IsDaemonStarting(cfg.DataDir) {
		fmt.Fprintln(out, "agentsview is starting up.")
		return
	}
	fmt.Fprintln(out, "No agentsview server is running.")
}

func serveStatusLines(rt *DaemonRuntime) []string {
	lines := []string{
		fmt.Sprintf("agentsview running at %s", urlFromDaemonRuntime(rt)),
		fmt.Sprintf("  pid:     %d", rt.Record.PID),
	}
	if rt.Record.Version != "" {
		lines = append(lines, fmt.Sprintf("  version: %s", rt.Record.Version))
	}
	if !rt.Record.StartedAt.IsZero() {
		lines = append(lines, fmt.Sprintf("  uptime:  %s", time.Since(rt.Record.StartedAt).Round(time.Second)))
	}
	if rt.ReadOnly {
		lines = append(lines, "  mode:    read-only")
	}
	if rt.RequireAuth {
		lines = append(lines, "  auth:    required")
	}
	if rt.NoSync {
		lines = append(lines, "  sync:    disabled")
	}
	return lines
}

func runServeStop(out io.Writer, cfg config.Config) {
	records := liveDaemonRecords(cfg.DataDir)
	if len(records) == 0 {
		if readProcessStartupActive(cfg.DataDir) || IsDaemonStarting(cfg.DataDir) {
			fatal("serve stop: a server is starting; retry once it is ready")
		}
		fmt.Fprintln(out, "No agentsview server is running.")
		return
	}
	stopped, skipped := 0, 0
	for _, rec := range records {
		if !stopTargetConfirmed(rec, cfg.AuthToken) {
			fmt.Fprintf(out, "Skipping pid %d: cannot confirm it is the recorded agentsview daemon (stale record or reused pid).\n", rec.PID)
			skipped++
			continue
		}
		if err := stopDaemonProcess(rec, serveStopGraceTimeout); err != nil {
			fatal("serve stop: stopping pid %d: %v", rec.PID, err)
		}
		stopOrphanedCaddyChild(out, rec)
		fmt.Fprintf(out, "Stopped agentsview (pid %d).\n", rec.PID)
		stopped++
	}
	if stopped == 0 && skipped > 0 {
		fmt.Fprintln(out, "No agentsview server was stopped; runtime records may be stale.")
	}
}

func readProcessStartupActive(dataDir string) bool {
	_, ok := readStartupState(dataDir)
	return ok
}

func stopTargetConfirmed(rec daemon.RuntimeRecord, authToken string) bool {
	return daemonRecordPingConfirmed(rec, authToken) || processIdentityConfirmed(rec)
}

func daemonRecordPingConfirmed(rec daemon.RuntimeRecord, authToken string) bool {
	info, err := probeRuntime(context.Background(), rec, authToken, daemon.ProbeOptions{
		ExpectedService: daemonService,
		Timeout:         500 * time.Millisecond,
	})
	return err == nil && info.PID == rec.PID
}

func processIdentityConfirmed(rec daemon.RuntimeRecord) bool {
	if rec.Metadata == nil {
		return false
	}
	return processCreateTimeMatches(rec.PID, rec.Metadata[runtimeCreateTime])
}

func processCreateTimeMatches(pid int, recordedMillis string) bool {
	if recordedMillis == "" {
		return false
	}
	recorded, err := strconv.ParseInt(recordedMillis, 10, 64)
	if err != nil {
		return false
	}
	live, ok := processCreateTimeMillis(pid)
	return ok && live == recorded
}

func stopOrphanedCaddyChild(out io.Writer, rec daemon.RuntimeRecord) {
	if rec.Metadata == nil {
		return
	}
	pid, err := strconv.Atoi(rec.Metadata[runtimeCaddyPID])
	if err != nil || pid <= 0 || !processIsRunning(pid) {
		return
	}
	if !processCreateTimeMatches(pid, rec.Metadata[runtimeCaddyCreateTime]) {
		return
	}
	if err := stopDaemonProcess(caddyStopRecord(pid, rec.Metadata[runtimeCaddyCreateTime]), serveStopGraceTimeout); err != nil {
		fmt.Fprintf(out, "warning: could not stop managed caddy (pid %d): %v\n", pid, err)
		return
	}
	fmt.Fprintf(out, "Stopped managed caddy (pid %d).\n", pid)
}

func caddyStopRecord(pid int, createTime string) daemon.RuntimeRecord {
	return daemon.RuntimeRecord{PID: pid, Metadata: map[string]string{runtimeCreateTime: createTime}}
}

func stopDaemonProcess(rec daemon.RuntimeRecord, grace time.Duration) error {
	proc, err := os.FindProcess(rec.PID)
	if err != nil {
		return fmt.Errorf("finding process: %w", err)
	}
	// On Windows this handle keeps the process object alive after the
	// process itself is gone, so it is released as soon as this
	// operation is done with it rather than left to the finalizer.
	defer func() { _ = proc.Release() }()
	if err := terminateProcess(proc); err != nil {
		return fmt.Errorf("signalling shutdown: %w", err)
	}
	if waitForProcessExit(rec.PID, grace) {
		removeRuntimeRecordFile(rec)
		return nil
	}
	if !recordedDaemonStillPresent(rec) {
		removeRuntimeRecordFile(rec)
		return nil
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("force killing: %w", err)
	}
	if !waitForProcessExit(rec.PID, grace) && recordedDaemonStillPresent(rec) {
		return fmt.Errorf("process %d still running after force kill", rec.PID)
	}
	removeRuntimeRecordFile(rec)
	return nil
}

func recordedDaemonStillPresent(rec daemon.RuntimeRecord) bool {
	if rec.Metadata == nil || rec.Metadata[runtimeCreateTime] == "" {
		return true
	}
	return processCreateTimeMatches(rec.PID, rec.Metadata[runtimeCreateTime])
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processIsRunning(pid) {
			return true
		}
		time.Sleep(startProbeTick)
	}
	return !processIsRunning(pid)
}

func removeRuntimeRecordFile(rec daemon.RuntimeRecord) {
	if rec.SourcePath != "" {
		_ = os.Remove(rec.SourcePath)
	}
}
