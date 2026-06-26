package server

import (
	"context"
	"net/http"

	"go.kenn.io/agentsview/internal/extract"
)

func (s *Server) registerExtractRoutes() {
	group := newRouteGroup(s.api, "/api/v1/extract", "Extract")
	get(s, group, "/audit", "List extraction audit history", s.humaExtractAudit)
	put(s, group, "/enable", "Enable or disable background extraction", s.humaExtractEnable)
}

type extractAuditInput struct {
	Limit int `query:"limit" doc:"Max records to return, newest first (0 = all)"`
}

type extractAuditOutput struct {
	Enabled   bool                `json:"enabled"`
	Available bool                `json:"available"`
	Records   []extract.RunRecord `json:"records"`
}

type extractEnableInput struct {
	Body extractEnableRequest
}

type extractEnableRequest struct {
	Enabled bool `json:"enabled"`
}

type extractEnableOutput struct {
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
}

func (s *Server) humaExtractAudit(
	ctx context.Context, in *extractAuditInput,
) (*jsonOutput[extractAuditOutput], error) {
	s.mu.RLock()
	enabled := s.cfg.ExtractEnabled
	dataDir := s.cfg.DataDir
	s.mu.RUnlock()
	available := s.extractCtl != nil
	if available {
		enabled = s.extractCtl.Enabled()
	}
	out := extractAuditOutput{Enabled: enabled, Available: available, Records: []extract.RunRecord{}}
	if dataDir == "" {
		return &jsonOutput[extractAuditOutput]{Body: out}, nil
	}
	records, err := extract.NewAuditLog(extract.AuditPath(dataDir)).Read(in.Limit)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if records != nil {
		out.Records = records
	}
	return &jsonOutput[extractAuditOutput]{Body: out}, nil
}

func (s *Server) humaExtractEnable(
	ctx context.Context, in *extractEnableInput,
) (*jsonOutput[extractEnableOutput], error) {
	if s.extractCtl == nil {
		return nil, apiError(http.StatusNotImplemented,
			"background extraction is not available in this mode "+
				"(no writable memory staging dir); set AGENTSVIEW_EXTRACT_ENABLED via config")
	}
	if s.db.ReadOnly() {
		return nil, apiError(http.StatusNotImplemented,
			"settings cannot be modified in read-only mode")
	}
	s.mu.Lock()
	err := s.cfg.SaveSettings(map[string]any{"extract_enabled": in.Body.Enabled})
	s.mu.Unlock()
	if err != nil {
		return nil, internalError("save extract setting", err)
	}
	s.extractCtl.SetEnabled(in.Body.Enabled)
	return &jsonOutput[extractEnableOutput]{Body: extractEnableOutput{Enabled: s.extractCtl.Enabled(), Available: true}}, nil
}
