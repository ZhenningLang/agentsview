package duckdb

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPhase23AnalyticsModelFilterDuckDBSkills(t *testing.T) {
	ctx := context.Background()
	duck := openTestDuckDB(t)
	require.NoError(t, EnsureSchema(ctx, duck))
	store := NewStoreFromDB(duck)
	seedPhase23DuckSkills(t, duck)

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

func seedPhase23DuckSkills(t *testing.T, duck *sql.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := duck.ExecContext(ctx, `
		INSERT INTO sessions (
			id, project, machine, agent, first_message, started_at, ended_at,
			created_at, message_count, user_message_count, relationship_type
		) VALUES
			('duck-p23-long', 'p23', 'local', 'claude', 'long',
			 '2024-05-31T23:00:00Z'::TIMESTAMP, '2024-06-01T00:10:00Z'::TIMESTAMP,
			 '2024-05-31T23:00:00Z'::TIMESTAMP, 2, 1, 'root'),
			('duck-p23-other', 'p23', 'local', 'claude', 'other',
			 '2024-06-01T01:00:00Z'::TIMESTAMP, '2024-06-01T01:10:00Z'::TIMESTAMP,
			 '2024-06-01T01:00:00Z'::TIMESTAMP, 1, 0, 'root')`)
	require.NoError(t, err)
	_, err = duck.ExecContext(ctx, `
		INSERT INTO messages (id, session_id, ordinal, role, content, timestamp, model, has_tool_use)
		VALUES
			(9101, 'duck-p23-long', 0, 'user', 'prompt', '2024-05-31T23:59:00Z'::TIMESTAMP, '', FALSE),
			(9102, 'duck-p23-long', 1, 'assistant', 'answer', '2024-06-01T00:05:00Z'::TIMESTAMP, 'model-a', TRUE),
			(9103, 'duck-p23-other', 0, 'assistant', 'other', '2024-06-01T01:05:00Z'::TIMESTAMP, 'model-b', TRUE)`)
	require.NoError(t, err)
	_, err = duck.ExecContext(ctx, `
		INSERT INTO tool_calls (id, message_id, session_id, tool_name, category, call_index, tool_use_id, skill_name)
		VALUES
			(9201, 9102, 'duck-p23-long', 'Skill', 'Task', 0, 'duck-skill-a', 'assist-learn'),
			(9202, 9102, 'duck-p23-long', 'Skill', 'Task', 1, 'duck-skill-b', 'assist-learn'),
			(9203, 9103, 'duck-p23-other', 'Skill', 'Task', 0, 'duck-skill-c', 'write-outline')`)
	require.NoError(t, err)
}
