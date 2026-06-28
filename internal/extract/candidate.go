// Package extract runs the optional LLM session->raw-memory extraction worker.
// It writes only raw staging candidates; promotion into memory/user remains
// owned by the downstream consolidation path.
package extract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/secrets"
)

var originSessionRE = regexp.MustCompile(`[^A-Za-z0-9_.:-]+`)

var problemTypeForCategory = map[string]string{
	"decision":     "decision",
	"correction":   "correction",
	"preference":   "preference",
	"failure-mode": "failure-mode",
	"failure_mode": "failure-mode",
	"fact":         "knowledge",
	"knowledge":    "knowledge",
	"pattern":      "pattern",
	"bug":          "bug",
}

// Candidate is the raw JSON written under memory/.staging/raw_memories. The
// shape intentionally matches dotfiles memory_capture.py and adds fields the
// consolidation promotion gate already understands (why/problem_type).
type Candidate struct {
	Summary        string   `json:"summary"`
	Evidence       string   `json:"evidence"`
	Implication    string   `json:"implication"`
	Category       string   `json:"category"`
	ProblemType    string   `json:"problem_type,omitempty"`
	OriginSession  string   `json:"origin_session"`
	OriginProject  string   `json:"origin_project"`
	Scope          string   `json:"scope"`
	SourcePlatform string   `json:"source_platform"`
	SourceRoles    []string `json:"source_roles"`
	Why            string   `json:"why,omitempty"`
	CreatedAt      string   `json:"created_at"`
	ID             string   `json:"id"`
}

// LLMCandidate is the model-facing subset. Session/source fields are filled by
// the worker so a hallucinated model value cannot move provenance.
type LLMCandidate struct {
	Summary     string `json:"summary"`
	Evidence    string `json:"evidence"`
	Implication string `json:"implication"`
	Category    string `json:"category"`
	Why         string `json:"why,omitempty"`
}

func NewCandidate(in LLMCandidate, sess db.Session, msgs []db.Message, now time.Time) (Candidate, error) {
	category := normalizeCategory(in.Category)
	problemType, ok := problemTypeForCategory[category]
	if !ok {
		return Candidate{}, fmt.Errorf("unsupported category %q", in.Category)
	}
	originProject, scope := originScope(sess)
	c := Candidate{
		Summary:        strings.TrimSpace(in.Summary),
		Evidence:       strings.TrimSpace(in.Evidence),
		Implication:    strings.TrimSpace(in.Implication),
		Category:       category,
		ProblemType:    problemType,
		OriginSession:  originSession(sess.ID),
		OriginProject:  originProject,
		Scope:          scope,
		SourcePlatform: strings.ToLower(strings.TrimSpace(sess.Agent)),
		SourceRoles:    sourceRoles(msgs),
		Why:            strings.TrimSpace(in.Why),
		CreatedAt:      now.UTC().Truncate(time.Second).Format(time.RFC3339),
	}
	if c.SourcePlatform == "" {
		c.SourcePlatform = "agentsview"
	}
	if err := c.Validate(); err != nil {
		return Candidate{}, err
	}
	c.ID = CandidateID(c)
	return c, nil
}

// originScope maps an extracted session to (origin_project, scope), mirroring
// dotfiles classify_origin_scope so extract-produced candidates carry the same
// user-vs-project tag the hook-capture path does. The dotfiles repo, home, and
// the agent config dirs (~/.claude, ~/.config) are general/user; any other repo
// is "project" named by the session's project. Sessions carry a Project name
// (basename) and an optional Cwd path; the Cwd is used only to detect the
// user buckets. Unknown/empty -> ("", "user").
func originScope(sess db.Session) (string, string) {
	cwd := filepath.ToSlash(strings.TrimSpace(sess.Cwd))
	if cwd != "" {
		if strings.Contains(cwd+"/", "/.claude/") || strings.Contains(cwd+"/", "/.config/") {
			return "", "user"
		}
	}
	switch strings.TrimSpace(sess.Project) {
	case "", ".dotfiles", "dotfiles":
		return "", "user"
	}
	return strings.TrimSpace(sess.Project), "project"
}

func normalizeCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func originSession(value string) string {
	cleaned := strings.Trim(originSessionRE.ReplaceAllString(value, "-"), "-")
	if len(cleaned) > 80 {
		cleaned = cleaned[:80]
	}
	if cleaned == "" {
		return "unknown-session"
	}
	return cleaned
}

func sourceRoles(msgs []db.Message) []string {
	seen := map[string]bool{}
	var roles []string
	for _, msg := range msgs {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		if !seen[role] {
			seen[role] = true
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return []string{"user"}
	}
	slices.Sort(roles)
	return roles
}

func (c Candidate) Validate() error {
	for field, value := range map[string]string{
		"summary":         c.Summary,
		"evidence":        c.Evidence,
		"implication":     c.Implication,
		"category":        c.Category,
		"origin_session":  c.OriginSession,
		"source_platform": c.SourcePlatform,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing required field %s", field)
		}
	}
	if _, ok := problemTypeForCategory[normalizeCategory(c.Category)]; !ok {
		return fmt.Errorf("unsupported category %q", c.Category)
	}
	if normalizeCategory(c.Category) == "decision" && strings.TrimSpace(c.Why) == "" {
		return fmt.Errorf("decision candidate requires why")
	}
	if len(c.SourceRoles) == 0 {
		return fmt.Errorf("missing required field source_roles")
	}
	if len(secrets.Scan(candidateSecretText(c))) > 0 {
		return fmt.Errorf("secret-like content rejected")
	}
	return nil
}

func candidateSecretText(c Candidate) string {
	return strings.Join([]string{c.Summary, c.Evidence, c.Implication, c.Why}, "\n")
}

// CanonicalForHash matches dotfiles memory_capture.py: remove id/created_at,
// JSON sort keys, ensure_ascii=false equivalent, compact separators.
func CanonicalForHash(c Candidate) (string, error) {
	raw := candidateMap(c)
	delete(raw, "id")
	delete(raw, "created_at")
	out, err := marshalJSON(raw, false)
	return string(out), err
}

func CandidateID(c Candidate) string {
	canonical, err := CanonicalForHash(c)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func CanonicalJSON(c Candidate) (string, error) {
	data, err := marshalJSON(candidateMap(c), true)
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func sameStableCandidate(existingText string, c Candidate) bool {
	var existing Candidate
	if err := json.Unmarshal([]byte(existingText), &existing); err != nil {
		return false
	}
	left, err := CanonicalForHash(existing)
	if err != nil {
		return false
	}
	right, err := CanonicalForHash(c)
	return err == nil && left == right
}

func marshalJSON(v any, indent bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func candidateMap(c Candidate) map[string]any {
	return map[string]any{
		"summary":         c.Summary,
		"evidence":        c.Evidence,
		"implication":     c.Implication,
		"category":        c.Category,
		"problem_type":    c.ProblemType,
		"origin_session":  c.OriginSession,
		"origin_project":  c.OriginProject,
		"scope":           c.Scope,
		"source_platform": c.SourcePlatform,
		"source_roles":    c.SourceRoles,
		"why":             c.Why,
		"created_at":      c.CreatedAt,
		"id":              c.ID,
	}
}
