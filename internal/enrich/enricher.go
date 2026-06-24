package enrich

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/llm"
)

type chatClient interface {
	ChatJSON(ctx context.Context, system, user string) (string, error)
}

type embedClient interface {
	Embed(ctx context.Context, input string) ([]float32, error)
}

// chatUsageClient and embedUsageClient are optional interfaces a client
// may implement to also report token usage. The enricher type-asserts
// them and falls back to the plain calls (with zero usage) otherwise.
type chatUsageClient interface {
	ChatJSONUsage(ctx context.Context, system, user string) (string, llm.Usage, error)
}

type embedUsageClient interface {
	EmbedUsage(ctx context.Context, input string) ([]float32, llm.Usage, error)
}

// tokenUsage accumulates token counts across a candidate's chat and
// embedding calls.
type tokenUsage struct {
	prompt     int
	completion int
	embed      int
}

type Options struct {
	Project string
	Force   bool
	Limit   int
	Now     time.Time
	// OnProgress, when set, is invoked once after the candidate set is
	// queried with (0, total), and again after each candidate finishes
	// with the cumulative done count and the total. It is called from
	// worker goroutines while holding the stats mutex, so it must not
	// block or re-enter the enricher.
	OnProgress func(done, total int)
}

type Stats struct {
	Disabled   bool
	Candidates int
	// SkippedTooShort is the number of sessions newly marked too short
	// in this run; historical skipped sessions are not recounted.
	SkippedTooShort int
	NoContent       int
	Succeeded       int
	Failed          int
	// Token usage summed across all processed candidates in this run.
	PromptTokens     int
	CompletionTokens int
	EmbedTokens      int
}

type Enricher struct {
	db     *db.DB
	client chatClient
	cfg    config.LLMConfig
}

func New(database *db.DB, client chatClient, cfg config.LLMConfig) *Enricher {
	return &Enricher{db: database, client: client, cfg: cfg}
}

func (e *Enricher) Run(ctx context.Context, opts Options) (Stats, error) {
	if !e.cfg.Enabled {
		return Stats{Disabled: true}, nil
	}
	if e.db == nil {
		return Stats{}, fmt.Errorf("enricher: nil database")
	}
	if e.client == nil {
		return Stats{}, fmt.Errorf("enricher: nil client")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	candidateOpts := db.EnrichCandidateOptions{
		Project:             opts.Project,
		Force:               opts.Force,
		Limit:               opts.Limit,
		MinUserMessages:     e.cfg.MinUserMessages,
		ReenrichMsgDelta:    e.cfg.ReenrichMsgDelta,
		ReenrichIdleMinutes: e.cfg.ReenrichIdleMinutes,
		Now:                 opts.Now,
	}
	stats := Stats{}
	skipped, err := e.db.MarkEnrichmentSkippedTooShort(ctx, candidateOpts)
	if err != nil {
		return stats, err
	}
	stats.SkippedTooShort = skipped
	candidates, err := e.db.EnrichCandidates(ctx, candidateOpts)
	if err != nil {
		return stats, err
	}
	stats.Candidates = len(candidates)
	if opts.OnProgress != nil {
		opts.OnProgress(0, len(candidates))
	}
	if len(candidates) == 0 {
		return stats, nil
	}
	workers := e.cfg.Concurrency
	if workers <= 0 {
		workers = 3
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}
	jobs := make(chan db.EnrichCandidate)
	var mu sync.Mutex
	var wg sync.WaitGroup
	total := len(candidates)
	done := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				result, usage := e.processCandidate(ctx, candidate, opts.Now)
				mu.Lock()
				switch result {
				case db.EnrichStatusOK:
					stats.Succeeded++
				case db.EnrichStatusNoContent:
					stats.NoContent++
				case db.EnrichStatusError:
					stats.Failed++
				}
				stats.PromptTokens += usage.prompt
				stats.CompletionTokens += usage.completion
				stats.EmbedTokens += usage.embed
				done++
				if opts.OnProgress != nil {
					opts.OnProgress(done, total)
				}
				mu.Unlock()
			}
		}()
	}
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return stats, ctx.Err()
		case jobs <- candidate:
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	return stats, nil
}

