// Package ghconnect implements the Phase 04 gh-connect state machine: it uses
// the locally authenticated `gh` CLI to validate, create, and claim a PRIVATE
// GitHub repository that will hold the memory backup (Phase 05 does the actual
// push). It never sets a git remote and never pushes — its only side effects
// are creating a repo (when missing) and writing the claim marker file.
//
// Every gh invocation goes through the Runner interface so unit tests can mock
// the CLI without spawning a process or touching the network.
package ghconnect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// MarkerPath is the repo-relative path of the claim marker. Its presence means
// "this repo is managed by agentsview as a memory backup target". A private
// repo carrying this marker can be linked without further checks.
const MarkerPath = ".memory-backup-marker"

// DefaultRepoName is appended to a bare namespace (owner only) to form the
// default backup repo full name `<owner>/agent-memory`.
const DefaultRepoName = "agent-memory"

// Runner abstracts a single `gh` invocation. Implementations run the real CLI
// (CLIRunner) or, in tests, return scripted output. ExitCode carries the
// process exit status so callers can distinguish gh's "not found"/404 signals
// from genuine spawn failures (returned as err).
type Runner interface {
	// Run executes `gh <args...>` and returns combined stdout, the process
	// exit code, and an error only for spawn failures (not for non-zero
	// exits, which are reported via exitCode).
	Run(ctx context.Context, args ...string) (stdout string, exitCode int, err error)
}

// Outcome enumerates the terminal states of the connect state machine.
type Outcome string

const (
	// OutcomeLinkedExisting: target already existed, was private, and either
	// carried the marker or was an empty repo we just claimed.
	OutcomeLinkedExisting Outcome = "linked_existing"
	// OutcomeCreated: target did not exist; we created it private and claimed it.
	OutcomeCreated Outcome = "created"
)

// Result is the successful return of Connect. The caller persists Repo and the
// linked flag into config. No git remote is set here (Phase 05 owns that).
type Result struct {
	// Repo is the resolved full name `<owner>/<name>` that was linked.
	Repo string `json:"repo"`
	// Outcome describes how the link was achieved.
	Outcome Outcome `json:"outcome"`
	// Private is always true on success (memory never enters a public repo).
	Private bool `json:"private"`
	// MarkerWritten reports whether this call wrote the claim marker (vs the
	// marker already being present).
	MarkerWritten bool `json:"marker_written"`
}

// ConnectError is a user-facing rejection from the state machine. Its message
// is safe to surface in the UI verbatim.
type ConnectError struct {
	// Code is a stable machine-readable reason.
	Code string
	// Message is the human-facing explanation.
	Message string
}

func (e *ConnectError) Error() string { return e.Message }

func rejection(code, msg string) *ConnectError { return &ConnectError{Code: code, Message: msg} }

// Reason codes for ConnectError.
const (
	CodeNotAuthenticated = "not_authenticated"
	CodeInvalidTarget    = "invalid_target"
	CodePublicRejected   = "public_rejected"
	CodeForeignContent   = "foreign_content"
)

// Connector runs the state machine over a Runner.
type Connector struct {
	Runner Runner
}

// New returns a Connector backed by the given Runner.
func New(r Runner) *Connector { return &Connector{Runner: r} }

// ConnectRequest holds the connect inputs.
type ConnectRequest struct {
	// Target is a bare namespace (owner), an `owner/name`, or a repo URL.
	Target string
	// MarkerContent is the body written into the marker file when this call
	// claims a repo. The caller supplies it (no time/clock is read inside this
	// package, per spec). Empty falls back to a minimal default body.
	MarkerContent string
}

// Connect runs the validation/creation/claim state machine and returns the
// linked repo. It performs at most: gh auth check, resolve current login,
// repo view, marker probe, contents probe, repo create, marker write. It never
// sets a git remote and never pushes.
func (c *Connector) Connect(ctx context.Context, req ConnectRequest) (*Result, error) {
	if c == nil || c.Runner == nil {
		return nil, errors.New("ghconnect: nil runner")
	}

	// 1. gh must be authenticated. A failed `gh auth status` => explicit reject.
	if _, code, err := c.Runner.Run(ctx, "auth", "status"); err != nil {
		return nil, rejection(CodeNotAuthenticated,
			"gh is not available: "+err.Error())
	} else if code != 0 {
		return nil, rejection(CodeNotAuthenticated,
			"gh 未登录,请先 gh auth login")
	}

	// 2. Resolve the repo full name. A bare namespace uses the default name.
	owner, name, err := c.resolveTarget(ctx, req.Target)
	if err != nil {
		return nil, err
	}
	full := owner + "/" + name

	// 3. Does the target exist? `gh repo view --json visibility,isPrivate`.
	exists, isPrivate, err := c.repoView(ctx, full)
	if err != nil {
		return nil, err
	}

	marker := req.MarkerContent
	if strings.TrimSpace(marker) == "" {
		marker = "agentsview memory backup marker\n"
	}

	if !exists {
		// 4a. Target missing: create private, then claim.
		if _, code, err := c.Runner.Run(ctx, "repo", "create", full, "--private"); err != nil {
			return nil, fmt.Errorf("gh repo create failed: %w", err)
		} else if code != 0 {
			return nil, fmt.Errorf("gh repo create exited %d for %s", code, full)
		}
		if err := c.writeMarker(ctx, full, marker); err != nil {
			return nil, err
		}
		return &Result{Repo: full, Outcome: OutcomeCreated, Private: true, MarkerWritten: true}, nil
	}

	// 4b. Target exists but is PUBLIC: memory never enters a public repo.
	if !isPrivate {
		return nil, rejection(CodePublicRejected,
			"目标 repo 是 PUBLIC,memory 不能进入公开仓库,请换一个 private repo")
	}

	// 4c. Exists + private + has marker => link directly.
	hasMarker, err := c.markerPresent(ctx, full)
	if err != nil {
		return nil, err
	}
	if hasMarker {
		return &Result{Repo: full, Outcome: OutcomeLinkedExisting, Private: true, MarkerWritten: false}, nil
	}

	// 4d. Exists + private + no marker: only claim an EMPTY repo. A repo with
	// foreign content is rejected so we never co-opt an unrelated project.
	empty, err := c.repoEmpty(ctx, full)
	if err != nil {
		return nil, err
	}
	if !empty {
		return nil, rejection(CodeForeignContent,
			"该 repo 已有内容且非本工具管理,确认请换 repo 或手动清空")
	}
	if err := c.writeMarker(ctx, full, marker); err != nil {
		return nil, err
	}
	return &Result{Repo: full, Outcome: OutcomeLinkedExisting, Private: true, MarkerWritten: true}, nil
}

