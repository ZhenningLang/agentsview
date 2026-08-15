package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPhase15HTTPListSessionsSortAndDescending(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "low", "phase15-http", 2)
	te.seedSession(t, "high", "phase15-http", 9)

	for _, tt := range []struct {
		name string
		url  string
		want []string
	}{
		{"default direction", "/api/v1/sessions?project=phase15-http&order_by=messages", []string{"low", "high"}},
		{"explicit false", "/api/v1/sessions?project=phase15-http&order_by=messages&descending=false", []string{"low", "high"}},
		{"explicit true", "/api/v1/sessions?project=phase15-http&order_by=messages&descending=true", []string{"high", "low"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := te.get(t, tt.url)
			assertStatus(t, w, http.StatusOK)
			resp := decode[sessionListResponse](t, w)
			assert.Equal(t, tt.want, phase15ServerSessionIDs(resp.Sessions))
		})
	}
}

func TestPhase15HTTPSortRejectsInvalidOrderBy(t *testing.T) {
	te := setup(t)
	for _, path := range []string{
		"/api/v1/sessions?order_by=messages:nope",
		"/api/v1/sessions/sidebar-index?order_by=messages:nope",
	} {
		w := te.get(t, path)
		assertStatus(t, w, http.StatusBadRequest)
		assert.Contains(t, w.Body.String(), "invalid order_by")
	}
}

func TestPhase15OpenAPIOrderByDocMatchesSortRegistry(t *testing.T) {
	te := setup(t)
	w := te.get(t, "/api/openapi.json")
	assertStatus(t, w, http.StatusOK)

	type schema struct {
		Description string `json:"description"`
	}
	type parameter struct {
		Name        string `json:"name"`
		In          string `json:"in"`
		Description string `json:"description"`
		Schema      schema `json:"schema"`
	}
	type operation struct {
		Parameters []parameter `json:"parameters"`
	}
	var spec struct {
		Paths map[string]map[string]operation `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &spec))

	op := spec.Paths["/api/v1/sessions"]["get"]
	var doc string
	for _, p := range op.Parameters {
		if p.Name == "order_by" && p.In == "query" {
			doc = p.Description + " " + p.Schema.Description
			break
		}
	}
	require.Contains(t, doc, "Valid keys:")
	doc = strings.TrimSpace(strings.SplitN(doc, "Valid keys:", 2)[1])
	if idx := strings.Index(doc, ". "); idx >= 0 {
		doc = doc[:idx]
	}
	doc = strings.TrimSuffix(doc, ".")
	parts := strings.Split(doc, ",")
	got := make([]string, 0, len(parts))
	for _, part := range parts {
		got = append(got, strings.TrimSpace(part))
	}
	assert.Equal(t, db.SortKeys(), got)
}

func phase15ServerSessionIDs(sessions []db.Session) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids
}
