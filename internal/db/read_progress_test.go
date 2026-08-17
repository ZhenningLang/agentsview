package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type phase20ColumnInfo struct {
	Name       string
	Type       string
	NotNull    int
	DefaultVal sql.NullString
}

func TestPhase20SQLiteSchemaFreshTranscriptRevisionDefault(t *testing.T) {
	d := testDB(t)

	info := requirePhase20TranscriptRevisionColumn(t, d.getReader())
	assert.Equal(t, "TEXT", info.Type)
	assert.Equal(t, 1, info.NotNull)
	require.True(t, info.DefaultVal.Valid, "transcript_revision default")
	assert.Equal(t, "'0'", info.DefaultVal.String)

	insertSession(t, d, "fresh-session", "project-a")
	var revision string
	require.NoError(t, d.getReader().QueryRow(
		`SELECT transcript_revision FROM sessions WHERE id = ?`,
		"fresh-session",
	).Scan(&revision))
	assert.Equal(t, "0", revision)
}

func TestPhase20SQLiteMigrationAddsTranscriptRevisionWithoutResync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-current.db")
	createPhase20CurrentArchiveWithoutTranscriptRevision(t, path)

	conn, err := sql.Open("sqlite3", makeDSN(path, true))
	require.NoError(t, err)
	assert.False(t, sqliteColumnExists(t, conn, "sessions", "transcript_revision"))
	requireRawPhase20Rows(t, conn)
	require.NoError(t, conn.Close())

	d, err := Open(path)
	require.NoError(t, err)
	defer d.Close()

	assert.False(t, d.NeedsResync())
	assert.Equal(t, 40, CurrentDataVersion())
	assertPhase20LegacyRepairProbeExcludesTranscriptRevision(t)

	info := requirePhase20TranscriptRevisionColumn(t, d.getReader())
	assert.Equal(t, "TEXT", info.Type)
	assert.Equal(t, 1, info.NotNull)
	require.True(t, info.DefaultVal.Valid, "transcript_revision default")
	assert.Equal(t, "'0'", info.DefaultVal.String)

	requireRawPhase20Rows(t, d.getReader())
	var revision string
	require.NoError(t, d.getReader().QueryRow(
		`SELECT transcript_revision FROM sessions WHERE id = ?`,
		"phase20-session",
	).Scan(&revision))
	assert.Equal(t, "0", revision)
}

