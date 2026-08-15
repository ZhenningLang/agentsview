//go:build pgtest

package postgres

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

// TestListSessions_HasSecret verifies that the HasSecret filter
// returns only sessions where secret_leak_count > 0.
func TestListSessions_HasSecret(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	require.NoError(t, err, "NewStore")
	defer store.Close()

	pg := store.DB()

	// Seed a session with leaks and one without.
	_, err = pg.Exec(`
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 started_at, ended_at, message_count,
			 user_message_count, secret_leak_count)
		VALUES
			('has-secret-leaky', 'test-machine', 'test-project',
			 'claude-code', 'secret session',
			 '2026-03-12T09:00:00Z'::timestamptz,
			 '2026-03-12T09:30:00Z'::timestamptz,
			 2, 1, 3),
			('has-secret-clean', 'test-machine', 'test-project',
			 'claude-code', 'clean session',
			 '2026-03-12T08:00:00Z'::timestamptz,
			 '2026-03-12T08:30:00Z'::timestamptz,
			 2, 1, 0)
	`)
	require.NoError(t, err, "inserting test sessions")

	ctx := context.Background()
	page, err := store.ListSessions(ctx, db.SessionFilter{
		HasSecret: true,
		Limit:     50,
	})
	require.NoError(t, err, "ListSessions")

	// Only the leaky session should appear.
	for _, s := range page.Sessions {
		assert.NotEqual(t, "has-secret-clean", s.ID,
			"clean session (secret_leak_count=0) included in HasSecret results")
	}

	var found *db.Session
	for i := range page.Sessions {
		if page.Sessions[i].ID == "has-secret-leaky" {
			found = &page.Sessions[i]
			break
		}
	}
	require.NotNil(t, found, "leaky session not found in HasSecret results")
	assert.Equal(t, 3, found.SecretLeakCount)

	_, err = pg.Exec(`
		UPDATE sessions
		SET secrets_rules_version = 'v-current'
		WHERE id = 'has-secret-leaky';
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 started_at, ended_at, message_count,
			 user_message_count, secret_leak_count, secrets_rules_version)
		VALUES
			('has-secret-stale', 'test-machine', 'test-project',
			 'claude-code', 'stale secret session',
			 '2026-03-12T07:00:00Z'::timestamptz,
			 '2026-03-12T07:30:00Z'::timestamptz,
			 2, 1, 2, 'old-rules')
	`)
	require.NoError(t, err, "seeding stale secret session")
	current, err := store.ListSessions(ctx, db.SessionFilter{
		HasSecret:            true,
		SecretsRulesVersions: []string{"v-current"},
		Limit:                50,
	})
	require.NoError(t, err, "ListSessions current rules")
	for _, s := range current.Sessions {
		require.NotEqual(t, "has-secret-stale", s.ID,
			"stale secret session included in versioned HasSecret results")
	}
}

func TestListSessionsActiveSinceUsesSessionActivity(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	require.NoError(t, err, "NewStore")
	defer store.Close()

	_, err = store.DB().Exec(`
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 created_at, started_at, ended_at, message_count,
			 user_message_count)
		VALUES
			('active-ended', 'test-machine', 'active-parity',
			 'claude-code', 'ended later',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:01:00Z'::timestamptz,
			 2, 2),
			('active-started', 'test-machine', 'active-parity',
			 'claude-code', 'started later',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:02:00Z'::timestamptz,
			 NULL,
			 2, 2),
			('active-created', 'test-machine', 'active-parity',
			 'claude-code', 'created later',
			 '2024-06-01T00:03:00Z'::timestamptz,
			 NULL,
			 NULL,
			 2, 2),
			('active-stale', 'test-machine', 'active-parity',
			 'claude-code', 'stale',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-05-31T23:59:59Z'::timestamptz,
			 2, 2)
	`)
	require.NoError(t, err, "inserting active_since sessions")

	page, err := store.ListSessions(context.Background(), db.SessionFilter{
		Project:     "active-parity",
		ActiveSince: "2024-06-01T00:00:00Z",
		Limit:       50,
	})
	require.NoError(t, err, "ListSessions ActiveSince")

	ids := make([]string, 0, len(page.Sessions))
	for _, sess := range page.Sessions {
		ids = append(ids, sess.ID)
	}
	assert.ElementsMatch(t,
		[]string{"active-ended", "active-started", "active-created"}, ids)
	assert.NotContains(t, ids, "active-stale")
}

