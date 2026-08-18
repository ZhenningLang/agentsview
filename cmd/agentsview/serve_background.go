package main

import (
	"fmt"
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
	dataDir, err := config.ResolveDataDir()
	if err != nil {
		fatal("serve background: resolving data dir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fatal("serve background: creating data dir: %v", err)
	}
	launchLock, ok := acquireBackgroundLaunchLock(dataDir)
	if !ok {
		reportBackgroundLaunchInProgress(dataDir, "")
		return
	}
	defer func() { _ = launchLock.Unlock() }()
	runServeBackground(mustLoadConfig(cmd), os.Args[1:])
}

func reportBackgroundLaunchInProgress(dataDir, authToken string) {
	WaitForDaemonStartup(dataDir, backgroundServeReadyTimeout, authToken)
	if rt := FindDaemonRuntime(dataDir, authToken); rt != nil && !rt.ReadOnly {
		fmt.Printf("agentsview already running at %s (pid %d)\n", urlFromDaemonRuntime(rt), rt.Record.PID)
		return
	}
	if state, ok := readStartupState(dataDir); ok {
		fmt.Printf("agentsview is starting (pid %d, phase %s).\n", state.PID, state.Phase)
		if state.LogPath != "" {
			fmt.Printf("Logs: %s\n", state.LogPath)
		}
		return
	}
	fmt.Println("agentsview serve --background is already in progress.")
}

func runServeBackground(cfg config.Config, args []string) {
	if cfg.RequireAuth {
		if err := cfg.EnsureAuthToken(); err != nil {
			fatal("serve background: generating auth token: %v", err)
		}
		if cfg.AuthToken != "" {
			fmt.Printf("Auth enabled. Token: %s\n", cfg.AuthToken)
		}
	}
	if rt := FindDaemonRuntime(cfg.DataDir, cfg.AuthToken); rt != nil && !rt.ReadOnly {
		fmt.Printf("agentsview already running at %s (pid %d)\n", urlFromDaemonRuntime(rt), rt.Record.PID)
		return
	}
	if IsLocalDaemonActive(cfg.DataDir, cfg.AuthToken) {
		reportBackgroundLaunchInProgress(cfg.DataDir, cfg.AuthToken)
		return
	}
	child, logPath, err := startServeBackgroundProcess(cfg, args)
	if err != nil {
		fatal("serve background: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- child.Wait() }()
	rt, err := waitForBackgroundServeReady(cfg.DataDir, cfg.AuthToken, waitCh, backgroundServeReadyTimeout)
	if err != nil {
		fatal("serve background: server exited before becoming ready: %v\nLogs: %s", err, logPath)
	}
	if rt != nil {
		fmt.Printf("agentsview running at %s (pid %d)\n", urlFromDaemonRuntime(rt), child.Process.Pid)
		fmt.Printf("Logs: %s\n", logPath)
		return
	}
	fmt.Printf("agentsview starting in background (pid %d)\n", child.Process.Pid)
	fmt.Printf("Logs: %s\n", logPath)
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
