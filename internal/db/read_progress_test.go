package db

import (
	"context"
	"database/sql"
	"errors"
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
	fileHash := "phase20-file-hash"
	localModifiedAt := "2026-08-18T00:03:00.000Z"
	insertSession(t, d, "phase20-parent", "project-a", func(s *Session) {
		s.StartedAt = new("2026-08-18T00:00:00Z")
		s.EndedAt = new("2026-08-18T00:01:00Z")
		s.FileHash = &fileHash
		s.LocalModifiedAt = &localModifiedAt
	})
	insertSession(t, d, "phase20-child", "project-a", func(s *Session) {
		s.ParentSessionID = new("phase20-parent")
		s.RelationshipType = "subagent"
		s.StartedAt = new("2026-08-18T00:02:00Z")
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
	assertPhase20SessionRevision(t, *got, "7", fileHash, localModifiedAt)

	full, err := d.GetSessionFull(ctx, "phase20-parent")
	require.NoError(t, err)
	require.NotNil(t, full)
	assertPhase20SessionRevision(t, *full, "7", fileHash, localModifiedAt)

	page, err := d.ListSessions(ctx, SessionFilter{Limit: 10})
	require.NoError(t, err)
	pageRows := phase20SessionsByID(page.Sessions)
	require.Contains(t, pageRows, "phase20-parent")
	assertPhase20SessionRevision(t, pageRows["phase20-parent"], "7", fileHash, localModifiedAt)

	children, err := d.GetChildSessions(ctx, "phase20-parent")
	require.NoError(t, err)
	require.Len(t, children, 1)
	assertPhase20SessionRevision(t, children[0], "3", fileHash, localModifiedAt)

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
	assertPhase20SessionRevision(t, modifiedRows["phase20-parent"], "7", fileHash, localModifiedAt)

	index, err := d.GetSidebarSessionIndex(ctx, SessionFilter{})
	require.NoError(t, err)
	indexRows := phase20SidebarRowsByID(index.Sessions)
	require.Contains(t, indexRows, "phase20-parent")
	require.NotNil(t, indexRows["phase20-parent"].TranscriptRevision)
	assert.Equal(t, "7", *indexRows["phase20-parent"].TranscriptRevision)
}

func assertPhase20SessionRevision(
	t *testing.T, s Session, want, fileHash, localModifiedAt string,
) {
	t.Helper()
	require.NotNil(t, s.TranscriptRevision)
	assert.Equal(t, want, *s.TranscriptRevision)
	assert.NotEqual(t, fileHash, *s.TranscriptRevision)
	assert.NotEqual(t, localModifiedAt, *s.TranscriptRevision)
}

func TestPhase20TranscriptMessagesEqualVisibleContract(t *testing.T) {
	base := phase20Transcript("s1", 0, "hello")
	sameVisible := phase20Transcript("s1", 0, "hello")
	sameVisible[0].ID = 99
	sameVisible[0].ContentLength = 500
	sameVisible[0].TokenUsage = []byte(`{"raw":"ignored"}`)
	sameVisible[0].ClaudeMessageID = "ignored-message-id"
	sameVisible[0].ClaudeRequestID = "ignored-request-id"
	sameVisible[0].SourceType = "ignored-source-type"
	sameVisible[0].SourceUUID = "ignored-source-uuid"
	sameVisible[0].SourceParentUUID = "ignored-parent-uuid"
	sameVisible[0].IsSidechain = true
	sameVisible[0].ToolCalls[0].MessageID = 123
	sameVisible[0].ToolCalls[0].SessionID = "ignored-session"
	sameVisible[0].ToolCalls[0].ResultContentLength = 999
	sameVisible[0].ToolCalls[0].ResultEvents[0].ContentLength = 999
	assert.True(t, transcriptMessagesEqual(base, sameVisible),
		"row ids, derived lengths, raw token payload and source bookkeeping are not visible transcript changes")

	cases := []struct {
		name string
		edit func([]Message) []Message
	}{
		{
			name: "missing ordinal",
			edit: func(msgs []Message) []Message { return nil },
		},
		{
			name: "duplicate ordinal",
			edit: func(msgs []Message) []Message {
				return append(msgs, msgs[0])
			},
		},
		{
			name: "message content",
			edit: func(msgs []Message) []Message {
				msgs[0].Content = "changed"
				return msgs
			},
		},
		{
			name: "thinking text",
			edit: func(msgs []Message) []Message {
				msgs[0].ThinkingText = "changed thinking"
				return msgs
			},
		},
		{
			name: "token presence",
			edit: func(msgs []Message) []Message {
				msgs[0].HasOutputTokens = false
				return msgs
			},
		},
		{
			name: "tool input",
			edit: func(msgs []Message) []Message {
				msgs[0].ToolCalls[0].InputJSON = `{"path":"changed"}`
				return msgs
			},
		},
		{
			name: "tool order",
			edit: func(msgs []Message) []Message {
				msgs[0].ToolCalls = append(msgs[0].ToolCalls, ToolCall{
					ToolName: "Bash", Category: "Bash",
				})
				msgs[0].ToolCalls[0], msgs[0].ToolCalls[1] = msgs[0].ToolCalls[1], msgs[0].ToolCalls[0]
				return msgs
			},
		},
		{
			name: "result event content",
			edit: func(msgs []Message) []Message {
				msgs[0].ToolCalls[0].ResultEvents[0].Content = "changed event"
				return msgs
			},
		},
		{
			name: "result event order",
			edit: func(msgs []Message) []Message {
				msgs[0].ToolCalls[0].ResultEvents = append(
					msgs[0].ToolCalls[0].ResultEvents,
					ToolResultEvent{Source: "stderr", Status: "ok", Content: "second"},
				)
				msgs[0].ToolCalls[0].ResultEvents[0], msgs[0].ToolCalls[0].ResultEvents[1] =
					msgs[0].ToolCalls[0].ResultEvents[1], msgs[0].ToolCalls[0].ResultEvents[0]
				return msgs
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := tc.edit(phase20Transcript("s1", 0, "hello"))
			assert.False(t, transcriptMessagesEqual(base, changed))
		})
	}
}

func TestPhase20SQLiteRevisionInsertMessagesBumpsDistinctSessionsOnce(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "phase20-insert-a", "project-a")
	insertSession(t, d, "phase20-insert-b", "project-a")

	require.NoError(t, d.InsertMessages([]Message{
		phase20Msg("phase20-insert-a", 0, "a0"),
		phase20Msg("phase20-insert-a", 1, "a1"),
		phase20Msg("phase20-insert-b", 0, "b0"),
	}))

	assert.Equal(t, "1", phase20Revision(t, d, "phase20-insert-a"))
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-insert-b"))
}

func TestPhase20SQLiteRevisionReplaceSessionMessagesVisibleChangesOnly(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "phase20-replace", "project-a")

	base := phase20Transcript("phase20-replace", 0, "hello")
	require.NoError(t, d.ReplaceSessionMessages("phase20-replace", base))
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-replace"))

	sameVisible := phase20Transcript("phase20-replace", 0, "hello")
	sameVisible[0].ID = 42
	sameVisible[0].ContentLength = 999
	sameVisible[0].TokenUsage = []byte(`{"raw":"ignored"}`)
	sameVisible[0].SourceUUID = "ignored-source-uuid"
	require.NoError(t, d.ReplaceSessionMessages("phase20-replace", sameVisible))
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-replace"))

	changed := phase20Transcript("phase20-replace", 0, "changed")
	require.NoError(t, d.ReplaceSessionMessages("phase20-replace", changed))
	assert.Equal(t, "2", phase20Revision(t, d, "phase20-replace"))

	changedTool := phase20Transcript("phase20-replace", 0, "changed")
	changedTool[0].ToolCalls[0].ResultEvents[0].Content = "changed tool event"
	require.NoError(t, d.ReplaceSessionMessages("phase20-replace", changedTool))
	assert.Equal(t, "3", phase20Revision(t, d, "phase20-replace"))
}

