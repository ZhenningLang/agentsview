package sync

import gosync "sync"

// Phase describes the current sync phase.
type Phase string

const (
	PhaseIdle        Phase = "idle"
	PhaseDiscovering Phase = "discovering"
	PhaseSyncing     Phase = "syncing"
	PhaseDone        Phase = "done"
)

// Progress reports sync progress to listeners.
type Progress struct {
	Phase           Phase  `json:"phase"`
	CurrentProject  string `json:"current_project,omitempty"`
	ProjectsTotal   int    `json:"projects_total"`
	ProjectsDone    int    `json:"projects_done"`
	SessionsTotal   int    `json:"sessions_total"`
	SessionsDone    int    `json:"sessions_done"`
	MessagesIndexed int    `json:"messages_indexed"`
}

// SyncResult describes the outcome of syncing a single session.
type SyncResult struct {
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Skipped   bool   `json:"skipped"`
	Messages  int    `json:"messages"`
}

// SyncStats summarizes a full sync run.
//
// TotalSessions counts discovered files plus DB-backed sessions.
// Synced counts sessions (one file can produce multiple via fork
// detection; DB-backed agents add sessions directly). Failed counts
// files with hard parse/stat errors. filesOK counts files that
// produced at least one session — used by ResyncAll to compare
// against Failed on the same unit.
type SyncStats struct {
	TotalSessions  int          `json:"total_sessions"`
	Synced         int          `json:"synced"`
	Skipped        int          `json:"skipped"`
	Failed         int          `json:"failed"`
	OrphanedCopied int          `json:"orphaned_copied,omitempty"`
	Warnings       []string     `json:"warnings,omitempty"`
	Aborted        bool         `json:"aborted,omitempty"`
	Anomalies      AnomalyStats `json:"anomalies,omitzero"`

	filesOK             int // unexported: file-level success counter
	filesDiscovered     int // file-based total, excludes DB-backed agents
	messagesIndexed     int // unexported: progress message counter
	parserExcludedFiles int // file-level intentional parser exclusions
	parserExcludedIDs   []string
}

type AnomalyStats struct {
	MalformedLinesByAgent map[string]int `json:"malformed_lines_by_agent,omitempty"`
	MalformedLinesTotal   int            `json:"malformed_lines_total,omitempty"`
	Sanitize              SanitizeStats  `json:"sanitize,omitzero"`
}

type SanitizeStats struct {
	ControlCharsStripped int `json:"control_chars_stripped,omitempty"`
	ModelClamped         int `json:"model_clamped,omitempty"`
	TokensClamped        int `json:"tokens_clamped,omitempty"`
	RoleCoerced          int `json:"role_coerced,omitempty"`
	TimestampsBlanked    int `json:"timestamps_blanked,omitempty"`
}

func (s SanitizeStats) Total() int {
	return s.ControlCharsStripped + s.ModelClamped + s.TokensClamped +
		s.RoleCoerced + s.TimestampsBlanked
}

func (s SanitizeStats) IsZero() bool {
	return s.Total() == 0
}

func (a AnomalyStats) IsZero() bool {
	return a.MalformedLinesTotal == 0 && a.Sanitize.IsZero()
}

func (a *AnomalyStats) RecordMalformedLines(agent string, n int) {
	if n <= 0 {
		return
	}
	if a.MalformedLinesByAgent == nil {
		a.MalformedLinesByAgent = make(map[string]int)
	}
	a.MalformedLinesByAgent[agent] += n
	a.MalformedLinesTotal += n
}

func (a *AnomalyStats) addSanitize(s SanitizeStats) {
	a.Sanitize.ControlCharsStripped += s.ControlCharsStripped
	a.Sanitize.ModelClamped += s.ModelClamped
	a.Sanitize.TokensClamped += s.TokensClamped
	a.Sanitize.RoleCoerced += s.RoleCoerced
	a.Sanitize.TimestampsBlanked += s.TimestampsBlanked
}

func (a *AnomalyStats) merge(o AnomalyStats) {
	for agent, n := range o.MalformedLinesByAgent {
		a.RecordMalformedLines(agent, n)
	}
	a.addSanitize(o.Sanitize)
}

type anomalyAccumulator struct {
	mu             gosync.Mutex
	stats          AnomalyStats
	malformedFiles map[string]bool
}

func (a *anomalyAccumulator) reset() {
	a.mu.Lock()
	a.stats = AnomalyStats{}
	a.malformedFiles = nil
	a.mu.Unlock()
}

func (a *anomalyAccumulator) recordMalformedLines(agent, sourcePath string, n int) {
	if n <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if sourcePath != "" {
		if a.malformedFiles == nil {
			a.malformedFiles = make(map[string]bool)
		}
		if a.malformedFiles[sourcePath] {
			return
		}
		a.malformedFiles[sourcePath] = true
	}
	a.stats.RecordMalformedLines(agent, n)
}

func (a *anomalyAccumulator) recordSanitize(s SanitizeStats) {
	if s.IsZero() {
		return
	}
	a.mu.Lock()
	a.stats.addSanitize(s)
	a.mu.Unlock()
}

func (a *anomalyAccumulator) applyTo(s *SyncStats) {
	a.mu.Lock()
	s.Anomalies.merge(a.stats)
	a.mu.Unlock()
}

// RecordSkip increments the skipped session counter.
func (s *SyncStats) RecordSkip() {
	s.Skipped++
}

// RecordSynced adds n to the synced session counter.
func (s *SyncStats) RecordSynced(n int) {
	s.Synced += n
}

// RecordFailed increments the hard-failure counter.
func (s *SyncStats) RecordFailed() {
	s.Failed++
}

// Percent returns the sync progress as a percentage (0–100).
func (p Progress) Percent() float64 {
	if p.SessionsTotal == 0 {
		return 0
	}
	return float64(p.SessionsDone) /
		float64(p.SessionsTotal) * 100
}

// ProgressFunc is called with progress updates during sync.
type ProgressFunc func(Progress)
