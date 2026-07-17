// Package synthesize runs the optional topic-synthesis worker: it clusters the
// atomic memory notes by embedding similarity and, per cluster, asks an LLM to
// distill one coherent, structured, source-language (中文) topic note. The
// deterministic write — citation validation, redact, INDEX rebuild, marking the
// source atomics stale — is delegated to the dotfiles compact_memory.py SSOT,
// mirroring how consolidate delegates to assist_consolidate.py.
package synthesize

import (
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/db"
	semantic "go.kenn.io/agentsview/internal/search"
)

// synthesizedOriginPrefix marks notes produced by compact_memory; they are
// included as existing topic notes so new near-duplicate atomics/topics can merge
// back into the topic layer instead of creating duplicate topics.
const synthesizedOriginPrefix = "compact-memory:"

// SourceNote is a note eligible to take part in a topic synthesis cluster — an
// atomic note to fold, or (IsTopic) an existing topic note to merge/supersede.
// ID is the note's filename stem (what compact_memory cites via "(because of
// <id>)").
type SourceNote struct {
	ID        string
	Title     string
	Body      string
	Project   string // origin_project of the source note ("" = general)
	Embedding []float32
	// IsTopic marks a note already produced by compact_memory (origin_session
	// starts with the synthesizedOriginPrefix). Such notes are NOT clustered as
	// raw input; they are only merge targets/sources so the worker can fold a
	// growing theme back into the existing topic instead of spawning a duplicate.
	IsTopic bool
}

// SourceNotesFromMemories converts active cross-agent notes into source notes.
// It now INCLUDES already-synthesized topic notes (flagged IsTopic) so the
// worker can merge into / dedup existing topics instead of always adding a new
// one. It still drops INDEX and notes without an embedding (clustering needs the
// vector).
func SourceNotesFromMemories(memories []db.Memory) []SourceNote {
	out := make([]SourceNote, 0, len(memories))
	for _, m := range memories {
		// status is filtered to active at the DB query; here we only drop notes
		// that are unsuitable as synthesis input regardless of status.
		if len(m.LLMEmbedding) == 0 {
			continue
		}
		id := strings.TrimSuffix(m.RelPath, ".md")
		if id == "" || strings.EqualFold(id, "INDEX") {
			continue
		}
		isTopic := strings.HasPrefix(m.OriginSession, synthesizedOriginPrefix)
		out = append(out, SourceNote{ID: id, Title: m.Title, Body: m.Body, Project: m.OriginProject, Embedding: m.LLMEmbedding, IsTopic: isTopic})
	}
	return out
}

// ClusterNotes greedily groups notes whose embeddings are within minSim cosine
// of a cluster seed. Deterministic: notes are processed in ID order, each note
// joins the first seed it is close enough to (or starts a new cluster). Only
// clusters with >= 2 members are returned (a single note has nothing to fold).
func ClusterNotes(notes []SourceNote, minSim float64) [][]SourceNote {
	ordered := make([]SourceNote, len(notes))
	copy(ordered, notes)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	var clusters [][]SourceNote
	assigned := make([]bool, len(ordered))
	for i := range ordered {
		if assigned[i] {
			continue
		}
		cluster := []SourceNote{ordered[i]}
		assigned[i] = true
		for j := i + 1; j < len(ordered); j++ {
			if assigned[j] {
				continue
			}
			if sim, ok := semantic.Cosine(ordered[i].Embedding, ordered[j].Embedding); ok && sim >= minSim {
				cluster = append(cluster, ordered[j])
				assigned[j] = true
			}
		}
		if len(cluster) >= 2 {
			clusters = append(clusters, cluster)
		}
	}
	return clusters
}

// bestMergeableTopicMatch returns the best active topic that every atomic in the
// cluster can safely merge into. Requiring every member to clear mergeMinSim
// avoids retiring a topic because only one atomic in a loose fold cluster was a
// near-duplicate of it.
func bestMergeableTopicMatch(cluster []SourceNote, topics []SourceNote, mergeMinSim float64) (idx int, best float64) {
	idx = -1
	for ti, t := range topics {
		topicBest := 0.0
		okAll := true
		for _, m := range cluster {
			sim, ok := semantic.Cosine(m.Embedding, t.Embedding)
			if !ok || sim < mergeMinSim {
				okAll = false
				break
			}
			if sim > topicBest {
				topicBest = sim
			}
		}
		if okAll && (idx == -1 || topicBest > best) {
			idx = ti
			best = topicBest
		}
	}
	return idx, best
}

func closeToAll(candidate SourceNote, group []SourceNote, mergeMinSim float64) bool {
	for _, existing := range group {
		if sim, ok := semantic.Cosine(existing.Embedding, candidate.Embedding); !ok || sim < mergeMinSim {
			return false
		}
	}
	return true
}

