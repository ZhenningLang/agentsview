package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

type phase24SessionServiceSpy struct {
	listFilters          []service.ListFilter
	contentSearchFilters []service.ContentSearchRequest
}

func (s *phase24SessionServiceSpy) Get(
	context.Context, string,
) (*service.SessionDetail, error) {
	return nil, nil
}

func (s *phase24SessionServiceSpy) List(
	_ context.Context, f service.ListFilter,
) (*service.SessionList, error) {
	s.listFilters = append(s.listFilters, f)
	return &service.SessionList{Sessions: []db.Session{}}, nil
}

func (s *phase24SessionServiceSpy) Messages(
	context.Context, string, service.MessageFilter,
) (*service.MessageList, error) {
	return &service.MessageList{}, nil
}

func (s *phase24SessionServiceSpy) ToolCalls(
	context.Context, string,
) (*service.ToolCallList, error) {
	return &service.ToolCallList{}, nil
}

func (s *phase24SessionServiceSpy) Sync(
	context.Context, service.SyncInput,
) (*service.SessionDetail, error) {
	return nil, db.ErrReadOnly
}

func (s *phase24SessionServiceSpy) Watch(
	context.Context, string,
) (<-chan service.Event, error) {
	return make(chan service.Event), nil
}

func (s *phase24SessionServiceSpy) Stats(
	context.Context, service.StatsFilter,
) (*service.SessionStats, error) {
	return &service.SessionStats{}, nil
}

func (s *phase24SessionServiceSpy) Search(
	context.Context, service.SearchRequest,
) (*service.SessionSearchResult, error) {
	return &service.SessionSearchResult{Results: []db.SearchResult{}}, nil
}

func (s *phase24SessionServiceSpy) SearchContent(
	_ context.Context, req service.ContentSearchRequest,
) (*service.ContentSearchResult, error) {
	s.contentSearchFilters = append(s.contentSearchFilters, req)
	return &service.ContentSearchResult{Matches: []db.ContentMatch{}}, nil
}

func (s *phase24SessionServiceSpy) ListSecrets(
	context.Context, service.SecretListFilter,
) (*service.SecretFindingList, error) {
	return &service.SecretFindingList{}, nil
}

func (s *phase24SessionServiceSpy) ScanSecrets(
	context.Context, service.SecretScanInput, func(service.SecretScanProgress),
) (*service.SecretScanSummary, error) {
	return &service.SecretScanSummary{}, nil
}

type phase24UsageStoreSpy struct {
	db.Store
	dailyFilters []db.UsageFilter
}

func (s *phase24UsageStoreSpy) GetDailyUsage(
	_ context.Context, f db.UsageFilter,
) (db.DailyUsageResult, error) {
	s.dailyFilters = append(s.dailyFilters, f)
	return db.DailyUsageResult{
		Daily: []db.DailyUsageEntry{},
		Totals: db.UsageTotals{
			HasCost: true,
		},
		SessionCounts: db.UsageSessionCounts{
			ByProject: map[string]int{},
			ByAgent:   map[string]int{},
		},
	}, nil
}

func newPhase24BranchFilterServer(
	sessions service.SessionService, store db.Store,
) *Server {
	s := &Server{
		cfg:      config.Config{Host: "127.0.0.1"},
		db:       store,
		sessions: sessions,
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func servePhase24BranchFilterRequest(
	t *testing.T, srv *Server, path string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	return w
}

func TestPhase24HumaSessionRoutesParseGitBranchIntoFilters(t *testing.T) {
	t.Parallel()
	token := db.EncodeBranchFilterToken("alpha", "main")
	sessions := &phase24SessionServiceSpy{}
	srv := newPhase24BranchFilterServer(sessions, &phase24UsageStoreSpy{})

	w := servePhase24BranchFilterRequest(t, srv,
		"/api/v1/sessions?git_branch="+url.QueryEscape(token))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Len(t, sessions.listFilters, 1)
	assert.Equal(t, token, sessions.listFilters[0].GitBranch)
}

func TestPhase24HumaSearchContentRouteParsesGitBranchIntoFilter(t *testing.T) {
	t.Parallel()
	token := db.EncodeBranchFilterToken("alpha", "feat,x")
	sessions := &phase24SessionServiceSpy{}
	srv := newPhase24BranchFilterServer(sessions, &phase24UsageStoreSpy{})

	w := servePhase24BranchFilterRequest(t, srv,
		"/api/v1/search/content?pattern=needle&git_branch="+url.QueryEscape(token))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Len(t, sessions.contentSearchFilters, 1)
	assert.Equal(t, token, sessions.contentSearchFilters[0].GitBranch)
}

func TestPhase24HumaUsageRouteParsesGitBranchIntoFilter(t *testing.T) {
	t.Parallel()
	token := db.EncodeBranchFilterToken("alpha", "main")
	usage := &phase24UsageStoreSpy{}
	srv := newPhase24BranchFilterServer(&phase24SessionServiceSpy{}, usage)

	w := servePhase24BranchFilterRequest(t, srv,
		"/api/v1/usage/summary?from=2024-06-01&to=2024-06-01&git_branch="+url.QueryEscape(token))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Len(t, usage.dailyFilters, 1)
	assert.Equal(t, token, usage.dailyFilters[0].GitBranch)
}
