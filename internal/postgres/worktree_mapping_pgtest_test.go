//go:build pgtest

package postgres

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPushMirrorsResolvedWorktreeProject(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_push_worktree_mapping_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err)
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err)
	require.NoError(t, EnsureSchema(ctx, pg, schema))
	t.Cleanup(func() {
		_, _ = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	})

	localDB, err := db.Open(filepath.Join(t.TempDir(), "local.db"))
	require.NoError(t, err)
	defer localDB.Close()
	syncer := &Sync{pg: pg, local: localDB, machine: "test-machine", schema: schema, schemaDone: true}

	root := t.TempDir()
	const sessionID = "pg-worktree-layout"
	_, err = localDB.CreateWorktreeProjectMapping(ctx, db.WorktreeProjectMapping{
		Machine:    "test-machine",
		PathPrefix: root,
		Layout:     db.WorktreeMappingLayoutRepoDotWorktrees,
		Enabled:    true,
	})
	require.NoError(t, err)
	require.NoError(t, localDB.UpsertSession(db.Session{
		ID: sessionID, Project: "leaf", Machine: "test-machine", Agent: "claude",
		Cwd: filepath.Join(root, "service.worktrees", "feature"), MessageCount: 1,
		CreatedAt: "2026-01-12T00:00:00.000Z",
	}))
	require.NoError(t, localDB.InsertMessages([]db.Message{{
		SessionID: sessionID, Ordinal: 0, Role: "user", Content: "pg worktree",
		ContentLength: len("pg worktree"), Timestamp: "2026-01-12T00:00:00.000Z",
	}}))
	result, err := localDB.ApplyWorktreeProjectMappings(ctx, "test-machine")
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedSessions)

	pushResult, err := syncer.Push(ctx, true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, pushResult.SessionsPushed)

	store := &Store{pg: pg}
	sess, err := store.GetSession(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, "service", sess.Project)

	var mappingTables int
	require.NoError(t, pg.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = 'worktree_project_mappings'`, schema,
	).Scan(&mappingTables))
	assert.Equal(t, 0, mappingTables)
}
