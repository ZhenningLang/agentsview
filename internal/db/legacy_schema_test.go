package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const legacyMessagesAndToolCallsSchema = `
CREATE TABLE messages (
    id             INTEGER PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal        INTEGER NOT NULL,
    role           TEXT NOT NULL,
    content        TEXT NOT NULL,
    timestamp      TEXT,
    has_thinking   INTEGER NOT NULL DEFAULT 0,
    has_tool_use   INTEGER NOT NULL DEFAULT 0,
    content_length INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ordinal)
);
CREATE TABLE tool_calls (
    id         INTEGER PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    tool_name  TEXT NOT NULL,
    category   TEXT NOT NULL
);`

const preParentLegacySchema = `
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    project     TEXT NOT NULL,
    machine     TEXT NOT NULL DEFAULT 'local',
    agent       TEXT NOT NULL DEFAULT 'claude',
    first_message TEXT,
    started_at  TEXT,
    ended_at    TEXT,
    message_count INTEGER NOT NULL DEFAULT 0,
    file_path   TEXT,
    file_size   INTEGER,
    file_mtime  INTEGER,
    file_hash   TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);` + legacyMessagesAndToolCallsSchema

const v06LegacySchema = `
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    project     TEXT NOT NULL,
    machine     TEXT NOT NULL DEFAULT 'local',
    agent       TEXT NOT NULL DEFAULT 'claude',
    first_message TEXT,
    started_at  TEXT,
    ended_at    TEXT,
    message_count INTEGER NOT NULL DEFAULT 0,
    user_message_count INTEGER NOT NULL DEFAULT 0,
    file_path   TEXT,
    file_size   INTEGER,
    file_mtime  INTEGER,
    file_hash   TEXT,
    parent_session_id TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE messages (
    id             INTEGER PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal        INTEGER NOT NULL,
    role           TEXT NOT NULL,
    content        TEXT NOT NULL,
    timestamp      TEXT,
    has_thinking   INTEGER NOT NULL DEFAULT 0,
    has_tool_use   INTEGER NOT NULL DEFAULT 0,
    content_length INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ordinal)
);
CREATE TABLE tool_calls (
    id         INTEGER PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    tool_name  TEXT NOT NULL,
    category   TEXT NOT NULL,
    tool_use_id TEXT,
    input_json TEXT,
    skill_name TEXT,
    result_content_length INTEGER
);
CREATE TABLE insights (
    id          INTEGER PRIMARY KEY,
    type        TEXT NOT NULL,
    date_from   TEXT NOT NULL,
    date_to     TEXT NOT NULL,
    project     TEXT,
    agent       TEXT NOT NULL,
    model       TEXT,
    prompt      TEXT,
    content     TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_insights_lookup
    ON insights(type, date_from, project);`

const legacyArchiveRows = `
INSERT INTO sessions (
    id, project, machine, agent, first_message, message_count,
    file_path, file_size, file_mtime, file_hash
) VALUES (
    'legacy-session', 'project-a', 'local', 'claude',
    'archived prompt', 1, '/archive/session.jsonl', 128, 42, 'legacy-hash'
);
INSERT INTO messages (
	id, session_id, ordinal, role, content, timestamp,
	has_tool_use, content_length
) VALUES (
	1, 'legacy-session', 0, 'assistant', 'archived answer',
	'2026-01-01T00:00:00Z', 1, 15
);
INSERT INTO tool_calls (
    id, message_id, session_id, tool_name, category
) VALUES (
    1, 1, 'legacy-session', 'Read', 'Read'
);`

func TestOpenLegacySchemasPreservesArchiveAndRequestsResync(t *testing.T) {
	tests := []struct {
		name            string
		schema          string
		wantInsightDate string
	}{
		{
			name:   "pre-parent-link archive",
			schema: preParentLegacySchema,
		},
		{
			name:            "v0.6 archive with range insight",
			schema:          v06LegacySchema,
			wantInsightDate: "2026-02-23",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.db")
			createLegacyArchive(t, path, tc.schema, tc.wantInsightDate)

			before, err := os.Stat(path)
			require.NoError(t, err)

			d, err := Open(path)
			require.NoError(t, err)
			assert.True(t, d.NeedsResync())

			after, err := os.Stat(path)
			require.NoError(t, err)
			assert.True(t, os.SameFile(before, after))

			requireArchivedRows(t, d)
			requireInsightRowsAndCurrentWrites(t, d, tc.wantInsightDate)
			requireLegacyRepairColumns(t, d)
			requireLegacyRepairIndexes(t, d)
			requireDBHealthy(t, d)
			require.NoError(t, d.Close())

			reopened, err := Open(path)
			require.NoError(t, err)
			defer reopened.Close()
			require.True(t, reopened.NeedsResync())
			requireArchivedRows(t, reopened)
		})
	}
}

func TestRepairLegacySchemaBeforeInitRollsBackOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyArchive(t, path, preParentLegacySchema, "")

	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	err = repairLegacySchemaBeforeInit(conn, []schemaColumnMigration{
		{
			"sessions", "rollback_probe",
			"ALTER TABLE sessions ADD COLUMN rollback_probe TEXT",
		},
		{
			"sessions", "broken_probe",
			"ALTER TABLE sessions ADD COLUMN broken_probe TEXT NOT NULL",
		},
	})
	require.Error(t, err)

	assert.False(t, sqliteColumnExists(t, conn, "sessions", "rollback_probe"))
	requireRawArchiveRows(t, conn)

	var version int
	require.NoError(t, conn.QueryRow(
		"PRAGMA user_version",
	).Scan(&version))
	assert.Equal(t, dataVersion, version)
}

