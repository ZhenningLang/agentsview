package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"go.kenn.io/agentsview/internal/ghconnect"
)

// registerMemoryBackupRoutes exposes the Phase 04 gh-connect config endpoints:
// GET the current backup link status and POST to validate/create/claim a
// PRIVATE GitHub repo as the memory backup target. The connect endpoint never
// sets a git remote and never pushes — Phase 05 owns the actual backup.
func (s *Server) registerMemoryBackupRoutes() {
	group := newRouteGroup(s.api, "/api/v1/config/memory-backup", "MemoryBackup")
	get(s, group, "", "Get memory backup link status", s.humaGetMemoryBackup)
	post(s, group, "/connect", "Validate/create/claim the memory backup repo", s.humaConnectMemoryBackup)
}

type memoryBackupStatusResponse struct {
	// Repo is the resolved `<owner>/<name>` of the linked backup repo, empty
	// when none is configured.
	Repo string `json:"repo"`
	// Linked reports whether the repo was validated/claimed (private + marker).
	Linked bool `json:"linked"`
}

type connectMemoryBackupInput struct {
	Body connectMemoryBackupRequest
}

type connectMemoryBackupRequest struct {
	// NamespaceOrURL is a bare namespace (owner), an `owner/name`, or a repo
	// URL. A bare namespace resolves to `<owner>/agent-memory`.
	NamespaceOrURL string `json:"namespace_or_url" required:"true" minLength:"1" doc:"Namespace, owner/name, or repo URL"`
	// MarkerContent is the optional body written into the claim marker. The
	// client supplies any timestamp it wants embedded (the server reads no
	// clock for the marker, per the locked spec).
	MarkerContent string `json:"marker_content,omitempty" doc:"Optional claim-marker body (client supplies any timestamp)"`
}

type connectMemoryBackupResponse struct {
	Repo string `json:"repo"`
	// Outcome is "linked_existing" or "created".
	Outcome       string `json:"outcome"`
	Private       bool   `json:"private"`
	MarkerWritten bool   `json:"marker_written"`
	Linked        bool   `json:"linked"`
}

// humaGetMemoryBackup returns the persisted backup link status.
func (s *Server) humaGetMemoryBackup(
	_ context.Context, _ *emptyInput,
) (*jsonOutput[memoryBackupStatusResponse], error) {
	s.mu.RLock()
	repo := s.cfg.MemoryBackupRepo
	linked := s.cfg.MemoryBackupLinked
	s.mu.RUnlock()
	return &jsonOutput[memoryBackupStatusResponse]{
		Body: memoryBackupStatusResponse{Repo: repo, Linked: linked},
	}, nil
}

// humaConnectMemoryBackup runs the gh-connect state machine and, on success,
// persists the resolved repo full name + linked flag into config. It is
// local-only and writable-only (mirrors the LLM/consolidate config gates):
// connecting a backup target is a side-effecting, gh-credentialed action that
// must never be driven by a remote client.
func (s *Server) humaConnectMemoryBackup(
	ctx context.Context, in *connectMemoryBackupInput,
) (*jsonOutput[connectMemoryBackupResponse], error) {
	if err := requireLocalLLMRequest(ctx); err != nil {
		return nil, err
	}
	if s.db.ReadOnly() {
		return nil, apiError(http.StatusForbidden, "not available in read-only mode")
	}
	target := strings.TrimSpace(in.Body.NamespaceOrURL)
	if target == "" {
		return nil, apiError(http.StatusBadRequest, "namespace_or_url required")
	}

	runner := s.ghRunner
	if runner == nil {
		runner = ghconnect.CLIRunner{}
	}
	res, err := ghconnect.New(runner).Connect(ctx, ghconnect.ConnectRequest{
		Target:        target,
		MarkerContent: in.Body.MarkerContent,
	})
	if err != nil {
		var ce *ghconnect.ConnectError
		if errors.As(err, &ce) {
			// User-facing rejection (not authed, public, foreign content,
			// invalid target): surface verbatim so the UI shows the reason.
			return nil, apiError(http.StatusBadRequest, ce.Message)
		}
		return nil, internalError("connect memory backup", err)
	}

	s.mu.Lock()
	saveErr := s.cfg.SaveSettings(map[string]any{
		"memory_backup_repo":   res.Repo,
		"memory_backup_linked": true,
	})
	s.mu.Unlock()
	if saveErr != nil {
		return nil, internalError("save memory backup config", saveErr)
	}

	return &jsonOutput[connectMemoryBackupResponse]{
		Body: connectMemoryBackupResponse{
			Repo:          res.Repo,
			Outcome:       string(res.Outcome),
			Private:       res.Private,
			MarkerWritten: res.MarkerWritten,
			Linked:        true,
		},
	}, nil
}
