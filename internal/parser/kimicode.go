package parser

// DiscoverKimiCodeSessions finds all wire.jsonl files under the Kimi
// Code sessions directory. The directory structure is:
// <sessionsDir>/wd_<workspace>_<hash>/session_<uuid>/agents/<agent-id>/wire.jsonl
func DiscoverKimiCodeSessions(sessionsDir string) []DiscoveredFile {
	return nil
}

// FindKimiCodeSourceFile locates a Kimi Code session wire file by its
// raw session ID (without the "kimicode:" prefix). The raw ID has the
// format "session_<uuid>" for the main agent, or
// "session_<uuid>:<agent-id>" for a subagent.
func FindKimiCodeSourceFile(sessionsDir, rawID string) string {
	return ""
}
