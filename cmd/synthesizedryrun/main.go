// Command synthesizedryrun previews topic-aware synthesis clusters against a
// real AgentsView SQLite database without calling an LLM or writing memory data.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"go.kenn.io/agentsview/internal/db"
	semantic "go.kenn.io/agentsview/internal/search"
	"go.kenn.io/agentsview/internal/synthesize"
)

type report struct {
	DBPath              string          `json:"db_path"`
	MinSimilarity       float64         `json:"min_similarity"`
	MergeSimilarity     float64         `json:"merge_similarity"`
	TopicBodyBudgetRune int             `json:"topic_body_budget_runes"`
	Counts              counts          `json:"counts"`
	TopicBodyStats      topicBodyStats  `json:"topic_body_stats"`
	Calibration         calibration     `json:"calibration"`
	Clusters            []clusterReport `json:"clusters"`
	Assertions          map[string]bool `json:"assertions"`
	Warnings            []string        `json:"warnings,omitempty"`
	Errors              []string        `json:"errors,omitempty"`
}

type counts struct {
	Rows             int `json:"rows"`
	SourceNotes      int `json:"source_notes"`
	Atomics          int `json:"atomics"`
	Topics           int `json:"topics"`
	Clusters         int `json:"clusters"`
	AddClusters      int `json:"add_clusters"`
	MergeClusters    int `json:"merge_clusters"`
	OversizedTopics  int `json:"oversized_topics"`
	BlockedOversized int `json:"blocked_oversized_topic_clusters"`
}

type topicBodyStats struct {
	Max int `json:"max"`
	P95 int `json:"p95"`
	P99 int `json:"p99"`
}

type calibration struct {
	TopicPairCount    int             `json:"topic_pair_count"`
	TopicPairStats    similarityStats `json:"topic_pair_stats"`
	TopTopicPairs     []pairReport    `json:"top_topic_pairs"`
	ThresholdScan     []thresholdRow  `json:"threshold_scan"`
	Recommended       float64         `json:"recommended_merge_similarity"`
	RecommendationWhy string          `json:"recommendation_why"`
}

type similarityStats struct {
	Max float64 `json:"max"`
	P99 float64 `json:"p99"`
	P95 float64 `json:"p95"`
	P90 float64 `json:"p90"`
	P75 float64 `json:"p75"`
	P50 float64 `json:"p50"`
}

type pairReport struct {
	A      string  `json:"a"`
	B      string  `json:"b"`
	TitleA string  `json:"title_a"`
	TitleB string  `json:"title_b"`
	Cosine float64 `json:"cosine"`
}

type thresholdRow struct {
	Threshold       float64 `json:"threshold"`
	PairCount       int     `json:"pair_count"`
	TopicMergeCount int     `json:"topic_merge_count"`
}

type clusterReport struct {
	Kind                 string      `json:"kind"`
	IDs                  []string    `json:"ids"`
	TopicIDs             []string    `json:"topic_ids,omitempty"`
	AtomicIDs            []string    `json:"atomic_ids,omitempty"`
	Titles               []noteTitle `json:"titles"`
	OversizedTopicIDs    []string    `json:"oversized_topic_ids,omitempty"`
	PairwiseMin          *float64    `json:"pairwise_min,omitempty"`
	PairwiseMax          *float64    `json:"pairwise_max,omitempty"`
	AtomicTopicMin       *float64    `json:"atomic_topic_min,omitempty"`
	AtomicTopicMax       *float64    `json:"atomic_topic_max,omitempty"`
	WouldCallLLM         bool        `json:"would_call_llm"`
	WouldWriteOnLLMAdd   bool        `json:"would_write_on_llm_add"`
	DeterministicBlocker string      `json:"deterministic_blocker,omitempty"`
}

type noteTitle struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	IsTopic   bool   `json:"is_topic"`
	BodyRunes int    `json:"body_runes"`
}

