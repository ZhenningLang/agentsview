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
	// defaultMergeSimilarity gates MERGE routing (atomic cluster -> existing
	// topic, or topic <-> topic). It is deliberately much higher than
	// defaultMinSimilarity: folding raw atomics into a fresh topic is cheap and
	// reversible, but pulling an existing topic into a merge supersedes it, so it
	// must require near-duplicate similarity to avoid collapsing distinct-but-
	// related topics into one. Distinct themes (cosine below this) stay separate.
	defaultMergeSimilarity = 0.82
	// defaultMaxClusters caps clusters planned per cycle. Canonical mode preserves
	// raw inputs, so this is a per-run LLM/spend cap rather than a self-clearing
	// backlog mechanism.
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

// CanonicalStore writes generated canonical rows. It is intentionally scoped to
// source replacement so canonical synthesis cannot mutate raw source rows.
type CanonicalStore interface {
	ReplaceMemoriesBySource(ctx context.Context, source string, memories []db.Memory) error
}

// Committer commits the memory dir as a local-only git repo (never pushes).
type Committer interface {
	Commit(ctx context.Context, message string) error
}

// Resyncer triggers an immediate incremental memory resync after a commit.
type Resyncer interface {
	Resync(ctx context.Context) error
}

// Worker runs one synthesis cycle. With CanonicalStore configured it writes
// raw-preserving source='canonical' DB rows; without it, the legacy compact
// memory script/commit/resync path is retained for compatibility.
type Worker struct {
	Root            string // dotfiles root (compact_memory --root)
	MinSimilarity   float64
	MergeSimilarity float64
	MaxClusters     int
	SourceAllowlist []string

	Store          NoteStore
	CanonicalStore CanonicalStore
	LLM            LLMClient
	Script         ScriptRunner
	Commit         Committer
	Resync         Resyncer
	Audit          *AuditLog

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

func (w *Worker) mergeSim() float64 {
	if w.MergeSimilarity > 0 {
		return w.MergeSimilarity
	}
	return defaultMergeSimilarity
}

func (w *Worker) maxClusters() int {
	if w.MaxClusters > 0 {
		return w.MaxClusters
	}
	return defaultMaxClusters
}

func (w *Worker) sourceAllowlist() []string {
	if len(w.SourceAllowlist) == 0 {
		return []string{db.SourceAssistMem, db.SourceCCNative, db.SourceCrossAgent}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(w.SourceAllowlist))
	for _, source := range w.SourceAllowlist {
		source = strings.TrimSpace(source)
		if source == "" || source == db.SourceCanonical || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	return out
}

// RunOnce performs a single synthesis cycle. It never panics on bad LLM output
// or a failing script: each recoverable problem is captured in the record.
func (w *Worker) RunOnce(ctx context.Context) (RunRecord, error) {
	if w.CanonicalStore != nil {
		return w.runCanonical(ctx, false)
	}
	return w.runLegacyCompact(ctx)
}

// Preview plans the canonical rows that RunOnce would write, without mutating
// the store. It is the dry-run surface for UI/API wiring in later phases.
func (w *Worker) Preview(ctx context.Context) (RunRecord, error) {
	return w.runCanonical(ctx, true)
}

func (w *Worker) runCanonical(ctx context.Context, dryRun bool) (RunRecord, error) {
	rec := RunRecord{StartedAt: w.clock().UTC().Format(time.RFC3339), DryRun: dryRun}
	notes, sourceCounts, eligibleCounts, err := w.loadSourceNotes(ctx)
	rec.SourceCounts = sourceCounts
	rec.EligibleSourceCounts = eligibleCounts
	rec.NoteCount = len(notes)
	if err != nil {
		rec.Error = fmt.Sprintf("reading notes: %v", err)
		w.record(rec)
		return rec, err
	}

	plan, err := w.planCanonical(ctx, notes)
	if err != nil {
		rec.Error = err.Error()
		w.record(rec)
		return rec, err
	}
	rec.ClusterCount = plan.ClusterCount
	rec.PlannedCanonicalCount = len(plan.Rows)
	rec.ConflictSamples = plan.ConflictSamples
	rec.SkippedCount = plan.SkippedCount
	rec.FailedCount = plan.FailedCount
	rec.ConflictCount = plan.ConflictCount
	rec.CoverageRefs = plan.CoverageRefs
	result := "write canonical"
	if dryRun {
		result = "plan canonical"
	}
	for _, row := range plan.Rows {
		rec.Topics = append(rec.Topics, TopicRecord{
			Title:       row.Title,
			SourceIDs:   sourceIDsFromRefsJSON(row.CanonicalCoveredRefs),
			CoveredRefs: rawRefsFromJSON(row.CanonicalCoveredRefs),
			RelPath:     row.RelPath,
			Result:      result,
		})
	}
	rec.Topics = append(rec.Topics, plan.SkippedTopics...)
	if !dryRun && plan.FailedCount > 0 {
		rec.Skipped = true
		rec.Note = "canonical write skipped after failed clusters; previous canonical rows preserved"
		w.record(rec)
		return rec, nil
	}
	if len(plan.Rows) == 0 {
		rec.Skipped = true
		rec.Note = "no canonical clusters; canonical source cleared"
		if !dryRun {
			if err := w.CanonicalStore.ReplaceMemoriesBySource(ctx, db.SourceCanonical, nil); err != nil {
				rec.Error = fmt.Sprintf("clearing canonical memories: %v", err)
				w.record(rec)
				return rec, err
			}
			rec.CanonicalWriteCount = 1
		}
		w.record(rec)
		return rec, nil
	}
	if dryRun {
		w.record(rec)
		return rec, nil
	}
	if err := w.CanonicalStore.ReplaceMemoriesBySource(ctx, db.SourceCanonical, plan.Rows); err != nil {
		rec.Error = fmt.Sprintf("writing canonical memories: %v", err)
		w.record(rec)
		return rec, err
	}
	rec.CanonicalWriteCount = len(plan.Rows)
	rec.WriteCount = len(plan.Rows)
	w.record(rec)
	return rec, nil
}

func (w *Worker) runLegacyCompact(ctx context.Context) (RunRecord, error) {
	rec := RunRecord{StartedAt: w.clock().UTC().Format(time.RFC3339)}

	mems, err := w.Store.MemoryEmbeddings(ctx, db.MemoryFilter{Source: db.SourceCrossAgent, Status: "active"})
	if err != nil {
		rec.Error = fmt.Sprintf("reading notes: %v", err)
		w.record(rec)
		return rec, err
	}
	notes := legacySourceNotesFromMemories(mems)
	rec.NoteCount = len(notes)

	clusters := BuildClusters(notes, w.minSim(), w.mergeSim())
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
	isMerge := clusterHasTopic(cluster)

	system, user := systemPrompt, BuildUserPrompt(cluster)
	if isMerge {
		system, user = mergeSystemPrompt, BuildMergeUserPrompt(cluster)
	}
	raw, err := w.LLM.ChatJSON(ctx, system, user)
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

	// Stale every source so it leaves active recall and the INDEX (dual-track —
	// the file stays for audit until the 90d archived-note GC, and is git-
	// recoverable). For a MERGE, the topic sources carry a "merged" reason and
	// compact_memory tier-retires them (topic->topic supersede via superseded_by);
	// atomics are "folded". This shrinks the fragmented pool into the coherent
	// topic layer and prevents duplicate topics for the same theme.
	stale := make(map[string]string, len(cluster))
	for _, n := range cluster {
		if n.IsTopic {
			stale[n.ID] = "merged into topic: " + d.Title
		} else {
			stale[n.ID] = "folded into topic: " + d.Title
		}
	}
	project, scope := clusterProject(cluster)
	decision := Decision{
		ID:            synthID(sourceIDs),
		Action:        "ADD",
		Title:         d.Title,
		Insight:       ensureCitations(d.Insight, sourceIDs),
		SourceIDs:     sourceIDs,
		Keywords:      d.Keywords,
		ProblemType:   d.ProblemType,
		StaleSources:  stale,
		OriginProject: project,
		Scope:         scope,
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
