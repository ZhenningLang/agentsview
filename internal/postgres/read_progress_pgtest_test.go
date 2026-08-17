//go:build pgtest

package postgres

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

func TestPhase20PostgresTranscriptRevisionSchemaPushRead(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_phase20_read_progress_projection"
	cleanPhase20PGSchema(t, pgURL, schema)
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schema) })

	ctx := context.Background()
	local := testDB(t)
	fileHash := "phase20-file-hash"
	localModifiedAt := "2026-08-17T03:33:00.000Z"
	session := phase20PGSession("phase20-pg-projection", "alpha", 1)
	session.FileHash = &fileHash
	session.LocalModifiedAt = &localModifiedAt
	revision := "7"
	session.TranscriptRevision = &revision
	seedPhase20PGLocalSession(t, local, session, true)
	setPhase20PGTranscriptRevision(t, local, session.ID, revision)

	nilRevision := phase20PGSession("phase20-pg-default", "alpha", 1)
	nilRevision.TranscriptRevision = nil
	seedPhase20PGLocalSession(t, local, nilRevision, false)

	syncer := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{})
	result, err := syncer.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 2, result.SessionsPushed)

	var columnDefault string
	require.NoError(t, syncer.DB().QueryRowContext(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = 'sessions'
		  AND column_name = 'transcript_revision'`,
		schema,
	).Scan(&columnDefault))
	assert.Equal(t, "'0'::text", columnDefault)
	require.NoError(t, CheckSchemaCompat(ctx, syncer.DB()))

	store := &Store{pg: syncer.DB()}
	detail, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assertPhase20PGSessionRevision(t, *detail, "7", fileHash, localModifiedAt)

	full, err := store.GetSessionFull(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	assertPhase20PGSessionRevision(t, *full, "7", fileHash, localModifiedAt)

	page, err := store.ListSessions(ctx, db.SessionFilter{Limit: 10})
	require.NoError(t, err)
	rows := phase20PGSessionsByID(page.Sessions)
	require.Contains(t, rows, session.ID)
	assertPhase20PGSessionRevision(t, rows[session.ID], "7", fileHash, localModifiedAt)
	require.Contains(t, rows, nilRevision.ID)
	assertPhase20PGSessionRevision(t, rows[nilRevision.ID], "0", fileHash, localModifiedAt)

	index, err := store.GetSidebarSessionIndex(ctx, db.SessionFilter{})
	require.NoError(t, err)
	indexRows := phase20PGSidebarRowsByID(index.Sessions)
	require.Contains(t, indexRows, session.ID)
	require.NotNil(t, indexRows[session.ID].TranscriptRevision)
	assert.Equal(t, "7", *indexRows[session.ID].TranscriptRevision)
}

// TestPhase20PostgresTranscriptRevisionMigratesSchemaMissingTheColumn covers
// the shape every existing PostgreSQL target is in on the first push after
// this phase ships: the schema and its rows already exist, and the column does
// not. The migration has to add it in place. Recreating the table would drop
// the mirror, and failing would leave the target unpushable.
//
// The fixture drops the column from a freshly created schema rather than
// hand-writing an old DDL, so it stays honest as the rest of the schema moves.
func TestPhase20PostgresTranscriptRevisionMigratesSchemaMissingTheColumn(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_phase20_read_progress_migration"
	cleanPhase20PGSchema(t, pgURL, schema)
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schema) })

	ctx := context.Background()
	local := testDB(t)
	session := phase20PGSession("phase20-pg-migrated", "alpha", 1)
	seedPhase20PGLocalSession(t, local, session, true)

	syncer := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{})
	first, err := syncer.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.SessionsPushed)

	// Rewind the target to what a release before this phase left behind.
	_, err = syncer.DB().ExecContext(ctx,
		`ALTER TABLE sessions DROP COLUMN transcript_revision`)
	require.NoError(t, err)
	require.Error(t, CheckSchemaCompat(ctx, syncer.DB()),
		"the compat probe has to notice the missing column, or this fixture proves nothing")

	// A fresh Sync, the way the next `pg push` invocation starts: the first
	// syncer caches that it already ensured the schema.
	migrated := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{})
	require.NoError(t, migrated.EnsureSchema(ctx))
	require.NoError(t, CheckSchemaCompat(ctx, migrated.DB()))

	var columnDefault string
	require.NoError(t, migrated.DB().QueryRowContext(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = 'sessions'
		  AND column_name = 'transcript_revision'`,
		schema,
	).Scan(&columnDefault))
	assert.Equal(t, "'0'::text", columnDefault)

	// The row that predates the column survived and reads the default.
	store := &Store{pg: migrated.DB()}
	existing, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.NotNil(t, existing.TranscriptRevision)
	assert.Equal(t, "0", *existing.TranscriptRevision)

	// And a later local bump still reaches the migrated column.
	setPhase20PGTranscriptRevision(t, local, session.ID, "9")
	second, err := migrated.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, second.SessionsPushed)

	pushed, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, pushed)
	require.NotNil(t, pushed.TranscriptRevision)
	assert.Equal(t, "9", *pushed.TranscriptRevision)

	index, err := store.GetSidebarSessionIndex(ctx, db.SessionFilter{})
	require.NoError(t, err)
	indexRows := phase20PGSidebarRowsByID(index.Sessions)
	require.Contains(t, indexRows, session.ID)
	require.NotNil(t, indexRows[session.ID].TranscriptRevision)
	assert.Equal(t, "9", *indexRows[session.ID].TranscriptRevision)
}

