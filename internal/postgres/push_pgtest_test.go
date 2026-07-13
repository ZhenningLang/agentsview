//go:build pgtest

package postgres

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

// TestPushSystemFingerprintCollisionRegression verifies that the fast-path
// in pushMessages correctly detects a change when the is_system flags are
// reclassified between two ordinal sets that previously collided under the
// two-component (SUM, SUM-of-squares) fingerprint: {0,4,5} and {1,2,6}
// both produce sum=9, sumSq=41.
//
// Steps:
//  1. Push a session with 7 messages where ordinals {0,4,5} are system.
//  2. Without changing content lengths, reclassify to {1,2,6} as system.
//  3. Push again with full=false.
//  4. Confirm PG now reflects the updated is_system values.
func TestPushSystemFingerprintCollisionRegression(t *testing.T) {
	pgURL := testPGURL(t)

	const schema = "agentsview_push_sysfingerprint_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err, "Open")
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err, "drop schema")
	require.NoError(t, EnsureSchema(ctx, pg, schema), "EnsureSchema")

	// Local SQLite DB.
	localDB, err := db.Open(
		filepath.Join(t.TempDir(), "local.db"),
	)
	require.NoError(t, err, "db.Open")
	defer localDB.Close()

	sync := &Sync{
		pg:      pg,
		local:   localDB,
		machine: "test-machine",
		schema:  schema,
		// Mark schema done so Push skips EnsureSchema.
		schemaDone: true,
	}

	const sessID = "fp-collision-001"
	sess := db.Session{
		ID:           sessID,
		Project:      "test-proj",
		Machine:      "test-machine",
		Agent:        "claude",
		MessageCount: 7,
		CreatedAt:    "2026-01-01T00:00:00Z",
	}
	require.NoError(t, localDB.UpsertSession(sess), "UpsertSession")

	// First set: system ordinals {0,4,5}.
	firstSet := map[int]bool{0: true, 4: true, 5: true}
	msgs := make([]db.Message, 7)
	for i := range 7 {
		msgs[i] = db.Message{
			SessionID:     sessID,
			Ordinal:       i,
			Role:          "user",
			Content:       "x",
			ContentLength: 1,
			IsSystem:      firstSet[i],
		}
	}
	require.NoError(t, localDB.InsertMessages(msgs), "InsertMessages (first set)")

	// First push.
	_, err = sync.Push(ctx, false, nil)
	require.NoError(t, err, "Push (first)")

	// Verify PG reflects system ordinals {0,4,5}.
	checkIsSystem(t, pg, sessID, firstSet, 7)

	// Switch to {1,2,6} — same sum(ordinal)=9, same sum(ordinal²)=41,
	// but the string fingerprint differs ("0,4,5" vs "1,2,6").
	// Replace local messages with updated is_system flags.
	secondSet := map[int]bool{1: true, 2: true, 6: true}
	for i := range 7 {
		msgs[i].IsSystem = secondSet[i]
	}
	require.NoError(t, localDB.ReplaceSessionMessages(sessID, msgs),
		"ReplaceSessionMessages (second set)")

	// Force re-evaluation by clearing both the watermark and the cached
	// session-level boundary fingerprints. The session-level fingerprint
	// does not include is_system flags (only metadata like MessageCount),
	// so the boundary cache must be cleared for the incremental push to
	// reach pushMessages and compare the message-level string fingerprint.
	require.NoError(t, localDB.SetSyncState("last_push_at", ""),
		"clearing last_push_at")
	require.NoError(t, localDB.SetSyncState(lastPushBoundaryStateKey, ""),
		"clearing boundary state")

	// Second push — must NOT skip due to fingerprint match.
	_, err = sync.Push(ctx, false, nil)
	require.NoError(t, err, "Push (second)")

	// Verify PG now reflects updated system ordinals {1,2,6}.
	checkIsSystem(t, pg, sessID, secondSet, 7)
}

