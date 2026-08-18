package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/kit/daemon"
)

type daemonCommandHooks struct {
	start            func(config.Config, []string, io.Writer) (daemonStartResult, error)
	stop             func(daemon.RuntimeRecord) error
	checkDataVersion func(string) error
}

var daemonCommands = daemonCommandHooks{
	start: startBackgroundServe,
	stop: func(rec daemon.RuntimeRecord) error {
		return stopDaemonProcess(rec, serveStopGraceTimeout)
	},
	checkDataVersion: db.CheckDataVersion,
}

const daemonLifecycleLockFile = "daemon.lifecycle.lock"

// acquireDaemonLifecycleLock serializes the commands that mutate the
// daemon's lifecycle against each other.
//
// It is deliberately not the write-owner lock. A running daemon holds
// that one for its whole life, so a lifecycle command that takes it
// first can never act on a daemon that is actually running - which is
// the only case restart exists for. Writer exclusivity is still
// enforced where it belongs, in the child that opens the archive.
func acquireDaemonLifecycleLock(dataDir string) (*flock.Flock, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating data dir for lifecycle lock: %w", err)
	}
	path := filepath.Join(dataDir, daemonLifecycleLockFile)
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquiring daemon lifecycle lock %s: %w", path, err)
	}
	if !locked {
		return nil, fmt.Errorf(
			"another agentsview daemon lifecycle command is running (%s); "+
				"retry once it finishes",
			path,
		)
	}
	return lock, nil
}

// configuredAuthToken reads the token a running daemon would demand.
//
// Every "is a daemon already running" probe needs it: with
// require_auth the daemon answers 401 to an unauthenticated ping, is
// never confirmed, and looks absent. The read is deliberately
// read-only - asking whether a daemon runs must not migrate config or
// persist anything.
func configuredAuthToken(dataDir string) string {
	cfg, err := config.LoadReadOnly()
	if err != nil || !samePath(dataDir, cfg.DataDir) {
		return ""
	}
	return cfg.AuthToken
}

func newDaemonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "daemon",
		Short:        "Manage the writable agentsview daemon",
		GroupID:      groupCore,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:          "start",
		Short:        "Start the writable daemon from config",
		SilenceUsage: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStartCommand(cmd)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:          "status",
		Short:        "Show writable daemon status",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadReadOnly()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			runDaemonStatus(cmd, cfg)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:          "stop",
		Short:        "Stop the writable daemon",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadReadOnly()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			return runDaemonStop(cmd.OutOrStdout(), cfg)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:          "restart",
		Short:        "Restart the writable daemon",
		SilenceUsage: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonRestartCommand(cmd)
		},
	})
	return cmd
}

func runDaemonStartCommand(cmd *cobra.Command) error {
	dataDir, err := config.ResolveDataDir()
	if err != nil {
		return fmt.Errorf("resolving data dir: %w", err)
	}
	lifecycle, err := acquireDaemonLifecycleLock(dataDir)
	if err != nil {
		return err
	}
	defer func() { _ = lifecycle.Unlock() }()
	// Probe with the configured token before reaching for any lock: an
	// already-running require_auth daemon is what makes this command
	// idempotent, and an unauthenticated probe cannot see one.
	if rt := FindDaemonRuntime(dataDir, configuredAuthToken(dataDir)); rt != nil && !rt.ReadOnly {
		fmt.Fprintf(cmd.OutOrStdout(), "agentsview daemon already running at %s (pid %d)\n", urlFromDaemonRuntime(rt), rt.Record.PID)
		return nil
	}
	owner, err := acquireWriteOwnerLock(context.Background(), dataDir)
	if err != nil {
		return err
	}
	defer owner.Close()
	if err := rejectConfigDataDirDrift(dataDir); err != nil {
		return err
	}

	cfg, err := config.LoadMinimal()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !samePath(dataDir, cfg.DataDir) {
		return fmt.Errorf("config data_dir changed after locking: locked %s, config resolved %s", dataDir, cfg.DataDir)
	}
	if err := owner.Close(); err != nil {
		return err
	}
	owner = nil
	return runDaemonStart(cmd, cfg)
}

func runDaemonStart(cmd *cobra.Command, cfg config.Config) error {
	out := cmd.OutOrStdout()
	if rt := FindDaemonRuntime(cfg.DataDir, cfg.AuthToken); rt != nil && !rt.ReadOnly {
		fmt.Fprintf(out, "agentsview daemon already running at %s (pid %d)\n", urlFromDaemonRuntime(rt), rt.Record.PID)
		return nil
	}
	if IsLocalDaemonActive(cfg.DataDir, cfg.AuthToken) {
		reportBackgroundLaunchInProgress(out, cfg.DataDir, cfg.AuthToken)
		return nil
	}
	res, err := daemonCommands.start(cfg, []string{"serve", "--background"}, out)
	if err != nil {
		return err
	}
	// res.Cfg, not cfg: a start under require_auth mints the token, and
	// the daemon it started answers 401 to anything else.
	effective := res.Cfg
	rt := res.Runtime
	if rt == nil {
		rt = FindDaemonRuntime(effective.DataDir, effective.AuthToken)
	}
	if rt != nil {
		fmt.Fprintf(out, "agentsview daemon running at %s (pid %d)\n", urlFromDaemonRuntime(rt), rt.Record.PID)
		return nil
	}
	fmt.Fprintln(out, "agentsview daemon start requested")
	return nil
}