// resolveTarget parses a bare namespace, `owner/name`, or repo URL into owner
// and name. A bare namespace gets the default repo name. The current gh login
// is only queried when the target omits an owner (it never does in the current
// forms, but kept for an empty target guard).
func (c *Connector) resolveTarget(ctx context.Context, target string) (owner, name string, err error) {
	t := strings.TrimSpace(target)
	if t == "" {
		return "", "", rejection(CodeInvalidTarget, "请填写 namespace 或 repo URL")
	}

	// Strip a URL down to its path: https://github.com/<owner>/<name>[.git]
	if i := strings.Index(t, "github.com"); i >= 0 {
		t = t[i+len("github.com"):]
		t = strings.TrimLeft(t, ":/")
	}
	t = strings.TrimSuffix(t, ".git")
	t = strings.Trim(t, "/")

	parts := strings.Split(t, "/")
	switch len(parts) {
	case 1:
		owner = strings.TrimSpace(parts[0])
		if owner == "" {
			return "", "", rejection(CodeInvalidTarget, "请填写 namespace 或 repo URL")
		}
		return owner, DefaultRepoName, nil
	case 2:
		owner = strings.TrimSpace(parts[0])
		name = strings.TrimSpace(parts[1])
		if owner == "" || name == "" {
			return "", "", rejection(CodeInvalidTarget, "无效的 repo 名: "+target)
		}
		return owner, name, nil
	default:
		return "", "", rejection(CodeInvalidTarget, "无效的 repo 名: "+target)
	}
}

// repoView reports whether full exists and, if so, whether it is private. A
// non-zero exit from `gh repo view` is treated as "does not exist" (the common
// case for a 404), not a hard error.
func (c *Connector) repoView(ctx context.Context, full string) (exists, isPrivate bool, err error) {
	out, code, err := c.Runner.Run(ctx, "repo", "view", full, "--json", "visibility,isPrivate")
	if err != nil {
		return false, false, fmt.Errorf("gh repo view failed: %w", err)
	}
	if code != 0 {
		// Not found / no access: treat as not existing so we fall through to create.
		return false, false, nil
	}
	var view struct {
		Visibility string `json:"visibility"`
		IsPrivate  bool   `json:"isPrivate"`
	}
	if jerr := json.Unmarshal([]byte(out), &view); jerr != nil {
		return false, false, fmt.Errorf("parsing gh repo view: %w", jerr)
	}
	priv := view.IsPrivate || strings.EqualFold(view.Visibility, "private")
	return true, priv, nil
}

// markerPresent probes the marker file via the contents API. HTTP 404 (gh
// exits non-zero) => absent; exit 0 => present.
func (c *Connector) markerPresent(ctx context.Context, full string) (bool, error) {
	owner, name := splitFull(full)
	_, code, err := c.Runner.Run(ctx, "api",
		fmt.Sprintf("/repos/%s/%s/contents/%s", owner, name, MarkerPath))
	if err != nil {
		return false, fmt.Errorf("gh api marker probe failed: %w", err)
	}
	return code == 0, nil
}

// repoEmpty probes the repo root contents. An empty array (or a 404 for a repo
// with no commits) => empty; a non-empty array => has foreign content.
func (c *Connector) repoEmpty(ctx context.Context, full string) (bool, error) {
	owner, name := splitFull(full)
	out, code, err := c.Runner.Run(ctx, "api",
		fmt.Sprintf("/repos/%s/%s/contents", owner, name))
	if err != nil {
		return false, fmt.Errorf("gh api contents probe failed: %w", err)
	}
	if code != 0 {
		// 404: a freshly created repo has no commits => treat as empty.
		return true, nil
	}
	var entries []json.RawMessage
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return true, nil
	}
	if jerr := json.Unmarshal([]byte(trimmed), &entries); jerr != nil {
		return false, fmt.Errorf("parsing gh api contents: %w", jerr)
	}
	return len(entries) == 0, nil
}

// writeMarker creates the claim marker via the contents PUT API. The content is
// base64-encoded per the GitHub contents API contract.
func (c *Connector) writeMarker(ctx context.Context, full, content string) error {
	owner, name := splitFull(full)
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	_, code, err := c.Runner.Run(ctx, "api", "--method", "PUT",
		fmt.Sprintf("/repos/%s/%s/contents/%s", owner, name, MarkerPath),
		"-f", "message=agentsview: claim memory backup marker",
		"-f", "content="+encoded,
	)
	if err != nil {
		return fmt.Errorf("gh api write marker failed: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("gh api write marker exited %d for %s", code, full)
	}
	return nil
}

// splitFull splits an already-validated `owner/name` full name.
func splitFull(full string) (owner, name string) {
	parts := strings.SplitN(full, "/", 2)
	if len(parts) != 2 {
		return full, ""
	}
	return parts[0], parts[1]
}
