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
// excluded from clustering so synthesis never feeds on its own output.
const synthesizedOriginPrefix = "compact-memory:"

// SourceNote is an atomic note eligible to be folded into a topic note. ID is
// the note's filename stem (what compact_memory cites via "(because of <id>)").
type SourceNote struct {
	ID        string
	Title     string
	Body      string
	Embedding []float32
}

// SourceNotesFromMemories converts active cross-agent atomic notes into source
// notes, dropping INDEX, already-synthesized notes, and notes without an
// embedding (clustering needs the vector).
func SourceNotesFromMemories(memories []db.Memory) []SourceNote {
	out := make([]SourceNote, 0, len(memories))
	for _, m := range memories {
		// status is filtered to active at the DB query; here we only drop notes
		// that are unsuitable as synthesis input regardless of status.
		if strings.HasPrefix(m.OriginSession, synthesizedOriginPrefix) {
			continue
		}
		if len(m.LLMEmbedding) == 0 {
			continue
		}
		id := strings.TrimSuffix(m.RelPath, ".md")
		if id == "" || strings.EqualFold(id, "INDEX") {
			continue
		}
		out = append(out, SourceNote{ID: id, Title: m.Title, Body: m.Body, Embedding: m.LLMEmbedding})
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
