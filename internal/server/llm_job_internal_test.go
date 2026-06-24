package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
)

type stubRoundTripper func(*http.Request) (*http.Response, error)

func (f stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func enrichStubHTTPClient() *http.Client {
	return &http.Client{Transport: stubRoundTripper(func(req *http.Request) (*http.Response, error) {
		body := `{}`
		if strings.HasSuffix(req.URL.Path, "/chat/completions") {
			body = `{"choices":[{"message":{"content":"{\"title\":\"T\",\"summary\":\"S\",\"keywords\":[\"k\"]}"}}]}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
}

func TestEnrichConfigReady(t *testing.T) {
	base := config.LLMConfig{Enabled: true, BaseURL: "https://x/v1", APIKey: "k", Model: "m"}
	assert.True(t, enrichConfigReady(base))

	disabled := base
	disabled.Enabled = false
	assert.False(t, enrichConfigReady(disabled))

	noKey := base
	noKey.APIKey = ""
	assert.False(t, enrichConfigReady(noKey))

	noModel := base
	noModel.Model = "  "
	assert.False(t, enrichConfigReady(noModel))
}

// TestPeriodicEnrichmentRespectsLiveToggle proves the previously inert
// "Run periodically" flag now drives a background job, and that the
// decision reads the live config so toggling it takes effect without a
// restart.
func TestPeriodicEnrichmentRespectsLiveToggle(t *testing.T) {
	s := testServer(t, 30*time.Second,
		WithLLMHTTPClient(enrichStubHTTPClient()),
		func(s *Server) {
			s.cfg.LLM = config.LLMConfig{
				Enabled:         true,
				BaseURL:         "https://chat.example/v1",
				APIKey:          "secret",
				Model:           "chat-model",
				MinUserMessages: 2,
				Concurrency:     1,
			}
		},
	)
	require.NotNil(t, s.llmWriter)

	first := "s1"
	ended := "2026-06-24T09:00:00Z"
	require.NoError(t, s.llmWriter.UpsertSession(db.Session{
		ID: "s1", Project: "proj", Machine: "test", Agent: "claude",
		FirstMessage: &first, EndedAt: &ended,
		MessageCount: 3, UserMessageCount: 2,
	}))
	require.NoError(t, s.llmWriter.InsertMessages([]db.Message{
		{SessionID: "s1", Ordinal: 0, Role: "user", Content: "need auth help", ContentLength: 14},
		{SessionID: "s1", Ordinal: 1, Role: "assistant", Content: "use tokens", ContentLength: 10},
	}))

	// Periodic disabled (default): a tick must not start a job.
	s.maybeStartPeriodicEnrichment()
	assert.Empty(t, s.enrichJob.snapshot().Source)

	// Enable on the live config; the next tick starts a periodic job.
	s.mu.Lock()
	s.cfg.LLM.Periodic = true
	s.mu.Unlock()
	s.maybeStartPeriodicEnrichment()
	assert.Equal(t, "periodic", s.enrichJob.snapshot().Source)

	require.Eventually(t, func() bool {
		st := s.enrichJob.snapshot()
		return !st.Running && st.Succeeded == 1
	}, 5*time.Second, 20*time.Millisecond)
}
