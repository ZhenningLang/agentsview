package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"go.kenn.io/agentsview/internal/backup"
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
	get(s, group, "/push-status", "Get background backup-push status", s.humaBackupPushStatus)
	put(s, group, "/enable", "Enable or disable background backup push", s.humaBackupPushEnable)
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

type backupPushStatusResponse struct {
	// Enabled reflects the live runtime armed state when a worker controller is
	// running, otherwise the persisted config value.
	Enabled bool `json:"enabled"`
	// Available reports whether runtime enable/disable is wired (a running
	// worker exists). When false the UI shows config/env as the only way to arm.
	Available bool `json:"available"`
	// Repo is the configured backup target (informational).
	Repo string `json:"repo,omitempty"`
	// LastAttemptAt / LastSuccessAt / LastError* surface the latest cycle so the
	// UI can show a green (last success) or red (last error) indicator.
	LastAttemptAt string `json:"last_attempt_at,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LastErrorAt   string `json:"last_error_at,omitempty"`
}

type backupPushEnableInput struct {
	Body backupPushEnableRequest
}

type backupPushEnableRequest struct {
	// Enabled is the desired armed state. Enabling persists the choice and fires
	// one immediate backup-push cycle; disabling stops further cycles.
	Enabled bool `json:"enabled"`
}

type backupPushEnableOutput struct {
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
}

// humaBackupPushStatus returns the latest backup-push status (last success /
// last error) plus the live enabled/available flags. It is fail-open: a missing
// status file reads as the never-run zero state rather than an error.
func (s *Server) humaBackupPushStatus(
	_ context.Context, _ *emptyInput,
) (*jsonOutput[backupPushStatusResponse], error) {
	s.mu.RLock()
	enabled := s.cfg.BackupEnabled
	dataDir := s.cfg.DataDir
	repo := s.cfg.MemoryBackupRepo
	s.mu.RUnlock()

	available := s.backupCtl != nil
	if available {
		enabled = s.backupCtl.Enabled()
	}
	out := backupPushStatusResponse{Enabled: enabled, Available: available, Repo: repo}

	st, err := backup.NewStatusStore(backup.StatusPath(dataDir)).Read()
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	out.LastAttemptAt = st.LastAttemptAt
	out.LastSuccessAt = st.LastSuccessAt
	out.LastError = st.LastError
	out.LastErrorAt = st.LastErrorAt
	if st.Repo != "" {
		out.Repo = st.Repo
	}
	return &jsonOutput[backupPushStatusResponse]{Body: out}, nil
}

// humaBackupPushEnable arms or disarms the background backup-push worker at
// runtime. It persists the choice to config (so it survives a restart) and then
// flips the live controller — enabling fires one immediate push cycle. A running
// controller is required (a configured backup repo + writable local store), so
// the toggle has a real effect rather than silently doing nothing.
func (s *Server) humaBackupPushEnable(
	ctx context.Context, in *backupPushEnableInput,
) (*jsonOutput[backupPushEnableOutput], error) {
	if err := requireLocalLLMRequest(ctx); err != nil {
		return nil, err
	}
	if s.backupCtl == nil {
		return nil, apiError(http.StatusNotImplemented,
			"background backup push is not available in this mode "+
				"(no backup repo configured or read-only store); "+
				"connect a private repo and set AGENTSVIEW_BACKUP_ENABLED via config")
	}
	if s.db.ReadOnly() {
		return nil, apiError(http.StatusForbidden, "not available in read-only mode")
	}
	s.mu.Lock()
	err := s.cfg.SaveSettings(map[string]any{"backup_enabled": in.Body.Enabled})
	s.mu.Unlock()
	if err != nil {
		return nil, internalError("save backup setting", err)
	}
	s.backupCtl.SetEnabled(in.Body.Enabled)
	return &jsonOutput[backupPushEnableOutput]{
		Body: backupPushEnableOutput{
			Enabled:   s.backupCtl.Enabled(),
			Available: true,
		},
	}, nil
}
