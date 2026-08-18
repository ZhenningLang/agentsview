package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.kenn.io/kit/daemon"
)

const startupStateFile = "serve.startup.json"

type startupState struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Phase     string    `json:"phase"`
	LogPath   string    `json:"log_path,omitempty"`
	// CreateTimeMillis identifies the writing process beyond its PID.
	// Runtime records already carry it under create_time_ms; startup
	// records carry the same value so both use one notion of identity.
	CreateTimeMillis int64 `json:"create_time_ms,omitempty"`
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
	if state.CreateTimeMillis == 0 {
		if created, ok := processCreateTimeMillis(state.PID); ok {
			state.CreateTimeMillis = created
		}
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
	if state.PID <= 0 || !daemon.ProcessAlive(state.PID) ||
		!startupStateOwnerAlive(state) {
		_ = os.Remove(path)
		return startupState{}, false
	}
	return state, true
}

// startupStateOwnerAlive reports whether the process that wrote the
// record is still the process behind its PID.
//
// A live PID on its own is not ownership. When a starting server dies
// before it publishes a runtime record, the kernel is free to hand its
// PID to an unrelated long-lived program; the record then stays
// "valid" indefinitely, `serve status` keeps reporting a startup that
// is over, and `serve stop` refuses to act because it believes a server
// is coming up. Comparing the process creation time is the same
// identity check runtime records already use before signalling a PID.
func startupStateOwnerAlive(state startupState) bool {
	if state.CreateTimeMillis <= 0 {
		// Written before this field existed, or on a platform where the
		// creation time is unreadable. Nothing to compare against, so
		// fall back to PID liveness rather than discarding a record we
		// cannot judge.
		return true
	}
	return processCreateTimeMatches(
		state.PID, strconv.FormatInt(state.CreateTimeMillis, 10),
	)
}

func cleanupStartupState(dataDir string) {
	_ = os.Remove(startupStatePath(dataDir))
}
