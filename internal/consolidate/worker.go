package consolidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/llm"
)

// LLMClient is the narrow LLM surface the worker needs. *llm.Client satisfies
// it via ChatJSON. Tests substitute a stub so a cycle never hits the network.
type LLMClient interface {
	ChatJSON(ctx context.Context, system, user string) (string, error)
}

type llmUsageClient interface {
	ChatJSONUsage(ctx context.Context, system, user string) (string, llm.Usage, error)
}

// ScriptResult is the parsed outcome of one assist_consolidate.py invocation.
// Stdout holds the script's per-candidate result lines (`write <id> ...`,
// `skip <id> ...`, ...); Stderr holds any `assist_consolidate failed: ...`
// lines. ExitCode is the process exit status; a non-zero exit means the script
// rejected at least one candidate (it is not fatal — the worker records it).
type ScriptResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ScriptRunner executes the dotfiles consolidation script. The production
// implementation shells out to `python3 assist_consolidate.py`; tests pass a
// fake so no real process runs.
type ScriptRunner interface {
	Run(ctx context.Context, root, rawDir, decisionFile string) (ScriptResult, error)
}

// Committer commits the memory dir as a local-only git repo (never pushes).
// *memory.FileWriter-style git is wrapped behind this so tests can assert the
// commit happened without spawning git.
type Committer interface {
	Commit(ctx context.Context, message string) error
}

// Resyncer triggers an immediate incremental memory resync after a successful
// commit, so the new notes are visible without waiting for the periodic tick
// (locked decision B2). It is best-effort.
type Resyncer interface {
	Resync(ctx context.Context) error
}

// Worker orchestrates one consolidation cycle: lock -> read candidates ->
// LLM decision -> script exec -> commit -> resync -> audit. All side-effecting
// collaborators are injected so the cycle is fully testable offline.
type Worker struct {
	StagingDir   string // <dotfiles>/memory/.staging
	RawDir       string // <dotfiles>/memory/.staging/raw_memories
	DotfilesRoot string // <dotfiles> (assist_consolidate.py --root)

	LLM    LLMClient
	Script ScriptRunner
	Commit Committer
	Resync Resyncer
	Audit  *AuditLog

	now func() time.Time
}

// NewWorker builds a Worker with the default clock.
func NewWorker(
	stagingDir, rawDir, dotfilesRoot string,
	llm LLMClient, script ScriptRunner, commit Committer, resync Resyncer, audit *AuditLog,
) *Worker {
	return &Worker{
		StagingDir:   stagingDir,
		RawDir:       rawDir,
		DotfilesRoot: dotfilesRoot,
		LLM:          llm,
		Script:       script,
		Commit:       commit,
		Resync:       resync,
		Audit:        audit,
		now:          time.Now,
	}
}

// RunOnce performs a single consolidation cycle. It returns the audit record
// for the cycle and never panics on bad LLM output or a failing script: every
// recoverable problem is captured in the record's Error/Decisions and the
// cycle ends cleanly. It returns ErrLocked (and a skip record) when another
// holder owns the single-flight lock.
func (w *Worker) RunOnce(ctx context.Context) (RunRecord, error) {
	rec := RunRecord{StartedAt: w.clock().UTC().Format(time.RFC3339)}

	lock, err := AcquireLock(w.StagingDir)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			rec.Skipped = true
			rec.Error = "lock held by another instance"
			w.record(rec)
			return rec, ErrLocked
		}
		rec.Error = fmt.Sprintf("acquiring lock: %v", err)
		w.record(rec)
		return rec, err
	}
	defer lock.Release()

	candidates, err := ReadCandidates(w.RawDir)
	if err != nil {
		rec.Error = fmt.Sprintf("reading candidates: %v", err)
		w.record(rec)
		return rec, err
	}
	rec.CandidateCount = len(candidates)
	if len(candidates) == 0 {
		rec.Skipped = true
		rec.Note = "no candidates"
		w.record(rec)
		return rec, nil
	}

	ids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.effectiveID())
	}

	started := time.Now()
	raw, usage, err := w.chatJSON(ctx, systemPrompt, BuildUserPrompt(candidates))
	rec.LLMCallCount++
	rec.LLMDurationMS += int(time.Since(started).Milliseconds())
	rec.ProviderUsage = "consolidate"
	if usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		rec.LLMUsage = &LLMUsage{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens}
	}
	if err != nil {
		rec.Error = fmt.Sprintf("llm: %v", err)
		w.record(rec)
		return rec, nil
	}

	decisions, err := ParseDecisions(raw, ids)
	rec.AddCount, rec.UpdateCount, rec.SkipCount = decisionCounts(decisions)
	if err != nil {
		// Defensive parse failure: skip the cycle, audit it, never write.
		rec.Error = fmt.Sprintf("parsing decisions: %v", err)
		w.record(rec)
		return rec, nil
	}

	decisionFile, err := w.writeDecisionFile(decisions)
	if err != nil {
		rec.Error = fmt.Sprintf("writing decision file: %v", err)
		w.record(rec)
		return rec, err
	}
	defer os.Remove(decisionFile)

	res, err := w.Script.Run(ctx, w.DotfilesRoot, w.RawDir, decisionFile)
	if err != nil {
		rec.Error = fmt.Sprintf("running script: %v", err)
		w.record(rec)
		return rec, nil
	}
	rec.Decisions = mergeDecisions(decisions, res)
	rec.ScriptExitCode = res.ExitCode
	if strings.TrimSpace(res.Stderr) != "" {
		rec.ScriptErrors = splitNonEmptyLines(res.Stderr)
	}

	wrote := anyWrite(rec.Decisions)
	if wrote && w.Commit != nil {
		if err := w.Commit.Commit(ctx, w.commitMessage(rec)); err != nil {
			rec.Error = fmt.Sprintf("committing memory: %v", err)
			w.record(rec)
			return rec, nil
		}
		rec.Committed = true
		// Trigger an immediate resync so the UI sees the new notes now.
		if w.Resync != nil {
			if err := w.Resync.Resync(ctx); err != nil {
				rec.Note = fmt.Sprintf("resync failed: %v", err)
			} else {
				rec.Resynced = true
			}
		}
	}

	w.record(rec)
	return rec, nil
}

