package server

import (
	"context"
	"net/http"

	"go.kenn.io/agentsview/internal/consolidate"
)

// registerConsolidateRoutes exposes the read-only consolidation audit history.
// The worker itself runs as a background timer (cmd/agentsview); these routes
// only surface its append-only jsonl audit so the UI can show each cycle's
// per-candidate decisions, including rejected ones.
func (s *Server) registerConsolidateRoutes() {
	group := newRouteGroup(s.api, "/api/v1/consolidate", "Consolidate")
	get(s, group, "/audit", "List consolidation audit history", s.humaConsolidateAudit)
	put(s, group, "/enable", "Automatic consolidation is removed", s.humaConsolidateEnable)
}

type consolidateAuditInput struct {
	Limit int `query:"limit" doc:"Max records to return, newest first (0 = all)"`
}

type consolidateAuditOutput struct {
	// Enabled reports whether the worker is currently armed. When the worker
	// is running it reflects the live runtime state (which the UI can toggle);
	// otherwise it mirrors config.
	Enabled bool `json:"enabled"`
	// Available reports whether runtime enable/disable is wired (a running
	// worker controller exists). When false the UI shows config/env as the
	// only way to arm it.
	Available bool                    `json:"available"`
	Records   []consolidate.RunRecord `json:"records"`
}

type consolidateEnableInput struct {
	Body consolidateEnableRequest
}

type consolidateEnableRequest struct {
	// Enabled is the desired armed state. Enabling persists the choice and
	// fires one immediate consolidation cycle; disabling stops further cycles.
	Enabled bool `json:"enabled"`
}

type consolidateEnableOutput struct {
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
}

// humaConsolidateAudit returns the consolidation run history newest-first. It
// is fail-open: when no memory dir is configured (audit feature disabled) it
// returns an empty list rather than an error, matching the rest of the memory
// surface.
func (s *Server) humaConsolidateAudit(
	ctx context.Context, in *consolidateAuditInput,
) (*jsonOutput[consolidateAuditOutput], error) {
	s.mu.RLock()
	enabled := s.cfg.ConsolidateEnabled
	dir := s.cfg.ResolveMemoryDir()
	s.mu.RUnlock()
	available := s.consolidateCtl != nil
	if available {
		// Live runtime state wins so a runtime toggle is reflected without a
		// config write race or restart.
		enabled = s.consolidateCtl.Enabled()
	}
	out := consolidateAuditOutput{
		Enabled:   enabled,
		Available: available,
		Records:   []consolidate.RunRecord{},
	}
	if dir == "" {
		return &jsonOutput[consolidateAuditOutput]{Body: out}, nil
	}
	records, err := consolidate.NewAuditLog(consolidate.AuditPath(dir)).Read(in.Limit)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if records != nil {
		out.Records = records
	}
	return &jsonOutput[consolidateAuditOutput]{Body: out}, nil
}

// humaConsolidateEnable arms or disarms the background consolidation worker at
// runtime (locked decision A2: the UI must be able to enable it, and enabling
// must make it auto-run). It persists the choice to config so the state
// survives a restart, then flips the live controller — enabling fires one
// immediate cycle so the user does not wait a full interval for the first run.
//
// It requires a running controller (the worker must have started: a writable
// local store with a configured memory dir) and a writable config, so the
// effect is real rather than a value that silently does nothing.
func (s *Server) humaConsolidateEnable(
	ctx context.Context, in *consolidateEnableInput,
) (*jsonOutput[consolidateEnableOutput], error) {
	return nil, apiError(http.StatusGone, "automatic memory consolidation has been removed")
}
