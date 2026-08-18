package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		s.CreatedAt = "2026-08-18T00:00:00Z"
		s.StartedAt = new("2026-08-18T00:00:00Z")
		s.EndedAt = new("2026-08-18T00:01:00Z")
		s.FileHash = &fileHash
		s.LocalModifiedAt = &localModifiedAt
	})
	insertSession(t, d, "phase20-child", "project-a", func(s *Session) {
		s.CreatedAt = "2026-08-18T00:02:00Z"
		s.ParentSessionID = new("phase20-parent")
		s.RelationshipType = "subagent"
		s.StartedAt = new("2026-08-18T00:02:00Z")
	})
	_, err := d.getWriter().Exec(
		`UPDATE sessions
		 SET transcript_revision = '7',
		     created_at = '2026-08-18T00:00:00Z',
		     local_modified_at = '2026-08-18T00:03:00.000Z'
		 WHERE id = 'phase20-parent'`,
	)
	require.NoError(t, err)
	_, err = d.getWriter().Exec(
		`UPDATE sessions
		 SET transcript_revision = '3',
		     created_at = '2026-08-18T00:02:00Z'
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

// TestPhase20TranscriptMessagesEqualIsOrdinalAware locks the comparison
// key to the ordinal instead of the slice position. The frontend read
// boundary indexes by ordinal, so an index-based comparator would make
// the revision owner and the browser disagree: a transcript reordered
// without any visible edit would bump the revision and show a phantom
// unread, and every ordinal-keyed case below has to keep its verdict
// even when the two slices are not aligned position by position.
func TestPhase20TranscriptMessagesEqualIsOrdinalAware(t *testing.T) {
	transcript := func(ordinals ...int) []Message {
		msgs := make([]Message, 0, len(ordinals))
		for _, ordinal := range ordinals {
			msgs = append(
				msgs,
				phase20Transcript("s1", ordinal, fmt.Sprintf("m%d", ordinal))[0],
			)
		}
		return msgs
	}

	t.Run("same ordinals in a different slice order are unchanged", func(t *testing.T) {
		assert.True(t, transcriptMessagesEqual(
			transcript(0, 1, 2),
			transcript(2, 0, 1),
		), "slice position is not visible to a reader; only ordinals are")
	})

	t.Run("shifted ordinal is changed", func(t *testing.T) {
		assert.False(t, transcriptMessagesEqual(
			transcript(0, 1, 2),
			transcript(0, 1, 3),
		))
	})

	t.Run("reordered with a rewritten message is changed", func(t *testing.T) {
		rewritten := transcript(2, 0, 1)
		rewritten[1].Content = "rewritten"
		assert.False(t, transcriptMessagesEqual(transcript(0, 1, 2), rewritten))
	})

	t.Run("duplicate ordinal at equal length is changed", func(t *testing.T) {
		assert.False(t, transcriptMessagesEqual(
			transcript(0, 1, 2),
			transcript(0, 1, 1),
		))
		assert.False(t, transcriptMessagesEqual(
			transcript(0, 1, 1),
			transcript(0, 1, 2),
		))
	})

	t.Run("missing ordinal at equal length is changed", func(t *testing.T) {
		gapped := transcript(0, 1, 2)
		gapped = append(gapped[:1], gapped[2:]...)
		assert.False(t, transcriptMessagesEqual(transcript(0, 1), gapped))
	})
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

// TestPhase20SQLiteRevisionReplaceSessionMessagesFieldContract drives the
// visible/derived split through a real writer rather than through the
// comparator alone. Each case starts from an identical rewrite, which must not
// move the counter, so a "visible" field that silently fails to persist would
// show up as a bump on the no-op instead of passing by accident.
func TestPhase20SQLiteRevisionReplaceSessionMessagesFieldContract(t *testing.T) {
	const id = "phase20-field-contract"

	visible := []struct {
		name string
		edit func([]Message)
	}{
		{"message content", func(m []Message) { m[0].Content = "changed" }},
		{"thinking text", func(m []Message) { m[0].ThinkingText = "changed thinking" }},
		{"thinking flag", func(m []Message) { m[0].HasThinking = false }},
		{"system flag", func(m []Message) { m[0].IsSystem = false }},
		{"model", func(m []Message) { m[0].Model = "model-b" }},
		{"context token presence", func(m []Message) { m[0].HasContextTokens = false }},
		{"output token presence", func(m []Message) { m[0].HasOutputTokens = false }},
		{"context token count", func(m []Message) { m[0].ContextTokens = 12 }},
		{"output token count", func(m []Message) { m[0].OutputTokens = 8 }},
		{"source subtype", func(m []Message) { m[0].SourceSubtype = "resume" }},
		{"compact boundary", func(m []Message) { m[0].IsCompactBoundary = false }},
		{"tool name", func(m []Message) { m[0].ToolCalls[0].ToolName = "Bash" }},
		{"tool input", func(m []Message) { m[0].ToolCalls[0].InputJSON = `{"path":"b.go"}` }},
		{"tool skill name", func(m []Message) { m[0].ToolCalls[0].SkillName = "guard-review" }},
		{"tool result content", func(m []Message) { m[0].ToolCalls[0].ResultContent = "changed result" }},
		{"tool subagent session", func(m []Message) { m[0].ToolCalls[0].SubagentSessionID = "other-child" }},
		{"result event content", func(m []Message) { m[0].ToolCalls[0].ResultEvents[0].Content = "changed event" }},
		{"result event status", func(m []Message) { m[0].ToolCalls[0].ResultEvents[0].Status = "error" }},
		{"result event source", func(m []Message) { m[0].ToolCalls[0].ResultEvents[0].Source = "stderr" }},
		{
			name: "shifted ordinal at equal length",
			edit: func(m []Message) { m[0].Ordinal = 7 },
		},
	}

	derived := []struct {
		name string
		edit func([]Message)
	}{
		{"message row id", func(m []Message) { m[0].ID = 4242 }},
		{"content length", func(m []Message) { m[0].ContentLength = 999 }},
		{"raw token payload", func(m []Message) { m[0].TokenUsage = []byte(`{"raw":"ignored"}`) }},
		{"claude message id", func(m []Message) { m[0].ClaudeMessageID = "msg_ignored" }},
		{"claude request id", func(m []Message) { m[0].ClaudeRequestID = "req_ignored" }},
		{"source bookkeeping", func(m []Message) {
			m[0].SourceType = "ignored-type"
			m[0].SourceUUID = "ignored-uuid"
			m[0].SourceParentUUID = "ignored-parent"
		}},
		{"sidechain flag", func(m []Message) { m[0].IsSidechain = true }},
		{"tool row bookkeeping", func(m []Message) {
			m[0].ToolCalls[0].MessageID = 4242
			m[0].ToolCalls[0].SessionID = "ignored-session"
		}},
		{"tool result length", func(m []Message) { m[0].ToolCalls[0].ResultContentLength = 999 }},
		{"result event length", func(m []Message) { m[0].ToolCalls[0].ResultEvents[0].ContentLength = 999 }},
	}

	seed := func(t *testing.T) *DB {
		t.Helper()
		d := testDB(t)
		insertSession(t, d, id, "project-a")
		require.NoError(t, d.ReplaceSessionMessages(id, phase20Transcript(id, 0, "hello")))
		require.Equal(t, "1", phase20Revision(t, d, id))
		require.NoError(t, d.ReplaceSessionMessages(id, phase20Transcript(id, 0, "hello")))
		require.Equal(t, "1", phase20Revision(t, d, id),
			"an identical rewrite must not move the counter")
		return d
	}

	for _, tc := range visible {
		t.Run("bumps on "+tc.name, func(t *testing.T) {
			d := seed(t)
			next := phase20Transcript(id, 0, "hello")
			tc.edit(next)
			require.NoError(t, d.ReplaceSessionMessages(id, next))
			assert.Equal(t, "2", phase20Revision(t, d, id))
		})
	}

	for _, tc := range derived {
		t.Run("ignores "+tc.name, func(t *testing.T) {
			d := seed(t)
			next := phase20Transcript(id, 0, "hello")
			tc.edit(next)
			require.NoError(t, d.ReplaceSessionMessages(id, next))
			assert.Equal(t, "1", phase20Revision(t, d, id))
		})
	}
}

// TestPhase20SQLiteRevisionBumpsConservativelyWhenTheOldTranscriptFailsToLoad
// covers the one path where the writer cannot answer "did anything visible
// change?": the stored transcript is unreadable. Guessing "unchanged" there
// would hide a real rewrite behind a stale marker, so the writer bumps.
//
// The corruption is a tool_calls row whose result_content_length holds text
// instead of a number, which SQLite's dynamic typing stores happily and the
// transcript scan then rejects.
func TestPhase20SQLiteRevisionBumpsConservativelyWhenTheOldTranscriptFailsToLoad(t *testing.T) {
	d := testDB(t)
	const id = "phase20-load-failure"
	insertSession(t, d, id, "project-a")
	require.NoError(t, d.ReplaceSessionMessages(id, phase20Transcript(id, 0, "hello")))
	require.Equal(t, "1", phase20Revision(t, d, id))

	require.NoError(t, d.Update(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`UPDATE tool_calls SET result_content_length = 'not-a-number'
			 WHERE session_id = ?`, id)
		return err
	}))
	require.NoError(t, d.Update(func(tx *sql.Tx) error {
		_, loadErr := loadSessionTranscriptTx(tx, id)
		require.Error(t, loadErr, "the fixture has to make the transcript load fail")
		return nil
	}))

	// Identical content: without the failure this rewrite would be a no-op.
	require.NoError(t, d.ReplaceSessionMessages(id, phase20Transcript(id, 0, "hello")))
	assert.Equal(t, "2", phase20Revision(t, d, id),
		"an unreadable old transcript must be treated as changed")
}

// TestPhase20SQLiteRevisionMetadataWritersDoNotBump pins the other half of the
// contract: everything that touches a session row without touching what the
// reader sees leaves the counter alone. A bump here would light up the sidebar
// on a rename or on a routine sync bookkeeping write.
func TestPhase20SQLiteRevisionMetadataWritersDoNotBump(t *testing.T) {
	d := testDB(t)
	const id = "phase20-metadata-writers"
	insertSession(t, d, id, "project-a")
	require.NoError(t, d.ReplaceSessionMessages(id, phase20Transcript(id, 0, "hello")))
	require.Equal(t, "1", phase20Revision(t, d, id))

	fileHash := "phase20-new-file-hash"
	modified := "2026-08-18T04:00:00.000Z"
	refreshed := phase20Session(id)
	refreshed.FileHash = &fileHash
	refreshed.LocalModifiedAt = &modified
	require.NoError(t, d.UpsertSession(refreshed))
	assert.Equal(t, "1", phase20Revision(t, d, id), "file metadata is not transcript content")

	displayName := "manual rename"
	require.NoError(t, d.RenameSession(id, &displayName))
	assert.Equal(t, "1", phase20Revision(t, d, id), "a rename is not transcript content")

	updateSignals(t, d, id, SessionSignalUpdate{
		Outcome: "completed", OutcomeConfidence: "high",
	})
	assert.Equal(t, "1", phase20Revision(t, d, id), "signals are not transcript content")

	require.NoError(t, d.ReplaceSessionContent(id, []Message{phase20Msg(id, 0, "hello")},
		SessionSignalUpdate{Outcome: "errored", OutcomeConfidence: "low"}, nil))
	assert.Equal(t, "2", phase20Revision(t, d, id),
		"the control: a real content change still bumps after all that metadata churn")
}

// TestPhase20SQLiteRevisionWriteSessionBatchOrdinalGapCountsAsChanged covers
// the batch writer's replacement path for a transcript whose ordinals moved
// without its length changing. Duplicate ordinals cannot be exercised through a
// writer -- messages carries UNIQUE(session_id, ordinal) -- so that half of the
// contract is pinned on the comparator by
// TestPhase20TranscriptMessagesEqualIsOrdinalAware.
func TestPhase20SQLiteRevisionWriteSessionBatchOrdinalGapCountsAsChanged(t *testing.T) {
	d := testDB(t)
	const id = "phase20-batch-ordinals"

	result, err := d.WriteSessionBatch([]SessionBatchWrite{{
		Session: phase20Session(id),
		Messages: []Message{
			phase20Msg(id, 0, "a0"),
			phase20Msg(id, 1, "a1"),
		},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	require.Equal(t, "1", phase20Revision(t, d, id))

	result, err = d.WriteSessionBatch([]SessionBatchWrite{{
		Session: phase20Session(id),
		Messages: []Message{
			phase20Msg(id, 0, "a0"),
			phase20Msg(id, 1, "a1"),
		},
		ReplaceMessages: true,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	require.Equal(t, "1", phase20Revision(t, d, id),
		"the same ordinals with the same content are not a change")

	result, err = d.WriteSessionBatch([]SessionBatchWrite{{
		Session: phase20Session(id),
		Messages: []Message{
			phase20Msg(id, 0, "a0"),
			phase20Msg(id, 2, "a1"),
		},
		ReplaceMessages: true,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)
	assert.Equal(t, "2", phase20Revision(t, d, id),
		"the same message count over a different ordinal set is a change")
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

// TestPhase20ResyncTranscriptRevisionReconciliationComparesToolsAndEvents
// pins the comparison surface of the reconciliation query. The query was
// rewritten from a per-session correlated difference into a single
// balanced multiset difference for cost reasons, and the rows it folds
// together still have to distinguish tool call input from tool result
// event content, both of which are visible in the transcript.
func TestPhase20ResyncTranscriptRevisionReconciliationComparesToolsAndEvents(t *testing.T) {
	src := testDB(t)
	dst := testDB(t)
	ids := []string{"phase20-tool-change", "phase20-event-change", "phase20-tool-same"}
	for _, id := range ids {
		insertSession(t, src, id, "proj")
		insertSession(t, dst, id, "proj")
	}
	toolMsg := func(id, input, event string) Message {
		msg := phase20Msg(id, 0, "body")
		msg.HasToolUse = true
		msg.ToolCalls = []ToolCall{{
			ToolName: "Read", Category: "Read", ToolUseID: "toolu_1",
			InputJSON: input, ResultContent: "result",
			ResultEvents: []ToolResultEvent{{
				ToolUseID: "toolu_1", Source: "stdout", Status: "ok",
				Content: event, EventIndex: 0,
			}},
		}}
		return msg
	}
	insertMessages(t, src,
		toolMsg("phase20-tool-change", `{"path":"old.go"}`, "event"),
		toolMsg("phase20-event-change", `{"path":"a.go"}`, "old event"),
		toolMsg("phase20-tool-same", `{"path":"a.go"}`, "event"),
	)
	insertMessages(t, dst,
		toolMsg("phase20-tool-change", `{"path":"new.go"}`, "event"),
		toolMsg("phase20-event-change", `{"path":"a.go"}`, "new event"),
		toolMsg("phase20-tool-same", `{"path":"a.go"}`, "event"),
	)
	for _, id := range ids {
		setPhase20Revision(t, src, id, "5")
		setPhase20Revision(t, dst, id, "1")
	}
	require.NoError(t, src.CloseConnections())

	copied, err := dst.CopyOrphanedDataFrom(src.Path())
	require.NoError(t, err)
	assert.Equal(t, 0, copied)
	assert.Equal(t, "6", phase20Revision(t, dst, "phase20-tool-change"),
		"tool call input is visible transcript content")
	assert.Equal(t, "6", phase20Revision(t, dst, "phase20-event-change"),
		"tool result event content is visible transcript content")
	assert.Equal(t, "5", phase20Revision(t, dst, "phase20-tool-same"))
}

// TestPhase20ResyncTranscriptRevisionReconciliationScalesWithArchiveSize
// guards the cost shape of the resync reconciliation, not its result.
//
// The reconciliation runs on every ResyncAll, inside the window where the
// original database is already quiesced and the swap has not happened, so
// a per-session comparison against the whole archive is not a slow path,
// it is an outage. A comparison that pairs every session with every other
// session's rows grows with the square of the session count: multiplying
// the sessions by phase20ReconcileScaleFactor would multiply the time by
// its square. This asserts the observed growth stays well under that.
//
// The fixtures are far smaller than the archives this repository targets
// (15483 sessions / 748394 messages); they only need to be big enough for
// the quadratic term to dominate the fixed setup cost.
func TestPhase20ResyncTranscriptRevisionReconciliationScalesWithArchiveSize(t *testing.T) {
	if testing.Short() {
		t.Skip("scale regression builds several thousand sessions")
	}
	const (
		baseSessions       = 300
		scaleFactor        = 4
		messagesPerSession = 10
		// Linear growth lands near scaleFactor, quadratic near its
		// square (16). Allow twice the linear expectation so ordinary
		// timing noise cannot fail the test.
		maxGrowth = 2.0 * scaleFactor
	)

	// Up to three attempts, passing on the first clean pair.
	//
	// Contention can only ever make a measurement slower, so a ratio
	// under the bound is valid evidence no matter what else the machine
	// was doing, while an inflated one proves nothing. A genuinely
	// quadratic reconciliation is ~16x in every attempt, clean or not,
	// so retrying does not weaken the bound - it only stops a saturated
	// `make test` from failing a linear implementation. Observed once at
	// 10.82x under a full-repo run while the isolated ratio was 3.7x.
	growth := math.Inf(1)
	for range 3 {
		base := phase20MeasureReconcile(t, baseSessions, messagesPerSession)
		scaled := phase20MeasureReconcile(
			t, baseSessions*scaleFactor, messagesPerSession,
		)
		attempt := float64(scaled) / float64(base)
		t.Logf(
			"reconcile %d sessions in %s, %d sessions in %s, growth %.2fx",
			baseSessions, base, baseSessions*scaleFactor, scaled, attempt,
		)
		growth = math.Min(growth, attempt)
		if growth < maxGrowth {
			break
		}
	}
	assert.Less(t, growth, maxGrowth,
		"reconciliation grew %.2fx for %dx the sessions in every attempt; "+
			"a per-session scan of the whole archive would grow ~%dx",
		growth, scaleFactor, scaleFactor*scaleFactor)
}

// phase20MeasureReconcile builds a source archive and an already parsed
// destination holding the same sessions with identical transcripts, then
// returns how long the orphan copy plus revision reconciliation took. It
// also asserts the reconciliation result so a fast but wrong query cannot
// satisfy the growth bound.
func phase20MeasureReconcile(
	t *testing.T, sessions, messagesPerSession int,
) time.Duration {
	t.Helper()
	src := testDB(t)
	dst := testDB(t)
	content := strings.Repeat("transcript body ", 12)

	srcMsgs := make([]Message, 0, sessions*messagesPerSession)
	dstMsgs := make([]Message, 0, sessions*messagesPerSession)
	ids := make([]string, 0, sessions)
	for i := range sessions {
		id := fmt.Sprintf("phase20-scale-%05d", i)
		ids = append(ids, id)
		insertSession(t, src, id, "proj")
		insertSession(t, dst, id, "proj")
		for ordinal := range messagesPerSession {
			msg := phase20Msg(id, ordinal, content)
			srcMsgs = append(srcMsgs, msg)
			dstMsgs = append(dstMsgs, msg)
		}
	}
	insertMessages(t, src, srcMsgs...)
	insertMessages(t, dst, dstMsgs...)
	for _, id := range ids {
		setPhase20Revision(t, src, id, "5")
		setPhase20Revision(t, dst, id, "1")
	}
	require.NoError(t, src.CloseConnections())

	start := time.Now()
	copied, err := dst.CopyOrphanedDataFrom(src.Path())
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 0, copied)
	assert.Equal(t, "5", phase20Revision(t, dst, ids[0]),
		"unchanged transcripts inherit the archived revision")
	assert.Equal(t, "5", phase20Revision(t, dst, ids[len(ids)-1]),
		"unchanged transcripts inherit the archived revision")
	return elapsed
}

// TestPhase20OrphanCopyLegacyArchiveSchemaSkipsTranscriptRevisionReconciliation
// covers the shape every existing user hits on their first resync after
// upgrading: the archive being replaced predates the column, so there is
// nothing to reconcile against. The reconciliation has to step aside quietly.
// Failing here would abort the resync with the original database already
// quiesced, and guessing a revision would fabricate unread state for the whole
// archive.
func TestPhase20OrphanCopyLegacyArchiveSchemaSkipsTranscriptRevisionReconciliation(t *testing.T) {
	dst := testDB(t)
	srcPath := filepath.Join(t.TempDir(), "legacy-archive.db")
	createPhase20CurrentArchiveWithoutTranscriptRevision(t, srcPath)
	addPhase20LegacyArchiveSession(t, srcPath, "phase20-legacy-orphan", "orphan answer")

	// The archive and the fresh parse share one session, so reconciliation
	// would run over it if the old schema were comparable at all.
	insertSession(t, dst, "phase20-session", "project-a")
	insertMessages(t, dst, userMsg("phase20-session", 0, "reparsed answer"))
	setPhase20Revision(t, dst, "phase20-session", "4")

	copied, err := dst.CopyOrphanedDataFrom(srcPath)
	require.NoError(t, err,
		"an archive without the column must be skipped, not fail the resync")
	assert.Equal(t, 1, copied)

	assert.Equal(t, "4", phase20Revision(t, dst, "phase20-session"),
		"an incomparable archive leaves the freshly computed revision alone")
	assert.Equal(t, "0", phase20Revision(t, dst, "phase20-legacy-orphan"),
		"an orphan carried over from a column-less archive lands on the default")

	ctx := context.Background()
	orphan, err := dst.GetAllMessages(ctx, "phase20-legacy-orphan")
	require.NoError(t, err)
	require.Len(t, orphan, 1)
	assert.Equal(t, "orphan answer", orphan[0].Content)
	matching, err := dst.GetAllMessages(ctx, "phase20-session")
	require.NoError(t, err)
	require.Len(t, matching, 1)
	assert.Equal(t, "reparsed answer", matching[0].Content)
}

// addPhase20LegacyArchiveSession appends one more session to an archive built
// by createPhase20CurrentArchiveWithoutTranscriptRevision, so the fixture can
// carry both a session that matches the fresh parse and one that is orphaned.
func addPhase20LegacyArchiveSession(t *testing.T, path, sessionID, content string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	defer func() { require.NoError(t, conn.Close()) }()

	_, err = conn.Exec(`
		INSERT INTO sessions (
			id, project, machine, agent, first_message, message_count
		) VALUES (?, 'project-a', 'local', 'claude', 'archived prompt', 1)`,
		sessionID,
	)
	require.NoError(t, err)
	_, err = conn.Exec(`
		INSERT INTO messages (
			session_id, ordinal, role, content, content_length, timestamp
		) VALUES (?, 0, 'assistant', ?, ?, ?)`,
		sessionID, content, len(content), tsZero,
	)
	require.NoError(t, err)
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