func TestPushNestedToolFingerprintRepairsDirtyPGAndThenNoOps(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_push_tool_fingerprint_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err)
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err)
	require.NoError(t, EnsureSchema(ctx, pg, schema))

	localDB, err := db.Open(filepath.Join(t.TempDir(), "local.db"))
	require.NoError(t, err)
	defer localDB.Close()
	syncer := &Sync{
		pg: pg, local: localDB, machine: "test-machine",
		schema: schema, schemaDone: true,
	}

	const sessionID = "tool-fingerprint-001"
	started := "2026-01-01T00:00:00Z"
	require.NoError(t, localDB.UpsertSession(db.Session{
		ID: sessionID, Project: "p", Machine: "test-machine",
		Agent: "claude", CreatedAt: started, StartedAt: &started,
		MessageCount: 1,
	}))
	require.NoError(t, localDB.InsertMessages([]db.Message{{
		SessionID: sessionID, Ordinal: 0,
		Role: "assistant", Content: "answer", ContentLength: 6,
		ToolCalls: []db.ToolCall{{
			ToolName: "Read", Category: "Read", ToolUseID: "tool-id",
			InputJSON:     `{"path":"README.md"}`,
			ResultContent: "same", ResultContentLength: 4,
			ResultEvents: []db.ToolResultEvent{{
				ToolUseID: "tool-id", Source: "custom",
				Status: "completed", Content: "equal", ContentLength: 5,
				Timestamp: started, EventIndex: 0,
			}},
		}},
	}}))

	first, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.MessagesPushed)

	_, err = pg.Exec(`
		UPDATE tool_calls SET tool_name = 'R' || chr(7) || 'ead'
		WHERE session_id = $1;
		UPDATE tool_result_events SET source = 'custo' || chr(7) || 'm'
		WHERE session_id = $1`, sessionID)
	require.NoError(t, err)
	require.NoError(t, localDB.SetSyncState("last_push_at", ""))
	require.NoError(t, localDB.SetSyncState(lastPushBoundaryStateKey, ""))

	second, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, second.MessagesPushed)
	var toolName, source string
	require.NoError(t, pg.QueryRow(`
		SELECT tool_name FROM tool_calls WHERE session_id = $1`,
		sessionID).Scan(&toolName))
	require.NoError(t, pg.QueryRow(`
		SELECT source FROM tool_result_events WHERE session_id = $1`,
		sessionID).Scan(&source))
	assert.Equal(t, "Read", toolName)
	assert.Equal(t, "custom", source)

	require.NoError(t, localDB.SetSyncState("last_push_at", ""))
	require.NoError(t, localDB.SetSyncState(lastPushBoundaryStateKey, ""))
	third, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Zero(t, third.MessagesPushed)
}

