package search

import (
	"context"
	"errors"
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/llm"
)

const MinMemoryRecallSemanticScore = 0.78

type MemoryRecallRequest struct {
	Query  string
	TopK   int
	Filter db.MemoryFilter
}

type MemoryRecallHit struct {
	RelPath     string  `json:"rel_path"`
	Source      string  `json:"source"`
	Title       string  `json:"title"`
	Date        string  `json:"date"`
	ProblemType string  `json:"problem_type"`
	Status      string  `json:"status"`
	Excerpt     string  `json:"excerpt"`
	Score       float64 `json:"score"`
	Semantic    float64 `json:"semantic"`
	Lexical     float64 `json:"lexical"`
}

type MemoryRecallResponse struct {
	Query    string            `json:"query"`
	Disabled bool              `json:"disabled"`
	Hits     []MemoryRecallHit `json:"hits"`
	Count    int               `json:"count"`
}

func MemoryRecall(
	ctx context.Context,
	store db.Store,
	embedder Embedder,
	cfg config.LLMConfig,
	req MemoryRecallRequest,
) (MemoryRecallResponse, error) {
	query := strings.TrimSpace(req.Query)
	resp := MemoryRecallResponse{Query: query, Hits: []MemoryRecallHit{}}
	if query == "" {
		return resp, nil
	}
	if disabled(cfg) {
		resp.Disabled = true
		return resp, nil
	}
	queryVector, err := embedder.Embed(ctx, query)
	if err != nil {
		if errors.Is(err, llm.ErrNotConfigured) || errors.Is(err, llm.ErrEmbeddingsUnsupported) {
			resp.Disabled = true
			return resp, nil
		}
		return resp, err
	}

	embedded, err := store.MemoryEmbeddings(ctx, req.Filter)
	if err != nil {
		return resp, err
	}
	lexicalFilter := req.Filter
	lexicalFilter.Q = query
	lexical, err := store.ListMemories(ctx, lexicalFilter)
	if err != nil {
		lexical = nil
	}

	hits := mergeMemoryRecall(queryVector, embedded, lexical)
	limit := req.TopK
	if limit <= 0 || limit > db.MaxSearchLimit {
		limit = db.DefaultSearchLimit
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	resp.Hits = hits
	resp.Count = len(hits)
	return resp, nil
}

func mergeMemoryRecall(queryVector []float32, embedded, lexical []db.Memory) []MemoryRecallHit {
	byPath := map[string]*MemoryRecallHit{}
	for _, m := range embedded {
		score, ok := Cosine(queryVector, m.LLMEmbedding)
		if !ok || score < MinMemoryRecallSemanticScore {
			continue
		}
		hit := memoryHit(m)
		hit.Semantic = score
		hit.Score = score * 0.85
		byPath[m.RelPath] = &hit
	}
	for idx, m := range lexical {
		lex := 1.0 / float64(idx+1)
		hit, ok := byPath[m.RelPath]
		if !ok {
			h := memoryHit(m)
			h.Lexical = lex
			h.Score = lex * 0.15
			byPath[m.RelPath] = &h
			continue
		}
		hit.Lexical = lex
		hit.Score += lex * 0.15
	}
	hits := make([]MemoryRecallHit, 0, len(byPath))
	for _, hit := range byPath {
		hits = append(hits, *hit)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].RelPath < hits[j].RelPath
		}
		return hits[i].Score > hits[j].Score
	})
	return hits
}

func memoryHit(m db.Memory) MemoryRecallHit {
	return MemoryRecallHit{
		RelPath:     m.RelPath,
		Source:      m.Source,
		Title:       fallbackTitle(m),
		Date:        m.Date,
		ProblemType: m.ProblemType,
		Status:      m.Status,
		Excerpt:     excerpt(m.Body),
	}
}

func fallbackTitle(m db.Memory) string {
	if strings.TrimSpace(m.Title) != "" {
		return strings.TrimSpace(m.Title)
	}
	return m.RelPath
}

func excerpt(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if len(body) <= 220 {
		return body
	}
	return strings.TrimSpace(body[:220])
}
