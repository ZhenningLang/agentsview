package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
)

func TestCosineRejectsInvalidVectors(t *testing.T) {
	_, ok := Cosine([]float32{1}, []float32{1, 2})
	assert.False(t, ok)
	_, ok = Cosine([]float32{0, 0}, []float32{1, 2})
	assert.False(t, ok)
}

func TestRankOrdersSemanticResultsByCosine(t *testing.T) {
	results := Rank([]float32{1, 0}, []db.SessionEmbedding{
		{SessionID: "orthogonal", Project: "p", Agent: "codex", Name: "Orthogonal", Vector: []float32{0, 1}},
		{SessionID: "best", Project: "p", Agent: "codex", Name: "Best", Vector: []float32{1, 0}},
		{SessionID: "second", Project: "p", Agent: "codex", Name: "Second", Vector: []float32{0.5, 0.5}},
	}, 2)
	require.Len(t, results, 2)
	assert.Equal(t, "best", results[0].SessionID)
	assert.Equal(t, "second", results[1].SessionID)
	assert.Equal(t, -1, results[0].Ordinal)
	assert.Equal(t, "Semantic match", results[0].Snippet)
}
