package server

import "go.kenn.io/agentsview/internal/db"

type searchResponse struct {
	Query   string            `json:"query"`
	Results []db.SearchResult `json:"results"`
	Count   int               `json:"count"`
	Next    int               `json:"next"`
}

// prepareFTSQuery wraps global search input as a canonical FTS phrase so
// operator characters are treated as content. Multi-word queries keep the
// existing exact-phrase semantics, and canonical quoted phrases are idempotent.
func prepareFTSQuery(raw string) string {
	return db.PrepareFTSQuery(raw)
}
