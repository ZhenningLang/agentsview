package search

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/llm"
)

const MinMemoryRecallSemanticScore = 0.78

// ftsTermRE matches significant ASCII terms (≥3 chars) for the lexical recall
// query. It uses alphanumeric runs only — matching FTS5's default unicode61
// tokenizer, which splits on '-' '_' ':' etc — so "lzn-preview" yields "lzn"
// and "preview" (the tokens actually stored), while camelCase identifiers like
// "commitPoolItemsEqual" stay whole. ftsMaxTerms caps how many are OR'd.
var ftsTermRE = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]{2,}`)

const ftsMaxTerms = 12

type MemoryRecallRequest struct {
	Query           string
	TopK            int
	Filter          db.MemoryFilter
	PreferCanonical bool
}

type MemoryRecallHit struct {
	RelPath              string  `json:"rel_path"`
	Source               string  `json:"source"`
	Title                string  `json:"title"`
	Date                 string  `json:"date"`
	ProblemType          string  `json:"problem_type"`
	Status               string  `json:"status"`
	CanonicalCoveredRefs string  `json:"canonical_covered_refs"`
	CanonicalProvenance  string  `json:"canonical_provenance"`
	Excerpt              string  `json:"excerpt"`
	Score                float64 `json:"score"`
	Semantic             float64 `json:"semantic"`
	Lexical              float64 `json:"lexical"`
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
	explicitSource := strings.TrimSpace(req.Filter.Source) != ""
	if !req.PreferCanonical && !explicitSource {
		embedded = withoutCanonicalMemories(embedded)
	}
	lexicalFilter := req.Filter
	lexicalFilter.Q = ftsQueryFromText(query)
	if req.PreferCanonical && !explicitSource {
		lexicalFilter.Q = query
	}
	var lexical []db.Memory
	if lexicalFilter.Q != "" {
		lexical, err = store.ListMemories(ctx, lexicalFilter)
		if err != nil {
			if req.PreferCanonical && !explicitSource {
				return resp, err
			}
			lexical = nil
		}
	}
	if !req.PreferCanonical && !explicitSource {
		lexical = withoutCanonicalMemories(lexical)
	}

	hits := mergeMemoryRecall(queryVector, embedded, lexical)
	limit := req.TopK
	if limit <= 0 || limit > db.MaxSearchLimit {
		limit = db.DefaultSearchLimit
	}
	if req.PreferCanonical && !explicitSource {
		hits = suppressCoveredRawWithinLimit(hits, limit)
	} else if len(hits) > limit {
		hits = hits[:limit]
	}
	resp.Hits = hits
	resp.Count = len(hits)
	return resp, nil
}

func withoutCanonicalMemories(memories []db.Memory) []db.Memory {
	out := memories[:0]
	for _, m := range memories {
		if m.Source == db.SourceCanonical {
			continue
		}
		out = append(out, m)
	}
	return out
}

type canonicalCoveredRef struct {
	Source  string `json:"source"`
	RelPath string `json:"rel_path"`
}

func suppressCoveredRawWithinLimit(hits []MemoryRecallHit, limit int) []MemoryRecallHit {
	selected := make([]MemoryRecallHit, 0, min(limit, len(hits)))
	covered := map[string]struct{}{}
	for _, hit := range hits {
		if hit.Source != db.SourceCanonical {
			if _, ok := covered[memoryRefKey(hit.Source, hit.RelPath)]; ok {
				continue
			}
			if len(selected) < limit {
				selected = append(selected, hit)
			}
			continue
		}

		refs := coveredRefs(hit)
		if len(selected) < limit {
			selected = append(selected, hit)
		} else {
			continue
		}
		if len(refs) == 0 {
			continue
		}
		for _, ref := range refs {
			covered[memoryRefKey(ref.Source, ref.RelPath)] = struct{}{}
		}
		selected = withoutCoveredRawHits(selected, covered)
	}
	return selected
}

func withoutCoveredRawHits(hits []MemoryRecallHit, covered map[string]struct{}) []MemoryRecallHit {
	out := hits[:0]
	for _, hit := range hits {
		if hit.Source != db.SourceCanonical {
			if _, ok := covered[memoryRefKey(hit.Source, hit.RelPath)]; ok {
				continue
			}
		}
		out = append(out, hit)
	}
	return out
}

func suppressCoveredRaw(hits []MemoryRecallHit) []MemoryRecallHit {
	covered := map[string]struct{}{}
	for _, hit := range hits {
		for _, ref := range coveredRefs(hit) {
			covered[memoryRefKey(ref.Source, ref.RelPath)] = struct{}{}
		}
	}
	if len(covered) == 0 {
		return hits
	}
	out := hits[:0]
	for _, hit := range hits {
		if hit.Source != db.SourceCanonical {
			if _, ok := covered[memoryRefKey(hit.Source, hit.RelPath)]; ok {
				continue
			}
		}
		out = append(out, hit)
	}
	return out
}

func coveredRefs(hit MemoryRecallHit) []canonicalCoveredRef {
	if hit.Source != db.SourceCanonical || strings.TrimSpace(hit.CanonicalCoveredRefs) == "" {
		return nil
	}
	var refs []canonicalCoveredRef
	if err := json.Unmarshal([]byte(hit.CanonicalCoveredRefs), &refs); err != nil {
		return nil
	}
	out := refs[:0]
	for _, ref := range refs {
		if ref.Source == "" || ref.RelPath == "" {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func memoryRefKey(source, relPath string) string {
	return source + "\x00" + relPath
}

// ftsQueryFromText turns an arbitrary candidate blob into a safe FTS5 MATCH
// expression. Passing the raw blob is doubly broken: punctuation like ':' '(' ')'
// is FTS5 syntax (a stray colon throws a query error, so lexical recall silently
// returns nothing), and bare space-separated tokens are implicitly ANDed (so a
// long blob matches almost nothing). Instead we extract significant ASCII tokens
// (code identifiers, file names, error words — which are shared across natural
// languages) and OR them as quoted literals. CJK prose is dropped; the shared
// identifiers carry the cross-language duplicate signal. Returns "" when no
// usable term exists, so the caller skips the lexical leg entirely.
func ftsQueryFromText(text string) string {
	matches := ftsTermRE.FindAllString(text, -1)
	seen := map[string]struct{}{}
	terms := make([]string, 0, ftsMaxTerms)
	for _, m := range matches {
		key := strings.ToLower(m)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		terms = append(terms, `"`+m+`"`)
		if len(terms) >= ftsMaxTerms {
			break
		}
	}
	return strings.Join(terms, " OR ")
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
		byPath[memoryRefKey(m.Source, m.RelPath)] = &hit
	}
	for idx, m := range lexical {
		lex := 1.0 / float64(idx+1)
		hit, ok := byPath[memoryRefKey(m.Source, m.RelPath)]
		if !ok {
			h := memoryHit(m)
			h.Lexical = lex
			h.Score = lex * 0.15
			byPath[memoryRefKey(m.Source, m.RelPath)] = &h
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
		RelPath:              m.RelPath,
		Source:               m.Source,
		Title:                fallbackTitle(m),
		Date:                 m.Date,
		ProblemType:          m.ProblemType,
		Status:               m.Status,
		CanonicalCoveredRefs: m.CanonicalCoveredRefs,
		CanonicalProvenance:  m.CanonicalProvenance,
		Excerpt:              excerpt(m.Body),
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
