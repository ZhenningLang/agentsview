package search

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/llm"
)

const minSemanticScore = 0.000001

type Embedder interface {
	Embed(ctx context.Context, input string) ([]float32, error)
}

type Request struct {
	Query   string
	Project string
	Limit   int
}

type SemanticResponse struct {
	Query    string            `json:"query"`
	Disabled bool              `json:"disabled"`
	Results  []db.SearchResult `json:"results"`
	Count    int               `json:"count"`
}

type StatusResponse struct {
	Available bool `json:"available"`
}

func Available(cfg config.LLMConfig) bool {
	return !disabled(cfg)
}

func Semantic(ctx context.Context, store db.Store, embedder Embedder, cfg config.LLMConfig, req Request) (SemanticResponse, error) {
	query := strings.TrimSpace(req.Query)
	resp := SemanticResponse{Query: query, Results: []db.SearchResult{}}
	isDisabled := disabled(cfg)
	if query == "" || isDisabled {
		resp.Disabled = isDisabled
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
	sessions, err := store.SessionEmbeddings(ctx, db.EmbeddingFilter{Project: req.Project})
	if err != nil {
		return resp, err
	}
	results := Rank(queryVector, sessions, req.Limit)
	resp.Results = results
	resp.Count = len(results)
	return resp, nil
}

func disabled(cfg config.LLMConfig) bool {
	return !cfg.Enabled || strings.TrimSpace(cfg.Embed.Model) == "" ||
		strings.TrimSpace(cfg.Embed.BaseURL) == ""
}

func Rank(query []float32, sessions []db.SessionEmbedding, limit int) []db.SearchResult {
	if limit <= 0 || limit > db.MaxSearchLimit {
		limit = db.DefaultSearchLimit
	}
	results := make([]db.SearchResult, 0, len(sessions))
	for _, session := range sessions {
		score, ok := Cosine(query, session.Vector)
		if !ok || score < minSemanticScore {
			continue
		}
		results = append(results, db.SearchResult{
			SessionID:      session.SessionID,
			Project:        session.Project,
			Agent:          session.Agent,
			Name:           session.Name,
			Ordinal:        -1,
			SessionEndedAt: session.SessionEndedAt,
			Snippet:        "Semantic match",
			Rank:           score,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Rank == results[j].Rank {
			return results[i].SessionID < results[j].SessionID
		}
		return results[i].Rank > results[j].Rank
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func Cosine(a, b []float32) (float64, bool) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, false
	}
	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0, false
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), true
}
