package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// statusBasename is the JSON file holding the latest backup-push status. A file
// (not a DB table) is used deliberately so the status needs no three-backend
// schema parity, mirroring the consolidate audit's file-based approach.
const statusBasename = ".backup-status.json"

// Status is the latest backup-push outcome surfaced to the UI. It records the
// last successful push and the last error (if any) so the config/status view
// can show a green/red indicator without the worker exposing the live loop.
type Status struct {
	// Repo is the configured backup target `<owner>/<name>` (informational).
	Repo string `json:"repo,omitempty"`
	// LastAttemptAt is the RFC3339 start time of the most recent cycle that ran
	// (i.e. was not skipped for the lock or being disabled).
	LastAttemptAt string `json:"last_attempt_at,omitempty"`
	// LastSuccessAt is the RFC3339 time of the most recent successful push.
	LastSuccessAt string `json:"last_success_at,omitempty"`
	// LastError is the message from the most recent failed cycle, cleared on a
	// success. A non-empty value drives the UI's red state.
	LastError string `json:"last_error,omitempty"`
	// LastErrorAt is the RFC3339 time of the most recent failure.
	LastErrorAt string `json:"last_error_at,omitempty"`
	// CrossAgentFiles / CCNativeFiles are the file counts from the last cycle
	// that reached the sync step (informational).
	CrossAgentFiles int `json:"cross_agent_files,omitempty"`
	CCNativeFiles   int `json:"cc_native_files,omitempty"`
}

// StatusStore reads/writes the single-record backup status JSON file. It is
// concurrency-safe; a missing file reads as the zero Status (never run yet).
type StatusStore struct {
	path string
	mu   sync.Mutex
}

// NewStatusStore builds a StatusStore at the given JSON path.
func NewStatusStore(path string) *StatusStore {
	return &StatusStore{path: path}
}

// StatusPath returns the canonical status file path inside a data dir.
func StatusPath(dataDir string) string {
	return filepath.Join(dataDir, statusBasename)
}

// Read returns the persisted status. A missing file yields the zero Status (a
// worker that has never run is a valid empty state), not an error.
func (s *StatusStore) Read() (Status, error) {
	if s == nil {
		return Status{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Status{}, nil
		}
		return Status{}, fmt.Errorf("reading backup status: %w", err)
	}
	var st Status
	if err := json.Unmarshal(data, &st); err != nil {
		// A corrupt status file should not wedge the feature: treat as empty.
		return Status{}, nil
	}
	return st, nil
}

// Write persists the status atomically (write-temp-then-rename) so a crash
// mid-write never leaves a truncated file.
func (s *StatusStore) Write(st Status) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating status dir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding backup status: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing backup status: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replacing backup status: %w", err)
	}
	return nil
}
