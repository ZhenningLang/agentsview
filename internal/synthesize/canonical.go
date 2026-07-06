package synthesize

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

const canonicalSynthesisVersion = "canonical-synthesis-v1"

// RawRef is the stable provenance identity for a raw memory row covered by a
// generated canonical memory row.
type RawRef struct {
	Source  string `json:"source"`
	RelPath string `json:"rel_path"`
}

type canonicalPlan struct {
	Rows            []db.Memory
	ClusterCount    int
	CoverageRefs    []RawRef
	ConflictSamples []string
	SkippedTopics   []TopicRecord
	SkippedCount    int
	FailedCount     int
	ConflictCount   int
}

type canonicalProvenance struct {
	Version      string         `json:"version"`
	SourceCounts map[string]int `json:"source_counts"`
	CoveredCount int            `json:"covered_count"`
	SourceIDs    []string       `json:"source_ids"`
	Scope        string         `json:"scope"`
}

func (w *Worker) loadSourceNotes(ctx context.Context) ([]SourceNote, map[string]int, map[string]int, error) {
	var all []db.Memory
	sourceCounts := map[string]int{}
	for _, source := range w.sourceAllowlist() {
		filter := db.MemoryFilter{Source: source, Status: "active"}
		if source == db.SourceCCNative {
			filter.Status = ""
		}
		mems, err := w.Store.MemoryEmbeddings(ctx, filter)
		if err != nil {
			return nil, sourceCounts, nil, fmt.Errorf("%s: %w", source, err)
		}
		for _, m := range mems {
			if !activeForCanonicalSynthesis(m) {
				continue
			}
			sourceCounts[source]++
			all = append(all, m)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Source != all[j].Source {
			return all[i].Source < all[j].Source
		}
		return all[i].RelPath < all[j].RelPath
	})
	notes := SourceNotesFromMemories(all)
	eligibleCounts := map[string]int{}
	for _, n := range notes {
		eligibleCounts[n.Source]++
	}
	return notes, sourceCounts, eligibleCounts, nil
}

func activeForCanonicalSynthesis(m db.Memory) bool {
	if m.Status == "active" {
		return true
	}
	return m.Source == db.SourceCCNative && strings.TrimSpace(m.Status) == ""
}

func (w *Worker) planCanonical(ctx context.Context, notes []SourceNote) (canonicalPlan, error) {
	clusters, guarded := w.canonicalClusters(notes)
	if cap := w.maxClusters(); len(clusters) > cap {
		clusters = clusters[:cap]
	}
	covered := map[string]bool{}
	plan := canonicalPlan{ClusterCount: len(clusters), ConflictCount: len(guarded)}
	for _, n := range guarded {
		plan.ConflictSamples = append(plan.ConflictSamples, "separate guard: "+n.RawRefID)
	}
	for _, cluster := range clusters {
		row, refs, err := w.synthesizeCanonicalCluster(ctx, cluster)
		if err != nil {
			plan.SkippedCount++
			plan.FailedCount++
			sourceIDs := sourceIDsFromCluster(cluster)
			plan.SkippedTopics = append(plan.SkippedTopics, TopicRecord{
				SourceIDs: sourceIDs,
				Skipped:   true,
				Result:    err.Error(),
				Error:     err.Error(),
			})
			plan.ConflictSamples = append(plan.ConflictSamples, err.Error())
			continue
		}
		plan.Rows = append(plan.Rows, row)
		for _, ref := range refs {
			key := stableRawRefID(ref.Source, ref.RelPath)
			covered[key] = true
			plan.CoverageRefs = append(plan.CoverageRefs, ref)
		}
	}
	for _, n := range notes {
		if !covered[n.RawRefID] && len(plan.ConflictSamples) < 8 {
			plan.ConflictSamples = append(plan.ConflictSamples, "separate singleton: "+n.RawRefID)
		}
	}
	sort.Slice(plan.Rows, func(i, j int) bool { return plan.Rows[i].RelPath < plan.Rows[j].RelPath })
	sort.Slice(plan.CoverageRefs, func(i, j int) bool {
		if plan.CoverageRefs[i].Source != plan.CoverageRefs[j].Source {
			return plan.CoverageRefs[i].Source < plan.CoverageRefs[j].Source
		}
		return plan.CoverageRefs[i].RelPath < plan.CoverageRefs[j].RelPath
	})
	return plan, nil
}

