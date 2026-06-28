package consolidate

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	semantic "go.kenn.io/agentsview/internal/search"
)

// systemPrompt instructs the model to act purely as a semantic classifier. It
// deliberately tells the model NOT to police safety: the anti-self-poisoning
// blacklist, the promotion judgement, and the redact gate all live in
// assist_consolidate.py, which re-checks every write regardless of what the
// model says. The model's only job is semantic triage and, for actions that
// target an existing note, selecting one of the provided note_id values.
const systemPrompt = `You consolidate raw memory candidates into a long-lived memory store.
For EACH candidate decide one action:
- "ADD": a genuinely new, reusable fact/decision/pattern worth keeping.
- "UPDATE": it supersedes one provided similar memory; set "note_id" to that note's filename.
- "SKIP": redundant, trivial, one-off, or not worth keeping.
- "DELETE": a provided similar memory is contradicted or obsolete; set "note_id" to that note's filename.
- "INVALIDATE": a provided similar memory should be soft-invalidated; set "note_id" to that note's filename.
For UPDATE, DELETE, and INVALIDATE, note_id MUST be exactly one of the
similar_memories[].note_id values shown for that candidate. If no provided
similar memory is the right target, use ADD or SKIP instead.
Do NOT worry about secrets, anti-poisoning, or promotion thresholds: a downstream
safety script re-checks and can override any decision. You only do semantic triage.
Respond with ONLY a JSON object mapping each candidate id to its decision:
{"<id>": {"action": "ADD|UPDATE|SKIP|DELETE|INVALIDATE", "note_id": "<optional>", "reason": "<short>"}}
Return one entry per candidate id you were given. No prose, no code fences.`

type ExistingNote struct {
	NoteID      string  `json:"note_id"`
	Title       string  `json:"title,omitempty"`
	ProblemType string  `json:"problem_type,omitempty"`
	Status      string  `json:"status,omitempty"`
	Excerpt     string  `json:"excerpt,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

// promptCandidate is the trimmed candidate shape sent to the model. The full
// candidate file is re-read by the python script when it acts, so the prompt
// only needs enough for a semantic judgement.
type promptCandidate struct {
	ID              string         `json:"id"`
	ProblemType     string         `json:"problem_type,omitempty"`
	Summary         string         `json:"summary"`
	Evidence        string         `json:"evidence,omitempty"`
	Implication     string         `json:"implication,omitempty"`
	OriginSession   string         `json:"origin_session,omitempty"`
	SimilarMemories []ExistingNote `json:"similar_memories"`
}

// maxFieldRunes caps each long text field sent to the model so a single huge
// candidate cannot blow the context budget. Truncation is prompt-only; the
// script always reads the full file.
const maxFieldRunes = 1200

// BuildUserPrompt renders the candidate list as a JSON array the model triages.
// Long text fields are truncated for the prompt only.
func BuildUserPrompt(candidates []Candidate, similarByID ...map[string][]ExistingNote) string {
	similar := map[string][]ExistingNote{}
	if len(similarByID) > 0 && similarByID[0] != nil {
		similar = similarByID[0]
	}
	items := make([]promptCandidate, 0, len(candidates))
	for _, c := range candidates {
		pt := c.ProblemType
		if pt == "" {
			pt = c.Category
		}
		id := c.effectiveID()
		notes := similar[id]
		if notes == nil {
			notes = []ExistingNote{}
		}
		items = append(items, promptCandidate{
			ID:              id,
			ProblemType:     pt,
			Summary:         truncateRunes(c.Summary, maxFieldRunes),
			Evidence:        truncateRunes(c.Evidence, maxFieldRunes),
			Implication:     truncateRunes(c.Implication, maxFieldRunes),
			OriginSession:   c.OriginSession,
			SimilarMemories: notes,
		})
	}
	data, err := json.Marshal(items)
	if err != nil {
		// items is plain data; marshal cannot realistically fail, but
		// keep the worker robust rather than panicking on a background tick.
		return "[]"
	}
	return fmt.Sprintf("Candidates to triage:\n%s", string(data))
}

func ExistingNoteFromRecallHit(hit semantic.MemoryRecallHit) ExistingNote {
	return ExistingNote{
		NoteID:      filepath.Base(hit.RelPath),
		Title:       truncateRunes(hit.Title, maxFieldRunes),
		ProblemType: hit.ProblemType,
		Status:      hit.Status,
		Excerpt:     truncateRunes(hit.Excerpt, maxFieldRunes),
		Score:       hit.Score,
	}
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
