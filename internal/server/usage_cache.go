package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

// usageCacheTTL bounds how long a usage query result is served from
// memory before it is recomputed. Entries are keyed by the full query
// filter, so different ranges/filters never collide. Live sessions
// write to the DB constantly, which is exactly why a short data-version
// invalidation would defeat caching here; for cost analytics a few
// minutes of staleness is an acceptable trade for not re-scanning and
// re-parsing ~100k message rows on every refresh. The manual refresh
// button bypasses the cache (no_cache=true) for an always-fresh escape
// hatch.
const usageCacheTTL = 10 * time.Minute

// usageCacheMaxEntries caps memory use. Distinct filter combinations in
// normal use are few; the cap is a backstop against unbounded growth
// from many one-off filter permutations.
const usageCacheMaxEntries = 256

type ttlCacheEntry[V any] struct {
	value   V
	expires time.Time
}

// ttlCache is a small, thread-safe, size-bounded cache with per-entry
// TTL. Eviction is intentionally simple: expired entries are dropped on
// access, and if still at capacity the soonest-to-expire entries are
// removed to make room.
type ttlCache[V any] struct {
	mu      sync.Mutex
	ttl     time.Duration
	maxN    int
	entries map[string]ttlCacheEntry[V]
}

func newTTLCache[V any](ttl time.Duration, maxN int) *ttlCache[V] {
	return &ttlCache[V]{
		ttl:     ttl,
		maxN:    maxN,
		entries: make(map[string]ttlCacheEntry[V]),
	}
}

// get returns the cached value for key when present and not expired.
func (c *ttlCache[V]) get(key string, now time.Time) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !now.Before(e.expires) {
		if ok {
			delete(c.entries, key)
		}
		var zero V
		return zero, false
	}
	return e.value, true
}

func (c *ttlCache[V]) set(key string, value V, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictLocked(now)
	c.entries[key] = ttlCacheEntry[V]{
		value:   value,
		expires: now.Add(c.ttl),
	}
}

// evictLocked drops expired entries; if still at/over capacity it drops
// the entries closest to expiry until back under the cap.
func (c *ttlCache[V]) evictLocked(now time.Time) {
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	for len(c.entries) >= c.maxN {
		var oldestKey string
		var oldest time.Time
		first := true
		for k, e := range c.entries {
			if first || e.expires.Before(oldest) {
				oldestKey, oldest, first = k, e.expires, false
			}
		}
		if first {
			return
		}
		delete(c.entries, oldestKey)
	}
}

// usageFilterKey derives a stable cache key from a usage filter. The
// %#v verb renders every field with its name, so two filters collide
// only when all fields are equal. NoCache is not part of db.UsageFilter
// (it is a request-level concern), so it never affects the key.
func usageFilterKey(f db.UsageFilter) string {
	return fmt.Sprintf("%#v", f)
}

// cachedDailyUsage serves GetDailyUsage results from the TTL cache.
// When noCache is true the cache is bypassed for the read but still
// refreshed with the fresh result, so later cached reads are current.
func (s *Server) cachedDailyUsage(
	ctx context.Context, f db.UsageFilter, noCache bool,
) (db.DailyUsageResult, error) {
	// Production servers always wire the cache in New; a nil cache means
	// a minimally-constructed Server (e.g. in tests), so degrade to an
	// uncached direct read rather than panicking.
	if s.dailyUsageCache == nil {
		return s.db.GetDailyUsage(ctx, f)
	}
	key := usageFilterKey(f)
	now := time.Now()
	if !noCache {
		if v, ok := s.dailyUsageCache.get(key, now); ok {
			return v, nil
		}
	}
	v, err := s.db.GetDailyUsage(ctx, f)
	if err != nil {
		return db.DailyUsageResult{}, err
	}
	s.dailyUsageCache.set(key, v, now)
	return v, nil
}

// cachedTopSessions serves GetTopSessionsByCost results from the TTL
// cache, keyed by filter plus limit.
func (s *Server) cachedTopSessions(
	ctx context.Context, f db.UsageFilter, limit int, noCache bool,
) ([]db.TopSessionEntry, error) {
	if s.topSessionsCache == nil {
		return s.db.GetTopSessionsByCost(ctx, f, limit)
	}
	key := fmt.Sprintf("%s|limit=%d", usageFilterKey(f), limit)
	now := time.Now()
	if !noCache {
		if v, ok := s.topSessionsCache.get(key, now); ok {
			return v, nil
		}
	}
	v, err := s.db.GetTopSessionsByCost(ctx, f, limit)
	if err != nil {
		return nil, err
	}
	s.topSessionsCache.set(key, v, now)
	return v, nil
}
