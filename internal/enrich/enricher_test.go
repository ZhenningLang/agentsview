package enrich

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/llm"
)

type mockChatClient struct {
	mu         sync.Mutex
	calls      int
	embedCalls int
	responses  []string
	errs       []error
	embeddings [][]float32
	embedErrs  []error
}

func (m *mockChatClient) ChatJSON(context.Context, string, string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.calls
	m.calls++
	if idx < len(m.errs) && m.errs[idx] != nil {
		return "", m.errs[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return `{"title":"title","summary":"summary","keywords":["auth"]}`, nil
}

func (m *mockChatClient) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *mockChatClient) Embed(_ context.Context, _ string) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.embedCalls
	m.embedCalls++
	if idx < len(m.embedErrs) && m.embedErrs[idx] != nil {
		return nil, m.embedErrs[idx]
	}
	if idx < len(m.embeddings) {
		return m.embeddings[idx], nil
	}
	return []float32{1}, nil
}

func (m *mockChatClient) EmbedCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.embedCalls
}

func TestEnricherDisabledDoesNotQueryOrCallClient(t *testing.T) {
	d := dbTestDB(t)
	client := &mockChatClient{}
	stats, err := New(d, client, config.LLMConfig{Enabled: false}).Run(context.Background(), Options{})
	require.NoError(t, err)
	assert.True(t, stats.Disabled)
	assert.Zero(t, client.Calls())
}

func TestEnricherRunSuccessWritesEnrichment(t *testing.T) {
	d := dbTestDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	insertEnrichSession(t, d, "s1", func(s *db.Session) {
		s.MessageCount = 3
		s.UserMessageCount = 2
		s.EndedAt = &ended
	})
	insertEnrichMessages(t, d,
		db.Message{SessionID: "s1", Ordinal: 0, Role: "user", Content: "Need auth help", ContentLength: 14},
		db.Message{SessionID: "s1", Ordinal: 1, Role: "assistant", Content: "Use token login", ContentLength: 15},
	)
	client := &mockChatClient{responses: []string{`{"title":"Auth tokens","summary":"Discussed login tokens.","keywords":["auth","token"]}`}}
	stats, err := New(d, client, config.LLMConfig{Enabled: true, Model: "deepseek-chat", MinUserMessages: 2, Concurrency: 1}).Run(ctx, Options{Now: time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Candidates)
	assert.Equal(t, 1, stats.Succeeded)
	assert.Equal(t, 1, client.Calls())
	s, err := d.GetSession(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "Auth tokens", s.LLMTitle)
	assert.Equal(t, "auth,token", s.LLMKeywords)
	assert.Equal(t, "deepseek-chat", s.EnrichModel)
	assert.Equal(t, db.EnrichStatusOK, s.EnrichStatus)
	assert.Equal(t, 3, s.EnrichedMsgCount)
}

func TestEnricherRunReportsProgress(t *testing.T) {
	d := dbTestDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	const n = 4
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("p%d", i)
		insertEnrichSession(t, d, id, func(s *db.Session) {
			s.MessageCount = 3
			s.UserMessageCount = 2
			s.EndedAt = &ended
		})
		insertEnrichMessages(t, d,
			db.Message{SessionID: id, Ordinal: 0, Role: "user", Content: "Need auth help", ContentLength: 14},
			db.Message{SessionID: id, Ordinal: 1, Role: "assistant", Content: "Use token login", ContentLength: 15},
		)
	}

	var mu sync.Mutex
	var updates [][2]int
	stats, err := New(d, &mockChatClient{}, config.LLMConfig{
		Enabled: true, Model: "m", MinUserMessages: 2, Concurrency: 2,
	}).Run(ctx, Options{
		Now: time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		OnProgress: func(done, total int) {
			mu.Lock()
			updates = append(updates, [2]int{done, total})
			mu.Unlock()
		},
	})
	require.NoError(t, err)
	assert.Equal(t, n, stats.Candidates)
	assert.Equal(t, n, stats.Succeeded)

	require.NotEmpty(t, updates)
	// First callback announces the total with zero done; the last
	// reports completion. The callback fires under the stats mutex so
	// done advances monotonically and never exceeds total.
	assert.Equal(t, [2]int{0, n}, updates[0])
	assert.Equal(t, [2]int{n, n}, updates[len(updates)-1])
	for _, u := range updates {
		assert.Equal(t, n, u[1])
		assert.LessOrEqual(t, u[0], n)
	}
}

