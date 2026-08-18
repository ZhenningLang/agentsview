// ABOUTME: `agentsview mcp` serves the archive over the Model Context
// ABOUTME: Protocol, read-only, over stdio by default or loopback HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/mcp"
	"go.kenn.io/agentsview/internal/postgres"
	"go.kenn.io/agentsview/internal/service"
)

// mcpOptions is the command line of `agentsview mcp`.
type mcpOptions struct {
	HTTPAddr          string
	HTTPAllowInsecure bool
	ServerURL         string
	ServerTokenFile   string
	PG                bool
}

// mcpBackendHooks lets tests observe which backend policy a flag set
// selects without standing up a daemon or a PostgreSQL instance.
type mcpBackendHooks struct {
	local  func(config.Config) (service.SessionService, func(), error)
	server func(config.Config, mcpOptions) (service.SessionService, func(), error)
	pg     func(config.Config) (service.SessionService, func(), error)
}

var mcpBackends = mcpBackendHooks{
	local:  newMCPLocalService,
	server: newMCPServerService,
	pg:     newMCPPGService,
}

func newMCPCommand() *cobra.Command {
	var opts mcpOptions
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve sessions to an MCP client (read-only)",
		Long: "Serve the session archive over the Model Context Protocol.\n\n" +
			"Reads only: the six tools search, list and read sessions and " +
			"report usage. Data comes from the local agentsview daemon, " +
			"which is started on demand, or from an explicit server or " +
			"PostgreSQL mirror. The archive file is never opened directly.",
		GroupID:      groupCore,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPCommand(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.HTTPAddr, "http", "",
		"Serve over HTTP on this port or host:port instead of stdio "+
			"(binds loopback unless --http-allow-insecure)")
	f.BoolVar(&opts.HTTPAllowInsecure, "http-allow-insecure", false,
		"Allow --http to bind a non-loopback address; an auth token is "+
			"still required")
	f.StringVar(&opts.ServerURL, "server", "",
		"Read from an already running agentsview server at this URL")
	f.StringVar(&opts.ServerTokenFile, "server-token-file", "",
		"File containing the bearer token for --server")
	f.BoolVar(&opts.PG, "pg", false,
		"Read from the configured PostgreSQL mirror")
	return cmd
}

// validate rejects flag combinations before any backend work.
func (o mcpOptions) validate() error {
	if o.ServerURL != "" && o.PG {
		return errors.New(
			"--server and --pg select different backends: pass only one",
		)
	}
	if o.ServerTokenFile != "" && o.ServerURL == "" {
		return errors.New("--server-token-file requires --server")
	}
	if o.HTTPAllowInsecure && o.HTTPAddr == "" {
		return errors.New("--http-allow-insecure requires --http")
	}
	return nil
}

// mcpSignalContext cancels on the signals a client or a shell uses to
// stop this process, so shutdown runs the same code path as an EOF on
// stdin rather than dropping the session mid-write.
func mcpSignalContext(
	parent context.Context,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

func runMCPCommand(cmd *cobra.Command, opts mcpOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}
	// LoadReadOnly, not LoadMinimal: an MCP server is started by a
	// client in the background and must not migrate config or persist
	// a cursor secret as a side effect of being launched.
	cfg, err := config.LoadReadOnly()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	svc, cleanup, err := newMCPService(cfg, opts)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, stop := mcpSignalContext(cmd.Context())
	defer stop()

	srv := mcp.NewServer(svc, &mcp.Options{Version: version})
	if opts.HTTPAddr == "" {
		return mcp.RunStdio(ctx, srv, nil, nil)
	}
	return runMCPHTTP(ctx, cmd, cfg, opts, srv)
}

// runMCPHTTP resolves the listener contract, binds it and serves until
// the context is cancelled.
func runMCPHTTP(
	ctx context.Context, cmd *cobra.Command, cfg config.Config,
	opts mcpOptions, srv *mcpsdk.Server,
) error {
	listener, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
		Addr:          opts.HTTPAddr,
		AllowInsecure: opts.HTTPAllowInsecure,
		Token:         cfg.AuthToken,
		RequireAuth:   cfg.RequireAuth,
	})
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listener.Addr)
	if err != nil {
		return fmt.Errorf("binding %s: %w", listener.Addr, err)
	}
	// The token goes to stderr, never to stdout: stdout is the MCP
	// stream in the default mode and a client may still be parsing it.
	if listener.TokenProvisioned {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Auth required. Token: %s\n", listener.Token)
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"agentsview mcp listening on http://%s\n", ln.Addr())
	return mcp.ServeHTTP(ctx, ln, mcp.NewHTTPHandler(srv, listener))
}

// newMCPService picks the backend named by the flags. Exactly one of
// the three policies applies, and validate has already rejected the
// combinations that would make that ambiguous.
func newMCPService(
	cfg config.Config, opts mcpOptions,
) (service.SessionService, func(), error) {
	switch {
	case opts.ServerURL != "":
		return mcpBackends.server(cfg, opts)
	case opts.PG:
		return mcpBackends.pg(cfg)
	default:
		return mcpBackends.local(cfg)
	}
}

// newMCPServerService reads from an explicit remote agentsview server.
func newMCPServerService(
	cfg config.Config, opts mcpOptions,
) (service.SessionService, func(), error) {
	url := strings.TrimSpace(opts.ServerURL)
	token := cfg.AuthToken
	if opts.ServerTokenFile != "" {
		fileToken, err := readMCPTokenFile(opts.ServerTokenFile)
		if err != nil {
			return nil, nil, err
		}
		token = fileToken
	}
	// readOnly: this service must refuse a write even if the remote
	// server would accept one. The MCP tool surface exposes no
	// mutation, and this is the second lock on that door.
	return service.NewHTTPBackend(url, token, true), func() {}, nil
}

// readMCPTokenFile loads a bearer token from disk. A file that exists
// but holds only whitespace is an error rather than "no token": it is
// almost always a half-written secret, and silently continuing
// unauthenticated is the wrong recovery.
func readMCPTokenFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading --server-token-file: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("--server-token-file %s is empty", path)
	}
	return token, nil
}

// newMCPPGService reads from the configured PostgreSQL mirror.
func newMCPPGService(
	cfg config.Config,
) (service.SessionService, func(), error) {
	pgCfg, err := cfg.ResolvePG()
	if err != nil {
		return nil, nil, fmt.Errorf("mcp --pg: %w", err)
	}
	if pgCfg.URL == "" {
		return nil, nil, errors.New(
			"mcp --pg: no PostgreSQL url configured",
		)
	}
	// The automation classifier is a package-level singleton that the
	// store reads while answering queries, so it has to be wired
	// before the store exists, exactly as `pg serve` does.
	applyClassifierConfig(cfg)
	store, err := postgres.NewStore(
		pgCfg.URL, pgCfg.Schema, pgCfg.AllowInsecure,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("mcp --pg: %w", err)
	}
	if len(cfg.CustomModelPricing) > 0 {
		store.SetCustomPricing(cfg.CustomModelPricing)
	}
	return service.NewReadOnlyBackend(store),
		func() { _ = store.Close() }, nil
}