func TestPhase20SQLiteSchemaProjectsTranscriptRevisionReads(t *testing.T) {
	ctx := context.Background()
	d := testDB(t)
	insertSession(t, d, "phase20-parent", "project-a", func(s *Session) {
		s.StartedAt = phase20Ptr("2026-08-18T00:00:00Z")
		s.EndedAt = phase20Ptr("2026-08-18T00:01:00Z")
	})
	insertSession(t, d, "phase20-child", "project-a", func(s *Session) {
		s.ParentSessionID = phase20Ptr("phase20-parent")
		s.RelationshipType = "subagent"
		s.StartedAt = phase20Ptr("2026-08-18T00:02:00Z")
	})
	_, err := d.getWriter().Exec(
		`UPDATE sessions
		 SET transcript_revision = '7',
		     local_modified_at = '2026-08-18T00:03:00.000Z'
		 WHERE id = 'phase20-parent'`,
	)
	require.NoError(t, err)
	_, err = d.getWriter().Exec(
		`UPDATE sessions SET transcript_revision = '3'
		 WHERE id = 'phase20-child'`,
	)
	require.NoError(t, err)

	got, err := d.GetSession(ctx, "phase20-parent")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.TranscriptRevision)
	assert.Equal(t, "7", *got.TranscriptRevision)

	full, err := d.GetSessionFull(ctx, "phase20-parent")
	require.NoError(t, err)
	require.NotNil(t, full)
	require.NotNil(t, full.TranscriptRevision)
	assert.Equal(t, "7", *full.TranscriptRevision)

	page, err := d.ListSessions(ctx, SessionFilter{Limit: 10})
	require.NoError(t, err)
	pageRows := phase20SessionsByID(page.Sessions)
	require.Contains(t, pageRows, "phase20-parent")
	require.NotNil(t, pageRows["phase20-parent"].TranscriptRevision)
	assert.Equal(t, "7", *pageRows["phase20-parent"].TranscriptRevision)

	children, err := d.GetChildSessions(ctx, "phase20-parent")
	require.NoError(t, err)
	require.Len(t, children, 1)
	require.NotNil(t, children[0].TranscriptRevision)
	assert.Equal(t, "3", *children[0].TranscriptRevision)

	modified, err := d.ListSessionsModifiedBetween(
		ctx,
		"2026-08-18T00:02:00Z",
		"2026-08-18T00:04:00Z",
		nil,
		nil,
	)
	require.NoError(t, err)
	modifiedRows := phase20SessionsByID(modified)
	require.Contains(t, modifiedRows, "phase20-parent")
	require.NotNil(t, modifiedRows["phase20-parent"].TranscriptRevision)
	assert.Equal(t, "7", *modifiedRows["phase20-parent"].TranscriptRevision)

	index, err := d.GetSidebarSessionIndex(ctx, SessionFilter{})
	require.NoError(t, err)
	indexRows := phase20SidebarRowsByID(index.Sessions)
	require.Contains(t, indexRows, "phase20-parent")
	require.NotNil(t, indexRows["phase20-parent"].TranscriptRevision)
	assert.Equal(t, "7", *indexRows["phase20-parent"].TranscriptRevision)
}