func TestEnricherWritesEmbeddingWhenConfigured(t *testing.T) {
	d := dbTestDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	insertEnrichSession(t, d, "s1", func(s *db.Session) {
		s.MessageCount = 3
		s.UserMessageCount = 2
		s.EndedAt = &ended
	})
	insertEnrichMessages(t, d,
		db.Message{SessionID: "s1", Ordinal: 0, Role: "user", Content: "Need auth help", ContentLength: 14},
		db.Message{SessionID: "s1", Ordinal: 1, Role: "assistant", Content: "Use token login", ContentLength: 15},
	)
	client := &mockChatClient{
		responses:  []string{`{"title":"Auth tokens","summary":"Discussed login tokens.","keywords":["auth","token"]}`},
		embeddings: [][]float32{{0.1, 0.9}},
	}
	stats, err := New(d, client, config.LLMConfig{
		Enabled: true, Model: "deepseek-chat", MinUserMessages: 2, Concurrency: 1,
		Embed: config.LLMEmbedConfig{Model: "text-embedding"},
	}).Run(ctx, Options{Now: time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Succeeded)
	assert.Equal(t, 1, client.EmbedCalls())

	embeddings, err := d.SessionEmbeddings(ctx, db.EmbeddingFilter{})
	require.NoError(t, err)
	require.Len(t, embeddings, 1)
	assert.Equal(t, []float32{0.1, 0.9}, embeddings[0].Vector)
}

func TestEnricherEmbeddingUnsupportedDoesNotFailTextEnrichment(t *testing.T) {
	d := dbTestDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	insertEnrichSession(t, d, "s1", func(s *db.Session) {
		s.MessageCount = 2
		s.UserMessageCount = 1
		s.EndedAt = &ended
	})
	insertEnrichMessages(t, d, db.Message{SessionID: "s1", Ordinal: 0, Role: "user", Content: "useful content", ContentLength: 14})
	client := &mockChatClient{embedErrs: []error{llm.ErrEmbeddingsUnsupported}}
	stats, err := New(d, client, config.LLMConfig{
		Enabled: true, MinUserMessages: 1, Concurrency: 1,
		Embed: config.LLMEmbedConfig{Model: "text-embedding"},
	}).Run(ctx, Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Succeeded)
	assert.Equal(t, 1, client.EmbedCalls())
	s, err := d.GetSession(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, db.EnrichStatusOK, s.EnrichStatus)
	assert.Zero(t, s.LLMEmbeddingDim)
}

func TestEnricherNonFiniteEmbeddingDoesNotFailTextEnrichment(t *testing.T) {
	d := dbTestDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	insertEnrichSession(t, d, "s1", func(s *db.Session) {
		s.MessageCount = 2
		s.UserMessageCount = 1
		s.EndedAt = &ended
	})
	insertEnrichMessages(t, d, db.Message{SessionID: "s1", Ordinal: 0, Role: "user", Content: "useful content", ContentLength: 14})
	client := &mockChatClient{embeddings: [][]float32{{float32(math.NaN())}}}
	stats, err := New(d, client, config.LLMConfig{
		Enabled: true, MinUserMessages: 1, Concurrency: 1,
		Embed: config.LLMEmbedConfig{Model: "text-embedding"},
	}).Run(ctx, Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Succeeded)
	assert.Equal(t, 1, client.EmbedCalls())
	s, err := d.GetSession(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, db.EnrichStatusOK, s.EnrichStatus)
	assert.Zero(t, s.LLMEmbeddingDim)
}

func TestEnricherNoContentDoesNotCallClient(t *testing.T) {
	d := dbTestDB(t)
	ended := "2026-06-24T09:00:00Z"
	insertEnrichSession(t, d, "s1", func(s *db.Session) {
		s.MessageCount = 1
		s.UserMessageCount = 1
		s.EndedAt = &ended
	})
	insertEnrichMessages(t, d, db.Message{SessionID: "s1", Ordinal: 0, Role: "user", Content: "短", ContentLength: 3})
	client := &mockChatClient{}
	stats, err := New(d, client, config.LLMConfig{Enabled: true, MinUserMessages: 1, Concurrency: 1}).Run(context.Background(), Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.NoContent)
	assert.Zero(t, client.Calls())
	s, err := d.GetSession(context.Background(), "s1")
	require.NoError(t, err)
	assert.Equal(t, db.EnrichStatusNoContent, s.EnrichStatus)
}

func TestEnricherFailureIsolation(t *testing.T) {
	d := dbTestDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	for _, id := range []string{"s1", "s2"} {
		insertEnrichSession(t, d, id, func(s *db.Session) {
			s.MessageCount = 2
			s.UserMessageCount = 1
			s.EndedAt = &ended
		})
		insertEnrichMessages(t, d, db.Message{SessionID: id, Ordinal: 0, Role: "user", Content: "useful content", ContentLength: 14})
	}
	client := &mockChatClient{
		errs:      []error{fmt.Errorf("provider failed secret-key")},
		responses: []string{"", `{"title":"Second","summary":"ok","keywords":["second"]}`},
	}
	stats, err := New(d, client, config.LLMConfig{Enabled: true, APIKey: "secret-key", MinUserMessages: 1, Concurrency: 1}).Run(ctx, Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Failed)
	assert.Equal(t, 1, stats.Succeeded)
	assert.Equal(t, 2, client.Calls())
	s1, err := d.GetSession(ctx, "s1")
	require.NoError(t, err)
	s2, err := d.GetSession(ctx, "s2")
	require.NoError(t, err)
	statuses := map[string]int{s1.EnrichStatus: 1, s2.EnrichStatus: 1}
	assert.Equal(t, 1, statuses[db.EnrichStatusError])
	assert.Equal(t, 1, statuses[db.EnrichStatusOK])
	for _, sess := range []*db.Session{s1, s2} {
		assert.NotContains(t, sess.EnrichError, "secret-key")
	}
}

func TestEnricherParseFailureIsolation(t *testing.T) {
	d := dbTestDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	for _, id := range []string{"s1", "s2"} {
		insertEnrichSession(t, d, id, func(s *db.Session) {
			s.MessageCount = 2
			s.UserMessageCount = 1
			s.EndedAt = &ended
		})
		insertEnrichMessages(t, d, db.Message{SessionID: id, Ordinal: 0, Role: "user", Content: "useful content", ContentLength: 14})
	}
	client := &mockChatClient{
		responses: []string{`{"summary":"missing title","keywords":["broken"]}`, `{"title":"Second","summary":"ok","keywords":["second"]}`},
	}
	stats, err := New(d, client, config.LLMConfig{Enabled: true, MinUserMessages: 1, Concurrency: 1}).Run(ctx, Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Failed)
	assert.Equal(t, 1, stats.Succeeded)
	assert.Equal(t, 2, client.Calls())
	s1, err := d.GetSession(ctx, "s1")
	require.NoError(t, err)
	s2, err := d.GetSession(ctx, "s2")
	require.NoError(t, err)
	statuses := map[string]int{s1.EnrichStatus: 1, s2.EnrichStatus: 1}
	assert.Equal(t, 1, statuses[db.EnrichStatusError])
	assert.Equal(t, 1, statuses[db.EnrichStatusOK])
}

func TestSanitizeErrorTruncatesUTF8Safely(t *testing.T) {
	msg := sanitizeError(errors.New(strings.Repeat("界", 400)))
	assert.True(t, utf8.ValidString(msg))
	assert.Len(t, []rune(msg), 300)
}

func dbTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func insertEnrichSession(t *testing.T, d *db.DB, id string, opts ...func(*db.Session)) {
	t.Helper()
	s := db.Session{ID: id, Project: "proj", Machine: "local", Agent: "codex", MessageCount: 1, UserMessageCount: 1}
	for _, opt := range opts {
		opt(&s)
	}
	require.NoError(t, d.UpsertSession(s))
}

func insertEnrichMessages(t *testing.T, d *db.DB, msgs ...db.Message) {
	t.Helper()
	require.NoError(t, d.InsertMessages(msgs))
}
