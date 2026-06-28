package server

import (
	"context"
	"net/http"

	"go.kenn.io/agentsview/internal/synthesize"
)

// registerSynthesizeRoutes exposes the topic-synthesis audit history and a
// runtime enable toggle, mirroring the consolidate routes.
func (s *Server) registerSynthesizeRoutes() {
	group := newRouteGroup(s.api, "/api/v1/synthesize", "Synthesize")
	get(s, group, "/audit", "List topic-synthesis audit history", s.humaSynthesizeAudit)
	put(s, group, "/enable", "Enable or disable background topic synthesis", s.humaSynthesizeEnable)
}

type synthesizeAuditInput struct {
	Limit int `query:"limit" doc:"Max records to return, newest first (0 = all)"`
}

type synthesizeAuditOutput struct {
	Enabled   bool                   `json:"enabled"`
	Available bool                   `json:"available"`
	Records   []synthesize.RunRecord `json:"records"`
}

type synthesizeEnableInput struct {
	Body synthesizeEnableRequest
}

type synthesizeEnableRequest struct {
	Enabled bool `json:"enabled"`
}

type synthesizeEnableOutput struct {
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
}

func (s *Server) humaSynthesizeAudit(
	ctx context.Context, in *synthesizeAuditInput,
) (*jsonOutput[synthesizeAuditOutput], error) {
	s.mu.RLock()
	enabled := s.cfg.SynthesizeEnabled
	dir := s.cfg.ResolveMemoryDir()
	s.mu.RUnlock()
	available := s.synthesizeCtl != nil
	if available {
		enabled = s.synthesizeCtl.Enabled()
	}
	out := synthesizeAuditOutput{Enabled: enabled, Available: available, Records: []synthesize.RunRecord{}}
	if dir == "" {
		return &jsonOutput[synthesizeAuditOutput]{Body: out}, nil
	}
	records, err := synthesize.NewAuditLog(synthesize.AuditPath(dir)).Read(in.Limit)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if records != nil {
		out.Records = records
	}
	return &jsonOutput[synthesizeAuditOutput]{Body: out}, nil
}

func (s *Server) humaSynthesizeEnable(
	ctx context.Context, in *synthesizeEnableInput,
) (*jsonOutput[synthesizeEnableOutput], error) {
	if s.synthesizeCtl == nil {
		return nil, apiError(http.StatusNotImplemented,
			"background synthesis is not available in this mode (no writable memory dir)")
	}
	if s.db.ReadOnly() {
		return nil, apiError(http.StatusNotImplemented, "settings cannot be modified in read-only mode")
	}
	s.mu.Lock()
	err := s.cfg.SaveSettings(map[string]any{"synthesize_enabled": in.Body.Enabled})
	s.mu.Unlock()
	if err != nil {
		return nil, internalError("save synthesize setting", err)
	}
	s.synthesizeCtl.SetEnabled(in.Body.Enabled)
	return &jsonOutput[synthesizeEnableOutput]{
		Body: synthesizeEnableOutput{Enabled: s.synthesizeCtl.Enabled(), Available: true},
	}, nil
}
