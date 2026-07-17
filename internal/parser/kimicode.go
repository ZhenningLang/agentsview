package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverKimiCodeSessions finds all wire.jsonl files under the Kimi
// Code sessions directory. The directory structure is:
// <sessionsDir>/wd_<workspace>_<hash>/session_<uuid>/agents/<agent-id>/wire.jsonl
func DiscoverKimiCodeSessions(sessionsDir string) []DiscoveredFile {
	if sessionsDir == "" {
		return nil
	}

	wsDirs, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	var files []DiscoveredFile
	for _, wsEntry := range wsDirs {
		if !isDirOrSymlink(wsEntry, sessionsDir) {
			continue
		}
		wsDir := filepath.Join(sessionsDir, wsEntry.Name())
		sessionDirs, err := os.ReadDir(wsDir)
		if err != nil {
			continue
		}
		for _, sessEntry := range sessionDirs {
			if !isDirOrSymlink(sessEntry, wsDir) {
				continue
			}
			agentsDir := filepath.Join(
				wsDir, sessEntry.Name(), "agents")
			agentDirs, err := os.ReadDir(agentsDir)
			if err != nil {
				continue
			}
			for _, agentEntry := range agentDirs {
				if !isDirOrSymlink(agentEntry, agentsDir) {
					continue
				}
				wirePath := filepath.Join(
					agentsDir, agentEntry.Name(), "wire.jsonl")
				if _, err := os.Stat(wirePath); err != nil {
					continue
				}
				files = append(files, DiscoveredFile{
					Path:    wirePath,
					Project: wsEntry.Name(),
					Agent:   AgentKimiCode,
				})
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

// FindKimiCodeSourceFile locates a Kimi Code session wire file by its
// raw session ID (without the "kimicode:" prefix). The raw ID has the
// format "session_<uuid>" for the main agent, or
// "session_<uuid>:<agent-id>" for a subagent. Because the workspace
// directory name is not part of the ID, workspace dirs are scanned in
// sorted order and the first match wins.
func FindKimiCodeSourceFile(sessionsDir, rawID string) string {
	if sessionsDir == "" {
		return ""
	}

	sessionDirName, agentID, _ := strings.Cut(rawID, ":")
	if agentID == "" {
		agentID = "main"
	}
	if !IsValidSessionID(sessionDirName) ||
		!IsValidSessionID(agentID) {
		return ""
	}

	wsDirs, err := os.ReadDir(sessionsDir)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(wsDirs))
	for _, e := range wsDirs {
		if isDirOrSymlink(e, sessionsDir) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, ws := range names {
		candidate := filepath.Join(
			sessionsDir, ws, sessionDirName,
			"agents", agentID, "wire.jsonl",
		)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