// BuildClusters routes the active notes into the clusters to synthesize this
// cycle, deterministically and conservatively:
//
//  1. Cluster ATOMIC-only notes at minSim (existing fold behavior).
//  2. For each atomic cluster, find the closest active topic that every atomic
//     member matches at >= mergeMinSim. That cluster becomes a MERGE into the
//     topic; otherwise it stays an ADD of a brand-new topic.
//  3. Dedup remaining active topics with a complete-link guard: a candidate
//     joins a merge group only when it is >= mergeMinSim to every group member.
//
// A note is emitted in at most one cluster per cycle. mergeMinSim is held high
// (near-duplicate) so distinct-but-related topics are never merged.
func BuildClusters(notes []SourceNote, minSim, mergeMinSim float64) [][]SourceNote {
	var atomics, topics []SourceNote
	for _, n := range notes {
		if n.IsTopic {
			topics = append(topics, n)
		} else {
			atomics = append(atomics, n)
		}
	}
	sort.Slice(topics, func(i, j int) bool { return topics[i].ID < topics[j].ID })

	assignedTopic := make([]bool, len(topics))
	var out [][]SourceNote

	// 1 + 2: atomic clusters, optionally merged into a near-duplicate topic.
	for _, cluster := range ClusterNotes(atomics, minSim) {
		// Only consider topics not already consumed by an earlier cluster, to keep
		// each note in at most one emitted cluster.
		free := make([]SourceNote, 0, len(topics))
		freeIdx := make([]int, 0, len(topics))
		for ti := range topics {
			if !assignedTopic[ti] {
				free = append(free, topics[ti])
				freeIdx = append(freeIdx, ti)
			}
		}
		if ti, best := bestMergeableTopicMatch(cluster, free, mergeMinSim); ti >= 0 && best >= mergeMinSim {
			cluster = append(cluster, free[ti])
			assignedTopic[freeIdx[ti]] = true
		}
		out = append(out, cluster)
	}

	// 3: dedup the still-free topics among themselves.
	for i := range topics {
		if assignedTopic[i] {
			continue
		}
		group := []SourceNote{topics[i]}
		assignedTopic[i] = true
		for j := i + 1; j < len(topics); j++ {
			if assignedTopic[j] {
				continue
			}
			if closeToAll(topics[j], group, mergeMinSim) {
				group = append(group, topics[j])
				assignedTopic[j] = true
			}
		}
		if len(group) >= 2 {
			out = append(out, group)
		}
	}
	return out
}

// clusterHasTopic reports whether any member of a cluster is an existing topic
// note, i.e. the cluster is a MERGE rather than a fresh ADD.
func clusterHasTopic(cluster []SourceNote) bool {
	for _, n := range cluster {
		if n.IsTopic {
			return true
		}
	}
	return false
}

// Decision is the JSON written to compact_memory.py's --decision-file. It must
// carry a "(because of <id>)" citation for each source in the insight; the
// worker guarantees that before invoking the script.
type Decision struct {
	ID           string            `json:"id"`
	Action       string            `json:"action"`
	Title        string            `json:"title"`
	Insight      string            `json:"insight"`
	SourceIDs    []string          `json:"source_ids"`
	Keywords     []string          `json:"keywords,omitempty"`
	ProblemType  string            `json:"problem_type,omitempty"`
	StaleSources map[string]string `json:"stale_sources,omitempty"`
	// OriginProject/Scope carry the topic's project dimension to
	// compact_memory.py: a single shared project (scope=project) or a
	// cross-project topic (scope=general, empty OriginProject = General bucket).
	OriginProject string `json:"origin_project,omitempty"`
	Scope         string `json:"scope,omitempty"`
}

// clusterProject derives the project dimension for a cluster of source notes:
// when every source shares one non-empty project the topic belongs to that
// project (scope=project); otherwise it is a cross-project topic (scope=general,
// empty project = the General bucket). This makes the project label emerge
// naturally from clustering — single-project topics stay tagged, cross-project
// lessons become general.
func clusterProject(cluster []SourceNote) (project, scope string) {
	seen := ""
	for _, n := range cluster {
		p := strings.TrimSpace(n.Project)
		if p == "" {
			return "", "general" // any general source makes the topic general
		}
		if seen == "" {
			seen = p
			continue
		}
		if p != seen {
			return "", "general" // spans multiple projects
		}
	}
	if seen == "" {
		return "", "general"
	}
	return seen, "project"
}

// ensureCitations appends a "(because of <id>)" for every source id missing from
// the insight, so compact_memory's citation gate (>=2 known sources) always
// passes even if the model omitted them.
func ensureCitations(insight string, sourceIDs []string) string {
	out := strings.TrimRight(strings.TrimSpace(insight), " ")
	for _, id := range sourceIDs {
		if !strings.Contains(out, "(because of "+id+")") {
			out += " (because of " + id + ")"
		}
	}
	return out
}
