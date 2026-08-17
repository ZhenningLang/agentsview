package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/service"
)

func seedPhase24BranchSession(
	t *testing.T, d *db.DB, id, project, branch, message string,
) {
	t.Helper()
	dbtest.SeedSession(t, d, id, project, func(s *db.Session) {
		s.GitBranch = branch
		s.MessageCount = 2
		s.UserMessageCount = 2
	})
	dbtest.SeedMessages(t, d,
		dbtest.UserMsg(id, 0, message),
		dbtest.AsstMsg(id, 1, "ack"),
	)
}

func TestPhase24ServiceDirectBranchFilterReachesStore(t *testing.T) {
	t.Parallel()
	d := dbtest.OpenTestDB(t)
	seedPhase24BranchSession(t, d, "alpha-main", "alpha", "main", "needle")
	seedPhase24BranchSession(t, d, "beta-main", "beta", "main", "needle")
	seedPhase24BranchSession(t, d, "alpha-other", "alpha", "feat,x", "needle")

	token := db.EncodeBranchFilterToken("alpha", "main")
	be := service.NewDirectBackend(d, nil)

	list, err := be.List(context.Background(), service.ListFilter{
		GitBranch:      token,
		IncludeOneShot: true,
		Limit:          10,
	})
	require.NoError(t, err)
	require.Len(t, list.Sessions, 1)
	assert.Equal(t, "alpha-main", list.Sessions[0].ID)

	search, err := be.SearchContent(context.Background(), service.ContentSearchRequest{
		Pattern:   "needle",
		GitBranch: token,
		Limit:     10,
	})
	require.NoError(t, err)
	require.Len(t, search.Matches, 1)
	assert.Equal(t, "alpha-main", search.Matches[0].SessionID)
}

func TestPhase24ServiceHTTPBranchFilterQueryRoundTrip(t *testing.T) {
	t.Parallel()
	token := db.EncodeBranchFilterToken("alpha", "feat,x")
	seen := make(chan url.Values, 2)
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			seen <- r.URL.Query()
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/api/v1/sessions":
				_, _ = w.Write([]byte(`{"sessions":[],"total":0}`))
			case "/api/v1/search/content":
				_, _ = w.Write([]byte(`{"matches":[],"next_cursor":0}`))
			default:
				http.NotFound(w, r)
			}
		}))
	defer srv.Close()

	be := service.NewHTTPBackend(srv.URL, "", true)
	_, err := be.List(context.Background(), service.ListFilter{
		GitBranch: token,
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, token, (<-seen).Get("git_branch"))

	_, err = be.SearchContent(context.Background(), service.ContentSearchRequest{
		Pattern:   "needle",
		GitBranch: token,
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, token, (<-seen).Get("git_branch"))
}
