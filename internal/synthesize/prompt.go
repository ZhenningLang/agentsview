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
Write in the SAME natural language as the notes (use 中文 when they are mostly Chinese).
Return JSON only: {"skip":false,"title":"...","problem_type":"decision|correction|preference|failure-mode|knowledge|pattern|bug","insight":"...","keywords":["..."]}.
- title: a short topic title (the theme the notes share).
- insight: a structured, readable synthesis of what these notes collectively teach — merge duplicates, keep every distinct point, drop one-off noise. Use short paragraphs or "- " bullets; multiple lines are fine.
- Set "skip":true only when the notes do not actually share a coherent topic.
Do NOT invent facts beyond the notes. Do NOT include secrets. Keep JSON keys and problem_type values in English.`

// promptNote is the trimmed atomic note sent to the model.
type promptNote struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
}

const maxBodyRunes = 800

// BuildUserPrompt renders a cluster as the JSON the model distills.
func BuildUserPrompt(cluster []SourceNote) string {
	items := make([]promptNote, 0, len(cluster))
	for _, n := range cluster {
		items = append(items, promptNote{ID: n.ID, Title: n.Title, Body: truncateRunes(n.Body, maxBodyRunes)})
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return fmt.Sprintf("Atomic notes that appear to share a topic:\n%s", string(data))
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