func TestOpenRejectsNewerLegacySchemaBeforeRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyArchive(t, path, preParentLegacySchema, "")
	futureVersion := dataVersion + 10

	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	_, err = conn.Exec(fmt.Sprintf(
		"PRAGMA user_version = %d", futureVersion,
	))
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	_, err = Open(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than supported version")

	conn, err = sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	defer conn.Close()
	assert.False(t, sqliteColumnExists(t, conn, "sessions", "parent_session_id"))
	requireRawArchiveRows(t, conn)

	var version int
	require.NoError(t, conn.QueryRow(
		"PRAGMA user_version",
	).Scan(&version))
	assert.Equal(t, futureVersion, version)
}

func createLegacyArchive(
	t *testing.T, path, schema, insightDate string,
) {
	t.Helper()
	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)

	_, err = conn.Exec(schema)
	require.NoError(t, err)
	_, err = conn.Exec(legacyArchiveRows)
	require.NoError(t, err)
	if insightDate != "" {
		_, err = conn.Exec(`
			INSERT INTO insights (
				id, type, date_from, date_to, project,
				agent, model, prompt, content
			) VALUES (
				1, 'daily', ?, ?, 'project-a', 'claude',
				'model-a', 'summarize', 'archived insight'
			)`, insightDate, insightDate)
		require.NoError(t, err)
	}
	_, err = conn.Exec(fmt.Sprintf(
		"PRAGMA user_version = %d", dataVersion,
	))
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func requireArchivedRows(t *testing.T, d *DB) {
	t.Helper()
	session := requireSessionExists(t, d, "legacy-session")
	assert.Equal(t, "project-a", session.Project)
	require.NotNil(t, session.FirstMessage)
	assert.Equal(t, "archived prompt", *session.FirstMessage)

	messages, err := d.GetMessages(
		context.Background(), "legacy-session", 0, 10, true,
	)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Len(t, messages[0].ToolCalls, 1)
	assert.Equal(t, "Read", messages[0].ToolCalls[0].ToolName)
}

func requireInsightRowsAndCurrentWrites(
	t *testing.T, d *DB, wantInsightDate string,
) {
	t.Helper()
	var dateFrom, dateTo string
	if wantInsightDate != "" {
		err := d.getReader().QueryRow(`
			SELECT date_from, date_to FROM insights WHERE id = 1
		`).Scan(&dateFrom, &dateTo)
		require.NoError(t, err)
		assert.Equal(t, wantInsightDate, dateFrom)
		assert.Equal(t, wantInsightDate, dateTo)
	}

	id, err := d.InsertInsight(Insight{
		Type:     "daily",
		DateFrom: "2026-07-14",
		DateTo:   "2026-07-14",
		Agent:    "claude",
		Content:  "new insight",
	})
	require.NoError(t, err)
	inserted, err := d.GetInsight(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, inserted)
	assert.Equal(t, "2026-07-14", inserted.DateFrom)
	assert.Equal(t, "2026-07-14", inserted.DateTo)
	assert.Equal(t, "new insight", inserted.Content)
}

func requireLegacyRepairColumns(t *testing.T, d *DB) {
	t.Helper()
	for _, migration := range legacySchemaColumnMigrations() {
		assert.True(
			t,
			sqliteColumnExists(t, d.getReader(), migration.table, migration.column),
			"%s.%s", migration.table, migration.column,
		)
	}
}

func requireLegacyRepairIndexes(t *testing.T, d *DB) {
	t.Helper()
	for _, name := range []string{
		"idx_sessions_parent",
		"idx_sessions_user_message_count",
		"idx_tool_calls_skill",
		"idx_tool_calls_subagent",
		"idx_insights_lookup",
	} {
		var count int
		err := d.getReader().QueryRow(`
			SELECT count(*) FROM sqlite_master
			WHERE type = 'index' AND name = ?
		`, name).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "index %s", name)
	}
}

func requireDBHealthy(t *testing.T, d *DB) {
	t.Helper()
	var mode string
	require.NoError(t, d.getReader().QueryRow(
		"PRAGMA journal_mode",
	).Scan(&mode))
	assert.Equal(t, "wal", mode)

	var integrity string
	require.NoError(t, d.getReader().QueryRow(
		"PRAGMA integrity_check",
	).Scan(&integrity))
	assert.Equal(t, "ok", integrity)
}

type sqliteQueryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func sqliteColumnExists(
	t *testing.T, conn sqliteQueryRower, table, column string,
) bool {
	t.Helper()
	var count int
	err := conn.QueryRow(fmt.Sprintf(
		"SELECT count(*) FROM pragma_table_info('%s') WHERE name = '%s'",
		table, column,
	)).Scan(&count)
	require.NoError(t, err)
	return count == 1
}

func requireRawArchiveRows(t *testing.T, conn *sql.DB) {
	t.Helper()
	var sessions, messages int
	require.NoError(t, conn.QueryRow(
		"SELECT count(*) FROM sessions WHERE id = 'legacy-session'",
	).Scan(&sessions))
	require.NoError(t, conn.QueryRow(
		"SELECT count(*) FROM messages WHERE session_id = 'legacy-session'",
	).Scan(&messages))
	assert.Equal(t, 1, sessions)
	assert.Equal(t, 1, messages)
}