func main() {
	var dbPath string
	var minSim float64
	var mergeSim float64
	var calibrationOnly bool
	flag.StringVar(&dbPath, "db", os.ExpandEnv("$HOME/.agentsview/sessions.db"), "AgentsView SQLite DB path (opened read-only)")
	flag.Float64Var(&minSim, "min-sim", synthesize.DefaultMinSimilarity(), "atomic fold clustering cosine threshold")
	flag.Float64Var(&mergeSim, "merge-sim", synthesize.DefaultMergeSimilarity(), "topic merge cosine threshold")
	flag.BoolVar(&calibrationOnly, "calibration-only", false, "omit cluster details and print only threshold calibration summary")
	flag.Parse()

	rep, err := run(context.Background(), dbPath, minSim, mergeSim, calibrationOnly)
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	}
	data, marshalErr := json.MarshalIndent(rep, "", "  ")
	if marshalErr != nil {
		fmt.Fprintf(os.Stderr, "marshal report: %v\n", marshalErr)
		os.Exit(1)
	}
	fmt.Println(string(data))
	if err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, dbPath string, minSim, mergeSim float64, calibrationOnly bool) (report, error) {
	rep := report{
		DBPath:              dbPath,
		MinSimilarity:       minSim,
		MergeSimilarity:     mergeSim,
		TopicBodyBudgetRune: synthesize.MaxTopicBodyRunes,
		Assertions:          map[string]bool{},
	}
	mems, err := readActiveMemories(ctx, dbPath)
	if err != nil {
		return rep, err
	}
	notes := synthesize.SourceNotesFromMemories(mems)
	rep.Counts.Rows = len(mems)
	rep.Counts.SourceNotes = len(notes)
	rep.Counts.Atomics, rep.Counts.Topics = countKinds(notes)
	rep.TopicBodyStats, rep.Counts.OversizedTopics = bodyStats(notes)
	rep.Calibration = calibrateTopics(notes, minSim, mergeSim)

	clusters := synthesize.BuildClusters(notes, minSim, mergeSim)
	rep.Counts.Clusters = len(clusters)
	for _, cluster := range clusters {
		cr := analyzeCluster(cluster, mergeSim)
		if cr.Kind == "MERGE" {
			rep.Counts.MergeClusters++
		} else {
			rep.Counts.AddClusters++
		}
		if cr.DeterministicBlocker == "topic_body_over_budget" {
			rep.Counts.BlockedOversized++
		}
		if !calibrationOnly {
			rep.Clusters = append(rep.Clusters, cr)
		}
	}
	rep.Assertions["dry_run_only_no_llm_no_writes"] = true
	rep.Assertions["all_topic_only_merge_clusters_pairwise_gte_threshold"] = allTopicOnlyMergePairwiseOK(rep.Clusters, mergeSim)
	rep.Assertions["all_atomic_topic_members_gte_threshold"] = allAtomicTopicOK(rep.Clusters, mergeSim)
	rep.Assertions["oversized_topics_blocked_before_write"] = oversizedMergesBlocked(rep.Clusters)
	if rep.Counts.Clusters == 0 {
		rep.Warnings = append(rep.Warnings, "no synthesis clusters found")
	}
	return rep, nil
}

func calibrateTopics(notes []synthesize.SourceNote, minSim, mergeSim float64) calibration {
	topics := topicNotes(notes)
	pairs := topicPairs(topics)
	sims := make([]float64, 0, len(pairs))
	for _, p := range pairs {
		sims = append(sims, p.Cosine)
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Cosine > pairs[j].Cosine })
	cal := calibration{
		TopicPairCount: len(pairs),
		TopicPairStats: stats(sims),
		TopTopicPairs:  topN(pairs, 12),
	}
	for _, threshold := range []float64{0.78, 0.80, 0.82, 0.83, 0.84, 0.85, 0.88, 0.90} {
		cal.ThresholdScan = append(cal.ThresholdScan, thresholdRow{
			Threshold:       threshold,
			PairCount:       countPairsAt(pairs, threshold),
			TopicMergeCount: countTopicMergeClusters(notes, minSim, threshold),
		})
	}
	cal.Recommended, cal.RecommendationWhy = recommendThreshold(pairs, mergeSim)
	return cal
}

func topicNotes(notes []synthesize.SourceNote) []synthesize.SourceNote {
	var topics []synthesize.SourceNote
	for _, n := range notes {
		if n.IsTopic {
			topics = append(topics, n)
		}
	}
	sort.Slice(topics, func(i, j int) bool { return topics[i].ID < topics[j].ID })
	return topics
}

func topicPairs(topics []synthesize.SourceNote) []pairReport {
	var pairs []pairReport
	for i := 0; i < len(topics); i++ {
		for j := i + 1; j < len(topics); j++ {
			sim, ok := semantic.Cosine(topics[i].Embedding, topics[j].Embedding)
			if !ok {
				continue
			}
			pairs = append(pairs, pairReport{A: topics[i].ID, B: topics[j].ID, TitleA: topics[i].Title, TitleB: topics[j].Title, Cosine: sim})
		}
	}
	return pairs
}

