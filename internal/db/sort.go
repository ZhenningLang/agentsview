// ABOUTME: session-list sort registry -- maps public sort keys to SQL
// ABOUTME: expressions and keyset cursor values shared by all stores.
package db

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

type valueKind int

const (
	kindTimestamp valueKind = iota
	kindInt
	kindReal
	kindText
)

const (
	sentinelIntDescSQL  = "-9223372036854775808"
	sentinelIntAscSQL   = "9223372036854775807"
	sentinelRealDescSQL = "-1e18"
	sentinelRealAscSQL  = "1e18"
)

type SessionSort struct {
	key               string
	kind              valueKind
	defaultDescending bool
	nullable          bool
	expr              func(*QueryBuilder, SessionFilter) string
	value             func(*Session, SessionFilter) (string, bool)
}

func (sp SessionSort) orderExpr(b *QueryBuilder, desc bool, f SessionFilter) string {
	e := sp.expr(b, f)
	if sp.nullable {
		e = "COALESCE(" + e + ", " + sentinelLiteral(sp.kind, desc) + ")"
	}
	return e
}

func (sp SessionSort) cursorValue(s *Session, desc bool, f SessionFilter) string {
	if v, ok := sp.value(s, f); ok {
		return v
	}
	return sentinelGoString(sp.kind, desc)
}

func (sp SessionSort) ResolveDescending(descending *bool) bool {
	if descending != nil {
		return *descending
	}
	return sp.defaultDescending
}

type SortKey struct {
	Key        string
	Descending *bool
}

type ResolvedSort struct {
	Sort SessionSort
	Desc bool
}

func ParseSortSpec(spec string) ([]SortKey, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	parts := strings.Split(spec, ",")
	keys := make([]SortKey, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty sort term")
		}
		key := part
		var dir *bool
		if before, after, ok := strings.Cut(part, ":"); ok {
			key = strings.TrimSpace(before)
			switch strings.TrimSpace(after) {
			case "asc":
				d := false
				dir = &d
			case "desc":
				d := true
				dir = &d
			default:
				return nil, fmt.Errorf("invalid sort direction in %q: want asc or desc", part)
			}
		}
		if _, ok := sessionSortByKey[key]; !ok {
			return nil, fmt.Errorf("unknown sort key %q", key)
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate sort key %q", key)
		}
		seen[key] = true
		keys = append(keys, SortKey{Key: key, Descending: dir})
	}
	return keys, nil
}

func FormatSortSpec(keys []SortKey) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		switch {
		case k.Descending == nil:
			parts[i] = k.Key
		case *k.Descending:
			parts[i] = k.Key + ":desc"
		default:
			parts[i] = k.Key + ":asc"
		}
	}
	return strings.Join(parts, ",")
}

func ApplyFallbackDirection(keys []SortKey, descending *bool) []SortKey {
	if descending == nil {
		return keys
	}
	out := make([]SortKey, len(keys))
	for i, k := range keys {
		if k.Descending == nil {
			d := *descending
			k.Descending = &d
		}
		out[i] = k
	}
	return out
}

