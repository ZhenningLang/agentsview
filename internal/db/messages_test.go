package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertAndGetMessage_ThinkingText(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	sessionID := "thinking-test"
	insertSession(t, d, sessionID, "proj1")

	insertMessages(t, d, Message{
		SessionID:    sessionID,
		Ordinal:      0,
		Role:         "assistant",
		Content:      "the answer",
		ThinkingText: "I am pondering",
	})

	got, err := d.GetAllMessages(context.Background(), sessionID)
	require.NoError(t, err, "GetAllMessages")
	require.Len(t, got, 1)
	assert.Equal(t, "I am pondering", got[0].ThinkingText, "ThinkingText")
}

func TestDirectDBWriteEntrypointsSanitizeParserOutput(t *testing.T) {
	t.Run("upsert session", func(t *testing.T) {
		d := testDB(t)
		first := "first\x07message"
		started := "1500-01-01T00:00:00Z"
		require.NoError(t, d.UpsertSession(Session{
			ID: "direct-session", Project: "proj\x1bect",
			Machine: defaultMachine, Agent: defaultAgent,
			FirstMessage: &first, StartedAt: &started,
		}))

		got, err := d.GetSessionFull(context.Background(), "direct-session")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "project", got.Project)
		require.NotNil(t, got.FirstMessage)
		assert.Equal(t, "firstmessage", *got.FirstMessage)
		assert.Nil(t, got.StartedAt)
	})

	t.Run("insert messages and nested tools", func(t *testing.T) {
		d := testDB(t)
		insertSession(t, d, "direct-message", "proj")
		resultRaw := "result\x07body"
		eventRaw := "event\u0085body"
		require.NoError(t, d.InsertMessages([]Message{{
			SessionID: "direct-message", Ordinal: 0,
			Role: "wizard", Content: "answer\x1bbody",
			ContentLength: len("answer\x1bbody"),
			Model:         strings.Repeat("m", MaxModelLen+20),
			ContextTokens: -1, OutputTokens: MaxPlausibleTokens + 1,
			HasContextTokens: true, HasOutputTokens: true,
			Timestamp: "2999-01-01T00:00:00Z",
			ToolCalls: []ToolCall{{
				ToolName: "Re\x07ad", Category: "R\x1bead",
				ToolUseID: "tool\x00id", InputJSON: "{\"x\":1}\x07",
				SkillName:           "sk\u0085ill",
				ResultContent:       resultRaw,
				ResultContentLength: len(resultRaw),
				SubagentSessionID:   "agent\x1bchild",
				ResultEvents: []ToolResultEvent{{
					ToolUseID: "tool\x00id", AgentID: "agent\x07id",
					SubagentSessionID: "child\u0085id",
					Source:            "custom\x1b", Status: "complete\x07d",
					Content: eventRaw, ContentLength: len(eventRaw),
					Timestamp: "2999-01-01T00:00:00Z",
				}},
			}},
		}}))

		msgs, err := d.GetAllMessages(context.Background(), "direct-message")
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.Empty(t, msgs[0].Role)
		assert.Equal(t, "answerbody", msgs[0].Content)
		assert.Equal(t, len("answerbody"), msgs[0].ContentLength)
		assert.Len(t, msgs[0].Model, MaxModelLen)
		assert.Zero(t, msgs[0].ContextTokens)
		assert.Equal(t, MaxPlausibleTokens, msgs[0].OutputTokens)
		assert.Empty(t, msgs[0].Timestamp)
		require.Len(t, msgs[0].ToolCalls, 1)
		call := msgs[0].ToolCalls[0]
		assert.Equal(t, "Read", call.ToolName)
		assert.Equal(t, "Read", call.Category)
		assert.Equal(t, "toolid", call.ToolUseID)
		assert.Equal(t, "{\"x\":1}", call.InputJSON)
		assert.Equal(t, "skill", call.SkillName)
		assert.Equal(t, "resultbody", call.ResultContent)
		assert.Equal(t, len("resultbody"), call.ResultContentLength)
		assert.Equal(t, "agentchild", call.SubagentSessionID)
		require.Len(t, call.ResultEvents, 1)
		event := call.ResultEvents[0]
		assert.Equal(t, "toolid", event.ToolUseID)
		assert.Equal(t, "agentid", event.AgentID)
		assert.Equal(t, "childid", event.SubagentSessionID)
		assert.Equal(t, "custom", event.Source)
		assert.Equal(t, "completed", event.Status)
		assert.Equal(t, "eventbody", event.Content)
		assert.Equal(t, len("eventbody"), event.ContentLength)
		assert.Empty(t, event.Timestamp)
	})

	t.Run("atomic batch and usage event", func(t *testing.T) {
		d := testDB(t)
		result, err := d.WriteSessionBatchAtomic([]SessionBatchWrite{{
			Session: Session{
				ID: "direct-batch", Project: "proj\x07ect",
				Machine: defaultMachine, Agent: defaultAgent,
				MessageCount:         1,
				TotalOutputTokens:    9_000_000,
				HasTotalOutputTokens: true,
			},
			Messages: []Message{{
				SessionID: "direct-batch", Ordinal: 0,
				Role: "assistant", Content: "ok",
				OutputTokens: 9_000_000, HasOutputTokens: true,
			}},
			UsageEvents: []UsageEvent{{
				SessionID: "direct-batch", Source: "generation\x07",
				Model:       strings.Repeat("m", MaxModelLen+1),
				InputTokens: 9_000_000,
				OccurredAt:  "1800-01-01T00:00:00Z",
			}},
			TokenAggregateSource: TokenAggregateMessages,
			ReplaceMessages:      true,
		}})
		require.NoError(t, err)
		assert.Equal(t, 1, result.WrittenSessions)

		session, err := d.GetSessionFull(context.Background(), "direct-batch")
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.Equal(t, "project", session.Project)
		assert.Equal(t, MaxPlausibleTokens, session.TotalOutputTokens)
		events, err := d.GetUsageEvents(context.Background(), "direct-batch")
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "generation", events[0].Source)
		assert.Len(t, events[0].Model, MaxModelLen)
		assert.Equal(t, MaxPlausibleTokens, events[0].InputTokens)
		assert.Empty(t, events[0].OccurredAt)
	})
}

