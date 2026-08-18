// ABOUTME: mcpLocalService is the local MCP backend: a SessionService
// ABOUTME: that resolves a running daemon per operation and never
// ABOUTME: opens the SQLite archive itself.
package main

import (
	"context"
	"errors"
	"fmt"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

// mcpLocalService reaches the archive only through a daemon.
//
// The archive has one writable owner at a time, held by the write-owner
// lock. An MCP server is long-lived, started by a client, and outlives
// individual tool calls, so opening the database here would make it a
// second process reading a file another process is mid-transaction on -
// and, on the writable path, a second owner. Every operation therefore
// resolves a live daemon and talks HTTP to it.
//
// Resolution is per operation rather than once at startup because the
// daemon's lifetime is not this process's lifetime: it can be stopped,
// restarted or replaced by an upgrade between two tool calls. A cached
// backend would keep pointing at a dead port for the rest of the
// session, and the client would see every subsequent call fail.
type mcpLocalService struct {
	cfg config.Config

	// find returns a compatible, confirmed daemon runtime, or nil.
	find func(config.Config) *DaemonRuntime

	// start launches a daemon using the canonical background start.
	start func(config.Config) (*DaemonRuntime, error)

	// newBackend builds the HTTP service for a resolved runtime.
	newBackend func(url, token string) service.SessionService
}

// newMCPLocalService builds the local backend for the MCP command.
func newMCPLocalService(
	cfg config.Config,
) (service.SessionService, func(), error) {
	return &mcpLocalService{
		cfg: cfg,
		find: func(c config.Config) *DaemonRuntime {
			return FindDaemonRuntime(c.DataDir, c.AuthToken)
		},
		start: func(c config.Config) (*DaemonRuntime, error) {
			// The canonical background start, the same one `daemon
			// start` and `serve --background` use, so the launch lock,
			// startup state, runtime record and log handling stay in
			// one place.
			return daemonCommands.start(c, []string{"serve", "--background"})
		},
		newBackend: func(url, token string) service.SessionService {
			return service.NewHTTPBackend(url, token, true)
		},
	}, func() {}, nil
}

// resolve returns a service for the current daemon, starting one if
// needed.
//
// The backend is built only from a published runtime record. A started
// process that has not published one yet is not reachable: its port is
// unknown and its database may still be migrating, so guessing an
// address here would produce connection errors that read like data
// errors to the client.
func (s *mcpLocalService) resolve() (service.SessionService, error) {
	if rt := s.find(s.cfg); rt != nil {
		return s.newBackend(urlFromDaemonRuntime(rt), s.cfg.AuthToken), nil
	}
	if _, err := s.start(s.cfg); err != nil {
		return nil, fmt.Errorf("starting the agentsview daemon: %w", err)
	}
	rt := s.find(s.cfg)
	if rt == nil {
		return nil, errors.New(
			"the agentsview daemon did not publish a runtime record; " +
				"run `agentsview daemon status` to see why",
		)
	}
	return s.newBackend(urlFromDaemonRuntime(rt), s.cfg.AuthToken), nil
}

func (s *mcpLocalService) Get(
	ctx context.Context, id string,
) (*service.SessionDetail, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.Get(ctx, id)
}

func (s *mcpLocalService) List(
	ctx context.Context, f service.ListFilter,
) (*service.SessionList, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.List(ctx, f)
}

func (s *mcpLocalService) Messages(
	ctx context.Context, id string, f service.MessageFilter,
) (*service.MessageList, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.Messages(ctx, id, f)
}

func (s *mcpLocalService) ToolCalls(
	ctx context.Context, id string,
) (*service.ToolCallList, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.ToolCalls(ctx, id)
}

func (s *mcpLocalService) Watch(
	ctx context.Context, id string,
) (<-chan service.Event, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.Watch(ctx, id)
}

func (s *mcpLocalService) Stats(
	ctx context.Context, f service.StatsFilter,
) (*service.SessionStats, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.Stats(ctx, f)
}

func (s *mcpLocalService) Search(
	ctx context.Context, req service.SearchRequest,
) (*service.SessionSearchResult, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.Search(ctx, req)
}

func (s *mcpLocalService) UsageSummary(
	ctx context.Context, req service.UsageRequest,
) (*service.UsageSummaryResult, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.UsageSummary(ctx, req)
}

// UsagePairwiseComparison completes the Phase 17 pairwise seam: the
// comparison is forwarded whole, so a caller of this backend gets the
// same metrics as the HTTP route rather than a reduced copy.
func (s *mcpLocalService) UsagePairwiseComparison(
	ctx context.Context, req service.UsagePairwiseComparisonRequest,
) (*service.PairwiseComparisonResponse, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.UsagePairwiseComparison(ctx, req)
}

func (s *mcpLocalService) SearchContent(
	ctx context.Context, req service.ContentSearchRequest,
) (*service.ContentSearchResult, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.SearchContent(ctx, req)
}

func (s *mcpLocalService) ListSecrets(
	ctx context.Context, f service.SecretListFilter,
) (*service.SecretFindingList, error) {
	svc, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return svc.ListSecrets(ctx, f)
}

// Sync and ScanSecrets are the two mutating methods on the interface.
// They are refused here rather than forwarded: this backend exists to
// serve a read-only tool surface, and a future caller that acquired it
// should fail loudly instead of writing through the daemon.
func (s *mcpLocalService) Sync(
	context.Context, service.SyncInput,
) (*service.SessionDetail, error) {
	return nil, db.ErrReadOnly
}

func (s *mcpLocalService) ScanSecrets(
	context.Context, service.SecretScanInput, func(service.SecretScanProgress),
) (*service.SecretScanSummary, error) {
	return nil, db.ErrReadOnly
}