func runDaemonStatus(cmd *cobra.Command, cfg config.Config) {
	if _, err := os.Stat(cfg.DataDir); os.IsNotExist(err) {
		fmt.Fprintln(cmd.OutOrStdout(), "No writable agentsview daemon is running.")
		return
	}
	for _, target := range listDaemonRuntimeTargets(cfg.DataDir, cfg.AuthToken) {
		if target.Runtime == nil || target.Runtime.ReadOnly {
			continue
		}
		if target.CompatibilityErr != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "writable agentsview daemon pid %d is incompatible: %v\n", target.Record.PID, target.CompatibilityErr)
			return
		}
		if !target.Confirmed {
			fmt.Fprintf(cmd.OutOrStdout(), "writable agentsview daemon pid %d is running but not responding to health checks.\n", target.Record.PID)
			return
		}
		for _, line := range serveStatusLines(target.Runtime) {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		return
	}
	if state, ok := readStartupState(cfg.DataDir); ok {
		fmt.Fprintf(cmd.OutOrStdout(), "writable agentsview daemon is starting (pid %d, phase %s).\n", state.PID, state.Phase)
		if state.LogPath != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Logs: %s\n", state.LogPath)
		}
		return
	}
	if IsDaemonStarting(cfg.DataDir) {
		fmt.Fprintln(cmd.OutOrStdout(), "writable agentsview daemon is starting up.")
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), "No writable agentsview daemon is running.")
}

func runDaemonStop(out io.Writer, cfg config.Config) error {
	var writable []daemonRuntimeTarget
	for _, target := range listDaemonRuntimeTargets(cfg.DataDir, cfg.AuthToken) {
		if target.Runtime != nil && !target.Runtime.ReadOnly {
			writable = append(writable, target)
		}
	}
	if len(writable) == 0 {
		fmt.Fprintln(out, "No writable agentsview daemon is running.")
		return nil
	}
	if len(writable) > 1 {
		return fmt.Errorf("multiple writable agentsview daemons found for %s; refusing to choose one", cfg.DataDir)
	}
	target := writable[0]
	if target.CompatibilityErr != nil {
		return fmt.Errorf("writable agentsview daemon pid %d is incompatible: %w", target.Record.PID, target.CompatibilityErr)
	}
	if !target.Confirmed && !stopTargetConfirmed(target.Record, cfg.AuthToken) {
		return fmt.Errorf("writable agentsview daemon pid %d cannot be confirmed; refusing to stop", target.Record.PID)
	}
	if err := daemonCommands.stop(target.Record); err != nil {
		return fmt.Errorf("stopping writable agentsview daemon pid %d: %w", target.Record.PID, err)
	}
	stopOrphanedCaddyChild(out, target.Record)
	fmt.Fprintf(out, "Stopped writable agentsview daemon (pid %d).\n", target.Record.PID)
	return nil
}

// runDaemonRestartCommand stops the writable daemon and starts a new
// one. It runs under the lifecycle lock and never takes the write-owner
// lock: the daemon it has to stop is holding that one, so acquiring it
// first would fail on exactly the case restart exists for. The
// preflight below reads config and the data version, which a live
// daemon does not prevent - CheckDataVersion opens the archive
// read-only.
func runDaemonRestartCommand(cmd *cobra.Command) error {
	dataDir, err := config.ResolveDataDir()
	if err != nil {
		return fmt.Errorf("resolving data dir: %w", err)
	}
	lifecycle, err := acquireDaemonLifecycleLock(dataDir)
	if err != nil {
		return err
	}
	defer func() { _ = lifecycle.Unlock() }()
	if err := rejectConfigDataDirDrift(dataDir); err != nil {
		return err
	}

	cfg, err := config.LoadMinimal()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !samePath(dataDir, cfg.DataDir) {
		return fmt.Errorf("config data_dir changed after locking: locked %s, config resolved %s", dataDir, cfg.DataDir)
	}
	if err := validateServeConfig(cfg); err != nil {
		return fmt.Errorf("invalid serve config: %w", err)
	}
	if err := daemonCommands.checkDataVersion(cfg.DBPath); err != nil {
		return err
	}
	if err := runDaemonStop(cmd.OutOrStdout(), cfg); err != nil {
		return err
	}
	return runDaemonStart(cmd, cfg)
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil && absA == absB {
		return true
	}
	realA, errA := filepath.EvalSymlinks(a)
	realB, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && realA == realB
}

func rejectConfigDataDirDrift(lockedDataDir string) error {
	path := filepath.Join(lockedDataDir, "config.toml")
	var file struct {
		DataDir string `toml:"data_dir"`
	}
	if _, err := toml.DecodeFile(path, &file); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("loading config file: %w", err)
	}
	if file.DataDir == "" || samePath(lockedDataDir, file.DataDir) {
		return nil
	}
	return fmt.Errorf("config data_dir changed after locking: locked %s, config resolved %s", lockedDataDir, file.DataDir)
}