func (e *Enricher) processCandidate(ctx context.Context, candidate db.EnrichCandidate, now time.Time) (string, tokenUsage) {
	var usage tokenUsage
	messages, err := e.db.GetAllMessages(ctx, candidate.ID)
	if err != nil {
		e.writeFailure(ctx, candidate.ID, err)
		return db.EnrichStatusError, usage
	}
	samples := sampleMessages(messages)
	if len(samples) == 0 {
		_ = e.db.WriteEnrichment(ctx, candidate.ID, db.EnrichmentWrite{
			Status: db.EnrichStatusNoContent,
			Error:  "no sampleable message content",
		})
		return db.EnrichStatusNoContent, usage
	}
	system, user := buildPrompt(candidate, samples)
	content, chatUsage, err := e.chatJSON(ctx, system, user)
	usage.prompt += chatUsage.PromptTokens
	usage.completion += chatUsage.CompletionTokens
	if err != nil {
		e.writeFailure(ctx, candidate.ID, err)
		return db.EnrichStatusError, usage
	}
	parsed, err := llm.ParseEnrichment(content)
	if err != nil {
		e.writeFailure(ctx, candidate.ID, err)
		return db.EnrichStatusError, usage
	}
	embedding, hasEmbedding, embedUsage := e.embeddingForSamples(ctx, samples)
	if embedUsage.TotalTokens > 0 {
		usage.embed += embedUsage.TotalTokens
	} else {
		usage.embed += embedUsage.PromptTokens
	}
	if err := e.db.WriteEnrichment(ctx, candidate.ID, db.EnrichmentWrite{
		Title:        parsed.Title,
		Summary:      parsed.Summary,
		Keywords:     parsed.Keywords,
		Model:        e.cfg.Model,
		Status:       db.EnrichStatusOK,
		MessageCnt:   candidate.MessageCount,
		EnrichedAt:   now,
		Embedding:    embedding,
		HasEmbedding: hasEmbedding,
	}); err != nil {
		e.writeFailure(ctx, candidate.ID, err)
		return db.EnrichStatusError, usage
	}
	return db.EnrichStatusOK, usage
}

// chatJSON calls the chat client, preferring the usage-reporting
// variant when the client implements it.
func (e *Enricher) chatJSON(ctx context.Context, system, user string) (string, llm.Usage, error) {
	if uc, ok := e.client.(chatUsageClient); ok {
		return uc.ChatJSONUsage(ctx, system, user)
	}
	content, err := e.client.ChatJSON(ctx, system, user)
	return content, llm.Usage{}, err
}

func (e *Enricher) embeddingForSamples(ctx context.Context, samples []string) ([]float32, bool, llm.Usage) {
	if strings.TrimSpace(e.cfg.Embed.Model) == "" {
		return nil, false, llm.Usage{}
	}
	vector, usage, err := e.embed(ctx, strings.Join(samples, "\n\n"))
	if errors.Is(err, errNoEmbedClient) {
		return nil, false, llm.Usage{}
	}
	if err != nil {
		log.Printf("LLM enrichment embedding skipped: %T", err)
		return nil, false, usage
	}
	if len(vector) == 0 {
		return nil, false, usage
	}
	if _, err := db.EncodeEmbedding(vector); err != nil {
		log.Printf("LLM enrichment embedding skipped: invalid vector")
		return nil, false, usage
	}
	return vector, true, usage
}

var errNoEmbedClient = errors.New("client does not support embeddings")

// embed calls the embed client, preferring the usage-reporting variant
// when the client implements it.
func (e *Enricher) embed(ctx context.Context, input string) ([]float32, llm.Usage, error) {
	if uc, ok := e.client.(embedUsageClient); ok {
		return uc.EmbedUsage(ctx, input)
	}
	if ec, ok := e.client.(embedClient); ok {
		vector, err := ec.Embed(ctx, input)
		return vector, llm.Usage{}, err
	}
	return nil, llm.Usage{}, errNoEmbedClient
}

func (e *Enricher) writeFailure(ctx context.Context, sessionID string, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	_ = e.db.WriteEnrichment(ctx, sessionID, db.EnrichmentWrite{
		Status: db.EnrichStatusError,
		Error:  sanitizeError(err, e.cfg.APIKey),
	})
}

func sanitizeError(err error, secrets ...string) string {
	msg := strings.TrimSpace(err.Error())
	for _, secret := range secrets {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[redacted]")
		}
	}
	if len([]rune(msg)) > 300 {
		msg = truncateRunes(msg, 300)
	}
	return msg
}
