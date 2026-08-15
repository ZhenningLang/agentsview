// ABOUTME: session command group root — programmatic CLI
// ABOUTME: surface for the SessionService interface.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/service"
)

func newSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "session",
		Short:        "Programmatic access to session data",
		GroupID:      groupData,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	registerFormatFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().String(
		"server", "",
		"Remote daemon URL",
	)
	cmd.PersistentFlags().String(
		"server-token-file", "",
		"Read bearer token for --server from file",
	)

	cmd.AddCommand(newSessionGetCommand())
	cmd.AddCommand(newSessionUsageCommand())
	cmd.AddCommand(newSessionListCommand())
	cmd.AddCommand(newSessionMessagesCommand())
	cmd.AddCommand(newSessionToolCallsCommand())
	cmd.AddCommand(newSessionExportCommand())
	cmd.AddCommand(newSessionSyncCommand())
	cmd.AddCommand(newSessionWatchCommand())
	cmd.AddCommand(newSessionSearchCommand())
	return cmd
}

// resolveService constructs the SessionService matching the
// current transport: HTTP when a daemon is discoverable, direct
// SQLite otherwise. Callers MUST defer the returned cleanup.
func resolveService(
	cmd *cobra.Command,
) (service.SessionService, func(), error) {
	remote, _ := cmd.Flags().GetString("server")
	if remote != "" {
		if err := rejectServerPGConflict(cmd); err != nil {
			return nil, nil, err
		}
		token, err := explicitServerToken(cmd)
		if err != nil {
			return nil, nil, err
		}
		return service.NewHTTPBackend(remote, token, false), func() {}, nil
	}
	cfg, err := config.LoadPFlags(cmd.Flags())
	if err != nil {
		return nil, nil, fmt.Errorf(
			"loading config: %w", err,
		)
	}
	tr, err := detectTransport(cfg.DataDir, cfg.AuthToken, 0)
	if err != nil {
		return nil, nil, err
	}
	return newService(cfg, tr)
}

// resolveWritableService constructs a write-capable SessionService:
// HTTP when a writable daemon is reachable, otherwise a direct
// backend wired with a real sync.Engine. It refuses read-only
// daemons (pg serve) and unreachable writable daemons. Callers MUST defer the returned
// cleanup. Read-only commands should use resolveService instead.
func resolveWritableService(
	cmd *cobra.Command,
) (service.SessionService, func(), error) {
	if remote, _ := cmd.Flags().GetString("server"); remote != "" {
		if err := rejectServerPGConflict(cmd); err != nil {
			return nil, nil, err
		}
		token, err := explicitServerToken(cmd)
		if err != nil {
			return nil, nil, err
		}
		return service.NewHTTPBackend(remote, token, false), func() {}, nil
	}
	cfg, err := config.LoadPFlags(cmd.Flags())
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}
	tr, err := detectTransport(cfg.DataDir, cfg.AuthToken, 0)
	if err != nil {
		return nil, nil, err
	}
	if tr.Mode == transportHTTP && tr.ReadOnly {
		return nil, nil, fmt.Errorf(
			"daemon at %s is read-only (pg serve); cannot write: stop "+
				"'pg serve' and use the local DB, or start a local daemon",
			tr.URL,
		)
	}
	if tr.Mode == transportDirect && tr.DirectReadOnly {
		return nil, nil, errors.New(
			"local daemon owns the SQLite archive but is not responding; " +
				"refusing to write directly. Retry once the daemon is " +
				"reachable, or stop it to write locally",
		)
	}
	return syncService(cfg, tr)
}

func explicitServerToken(cmd *cobra.Command) (string, error) {
	if path, _ := cmd.Flags().GetString("server-token-file"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading --server-token-file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return strings.TrimSpace(os.Getenv("AGENTSVIEW_SERVER_TOKEN")), nil
}

func rejectServerPGConflict(cmd *cobra.Command) error {
	if cmd.Flags().Lookup("pg") != nil && cmd.Flags().Changed("pg") {
		return errors.New("--server and --pg are mutually exclusive")
	}
	return nil
}
