package postgres

import (
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pgMemoryScanRows struct {
	values []driver.Value
}

func (r pgMemoryScanRows) Scan(dest ...any) error {
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = r.values[i].(string)
		case *int:
			*d = int(r.values[i].(int64))
		case *int64:
			*d = r.values[i].(int64)
		case *[]byte:
			*d = r.values[i].([]byte)
		}
	}
	return nil
}

type pgMemoryEmbeddingRows struct {
	rows []pgMemoryScanRows
	next int
	cur  pgMemoryScanRows
}

func (r *pgMemoryEmbeddingRows) Next() bool {
	if r.next >= len(r.rows) {
		return false
	}
	r.cur = r.rows[r.next]
	r.next++
	return true
}

func (r *pgMemoryEmbeddingRows) Scan(dest ...any) error { return r.cur.Scan(dest...) }

func (r *pgMemoryEmbeddingRows) Err() error { return nil }

func TestPGMemoryScanIncludesCanonicalMetadata(t *testing.T) {
	row := pgMemoryScanRows{values: canonicalPGMemoryValues()}
	m, err := scanPGMemory(row)
	require.NoError(t, err)

	assert.Equal(t, "canonical/entrypoint.json", m.RelPath)
	assert.Equal(t, "canonical", m.Source)
	assert.Equal(t, `[{"source":"assist-mem"}]`, m.CanonicalCoveredRefs)
	assert.Equal(t, `{"topic":"entrypoint"}`, m.CanonicalProvenance)
	assert.Equal(t, "oss-atlas", m.OriginProject)
	assert.Equal(t, "down", m.FeedbackVote)
	assert.Contains(t, pgMemoryCols, "canonical_covered_refs")
	assert.Contains(t, pgMemoryCols, "canonical_provenance")
}

func TestPGMemoryEmbeddingScanIncludesCanonicalMetadata(t *testing.T) {
	data, err := driver.DefaultParameterConverter.ConvertValue([]byte{0x00, 0x00, 0x80, 0x3f})
	require.NoError(t, err)
	values := append(canonicalPGMemoryValues(), data, int64(1))
	rows := &pgMemoryEmbeddingRows{rows: []pgMemoryScanRows{{values: values}}}

	memories, err := scanMemoryEmbeddings(rows)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	assert.Equal(t, `[{"source":"assist-mem"}]`, memories[0].CanonicalCoveredRefs)
	assert.Equal(t, `{"topic":"entrypoint"}`, memories[0].CanonicalProvenance)
	assert.Equal(t, []float32{1}, memories[0].LLMEmbedding)
	assert.NoError(t, rows.Err())
}

func canonicalPGMemoryValues() []driver.Value {
	return []driver.Value{
		"canonical/entrypoint.json", "canonical", "Entrypoint", "2026-07-02",
		"knowledge", "semantic", "active", "sess", "oss-atlas", "down",
		"needs split", "pending", `[{"source":"assist-mem"}]`,
		`{"topic":"entrypoint"}`, "canonical body", int64(2), int64(100),
		"2026-07-02T00:00:00.000Z",
	}
}