func ResolveSort(f SessionFilter) []ResolvedSort {
	terms := f.Sort
	if len(terms) == 0 {
		parsed, err := ParseSortSpec(f.OrderBy)
		if err != nil {
			parsed = nil
		}
		terms = ApplyFallbackDirection(parsed, f.Descending)
	}
	rs := make([]ResolvedSort, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, t := range terms {
		sp, ok := sessionSortByKey[t.Key]
		if !ok || seen[sp.key] {
			continue
		}
		seen[sp.key] = true
		rs = append(rs, ResolvedSort{Sort: sp, Desc: sp.ResolveDescending(t.Descending)})
	}
	if len(rs) == 0 {
		sp := sessionSortByKey[defaultSortKey]
		desc := sp.defaultDescending
		if len(f.Sort) == 0 && f.Descending != nil {
			desc = *f.Descending
		}
		rs = append(rs, ResolvedSort{Sort: sp, Desc: desc})
	}
	return rs
}

func appendIDTiebreaker(rs []ResolvedSort) []ResolvedSort {
	for _, r := range rs {
		if r.Sort.key == "id" {
			return rs
		}
	}
	out := make([]ResolvedSort, len(rs), len(rs)+1)
	copy(out, rs)
	return append(out, ResolvedSort{Sort: sessionSortByKey["id"], Desc: rs[len(rs)-1].Desc})
}

func NextSessionCursor(last *Session, rs []ResolvedSort, total int, f SessionFilter) SessionCursor {
	cur := SessionCursor{ID: last.ID, Total: total}
	cur.Keys = make([]SessionCursorKey, len(rs))
	for i, r := range rs {
		cur.Keys[i] = SessionCursorKey{Sort: r.Sort.key, Desc: r.Desc, Value: r.Sort.cursorValue(last, r.Desc, f)}
	}
	if len(rs) == 1 {
		k := cur.Keys[0]
		cur.Sort = k.Sort
		cur.Desc = k.Desc
		cur.Value = k.Value
		if k.Sort == defaultSortKey && k.Desc {
			cur.EndedAt = k.Value
		}
	}
	return cur
}

func CursorPredicateValues(cur SessionCursor, rs []ResolvedSort) ([]any, error) {
	keys := cur.resolvedKeys()
	if len(keys) != len(rs) {
		return nil, fmt.Errorf("%w: sort mismatch", ErrInvalidCursor)
	}
	vals := make([]any, len(rs))
	for i, r := range rs {
		if keys[i].Sort != r.Sort.key || keys[i].Desc != r.Desc {
			return nil, fmt.Errorf("%w: sort mismatch", ErrInvalidCursor)
		}
		v, err := typedCursorValue(keys[i].Value, r.Sort.kind)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
		}
		vals[i] = v
	}
	return vals, nil
}

func sentinelLiteral(kind valueKind, desc bool) string {
	switch kind {
	case kindReal:
		if desc {
			return sentinelRealDescSQL
		}
		return sentinelRealAscSQL
	default:
		if desc {
			return sentinelIntDescSQL
		}
		return sentinelIntAscSQL
	}
}

func sentinelGoString(kind valueKind, desc bool) string {
	switch kind {
	case kindReal:
		if desc {
			return strconv.FormatFloat(-1e18, 'g', -1, 64)
		}
		return strconv.FormatFloat(1e18, 'g', -1, 64)
	default:
		if desc {
			return strconv.FormatInt(math.MinInt64, 10)
		}
		return strconv.FormatInt(math.MaxInt64, 10)
	}
}

func typedCursorValue(value string, kind valueKind) (any, error) {
	switch kind {
	case kindInt:
		return strconv.ParseInt(value, 10, 64)
	case kindReal:
		return strconv.ParseFloat(value, 64)
	default:
		return value, nil
	}
}

func tsValue(s *Session, _ SessionFilter) (string, bool) {
	v := s.CreatedAt
	if s.StartedAt != nil && *s.StartedAt != "" {
		v = *s.StartedAt
	}
	if s.EndedAt != nil && *s.EndedAt != "" {
		v = *s.EndedAt
	}
	return v, true
}

func startedValue(s *Session, _ SessionFilter) (string, bool) {
	if s.StartedAt != nil && *s.StartedAt != "" {
		return *s.StartedAt, true
	}
	return s.CreatedAt, true
}

func intValue(get func(*Session) int) func(*Session, SessionFilter) (string, bool) {
	return func(s *Session, _ SessionFilter) (string, bool) {
		return strconv.Itoa(get(s)), true
	}
}

func recentExpr(b *QueryBuilder, _ SessionFilter) string {
	return "COALESCE(" + b.dialect.timestampExpr("ended_at") + ", " +
		b.dialect.timestampExpr("started_at") + ", created_at)"
}

func startedExpr(b *QueryBuilder, _ SessionFilter) string {
	return "COALESCE(" + b.dialect.timestampExpr("started_at") + ", created_at)"
}

func plainExpr(col string) func(*QueryBuilder, SessionFilter) string {
	return func(*QueryBuilder, SessionFilter) string { return col }
}