func TestPhase15PGListSessionsSortCursorParity(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	require.NoError(t, err, "NewStore")
	defer store.Close()

	_, err = store.DB().Exec(`
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 created_at, started_at, ended_at, message_count,
			 user_message_count)
		VALUES
			('phase15-pg-a', 'test-machine', 'phase15-pg-sort',
			 'claude-code', 'a',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:02:00Z'::timestamptz,
			 '2024-06-01T00:02:30Z'::timestamptz,
			 7, 2),
			('phase15-pg-b', 'test-machine', 'phase15-pg-sort',
			 'claude-code', 'b',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:01:00Z'::timestamptz,
			 '2024-06-01T00:01:30Z'::timestamptz,
			 7, 2),
			('phase15-pg-c', 'test-machine', 'phase15-pg-sort',
			 'claude-code', 'c',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:03:00Z'::timestamptz,
			 '2024-06-01T00:03:30Z'::timestamptz,
			 4, 2)
	`)
	require.NoError(t, err, "inserting phase15 pg sort sessions")

	ctx := context.Background()
	first, err := store.ListSessions(ctx, db.SessionFilter{
		Project: "phase15-pg-sort", OrderBy: "messages:desc,started:asc", Limit: 1,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"phase15-pg-b"}, pgSessionIDs(first.Sessions))
	require.NotEmpty(t, first.NextCursor)

	cur, err := store.DecodeCursor(first.NextCursor)
	require.NoError(t, err)
	require.Len(t, cur.Keys, 2)
	require.Equal(t, "messages", cur.Keys[0].Sort)
	require.True(t, cur.Keys[0].Desc)
	require.Equal(t, "started", cur.Keys[1].Sort)
	require.False(t, cur.Keys[1].Desc)

	second, err := store.ListSessions(ctx, db.SessionFilter{
		Project: "phase15-pg-sort", OrderBy: "messages:desc,started:asc",
		Limit: 1, Cursor: first.NextCursor,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"phase15-pg-a"}, pgSessionIDs(second.Sessions))
	require.Equal(t, []string{"phase15-pg-b", "phase15-pg-a", "phase15-pg-c"},
		pgWalkSessionIDs(t, store, db.SessionFilter{
			Project: "phase15-pg-sort", OrderBy: "messages:desc,started:asc", Limit: 1,
		}))

	for _, payload := range []string{
		`{"i":"phase15-pg-b","ks":[{"k":"messages","d":true,"v":"7"}]}`,
		`{"i":"phase15-pg-b","KS":[{"k":"messages","d":true,"v":"7"}]}`,
		`{"i":"phase15-pg-b","K":"messages","D":true,"V":"7"}`,
	} {
		_, err = store.DecodeCursor(base64.RawURLEncoding.EncodeToString([]byte(payload)))
		require.ErrorIs(t, err, db.ErrInvalidCursor)
	}

	_, err = store.DB().Exec(`
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 created_at, started_at, ended_at, message_count,
			 user_message_count, health_score, context_pressure_max,
			 secret_leak_count, secrets_rules_version)
		VALUES
			('phase15-pg-null', 'test-machine', 'phase15-pg-extra', 'claude-code', 'n',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:01:00Z'::timestamptz,
			 '2024-06-01T00:01:30Z'::timestamptz,
			 1, 1, NULL, NULL, 0, ''),
			('phase15-pg-low', 'test-machine', 'phase15-pg-extra', 'claude-code', 'l',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:02:00Z'::timestamptz,
			 '2024-06-01T00:02:30Z'::timestamptz,
			 1, 1, 10, 0.95, 9, 'old-rules'),
			('phase15-pg-high', 'test-machine', 'phase15-pg-extra', 'claude-code', 'h',
			 '2024-01-01T00:00:00Z'::timestamptz,
			 '2024-06-01T00:03:00Z'::timestamptz,
			 '2024-06-01T00:03:30Z'::timestamptz,
			 1, 1, 90, 0.25, 1, 'current-rules')
	`)
	require.NoError(t, err, "inserting phase15 pg extra sessions")
	require.Equal(t, []string{"phase15-pg-low", "phase15-pg-high", "phase15-pg-null"},
		pgWalkSessionIDs(t, store, db.SessionFilter{Project: "phase15-pg-extra", OrderBy: "health", Limit: 1}))
	require.Equal(t, []string{"phase15-pg-high", "phase15-pg-low", "phase15-pg-null"},
		pgWalkSessionIDs(t, store, db.SessionFilter{Project: "phase15-pg-extra", OrderBy: "health:desc", Limit: 1}))
	require.Equal(t, []string{"phase15-pg-high", "phase15-pg-low", "phase15-pg-null"},
		pgWalkSessionIDs(t, store, db.SessionFilter{Project: "phase15-pg-extra", OrderBy: "context-pressure", Limit: 1}))
	require.Equal(t, []string{"phase15-pg-high", "phase15-pg-null", "phase15-pg-low"},
		pgWalkSessionIDs(t, store, db.SessionFilter{
			Project: "phase15-pg-extra", OrderBy: "secrets:desc", Limit: 1,
			SecretsRulesVersions: []string{"current-rules"},
		}))
}

func pgWalkSessionIDs(t *testing.T, store *Store, f db.SessionFilter) []string {
	t.Helper()
	ctx := context.Background()
	cursor := f.Cursor
	seen := map[string]bool{}
	ids := []string{}
	for i := 0; i < 20; i++ {
		f.Cursor = cursor
		page, err := store.ListSessions(ctx, f)
		require.NoError(t, err)
		ids = append(ids, pgSessionIDs(page.Sessions)...)
		if page.NextCursor == "" {
			return ids
		}
		require.False(t, seen[page.NextCursor], "cursor repeated")
		seen[page.NextCursor] = true
		cursor = page.NextCursor
	}
	t.Fatalf("pagination did not terminate; ids=%v", ids)
	return nil
}

func pgSessionIDs(sessions []db.Session) []string {
	ids := make([]string, len(sessions))
	for i, sess := range sessions {
		ids[i] = sess.ID
	}
	return ids
}