func createPhase20CurrentArchiveWithoutTranscriptRevision(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)

	_, err = conn.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			project TEXT NOT NULL,
			machine TEXT NOT NULL DEFAULT 'local',
			agent TEXT NOT NULL DEFAULT 'claude',
			first_message TEXT,
			display_name TEXT,
			session_name TEXT,
			started_at TEXT,
			ended_at TEXT,
			message_count INTEGER NOT NULL DEFAULT 0,
			user_message_count INTEGER NOT NULL DEFAULT 0,
			file_path TEXT,
			file_size INTEGER,
			file_mtime INTEGER,
			file_inode INTEGER,
			file_device INTEGER,
			file_hash TEXT,
			local_modified_at TEXT,
			parent_session_id TEXT,
			relationship_type TEXT NOT NULL DEFAULT '',
			total_output_tokens INTEGER NOT NULL DEFAULT 0,
			peak_context_tokens INTEGER NOT NULL DEFAULT 0,
			has_total_output_tokens INTEGER NOT NULL DEFAULT 0,
			has_peak_context_tokens INTEGER NOT NULL DEFAULT 0,
			is_automated INTEGER NOT NULL DEFAULT 0,
			tool_failure_signal_count INTEGER NOT NULL DEFAULT 0,
			tool_retry_count INTEGER NOT NULL DEFAULT 0,
			edit_churn_count INTEGER NOT NULL DEFAULT 0,
			consecutive_failure_max INTEGER NOT NULL DEFAULT 0,
			outcome TEXT NOT NULL DEFAULT 'unknown',
			outcome_confidence TEXT NOT NULL DEFAULT 'low',
			ended_with_role TEXT NOT NULL DEFAULT '',
			final_failure_streak INTEGER NOT NULL DEFAULT 0,
			signals_pending_since TEXT,
			compaction_count INTEGER NOT NULL DEFAULT 0,
			mid_task_compaction_count INTEGER NOT NULL DEFAULT 0,
			context_pressure_max REAL,
			health_score INTEGER,
			health_grade TEXT,
			has_tool_calls INTEGER NOT NULL DEFAULT 0,
			has_context_data INTEGER NOT NULL DEFAULT 0,
			data_version INTEGER NOT NULL DEFAULT 0,
			cwd TEXT NOT NULL DEFAULT '',
			git_branch TEXT NOT NULL DEFAULT '',
			source_session_id TEXT NOT NULL DEFAULT '',
			source_version TEXT NOT NULL DEFAULT '',
			parser_malformed_lines INTEGER NOT NULL DEFAULT 0,
			is_truncated INTEGER NOT NULL DEFAULT 0,
			deleted_at TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			termination_status TEXT,
			secret_leak_count INTEGER NOT NULL DEFAULT 0,
			secrets_rules_version TEXT NOT NULL DEFAULT '',
			llm_title TEXT NOT NULL DEFAULT '',
			llm_summary TEXT NOT NULL DEFAULT '',
			llm_keywords TEXT NOT NULL DEFAULT '',
			llm_embedding BLOB,
			llm_embedding_dim INTEGER NOT NULL DEFAULT 0,
			enriched_at TEXT NOT NULL DEFAULT '',
			enriched_msg_count INTEGER NOT NULL DEFAULT 0,
			enrich_model TEXT NOT NULL DEFAULT '',
			enrich_status TEXT NOT NULL DEFAULT '',
			enrich_error TEXT NOT NULL DEFAULT ''
		);
	`)
	require.NoError(t, err)
	_, err = conn.Exec(schemaSQL)
	require.NoError(t, err)
	_, err = conn.Exec(`
		INSERT INTO sessions (
			id, project, machine, agent, first_message, message_count
		) VALUES (
			'phase20-session', 'project-a', 'local', 'claude',
			'archived prompt', 1
		);
		INSERT INTO messages (
			session_id, ordinal, role, content, content_length
		) VALUES (
			'phase20-session', 0, 'assistant', 'archived answer', 15
		);
	`)
	require.NoError(t, err)
	_, err = conn.Exec(fmt.Sprintf("PRAGMA user_version = %d", dataVersion))
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func requirePhase20TranscriptRevisionColumn(
	t *testing.T, conn phase20SQLiteQueryer,
) phase20ColumnInfo {
	t.Helper()
	rows, err := conn.Query("PRAGMA table_info('sessions')")
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var cid int
		var info phase20ColumnInfo
		var pk int
		require.NoError(t, rows.Scan(
			&cid, &info.Name, &info.Type, &info.NotNull,
			&info.DefaultVal, &pk,
		))
		if info.Name == "transcript_revision" {
			return info
		}
	}
	require.NoError(t, rows.Err())
	require.FailNow(t, "sessions.transcript_revision column not found")
	return phase20ColumnInfo{}
}

type phase20SQLiteQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func assertPhase20LegacyRepairProbeExcludesTranscriptRevision(t *testing.T) {
	t.Helper()
	for _, migration := range legacySchemaColumnMigrations() {
		assert.NotEqual(t, "transcript_revision", migration.column,
			"legacy repair probes must not include transcript_revision")
	}
}

func requireRawPhase20Rows(t *testing.T, conn sqliteQueryRower) {
	t.Helper()
	var sessions, messages int
	require.NoError(t, conn.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE id = 'phase20-session'",
	).Scan(&sessions))
	require.NoError(t, conn.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE session_id = 'phase20-session'",
	).Scan(&messages))
	assert.Equal(t, 1, sessions)
	assert.Equal(t, 1, messages)
}

func phase20SessionsByID(sessions []Session) map[string]Session {
	rows := make(map[string]Session, len(sessions))
	for _, session := range sessions {
		rows[session.ID] = session
	}
	return rows
}

func phase20Ptr[T any](v T) *T { return &v }

func phase20SidebarRowsByID(
	sessions []SidebarSessionIndexRow,
) map[string]SidebarSessionIndexRow {
	rows := make(map[string]SidebarSessionIndexRow, len(sessions))
	for _, session := range sessions {
		rows[session.ID] = session
	}
	return rows
}
