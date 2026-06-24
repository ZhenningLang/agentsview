package enrich

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.kenn.io/agentsview/internal/db"
)

func TestBuildPromptContainsJSONContractAndNoSecrets(t *testing.T) {
	system, user := buildPrompt(db.EnrichCandidate{
		ID: "s1", Project: "proj", Agent: "codex", FirstMessage: "start",
	}, []string{"sample one"})
	assert.Contains(t, system, "JSON object")
	assert.Contains(t, system, "title")
	assert.Contains(t, system, "summary")
	assert.Contains(t, system, "keywords")
	assert.Contains(t, user, "Project: proj")
	assert.Contains(t, user, "sample one")
	assert.NotContains(t, user, "api_key")
	assert.NotContains(t, user, "Authorization")
}