func stats(vals []float64) similarityStats {
	if len(vals) == 0 {
		return similarityStats{}
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	return similarityStats{
		Max: sorted[len(sorted)-1],
		P99: percentileFloat(sorted, 0.99),
		P95: percentileFloat(sorted, 0.95),
		P90: percentileFloat(sorted, 0.90),
		P75: percentileFloat(sorted, 0.75),
		P50: percentileFloat(sorted, 0.50),
	}
}

func percentileFloat(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	idx := int(float64(len(vals)-1) * p)
	return vals[idx]
}

func topN(pairs []pairReport, n int) []pairReport {
	if len(pairs) < n {
		n = len(pairs)
	}
	return append([]pairReport(nil), pairs[:n]...)
}

func countPairsAt(pairs []pairReport, threshold float64) int {
	count := 0
	for _, p := range pairs {
		if p.Cosine >= threshold {
			count++
		}
	}
	return count
}

func countTopicMergeClusters(notes []synthesize.SourceNote, minSim, threshold float64) int {
	count := 0
	for _, cluster := range synthesize.BuildClusters(notes, minSim, threshold) {
		atoms, topics := 0, 0
		for _, n := range cluster {
			if n.IsTopic {
				topics++
			} else {
				atoms++
			}
		}
		if topics >= 2 && atoms == 0 {
			count++
		}
	}
	return count
}

func recommendThreshold(pairs []pairReport, fallback float64) (float64, string) {
	if len(pairs) == 0 {
		return fallback, "no topic pairs available; keep configured fallback"
	}
	sorted := append([]pairReport(nil), pairs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Cosine > sorted[j].Cosine })
	if len(sorted) == 1 {
		return sorted[0].Cosine, "only one topic pair available; use its cosine as the review boundary"
	}
	bestIdx := 0
	bestGap := sorted[0].Cosine - sorted[1].Cosine
	limit := len(sorted) - 1
	if limit > 12 {
		limit = 12
	}
	for i := 1; i < limit; i++ {
		gap := sorted[i].Cosine - sorted[i+1].Cosine
		if gap > bestGap {
			bestGap = gap
			bestIdx = i
		}
	}
	threshold := (sorted[bestIdx].Cosine + sorted[bestIdx+1].Cosine) / 2
	return threshold, fmt.Sprintf("largest top-pair gap %.6f is between rank %d (%.6f) and rank %d (%.6f)", bestGap, bestIdx+1, sorted[bestIdx].Cosine, bestIdx+2, sorted[bestIdx+1].Cosine)
}

func readActiveMemories(ctx context.Context, dbPath string) ([]db.Memory, error) {
	conn, err := sql.Open("sqlite3", readOnlyDSN(dbPath))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, `SELECT rel_path, source, title, status, origin_session, origin_project, body, llm_embedding, llm_embedding_dim
		FROM memory
		WHERE source = ? AND status = ? AND llm_embedding IS NOT NULL AND llm_embedding_dim > 0
		ORDER BY rel_path`, db.SourceCrossAgent, "active")
	if err != nil {
		return nil, fmt.Errorf("query active memory embeddings: %w", err)
	}
	defer rows.Close()
	var out []db.Memory
	for rows.Next() {
		var m db.Memory
		var data []byte
		var dim int
		if err := rows.Scan(&m.RelPath, &m.Source, &m.Title, &m.Status, &m.OriginSession, &m.OriginProject, &m.Body, &data, &dim); err != nil {
			return nil, err
		}
		vec, err := db.DecodeEmbedding(data, dim)
		if err != nil {
			return nil, fmt.Errorf("decode embedding %q: %w", m.RelPath, err)
		}
		m.LLMEmbedding = vec
		m.LLMEmbeddingDim = dim
		out = append(out, m)
	}
	return out, rows.Err()
}

func readOnlyDSN(path string) string {
	params := url.Values{}
	params.Set("mode", "ro")
	params.Set("_query_only", "1")
	params.Set("_busy_timeout", "5000")
	return "file:" + path + "?" + params.Encode()
}

func countKinds(notes []synthesize.SourceNote) (atomics, topics int) {
	for _, n := range notes {
		if n.IsTopic {
			topics++
		} else {
			atomics++
		}
	}
	return atomics, topics
}