func TestPushExactMessageAndUsageFingerprintRepairsDirtyPGAndThenNoOps(t *testing.T) {
	pgURL := testPGURL(t)
	const schema = "agentsview_push_message_fingerprint_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err)
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err)
	require.NoError(t, EnsureSchema(ctx, pg, schema))

	localDB, err := db.Open(filepath.Join(t.TempDir(), "local.db"))
	require.NoError(t, err)
	defer localDB.Close()
	syncer := &Sync{
		pg: pg, local: localDB, machine: "test-machine",
		schema: schema, schemaDone: true,
	}

	const sessionID = "message-fingerprint-001"
	started := "2026-01-01T00:00:00.123456789Z"
	require.NoError(t, localDB.UpsertSession(db.Session{
		ID: sessionID, Project: "p", Machine: "test-machine",
		Agent: "claude", CreatedAt: started, StartedAt: &started,
		MessageCount: 1,
	}))
	require.NoError(t, localDB.InsertMessages([]db.Message{{
		SessionID: sessionID, Ordinal: 0,
		Role: "assistant", Content: "answer aaaa", ThinkingText: "plan",
		Timestamp: started, HasThinking: true, HasToolUse: false,
		ContentLength: len("answer aaaa") + len("plan"), Model: "model",
	}}))
	require.NoError(t, localDB.ReplaceSessionUsageEvents(sessionID,
		[]db.UsageEvent{{
			SessionID: sessionID, Source: "generation", Model: "model",
			InputTokens: 1, OutputTokens: 2,
			OccurredAt: started, DedupKey: "generation",
		}}))
	first, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.MessagesPushed)

	_, err = pg.Exec(`
		UPDATE messages SET
			role = 'assistant' || chr(7),
			content = 'answer bbbb',
			thinking_text = 'idea',
			has_thinking = FALSE,
			timestamp = '2026-01-01T00:00:01Z'
		WHERE session_id = $1;
		UPDATE usage_events SET
			occurred_at = '2026-01-01T00:00:01Z'
		WHERE session_id = $1`, sessionID)
	require.NoError(t, err)
	require.NoError(t, localDB.SetSyncState("last_push_at", ""))
	require.NoError(t, localDB.SetSyncState(lastPushBoundaryStateKey, ""))

	second, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, second.MessagesPushed)
	var role, content, thinking string
	var hasThinking bool
	var messageTimestamp, usageTimestamp time.Time
	require.NoError(t, pg.QueryRow(`
		SELECT role, content, thinking_text, has_thinking, timestamp
		FROM messages WHERE session_id = $1`, sessionID).Scan(
		&role, &content, &thinking, &hasThinking, &messageTimestamp,
	))
	require.NoError(t, pg.QueryRow(`
		SELECT occurred_at FROM usage_events WHERE session_id = $1`,
		sessionID).Scan(&usageTimestamp))
	assert.Equal(t, "assistant", role)
	assert.Equal(t, "answer aaaa", content)
	assert.Equal(t, "plan", thinking)
	assert.True(t, hasThinking)
	assert.Equal(t, started[:26]+"Z", FormatISO8601(messageTimestamp))
	assert.Equal(t, started[:26]+"Z", FormatISO8601(usageTimestamp))

	require.NoError(t, localDB.SetSyncState("last_push_at", ""))
	require.NoError(t, localDB.SetSyncState(lastPushBoundaryStateKey, ""))
	third, err := syncer.Push(ctx, false, nil)
	require.NoError(t, err)
	assert.Zero(t, third.MessagesPushed)
}

// TestPushSessionTerminationStatus verifies that pushSession round-trips
// the termination_status column to PG: a non-nil value writes the string,
// and a subsequent push with nil clears the column back to NULL via the
// ON CONFLICT DO UPDATE path.
func TestPushSessionTerminationStatus(t *testing.T) {
	pgURL := testPGURL(t)

	const schema = "agentsview_push_termstatus_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err, "Open")
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err, "drop schema")
	require.NoError(t, EnsureSchema(ctx, pg, schema), "EnsureSchema")

	localDB, err := db.Open(filepath.Join(t.TempDir(), "local.db"))
	require.NoError(t, err, "db.Open")
	defer localDB.Close()

	sync := &Sync{
		pg:         pg,
		local:      localDB,
		machine:    "test-machine",
		schema:     schema,
		schemaDone: true,
	}

	pending := "tool_call_pending"
	sess := db.Session{
		ID:               "term-test-1",
		Project:          "p",
		Machine:          "test-machine",
		Agent:            "claude",
		MessageCount:     1,
		UserMessageCount: 1,
		// CreatedAt must be parseable by ParseSQLiteTimestamp;
		// PG's NOT NULL on created_at would otherwise reject NULL.
		CreatedAt:         "2024-01-01T00:00:00Z",
		TerminationStatus: &pending,
	}

	pushOnce := func(s db.Session) {
		t.Helper()
		tx, err := pg.BeginTx(ctx, nil)
		require.NoError(t, err, "BeginTx")
		if err := sync.pushSession(ctx, tx, s); err != nil {
			_ = tx.Rollback()
			t.Fatalf("pushSession: %v", err)
		}
		require.NoError(t, tx.Commit(), "Commit")
	}

	pushOnce(sess)

	var got *string
	require.NoError(t, pg.QueryRow(
		`SELECT termination_status FROM sessions WHERE id = $1`,
		sess.ID,
	).Scan(&got), "read back")
	require.NotNil(t, got)
	assert.Equal(t, "tool_call_pending", *got)

	// Update to NULL and verify ON CONFLICT clears it.
	sess.TerminationStatus = nil
	pushOnce(sess)

	require.NoError(t, pg.QueryRow(
		`SELECT termination_status FROM sessions WHERE id = $1`,
		sess.ID,
	).Scan(&got), "read back 2")
	assert.Nil(t, got)
}

