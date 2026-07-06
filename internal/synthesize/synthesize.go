// Package synthesize runs the optional memory-synthesis worker: canonical mode
// clusters raw memory rows by embedding similarity and writes generated
// source='canonical' DB rows with raw provenance. The legacy compact-memory
// script path remains available when no CanonicalStore is configured.
package synthesize

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/db"
	semantic "go.kenn.io/agentsview/internal/search"
)

// synthesizedOriginPrefix marks notes produced by compact_memory; they are
// excluded from clustering so synthesis never feeds on its own output.
const synthesizedOriginPrefix = "compact-memory:"

// SourceNote is a memory row eligible to take part in a synthesis cluster. In
// canonical mode ID/RawRefID are stable {source, rel_path} refs; in legacy mode
// ID is kept as the compact_memory citation stem.
type SourceNote struct {
	ID          string
	Source      string
	RelPath     string
	RawRefID    string
	Title       string
	Body        string
	Project     string // origin_project of the source note ("" = general)
	ProblemType string
	Embedding   []float32
	// IsTopic marks a note already produced by compact_memory (origin_session
	// starts with the synthesizedOriginPrefix). Such notes are NOT clustered as
	// raw input; they are only merge targets/sources so the worker can fold a
	// growing theme back into the existing topic instead of spawning a duplicate.
	IsTopic bool
}

// SourceNotesFromMemories converts raw memory rows into source notes. It drops
// canonical rows, INDEX, and rows without embeddings because clustering needs a
// vector. Existing compact-memory topics are flagged IsTopic for the legacy
// path.
func SourceNotesFromMemories(memories []db.Memory) []SourceNote {
	return sourceNotesFromMemories(memories, false)
}

func legacySourceNotesFromMemories(memories []db.Memory) []SourceNote {
	return sourceNotesFromMemories(memories, true)
}

func sourceNotesFromMemories(memories []db.Memory, legacyIDs bool) []SourceNote {
	out := make([]SourceNote, 0, len(memories))
	for _, m := range memories {
		if m.Source == db.SourceCanonical {
			continue
		}
		// status is filtered to active at the DB query; here we only drop notes
		// that are unsuitable as synthesis input regardless of status.
		if len(m.LLMEmbedding) == 0 {
			continue
		}
		source := m.Source
		if source == "" {
			source = db.SourceCrossAgent
		}
		stem := strings.TrimSuffix(m.RelPath, ".md")
		if stem == "" || strings.EqualFold(stem, "INDEX") {
			continue
		}
		id := stableRawRefID(source, m.RelPath)
		if legacyIDs {
			id = stem
		}
		isTopic := strings.HasPrefix(m.OriginSession, synthesizedOriginPrefix)
		out = append(out, SourceNote{
			ID:          id,
			Source:      source,
			RelPath:     m.RelPath,
			RawRefID:    stableRawRefID(source, m.RelPath),
			Title:       m.Title,
			Body:        m.Body,
			Project:     m.OriginProject,
			ProblemType: m.ProblemType,
			Embedding:   m.LLMEmbedding,
			IsTopic:     isTopic,
		})
	}
	return out
}

func stableRawRefID(source, relPath string) string {
	return strings.TrimSpace(source) + ":" + strings.TrimSpace(relPath)
}

func canonicalRelPath(refIDs []string) string {
	ids := append([]string(nil), refIDs...)
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "|")))
	return "canonical/" + hex.EncodeToString(sum[:])[:16] + ".json"
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

// bestTopicMatch returns the index of the active topic in topics whose embedding
// is closest (cosine) to any member of cluster, and that best similarity. It
// returns (-1, 0) when there are no topics or none has a usable embedding.
func bestTopicMatch(cluster []SourceNote, topics []SourceNote) (idx int, best float64) {
	idx = -1
	for ti, t := range topics {
		for _, m := range cluster {
			if sim, ok := semantic.Cosine(m.Embedding, t.Embedding); ok && (idx == -1 || sim > best) {
				idx = ti
				best = sim
			}
		}
	}
	return idx, best
}

// BuildClusters routes the active notes into the clusters to synthesize this
// cycle, deterministically and conservatively:
//
//  1. Cluster ATOMIC-only notes at minSim (existing fold behavior).
//  2. For each atomic cluster, find the closest active topic. If that cosine is
//     >= mergeMinSim the cluster becomes a MERGE into that topic (the topic is
//     added as a source); otherwise it stays an ADD of a brand-new topic.
//  3. Dedup remaining (unassigned) active topics: greedily group topics whose
//     pairwise cosine is >= mergeMinSim; any group of >= 2 becomes a MERGE
//     cluster (all those topics are sources).
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
		if ti, best := bestTopicMatch(cluster, free); ti >= 0 && best >= mergeMinSim {
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
			if sim, ok := semantic.Cosine(topics[i].Embedding, topics[j].Embedding); ok && sim >= mergeMinSim {
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
