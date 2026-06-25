package extract

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const auditBasename = ".extract-audit.jsonl"

type CandidateRecord struct {
	CandidateID string `json:"candidate_id,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	Path        string `json:"path,omitempty"`
}

type RunRecord struct {
	StartedAt      string            `json:"started_at"`
	FinishedAt     string            `json:"finished_at,omitempty"`
	SessionCount   int               `json:"session_count"`
	CandidateCount int               `json:"candidate_count"`
	Written        int               `json:"written"`
	Deduped        int               `json:"deduped"`
	Rejected       int               `json:"rejected"`
	DriftRefused   int               `json:"drift_refused"`
	StagingFiles   int               `json:"staging_files"`
	Skipped        bool              `json:"skipped,omitempty"`
	Note           string            `json:"note,omitempty"`
	Error          string            `json:"error,omitempty"`
	Candidates     []CandidateRecord `json:"candidates,omitempty"`
}

type AuditLog struct {
	path string
	mu   sync.Mutex
}

func AuditPath(memoryDir string) string {
	return filepath.Join(memoryDir, auditBasename)
}

func NewAuditLog(path string) *AuditLog { return &AuditLog{path: path} }

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
	_, err = f.Write(append(data, '\n'))
	return err
}

func (a *AuditLog) Read(limit int) ([]RunRecord, error) {
	if a == nil {
		return []RunRecord{}, nil
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
		var rec RunRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err == nil {
			all = append(all, rec)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
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
