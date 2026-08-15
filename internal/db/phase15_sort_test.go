package db

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase15SortSpecParsesPerKeyDirections(t *testing.T) {
	keys, err := ParseSortSpec("messages:desc,started:asc")
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.NotNil(t, keys[0].Descending)
	require.NotNil(t, keys[1].Descending)
	assert.Equal(t, "messages", keys[0].Key)
	assert.True(t, *keys[0].Descending)
	assert.Equal(t, "started", keys[1].Key)
	assert.False(t, *keys[1].Descending)
	assert.Equal(t, "messages:desc,started:asc", FormatSortSpec(keys))

	for _, spec := range []string{"missing", "messages:nope", "messages,messages", "messages,"} {
		_, err := ParseSortSpec(spec)
		assert.Error(t, err, spec)
	}
}

func TestPhase15SessionPaginationMixedDirectionCursor(t *testing.T) {
	d := testDB(t)
	insertPhase15SortSession(t, d, "s-1", "2026-01-01T00:00:00Z", 4)
	insertPhase15SortSession(t, d, "s-2", "2026-01-02T00:00:00Z", 7)
	insertPhase15SortSession(t, d, "s-3", "2026-01-03T00:00:00Z", 7)

	ctx := context.Background()
	got := phase15WalkSessionIDs(t, d, SessionFilter{
		Project: "phase15-sort", OrderBy: "messages:desc,started:asc", Limit: 1,
	})
	assert.Equal(t, []string{"s-2", "s-3", "s-1"}, got)

	first, err := d.ListSessions(ctx, SessionFilter{
		Project: "phase15-sort", OrderBy: "messages", Limit: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, first.NextCursor)
	_, err = d.ListSessions(ctx, SessionFilter{
		Project: "phase15-sort", OrderBy: "started", Limit: 1,
		Cursor: first.NextCursor,
	})
	require.ErrorIs(t, err, ErrInvalidCursor)
}

func TestPhase15UnsignedLegacyCursorRejectsSignedSortFields(t *testing.T) {
	d := testDB(t)
	insertPhase15SortSession(t, d, "s-1", "2026-01-01T00:00:00Z", 4)
	tests := []string{
		`{"i":"s-1","ks":[{"k":"messages","d":true,"v":"4"}]}`,
		`{"i":"s-1","KS":[{"k":"messages","d":true,"v":"4"}]}`,
		`{"i":"s-1","Ks":[{"k":"messages","d":true,"v":"4"}]}`,
		`{"i":"s-1","K":"messages","D":true,"V":"4"}`,
	}
	for _, payload := range tests {
		t.Run(payload, func(t *testing.T) {
			unsigned := base64.RawURLEncoding.EncodeToString([]byte(payload))
			_, err := d.ListSessions(context.Background(), SessionFilter{
				Project: "phase15-sort", OrderBy: "messages:desc,started:asc", Cursor: unsigned,
			})
			require.ErrorIs(t, err, ErrInvalidCursor)
		})
	}
}

func TestPhase15LegacyRecentCursorStillPaginatesDefaultSort(t *testing.T) {
	d := testDB(t)
	insertPhase15SortSession(t, d, "s-old", "2026-01-01T00:00:00Z", 4)
	insertPhase15SortSession(t, d, "s-new", "2026-01-02T00:00:00Z", 4)
	legacy := d.EncodeCursor("2026-01-02T00:00:00Z", "s-new")

	page, err := d.ListSessions(context.Background(), SessionFilter{
		Project: "phase15-sort", Cursor: legacy, Limit: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"s-old"}, phase15SessionIDs(page.Sessions))
}

func TestPhase15NullableSortsPutNullLastAcrossCursorBoundary(t *testing.T) {
	d := testDB(t)
	insertPhase15SortSession(t, d, "health-null", "2026-01-01T00:00:00Z", 1)
	insertPhase15SortSession(t, d, "health-low", "2026-01-02T00:00:00Z", 1)
	insertPhase15SortSession(t, d, "health-high", "2026-01-03T00:00:00Z", 1)
	require.NoError(t, d.UpdateSessionSignals("health-low", SessionSignalUpdate{HealthScore: Ptr(10)}))
	require.NoError(t, d.UpdateSessionSignals("health-high", SessionSignalUpdate{HealthScore: Ptr(90)}))

	asc := phase15WalkSessionIDs(t, d, SessionFilter{Project: "phase15-sort", OrderBy: "health", Limit: 1})
	desc := phase15WalkSessionIDs(t, d, SessionFilter{Project: "phase15-sort", OrderBy: "health:desc", Limit: 1})
	assert.Equal(t, []string{"health-low", "health-high", "health-null"}, asc)
	assert.Equal(t, []string{"health-high", "health-low", "health-null"}, desc)

	cpLow, cpHigh := 0.25, 0.95
	require.NoError(t, d.UpdateSessionSignals("health-low", SessionSignalUpdate{ContextPressureMax: &cpHigh}))
	require.NoError(t, d.UpdateSessionSignals("health-high", SessionSignalUpdate{ContextPressureMax: &cpLow}))
	cpAsc := phase15WalkSessionIDs(t, d, SessionFilter{Project: "phase15-sort", OrderBy: "context-pressure", Limit: 1})
	assert.Equal(t, []string{"health-high", "health-low", "health-null"}, cpAsc)
}

func TestPhase15SecretsSortUsesActiveRulesVersion(t *testing.T) {
	d := testDB(t)
	insertPhase15SortSession(t, d, "stale-many", "2026-01-01T00:00:00Z", 1)
	insertPhase15SortSession(t, d, "current-one", "2026-01-02T00:00:00Z", 1)
	insertPhase15SortSession(t, d, "clean", "2026-01-03T00:00:00Z", 1)
	require.NoError(t, d.ReplaceSessionSecretFindings("stale-many", nil, 9, "old-rules"))
	require.NoError(t, d.ReplaceSessionSecretFindings("current-one", nil, 1, "current-rules"))

	ids := phase15WalkSessionIDs(t, d, SessionFilter{
		Project: "phase15-sort", OrderBy: "secrets:desc", Limit: 1,
		SecretsRulesVersions: []string{"current-rules"},
	})
	assert.Equal(t, []string{"current-one", "stale-many", "clean"}, ids)
}

func TestPhase15AllSortKeysAreStableWithIDTieBreaker(t *testing.T) {
	d := testDB(t)
	insertPhase15SortSession(t, d, "tie-b", "2026-01-01T00:00:00Z", 4)
	insertPhase15SortSession(t, d, "tie-a", "2026-01-01T00:00:00Z", 4)
	for _, key := range SortKeys() {
		t.Run(key, func(t *testing.T) {
			ids := phase15WalkSessionIDs(t, d, SessionFilter{
				Project: "phase15-sort", OrderBy: key + ":asc", Limit: 1,
			})
			assert.Equal(t, []string{"tie-a", "tie-b"}, ids)
		})
	}
}

func TestPhase15SessionListSortRejectsInvalidCursorValue(t *testing.T) {
	d := testDB(t)
	insertPhase15SortSession(t, d, "s-1", "2026-01-01T00:00:00Z", 4)
	bad := d.EncodeSessionCursor(SessionCursor{
		ID: "s-1", Sort: "messages", Value: "not-int",
		Keys: []SessionCursorKey{{Sort: "messages", Value: "not-int"}},
	})
	_, err := d.ListSessions(context.Background(), SessionFilter{
		Project: "phase15-sort", OrderBy: "messages", Cursor: bad,
	})
	require.True(t, errors.Is(err, ErrInvalidCursor), "err=%v", err)
}

func insertPhase15SortSession(t *testing.T, d *DB, id, started string, messages int) {
	t.Helper()
	ended := started
	require.NoError(t, d.UpsertSession(Session{
		ID: id, Project: "phase15-sort", Machine: "m", Agent: "claude",
		StartedAt: &started, EndedAt: &ended,
		MessageCount: messages, UserMessageCount: 2,
	}))
}

func phase15WalkSessionIDs(t *testing.T, d *DB, f SessionFilter) []string {
	t.Helper()
	ctx := context.Background()
	cursor := f.Cursor
	seen := map[string]bool{}
	ids := []string{}
	for pageNum := 0; pageNum < 20; pageNum++ {
		f.Cursor = cursor
		page, err := d.ListSessions(ctx, f)
		require.NoError(t, err)
		for _, s := range page.Sessions {
			ids = append(ids, s.ID)
		}
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

func phase15SessionIDs(sessions []Session) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids
}
