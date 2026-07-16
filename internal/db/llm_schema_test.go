package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sqliteLLMColumnInfo struct {
	Type         string
	NotNull      bool
	DefaultValue string
}

func TestLLMSessionColumnsFreshSchemaDefaults(t *testing.T) {
	d := testDB(t)
	assert.Equal(t, 39, CurrentDataVersion(),
		"LLM schema columns are additive and must not bump dataVersion")

	columns := sqliteLLMTableColumns(t, d, "sessions")
	want := map[string]sqliteLLMColumnInfo{
		"llm_title":          {Type: "TEXT", NotNull: true, DefaultValue: "''"},
		"llm_summary":        {Type: "TEXT", NotNull: true, DefaultValue: "''"},
		"llm_keywords":       {Type: "TEXT", NotNull: true, DefaultValue: "''"},
		"llm_embedding":      {Type: "BLOB", NotNull: false, DefaultValue: ""},
		"llm_embedding_dim":  {Type: "INTEGER", NotNull: true, DefaultValue: "0"},
		"enriched_at":        {Type: "TEXT", NotNull: true, DefaultValue: "''"},
		"enriched_msg_count": {Type: "INTEGER", NotNull: true, DefaultValue: "0"},
		"enrich_model":       {Type: "TEXT", NotNull: true, DefaultValue: "''"},
		"enrich_status":      {Type: "TEXT", NotNull: true, DefaultValue: "''"},
		"enrich_error":       {Type: "TEXT", NotNull: true, DefaultValue: "''"},
	}
	for name, spec := range want {
		got, ok := columns[name]
		require.True(t, ok, "missing sessions.%s", name)
		assert.Equal(t, spec.Type, got.Type, "type for %s", name)
		assert.Equal(t, spec.NotNull, got.NotNull, "notnull for %s", name)
		assert.Equal(t, spec.DefaultValue, got.DefaultValue, "default for %s", name)
	}
}

func TestOpenMigratesLegacyLLMSessionColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-llm.db")
	d, err := Open(path)
	require.NoError(t, err, "Open initial")
	insertSession(t, d, "legacy-llm", "proj", func(s *Session) {
		s.CreatedAt = "2026-01-01T00:00:00Z"
	})
	require.NoError(t, d.Close(), "close initial")

	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err, "open raw sqlite")
	for _, column := range llmSessionColumnNames() {
		_, err = conn.Exec("ALTER TABLE sessions DROP COLUMN " + column)
		require.NoError(t, err, "drop legacy column %s", column)
	}
	require.NoError(t, conn.Close(), "close raw sqlite")

	d, err = Open(path)
	require.NoError(t, err, "Open migrates LLM columns")
	t.Cleanup(func() { _ = d.Close() })
	s, err := d.GetSession(context.Background(), "legacy-llm")
	require.NoError(t, err, "GetSession after migration")
	require.NotNil(t, s)
	assert.Equal(t, "proj", s.Project)
	assert.Equal(t, "", s.LLMTitle)
	assert.Equal(t, "", s.EnrichStatus)
	assert.Zero(t, s.LLMEmbeddingDim)

	for _, column := range llmSessionColumnNames() {
		assert.Contains(t, sqliteLLMTableColumns(t, d, "sessions"), column)
	}
	require.NoError(t, d.Close(), "close before second Open")
	d, err = Open(path)
	require.NoError(t, err, "second Open is idempotent")
	t.Cleanup(func() { _ = d.Close() })
}

func TestLLMFieldsReadAndEmbeddingExcludedFromOrdinaryColumns(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	insertSession(t, d, "llm-read", "proj", func(s *Session) {
		s.CreatedAt = "2026-01-01T00:00:00Z"
	})
	_, err := d.getWriter().Exec(`
		UPDATE sessions SET
			llm_title = 'LLM title',
			llm_summary = 'summary',
			llm_keywords = 'auth,token',
			llm_embedding = X'0000803f',
			llm_embedding_dim = 1,
			enriched_at = '2026-01-01T01:00:00Z',
			enriched_msg_count = 7,
			enrich_model = 'deepseek-chat',
			enrich_status = 'ok',
			enrich_error = ''
		WHERE id = 'llm-read'`)
	require.NoError(t, err, "update llm fields")

	s, err := d.GetSession(ctx, "llm-read")
	require.NoError(t, err, "GetSession")
	require.NotNil(t, s)
	assert.Equal(t, "LLM title", s.LLMTitle)
	assert.Equal(t, "summary", s.LLMSummary)
	assert.Equal(t, "auth,token", s.LLMKeywords)
	assert.Equal(t, 1, s.LLMEmbeddingDim)
	assert.Empty(t, s.LLMEmbedding, "ordinary GetSession must not select embedding")

	full, err := d.GetSessionFull(ctx, "llm-read")
	require.NoError(t, err, "GetSessionFull")
	require.NotNil(t, full)
	assert.Equal(t, "LLM title", full.LLMTitle)
	assert.Empty(t, full.LLMEmbedding, "ordinary GetSessionFull must not select embedding")

	synced, err := d.ListSessionsModifiedBetween(ctx, "", "", nil, nil)
	require.NoError(t, err, "ListSessionsModifiedBetween")
	require.Len(t, synced, 1)
	assert.Equal(t, []byte{0x00, 0x00, 0x80, 0x3f}, synced[0].LLMEmbedding)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal session")
	assert.Contains(t, string(data), `"llm_title":"LLM title"`)
	assert.False(t, sqliteColumnListHasExact(sessionBaseCols, "llm_embedding"))
	assert.False(t, sqliteColumnListHasExact(sessionFullCols, "llm_embedding"))
	assert.True(t, sqliteColumnListHasExact(sessionSyncCols, "llm_embedding"))
}

func llmSessionColumnNames() []string {
	return []string{
		"llm_title",
		"llm_summary",
		"llm_keywords",
		"llm_embedding",
		"llm_embedding_dim",
		"enriched_at",
		"enriched_msg_count",
		"enrich_model",
		"enrich_status",
		"enrich_error",
	}
}

func sqliteLLMTableColumns(t *testing.T, d *DB, table string) map[string]sqliteLLMColumnInfo {
	t.Helper()
	rows, err := d.getReader().Query("PRAGMA table_info(" + table + ")")
	require.NoError(t, err, "pragma_table_info")
	defer rows.Close()
	columns := map[string]sqliteLLMColumnInfo{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk), "scan table_info")
		columns[name] = sqliteLLMColumnInfo{
			Type:         typ,
			NotNull:      notNull == 1,
			DefaultValue: defaultValue.String,
		}
	}
	require.NoError(t, rows.Err(), "table_info rows")
	return columns
}

func sqliteColumnListHasExact(cols, column string) bool {
	for _, part := range strings.Split(cols, ",") {
		if strings.TrimSpace(part) == column {
			return true
		}
	}
	return false
}
