package server

import (
	"context"
	"errors"
	"strconv"
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

	// Token usage reported by the provider for this run.
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	EmbedTokens      int `json:"embed_tokens"`

	// Chat-provider balance delta, populated only when the provider
	// exposes a balance endpoint (e.g. DeepSeek). CostSpent is the
	// formatted BalanceStart-BalanceEnd amount in CostCurrency.
	CostCurrency string `json:"cost_currency,omitempty"`
	CostSpent    string `json:"cost_spent,omitempty"`
	BalanceStart string `json:"balance_start,omitempty"`
	BalanceEnd   string `json:"balance_end,omitempty"`

	// Embedding-provider balance delta, tracked independently from chat
	// because the embedding provider is configured separately (its own
	// base URL, key, and balance endpoint). Populated only when the embed
	// provider exposes a balance endpoint.
	EmbedCostCurrency string `json:"embed_cost_currency,omitempty"`
	EmbedCostSpent    string `json:"embed_cost_spent,omitempty"`
	EmbedBalanceStart string `json:"embed_balance_start,omitempty"`
	EmbedBalanceEnd   string `json:"embed_balance_end,omitempty"`
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

	// Snapshot the chat-provider balance before the run so the cost of
	// this run can be reported as a balance delta (best-effort; only
	// providers with a balance endpoint, e.g. DeepSeek, populate it).
	if startBal := s.fetchLLMBalance(ctx, cfg); startBal.Supported {
		j.mu.Lock()
		j.state.CostCurrency = startBal.Currency
		j.state.BalanceStart = startBal.Amount
		j.mu.Unlock()
	}

	// Snapshot the embedding-provider balance independently. The embed
	// provider is configured separately, so its spend is its own balance
	// delta rather than part of the chat delta. Only attempted when an
	// embed balance endpoint is explicitly configured (no fallback to the
	// chat account, to avoid double-counting a shared balance).
	if embedCfg, ok := embedBalanceConfig(cfg); ok {
		if startEmbed := s.fetchLLMBalance(ctx, embedCfg); startEmbed.Supported {
			j.mu.Lock()
			j.state.EmbedCostCurrency = startEmbed.Currency
			j.state.EmbedBalanceStart = startEmbed.Amount
			j.mu.Unlock()
		}
	}

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

	// Fetch the ending balance on an independent timeout so a stopped
	// or cancelled run still reports its cost.
	balCtx, balCancel := context.WithTimeout(context.Background(), 10*time.Second)
	endBal := s.fetchLLMBalance(balCtx, cfg)
	var endEmbed llmBalanceResponse
	if embedCfg, ok := embedBalanceConfig(cfg); ok {
		endEmbed = s.fetchLLMBalance(balCtx, embedCfg)
	}
	balCancel()

	j.mu.Lock()
	j.state.Running = false
	j.state.DoneAt = time.Now().UTC().Format(time.RFC3339)
	j.state.Succeeded = stats.Succeeded
	j.state.NoContent = stats.NoContent
	j.state.Failed = stats.Failed
	j.state.Skipped = stats.SkippedTooShort
	j.state.PromptTokens = stats.PromptTokens
	j.state.CompletionTokens = stats.CompletionTokens
	j.state.EmbedTokens = stats.EmbedTokens
	if j.state.Total == 0 {
		j.state.Total = stats.Candidates
	}
	if endBal.Supported {
		j.state.BalanceEnd = endBal.Amount
		if j.state.CostCurrency == "" {
			j.state.CostCurrency = endBal.Currency
		}
		if spent, ok := balanceSpent(j.state.BalanceStart, endBal.Amount); ok {
			j.state.CostSpent = spent
		}
	}
	if endEmbed.Supported {
		j.state.EmbedBalanceEnd = endEmbed.Amount
		if j.state.EmbedCostCurrency == "" {
			j.state.EmbedCostCurrency = endEmbed.Currency
		}
		if spent, ok := balanceSpent(j.state.EmbedBalanceStart, endEmbed.Amount); ok {
			j.state.EmbedCostSpent = spent
		}
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

// embedBalanceConfig builds a balance-only LLMConfig view of the embed
// provider so the shared fetchLLMBalance path can query its balance, the
// same way chat balance is resolved (explicit balance_url, else derived
// from a known base URL host like DeepSeek).
//
// It reports ok=false when the embed provider has no resolvable balance
// endpoint, or when that endpoint plus API key is identical to the chat
// account's — in the shared-account case the chat balance delta already
// covers embed spend, so tracking it again would double-count.
func embedBalanceConfig(cfg config.LLMConfig) (config.LLMConfig, bool) {
	embed := config.LLMConfig{
		Enabled:    cfg.Enabled,
		BaseURL:    cfg.Embed.BaseURL,
		APIKey:     cfg.Embed.APIKey,
		BalanceURL: cfg.Embed.BalanceURL,
	}
	embedEP, ok := llmBalanceEndpoint(embed)
	if !ok {
		return config.LLMConfig{}, false
	}
	if chatEP, chatOK := llmBalanceEndpoint(cfg); chatOK &&
		embedEP == chatEP &&
		strings.TrimSpace(embed.APIKey) == strings.TrimSpace(cfg.APIKey) {
		return config.LLMConfig{}, false
	}
	return embed, true
}

// balanceSpent returns the formatted start-minus-end amount when both
// parse as numbers. A negative delta (e.g. the account was topped up
// mid-run) is clamped to zero rather than reported as negative spend.
func balanceSpent(start, end string) (string, bool) {
	s, errS := strconv.ParseFloat(strings.TrimSpace(start), 64)
	e, errE := strconv.ParseFloat(strings.TrimSpace(end), 64)
	if errS != nil || errE != nil {
		return "", false
	}
	delta := s - e
	if delta < 0 {
		delta = 0
	}
	return strconv.FormatFloat(delta, 'f', 4, 64), true
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
