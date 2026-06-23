package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnrichment(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Enrichment
	}{
		{
			name: "normal json",
			in:   `{"title":"Auth work","summary":"Token cleanup","keywords":["auth","token"]}`,
			want: Enrichment{Title: "Auth work", Summary: "Token cleanup", Keywords: []string{"auth", "token"}},
		},
		{
			name: "fenced json",
			in:   "```json\n{\"title\":\"鉴权\",\"keywords\":[\"登录\",\"token\"]}\n```",
			want: Enrichment{Title: "鉴权", Keywords: []string{"登录", "token"}},
		},
		{
			name: "leading trailing prose",
			in:   "result: {\"title\":\"Search\",\"summary\":\"\",\"keywords\":\"login， token, auth\"} done",
			want: Enrichment{Title: "Search", Keywords: []string{"login", "token", "auth"}},
		},
		{
			name: "drops empty keywords",
			in:   `{"title":"T","keywords":["one"," ","two"]}`,
			want: Enrichment{Title: "T", Keywords: []string{"one", "two"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnrichment(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseEnrichment_RejectsMissingTitle(t *testing.T) {
	_, err := ParseEnrichment(`{"summary":"missing","keywords":["x"]}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title")
}
