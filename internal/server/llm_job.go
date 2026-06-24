package server

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/enrich"
	"go.kenn.io/agentsview/internal/llm"
)

// llmPeriodicInterval is how often the background loop checks whether a
// periodic enrichment job should run. Each tick re-reads live config.
const llmPeriodicInterval = 15 * time.Minute

// enrichJobState is the externally visible snapshot of a background
// enrichment job. Counts cover the current (or most recent) job only.
type enrichJobState struct {
	Running   bool   `json:"running"`
	Source    string `json:"source,omitempty"`
	Processed int    `json:"processed"`
	Total     int    `json:"total"`
	Succeeded int    `json:"succeeded"`
	NoContent int    `json:"no_content"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
	StartedAt string `json:"started_at,omitempty"`
	DoneAt    string `json:"done_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

// enrichJob is the single-flight background enrichment job tracker.
type enrichJob struct {
	mu     sync.Mutex
	state  enrichJobState
	cancel context.CancelFunc
}

func newEnrichJob() *enrichJob { return &enrichJob{} }

func (j *enrichJob) snapshot() enrichJobState {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state
}

// startEnrichJob launches a background job that processes every pending
// candidate in a single pass. If a job is already running it is a no-op
// and returns the live state with started=false. Config must already be
// validated by the caller.
func (s *Server) startEnrichJob(source string, cfg config.LLMConfig) (enrichJobState, bool) {
	j := s.enrichJob
	j.mu.Lock()
	if j.state.Running {
		st := j.state
		j.mu.Unlock()
		return st, false
	}
	base := s.baseCtx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	j.cancel = cancel
	j.state = enrichJobState{
		Running:   true,
		Source:    source,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	j.mu.Unlock()

	go s.runEnrichJob(ctx, cancel, cfg, s.llmClient(cfg))
	return j.snapshot(), true
}

// runEnrichJob processes all candidates in one pass. Resumability and the
// failed set are owned by the DB, not by in-memory job state: each success
// writes enrich_status='ok' and drops out of the candidate query, so a job
// restarted after a crash or stop naturally continues from where it left
// off; each failure is persisted via writeFailure as enrich_status='error'
// plus the error text, remains a retryable candidate, and is counted in the
// status report (errors/by_status). Re-running is therefore idempotent and
// safe.
func (s *Server) runEnrichJob(
	ctx context.Context,
	cancel context.CancelFunc,
	cfg config.LLMConfig,
	client *llm.Client,
) {
	j := s.enrichJob
	runner := enrich.New(s.llmWriter, client, cfg)
	stats, err := runner.Run(ctx, enrich.Options{
		Limit: 0,
		OnProgress: func(done, total int) {
			j.mu.Lock()
			j.state.Processed = done
			j.state.Total = total
			j.mu.Unlock()
		},
	})

	j.mu.Lock()
	j.state.Running = false
	j.state.DoneAt = time.Now().UTC().Format(time.RFC3339)
	j.state.Succeeded = stats.Succeeded
	j.state.NoContent = stats.NoContent
	j.state.Failed = stats.Failed
	j.state.Skipped = stats.SkippedTooShort
	if j.state.Total == 0 {
		j.state.Total = stats.Candidates
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		j.state.Error = err.Error()
	} else {
		j.state.Error = ""
	}
	j.cancel = nil
	j.mu.Unlock()
	cancel()
}

// stopEnrichJob cancels the running job, if any, and returns the
// last-known state. The job goroutine flips Running to false when it
// unwinds.
func (s *Server) stopEnrichJob() enrichJobState {
	j := s.enrichJob
	j.mu.Lock()
	cancel := j.cancel
	st := j.state
	j.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return st
}

func (s *Server) runPeriodicEnrichment(ctx context.Context) {
	ticker := time.NewTicker(llmPeriodicInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.maybeStartPeriodicEnrichment()
		}
	}
}

// maybeStartPeriodicEnrichment starts a periodic job when the live
// config enables it and a writable backend with credentials is present.
// It is a no-op while a job already runs.
func (s *Server) maybeStartPeriodicEnrichment() {
	if s.llmWriter == nil {
		return
	}
	s.mu.RLock()
	cfg := s.cfg.ResolveLLM()
	s.mu.RUnlock()
	if !enrichConfigReady(cfg) || !cfg.Periodic {
		return
	}
	s.startEnrichJob("periodic", cfg)
}

// enrichConfigReady reports whether the chat side of LLM enrichment is
// enabled and fully configured.
func enrichConfigReady(cfg config.LLMConfig) bool {
	return cfg.Enabled &&
		strings.TrimSpace(cfg.APIKey) != "" &&
		strings.TrimSpace(cfg.BaseURL) != "" &&
		strings.TrimSpace(cfg.Model) != ""
}