func (w *Worker) canonicalClusters(notes []SourceNote) ([][]SourceNote, []SourceNote) {
	buckets := map[string][]SourceNote{}
	var guarded []SourceNote
	for _, n := range notes {
		key := canonicalBucket(n)
		if key != "fact" {
			guarded = append(guarded, n)
		}
		buckets[key] = append(buckets[key], n)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out [][]SourceNote
	for _, key := range keys {
		out = append(out, BuildClusters(buckets[key], w.minSim(), w.mergeSim())...)
	}
	return out, guarded
}

func canonicalBucket(n SourceNote) string {
	text := strings.ToLower(strings.Join([]string{n.ProblemType, n.Title, n.Body, n.RelPath}, " "))
	switch {
	case strings.Contains(text, "security") || strings.Contains(text, "exception") || strings.Contains(text, "安全") || strings.Contains(text, "例外"):
		return "security"
	case strings.Contains(text, "environment") || strings.Contains(text, "env") || strings.Contains(text, "环境"):
		return "environment"
	default:
		return "fact"
	}
}

func sourceIDsFromCluster(cluster []SourceNote) []string {
	out := make([]string, 0, len(cluster))
	for _, n := range cluster {
		out = append(out, n.RawRefID)
	}
	sort.Strings(out)
	return out
}

func (w *Worker) synthesizeCanonicalCluster(ctx context.Context, cluster []SourceNote) (db.Memory, []RawRef, error) {
	sourceIDs := make([]string, 0, len(cluster))
	refs := make([]RawRef, 0, len(cluster))
	sourceCounts := map[string]int{}
	for _, n := range cluster {
		sourceIDs = append(sourceIDs, n.RawRefID)
		refs = append(refs, RawRef{Source: n.Source, RelPath: n.RelPath})
		sourceCounts[n.Source]++
	}
	sort.Strings(sourceIDs)
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Source != refs[j].Source {
			return refs[i].Source < refs[j].Source
		}
		return refs[i].RelPath < refs[j].RelPath
	})

	if w.LLM == nil {
		return db.Memory{}, refs, fmt.Errorf("canonical cluster %s skipped: no llm", strings.Join(sourceIDs, ","))
	}
	raw, err := w.LLM.ChatJSON(ctx, systemPrompt, BuildUserPrompt(cluster))
	if err != nil {
		return db.Memory{}, refs, fmt.Errorf("canonical cluster %s llm: %w", strings.Join(sourceIDs, ","), err)
	}
	d, err := ParseLLMDecision(raw)
	if err != nil {
		return db.Memory{}, refs, err
	}
	if d.Skip || strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Insight) == "" {
		return db.Memory{}, refs, fmt.Errorf("canonical cluster %s skipped: llm_skip", strings.Join(sourceIDs, ","))
	}

	coveredJSON, err := json.Marshal(refs)
	if err != nil {
		return db.Memory{}, refs, err
	}
	project, scope := clusterProject(cluster)
	now := w.clock().UTC()
	provJSON, err := json.Marshal(canonicalProvenance{
		Version:      canonicalSynthesisVersion,
		SourceCounts: sourceCounts,
		CoveredCount: len(refs),
		SourceIDs:    sourceIDs,
		Scope:        scope,
	})
	if err != nil {
		return db.Memory{}, refs, err
	}
	return db.Memory{
		RelPath:              canonicalRelPath(sourceIDs),
		Source:               db.SourceCanonical,
		Title:                strings.TrimSpace(d.Title),
		Date:                 now.Format("2006-01-02"),
		ProblemType:          strings.TrimSpace(d.ProblemType),
		Type:                 "canonical",
		Status:               "active",
		OriginSession:        "synthesize:" + canonicalSynthesisVersion,
		OriginProject:        project,
		CanonicalCoveredRefs: string(coveredJSON),
		CanonicalProvenance:  string(provJSON),
		Body:                 strings.TrimSpace(d.Insight),
		SyncedAt:             now.Format(time.RFC3339),
	}, refs, nil
}

func rawRefsFromJSON(s string) []RawRef {
	var refs []RawRef
	_ = json.Unmarshal([]byte(s), &refs)
	return refs
}

func sourceIDsFromRefsJSON(s string) []string {
	refs := rawRefsFromJSON(s)
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, stableRawRefID(ref.Source, ref.RelPath))
	}
	return out
}