func TestSessionBatchTokenAggregateProvenanceIsExplicit(t *testing.T) {
	tests := []struct {
		name       string
		source     TokenAggregateSource
		wantOutput int
		wantPeak   int
	}{
		{
			name:       "authoritative summary survives row-value collision",
			source:     TokenAggregateSummary,
			wantOutput: 3_000_000,
			wantPeak:   3_000_000,
		},
		{
			name:       "message-derived aggregate follows sanitized rows",
			source:     TokenAggregateMessages,
			wantOutput: MaxPlausibleTokens,
			wantPeak:   MaxPlausibleTokens,
		},
		{
			name:       "unknown provenance preserves aggregate without guessing",
			source:     TokenAggregateUnknown,
			wantOutput: 3_000_000,
			wantPeak:   3_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDB(t)
			result, err := d.WriteSessionBatchAtomic([]SessionBatchWrite{{
				Session: Session{
					ID: "provenance", Project: "proj",
					Machine: defaultMachine, Agent: defaultAgent,
					MessageCount:         1,
					TotalOutputTokens:    3_000_000,
					PeakContextTokens:    3_000_000,
					HasTotalOutputTokens: true,
					HasPeakContextTokens: true,
				},
				Messages: []Message{{
					SessionID: "provenance", Ordinal: 0,
					Role: "assistant", Content: "answer",
					OutputTokens: 3_000_000, ContextTokens: 3_000_000,
					HasOutputTokens: true, HasContextTokens: true,
				}},
				TokenAggregateSource: tt.source,
				ReplaceMessages:      true,
			}})
			require.NoError(t, err)
			assert.Equal(t, 1, result.WrittenSessions)

			got, err := d.GetSessionFull(context.Background(), "provenance")
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantOutput, got.TotalOutputTokens)
			assert.Equal(t, tt.wantPeak, got.PeakContextTokens)
		})
	}
}

