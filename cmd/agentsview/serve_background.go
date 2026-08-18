package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/config"
)

const (
	backgroundChildEnvVar       = "AGENTSVIEW_BACKGROUND_CHILD"
	backgroundLogEnvVar         = "AGENTSVIEW_BACKGROUND_LOG"
	backgroundServeReadyTimeout = 5 * time.Second
)

func runningAsBackgroundChild() bool {
	return os.Getenv(backgroundChildEnvVar) == "1"
}

func backgroundLaunchLockPath(dataDir string) string {
	return filepath.Join(dataDir, "serve.background.lock")
}

func acquireBackgroundLaunchLock(dataDir string) (*flock.Flock, bool) {
	lock := flock.New(backgroundLaunchLockPath(dataDir))
	locked, err := lock.TryLock()
	if err != nil || !locked {
		return nil, false
	}
	return lock, true
}

func runServeBackgroundCommand(cmd *cobra.Command) {
	if _, err := startBackgroundServe(
		mustLoadConfig(cmd), os.Args[1:], cmd.OutOrStdout(),
	); err != nil {
		fatal("%v", err)
	}
}

// daemonStartResult is what a background start produces for its caller.
//
// Cfg is the *effective* config, not the one passed in: a start with
// require_auth enabled mints and persists a token, and a caller that
// keeps probing with its original empty token cannot discover the
// daemon it just started.
type daemonStartResult struct {
	Runtime *DaemonRuntime
	Cfg     config.Config
}

func reportBackgroundLaunchInProgress(out io.Writer, dataDir, authToken string) {
	WaitForDaemonStartup(dataDir, backgroundServeReadyTimeout, authToken)
	if rt := FindDaemonRuntime(dataDir, authToken); rt != nil && !rt.ReadOnly {
		fmt.Fprintf(out, "agentsview already running at %s (pid %d)\n", urlFromDaemonRuntime(rt), rt.Record.PID)
		return
	}
	if state, ok := readStartupState(dataDir); ok {
		fmt.Fprintf(out, "agentsview is starting (pid %d, phase %s).\n", state.PID, state.Phase)
		if state.LogPath != "" {
			fmt.Fprintf(out, "Logs: %s\n", state.LogPath)
		}
		return
	}
	fmt.Fprintln(out, "agentsview serve --background is already in progress.")
}

// startBackgroundServe is the one background-launch entrypoint. The
// `serve --background` command, `daemon start` and the MCP on-demand
// wake-up all reach the archive through it, so the launch lock is taken
// here rather than in the Cobra wrapper: a lock held by only one of
// three callers serializes nothing, and the two that skipped it could
// each spawn a child and each provision an auth token before either
// child reached the write-owner lock.
//
// It reports progress to out and returns an error rather than writing
// to os.Stdout and calling fatal. Both matter to the MCP command: its
// default transport is JSON-RPC over this process's stdout, so a status
// line there corrupts the session, and exiting the process turns a
// recoverable start failure into a dead client connection.
//
// Losing the launch race is not an error. The winner is starting the
// daemon this caller wanted, so report its progress and return it.
func startBackgroundServe(
	cfg config.Config, args []string, out io.Writer,
) (daemonStartResult, error) {
	if out == nil {
		out = io.Discard
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return daemonStartResult{Cfg: cfg},
			fmt.Errorf("serve background: creating data dir: %w", err)
	}
	launchLock, ok := acquireBackgroundLaunchLock(cfg.DataDir)
	if !ok {
		reportBackgroundLaunchInProgress(out, cfg.DataDir, cfg.AuthToken)
		return daemonStartResult{
			Runtime: FindDaemonRuntime(cfg.DataDir, cfg.AuthToken),
			Cfg:     cfg,
		}, nil
	}
	defer func() { _ = launchLock.Unlock() }()
	return startBackgroundServeLocked(cfg, args, out)
}

