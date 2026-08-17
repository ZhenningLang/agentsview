//go:build pgtest

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPhase23AnalyticsModelFilterPostgresSkills(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)
	store, err := NewStore(pgURL, testSchema, true)
	require.NoError(t, err)
	defer store.Close()
	seedPhase23PGSkills(t, store)

	ctx := context.Background()
	resp, err := store.GetAnalyticsSkills(ctx, db.AnalyticsFilter{
		From: "2024-06-01", To: "2024-06-01", Timezone: "UTC", Model: "model-a",
	}, "day")
	require.NoError(t, err)
	require.Len(t, resp.BySkill, 1)
	assert.Equal(t, "assist-learn", resp.BySkill[0].SkillName)
	assert.Equal(t, 2, resp.TotalSkillCalls)
	assert.Equal(t, "2024-06-01T00:05:00Z", resp.BySkill[0].LastUsedAt)
	require.Len(t, resp.Trend, 1)
	assert.Equal(t, 2, resp.Trend[0].BySkill["assist-learn"])

	empty, err := store.GetAnalyticsSkills(ctx, db.AnalyticsFilter{
		From: "2024-06-01", To: "2024-06-01", Timezone: "UTC", Model: "missing-model",
	}, "month")
	require.NoError(t, err)
	assert.Empty(t, empty.BySkill)
	assert.Empty(t, empty.Trend)
}

func TestPhase23AnalyticsModelFilterPostgresSkillsNullTimestamp(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)
	store, err := NewStore(pgURL, testSchema, true)
	require.NoError(t, err)
	defer store.Close()
	seedPhase23PGNullTimestampSkill(t, store)

	resp, err := store.GetAnalyticsSkills(context.Background(), db.AnalyticsFilter{
		From: "2024-06-01", To: "2024-06-01", Timezone: "UTC", Model: "model-a",
	}, "day")
	require.NoError(t, err)
	require.Len(t, resp.BySkill, 1)
	assert.Equal(t, "assist-learn", resp.BySkill[0].SkillName)
	assert.Equal(t, "2024-06-01T08:00:00Z", resp.BySkill[0].LastUsedAt)
}

func seedPhase23PGSkills(t *testing.T, store *Store) {
	t.Helper()
	_, err := store.DB().Exec(`
		INSERT INTO sessions (
			id, machine, project, agent, first_message, started_at, ended_at,
			created_at, message_count, user_message_count, relationship_type
		) VALUES
			('pg-p23-long', 'local', 'p23', 'claude', 'long',
			 '2024-05-31T23:00:00Z'::timestamptz, '2024-06-01T00:10:00Z'::timestamptz,
			 '2024-05-31T23:00:00Z'::timestamptz, 2, 1, 'root'),
			('pg-p23-other', 'local', 'p23', 'claude', 'other',
			 '2024-06-01T01:00:00Z'::timestamptz, '2024-06-01T01:10:00Z'::timestamptz,
			 '2024-06-01T01:00:00Z'::timestamptz, 1, 0, 'root')`)
	require.NoError(t, err)
	_, err = store.DB().Exec(`
		INSERT INTO messages (session_id, ordinal, role, content, timestamp, model, has_tool_use)
		VALUES
			('pg-p23-long', 0, 'user', 'prompt', '2024-05-31T23:59:00Z'::timestamptz, '', FALSE),
			('pg-p23-long', 1, 'assistant', 'answer', '2024-06-01T00:05:00Z'::timestamptz, 'model-a', TRUE),
			('pg-p23-other', 0, 'assistant', 'other', '2024-06-01T01:05:00Z'::timestamptz, 'model-b', TRUE)`)
	require.NoError(t, err)
	_, err = store.DB().Exec(`
		INSERT INTO tool_calls (session_id, message_ordinal, call_index, tool_name, category, tool_use_id, skill_name)
		VALUES
			('pg-p23-long', 1, 0, 'Skill', 'Task', 'pg-skill-a', 'assist-learn'),
			('pg-p23-long', 1, 1, 'Skill', 'Task', 'pg-skill-b', 'assist-learn'),
			('pg-p23-other', 0, 0, 'Skill', 'Task', 'pg-skill-c', 'write-outline')`)
	require.NoError(t, err)
}

func seedPhase23PGNullTimestampSkill(t *testing.T, store *Store) {
	t.Helper()
	_, err := store.DB().Exec(`
		INSERT INTO sessions (
			id, machine, project, agent, first_message, started_at, ended_at,
			created_at, message_count, user_message_count, relationship_type
		) VALUES (
			'pg-p23-null-ts', 'local', 'p23', 'claude', 'null ts',
			'2024-06-01T08:00:00Z'::timestamptz, '2024-06-01T08:10:00Z'::timestamptz,
			'2024-06-01T08:00:00Z'::timestamptz, 1, 0, 'root')`)
	require.NoError(t, err)
	_, err = store.DB().Exec(`
		INSERT INTO messages (session_id, ordinal, role, content, timestamp, model, has_tool_use)
		VALUES ('pg-p23-null-ts', 0, 'assistant', 'answer', NULL, 'model-a', TRUE)`)
	require.NoError(t, err)
	_, err = store.DB().Exec(`
		INSERT INTO tool_calls (session_id, message_ordinal, call_index, tool_name, category, tool_use_id, skill_name)
		VALUES ('pg-p23-null-ts', 0, 0, 'Skill', 'Task', 'pg-null-skill', 'assist-learn')`)
	require.NoError(t, err)
}
