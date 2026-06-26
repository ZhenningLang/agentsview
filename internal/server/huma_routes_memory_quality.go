package server

import (
	"context"
	"net/http"

	"go.kenn.io/agentsview/internal/memoryquality"
)

func (s *Server) registerMemoryQualityRoutes() {
	group := newRouteGroup(s.api, "/api/v1/memory", "Memory Quality")
	get(s, group, "/quality", "Get memory quality telemetry", s.humaMemoryQuality)
}

type memoryQualityInput struct {
	Limit int `query:"limit" doc:"Max telemetry/audit rows to aggregate (0 = all)"`
}

func (s *Server) humaMemoryQuality(
	ctx context.Context,
	in *memoryQualityInput,
) (*jsonOutput[memoryquality.QualityResponse], error) {
	s.mu.RLock()
	dotfilesRoot := s.cfg.ResolveDotfilesRoot()
	dataDir := s.cfg.DataDir
	memoryDir := s.cfg.ResolveMemoryDir()
	s.mu.RUnlock()
	resp, err := memoryquality.BuildQualityResponse(dotfilesRoot, dataDir, memoryDir, in.Limit)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if resp.Telemetry.Scores == nil {
		resp.Telemetry.Scores = []float64{}
	}
	if resp.TelemetryRows == nil {
		resp.TelemetryRows = []memoryquality.TelemetryRecord{}
	}
	if resp.Extract.ProviderUsage == nil {
		resp.Extract.ProviderUsage = map[string]int{}
	}
	if resp.Consolidate.ProviderUsage == nil {
		resp.Consolidate.ProviderUsage = map[string]int{}
	}
	return &jsonOutput[memoryquality.QualityResponse]{Body: resp}, nil
}