func TestPushPropagatesLLMFieldsOnInsertAndUpdate(t *testing.T) {
	pgURL := testPGURL(t)

	const schema = "agentsview_push_llm_fields_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err, "Open")
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err, "drop schema")
	require.NoError(t, EnsureSchema(ctx, pg, schema), "EnsureSchema")

	localDB, err := db.Open(filepath.Join(t.TempDir(), "local.db"))
	require.NoError(t, err, "db.Open")
	defer localDB.Close()

	sync := &Sync{
		pg:         pg,
		local:      localDB,
		machine:    "test-machine",
		schema:     schema,
		schemaDone: true,
	}

	started := "2026-01-01T00:00:00Z"
	sess := db.Session{
		ID:               "llm-push-001",
		Project:          "p",
		Machine:          "test-machine",
		Agent:            "claude",
		StartedAt:        &started,
		CreatedAt:        started,
		MessageCount:     1,
		UserMessageCount: 1,
	}
	require.NoError(t, localDB.UpsertSession(sess), "UpsertSession")
	setLocalPGLLMFields(t, localDB, sess.ID, pgLLMFixtureValues{
		Title:            "First PG LLM title",
		Summary:          "First summary",
		Keywords:         "auth,token",
		Embedding:        []byte{0x00, 0x00, 0x80, 0x3f},
		EmbeddingDim:     1,
		EnrichedAt:       "2026-01-01T01:00:00Z",
		EnrichedMsgCount: 1,
		Model:            "deepseek-chat",
		Status:           "ok",
		Error:            "",
		LocalModifiedAt:  "2026-01-01T01:00:00Z",
	})

	first, err := sync.Push(ctx, false, nil)
	require.NoError(t, err, "first Push")
	assert.Equal(t, 1, first.SessionsPushed)
	assertPGLLMFields(t, pg, sess.ID, pgLLMFixtureValues{
		Title:            "First PG LLM title",
		Summary:          "First summary",
		Keywords:         "auth,token",
		Embedding:        []byte{0x00, 0x00, 0x80, 0x3f},
		EmbeddingDim:     1,
		EnrichedAt:       "2026-01-01T01:00:00Z",
		EnrichedMsgCount: 1,
		Model:            "deepseek-chat",
		Status:           "ok",
		Error:            "",
	})

	time.Sleep(time.Millisecond)
	setLocalPGLLMFields(t, localDB, sess.ID, pgLLMFixtureValues{
		Title:            "Updated PG LLM title",
		Summary:          "Updated summary",
		Keywords:         "semantic,search",
		Embedding:        []byte{0x00, 0x00, 0x00, 0x40},
		EmbeddingDim:     1,
		EnrichedAt:       "2026-01-01T02:00:00Z",
		EnrichedMsgCount: 2,
		Model:            "deepseek-chat",
		Status:           "error",
		Error:            "rate limited",
		LocalModifiedAt:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	})
	second, err := sync.Push(ctx, false, nil)
	require.NoError(t, err, "second Push")
	assert.Equal(t, 1, second.SessionsPushed)
	assertPGLLMFields(t, pg, sess.ID, pgLLMFixtureValues{
		Title:            "Updated PG LLM title",
		Summary:          "Updated summary",
		Keywords:         "semantic,search",
		Embedding:        []byte{0x00, 0x00, 0x00, 0x40},
		EmbeddingDim:     1,
		EnrichedAt:       "2026-01-01T02:00:00Z",
		EnrichedMsgCount: 2,
		Model:            "deepseek-chat",
		Status:           "error",
		Error:            "rate limited",
	})
}

