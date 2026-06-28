package consolidate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// auditBasename is the append-only jsonl audit log written alongside the
// memory notes. A file (not a DB table) is used deliberately so the audit
// trail needs no three-backend schema parity (locked decision in the spec).
const auditBasename = ".consolidate-audit.jsonl"

// DecisionRecord is the audit row for one candidate in a cycle: the action the
// LLM chose plus the result line the python script reported. A rejected
// candidate keeps a `skip <id> ...` result so the UI can show why it was not
// written.
type DecisionRecord struct {
	CandidateID string `json:"candidate_id"`
	Action      string `json:"action"`
	NoteID      string `json:"note_id,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Result      string `json:"result,omitempty"`
}

// RunRecord is one consolidation cycle's audit entry.
type RunRecord struct {
	StartedAt       string           `json:"started_at"`
	FinishedAt      string           `json:"finished_at,omitempty"`
	CandidateCount  int              `json:"candidate_count"`
	AddCount        int              `json:"add_count,omitempty"`
	UpdateCount     int              `json:"update_count,omitempty"`
	SkipCount       int              `json:"skip_count,omitempty"`
	DeleteCount     int              `json:"delete_count,omitempty"`
	InvalidateCount int              `json:"invalidate_count,omitempty"`
	Decisions       []DecisionRecord `json:"decisions,omitempty"`
	ScriptExitCode  int              `json:"script_exit_code,omitempty"`
	ScriptErrors    []string         `json:"script_errors,omitempty"`
	DrainedCount    int              `json:"drained_count,omitempty"`
	Committed       bool             `json:"committed"`
	Resynced        bool             `json:"resynced"`
	Skipped         bool             `json:"skipped,omitempty"`
	Note            string           `json:"note,omitempty"`
	Error           string           `json:"error,omitempty"`
	LLMDurationMS   int              `json:"llm_duration_ms,omitempty"`
	LLMCallCount    int              `json:"llm_call_count,omitempty"`
	ProviderUsage   string           `json:"provider_usage,omitempty"`
	LLMUsage        *LLMUsage        `json:"llm_usage,omitempty"`
	LLMCost         *LLMCost         `json:"llm_cost,omitempty"`
}

type LLMUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type LLMCost struct {
	Currency string `json:"currency,omitempty"`
	Amount   string `json:"amount,omitempty"`
}

// AuditLog is a concurrency-safe append-only jsonl writer/reader for run
// records. Append is atomic per line under the mutex so concurrent cycles
// never interleave a record.
type AuditLog struct {
	path string
	mu   sync.Mutex
}

// NewAuditLog builds an AuditLog at the given jsonl path.
func NewAuditLog(path string) *AuditLog {
	return &AuditLog{path: path}
}

// Append writes one record as a single jsonl line, creating the file and
// parent dir if needed.
func (a *AuditLog) Append(rec RunRecord) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return fmt.Errorf("creating audit dir: %w", err)
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("opening audit log: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encoding audit record: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing audit record: %w", err)
	}
	return nil
}

// Read returns all audit records newest-first, up to limit (limit <= 0 returns
// all). A missing log yields an empty slice (not an error): a worker that has
// never run is a valid empty state. Unparseable lines are skipped fail-soft.
func (a *AuditLog) Read(limit int) ([]RunRecord, error) {
	if a == nil {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.Open(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunRecord{}, nil
		}
		return nil, fmt.Errorf("opening audit log: %w", err)
	}
	defer f.Close()

	var all []RunRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec RunRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		all = append(all, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading audit log: %w", err)
	}
	// Reverse to newest-first.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []RunRecord{}
	}
	return all, nil
}
