package server

import (
	"context"
	"encoding/base64"
	"net/http"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Server) registerMemoryRoutes() {
	group := newRouteGroup(s.api, "/api/v1/memories", "Memories")

	get(s, group, "", "List user-memory notes", s.humaListMemories)
	get(s, group, "/{path}", "Get one memory note", s.humaGetMemory)
}

type memoriesListInput struct {
	ProblemType   string `query:"problem_type" doc:"Filter by frontmatter problem_type"`
	Type          string `query:"type" doc:"Filter by frontmatter type"`
	Status        string `query:"status" doc:"Filter by frontmatter status"`
	OriginSession string `query:"origin_session" doc:"Filter by originating session id"`
	Q             string `query:"q" doc:"Full-text query over the note body"`
}

type memoriesListOutput struct {
	Memories []db.Memory `json:"memories"`
}

func (s *Server) humaListMemories(
	ctx context.Context, in *memoriesListInput,
) (*jsonOutput[memoriesListOutput], error) {
	memories, err := s.db.ListMemories(ctx, db.MemoryFilter{
		ProblemType:   in.ProblemType,
		Type:          in.Type,
		Status:        in.Status,
		OriginSession: in.OriginSession,
		Q:             in.Q,
	})
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if memories == nil {
		memories = []db.Memory{}
	}
	return &jsonOutput[memoriesListOutput]{Body: memoriesListOutput{Memories: memories}}, nil
}

type memoryGetInput struct {
	// Path is the URL-safe base64 encoding of the note's rel_path. rel_path
	// contains slashes (e.g. "user/foo.md"), which cannot ride in a single
	// path segment, so callers encode it and the handler decodes it back.
	Path string `path:"path" doc:"URL-safe base64 of the memory rel_path"`
}

func (s *Server) humaGetMemory(
	ctx context.Context, in *memoryGetInput,
) (*jsonOutput[db.Memory], error) {
	raw, err := base64.RawURLEncoding.DecodeString(in.Path)
	if err != nil {
		return nil, apiError(http.StatusBadRequest, "invalid memory path encoding")
	}
	memory, err := s.db.GetMemory(ctx, string(raw))
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if memory == nil {
		return nil, apiError(http.StatusNotFound, "memory not found")
	}
	return &jsonOutput[db.Memory]{Body: *memory}, nil
}
