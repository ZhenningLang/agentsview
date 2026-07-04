package server

import (
	"context"
	"net/http"

	"go.kenn.io/agentsview/internal/extract"
)

func (s *Server) registerExtractRoutes() {
	group := newRouteGroup(s.api, "/api/v1/extract", "Extract")
	get(s, group, "/audit", "List extraction audit history", s.humaExtractAudit)
	put(s, group, "/enable", "Automatic extraction is removed", s.humaExtractEnable)
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
	return nil, apiError(http.StatusGone, "automatic memory extraction has been removed")
}