type pgLLMFixtureValues struct {
	Title            string
	Summary          string
	Keywords         string
	Embedding        []byte
	EmbeddingDim     int
	EnrichedAt       string
	EnrichedMsgCount int
	Model            string
	Status           string
	Error            string
	LocalModifiedAt  string
}

func setLocalPGLLMFields(t *testing.T, local *db.DB, sessionID string, values pgLLMFixtureValues) {
	t.Helper()
	raw, err := sql.Open("sqlite3", local.Path())
	require.NoError(t, err, "open local sqlite")
	defer raw.Close()
	_, err = raw.Exec(`
		UPDATE sessions SET
			llm_title = ?,
			llm_summary = ?,
			llm_keywords = ?,
			llm_embedding = ?,
			llm_embedding_dim = ?,
			enriched_at = ?,
			enriched_msg_count = ?,
			enrich_model = ?,
			enrich_status = ?,
			enrich_error = ?,
			local_modified_at = ?
		WHERE id = ?`,
		values.Title,
		values.Summary,
		values.Keywords,
		values.Embedding,
		values.EmbeddingDim,
		values.EnrichedAt,
		values.EnrichedMsgCount,
		values.Model,
		values.Status,
		values.Error,
		values.LocalModifiedAt,
		sessionID,
	)
	require.NoError(t, err, "updating local LLM fields")
}

func assertPGLLMFields(t *testing.T, pg *sql.DB, sessionID string, want pgLLMFixtureValues) {
	t.Helper()
	var got pgLLMFixtureValues
	require.NoError(t, pg.QueryRow(`
		SELECT llm_title, llm_summary, llm_keywords, llm_embedding,
			llm_embedding_dim, enriched_at, enriched_msg_count,
			enrich_model, enrich_status, enrich_error
		FROM sessions WHERE id = $1`, sessionID).Scan(
		&got.Title,
		&got.Summary,
		&got.Keywords,
		&got.Embedding,
		&got.EmbeddingDim,
		&got.EnrichedAt,
		&got.EnrichedMsgCount,
		&got.Model,
		&got.Status,
		&got.Error,
	), "read PG LLM fields")
	assert.Equal(t, want.Title, got.Title)
	assert.Equal(t, want.Summary, got.Summary)
	assert.Equal(t, want.Keywords, got.Keywords)
	assert.Equal(t, want.Embedding, got.Embedding)
	assert.Equal(t, want.EmbeddingDim, got.EmbeddingDim)
	assert.Equal(t, want.EnrichedAt, got.EnrichedAt)
	assert.Equal(t, want.EnrichedMsgCount, got.EnrichedMsgCount)
	assert.Equal(t, want.Model, got.Model)
	assert.Equal(t, want.Status, got.Status)
	assert.Equal(t, want.Error, got.Error)
}

