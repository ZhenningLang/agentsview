package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.kenn.io/agentsview/internal/db"
)

func TestStripFTSQuotes(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		prepared string
		want     string
	}{
		{name: "single word", raw: "login", prepared: `"login"`, want: "login"},
		{name: "dash", raw: "error-401", prepared: `"error-401"`, want: "error-401"},
		{name: "colon", raw: "status:500", prepared: `"status:500"`, want: "status:500"},
		{name: "asterisk", raw: "foo*bar", prepared: `"foo*bar"`, want: "foo*bar"},
		{name: "NEAR", raw: "NEAR", prepared: `"NEAR"`, want: "NEAR"},
		{name: "AND phrase", raw: "a AND b", prepared: `"a AND b"`, want: "a AND b"},
		{name: "quoted phrase", raw: `"quoted phrase"`, prepared: `"quoted phrase"`, want: "quoted phrase"},
		{name: "embedded quote", raw: `裸"双引号`, prepared: `"裸""双引号"`, want: `裸"双引号`},
		{name: "trailing backslash", raw: `tail\`, prepared: `"tail\"`, want: `tail\`},
		{name: "CJK", raw: "侯爽", prepared: `"侯爽"`, want: "侯爽"},
		{name: "CJK ASCII", raw: "侯s", prepared: `"侯s"`, want: "侯s"},
		{name: "empty", raw: "", prepared: "", want: ""},
		{name: "like wildcards", raw: `%_\`, prepared: `"%_\"`, want: `%_\`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.prepared, db.PrepareFTSQuery(tt.raw))
			assert.Equal(t, tt.want, db.StripFTSQuotes(tt.prepared))
		})
	}
}

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"100%", `100\%`},
		{"under_score", `under\_score`},
		{`back\slash`, `back\\slash`},
		{`%_\`, `\%\_\\`},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, escapeLike(tt.input),
			"input=%q", tt.input)
	}
}