func TestPhase20SQLiteRevisionReplaceSessionContentSharesMessageSemantics(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "phase20-content", "project-a")
	signals := SessionSignalUpdate{Outcome: "completed", OutcomeConfidence: "high"}

	base := []Message{phase20Msg("phase20-content", 0, "hello")}
	require.NoError(t, d.ReplaceSessionContent("phase20-content", base, signals, nil))
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-content"))

	newSignals := SessionSignalUpdate{Outcome: "errored", OutcomeConfidence: "low"}
	findings := []SecretFinding{{
		RuleName: "phase20-rule", Confidence: "low", LocationKind: "message",
		MessageOrdinal: 0, MatchStart: 0, MatchEnd: 4, MatchIndex: 0,
		RedactedMatch: "redacted", RulesVersion: "phase20-rules",
	}}
	require.NoError(t, d.ReplaceSessionContent("phase20-content", base, newSignals, findings))
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-content"))

	changed := []Message{phase20Msg("phase20-content", 0, "changed")}
	require.NoError(t, d.ReplaceSessionContent("phase20-content", changed, newSignals, nil))
	assert.Equal(t, "2", phase20Revision(t, d, "phase20-content"))
}

func TestPhase20SQLiteRevisionWriteSessionBatchAppendAndReplace(t *testing.T) {
	d := testDB(t)

	result, err := d.WriteSessionBatch([]SessionBatchWrite{{
		Session: phase20Session("phase20-batch-empty"),
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "0", phase20Revision(t, d, "phase20-batch-empty"))

	result, err = d.WriteSessionBatch([]SessionBatchWrite{{
		Session:  phase20Session("phase20-batch-append"),
		Messages: []Message{phase20Msg("phase20-batch-append", 0, "a0")},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-batch-append"))

	result, err = d.WriteSessionBatch([]SessionBatchWrite{{
		Session:  phase20Session("phase20-batch-append"),
		Messages: []Message{phase20Msg("phase20-batch-append", 0, "a0")},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-batch-append"))

	result, err = d.WriteSessionBatch([]SessionBatchWrite{{
		Session: phase20Session("phase20-batch-append"),
		Messages: []Message{
			phase20Msg("phase20-batch-append", 0, "a0"),
			phase20Msg("phase20-batch-append", 1, "a1"),
		},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "2", phase20Revision(t, d, "phase20-batch-append"))

	result, err = d.WriteSessionBatch([]SessionBatchWrite{{
		Session:         phase20Session("phase20-batch-append"),
		Messages:        []Message{phase20Msg("phase20-batch-append", 0, "a0"), phase20Msg("phase20-batch-append", 1, "a1")},
		ReplaceMessages: true,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "2", phase20Revision(t, d, "phase20-batch-append"))

	result, err = d.WriteSessionBatch([]SessionBatchWrite{{
		Session:         phase20Session("phase20-batch-append"),
		Messages:        []Message{phase20Msg("phase20-batch-append", 0, "rewritten")},
		ReplaceMessages: true,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "3", phase20Revision(t, d, "phase20-batch-append"))
}

func TestPhase20SQLiteRevisionWriteSessionBatchAtomicRollsBackBump(t *testing.T) {
	d := testDB(t)

	result, err := d.WriteSessionBatchAtomic([]SessionBatchWrite{{
		Session: phase20Session("phase20-atomic"),
		Messages: []Message{
			phase20Msg("phase20-atomic", 0, "a0"),
			phase20Msg("phase20-atomic", 1, "a1"),
		},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-atomic"))

	sentinel := errors.New("phase20 rollback sentinel")
	result, err = d.WriteSessionBatchAtomic([]SessionBatchWrite{{
		Session: phase20Session("phase20-atomic"),
		Messages: []Message{
			phase20Msg("phase20-atomic", 0, "a0"),
			phase20Msg("phase20-atomic", 1, "a1"),
			phase20Msg("phase20-atomic", 2, "a2"),
		},
	}}, func() error { return sentinel })
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 0, result.WrittenSessions)
	assert.Equal(t, "1", phase20Revision(t, d, "phase20-atomic"))
	assert.Equal(t, 1, d.MaxOrdinal("phase20-atomic"))
}

func TestPhase20OrphanCopyPreservesTranscriptRevision(t *testing.T) {
	src := testDB(t)
	dst := testDB(t)
	insertSession(t, src, "phase20-orphan", "proj")
	insertMessages(t, src,
		userMsg("phase20-orphan", 0, "orphan prompt"),
		asstMsg("phase20-orphan", 1, "orphan reply"),
	)
	setPhase20Revision(t, src, "phase20-orphan", "17")
	require.NoError(t, src.CloseConnections())

	copied, err := dst.CopyOrphanedDataFrom(src.Path())
	require.NoError(t, err)
	assert.Equal(t, 1, copied)
	assert.Equal(t, "17", phase20Revision(t, dst, "phase20-orphan"))
	msgs, err := dst.GetAllMessages(context.Background(), "phase20-orphan")
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "orphan prompt", msgs[0].Content)
	assert.Equal(t, "orphan reply", msgs[1].Content)
}

func TestPhase20OrphanCopyReconcilesMatchingTranscriptRevisionsWhenNoOrphans(t *testing.T) {
	src := testDB(t)
	dst := testDB(t)
	for _, id := range []string{"phase20-unchanged", "phase20-rewrite", "phase20-rename"} {
		insertSession(t, src, id, "proj")
		insertSession(t, dst, id, "proj")
	}
	insertMessages(t, src,
		userMsg("phase20-unchanged", 0, "same"),
		userMsg("phase20-rewrite", 0, "old"),
		userMsg("phase20-rename", 0, "rename visible"),
	)
	insertMessages(t, dst,
		userMsg("phase20-unchanged", 0, "same"),
		userMsg("phase20-rewrite", 0, "new"),
		userMsg("phase20-rename", 0, "rename visible"),
	)
	setPhase20Revision(t, src, "phase20-unchanged", "7")
	setPhase20Revision(t, src, "phase20-rewrite", "11")
	setPhase20Revision(t, src, "phase20-rename", "13")
	setPhase20Revision(t, dst, "phase20-unchanged", "1")
	setPhase20Revision(t, dst, "phase20-rewrite", "1")
	setPhase20Revision(t, dst, "phase20-rename", "1")
	customName := "manual rename"
	require.NoError(t, src.RenameSession("phase20-rename", &customName))
	require.NoError(t, src.CloseConnections())

	copied, err := dst.CopyOrphanedDataFrom(src.Path())
	require.NoError(t, err)
	assert.Equal(t, 0, copied)
	assert.Equal(t, "7", phase20Revision(t, dst, "phase20-unchanged"))
	assert.Equal(t, "12", phase20Revision(t, dst, "phase20-rewrite"))
	assert.Equal(t, "13", phase20Revision(t, dst, "phase20-rename"))
}

func setPhase20Revision(t *testing.T, d *DB, sessionID, revision string) {
	t.Helper()
	require.NoError(t, d.Update(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`UPDATE sessions SET transcript_revision = ? WHERE id = ?`,
			revision, sessionID,
		)
		return err
	}))
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

func phase20Session(id string) Session {
	return Session{
		ID: id, Project: "project-a", Machine: defaultMachine,
		Agent: defaultAgent, MessageCount: 1,
	}
}

func phase20Msg(sessionID string, ordinal int, content string) Message {
	return Message{
		SessionID: sessionID, Ordinal: ordinal, Role: "assistant",
		Content: content, ContentLength: len(content), Timestamp: tsZero,
	}
}

func phase20Transcript(sessionID string, ordinal int, content string) []Message {
	msg := phase20Msg(sessionID, ordinal, content)
	msg.ID = 10
	msg.ThinkingText = "thinking"
	msg.HasThinking = true
	msg.HasToolUse = true
	msg.IsSystem = true
	msg.Model = "model-a"
	msg.ContextTokens = 11
	msg.OutputTokens = 7
	msg.HasContextTokens = true
	msg.HasOutputTokens = true
	msg.SourceSubtype = "compact"
	msg.IsCompactBoundary = true
	msg.ToolCalls = []ToolCall{{
		MessageID: 10, SessionID: sessionID, ToolName: "Read",
		Category: "Read", ToolUseID: "toolu_1",
		InputJSON: `{"path":"a.go"}`, SkillName: "assist-learn",
		ResultContent: "result", ResultContentLength: 6,
		SubagentSessionID: "child-session",
		ResultEvents: []ToolResultEvent{{
			ToolUseID: "toolu_1", AgentID: "agent-a",
			SubagentSessionID: "child-session", Source: "stdout",
			Status: "ok", Content: "event", ContentLength: 5,
			Timestamp: tsZeroS1, EventIndex: 0,
		}},
	}}
	return []Message{msg}
}

func phase20Revision(t *testing.T, d *DB, sessionID string) string {
	t.Helper()
	var revision string
	require.NoError(t, d.getReader().QueryRow(
		`SELECT transcript_revision FROM sessions WHERE id = ?`,
		sessionID,
	).Scan(&revision))
	return revision
}

func phase20SidebarRowsByID(
	sessions []SidebarSessionIndexRow,
) map[string]SidebarSessionIndexRow {
	rows := make(map[string]SidebarSessionIndexRow, len(sessions))
	for _, session := range sessions {
		rows[session.ID] = session
	}
	return rows
}