// TestPushSyncsUsageEventsForZeroMessageSession verifies that a session
// carrying token/cost accounting as a usage_event but no transcript
// messages still has its usage_event pushed to PG. This is the shape of a
// hermes state.db-only session: parseHermesStateSession emits a single
// usage_event (model + tokens) with MessageCount 0. The session row (and
// its aggregate token columns) pushes via pushSession, but pushMessages
// must not skip usage_event syncing just because the message count is 0 --
// otherwise the dashboard shows tokens with a $0 cost.
func TestPushSyncsUsageEventsForZeroMessageSession(t *testing.T) {
	pgURL := testPGURL(t)

	const schema = "agentsview_push_zeromsg_usage_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err, "Open")
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err, "drop schema")
	require.NoError(t, EnsureSchema(ctx, pg, schema), "EnsureSchema")

	localDB, err := db.Open(filepath.Join(t.TempDir(), "local.db"))
	require.NoError(t, err, "db.Open")
	defer localDB.Close()

	sync := &Sync{
		pg:         pg,
		local:      localDB,
		machine:    "test-machine",
		schema:     schema,
		schemaDone: true,
	}

	const sessID = "hermes:zero-msg-001"
	started := "2026-05-26T10:00:00Z"
	sess := db.Session{
		ID:                   sessID,
		Project:              "hermes-proj",
		Machine:              "test-machine",
		Agent:                "hermes",
		MessageCount:         0,
		StartedAt:            &started,
		CreatedAt:            started,
		TotalOutputTokens:    500000,
		HasTotalOutputTokens: true,
	}
	require.NoError(t, localDB.UpsertSession(sess), "UpsertSession")

	// gpt-5.5 usage event with NULL cost so it is priced from the catalog.
	require.NoError(t, localDB.ReplaceSessionUsageEvents(sessID, []db.UsageEvent{{
		SessionID:    sessID,
		Source:       "session",
		Model:        "gpt-5.5",
		InputTokens:  1000000,
		OutputTokens: 500000,
		CostUSD:      nil,
		OccurredAt:   started,
		DedupKey:     "session:" + sessID,
	}}), "ReplaceSessionUsageEvents")

	_, err = sync.Push(ctx, false, nil)
	require.NoError(t, err, "Push")

	// The usage_event must reach PG even though the session has no messages.
	var pgUsageCount int
	require.NoError(t, pg.QueryRow(
		`SELECT COUNT(*) FROM usage_events WHERE session_id = $1`,
		sessID,
	).Scan(&pgUsageCount), "count pg usage_events")
	assert.Equal(t, 1, pgUsageCount,
		"usage_event for a zero-message session was not pushed")

	// And the read side prices it from the gpt-5.5 catalog rate:
	// input 5/Mtok, output 30/Mtok -> 1.0*5 + 0.5*30 = 20.
	store, err := NewStore(pgURL, schema, true)
	require.NoError(t, err, "NewStore")
	defer store.Close()

	result, err := store.GetDailyUsage(ctx, db.UsageFilter{
		From:     "2026-05-26",
		To:       "2026-05-26",
		Timezone: "UTC",
	})
	require.NoError(t, err, "GetDailyUsage")
	assert.InDelta(t, 20.0, result.Totals.TotalCost, 1e-9,
		"gpt-5.5 usage should be priced from the catalog")
}

// checkIsSystem asserts that PG contains exactly wantTotal rows for the
// session with ordinals 0..wantTotal-1, and that each row's is_system
// matches wantSystem. Tracking the exact ordinal set prevents false
// positives from wrong-but-equal-count row sets.
func checkIsSystem(
	t *testing.T,
	pg *sql.DB,
	sessID string,
	wantSystem map[int]bool,
	wantTotal int,
) {
	t.Helper()
	rows, err := pg.Query(
		`SELECT ordinal, is_system FROM messages
		 WHERE session_id = $1 ORDER BY ordinal`,
		sessID,
	)
	require.NoError(t, err, "querying PG messages")
	defer rows.Close()
	seen := make(map[int]bool, wantTotal)
	for rows.Next() {
		var ordinal int
		var isSystem bool
		require.NoError(t, rows.Scan(&ordinal, &isSystem), "scanning row")
		seen[ordinal] = true
		want := wantSystem[ordinal]
		assert.Equal(t, want, isSystem, "ordinal %d is_system", ordinal)
	}
	require.NoError(t, rows.Err(), "rows error")
	assert.Len(t, seen, wantTotal,
		"PG has %d message rows for session %s, want %d",
		len(seen), sessID, wantTotal)
	// Verify every expected ordinal was present (no gaps or substitutions).
	for i := range wantTotal {
		assert.True(t, seen[i], "ordinal %d missing from PG messages", i)
	}
}
