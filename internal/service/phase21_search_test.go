package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

// phase21SearchStoreSpy records what the direct backend hands to the
// store. Only HasFTS and Search are needed, so the rest of db.Store is
// satisfied by the embedded nil interface: any other call would panic
// loudly rather than silently returning a zero value.
type phase21SearchStoreSpy struct {
	db.Store
	hasFTS bool
	calls  []db.SearchFilter
	page   db.SearchPage
	err    error
}

func (s *phase21SearchStoreSpy) HasFTS() bool { return s.hasFTS }

func (s *phase21SearchStoreSpy) Search(
	_ context.Context, f db.SearchFilter,
) (db.SearchPage, error) {
	s.calls = append(s.calls, f)
	return s.page, s.err
}

func newPhase21DirectSearchSvc(
	t *testing.T,
) (service.SessionService, *phase21SearchStoreSpy) {
	t.Helper()
	spy := &phase21SearchStoreSpy{hasFTS: true}
	return service.NewReadOnlyBackend(spy), spy
}

// phase21QueryCapture is a stub daemon that records the query string of
// every request it serves and replies with a fixed search response.
type phase21QueryCapture struct {
	queries []url.Values
	paths   []string
	status  int
	body    any
}

func (c *phase21QueryCapture) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			c.paths = append(c.paths, r.URL.Path)
			c.queries = append(c.queries, r.URL.Query())
			status := c.status
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			body := c.body
			if body == nil {
				body = map[string]any{
					"query": "x", "results": []any{}, "count": 0, "next": 0,
				}
			}
			_ = json.NewEncoder(w).Encode(body)
		}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// ---------------------------------------------------------------------
// Query validation: both transports must reject the same input, locally,
// before touching the store or the network.
// ---------------------------------------------------------------------

func TestPhase21DirectSearchRequiresQuery(t *testing.T) {
	t.Parallel()
	for _, q := range []string{"", "   ", "\t\n"} {
		svc, spy := newPhase21DirectSearchSvc(t)
		res, err := svc.Search(context.Background(), service.SearchRequest{Query: q})
		require.ErrorIs(t, err, service.ErrSearchQueryRequired, "query %q", q)
		assert.Nil(t, res)
		assert.Empty(t, spy.calls, "store must not be queried for %q", q)
	}
}

func TestPhase21HTTPSearchRequiresQuery(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	res, err := svc.Search(context.Background(), service.SearchRequest{Query: "  "})
	require.ErrorIs(t, err, service.ErrSearchQueryRequired)
	assert.Nil(t, res)
	assert.Empty(t, cap.paths, "empty query must not reach the daemon")
}

// ---------------------------------------------------------------------
// Limit clamping: the service applies the same clamp the HTTP route does
// (<=0 -> default, >max -> max), so a caller sees one page size on either
// transport instead of db.DB.Search's stricter over-max fold to default.
// ---------------------------------------------------------------------

func TestPhase21DirectSearchClampsLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero means default", 0, db.DefaultSearchLimit},
		{"negative means default", -7, db.DefaultSearchLimit},
		{"in range is preserved", 25, 25},
		{"at max is preserved", db.MaxSearchLimit, db.MaxSearchLimit},
		{"over max clamps to max", db.MaxSearchLimit * 20, db.MaxSearchLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, spy := newPhase21DirectSearchSvc(t)
			_, err := svc.Search(context.Background(), service.SearchRequest{
				Query: "needle", Limit: tc.in,
			})
			require.NoError(t, err)
			require.Len(t, spy.calls, 1)
			assert.Equal(t, tc.want, spy.calls[0].Limit)
		})
	}
}

func TestPhase21HTTPSearchClampsLimit(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	_, err := svc.Search(context.Background(), service.SearchRequest{
		Query: "needle", Limit: db.MaxSearchLimit * 20,
	})
	require.NoError(t, err)
	require.Len(t, cap.queries, 1)
	assert.Equal(t, strconv.Itoa(db.MaxSearchLimit),
		cap.queries[0].Get("limit"))
}

// ---------------------------------------------------------------------
// Forwarding: every field of SearchRequest has to reach the store and the
// wire. Dropping one is the "half-wired" failure this phase keeps hitting:
// it compiles, the tests pass, and the filter silently does nothing.
// ---------------------------------------------------------------------

func TestPhase21DirectSearchForwardsEveryField(t *testing.T) {
	t.Parallel()
	svc, spy := newPhase21DirectSearchSvc(t)

	_, err := svc.Search(context.Background(), service.SearchRequest{
		Query:   "needle haystack",
		Project: "my-app",
		Sort:    "recency",
		Limit:   7,
		Cursor:  42,
	})
	require.NoError(t, err)
	require.Len(t, spy.calls, 1)
	got := spy.calls[0]
	assert.Equal(t, db.PrepareFTSQuery("needle haystack"), got.Query,
		"direct search must canonicalise the query the same way the route does")
	assert.Equal(t, "my-app", got.Project)
	assert.Equal(t, "recency", got.Sort)
	assert.Equal(t, 7, got.Limit)
	assert.Equal(t, 42, got.Cursor)
}

