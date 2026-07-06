package synthesize

import (
	"encoding/json"
	"fmt"
	"strings"
)

// systemPrompt instructs the model to distill one cluster of atomic notes into a
// single coherent topic note, in the notes' own language (中文 when they are
// Chinese), with a structured body. It is a pure writer: redact / citation /
// promotion safety all live in compact_memory.py downstream.
const systemPrompt = `You distill several related atomic memory notes into ONE coherent topic note.
Write the title and insight in 中文 (Chinese) — this note is for a Chinese-reading user, so translate and synthesize even when the source notes are in English. Keep code identifiers, file names, commands and API names verbatim.
Return JSON only: {"skip":false,"title":"...","problem_type":"decision|correction|preference|failure-mode|knowledge|pattern|bug","insight":"...","keywords":["..."]}.
- title: 一个简短的中文主题标题(这些 note 共同的主题)。
- insight: 用结构化、可读的中文综合这些 note 共同表达的知识 —— 合并重复、保留每个独立要点、丢弃一次性噪声。用短段落或「- 」列表,多行可以。
- Set "skip":true only when the notes do not actually share a coherent topic.
Do NOT invent facts beyond the notes. Do NOT include secrets. Keep JSON keys and problem_type values in English.`

// mergeSystemPrompt instructs the model to merge a cluster that contains one or
// more EXISTING topic notes (plus possibly new atomics, or another topic) into
// ONE coherent merged topic that preserves every distinct point — or to SKIP
// when the cluster is not actually a single topic (the candidates cover distinct
// themes). The downstream write supersedes the old topic(s) with the new note,
// so a wrong merge loses information; conservatism (SKIP) is preferred.
const mergeSystemPrompt = `You merge memory notes that may include one or more EXISTING topic notes into ONE coherent topic note.
Some inputs are existing topic notes (is_topic=true) and some may be new atomic notes. The merged note will REPLACE the existing topic note(s), so you MUST preserve every distinct point from all inputs — never drop information that is in an existing topic.
Write the title and insight in 中文 (Chinese) — this note is for a Chinese-reading user, so translate and synthesize even when the source notes are in English. Keep code identifiers, file names, commands and API names verbatim.
Return JSON only: {"skip":false,"title":"...","problem_type":"decision|correction|preference|failure-mode|knowledge|pattern|bug","insight":"...","keywords":["..."]}.
- title: 一个简短的中文主题标题(覆盖合并后的整个主题)。
- insight: 用结构化、可读的中文综合所有 note 的知识 —— 合并重复、保留每个独立要点(尤其是已有 topic 中的要点)、丢弃一次性噪声。用短段落或「- 」列表,多行可以。
- Set "skip":true when the inputs do NOT actually describe ONE coherent topic (they cover distinct themes that should stay separate). When in doubt, SKIP — a wrong merge supersedes and loses the existing topic.
Do NOT invent facts beyond the notes. Do NOT include secrets. Keep JSON keys and problem_type values in English.`

// promptNote is the trimmed note sent to the model. IsTopic flags an existing
// topic note so the merge prompt can tell the model which inputs it must not lose.
type promptNote struct {
	ID      string `json:"id"`
	Source  string `json:"source,omitempty"`
	RelPath string `json:"rel_path,omitempty"`
	Title   string `json:"title"`
	Body    string `json:"body,omitempty"`
	IsTopic bool   `json:"is_topic,omitempty"`
}

const maxBodyRunes = 800

// maxTopicBodyRunes is larger than maxBodyRunes: an existing topic note is
// already a synthesis and the model must preserve all of it, so it gets more room.
const maxTopicBodyRunes = 2000

// BuildUserPrompt renders an all-atomic cluster as the JSON the model distills.
func BuildUserPrompt(cluster []SourceNote) string {
	items := make([]promptNote, 0, len(cluster))
	for _, n := range cluster {
		items = append(items, promptNote{ID: n.ID, Source: n.Source, RelPath: n.RelPath, Title: n.Title, Body: truncateRunes(n.Body, maxBodyRunes)})
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return fmt.Sprintf("Atomic notes that appear to share a topic:\n%s", string(data))
}

// BuildMergeUserPrompt renders a merge cluster (>=1 existing topic note) as the
// JSON the model merges. Topic bodies get more room so no point is dropped.
func BuildMergeUserPrompt(cluster []SourceNote) string {
	items := make([]promptNote, 0, len(cluster))
	for _, n := range cluster {
		limit := maxBodyRunes
		if n.IsTopic {
			limit = maxTopicBodyRunes
		}
		items = append(items, promptNote{ID: n.ID, Source: n.Source, RelPath: n.RelPath, Title: n.Title, Body: truncateRunes(n.Body, limit), IsTopic: n.IsTopic})
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return fmt.Sprintf("Notes to merge into ONE topic (is_topic=true are existing topic notes you must not lose; SKIP if they are not one topic):\n%s", string(data))
}

// LLMDecision is the model-facing synthesis result. Source ids and the final
// citations are filled by the worker so a hallucinated value cannot move provenance.
type LLMDecision struct {
	Skip        bool     `json:"skip"`
	Title       string   `json:"title"`
	ProblemType string   `json:"problem_type"`
	Insight     string   `json:"insight"`
	Keywords    []string `json:"keywords"`
}

// ParseLLMDecision extracts the synthesis decision from the model's raw output,
// tolerating a ```json code fence.
func ParseLLMDecision(raw string) (LLMDecision, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	var d LLMDecision
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		return LLMDecision{}, fmt.Errorf("parsing synthesis decision: %w", err)
	}
	return d, nil
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