// startBackgroundServeLocked runs with the launch lock held. Token
// provisioning lives inside it so two starters cannot write competing
// tokens, and the minted token travels back out in the result.
func startBackgroundServeLocked(
	cfg config.Config, args []string, out io.Writer,
) (daemonStartResult, error) {
	if cfg.RequireAuth {
		if err := cfg.EnsureAuthToken(); err != nil {
			return daemonStartResult{Cfg: cfg},
				fmt.Errorf("serve background: generating auth token: %w", err)
		}
		if cfg.AuthToken != "" {
			fmt.Fprintf(out, "Auth enabled. Token: %s\n", cfg.AuthToken)
		}
	}
	res := daemonStartResult{Cfg: cfg}
	if rt := FindDaemonRuntime(cfg.DataDir, cfg.AuthToken); rt != nil && !rt.ReadOnly {
		fmt.Fprintf(out, "agentsview already running at %s (pid %d)\n", urlFromDaemonRuntime(rt), rt.Record.PID)
		res.Runtime = rt
		return res, nil
	}
	if IsLocalDaemonActive(cfg.DataDir, cfg.AuthToken) {
		reportBackgroundLaunchInProgress(out, cfg.DataDir, cfg.AuthToken)
		res.Runtime = FindDaemonRuntime(cfg.DataDir, cfg.AuthToken)
		return res, nil
	}
	child, logPath, err := startServeBackgroundProcess(cfg, args)
	if err != nil {
		return res, fmt.Errorf("serve background: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- child.Wait() }()
	rt, err := waitForBackgroundServeReady(cfg.DataDir, cfg.AuthToken, waitCh, backgroundServeReadyTimeout)
	if err != nil {
		return res, fmt.Errorf(
			"serve background: server exited before becoming ready: %w\nLogs: %s",
			err, logPath,
		)
	}
	res.Runtime = rt
	if rt != nil {
		fmt.Fprintf(out, "agentsview running at %s (pid %d)\n", urlFromDaemonRuntime(rt), child.Process.Pid)
		fmt.Fprintf(out, "Logs: %s\n", logPath)
		return res, nil
	}
	fmt.Fprintf(out, "agentsview starting in background (pid %d)\n", child.Process.Pid)
	fmt.Fprintf(out, "Logs: %s\n", logPath)
	return res, nil
}

func startServeBackgroundProcess(cfg config.Config, args []string) (*exec.Cmd, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, "", fmt.Errorf("finding executable: %w", err)
	}
	logPath := filepath.Join(cfg.DataDir, "serve.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()
	if _, err := fmt.Fprintf(logFile, "\n--- agentsview serve background start %s ---\n", time.Now().Format(time.RFC3339)); err != nil {
		return nil, "", fmt.Errorf("writing log header: %w", err)
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, "", fmt.Errorf("opening null device: %w", err)
	}
	defer devNull.Close()
	cmd := exec.Command(exe, serveBackgroundChildArgs(args)...)
	cmd.Env = append(os.Environ(), backgroundChildEnvVar+"=1", backgroundLogEnvVar+"="+logPath)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	configureServeBackgroundCommand(cmd)
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("starting server: %w", err)
	}
	return cmd, logPath, nil
}

func serveBackgroundChildArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if isBackgroundFlagArg(arg) {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func isBackgroundFlagArg(arg string) bool {
	for _, name := range []string{"--background", "-background"} {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func waitForBackgroundServeReady(dataDir, authToken string, waitCh <-chan error, timeout time.Duration) (*DaemonRuntime, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(startProbeTick)
	defer ticker.Stop()
	for {
		if rt := FindDaemonRuntime(dataDir, authToken); rt != nil && !rt.ReadOnly {
			return rt, nil
		}
		select {
		case err := <-waitCh:
			if err == nil {
				err = fmt.Errorf("server process exited")
			}
			return nil, err
		case <-ticker.C:
		case <-timer.C:
			return nil, nil
		}
	}
}
