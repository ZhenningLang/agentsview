package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestTTLCacheHitMissAndExpiry(t *testing.T) {
	c := newTTLCache[int](time.Minute, 10)
	base := time.Unix(1_700_000_000, 0)

	_, ok := c.get("a", base)
	assert.False(t, ok, "empty cache should miss")

	c.set("a", 42, base)
	v, ok := c.get("a", base.Add(30*time.Second))
	require.True(t, ok, "within TTL should hit")
	assert.Equal(t, 42, v)

	_, ok = c.get("a", base.Add(time.Minute))
	assert.False(t, ok, "at/after expiry should miss")

	// An expired entry is dropped on the missing read.
	_, ok = c.get("a", base.Add(2*time.Minute))
	assert.False(t, ok)
}

func TestTTLCacheKeysAreIndependent(t *testing.T) {
	c := newTTLCache[string](time.Minute, 10)
	base := time.Unix(1_700_000_000, 0)
	c.set("a", "av", base)
	c.set("b", "bv", base)

	v, ok := c.get("a", base)
	require.True(t, ok)
	assert.Equal(t, "av", v)
	v, ok = c.get("b", base)
	require.True(t, ok)
	assert.Equal(t, "bv", v)
}

func TestTTLCacheEvictsAtCapacity(t *testing.T) {
	c := newTTLCache[int](time.Hour, 2)
	base := time.Unix(1_700_000_000, 0)

	// Stagger expiry so eviction order is deterministic: "a" expires
	// soonest and should be the first dropped when capacity is hit.
	c.set("a", 1, base)
	c.set("b", 2, base.Add(time.Second))
	c.set("c", 3, base.Add(2*time.Second))

	assert.LessOrEqual(t, len(c.entries), 2, "must not exceed cap")
	_, ok := c.get("a", base.Add(3*time.Second))
	assert.False(t, ok, "soonest-to-expire entry should be evicted")
	_, ok = c.get("c", base.Add(3*time.Second))
	assert.True(t, ok, "most recently set entry should survive")
}

// countingStore embeds db.Store so it satisfies the interface while
// only the usage methods are overridden (and counted). The embedded
// interface stays nil; no other method is exercised by these tests.
type countingStore struct {
	db.Store
	dailyCalls int
	topCalls   int
	daily      db.DailyUsageResult
	top        []db.TopSessionEntry
}

func (c *countingStore) GetDailyUsage(
	_ context.Context, _ db.UsageFilter,
) (db.DailyUsageResult, error) {
	c.dailyCalls++
	return c.daily, nil
}

func (c *countingStore) GetTopSessionsByCost(
	_ context.Context, _ db.UsageFilter, _ int,
) ([]db.TopSessionEntry, error) {
	c.topCalls++
	return c.top, nil
}

func newCacheTestServer(store db.Store) *Server {
	return &Server{
		db: store,
		dailyUsageCache: newTTLCache[db.DailyUsageResult](
			usageCacheTTL, usageCacheMaxEntries,
		),
		topSessionsCache: newTTLCache[[]db.TopSessionEntry](
			usageCacheTTL, usageCacheMaxEntries,
		),
	}
}

func TestCachedDailyUsageServesFromCache(t *testing.T) {
	store := &countingStore{
		daily: db.DailyUsageResult{
			Totals: db.UsageTotals{TotalCost: 1.5},
		},
	}
	s := newCacheTestServer(store)
	f := db.UsageFilter{From: "2026-01-01", To: "2026-01-31"}
	ctx := context.Background()

	r1, err := s.cachedDailyUsage(ctx, f, false)
	require.NoError(t, err)
	assert.Equal(t, 1.5, r1.Totals.TotalCost)

	r2, err := s.cachedDailyUsage(ctx, f, false)
	require.NoError(t, err)
	assert.Equal(t, 1.5, r2.Totals.TotalCost)
	assert.Equal(t, 1, store.dailyCalls, "second identical read should hit cache")

	// A different filter must not collide with the cached entry.
	_, err = s.cachedDailyUsage(ctx, db.UsageFilter{From: "2025-01-01", To: "2025-01-31"}, false)
	require.NoError(t, err)
	assert.Equal(t, 2, store.dailyCalls, "different filter recomputes")
}

func TestCachedDailyUsageNoCacheBypassesButRefreshes(t *testing.T) {
	store := &countingStore{}
	s := newCacheTestServer(store)
	f := db.UsageFilter{From: "2026-01-01", To: "2026-01-31"}
	ctx := context.Background()

	_, err := s.cachedDailyUsage(ctx, f, false)
	require.NoError(t, err)
	assert.Equal(t, 1, store.dailyCalls)

	// no_cache bypasses the read and recomputes.
	_, err = s.cachedDailyUsage(ctx, f, true)
	require.NoError(t, err)
	assert.Equal(t, 2, store.dailyCalls, "no_cache forces recompute")

	// ...but it also refreshed the cache, so a normal read hits again.
	_, err = s.cachedDailyUsage(ctx, f, false)
	require.NoError(t, err)
	assert.Equal(t, 2, store.dailyCalls, "refreshed entry serves the next read")
}

func TestCachedTopSessionsKeyedByLimit(t *testing.T) {
	store := &countingStore{top: []db.TopSessionEntry{}}
	s := newCacheTestServer(store)
	f := db.UsageFilter{From: "2026-01-01", To: "2026-01-31"}
	ctx := context.Background()

	_, err := s.cachedTopSessions(ctx, f, 20, false)
	require.NoError(t, err)
	_, err = s.cachedTopSessions(ctx, f, 20, false)
	require.NoError(t, err)
	assert.Equal(t, 1, store.topCalls, "same filter+limit hits cache")

	// A different limit is a distinct query and must recompute.
	_, err = s.cachedTopSessions(ctx, f, 50, false)
	require.NoError(t, err)
	assert.Equal(t, 2, store.topCalls, "different limit recomputes")
}
