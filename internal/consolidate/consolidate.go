// Package consolidate runs the background staging->memory/user consolidation
// worker: it reads raw memory candidates from the staging dir, asks an
// independent LLM for an ADD/UPDATE/SKIP decision per candidate, shells out to
// the dotfiles assist_consolidate.py (which owns ALL safety gates: the
// anti-self-poisoning blacklist, the promotion judgement, and the fail-closed
// redact gate — Go never reimplements them), commits the resulting memory/user
// changes as a local-only repo (never pushed), triggers an immediate memory
// resync so the UI sees the new notes without waiting for the periodic tick,
// and records each run in an append-only jsonl audit log.
//
// Everything that touches the outside world (the LLM call, the python exec,
// and the git commit) goes through small interfaces so tests can mock them and
// never spawn a real process or hit the network.
package consolidate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Action is the decision verb the LLM emits per candidate. The downstream
// python script ultimately re-derives and gate-checks these, but the worker
// passes them through so the script can act (UPDATE/INVALIDATE need note_id).
type Action string

const (
	ActionADD        Action = "ADD"
	ActionUPDATE     Action = "UPDATE"
	ActionSKIP       Action = "SKIP"
	ActionDELETE     Action = "DELETE"
	ActionINVALIDATE Action = "INVALIDATE"
)

// validActions is the closed set the worker will forward to the script. Any
// other verb is coerced defensively (see Decision.normalize).
var validActions = map[Action]bool{
	ActionADD:        true,
	ActionUPDATE:     true,
	ActionSKIP:       true,
	ActionDELETE:     true,
	ActionINVALIDATE: true,
}

// Decision is one per-candidate decision in the LLM's JSON object. It mirrors
// the decision-file schema assist_consolidate.py consumes:
//
//	{cand_id: {"action": "...", "note_id"?: "...", "reason"?: "..."}}
type Decision struct {
	Action Action `json:"action"`
	NoteID string `json:"note_id,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// normalize upgrades a parsed decision to a safe, script-acceptable form:
// the action is upper-cased and any unrecognized/empty verb collapses to SKIP
// so a malformed LLM verb can never cause an unintended write. The script
// still independently gates every write, so SKIP here is purely conservative.
func (d Decision) normalize() Decision {
	d.Action = Action(strings.ToUpper(strings.TrimSpace(string(d.Action))))
	if !validActions[d.Action] {
		d.Action = ActionSKIP
		if d.Reason == "" {
			d.Reason = "unrecognized_action"
		}
	}
	return d
}

// ParseDecisions defensively parses the LLM's raw response into a per-candidate
// decision map. It tolerates the model wrapping its object (e.g. ```json fences
// or a leading/trailing note) by extracting the first balanced JSON object. A
// non-object, unparseable, or empty payload returns an error so the caller can
// skip the cycle and record an audit entry rather than write garbage.
//
// Each entry is normalized so an unknown action becomes SKIP. candidateIDs, if
// non-empty, restricts the returned map to known candidates and drops any
// hallucinated ids the model invented.
func ParseDecisions(raw string, candidateIDs []string) (map[string]Decision, error) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return nil, fmt.Errorf("no JSON object in LLM response")
	}
	var parsed map[string]Decision
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		return nil, fmt.Errorf("decoding decision JSON: %w", err)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("empty decision object")
	}
	known := make(map[string]bool, len(candidateIDs))
	for _, id := range candidateIDs {
		known[id] = true
	}
	out := make(map[string]Decision, len(parsed))
	for id, d := range parsed {
		if len(known) > 0 && !known[id] {
			// Drop ids the model invented for candidates we did not send.
			continue
		}
		out[id] = d.normalize()
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no decisions matched the sent candidates")
	}
	return out, nil
}

// extractJSONObject returns the first top-level {...} block in s, trimming any
// prose or code fences around it. It returns "" when no balanced object is
// found. It is brace-depth based and string-literal aware so a "}" inside a
// string value does not prematurely close the object.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// Candidate is a raw staging memory candidate as read from a *.json file in the
// staging raw dir. Only the fields the prompt needs are decoded; the python
// script re-reads the full file when it acts, so this struct is prompt-only.
type Candidate struct {
	ID            string `json:"id"`
	Category      string `json:"category"`
	ProblemType   string `json:"problem_type"`
	Summary       string `json:"summary"`
	Evidence      string `json:"evidence"`
	Implication   string `json:"implication"`
	OriginSession string `json:"origin_session"`

	// fileName is the on-disk basename (without dir), used to derive the id
	// when the file omits one. Not serialized.
	fileName string
}

// effectiveID returns the candidate's id, falling back to the filename stem
// (matching the python script's `path.stem` fallback) so the decision keys the
// worker emits line up with what the script expects.
func (c Candidate) effectiveID() string {
	if id := strings.TrimSpace(c.ID); id != "" {
		return id
	}
	return strings.TrimSuffix(c.fileName, filepath.Ext(c.fileName))
}

// ReadCandidates reads every *.json file in rawDir as a Candidate, sorted by
// filename for deterministic prompt ordering. A missing dir yields no
// candidates (not an error). A single unparseable file is skipped fail-soft so
// one bad file never blocks the whole cycle.
func ReadCandidates(rawDir string) ([]Candidate, error) {
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading staging dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make([]Candidate, 0, len(names))
	for _, name := range names {
		data, readErr := os.ReadFile(filepath.Join(rawDir, name))
		if readErr != nil {
			continue
		}
		var c Candidate
		if err := json.Unmarshal(data, &c); err != nil {
			// Fail-soft: skip malformed candidate files. The python
			// script will likewise reject them when it runs.
			continue
		}
		c.fileName = name
		out = append(out, c)
	}
	return out, nil
}