func TestSessionBatchSanitizesDirtySummarySourceWithoutReclassifyingAggregate(t *testing.T) {
	d := testDB(t)
	result, err := d.WriteSessionBatchAtomic([]SessionBatchWrite{{
		Session: Session{
			ID: "dirty-summary", Project: "proj",
			Machine: defaultMachine, Agent: "droid",
			MessageCount:         1,
			TotalOutputTokens:    3_000_000,
			PeakContextTokens:    3_500_000,
			HasTotalOutputTokens: true,
			HasPeakContextTokens: true,
		},
		Messages: []Message{{
			SessionID: "dirty-summary", Ordinal: 0,
			Role: "user", Content: "hello",
		}},
		UsageEvents: []UsageEvent{{
			SessionID: "dirty-summary", Source: "droid-settings\x07",
			InputTokens: 2_500_000, OutputTokens: 3_000_000,
			CacheCreationInputTokens: 500_000,
			CacheReadInputTokens:     500_000,
		}},
		TokenAggregateSource: TokenAggregateSummary,
		ReplaceMessages:      true,
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, result.WrittenSessions)

	got, err := d.GetSessionFull(context.Background(), "dirty-summary")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 3_000_000, got.TotalOutputTokens)
	assert.Equal(t, 3_500_000, got.PeakContextTokens)
	events, err := d.GetUsageEvents(context.Background(), "dirty-summary")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "droid-settings", events[0].Source)
	assert.Equal(t, 3_000_000, events[0].OutputTokens)
	assert.Equal(t, 2_500_000, events[0].InputTokens)
}

