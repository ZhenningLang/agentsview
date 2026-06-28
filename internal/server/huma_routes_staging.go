package server

import (
	"context"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/consolidate"
)

// registerStagingRoutes exposes the read-only staging candidate pool — the raw
// memory candidates awaiting consolidation. Unlike the consolidate audit (which
// shows decided cycles), this shows what is currently queued, split by origin
// scope (user vs project) so the user can browse "整体 vs 项目".
func (s *Server) registerStagingRoutes() {
	group := newRouteGroup(s.api, "/api/v1/staging", "Staging")
	get(s, group, "/candidates", "List staging memory candidates (the 备选池)", s.humaStagingCandidates)
}

type stagingCandidatesInput struct {
	Scope string `query:"scope" doc:"Filter by origin scope: user | project (empty = all)"`
	Limit int    `query:"limit" doc:"Max candidates to return (0 = all)"`
}

type stagingCandidate struct {
	ID            string `json:"id"`
	Summary       string `json:"summary"`
	Category      string `json:"category"`
	Scope         string `json:"scope"`
	OriginProject string `json:"origin_project"`
	OriginSession string `json:"origin_session"`
	CreatedAt     string `json:"created_at"`
}

type stagingCandidatesOutput struct {
	// Available reports whether a dotfiles root is configured (so the staging
	// dir can be located). When false the UI shows config as the way to enable.
	Available bool `json:"available"`
	// Total is the full pool size before any scope filter / limit.
	Total int `json:"total"`
	// ByScope counts the full pool per origin scope (user/project), unfiltered.
	ByScope map[string]int `json:"by_scope"`
	// Projects counts candidates per origin_project (project-scoped only).
	Projects   map[string]int     `json:"projects"`
	Candidates []stagingCandidate `json:"candidates"`
}

// humaStagingCandidates reads the raw staging pool from disk on request (the
// pool is ephemeral — drained after consolidation — so it is not indexed into
// the DB). Fail-open: a missing dotfiles root or staging dir yields an empty,
// available=false response rather than an error, matching the memory surface.
func (s *Server) humaStagingCandidates(
	ctx context.Context, in *stagingCandidatesInput,
) (*jsonOutput[stagingCandidatesOutput], error) {
	s.mu.RLock()
	root := s.cfg.ResolveDotfilesRoot()
	s.mu.RUnlock()

	out := stagingCandidatesOutput{
		ByScope:    map[string]int{"user": 0, "project": 0},
		Projects:   map[string]int{},
		Candidates: []stagingCandidate{},
	}
	if root == "" {
		return &jsonOutput[stagingCandidatesOutput]{Body: out}, nil
	}
	out.Available = true

	rawDir := filepath.Join(root, "memory", ".staging", "raw_memories")
	candidates, err := consolidate.ReadCandidates(rawDir)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}

	wantScope := strings.TrimSpace(strings.ToLower(in.Scope))
	for _, c := range candidates {
		scope := c.EffectiveScope()
		out.Total++
		out.ByScope[scope]++
		if scope == "project" && strings.TrimSpace(c.OriginProject) != "" {
			out.Projects[c.OriginProject]++
		}
		if wantScope != "" && scope != wantScope {
			continue
		}
		out.Candidates = append(out.Candidates, stagingCandidate{
			ID:            c.EffectiveID(),
			Summary:       c.Summary,
			Category:      firstNonEmpty(c.Category, c.ProblemType),
			Scope:         scope,
			OriginProject: c.OriginProject,
			OriginSession: c.OriginSession,
			CreatedAt:     c.CreatedAt,
		})
	}

	// Newest first so the most recent captures surface at the top.
	sort.SliceStable(out.Candidates, func(i, j int) bool {
		return out.Candidates[i].CreatedAt > out.Candidates[j].CreatedAt
	})
	if in.Limit > 0 && len(out.Candidates) > in.Limit {
		out.Candidates = out.Candidates[:in.Limit]
	}
	return &jsonOutput[stagingCandidatesOutput]{Body: out}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