func TestPhase21HTTPSearchForwardsEveryQueryParam(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	_, err := svc.Search(context.Background(), service.SearchRequest{
		Query:   "needle haystack",
		Project: "my-app",
		Sort:    "recency",
		Limit:   7,
		Cursor:  42,
	})
	require.NoError(t, err)
	require.Len(t, cap.queries, 1)
	assert.Equal(t, "/api/v1/search", cap.paths[0])
	assert.Equal(t, url.Values{
		"q":       {"needle haystack"},
		"project": {"my-app"},
		"sort":    {"recency"},
		"limit":   {"7"},
		"cursor":  {"42"},
	}, cap.queries[0])
}

// TestPhase21HTTPSearchOmitsUnsetParams keeps the daemon on its own
// defaults (sort=relevance, cursor=0) instead of pinning them from the
// client, while the clamped limit is always explicit.
func TestPhase21HTTPSearchOmitsUnsetParams(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	_, err := svc.Search(context.Background(), service.SearchRequest{Query: "needle"})
	require.NoError(t, err)
	require.Len(t, cap.queries, 1)
	assert.Equal(t, url.Values{
		"q":     {"needle"},
		"limit": {strconv.Itoa(db.DefaultSearchLimit)},
	}, cap.queries[0])
}

// ---------------------------------------------------------------------
// Unavailable sentinel.
// ---------------------------------------------------------------------

func TestPhase21DirectSearchUnavailableWithoutFTS(t *testing.T) {
	t.Parallel()
	spy := &phase21SearchStoreSpy{hasFTS: false}
	svc := service.NewReadOnlyBackend(spy)

	res, err := svc.Search(context.Background(), service.SearchRequest{Query: "needle"})
	require.ErrorIs(t, err, service.ErrSearchUnavailable)
	assert.Nil(t, res)
	assert.Empty(t, spy.calls, "store without FTS must not be queried")
}

func TestPhase21HTTPSearchUnavailableOnNotImplemented(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{
		status: http.StatusNotImplemented,
		body:   map[string]any{"detail": "search not available"},
	}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	res, err := svc.Search(context.Background(), service.SearchRequest{Query: "needle"})
	require.ErrorIs(t, err, service.ErrSearchUnavailable,
		"501 from the daemon is the wire form of ErrSearchUnavailable")
	assert.Nil(t, res)
}

// TestPhase21HTTPSearchOtherStatusIsNotUnavailable guards the sentinel
// from swallowing unrelated failures.
func TestPhase21HTTPSearchOtherStatusIsNotUnavailable(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{status: http.StatusInternalServerError}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	_, err := svc.Search(context.Background(), service.SearchRequest{Query: "needle"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, service.ErrSearchUnavailable)
}

// ---------------------------------------------------------------------
// Empty-result shape and transport parity against a real server.
// ---------------------------------------------------------------------

func TestPhase21DirectSearchEmptyResultsAreNotNil(t *testing.T) {
	t.Parallel()
	svc, _ := newPhase21DirectSearchSvc(t)

	res, err := svc.Search(context.Background(), service.SearchRequest{Query: "nothing"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotNil(t, res.Results, "empty page must encode as [], not null")
	assert.Empty(t, res.Results)
	assert.Equal(t, 0, res.Count)
	assert.Equal(t, "nothing", res.Query,
		"echoed query is the caller's text, not the FTS-canonical form")
}

// TestPhase21SearchTransportParity is the consumption-chain gate: the same
// request answered by a local store and by a real HTTP daemon over that
// same store must produce identical results, including pagination.
func TestPhase21SearchTransportParity(t *testing.T) {
	t.Parallel()
	baseURL, d := newHTTPTestServer(t)
	require.True(t, d.HasFTS(), "parity test needs the fts5 build tag")

	seedServiceSearchSession(t, d, "p21-a", "alpha", "the parity needle is here")
	seedServiceSearchSession(t, d, "p21-b", "beta", "another parity needle appears")
	seedServiceSearchSession(t, d, "p21-c", "alpha", "unrelated content")

	direct := service.NewDirectBackend(d, nil)
	remote := service.NewHTTPBackend(baseURL, "", false)

	reqs := []service.SearchRequest{
		{Query: "needle"},
		{Query: "needle", Project: "alpha"},
		{Query: "needle", Sort: "recency"},
		{Query: "needle", Limit: 1},
		{Query: "needle", Limit: 1, Cursor: 1},
		{Query: "no-such-token-anywhere"},
	}
	for _, req := range reqs {
		t.Run(req.Query+"/"+req.Project+"/"+req.Sort, func(t *testing.T) {
			ctx := context.Background()
			want, err := direct.Search(ctx, req)
			require.NoError(t, err)
			got, err := remote.Search(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, want, got, "direct and HTTP disagree for %+v", req)
		})
	}

	res, err := direct.Search(context.Background(), service.SearchRequest{
		Query: "needle", Limit: 1,
	})
	require.NoError(t, err)
	assert.Len(t, res.Results, 1)
	assert.Equal(t, 1, res.NextCursor, "a truncated page must advertise a cursor")
}