func TestWriteSessionBatchCommitsGoodRowsAndSkipsBadRows(t *testing.T) {
	d := testDB(t)

	require.NoError(t, d.Update(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO excluded_sessions (id) VALUES (?)",
			"excluded",
		)
		return err
	}), "seed excluded session")
	require.NoError(t, d.UpsertSession(Session{
		ID:      "trashed",
		Project: "proj",
		Machine: defaultMachine,
		Agent:   defaultAgent,
	}), "seed trashed session")
	require.NoError(t, d.SoftDeleteSession("trashed"), "soft delete session")

	health := 95
	grade := "A"
	result, err := d.WriteSessionBatch([]SessionBatchWrite{
		{
			Session: Session{
				ID:               "good",
				Project:          "proj",
				Machine:          defaultMachine,
				Agent:            defaultAgent,
				FirstMessage:     new(string("hello")),
				MessageCount:     2,
				UserMessageCount: 1,
			},
			Messages: []Message{
				userMsg("good", 0, "hello"),
				{
					SessionID:     "good",
					Ordinal:       1,
					Role:          "assistant",
					Content:       "answer",
					ContentLength: 6,
					ToolCalls: []ToolCall{{
						ToolName:  "Read",
						Category:  "Read",
						ToolUseID: "toolu_1",
					}},
				},
			},
			Signals: SessionSignalUpdate{
				Outcome:           "success",
				OutcomeConfidence: "high",
				EndedWithRole:     "assistant",
				HealthScore:       &health,
				HealthGrade:       &grade,
				HasToolCalls:      true,
			},
			DataVersion: CurrentDataVersion(),
		},
		{
			Session: Session{
				ID:               "bad",
				Project:          "proj",
				Machine:          defaultMachine,
				Agent:            defaultAgent,
				MessageCount:     1,
				UserMessageCount: 1,
			},
			Messages: []Message{
				userMsg("missing-session", 0, "broken"),
			},
			DataVersion: CurrentDataVersion(),
		},
		{
			Session: Session{
				ID:               "excluded",
				Project:          "proj",
				Machine:          defaultMachine,
				Agent:            defaultAgent,
				MessageCount:     1,
				UserMessageCount: 1,
			},
			Messages: []Message{
				userMsg("excluded", 0, "deleted"),
			},
			DataVersion: CurrentDataVersion(),
		},
		{
			Session: Session{
				ID:               "trashed",
				Project:          "proj",
				Machine:          defaultMachine,
				Agent:            defaultAgent,
				MessageCount:     1,
				UserMessageCount: 1,
			},
			Messages: []Message{
				userMsg("trashed", 0, "trashed"),
			},
			DataVersion: CurrentDataVersion(),
		},
	})
	require.NoError(t, err, "WriteSessionBatch")
	require.Equal(t, 1, result.WrittenSessions, "WrittenSessions")
	require.Equal(t, 2, result.WrittenMessages, "WrittenMessages")
	require.Equal(t, 1, result.FailedSessions, "FailedSessions")
	require.Equal(t, 2, result.ExcludedSessions, "ExcludedSessions")

	sess, err := d.GetSessionFull(context.Background(), "good")
	require.NoError(t, err, "GetSessionFull good")
	require.NotNil(t, sess, "good session not found")
	assert.Equal(t, CurrentDataVersion(), sess.DataVersion, "DataVersion")
	assert.Equal(t, "success", sess.Outcome, "Outcome")
	assert.Equal(t, "high", sess.OutcomeConfidence, "OutcomeConfidence")
	assert.True(t, sess.HasToolCalls, "HasToolCalls")
	trashed, err := d.GetSessionFull(context.Background(), "trashed")
	require.NoError(t, err, "GetSessionFull trashed")
	require.NotNil(t, trashed, "trashed session was not preserved in trash")
	assert.NotNil(t, trashed.DeletedAt, "trashed session was not preserved in trash")

	msgs, err := d.GetAllMessages(context.Background(), "good")
	require.NoError(t, err, "GetAllMessages good")
	require.Len(t, msgs, 2)
	require.Len(t, msgs[1].ToolCalls, 1, "assistant tool calls")

	bad, err := d.GetSessionFull(context.Background(), "bad")
	require.NoError(t, err, "GetSessionFull bad")
	assert.Nil(t, bad, "bad session should have rolled back")
	excluded, err := d.GetSessionFull(context.Background(), "excluded")
	require.NoError(t, err, "GetSessionFull excluded")
	assert.Nil(t, excluded, "excluded session should not be written")
}