func secretsExpr(b *QueryBuilder, f SessionFilter) string {
	versions := nonEmpty(f.SecretsRulesVersions)
	if len(versions) == 0 {
		return "secret_leak_count"
	}
	return "CASE WHEN " + inPredicate("secrets_rules_version", versions, b) +
		" THEN secret_leak_count ELSE 0 END"
}

func secretsValue(s *Session, f SessionFilter) (string, bool) {
	n := s.SecretLeakCount
	versions := nonEmpty(f.SecretsRulesVersions)
	if len(versions) > 0 && !slices.Contains(versions, s.SecretsRulesVersion) {
		n = 0
	}
	return strconv.Itoa(n), true
}

var sessionSorts = []SessionSort{
	{key: "recent", kind: kindTimestamp, defaultDescending: true, expr: recentExpr, value: tsValue},
	{key: "started", kind: kindTimestamp, expr: startedExpr, value: startedValue},
	{key: "messages", kind: kindInt, expr: plainExpr("message_count"), value: intValue(func(s *Session) int { return s.MessageCount })},
	{key: "user-messages", kind: kindInt, expr: plainExpr("user_message_count"), value: intValue(func(s *Session) int { return s.UserMessageCount })},
	{key: "output-tokens", kind: kindInt, expr: plainExpr("total_output_tokens"), value: intValue(func(s *Session) int { return s.TotalOutputTokens })},
	{key: "peak-context", kind: kindInt, expr: plainExpr("peak_context_tokens"), value: intValue(func(s *Session) int { return s.PeakContextTokens })},
	{key: "failures", kind: kindInt, expr: plainExpr("tool_failure_signal_count"), value: intValue(func(s *Session) int { return s.ToolFailureSignalCount })},
	{key: "retries", kind: kindInt, expr: plainExpr("tool_retry_count"), value: intValue(func(s *Session) int { return s.ToolRetryCount })},
	{key: "edit-churn", kind: kindInt, expr: plainExpr("edit_churn_count"), value: intValue(func(s *Session) int { return s.EditChurnCount })},
	{key: "compactions", kind: kindInt, expr: plainExpr("compaction_count"), value: intValue(func(s *Session) int { return s.CompactionCount })},
	{key: "context-pressure", kind: kindReal, nullable: true, expr: plainExpr("context_pressure_max"), value: func(s *Session, _ SessionFilter) (string, bool) {
		if s.ContextPressureMax == nil {
			return "", false
		}
		return strconv.FormatFloat(*s.ContextPressureMax, 'g', -1, 64), true
	}},
	{key: "health", kind: kindInt, nullable: true, expr: plainExpr("health_score"), value: func(s *Session, _ SessionFilter) (string, bool) {
		if s.HealthScore == nil {
			return "", false
		}
		return strconv.Itoa(*s.HealthScore), true
	}},
	{key: "secrets", kind: kindInt, expr: secretsExpr, value: secretsValue},
	{key: "id", kind: kindText, expr: plainExpr("id"), value: func(s *Session, _ SessionFilter) (string, bool) { return s.ID, true }},
}

var sessionSortByKey = func() map[string]SessionSort {
	m := make(map[string]SessionSort, len(sessionSorts))
	for _, s := range sessionSorts {
		m[s.key] = s
	}
	return m
}()

const defaultSortKey = "recent"

func DefaultSortKey() string { return defaultSortKey }

func SessionSortFor(key string) (SessionSort, bool) {
	if key == "" {
		return sessionSortByKey[defaultSortKey], true
	}
	sp, ok := sessionSortByKey[key]
	if !ok {
		return sessionSortByKey[defaultSortKey], false
	}
	return sp, true
}

func ValidSortKey(key string) bool {
	if key == "" {
		return true
	}
	_, ok := sessionSortByKey[key]
	return ok
}

func SortDefaultDescending(key string) bool {
	sp, _ := SessionSortFor(key)
	return sp.defaultDescending
}

func SortKeys() []string {
	keys := make([]string, len(sessionSorts))
	for i, s := range sessionSorts {
		keys[i] = s.key
	}
	return keys
}