func TestPhase20PostgresTranscriptRevisionScopedMarkersDoNotCrossTargetOrFilter(t *testing.T) {
	pgURL := testPGURL(t)
	const schemaA = "agentsview_phase20_read_progress_scope_a"
	const schemaB = "agentsview_phase20_read_progress_scope_b"
	cleanPhase20PGSchema(t, pgURL, schemaA)
	cleanPhase20PGSchema(t, pgURL, schemaB)
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schemaA) })
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schemaB) })

	ctx := context.Background()
	local := testDB(t)
	seedPhase20PGLocalSession(t, local, phase20PGSession("phase20-alpha", "alpha", 1), true)
	seedPhase20PGLocalSession(t, local, phase20PGSession("phase20-beta", "beta", 1), true)

	alphaA := newPhase20PGSync(t, pgURL, schemaA, local, SyncOptions{Projects: []string{"alpha"}})
	first, err := alphaA.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.SessionsPushed)
	second, err := alphaA.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, second.SessionsPushed)
	require.NotEmpty(t, phase20PGSyncState(t, local, alphaA.lastPushBoundaryStateKey()))

	alphaB := newPhase20PGSync(t, pgURL, schemaB, local, SyncOptions{Projects: []string{"alpha"}})
	otherTarget, err := alphaB.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, otherTarget.SessionsPushed)
	require.NotEqual(t, alphaA.lastPushBoundaryStateKey(), alphaB.lastPushBoundaryStateKey())

	betaA := newPhase20PGSync(t, pgURL, schemaA, local, SyncOptions{Projects: []string{"beta"}})
	otherFilter, err := betaA.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, otherFilter.SessionsPushed)
	require.NotEqual(t, alphaA.lastPushBoundaryStateKey(), betaA.lastPushBoundaryStateKey())
}