func TestWriteSessionSnapshotReconcilesStaleRowsAtomically(t *testing.T) {
	t.Run("deletes stale active rows but preserves trash", func(t *testing.T) {
		d := testDB(t)
		insertSession(t, d, "stale", "proj")
		insertSession(t, d, "trashed-stale", "proj")
		insertSession(t, d, "trashed-parser-excluded", "proj")
		require.NoError(t, d.SoftDeleteSession("trashed-stale"))
		require.NoError(t, d.SoftDeleteSession("trashed-parser-excluded"))

		result, err := d.WriteSessionSnapshot(
			[]SessionBatchWrite{{
				Session: Session{
					ID: "current", Project: "proj",
					Machine: defaultMachine, Agent: defaultAgent,
					MessageCount: 1, UserMessageCount: 1,
				},
				Messages:        []Message{userMsg("current", 0, "current")},
				DataVersion:     CurrentDataVersion(),
				ReplaceMessages: true,
			}},
			[]string{"stale", "trashed-stale"},
			[]string{"trashed-parser-excluded"},
		)
		require.NoError(t, err)
		assert.Equal(t, 1, result.WrittenSessions)

		stale, err := d.GetSessionFull(context.Background(), "stale")
		require.NoError(t, err)
		assert.Nil(t, stale)
		trashed, err := d.GetSessionFull(context.Background(), "trashed-stale")
		require.NoError(t, err)
		require.NotNil(t, trashed)
		assert.NotNil(t, trashed.DeletedAt)
		parserExcludedTrash, err := d.GetSessionFull(
			context.Background(), "trashed-parser-excluded",
		)
		require.NoError(t, err)
		require.NotNil(t, parserExcludedTrash)
		assert.NotNil(t, parserExcludedTrash.DeletedAt)
	})

	t.Run("write failure preserves stale rows", func(t *testing.T) {
		d := testDB(t)
		insertSession(t, d, "stale", "proj")
		insertSession(t, d, "parser-excluded", "proj")

		_, err := d.WriteSessionSnapshot(
			[]SessionBatchWrite{{
				Session: Session{
					ID: "current", Project: "proj",
					Machine: defaultMachine, Agent: defaultAgent,
					MessageCount: 1, UserMessageCount: 1,
				},
				Messages: []Message{userMsg("missing-session", 0, "broken")},
			}},
			[]string{"stale"}, []string{"parser-excluded"},
		)
		require.Error(t, err)

		stale, getErr := d.GetSessionFull(context.Background(), "stale")
		require.NoError(t, getErr)
		assert.NotNil(t, stale)
		excluded, getErr := d.GetSessionFull(
			context.Background(), "parser-excluded",
		)
		require.NoError(t, getErr)
		assert.NotNil(t, excluded)
		current, getErr := d.GetSessionFull(context.Background(), "current")
		require.NoError(t, getErr)
		assert.Nil(t, current)
	})
}

