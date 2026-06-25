package server

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/memory"
)

func (s *Server) registerMemoryRoutes() {
	group := newRouteGroup(s.api, "/api/v1/memories", "Memories")

	get(s, group, "", "List user-memory notes", s.humaListMemories)
	get(s, group, "/{path}", "Get one memory note", s.humaGetMemory)
	get(s, group, "/{path}/raw",
		"Get one memory note's raw on-disk content and sha", s.humaGetMemoryRaw)
	put(s, group, "/{path}", "Write back one memory note", s.humaPutMemory)
	get(s, group, "/{path}/history",
		"List git history for one memory note", s.humaMemoryHistory)
	get(s, group, "/{path}/history/{commit}",
		"Get one memory note at a specific commit", s.humaMemoryAtCommit)
	post(s, group, "/{path}/revert",
		"Revert one memory note to a commit", s.humaRevertMemory)
}

type memoriesListInput struct {
	Source        string `query:"source" doc:"Filter by data source (cross-agent | cc-native)"`
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
		Source:        in.Source,
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

// decodeMemoryPath decodes the URL-safe base64 path segment into a rel_path.
func decodeMemoryPath(encoded string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", apiError(http.StatusBadRequest, "invalid memory path encoding")
	}
	return string(raw), nil
}

func (s *Server) humaGetMemory(
	ctx context.Context, in *memoryGetInput,
) (*jsonOutput[db.Memory], error) {
	relPath, err := decodeMemoryPath(in.Path)
	if err != nil {
		return nil, err
	}
	memory, err := s.db.GetMemory(ctx, relPath)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if memory == nil {
		return nil, apiError(http.StatusNotFound, "memory not found")
	}
	return &jsonOutput[db.Memory]{Body: *memory}, nil
}

type memoryRawOutput struct {
	// Content is the verbatim on-disk file content the editor loads into the
	// edit form. SHA is its sha256, echoed back as base_sha on write/revert.
	Content string `json:"content"`
	SHA     string `json:"sha"`
}

// humaGetMemoryRaw returns the verbatim on-disk content plus its sha256. The
// editor needs the raw file (not the DB-parsed view) so it can round-trip
// untracked frontmatter keys and present a base_sha that matches what Write
// gates on.
func (s *Server) humaGetMemoryRaw(
	ctx context.Context, in *memoryGetInput,
) (*jsonOutput[memoryRawOutput], error) {
	relPath, err := decodeMemoryPath(in.Path)
	if err != nil {
		return nil, err
	}
	w, err := s.memoryWriter()
	if err != nil {
		return nil, err
	}
	content, sha, err := w.Read(ctx, relPath)
	if err != nil {
		if errors.Is(err, memory.ErrPathTraversal) {
			return nil, apiError(http.StatusBadRequest, err.Error())
		}
		return nil, apiError(http.StatusNotFound, "memory not found")
	}
	return &jsonOutput[memoryRawOutput]{
		Body: memoryRawOutput{Content: content, SHA: sha},
	}, nil
}

// memoryWriter resolves the effective memory dir and builds a Writer, or
// returns a 404 when the memory feature is disabled (no dir configured).
func (s *Server) memoryWriter() (*memory.FileWriter, error) {
	dir := s.cfg.ResolveMemoryDir()
	if dir == "" {
		return nil, apiError(http.StatusNotFound, "memory not configured")
	}
	return memory.NewWriter(dir), nil
}

// resyncMemory best-effort refreshes the DB cache from disk after a write.
// The on-disk *.md files are the SSOT; this just keeps the read cache in
// step instead of waiting for the periodic sync. It is fail-soft.
func (s *Server) resyncMemory(ctx context.Context) {
	w, ok := s.db.(memory.Writer)
	if !ok {
		return
	}
	dir := s.cfg.ResolveMemoryDir()
	if dir == "" {
		return
	}
	_ = memory.NewSyncer(dir, w, nil).Sync(ctx)
}

// writeError maps writer-layer errors to HTTP statuses.
func memoryWriteError(err error) error {
	switch {
	case errors.Is(err, memory.ErrPathTraversal):
		return apiError(http.StatusBadRequest, err.Error())
	case errors.Is(err, memory.ErrConflict):
		return apiError(http.StatusConflict, "modified on disk, reload")
	default:
		return apiError(http.StatusInternalServerError, err.Error())
	}
}