func TestPhase20PostgresTranscriptRevisionScopedMarkerUsesCanonicalLocalPathAndNoRawPath(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_phase20_read_progress_canonical"
	cleanPhase20PGSchema(t, pgURL, schema)
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schema) })

	ctx := context.Background()
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(realDir, 0o755))
	aliasDir := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(realDir, aliasDir))

	realLocal, err := db.Open(filepath.Join(realDir, "local.db"))
	require.NoError(t, err)
	t.Cleanup(func() { realLocal.Close() })
	seedPhase20PGLocalSession(t, realLocal, phase20PGSession("phase20-canonical", "alpha", 1), true)

	realSync := newPhase20PGSync(t, pgURL, schema, realLocal, SyncOptions{Projects: []string{"alpha"}})
	first, err := realSync.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.SessionsPushed)
	require.NoError(t, realSync.Close())
	require.NoError(t, realLocal.Close())

	aliasLocal, err := db.Open(filepath.Join(aliasDir, "local.db"))
	require.NoError(t, err)
	t.Cleanup(func() { aliasLocal.Close() })
	aliased := newPhase20PGSync(t, pgURL, schema, aliasLocal, SyncOptions{Projects: []string{"alpha"}})
	require.Equal(t, realSync.lastPushBoundaryStateKey(), aliased.lastPushBoundaryStateKey())
	second, err := aliased.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, second.SessionsPushed)

	for _, key := range []string{aliased.lastPushStateKey(), aliased.lastPushBoundaryStateKey()} {
		assert.NotContains(t, key, root)
		assert.NotContains(t, key, aliasDir)
		assert.NotContains(t, key, pgURL)
	}
	assert.NotEmpty(t, phase20PGSyncState(t, aliasLocal, aliased.lastPushBoundaryStateKey()))
}

func TestPhase20PostgresTranscriptRevisionFailedPushDoesNotMarkScopedMarkerComplete(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_phase20_read_progress_failure"
	cleanPhase20PGSchema(t, pgURL, schema)
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schema) })

	ctx := context.Background()
	local := testDB(t)
	seedPhase20PGLocalSession(t, local, phase20PGSession("phase20-failure", "alpha", 1), true)
	syncer := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{Projects: []string{"alpha"}})
	require.NoError(t, syncer.EnsureSchema(ctx))
	_, err := syncer.DB().ExecContext(ctx, `DROP TABLE messages`)
	require.NoError(t, err)

	result, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	require.NotZero(t, result.Errors)
	assert.Empty(t, phase20PGSyncState(t, local, syncer.lastPushStateKey()))
	assert.Empty(t, phase20PGSyncState(t, local, syncer.lastPushBoundaryStateKey()))

	repaired := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{Projects: []string{"alpha"}})
	push, err := repaired.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, push.SessionsPushed)
}

func TestPhase20PostgresTranscriptRevisionEmptyCandidateCompletesScopedMarker(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_phase20_read_progress_empty"
	cleanPhase20PGSchema(t, pgURL, schema)
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schema) })

	ctx := context.Background()
	local := testDB(t)
	syncer := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{Projects: []string{"alpha"}})
	push, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Zero(t, push.SessionsPushed)
	assert.NotEmpty(t, phase20PGSyncState(t, local, syncer.lastPushBoundaryStateKey()))
}

