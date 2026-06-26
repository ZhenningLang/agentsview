package server

import (
	"context"
	"net/http"
	"strings"

	semantic "go.kenn.io/agentsview/internal/search"
)

func (s *Server) registerMemoryRecallRoutes() {
	group := newRouteGroup(s.api, "/api/v1/memory", "Memory Recall")
	post(s, group, "/recall", "Recall memory notes", s.humaRecallMemory)
}

type memoryRecallInput struct {
	Body struct {
		Query string `json:"query" doc:"Recall query"`
		TopK  int    `json:"top_k" doc:"Maximum number of hits"`
	}
}

func (s *Server) humaRecallMemory(
	ctx context.Context, in *memoryRecallInput,
) (*jsonOutput[semantic.MemoryRecallResponse], error) {
	if !isLocalhostContext(ctx) {
		return nil, apiError(http.StatusForbidden, "not available from remote clients")
	}
	llmCfg := s.cfg.ResolveUsageLLM("embed")
	res, err := semantic.MemoryRecall(ctx, s.db, s.llmClient(llmCfg), llmCfg, semantic.MemoryRecallRequest{
		Query: strings.TrimSpace(in.Body.Query),
		TopK:  in.Body.TopK,
	})
	if err != nil {
		if handled := handleHumaContextError(err); handled != nil {
			return nil, handled
		}
		return nil, serverError(err)
	}
	if res.Hits == nil {
		res.Hits = []semantic.MemoryRecallHit{}
	}
	return &jsonOutput[semantic.MemoryRecallResponse]{Body: res}, nil
}
