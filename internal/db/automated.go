package db

import (
	"slices"
	"strings"
	"sync"
)

// automatedPrefixes are first_message prefixes that identify
// automated (roborev) sessions. Matched case-sensitively.
// Combined with the single-turn gate (user_message_count <= 1)
// to avoid misclassifying interactive sessions.
var automatedPrefixes = []string{
	"You are a code reviewer.",
	"You are a security code reviewer.",
	"You are a design reviewer.",
	"You are a code assistant. Your task is to address",
	"You are a code review insights analyst.",
	"You are reviewing whether an implementation matches",
	"You are a plan document reviewer.",
	"You are a spec document reviewer.",
	"You are summarizing a day of AI agent activity.",
	"You are analyzing AI agent sessions.",
	"## Analysis Request",
	"# Fix Request",
	"You are a helpful assistant working on a software project.",
	"You are combining multiple code review outputs into a single GitHub PR comment.",
	"You are generating a changelog",
	"<user_action>",
	"Review the code changes introduced by commit ",
	"Review the code changes in commit ",
	"Implement the following plan:",
}

// automatedSubstrings are patterns matched anywhere in the
// first message. Used for catch-all markers embedded in
// longer prompts.
var automatedSubstrings = []string{
	"invoked by roborev to perform this review",
	"You are a conversation title generator",
}

// automatedExactMatches are first messages that, after trimming
// surrounding whitespace, exactly equal one of these strings.
// Used for prompts too generic for prefix or substring matching
// (e.g., a single-word warmup ping).
var automatedExactMatches = []string{
	"Warmup",
	"Respond with exactly: OK",
	"Reply with exactly OK.",
}

const userPrefixMaxLen = 1024

type userAutomationConfig struct {
	prefixes     []string
	substrings   []string
	exactMatches []string
}

var (
	userAutomationMu sync.RWMutex
	userAutomation   userAutomationConfig
)

// SetUserAutomationPrefixes replaces the user-pattern slice
// with a normalized copy of the input. Normalization (trim,
// drop empty, length cap, dedupe within input, drop entries
// that equal a built-in prefix) happens here so callers can
// pass the raw list straight from config. Pass nil to clear.
// Idempotent and silent — safe to call from quiet CLI paths
// (statusline, JSON output). Callers that want a startup
// summary should read len(UserAutomationPrefixes()).
func SetUserAutomationPrefixes(prefixes []string) {
	SetUserAutomationMatchers(prefixes, nil, nil)
}

// SetUserAutomationMatchers replaces all user-supplied automated-session
// matcher slices with normalized copies of the input. Pass nil slices to clear.
func SetUserAutomationMatchers(prefixes, substrings, exactMatches []string) {
	cleaned := userAutomationConfig{
		prefixes:     normalizeUserPatterns(prefixes, automatedPrefixes),
		substrings:   normalizeUserPatterns(substrings, automatedSubstrings),
		exactMatches: normalizeUserPatterns(exactMatches, automatedExactMatches),
	}
	userAutomationMu.Lock()
	defer userAutomationMu.Unlock()
	userAutomation = cleaned
}

// UserAutomationPrefixes returns a copy of the current
// user-prefix slice. Used by ClassifierHash and tests; the
// copy prevents callers from mutating singleton state.
func UserAutomationPrefixes() []string {
	userAutomationMu.RLock()
	defer userAutomationMu.RUnlock()
	return append([]string(nil), userAutomation.prefixes...)
}

// UserAutomationSubstrings returns a copy of the current user substring slice.
func UserAutomationSubstrings() []string {
	userAutomationMu.RLock()
	defer userAutomationMu.RUnlock()
	return append([]string(nil), userAutomation.substrings...)
}

// UserAutomationExactMatches returns a copy of the current user exact slice.
func UserAutomationExactMatches() []string {
	userAutomationMu.RLock()
	defer userAutomationMu.RUnlock()
	return append([]string(nil), userAutomation.exactMatches...)
}

// normalizeUserPrefixes applies the validation rules from the
// design spec ("Validation behavior" section). Built-in
// overlap is checked against the package-private
// automatedPrefixes directly.
func normalizeUserPrefixes(in []string) []string {
	return normalizeUserPatterns(in, automatedPrefixes)
}

func normalizeUserPatterns(in []string, builtins []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" || len(s) > userPrefixMaxLen {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		if slices.Contains(builtins, s) {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// IsAutomatedSession returns true if the first message
// matches a known automated review/fix prompt pattern.
func IsAutomatedSession(firstMessage string) bool {
	for _, prefix := range automatedPrefixes {
		if strings.HasPrefix(firstMessage, prefix) {
			return true
		}
	}
	userAutomationMu.RLock()
	configured := userAutomationConfig{
		prefixes:     append([]string(nil), userAutomation.prefixes...),
		substrings:   append([]string(nil), userAutomation.substrings...),
		exactMatches: append([]string(nil), userAutomation.exactMatches...),
	}
	userAutomationMu.RUnlock()
	for _, prefix := range configured.prefixes {
		if strings.HasPrefix(firstMessage, prefix) {
			return true
		}
	}
	for _, sub := range automatedSubstrings {
		if strings.Contains(firstMessage, sub) {
			return true
		}
	}
	for _, sub := range configured.substrings {
		if strings.Contains(firstMessage, sub) {
			return true
		}
	}
	trimmed := strings.TrimSpace(firstMessage)
	return slices.Contains(automatedExactMatches, trimmed) ||
		slices.Contains(configured.exactMatches, trimmed)
}