// TestPhase20PostgresLegacyGlobalWatermarkDoesNotSeedScopedState pins the
// upgrade path, and pins it as a deliberate cost rather than an oversight.
//
// Sync state used to live under two global keys. It is now keyed per
// (canonical local path, target, filter scope), so an existing target's first
// push after upgrading finds no scoped watermark and re-pushes everything.
// Seeding the scoped key from the leftover global one would avoid that, and
// must not be done: the global key does not record *which* target it belonged
// to, so seeding would hand one target's watermark to every scope and a second
// target would silently skip its own backfill — exactly what
// TestPhase20PostgresTranscriptRevisionScopedMarkersDoNotCrossTargetOrFilter
// forbids. One idempotent full push is the cheaper mistake.
func TestPhase20PostgresLegacyGlobalWatermarkDoesNotSeedScopedState(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_phase20_legacy_watermark"
	cleanPhase20PGSchema(t, pgURL, schema)
	t.Cleanup(func() { cleanPhase20PGSchema(t, pgURL, schema) })

	ctx := context.Background()
	local := testDB(t)
	for _, id := range []string{"phase20-legacy-a", "phase20-legacy-b"} {
		seedPhase20PGLocalSession(t, local, phase20PGSession(id, "alpha", 1), true)
	}

	syncer := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{})
	first, err := syncer.Push(ctx, true, nil)
	require.NoError(t, err)
	require.Equal(t, 2, first.SessionsPushed)

	// An unfiltered push writes both the scoped and the legacy global
	// watermark, so the legacy key is what a pre-scope release left on disk.
	legacyWatermark := phase20PGSyncState(t, local, lastPushStateKey)
	require.NotEmpty(t, legacyWatermark)
	require.NotEmpty(t, phase20PGSyncState(t, local, syncer.lastPushStateKey()))

	// Rewind to the on-disk state of a user who upgraded: the global keys are
	// populated, the scoped ones do not exist yet.
	require.NoError(t, local.SetSyncState(syncer.lastPushStateKey(), ""))
	require.NoError(t, local.SetSyncState(syncer.lastPushBoundaryStateKey(), ""))

	upgraded := newPhase20PGSync(t, pgURL, schema, local, SyncOptions{})
	require.Equal(t, syncer.lastPushStateKey(), upgraded.lastPushStateKey(),
		"the scope is stable across processes for the same target and filter")

	second, err := upgraded.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, second.SessionsPushed,
		"a leftover global watermark must not be adopted as the scoped one")

	// The full re-push is idempotent: same rows, and the scoped state is now
	// established so the next push is a no-op.
	third, err := upgraded.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Zero(t, third.SessionsPushed)

	var sessionCount int
	require.NoError(t, upgraded.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM sessions`).Scan(&sessionCount))
	assert.Equal(t, 2, sessionCount, "the full re-push upserts rather than duplicating")
}

func newPhase20PGSync(t *testing.T, pgURL, schema string, local *db.DB, opts SyncOptions) *Sync {
	t.Helper()
	syncer, err := New(pgURL, schema, local, "phase20-machine", true, opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, syncer.Close()) })
	return syncer
}

func phase20PGSession(id, project string, messageCount int) db.Session {
	started := "2026-08-17T03:32:00.000Z"
	return db.Session{
		ID: id, Project: project, Machine: "local", Agent: "claude",
		StartedAt: &started, MessageCount: messageCount, UserMessageCount: messageCount,
	}
}

func seedPhase20PGLocalSession(t *testing.T, local *db.DB, session db.Session, withMessage bool) {
	t.Helper()
	require.NoError(t, local.UpsertSession(session))
	if !withMessage {
		return
	}
	require.NoError(t, local.InsertMessages([]db.Message{{
		SessionID: session.ID,
		Ordinal:   0,
		Role:      "user",
		Content:   "phase20 " + session.ID,
	}}))
}

func setPhase20PGTranscriptRevision(t *testing.T, local *db.DB, sessionID, revision string) {
	t.Helper()
	require.NoError(t, local.Update(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`UPDATE sessions SET transcript_revision = ? WHERE id = ?`,
			revision, sessionID,
		)
		return err
	}))
}

func assertPhase20PGSessionRevision(t *testing.T, s db.Session, want, fileHash, localModifiedAt string) {
	t.Helper()
	require.NotNil(t, s.TranscriptRevision)
	assert.Equal(t, want, *s.TranscriptRevision)
	assert.NotEqual(t, fileHash, *s.TranscriptRevision)
	assert.NotEqual(t, localModifiedAt, *s.TranscriptRevision)
}

func phase20PGSessionsByID(sessions []db.Session) map[string]db.Session {
	rows := make(map[string]db.Session, len(sessions))
	for _, session := range sessions {
		rows[session.ID] = session
	}
	return rows
}

func phase20PGSidebarRowsByID(sessions []db.SidebarSessionIndexRow) map[string]db.SidebarSessionIndexRow {
	rows := make(map[string]db.SidebarSessionIndexRow, len(sessions))
	for _, session := range sessions {
		rows[session.ID] = session
	}
	return rows
}

func phase20PGSyncState(t *testing.T, local *db.DB, key string) string {
	t.Helper()
	value, err := local.GetSyncState(key)
	require.NoError(t, err)
	return value
}

func cleanPhase20PGSchema(t *testing.T, pgURL, schema string) {
	t.Helper()
	pg, err := sql.Open("pgx", pgURL)
	require.NoError(t, err)
	defer pg.Close()
	quoted, err := quoteIdentifier(schema)
	require.NoError(t, err)
	_, _ = pg.Exec("DROP SCHEMA IF EXISTS " + quoted + " CASCADE")
}
