package synthesize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

const (
	defaultMinSimilarity = 0.55
	// defaultMaxClusters caps clusters folded per cycle. With the ~4h interval
	// this comfortably keeps up with new atomics and self-clears a burst backlog
	// over a few cycles (verified: resync between cycles excludes folded sources,
	// so a higher cap does not re-synthesize already-folded clusters).
	defaultMaxClusters = 20
)

// LLMClient is the narrow LLM surface the worker needs.
type LLMClient interface {
	ChatJSON(ctx context.Context, system, user string) (string, error)
}

// NoteStore reads the atomic notes (with embeddings) to cluster.
type NoteStore interface {
	MemoryEmbeddings(ctx context.Context, f db.MemoryFilter) ([]db.Memory, error)
}

// Committer commits the memory dir as a local-only git repo (never pushes).
type Committer interface {
	Commit(ctx context.Context, message string) error
}

// Resyncer triggers an immediate incremental memory resync after a commit.
type Resyncer interface {
	Resync(ctx context.Context) error
}

// Worker runs one synthesis cycle: read atomic notes -> cluster -> per cluster
// LLM distill -> compact_memory.py write -> commit -> resync -> audit.
type Worker struct {
	Root          string // dotfiles root (compact_memory --root)
	MinSimilarity float64
	MaxClusters   int

	Store  NoteStore
	LLM    LLMClient
	Script ScriptRunner
	Commit Committer
	Resync Resyncer
	Audit  *AuditLog

	now func() time.Time
}

func (w *Worker) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func (w *Worker) minSim() float64 {
	if w.MinSimilarity > 0 {
		return w.MinSimilarity
	}
	return defaultMinSimilarity
}

func (w *Worker) maxClusters() int {
	if w.MaxClusters > 0 {
		return w.MaxClusters
	}
	return defaultMaxClusters
}

// RunOnce performs a single synthesis cycle. It never panics on bad LLM output
// or a failing script: each recoverable problem is captured in the record.
func (w *Worker) RunOnce(ctx context.Context) (RunRecord, error) {
	rec := RunRecord{StartedAt: w.clock().UTC().Format(time.RFC3339)}

	mems, err := w.Store.MemoryEmbeddings(ctx, db.MemoryFilter{Source: db.SourceCrossAgent, Status: "active"})
	if err != nil {
		rec.Error = fmt.Sprintf("reading notes: %v", err)
		w.record(rec)
		return rec, err
	}
	notes := SourceNotesFromMemories(mems)
	rec.NoteCount = len(notes)

	clusters := ClusterNotes(notes, w.minSim())
	if cap := w.maxClusters(); len(clusters) > cap {
		clusters = clusters[:cap]
	}
	rec.ClusterCount = len(clusters)
	if len(clusters) == 0 {
		rec.Skipped = true
		rec.Note = "no clusters"
		w.record(rec)
		return rec, nil
	}

	wrote := false
	for _, cluster := range clusters {
		topic := w.synthesizeCluster(ctx, cluster)
		rec.Topics = append(rec.Topics, topic)
		if isWriteResult(topic.Result) {
			wrote = true
			rec.WriteCount++
		}
	}

	if wrote && w.Commit != nil {
		if err := w.Commit.Commit(ctx, w.commitMessage(rec)); err != nil {
			rec.Error = fmt.Sprintf("committing memory: %v", err)
			w.record(rec)
			return rec, nil
		}
		rec.Committed = true
		if w.Resync != nil {
			if err := w.Resync.Resync(ctx); err != nil {
				rec.Note = fmt.Sprintf("resync failed: %v", err)
			} else {
				rec.Resynced = true
			}
		}
	}

	w.record(rec)
	return rec, nil
}

func (w *Worker) synthesizeCluster(ctx context.Context, cluster []SourceNote) TopicRecord {
	sourceIDs := make([]string, 0, len(cluster))
	for _, n := range cluster {
		sourceIDs = append(sourceIDs, n.ID)
	}

	raw, err := w.LLM.ChatJSON(ctx, systemPrompt, BuildUserPrompt(cluster))
	if err != nil {
		return TopicRecord{SourceIDs: sourceIDs, Skipped: true, Error: fmt.Sprintf("llm: %v", err)}
	}
	d, err := ParseLLMDecision(raw)
	if err != nil {
		return TopicRecord{SourceIDs: sourceIDs, Skipped: true, Error: err.Error()}
	}
	if d.Skip || strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Insight) == "" {
		return TopicRecord{Title: d.Title, SourceIDs: sourceIDs, Skipped: true, Result: "skip llm_skip"}
	}

	// Fold the source atomics: mark each stale so it leaves active recall and the
	// INDEX (dual-track — the file stays for audit until the 90d archived-note GC,
	// and is git-recoverable). This is what shrinks the fragmented pool into the
	// coherent topic layer.
	stale := make(map[string]string, len(sourceIDs))
	for _, id := range sourceIDs {
		stale[id] = "folded into topic: " + d.Title
	}
	decision := Decision{
		ID:           synthID(sourceIDs),
		Action:       "ADD",
		Title:        d.Title,
		Insight:      ensureCitations(d.Insight, sourceIDs),
		SourceIDs:    sourceIDs,
		Keywords:     d.Keywords,
		ProblemType:  d.ProblemType,
		StaleSources: stale,
	}
	file, err := writeDecisionFile(decision)
	if err != nil {
		return TopicRecord{Title: d.Title, SourceIDs: sourceIDs, Skipped: true, Error: fmt.Sprintf("decision file: %v", err)}
	}
	defer os.Remove(file)

	res, err := w.Script.Run(ctx, w.Root, file)
	if err != nil {
		return TopicRecord{Title: d.Title, SourceIDs: sourceIDs, Skipped: true, Error: fmt.Sprintf("script: %v", err)}
	}
	result := firstNonEmptyLine(res.Stdout)
	topic := TopicRecord{Title: d.Title, SourceIDs: sourceIDs, Result: result, Skipped: !isWriteResult(result)}
	if strings.TrimSpace(res.Stderr) != "" && result == "" {
		topic.Error = firstNonEmptyLine(res.Stderr)
	}
	return topic
}

// synthID is a deterministic id for a cluster (sorted source ids hash) so
// re-running the same cluster reuses the id and compact_memory's duplicate gate
// keeps it idempotent.
func synthID(sourceIDs []string) string {
	ids := append([]string(nil), sourceIDs...)
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "|")))
	return "syn-" + hex.EncodeToString(sum[:])[:10]
}

func writeDecisionFile(decision Decision) (string, error) {
	data, err := json.Marshal(decision)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "synthesize-decision-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func (w *Worker) commitMessage(rec RunRecord) string {
	return fmt.Sprintf("memory(synthesize): %d topic note(s) at %s", rec.WriteCount, rec.StartedAt)
}

func (w *Worker) record(rec RunRecord) {
	rec.FinishedAt = w.clock().UTC().Format(time.RFC3339)
	if w.Audit != nil {
		_ = w.Audit.Append(rec)
	}
}

func isWriteResult(result string) bool {
	return strings.HasPrefix(strings.TrimSpace(result), "write ")
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