func TestMigration_ThinkingTextColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create a DB with the current schema then drop the
	// thinking_text column to simulate a pre-migration DB.
	d, err := Open(path)
	require.NoError(t, err, "initial open")
	insertSession(t, d, "s1", "proj")
	insertMessages(t, d,
		userMsg("s1", 0, "hello"),
		Message{
			SessionID:    "s1",
			Ordinal:      1,
			Role:         "assistant",
			Content:      "answer",
			ThinkingText: "pre-migration thought",
		},
	)
	d.Close()

	// Remove thinking_text via ALTER TABLE DROP COLUMN
	// (SQLite 3.35+) to simulate a legacy schema.
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err, "raw open")
	_, err = conn.Exec(
		`ALTER TABLE messages DROP COLUMN thinking_text`,
	)
	require.NoError(t, err, "drop thinking_text column")

	// Verify column is gone.
	var count int
	err = conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('messages')` +
			` WHERE name = 'thinking_text'`,
	).Scan(&count)
	require.NoError(t, err, "verify column removed")
	require.Zero(t, count, "expected thinking_text column to be absent")

	// Insert a legacy row with an explicit column list that
	// cannot reference thinking_text (column doesn't exist yet).
	_, err = conn.Exec(`
		INSERT INTO messages (
			session_id, ordinal, role, content, timestamp,
			has_thinking, has_tool_use, content_length,
			is_system, model, token_usage,
			context_tokens, output_tokens,
			has_context_tokens, has_output_tokens,
			claude_message_id, claude_request_id,
			source_type, source_subtype, source_uuid,
			source_parent_uuid, is_sidechain,
			is_compact_boundary
		) VALUES (
			's1', 2, 'user', 'legacy', '',
			0, 0, 6,
			0, '', '',
			0, 0,
			0, 0,
			'', '',
			'', '', '',
			'', 0,
			0
		)`)
	require.NoError(t, err, "insert legacy row")
	conn.Close()

	// Reopen with Open() — migration should add the column.
	d2, err := Open(path)
	require.NoError(t, err, "reopen after migration")
	defer d2.Close()

	// Verify column exists.
	err = d2.getReader().QueryRow(
		`SELECT count(*) FROM pragma_table_info('messages')` +
			` WHERE name = 'thinking_text'`,
	).Scan(&count)
	require.NoError(t, err, "verify column added")
	require.Equal(t, 1, count, "expected thinking_text column after migration")

	// Verify all rows survive and the legacy row defaults to "".
	msgs, err := d2.GetAllMessages(context.Background(), "s1")
	require.NoError(t, err, "get messages")
	require.Len(t, msgs, 3)
	for _, m := range msgs {
		assert.Empty(t, m.ThinkingText, "ord=%d ThinkingText", m.Ordinal)
	}

	// Insert a new message with ThinkingText and verify round-trip.
	insertMessages(t, d2, Message{
		SessionID:    "s1",
		Ordinal:      3,
		Role:         "assistant",
		Content:      "post-migration answer",
		ThinkingText: "x",
	})
	msgs, err = d2.GetAllMessages(context.Background(), "s1")
	require.NoError(t, err, "get messages after insert")
	require.Len(t, msgs, 4)
	assert.Equal(t, "x", msgs[3].ThinkingText, "ThinkingText")
}

// TestReplaceSessionMessages_LargeSession is a perf regression test
// for the FTS5 trigger-cascade hang fixed alongside the bulk-delete
// path in ReplaceSessionMessages. Before the fix, deleting a session
// whose messages contained multi-MB content blobs would fan out into
// per-row FTS 'delete' commands, each tokenizing the old content, and
// could stall the writer for minutes on real data. The bulk path
// makes the cost effectively flat regardless of blob size, so this
// test puts a hard 10s ceiling on the full replace cycle for a
// session that mixes 1000 small messages with one ~5MB content blob.
// Skipped under -short since a clean run is well under 1s but CI
// scheduling jitter can push slow paths up.
func TestReplaceSessionMessages_LargeSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf test in -short mode")
	}
	t.Parallel()
	d := testDB(t)
	requireFTS(t, d)
	const sessionID = "perf-large"
	insertSession(t, d, sessionID, "proj")

	const n = 1000
	msgs := make([]Message, 0, n)
	for i := range n {
		msgs = append(msgs, userMsg(sessionID, i, "small"))
	}
	// One ~5MB content blob in the middle of the stream — the
	// pathological case that blew up the per-row FTS delete path.
	big := strings.Repeat("x ", 5*1024*1024/2)
	msgs[n/2] = Message{
		SessionID:     sessionID,
		Ordinal:       n / 2,
		Role:          "assistant",
		Content:       big,
		ContentLength: len(big),
		Timestamp:     tsZero,
	}
	insertMessages(t, d, msgs...)

	// Replace with a different small set so the delete path has to
	// remove all 1000 rows including the 5MB blob.
	repl := make([]Message, 0, 10)
	for i := range 10 {
		repl = append(repl, userMsg(sessionID, i, "after"))
	}
	start := time.Now()
	require.NoError(t, d.ReplaceSessionMessages(sessionID, repl),
		"ReplaceSessionMessages")
	elapsed := time.Since(start)
	require.LessOrEqual(t, elapsed, 10*time.Second,
		"ReplaceSessionMessages took %s, want < 10s (per-row FTS trigger regression?)",
		elapsed.Round(time.Millisecond))

	got, err := d.GetAllMessages(context.Background(), sessionID)
	require.NoError(t, err, "GetAllMessages after replace")
	require.Len(t, got, len(repl), "after replace")

	// Verify the FTS index was actually scrubbed: count rows in
	// messages_fts that join back to the (now-deleted) original
	// session rows. Should be zero. If the messages_ad trigger
	// restoration failed silently or the bulk-delete INSERT...SELECT
	// got skipped, stale tokens would still resolve here.
	var leaked int
	err = d.getReader().QueryRow(
		`SELECT count(*) FROM messages_fts
		 WHERE messages_fts MATCH 'xxx'`,
	).Scan(&leaked)
	require.NoError(t, err, "fts leak check")
	assert.Zero(t, leaked,
		"FTS still contains rows matching 'xxx' from deleted blob")
}