func (w *Worker) chatJSON(ctx context.Context, system, user string) (string, llm.Usage, error) {
	if c, ok := w.LLM.(llmUsageClient); ok {
		return c.ChatJSONUsage(ctx, system, user)
	}
	raw, err := w.LLM.ChatJSON(ctx, system, user)
	return raw, llm.Usage{}, err
}

func decisionCounts(decisions map[string]Decision) (adds, updates, skips int) {
	for _, decision := range decisions {
		switch decision.Action {
		case ActionADD:
			adds++
		case ActionUPDATE:
			updates++
		case ActionSKIP:
			skips++
		}
	}
	return adds, updates, skips
}

// writeDecisionFile writes the per-candidate decision map as the JSON the
// python script consumes via --decision-file, in a temp file the caller removes.
func (w *Worker) writeDecisionFile(decisions map[string]Decision) (string, error) {
	data, err := json.Marshal(decisions)
	if err != nil {
		return "", fmt.Errorf("encoding decisions: %w", err)
	}
	f, err := os.CreateTemp("", "consolidate-decision-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// commitMessage builds the local-only commit subject for a cycle. It is purely
// informational (the diff is the real audit); it names the write count.
func (w *Worker) commitMessage(rec RunRecord) string {
	writes := 0
	for _, d := range rec.Decisions {
		if isWriteResult(d.Result) {
			writes++
		}
	}
	return fmt.Sprintf("memory(consolidate): %d note(s) at %s", writes, rec.StartedAt)
}

func (w *Worker) record(rec RunRecord) {
	rec.FinishedAt = w.clock().UTC().Format(time.RFC3339)
	if w.Audit != nil {
		_ = w.Audit.Append(rec)
	}
}

func (w *Worker) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

// mergeDecisions joins each forwarded decision with the script's per-candidate
// result line (parsed from stdout), producing the audit's decision rows. A
// candidate the script did not report on keeps an empty Result.
func mergeDecisions(decisions map[string]Decision, res ScriptResult) []DecisionRecord {
	results := parseScriptResults(res.Stdout)
	ids := make([]string, 0, len(decisions))
	for id := range decisions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]DecisionRecord, 0, len(ids))
	for _, id := range ids {
		d := decisions[id]
		out = append(out, DecisionRecord{
			CandidateID: id,
			Action:      string(d.Action),
			NoteID:      d.NoteID,
			Reason:      d.Reason,
			Result:      results[id],
		})
	}
	return out
}

// parseScriptResults maps candidate id -> the script's result line. The script
// prints lines like `write <id> <path>`, `skip <id> <reason>`,
// `update <id> ...`, `soft_invalidate <id> ...`; the second token is the id.
func parseScriptResults(stdout string) map[string]string {
	out := map[string]string{}
	for _, line := range splitNonEmptyLines(stdout) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		out[fields[1]] = line
	}
	return out
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// anyWrite reports whether any decision's script result indicates a file was
// written/updated/invalidated, i.e. the memory dir changed and is worth a commit.
func anyWrite(decs []DecisionRecord) bool {
	for _, d := range decs {
		if isWriteResult(d.Result) {
			return true
		}
	}
	return false
}

// isWriteResult reports whether a script result line indicates a disk change.
func isWriteResult(result string) bool {
	r := strings.TrimSpace(result)
	return strings.HasPrefix(r, "write ") ||
		strings.HasPrefix(r, "update ") ||
		strings.HasPrefix(r, "soft_invalidate ")
}

// AuditPath returns the canonical audit jsonl path for a memory dir.
func AuditPath(memoryDir string) string {
	return filepath.Join(memoryDir, auditBasename)
}
