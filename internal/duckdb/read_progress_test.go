package duckdb

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPhase20DuckDBSchemaAndReadProgressProjection(t *testing.T) {
	ctx := context.Background()
	local := newLocalDB(t)
	fileHash := "phase20-file-hash"
	localModifiedAt := "2026-08-17T03:33:00.000Z"
	session := syncSession(
		"phase20-duck-projection", "alpha", "phase20 first",
		"2026-08-17T03:32:00.000Z", 2,
	)
	session.UserMessageCount = 2
	session.FileHash = &fileHash
	session.LocalModifiedAt = &localModifiedAt
	revision := "1"
	session.TranscriptRevision = &revision
	_, err := local.WriteSessionBatchAtomic([]db.SessionBatchWrite{{
		Session:         session,
		Messages:        []db.Message{syncMessage(session.ID, 0, "user", "phase20", "2026-08-17T03:32:00.000Z")},
		DataVersion:     1,
		ReplaceMessages: true,
	}})
	require.NoError(t, err)
	nilRevision := syncSession(
		"phase20-duck-default", "alpha", "phase20 default",
		"2026-08-17T03:33:00.000Z", 1,
	)
	nilRevision.TranscriptRevision = nil
	err = local.UpsertSession(nilRevision)
	require.NoError(t, err)

	syncer := newTestSync(t, filepath.Join(t.TempDir(), "phase20.duckdb"), local, SyncOptions{})
	result, err := syncer.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 2, result.SessionsPushed)

	var schemaVersion string
	require.NoError(t, syncer.DB().QueryRowContext(ctx,
		`SELECT value FROM sync_metadata WHERE key = ?`,
		schemaVersionMetadataKey,
	).Scan(&schemaVersion))
	assert.Equal(t, "6", schemaVersion)

	var columnDefault sql.NullString
	require.NoError(t, syncer.DB().QueryRowContext(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_name = 'sessions' AND column_name = 'transcript_revision'`,
	).Scan(&columnDefault))
	require.True(t, columnDefault.Valid)
	assert.Equal(t, "'0'", columnDefault.String)

	store := NewStoreFromDB(syncer.DB())
	detail, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assertPhase20DuckSessionRevision(t, *detail, "1", fileHash, localModifiedAt)
	full, err := store.GetSessionFull(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	assertPhase20DuckSessionRevision(t, *full, "1", fileHash, localModifiedAt)
	page, err := store.ListSessions(ctx, db.SessionFilter{Limit: 10})
	require.NoError(t, err)
	rows := phase20DuckSessionsByID(page.Sessions)
	require.Contains(t, rows, session.ID)
	assertPhase20DuckSessionRevision(t, rows[session.ID], "1", fileHash, localModifiedAt)
	require.Contains(t, rows, nilRevision.ID)
	assertPhase20DuckSessionRevision(t, rows[nilRevision.ID], "0", fileHash, localModifiedAt)
	index, err := store.GetSidebarSessionIndex(ctx, db.SessionFilter{})
	require.NoError(t, err)
	indexRows := phase20DuckSidebarRowsByID(index.Sessions)
	require.Contains(t, indexRows, session.ID)
	require.NotNil(t, indexRows[session.ID].TranscriptRevision)
	assert.Equal(t, "1", *indexRows[session.ID].TranscriptRevision)
}

func TestPhase20DuckDBScopedMarkersDoNotCrossTargetOrFilter(t *testing.T) {
	ctx := context.Background()
	local := newLocalDB(t)
	fixture := seedDuckDBSyncFixture(t, local)
	root := t.TempDir()
	targetA := filepath.Join(root, "target-a.duckdb")
	targetB := filepath.Join(root, "target-b.duckdb")

	alphaA := newTestSync(t, targetA, local, SyncOptions{Projects: []string{"alpha"}})
	first, err := alphaA.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.SessionsPushed)
	second, err := alphaA.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, second.SessionsPushed)
	require.NotEmpty(t, phase20DuckSyncState(t, local, alphaA.lastPushBoundaryStateKey()))

	alphaB := newTestSync(t, targetB, local, SyncOptions{Projects: []string{"alpha"}})
	otherTarget, err := alphaB.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, otherTarget.SessionsPushed)
	require.NotEqual(t, alphaA.lastPushBoundaryStateKey(), alphaB.lastPushBoundaryStateKey())

	betaA := newTestSync(t, targetA, local, SyncOptions{Projects: []string{"beta"}})
	otherFilter, err := betaA.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, otherFilter.SessionsPushed)
	require.NotEqual(t, alphaA.lastPushBoundaryStateKey(), betaA.lastPushBoundaryStateKey())
	assertDuckDBCountWhere(t, betaA.DB(), "sessions", "id = ?", fixture.betaID, 1)
}

func TestPhase20DuckDBScopedMarkerUsesCanonicalPathAndNoRawPath(t *testing.T) {
	ctx := context.Background()
	local := newLocalDB(t)
	seedDuckDBSyncFixture(t, local)
	root := t.TempDir()
	target := filepath.Join(root, "canonical.duckdb")
	syncer := newTestSync(t, target, local, SyncOptions{Projects: []string{"alpha"}})
	first, err := syncer.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.SessionsPushed)
	require.NoError(t, syncer.Close())

	aliasDir := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(root, aliasDir))
	aliasTarget := filepath.Join(aliasDir, "canonical.duckdb")
	aliased, err := New(aliasTarget, local, "test-machine", SyncOptions{Projects: []string{"alpha"}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, aliased.Close()) })
	require.Equal(t, syncer.lastPushBoundaryStateKey(), aliased.lastPushBoundaryStateKey())
	second, err := aliased.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, second.SessionsPushed)

	keys := []string{aliased.lastPushStateKey(), aliased.lastPushBoundaryStateKey()}
	for _, key := range keys {
		assert.NotContains(t, key, root)
		assert.NotContains(t, key, aliasTarget)
	}
	assert.NotEmpty(t, phase20DuckSyncState(t, local, aliased.lastPushBoundaryStateKey()))
}

func TestPhase20DuckDBFailedPushDoesNotMarkScopedMarkerComplete(t *testing.T) {
	ctx := context.Background()
	local := newLocalDB(t)
	seedDuckDBSyncFixture(t, local)
	syncer := newTestSync(t,
		filepath.Join(t.TempDir(), "failure.duckdb"),
		local, SyncOptions{Projects: []string{"alpha"}},
	)
	require.NoError(t, syncer.EnsureSchema(ctx))
	_, err := syncer.DB().ExecContext(ctx, `DROP TABLE messages`)
	require.NoError(t, err)

	_, err = syncer.Push(ctx, false, nil)
	require.Error(t, err)
	assert.Empty(t, phase20DuckSyncState(t, local, syncer.lastPushStateKey()))
	assert.Empty(t, phase20DuckSyncState(t, local, syncer.lastPushBoundaryStateKey()))

	repaired := newTestSync(t,
		filepath.Join(t.TempDir(), "failure.duckdb"),
		local, SyncOptions{Projects: []string{"alpha"}},
	)
	result, err := repaired.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.SessionsPushed)
}

func assertPhase20DuckSessionRevision(
	t *testing.T, s db.Session, want, fileHash, localModifiedAt string,
) {
	t.Helper()
	require.NotNil(t, s.TranscriptRevision)
	assert.Equal(t, want, *s.TranscriptRevision)
	assert.NotEqual(t, fileHash, *s.TranscriptRevision)
	assert.NotEqual(t, localModifiedAt, *s.TranscriptRevision)
}

func phase20DuckSessionsByID(sessions []db.Session) map[string]db.Session {
	rows := make(map[string]db.Session, len(sessions))
	for _, session := range sessions {
		rows[session.ID] = session
	}
	return rows
}

func phase20DuckSidebarRowsByID(
	sessions []db.SidebarSessionIndexRow,
) map[string]db.SidebarSessionIndexRow {
	rows := make(map[string]db.SidebarSessionIndexRow, len(sessions))
	for _, session := range sessions {
		rows[session.ID] = session
	}
	return rows
}

func phase20DuckSyncState(t *testing.T, local *db.DB, key string) string {
	t.Helper()
	value, err := local.GetSyncState(key)
	require.NoError(t, err)
	return value
}

func TestPhase20SQLiteDataVersionUnchangedForDuckDBReadProgress(t *testing.T) {
	assert.Equal(t, 40, db.CurrentDataVersion())
}