func bodyStats(notes []synthesize.SourceNote) (topicBodyStats, int) {
	var lens []int
	oversized := 0
	for _, n := range notes {
		if !n.IsTopic {
			continue
		}
		l := runeLen(n.Body)
		lens = append(lens, l)
		if l > synthesize.MaxTopicBodyRunes {
			oversized++
		}
	}
	sort.Ints(lens)
	return topicBodyStats{Max: percentile(lens, 1), P95: percentile(lens, 0.95), P99: percentile(lens, 0.99)}, oversized
}

func percentile(vals []int, p float64) int {
	if len(vals) == 0 {
		return 0
	}
	idx := int(float64(len(vals)-1) * p)
	return vals[idx]
}

func analyzeCluster(cluster []synthesize.SourceNote, mergeSim float64) clusterReport {
	cr := clusterReport{Kind: "ADD", WouldCallLLM: true, WouldWriteOnLLMAdd: true}
	for _, n := range cluster {
		cr.IDs = append(cr.IDs, n.ID)
		cr.Titles = append(cr.Titles, noteTitle{ID: n.ID, Title: n.Title, IsTopic: n.IsTopic, BodyRunes: runeLen(n.Body)})
		if n.IsTopic {
			cr.Kind = "MERGE"
			cr.TopicIDs = append(cr.TopicIDs, n.ID)
			if runeLen(n.Body) > synthesize.MaxTopicBodyRunes {
				cr.OversizedTopicIDs = append(cr.OversizedTopicIDs, n.ID)
			}
		} else {
			cr.AtomicIDs = append(cr.AtomicIDs, n.ID)
		}
	}
	if cr.Kind == "MERGE" {
		cr.PairwiseMin, cr.PairwiseMax = pairwiseRange(cluster)
		if len(cr.AtomicIDs) > 0 && len(cr.TopicIDs) > 0 {
			cr.AtomicTopicMin, cr.AtomicTopicMax = atomicTopicRange(cluster)
		}
		if len(cr.OversizedTopicIDs) > 0 {
			cr.WouldCallLLM = false
			cr.WouldWriteOnLLMAdd = false
			cr.DeterministicBlocker = "topic_body_over_budget"
		}
	}
	return cr
}

func pairwiseRange(cluster []synthesize.SourceNote) (*float64, *float64) {
	var min, max float64
	seen := false
	for i := 0; i < len(cluster); i++ {
		for j := i + 1; j < len(cluster); j++ {
			sim, ok := semantic.Cosine(cluster[i].Embedding, cluster[j].Embedding)
			if !ok {
				continue
			}
			if !seen || sim < min {
				min = sim
			}
			if !seen || sim > max {
				max = sim
			}
			seen = true
		}
	}
	if !seen {
		return nil, nil
	}
	return &min, &max
}

func atomicTopicRange(cluster []synthesize.SourceNote) (*float64, *float64) {
	var min, max float64
	seen := false
	for _, a := range cluster {
		if a.IsTopic {
			continue
		}
		for _, t := range cluster {
			if !t.IsTopic {
				continue
			}
			sim, ok := semantic.Cosine(a.Embedding, t.Embedding)
			if !ok {
				continue
			}
			if !seen || sim < min {
				min = sim
			}
			if !seen || sim > max {
				max = sim
			}
			seen = true
		}
	}
	if !seen {
		return nil, nil
	}
	return &min, &max
}

func allTopicOnlyMergePairwiseOK(clusters []clusterReport, threshold float64) bool {
	for _, c := range clusters {
		if c.Kind == "MERGE" && len(c.AtomicIDs) == 0 && c.PairwiseMin != nil && *c.PairwiseMin < threshold {
			return false
		}
	}
	return true
}

func allAtomicTopicOK(clusters []clusterReport, threshold float64) bool {
	for _, c := range clusters {
		if c.Kind == "MERGE" && c.AtomicTopicMin != nil && *c.AtomicTopicMin < threshold {
			return false
		}
	}
	return true
}

func oversizedMergesBlocked(clusters []clusterReport) bool {
	for _, c := range clusters {
		if len(c.OversizedTopicIDs) > 0 && c.DeterministicBlocker != "topic_body_over_budget" {
			return false
		}
	}
	return true
}

func runeLen(s string) int {
	return len([]rune(strings.TrimSpace(s)))
}
