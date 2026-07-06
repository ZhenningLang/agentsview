package synthesize

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const auditBasename = ".synthesize-audit.jsonl"

// AuditPath returns the canonical audit jsonl path for a memory dir.
func AuditPath(memoryDir string) string {
	return filepath.Join(memoryDir, auditBasename)
}

// TopicRecord is one cluster's synthesis outcome within a cycle.
type TopicRecord struct {
	Title       string   `json:"title"`
	SourceIDs   []string `json:"source_ids"`
	CoveredRefs []RawRef `json:"covered_refs,omitempty"`
	RelPath     string   `json:"rel_path,omitempty"`
	Result      string   `json:"result"` // compact_memory result line (write/skip/...)
	Skipped     bool     `json:"skipped,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// RunRecord is one synthesis cycle's audit entry.
type RunRecord struct {
	StartedAt             string         `json:"started_at"`
	FinishedAt            string         `json:"finished_at,omitempty"`
	NoteCount             int            `json:"note_count"`
	SourceCounts          map[string]int `json:"source_counts,omitempty"`
	EligibleSourceCounts  map[string]int `json:"eligible_source_counts,omitempty"`
	ClusterCount          int            `json:"cluster_count"`
	PlannedCanonicalCount int            `json:"planned_canonical_count,omitempty"`
	CanonicalWriteCount   int            `json:"canonical_write_count,omitempty"`
	SkippedCount          int            `json:"skipped_count,omitempty"`
	FailedCount           int            `json:"failed_count,omitempty"`
	ConflictCount         int            `json:"conflict_count,omitempty"`
	WriteCount            int            `json:"write_count"`
	ConflictSamples       []string       `json:"conflict_samples,omitempty"`
	CoverageRefs          []RawRef       `json:"coverage_refs,omitempty"`
	DryRun                bool           `json:"dry_run,omitempty"`
	Topics                []TopicRecord  `json:"topics,omitempty"`
	Committed             bool           `json:"committed"`
	Resynced              bool           `json:"resynced"`
	Skipped               bool           `json:"skipped,omitempty"`
	Note                  string         `json:"note,omitempty"`
	Error                 string         `json:"error,omitempty"`
}

// AuditLog is an append-only jsonl writer/reader for synthesis cycles.
type AuditLog struct {
	path string
	mu   sync.Mutex
}

func NewAuditLog(path string) *AuditLog { return &AuditLog{path: path} }

func (a *AuditLog) Append(rec RunRecord) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// Read returns up to limit records, newest-first (0 = all).
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
		return nil, err
	}
	defer f.Close()
	var recs []RunRecord
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
		recs = append(recs, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// newest-first
	for i, j := 0, len(recs)-1; i < j; i, j = i+1, j-1 {
		recs[i], recs[j] = recs[j], recs[i]
	}
	if limit > 0 && len(recs) > limit {
		recs = recs[:limit]
	}
	return recs, nil
}
