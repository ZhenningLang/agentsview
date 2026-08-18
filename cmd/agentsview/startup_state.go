package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.kenn.io/kit/daemon"
)

const startupStateFile = "serve.startup.json"

type startupState struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Phase     string    `json:"phase"`
	LogPath   string    `json:"log_path,omitempty"`
}

func startupStatePath(dataDir string) string {
	return filepath.Join(dataDir, startupStateFile)
}

func writeStartupState(dataDir string, state startupState) error {
	if state.PID == 0 {
		state.PID = os.Getpid()
	}
	if state.StartedAt.IsZero() {
		state.StartedAt = time.Now().UTC()
	} else {
		state.StartedAt = state.StartedAt.UTC()
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir for startup state: %w", err)
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal startup state: %w", err)
	}
	tmp, err := os.CreateTemp(dataDir, startupStateFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("create startup state temp file: %w", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write startup state temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod startup state temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close startup state temp file: %w", err)
	}
	if err := os.Rename(tmpPath, startupStatePath(dataDir)); err != nil {
		return fmt.Errorf("rename startup state file: %w", err)
	}
	success = true
	return nil
}

func readStartupState(dataDir string) (startupState, bool) {
	path := startupStatePath(dataDir)
	body, err := os.ReadFile(path)
	if err != nil {
		return startupState{}, false
	}
	var state startupState
	if err := json.Unmarshal(body, &state); err != nil {
		return startupState{}, false
	}
	if state.PID <= 0 || !daemon.ProcessAlive(state.PID) {
		_ = os.Remove(path)
		return startupState{}, false
	}
	return state, true
}

func cleanupStartupState(dataDir string) {
	_ = os.Remove(startupStatePath(dataDir))
}
