package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalSystemPrefixSQL evaluates the SystemPrefixSQL clause in SQLite for
// one (content, role) pair and returns whether the clause keeps the row.
// The clause is generated with "?" for both column names, so the exact SQL
// the search queries embed is exercised without depending on a table.
// Placeholder order follows SystemPrefixSQL's own layout: the role column
// once, then the content column once per system prefix.
func evalSystemPrefixSQL(t *testing.T, d *DB, content, role string) bool {
	t.Helper()
	clause := SystemPrefixSQL("?", "?")
	require.Equal(t, len(SystemMsgPrefixes)+1, strings.Count(clause, "?"),
		"placeholder count changed; update the arg order below")
	args := make([]any, 0, len(SystemMsgPrefixes)+1)
	args = append(args, role)
	for range SystemMsgPrefixes {
		args = append(args, content)
	}
	var keep bool
	require.NoError(t,
		d.Reader().QueryRow("SELECT "+clause, args...).Scan(&keep))
	return keep
}

// systemPrefixCase is one (content, role) fixture plus the semantics the
// SQL clause and the Go helper must agree on.
type systemPrefixCase struct {
	name     string
	content  string
	role     string
	isSystem bool
}

func systemPrefixCases() []systemPrefixCase {
	const p = "<command-name>"
	return []systemPrefixCase{
		{"plain user message", "how do I run the tests?", "user", false},
		{"bare prefix", p + " /clear", "user", true},
		{"prefix only", p, "user", true},
		{"leading spaces", "   " + p, "user", true},
		{"leading tab and newline", "\t\n" + p, "user", true},
		{"leading CR LF", "\r\n" + p, "user", true},
		{"leading vertical tab and form feed", "\v\f" + p, "user", true},
		{"leading BOM", "\uFEFF" + p, "user", true},
		{"leading NEL", "\u0085" + p, "user", true},
		{"leading NBSP", "\u00A0" + p, "user", true},
		{"leading OGHAM space", "\u1680" + p, "user", true},
		{"leading EN QUAD", "\u2000" + p, "user", true},
		{"leading HAIR SPACE", "\u200A" + p, "user", true},
		{"leading LINE SEPARATOR", "\u2028" + p, "user", true},
		{"leading PARAGRAPH SEPARATOR", "\u2029" + p, "user", true},
		{"leading NARROW NBSP", "\u202F" + p, "user", true},
		{"leading MEDIUM MATHEMATICAL SPACE", "\u205F" + p, "user", true},
		{"leading IDEOGRAPHIC SPACE", "\u3000" + p, "user", true},
		{"mixed leading whitespace", "\uFEFF \u3000\t" + p, "user", true},
		{"prefix not at start", "see " + p + " above", "user", false},
		{"assistant with prefix", p, "assistant", false},
		{"system role with prefix", p, "system", false},
		{"empty content", "", "user", false},
		{"whitespace only", " \t\uFEFF", "user", false},
		{"shorter than prefix", "<comm", "user", false},
		{"case differs", strings.ToUpper(p), "user", false},
		{"non-ascii content", "怎么跑测试?", "user", false},
		{"session continued prefix", "This session is being continued from", "user", true},
		{"request interrupted prefix", "[Request interrupted by user]", "user", true},
		{"task notification prefix", "<task-notification>x</task-notification>", "user", true},
		{"command message prefix", "<command-message>run</command-message>", "user", true},
		{"local command prefix", "<local-command-stdout>ok", "user", true},
		{"stop hook prefix", "Stop hook feedback:\nblocked", "user", true},
	}
}

// TestPhase21IsSystemPrefixedMatchesSystemPrefixSQL is the parity gate:
// the in-memory filter used by read-only consumers must classify exactly
// what the SQL clause excludes. A divergence here is invisible at runtime
// (no query error, no other failing assertion) and surfaces only as two
// different answers for the same session.
func TestPhase21IsSystemPrefixedMatchesSystemPrefixSQL(t *testing.T) {
	d := testDB(t)
	for _, tc := range systemPrefixCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSystemPrefixed(tc.content, tc.role)
			assert.Equal(t, tc.isSystem, got, "IsSystemPrefixed(%q, %q)",
				tc.content, tc.role)

			keep := evalSystemPrefixSQL(t, d, tc.content, tc.role)
			assert.Equal(t, tc.isSystem, !keep,
				"SystemPrefixSQL keep=%v for (%q, %q)", keep, tc.content, tc.role)

			assert.Equal(t, !keep, got,
				"SQL and Go disagree on (%q, %q): sql excludes=%v go=%v",
				tc.content, tc.role, !keep, got)
		})
	}
}

// TestPhase21SystemPrefixSQLUsesSharedCutset pins the structural
// guarantee: the SQL LTRIM cutset is the same constant the Go helper
// trims with, so the two cannot drift when a character is added.
func TestPhase21SystemPrefixSQLUsesSharedCutset(t *testing.T) {
	clause := SystemPrefixSQL("m.content", "m.role")
	assert.Contains(t, clause, "LTRIM(m.content, '"+systemPrefixTrimCutset+"')")
	assert.NotContains(t, systemPrefixTrimCutset, "'",
		"cutset must not contain a quote; it is embedded in SQL literally")
}
