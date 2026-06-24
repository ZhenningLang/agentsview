//go:build pgtest

package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestStoreGetEnrichmentStatus(t *testing.T) {
	pgURL := testPGURL(t)
	cleanPGSchema(t, pgURL)
	t.Cleanup(func() { cleanPGSchema(t, pgURL) })
	ctx := context.Background()
	pg, err := Open(pgURL, "agentsview", true)
	require.NoError(t, err)
	defer pg.Close()
	syncer := &Sync{pg: pg, schema: "agentsview"}
	require.NoError(t, syncer.EnsureSchema(ctx))
	for _, tc := range []struct {
		id     string
		status string
	}{
		{id: "pending"},
		{id: "ok", status: db.EnrichStatusOK},
		{id: "short", status: db.EnrichStatusSkippedTooShort},
		{id: "empty", status: db.EnrichStatusNoContent},
		{id: "error", status: db.EnrichStatusError},
	} {
		seedPGEnrichmentStatusSession(t, pg, tc.id, tc.status)
	}
	_, err = pg.Exec(`
		INSERT INTO sessions (id, machine, project, agent, first_message, started_at, deleted_at)
		VALUES ('deleted', 'test-machine', 'proj', 'claude', 'deleted first', '2026-06-24T10:00:00Z', '2026-06-24T11:00:00Z')`)
	require.NoError(t, err)
	store, err := NewStore(pgURL, "agentsview", true)
	require.NoError(t, err)
	defer store.Close()

	status, err := store.GetEnrichmentStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, status.Total)
	assert.Equal(t, 1, status.Enriched)
	assert.Equal(t, 1, status.Pending)
	assert.Equal(t, 1, status.SkippedTooShort)
	assert.Equal(t, 1, status.NoContent)
	assert.Equal(t, 1, status.Errors)
	assert.Equal(t, 1, status.ByStatus[""])
	assert.Equal(t, 1, status.ByStatus[db.EnrichStatusOK])
}

func seedPGEnrichmentStatusSession(t *testing.T, pg *sql.DB, id, status string) {
	t.Helper()
	first := id + " first"
	_, err := pg.Exec(`
		INSERT INTO sessions (
			id, machine, project, agent, first_message, started_at,
			message_count, user_message_count, enrich_status,
			llm_title, llm_summary, llm_keywords, enrich_model,
			enriched_at, enriched_msg_count, enrich_error
		) VALUES ($1, 'test-machine', 'proj', 'claude', $2, '2026-06-24T10:00:00Z',
			1, 1, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, first, status, statusValue(status, "title"), statusValue(status, "summary"),
		statusValue(status, "key"), statusValue(status, "model"), statusValue(status, "2026-06-24T10:00:00Z"),
		statusMsgCount(status), statusValue(status, "err"))
	require.NoError(t, err)
}

func statusValue(status, value string) string {
	switch status {
	case "":
		return ""
	case db.EnrichStatusOK:
		return value
	case db.EnrichStatusSkippedTooShort, db.EnrichStatusNoContent, db.EnrichStatusError:
		if value == "err" {
			return value
		}
		return ""
	default:
		return ""
	}
}

func statusMsgCount(status string) int {
	if status == db.EnrichStatusOK {
		return 1
	}
	return 0
}