type memoryPutInput struct {
	Path string `path:"path" doc:"URL-safe base64 of the memory rel_path"`
	Body struct {
		// Content is the full reconstructed file content (frontmatter + body)
		// to write verbatim. The caller assembles it from the edited fields.
		Content string `json:"content" doc:"Full file content to write"`
		// BaseSHA is the sha256 (hex) of the file content the editor read, used
		// for optimistic concurrency. Empty means the caller expects a new file.
		BaseSHA string `json:"base_sha" doc:"sha256 of the content the editor read"`
	}
}

type memoryWriteOutput struct {
	// SHA is the sha256 (hex) of the newly written content. The editor uses
	// it as the base_sha for the next write.
	SHA string `json:"sha"`
}

func (s *Server) humaPutMemory(
	ctx context.Context, in *memoryPutInput,
) (*jsonOutput[memoryWriteOutput], error) {
	relPath, err := decodeMemoryPath(in.Path)
	if err != nil {
		return nil, err
	}
	w, err := s.memoryWriter()
	if err != nil {
		return nil, err
	}
	sha, err := w.Write(ctx, memory.WriteRequest{
		RelPath: relPath,
		Content: in.Body.Content,
		BaseSHA: in.Body.BaseSHA,
	})
	if err != nil {
		return nil, memoryWriteError(err)
	}
	s.resyncMemory(ctx)
	return &jsonOutput[memoryWriteOutput]{Body: memoryWriteOutput{SHA: sha}}, nil
}

type memoryHistoryOutput struct {
	History []memory.HistoryEntry `json:"history"`
}

func (s *Server) humaMemoryHistory(
	ctx context.Context, in *memoryGetInput,
) (*jsonOutput[memoryHistoryOutput], error) {
	relPath, err := decodeMemoryPath(in.Path)
	if err != nil {
		return nil, err
	}
	w, err := s.memoryWriter()
	if err != nil {
		return nil, err
	}
	hist, err := w.History(ctx, relPath)
	if err != nil {
		return nil, memoryWriteError(err)
	}
	if hist == nil {
		hist = []memory.HistoryEntry{}
	}
	return &jsonOutput[memoryHistoryOutput]{Body: memoryHistoryOutput{History: hist}}, nil
}

type memoryCommitInput struct {
	Path   string `path:"path" doc:"URL-safe base64 of the memory rel_path"`
	Commit string `path:"commit" doc:"Git commit hash"`
}

type memoryContentOutput struct {
	Content string `json:"content"`
}

func (s *Server) humaMemoryAtCommit(
	ctx context.Context, in *memoryCommitInput,
) (*jsonOutput[memoryContentOutput], error) {
	relPath, err := decodeMemoryPath(in.Path)
	if err != nil {
		return nil, err
	}
	w, err := s.memoryWriter()
	if err != nil {
		return nil, err
	}
	content, err := w.FileAtCommit(ctx, relPath, in.Commit)
	if err != nil {
		if errors.Is(err, memory.ErrPathTraversal) {
			return nil, apiError(http.StatusBadRequest, err.Error())
		}
		return nil, apiError(http.StatusNotFound, err.Error())
	}
	return &jsonOutput[memoryContentOutput]{Body: memoryContentOutput{Content: content}}, nil
}

type memoryRevertInput struct {
	Path string `path:"path" doc:"URL-safe base64 of the memory rel_path"`
	Body struct {
		Commit  string `json:"commit" doc:"Commit to revert the note to"`
		BaseSHA string `json:"base_sha" doc:"sha256 of the content the editor read"`
	}
}

func (s *Server) humaRevertMemory(
	ctx context.Context, in *memoryRevertInput,
) (*jsonOutput[memoryWriteOutput], error) {
	relPath, err := decodeMemoryPath(in.Path)
	if err != nil {
		return nil, err
	}
	w, err := s.memoryWriter()
	if err != nil {
		return nil, err
	}
	sha, err := w.Revert(ctx, relPath, in.Body.Commit, in.Body.BaseSHA)
	if err != nil {
		if errors.Is(err, memory.ErrPathTraversal) ||
			errors.Is(err, memory.ErrConflict) {
			return nil, memoryWriteError(err)
		}
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	s.resyncMemory(ctx)
	return &jsonOutput[memoryWriteOutput]{Body: memoryWriteOutput{SHA: sha}}, nil
}
