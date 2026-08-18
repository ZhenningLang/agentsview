package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/insight"
	"go.kenn.io/agentsview/internal/server"
)

type listInsightsResponse struct {
	Insights []db.Insight `json:"insights"`
}

type failFirstWriteRecorder struct {
	header  http.Header
	writes  int
	status  int
	flushed bool
}

func newFailFirstWriteRecorder() *failFirstWriteRecorder {
	return &failFirstWriteRecorder{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (f *failFirstWriteRecorder) Header() http.Header {
	return f.header
}

func (f *failFirstWriteRecorder) WriteHeader(statusCode int) {
	f.status = statusCode
}

func (f *failFirstWriteRecorder) Write(b []byte) (int, error) {
	f.writes++
	if f.writes == 1 {
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}

func (f *failFirstWriteRecorder) Flush() {
	f.flushed = true
}

func TestListInsights(t *testing.T) {
	tests := []struct {
		name       string
		seed       func(t *testing.T, te *testEnv)
		path       string
		wantStatus int
		wantCount  int
		wantBody   string
	}{
		{
			name:       "Empty",
			seed:       func(t *testing.T, te *testEnv) {},
			path:       "/api/v1/insights",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "WithData",
			seed: func(t *testing.T, te *testEnv) {
				te.seedInsight(t, "daily_activity", "2025-01-15", new("my-app"))
				te.seedInsight(t, "daily_activity", "2025-01-15", new("other-app"))
				te.seedInsight(t, "agent_analysis", "2025-01-15", nil)
			},
			path:       "/api/v1/insights",
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name: "TypeFilter",
			seed: func(t *testing.T, te *testEnv) {
				te.seedInsight(t, "daily_activity", "2025-01-15", new("my-app"))
				te.seedInsight(t, "agent_analysis", "2025-01-15", nil)
			},
			path:       "/api/v1/insights?type=daily_activity",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "ReturnsAll",
			seed: func(t *testing.T, te *testEnv) {
				te.seedInsight(t, "daily_activity", "2025-01-15", new("my-app"))
				te.seedInsight(t, "daily_activity", "2025-01-16", new("my-app"))
			},
			path:       "/api/v1/insights",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "InvalidType",
			seed:       func(t *testing.T, te *testEnv) {},
			path:       "/api/v1/insights?type=invalid",
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			tt.seed(t, te)

			w := te.get(t, tt.path)
			assertStatus(t, w, tt.wantStatus)

			if tt.wantBody != "" {
				assertBodyContains(t, w, tt.wantBody)
			}

			if tt.wantStatus == http.StatusOK {
				r := decode[listInsightsResponse](t, w)
				require.Len(t, r.Insights, tt.wantCount)
			}
		})
	}
}

func TestGetInsight_Found(t *testing.T) {
	te := setup(t)

	id := te.seedInsight(t, "daily_activity", "2025-01-15",
		new("my-app"))

	w := te.get(t, fmt.Sprintf("/api/v1/insights/%d", id))
	assertStatus(t, w, http.StatusOK)

	r := decode[db.Insight](t, w)
	require.Equal(t, id, r.ID)
	assert.Equal(t, "daily_activity", r.Type)
}

func TestGenerateInsight_Validation(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantBody string
	}{
		{"InvalidType", `{"type":"bad","date_from":"2025-01-15","date_to":"2025-01-15"}`, ""},
		{"InvalidDateFrom", `{"type":"daily_activity","date_from":"bad","date_to":"2025-01-15"}`, "date_from"},
		{"InvalidDateTo", `{"type":"daily_activity","date_from":"2025-01-15","date_to":"bad"}`, "date_to"},
		{"DateToBeforeDateFrom", `{"type":"daily_activity","date_from":"2025-01-16","date_to":"2025-01-15"}`, "date_to must be"},
		{"InvalidJSON", `{bad json`, ""},
		{"InvalidAgent", `{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"gpt"}`, "invalid agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			w := te.post(t, "/api/v1/insights/generate", tt.payload)

			assertStatus(t, w, http.StatusBadRequest)
			if tt.wantBody != "" {
				assertBodyContains(t, w, tt.wantBody)
			}
		})
	}
}

func TestGenerateInsight_DefaultAgent(t *testing.T) {
	stubGen := func(
		_ context.Context, agent, _ string,
	) (insight.Result, error) {
		assert.Equal(t, "claude", agent, "expected default agent claude")
		return insight.Result{}, fmt.Errorf("stub: no CLI")
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateFunc(stubGen),
	})

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15"}`)
	assertStatus(t, w, http.StatusOK)
	assertBodyContains(t, w, "event: error")
	assertBodyContains(t, w, "stub: no CLI")
}

func TestGenerateInsight_ErrorMessageStripsStderr(t *testing.T) {
	stubGen := func(
		_ context.Context, _, _ string,
	) (insight.Result, error) {
		return insight.Result{}, fmt.Errorf(
			"claude CLI failed: exit status 1\nstderr: some debug output",
		)
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateFunc(stubGen),
	})

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15"}`)
	assertStatus(t, w, http.StatusOK)
	body := w.Body.String()
	require.Contains(t, body, "claude CLI failed: exit status 1")
	require.NotContains(t, body, "some debug output",
		"expected stderr to be stripped from client message")
}

func TestGenerateInsight_ErrorMessageStripsRaw(t *testing.T) {
	stubGen := func(
		_ context.Context, _, _ string,
	) (insight.Result, error) {
		return insight.Result{}, fmt.Errorf(
			"claude returned empty result\nraw: {\"type\":\"result\",\"result\":\"\"}",
		)
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateFunc(stubGen),
	})

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15"}`)
	assertStatus(t, w, http.StatusOK)
	body := w.Body.String()
	require.Contains(t, body, "claude returned empty result")
	require.NotContains(t, body, `"type":"result"`,
		"expected raw payload to be stripped from client message")
}

func TestGenerateInsight_InitialStatusWriteFailureSkipsGeneration(t *testing.T) {
	var called atomic.Bool
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(func(
			_ context.Context, _ string, _ string, _ insight.LogFunc,
		) (insight.Result, error) {
			called.Store(true)
			return insight.Result{Content: "should not run"}, nil
		}),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")

	w := newFailFirstWriteRecorder()
	te.handler.ServeHTTP(w, req)

	require.False(t, called.Load(),
		"generation should not run when initial SSE status write fails")
}

func TestGenerateInsight_StreamsLogs(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		onLog(insight.LogEvent{
			Stream: "stdout",
			Line:   `{"type":"system","status":"ready"}`,
		})
		onLog(insight.LogEvent{
			Stream: "stderr",
			Line:   "rate limit warning",
		})
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
			Model:   "test-model",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`)
	assertStatus(t, w, http.StatusOK)

	events := parseSSE(w.Body.String())
	require.GreaterOrEqual(t, len(events), 4, "expected >=4 SSE events: %s", w.Body.String())
	require.Equal(t, "status", events[0].Event, "first event")
	require.Equal(t, "log", events[1].Event, "events: %#v", events)
	require.Equal(t, "log", events[2].Event, "events: %#v", events)
	require.Equal(t, "done", events[len(events)-1].Event, "last event")

	var log1 insight.LogEvent
	require.NoError(t, json.Unmarshal([]byte(events[1].Data), &log1))
	require.Equal(t, "stdout", log1.Stream)

	var log2 insight.LogEvent
	require.NoError(t, json.Unmarshal([]byte(events[2].Data), &log2))
	require.Equal(t, "stderr", log2.Stream)
}

type slowFlushRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
	mu    sync.Mutex
}

func (f *slowFlushRecorder) Write(
	b []byte,
) (int, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(b)
}

func (f *slowFlushRecorder) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResponseRecorder.Flush()
}

func (f *slowFlushRecorder) BodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Body.String()
}

type slowLogRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
	mu    sync.Mutex
}

func (f *slowLogRecorder) Write(
	b []byte,
) (int, error) {
	if strings.HasPrefix(string(b), "event: log\n") {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(b)
}

func (f *slowLogRecorder) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResponseRecorder.Flush()
}

func (f *slowLogRecorder) BodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Body.String()
}

type blockingLogRecorder struct {
	*httptest.ResponseRecorder
	release <-chan struct{}
	mu      sync.Mutex
}

func (f *blockingLogRecorder) Write(
	b []byte,
) (int, error) {
	if strings.HasPrefix(string(b), "event: log\n") {
		<-f.release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(b)
}

func (f *blockingLogRecorder) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResponseRecorder.Flush()
}

func (f *blockingLogRecorder) BodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Body.String()
}

type firstLogDelayRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
	once  sync.Once
	mu    sync.Mutex
}

func (f *firstLogDelayRecorder) Write(
	b []byte,
) (int, error) {
	if strings.HasPrefix(string(b), "event: log\n") {
		f.once.Do(func() {
			time.Sleep(f.delay)
		})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(b)
}

func (f *firstLogDelayRecorder) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResponseRecorder.Flush()
}

func (f *firstLogDelayRecorder) BodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Body.String()
}

type deadlineAwareBlockingLogRecorder struct {
	*httptest.ResponseRecorder
	handlerReturned     <-chan struct{}
	postReturnWrites    atomic.Int32
	postReturnAttempted chan struct{}
	deadlineUpdates     chan struct{}
	mu                  sync.Mutex
	writeDeadline       time.Time
}

func newDeadlineAwareBlockingLogRecorder(
	handlerReturned <-chan struct{},
) *deadlineAwareBlockingLogRecorder {
	return &deadlineAwareBlockingLogRecorder{
		ResponseRecorder:    httptest.NewRecorder(),
		handlerReturned:     handlerReturned,
		postReturnAttempted: make(chan struct{}, 1),
		deadlineUpdates:     make(chan struct{}, 1),
	}
}

func (f *deadlineAwareBlockingLogRecorder) SetWriteDeadline(t time.Time) error {
	f.mu.Lock()
	f.writeDeadline = t
	f.mu.Unlock()
	select {
	case f.deadlineUpdates <- struct{}{}:
	default:
	}
	return nil
}

func (f *deadlineAwareBlockingLogRecorder) Write(
	b []byte,
) (int, error) {
	if f.handlerReturned != nil {
		select {
		case <-f.handlerReturned:
			f.postReturnWrites.Add(1)
			select {
			case f.postReturnAttempted <- struct{}{}:
			default:
			}
		default:
		}
	}

	if strings.HasPrefix(string(b), "event: log\n") {
		for {
			f.mu.Lock()
			deadline := f.writeDeadline
			f.mu.Unlock()
			if !deadline.IsZero() && !deadline.After(time.Now()) {
				return 0, os.ErrDeadlineExceeded
			}
			<-f.deadlineUpdates
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(b)
}

func (f *deadlineAwareBlockingLogRecorder) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResponseRecorder.Flush()
}

func (f *deadlineAwareBlockingLogRecorder) PostReturnWrites() int32 {
	return f.postReturnWrites.Load()
}

func (f *deadlineAwareBlockingLogRecorder) PostReturnAttempted() <-chan struct{} {
	return f.postReturnAttempted
}

func (f *deadlineAwareBlockingLogRecorder) BodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Body.String()
}

func TestGenerateInsight_LogDropSummaryAndCompletion(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		for i := range 5000 {
			onLog(insight.LogEvent{
				Stream: "stdout",
				Line:   fmt.Sprintf("line-%d", i),
			})
		}
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	w := &slowFlushRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		delay:            4 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(8 * time.Second):
		require.Fail(t, "timed out waiting for generate handler")
	}

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.BodyString())

	foundDone := false
	foundDropSummary := false
	for _, ev := range events {
		if ev.Event == "done" {
			foundDone = true
		}
		if ev.Event != "log" {
			continue
		}
		var line insight.LogEvent
		if json.Unmarshal([]byte(ev.Data), &line) != nil {
			continue
		}
		if line.Stream == "stderr" &&
			strings.Contains(line.Line, "dropped ") &&
			strings.Contains(line.Line, "slow client") {
			foundDropSummary = true
		}
	}
	require.True(t, foundDropSummary,
		"expected dropped-log summary event, got %d events", len(events))
	require.True(t, foundDone, "expected done event")
}

func TestGenerateInsight_LogDrainTimeoutReturnsWithoutHang(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		for i := range 10 {
			onLog(insight.LogEvent{
				Stream: "stdout",
				Line:   fmt.Sprintf("slow-line-%d", i),
			})
		}
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	w := &slowLogRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		delay:            5 * time.Second,
	}

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(12 * time.Second):
		require.Fail(t, "timed out waiting for generate handler completion")
	}
	require.LessOrEqual(t, time.Since(started), 7*time.Second,
		"handler should return within bounded timeout handling")

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.BodyString())
	for _, ev := range events {
		require.NotEqual(t, "done", ev.Event,
			"did not expect done event when timeout path is triggered")
	}
}

func TestGenerateInsight_LogDrainTimeoutReportsBufferedDrops(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		for i := range 10 {
			onLog(insight.LogEvent{
				Stream: "stdout",
				Line:   fmt.Sprintf("slow-line-%d", i),
			})
		}
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	w := &firstLogDelayRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		delay:            2200 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		require.Fail(t, "timed out waiting for generate handler completion")
	}

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.BodyString())
	foundTimeoutError := false
	foundDropSummary := false
	for _, ev := range events {
		require.NotEqual(t, "done", ev.Event,
			"did not expect done event when timeout path is triggered")
		if ev.Event == "error" &&
			strings.Contains(ev.Data, "timed out before completion") {
			foundTimeoutError = true
		}
		if ev.Event != "log" {
			continue
		}
		var line insight.LogEvent
		if json.Unmarshal([]byte(ev.Data), &line) != nil {
			continue
		}
		if line.Stream != "stderr" ||
			!strings.HasPrefix(line.Line, "dropped ") ||
			!strings.Contains(line.Line, "log stream timeout") {
			continue
		}
		parts := strings.SplitN(line.Line, " ", 3)
		if len(parts) < 3 {
			continue
		}
		dropped, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		// 10 events were enqueued; timeout truncation should account
		// for most buffered entries that were never flushed.
		require.GreaterOrEqual(t, dropped, 8,
			"expected timeout drop summary >=8 (%q)", line.Line)
		foundDropSummary = true
	}
	require.True(t, foundTimeoutError,
		"expected timeout error event, got %d events", len(events))
	require.True(t, foundDropSummary,
		"expected timeout-aware drop summary, got %d events", len(events))
}

func TestGenerateInsight_LogDrainTimeoutBoundedWhenWriterStuck(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		onLog(insight.LogEvent{Stream: "stdout", Line: "stuck-line"})
		return insight.Result{Content: "# Insight", Agent: "claude"}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	release := make(chan struct{})
	w := &blockingLogRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		release:          release,
	}

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(7 * time.Second):
		require.Fail(t, "timed out waiting for bounded timeout behavior")
	}
	elapsed := time.Since(started)
	require.LessOrEqual(t, elapsed, 6*time.Second,
		"handler returned too slowly for stuck writer path")
	close(release)

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.BodyString())
	for _, ev := range events {
		require.NotEqual(t, "done", ev.Event,
			"did not expect done event on stuck writer timeout path")
	}
}

func TestGenerateInsight_LogDrainTimeoutForceUnblocksAndNoPostReturnWrites(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		onLog(insight.LogEvent{Stream: "stdout", Line: "force-unblock-line"})
		return insight.Result{Content: "# Insight", Agent: "claude"}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	handlerReturned := make(chan struct{})
	w := newDeadlineAwareBlockingLogRecorder(handlerReturned)

	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
		close(handlerReturned)
	}()

	select {
	case <-done:
	case <-time.After(8 * time.Second):
		require.Fail(t, "timed out waiting for forced-unblock completion")
	}

	select {
	case <-w.PostReturnAttempted():
		require.Fail(t, "expected no writes after handler return")
	case <-time.After(300 * time.Millisecond):
	}
	require.Zero(t, w.PostReturnWrites(), "expected no writes after handler return")

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.BodyString())
	foundTimeoutError := false
	for _, ev := range events {
		require.NotEqual(t, "done", ev.Event,
			"did not expect done event on forced-unblock timeout path")
		if ev.Event == "error" &&
			strings.Contains(ev.Data, "timed out before completion") {
			foundTimeoutError = true
		}
	}
	require.True(t, foundTimeoutError, "expected timeout error event")
}

func TestDeleteInsight_Found(t *testing.T) {
	te := setup(t)

	id := te.seedInsight(t, "daily_activity", "2025-01-15",
		new("my-app"))

	w := te.del(t, fmt.Sprintf("/api/v1/insights/%d", id))
	assertStatus(t, w, http.StatusNoContent)

	// Verify it's gone.
	w = te.get(t, fmt.Sprintf("/api/v1/insights/%d", id))
	assertStatus(t, w, http.StatusNotFound)
}

func TestInsight_ResourceErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{"Get_NotFound", http.MethodGet, "/api/v1/insights/99999", http.StatusNotFound},
		{"Get_InvalidID", http.MethodGet, "/api/v1/insights/abc", http.StatusBadRequest},
		{"Delete_NotFound", http.MethodDelete, "/api/v1/insights/99999", http.StatusNotFound},
		{"Delete_InvalidID", http.MethodDelete, "/api/v1/insights/abc", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			if tt.method == http.MethodGet {
				w := te.get(t, tt.path)
				assertStatus(t, w, tt.status)
			} else {
				w := te.del(t, tt.path)
				assertStatus(t, w, tt.status)
			}
		})
	}
}

// --- helpers ---

func (te *testEnv) seedInsight(
	t *testing.T,
	typ, date string,
	project *string,
) int64 {
	t.Helper()
	id, err := te.db.InsertInsight(db.Insight{
		Type:     typ,
		DateFrom: date,
		DateTo:   date,
		Project:  project,
		Agent:    "claude",
		Content:  "Test insight content",
	})
	require.NoError(t, err)
	return id
}

// --- Phase 25: insight HTML / Markdown export routes ---

// phase25EventAttrRe matches an HTML event-handler attribute that lives
// inside a real tag. Escaped text such as
// `&lt;img src=x onerror=alert(1)&gt;` contains neither `<` nor `>`, so a
// zero match count proves hostile input never reopened a tag.
var phase25EventAttrRe = regexp.MustCompile(`<[^>]*\son[a-zA-Z]+\s*=`)

// phase25ChipRe extracts the metadata chips from an exported insight so
// tests can assert the full ordered set, including absent optional chips.
var phase25ChipRe = regexp.MustCompile(
	`<span class="chip">(.*?)</span>`)

func (te *testEnv) seedPhase25Insight(
	t *testing.T, in db.Insight,
) int64 {
	t.Helper()
	id, err := te.db.InsertInsight(in)
	require.NoError(t, err)
	return id
}

func phase25Chips(body string) []string {
	matches := phase25ChipRe.FindAllStringSubmatch(body, -1)
	chips := make([]string, 0, len(matches))
	for _, m := range matches {
		chips = append(chips, m[1])
	}
	return chips
}

func TestPhase25InsightHTMLExport(t *testing.T) {
	tests := []struct {
		name         string
		insight      db.Insight
		wantFilename string
		wantTitle    string
		wantChips    []string
		wantContains []string
	}{
		{
			name: "DailyActivityProjectScopedWithModel",
			insight: db.Insight{
				Type:     "daily_activity",
				DateFrom: "2025-01-15",
				DateTo:   "2025-01-15",
				Project:  new("my-app"),
				Agent:    "claude",
				Model:    new("opus-5"),
				Content:  "Shipped the export route.",
			},
			wantFilename: "insight-daily_activity-my-app-20250115.html",
			wantTitle:    "Daily Activity Insight",
			wantChips: []string{
				"Daily Activity", "my-app", "2025-01-15",
				"Claude Code", "opus-5",
			},
			wantContains: []string{"Shipped the export route."},
		},
		{
			name: "AgentAnalysisGlobalDateRangeNoModel",
			insight: db.Insight{
				Type:     "agent_analysis",
				DateFrom: "2025-01-15",
				DateTo:   "2025-01-20",
				Project:  nil,
				Agent:    "claude",
				Model:    nil,
				Content:  "Agent comparison.",
			},
			wantFilename: "insight-agent_analysis-global-20250115-20250120.html",
			wantTitle:    "Agent Analysis Insight",
			wantChips: []string{
				"Agent Analysis", "global", "2025-01-15 to 2025-01-20",
				"Claude Code",
			},
			wantContains: []string{"Agent comparison."},
		},
		{
			name: "UnknownAgentFallsBackToRawName",
			insight: db.Insight{
				Type:     "daily_activity",
				DateFrom: "2025-02-01",
				DateTo:   "2025-02-01",
				Project:  new("my-app"),
				Agent:    "not-a-registered-agent",
				Content:  "Body.",
			},
			wantFilename: "insight-daily_activity-my-app-20250201.html",
			wantTitle:    "Daily Activity Insight",
			wantChips: []string{
				"Daily Activity", "my-app", "2025-02-01",
				"not-a-registered-agent",
			},
		},
		{
			name: "EmptyProjectStringIsGlobal",
			insight: db.Insight{
				Type:     "daily_activity",
				DateFrom: "2025-03-01",
				DateTo:   "",
				Project:  new("   "),
				Agent:    "claude",
				Content:  "Body.",
			},
			wantFilename: "insight-daily_activity-global-20250301.html",
			wantTitle:    "Daily Activity Insight",
			wantChips: []string{
				"Daily Activity", "global", "2025-03-01", "Claude Code",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			id := te.seedPhase25Insight(t, tt.insight)

			w := te.get(t, fmt.Sprintf(
				"/api/v1/insights/%d/export", id))

			require.Equal(t, http.StatusOK, w.Code,
				"body: %.200s", w.Body.String())
			assert.Equal(t, "text/html; charset=utf-8",
				w.Header().Get("Content-Type"))
			assert.Equal(t,
				fmt.Sprintf(`attachment; filename="%s"`, tt.wantFilename),
				w.Header().Get("Content-Disposition"))

			body := w.Body.String()
			assert.Contains(t, body,
				"<title>"+tt.wantTitle+"</title>")
			assert.Contains(t, body, "<h1>"+tt.wantTitle+"</h1>")
			// The trailing chip is the DB-assigned created_at, so it
			// is pinned by shape rather than by value.
			chips := phase25Chips(body)
			require.Len(t, chips, len(tt.wantChips)+1,
				"chips: %#v", chips)
			assert.Equal(t, tt.wantChips, chips[:len(tt.wantChips)])
			assert.Regexp(t,
				`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`,
				chips[len(chips)-1])
			for _, want := range tt.wantContains {
				assert.Contains(t, body, want)
			}
			assert.Empty(t, phase25EventAttrRe.FindAllString(body, -1),
				"exported insight must not contain event attributes")
		})
	}
}

func TestPhase25InsightHTMLExportEscapesHostileMetadataAndContent(t *testing.T) {
	te := setup(t)
	id := te.seedPhase25Insight(t, db.Insight{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
		Project:  new(`<img src=x onerror=alert(1)>`),
		Agent:    `<svg onload=alert(2)>`,
		Model:    new(`" onmouseover="alert(3)`),
		Content: "<script>alert('xss')</script>\n" +
			"<div onclick=\"steal()\">click</div>\n" +
			"```js\n<script>alert('in-code')</script>\n```\n" +
			"`<b>inline</b>`\n",
	})

	w := te.get(t, fmt.Sprintf("/api/v1/insights/%d/export", id))

	require.Equal(t, http.StatusOK, w.Code,
		"body: %.200s", w.Body.String())
	body := w.Body.String()

	// No hostile input may reopen a tag or introduce a handler.
	assert.Empty(t, phase25EventAttrRe.FindAllString(body, -1),
		"hostile metadata/content must not produce event attributes")
	assert.NotContains(t, body, "<script")
	assert.NotContains(t, body, "<img")
	assert.NotContains(t, body, "<svg")
	assert.NotContains(t, body, "<b>inline</b>")
	assert.NotContains(t, body, `" onmouseover=`)

	// The hostile text must still be visible, escaped.
	assert.Contains(t, body, "&lt;script&gt;")
	assert.Contains(t, body, "&lt;img src=x onerror=alert(1)&gt;")
	assert.Contains(t, body, "&lt;svg onload=alert(2)&gt;")
	assert.Contains(t, body, "&lt;b&gt;inline&lt;/b&gt;")

	// A hostile project name must not escape the filename either.
	disposition := w.Header().Get("Content-Disposition")
	require.True(t,
		strings.HasPrefix(disposition, `attachment; filename="`),
		"disposition: %q", disposition)
	filename := strings.TrimSuffix(
		strings.TrimPrefix(disposition, `attachment; filename="`), `"`)
	assert.True(t, strings.HasSuffix(filename, ".html"),
		"filename: %q", filename)
	assert.Regexp(t, `^[\w.\-]+$`, filename)
}

func TestPhase25InsightHTMLExportNotFoundIsJSON404(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/insights/999999/export")

	require.Equal(t, http.StatusNotFound, w.Code,
		"body: %.200s", w.Body.String())
	assert.NotContains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "insight not found")
}

func TestPhase25InsightMarkdownExport(t *testing.T) {
	tests := []struct {
		name         string
		insight      db.Insight
		wantFilename string
	}{
		{
			name: "ProjectScopedSingleDay",
			insight: db.Insight{
				Type:     "daily_activity",
				DateFrom: "2025-01-15",
				DateTo:   "2025-01-15",
				Project:  new("my-app"),
				Agent:    "claude",
				Model:    new("opus-5"),
				Content: "# Heading\n\n" +
					"<script>alert('raw')</script>\n\n" +
					"```js\nconst a = 1 < 2 && 3 > 2;\n```\n" +
					"中文 & \"quotes\" 'apostrophes'\n",
			},
			wantFilename: "insight-daily_activity-my-app-20250115.md",
		},
		{
			name: "GlobalDateRange",
			insight: db.Insight{
				Type:     "agent_analysis",
				DateFrom: "2025-01-15",
				DateTo:   "2025-01-20",
				Project:  nil,
				Agent:    "claude",
				Content:  "- item\n- item\n",
			},
			wantFilename: "insight-agent_analysis-global-20250115-20250120.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			id := te.seedPhase25Insight(t, tt.insight)

			w := te.get(t, fmt.Sprintf(
				"/api/v1/insights/%d/md", id))

			require.Equal(t, http.StatusOK, w.Code,
				"body: %.200s", w.Body.String())
			assert.Equal(t, "text/markdown; charset=utf-8",
				w.Header().Get("Content-Type"))
			assert.Equal(t,
				fmt.Sprintf(`inline; filename="%s"`, tt.wantFilename),
				w.Header().Get("Content-Disposition"))
			// Byte-for-byte: the Markdown route must never rewrite,
			// escape or re-render the stored content.
			assert.Equal(t, tt.insight.Content, w.Body.String())
		})
	}
}

func TestPhase25InsightMarkdownExportNotFoundIsJSON404(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/insights/999999/md")

	require.Equal(t, http.StatusNotFound, w.Code,
		"body: %.200s", w.Body.String())
	assert.NotContains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "insight not found")
}

// --- Phase 25: insight Gist publish through an injected endpoint ---

// phase25Token is a fixture credential. It is deliberately not shaped
// like a real GitHub token so secret scanners have nothing to match.
const phase25Token = "phase25-token"

type phase25GistFile struct {
	Content string `json:"content"`
}

type phase25GistPayload struct {
	Description string                     `json:"description"`
	Public      bool                       `json:"public"`
	Files       map[string]phase25GistFile `json:"files"`
}

type phase25GistRequest struct {
	Method        string
	UserAgent     string
	Authorization string
	Accept        string
	ContentType   string
	Payload       phase25GistPayload
}

// phase25GistStub is a loopback stand-in for GitHub's Create Gist API.
// Every publish test points the server at one of these, so no test can
// reach api.github.com or create a real gist.
type phase25GistStub struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []phase25GistRequest
}

func newPhase25GistStub(
	t *testing.T, status int, body string,
) *phase25GistStub {
	t.Helper()
	stub := &phase25GistStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			stub.record(t, r)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if body != "" {
				_, _ = io.WriteString(w, body)
			}
		}))
	t.Cleanup(stub.server.Close)
	return stub
}

func (g *phase25GistStub) record(t *testing.T, r *http.Request) {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if !assert.NoError(t, err) {
		return
	}
	rec := phase25GistRequest{
		Method:        r.Method,
		UserAgent:     r.Header.Get("User-Agent"),
		Authorization: r.Header.Get("Authorization"),
		Accept:        r.Header.Get("Accept"),
		ContentType:   r.Header.Get("Content-Type"),
	}
	assert.NoError(t, json.Unmarshal(raw, &rec.Payload),
		"gist payload: %s", raw)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, rec)
}

func (g *phase25GistStub) captured() []phase25GistRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]phase25GistRequest(nil), g.requests...)
}

func (g *phase25GistStub) only(t *testing.T) phase25GistRequest {
	t.Helper()
	got := g.captured()
	require.Len(t, got, 1, "expected exactly one gist request")
	return got[0]
}

// setupPhase25Publish wires a server whose Create Gist endpoint is the
// given loopback stub and whose GitHub token is the fixture token.
func setupPhase25Publish(
	t *testing.T, stub *phase25GistStub,
) *testEnv {
	t.Helper()
	te := setupWithServerOpts(t, []server.Option{
		server.WithGithubGistAPIURL(stub.server.URL),
	})
	te.srv.SetGithubToken(phase25Token)
	return te
}

func phase25DailyInsight() db.Insight {
	return db.Insight{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
		Project:  new("my-app"),
		Agent:    "claude",
		Model:    new("opus-5"),
		Content:  "Shipped the export route.",
	}
}

const phase25GistOKBody = `{
  "id": "gist-id-123",
  "html_url": "https://gist.github.com/example-user/gist-id-123",
  "owner": {"login": "example-user"}
}`

func TestPhase25InsightPublishSuccessSendsAuthenticatedPOST(t *testing.T) {
	stub := newPhase25GistStub(t, http.StatusCreated, phase25GistOKBody)
	te := setupPhase25Publish(t, stub)
	id := te.seedPhase25Insight(t, phase25DailyInsight())

	w := te.post(t, fmt.Sprintf(
		"/api/v1/insights/%d/publish", id), "{}")

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := stub.only(t)
	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "agentsview", got.UserAgent)
	assert.Equal(t, "token "+phase25Token, got.Authorization)
	assert.Equal(t, "application/vnd.github.v3+json", got.Accept)
	assert.Equal(t, "application/json", got.ContentType)
}

func TestPhase25InsightPublishPayloadMatchesHTMLExport(t *testing.T) {
	stub := newPhase25GistStub(t, http.StatusCreated, phase25GistOKBody)
	te := setupPhase25Publish(t, stub)
	id := te.seedPhase25Insight(t, phase25DailyInsight())

	// The gist must carry the same document the export route serves,
	// not a separately rendered near-copy.
	exported := te.get(t, fmt.Sprintf(
		"/api/v1/insights/%d/export", id))
	require.Equal(t, http.StatusOK, exported.Code)

	w := te.post(t, fmt.Sprintf(
		"/api/v1/insights/%d/publish", id), "{}")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	got := stub.only(t)
	assert.Equal(t,
		"Insight: Daily Activity - my-app - 2025-01-15",
		got.Payload.Description)
	require.Len(t, got.Payload.Files, 1)
	file, ok := got.Payload.Files["insight-daily_activity-my-app-20250115.html"]
	require.True(t, ok, "files: %#v", got.Payload.Files)
	assert.Equal(t, exported.Body.String(), file.Content)
}

func TestPhase25InsightPublishVisibilityMapsSecretToPublicFlag(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantPublic bool
	}{
		{"DefaultIsPublic", "", true},
		{"SecretFalseIsPublic", "?secret=false", true},
		// The important direction: a secret publish must never go out
		// as a public gist, which would expose session-derived content.
		{"SecretTrueIsPrivate", "?secret=true", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newPhase25GistStub(
				t, http.StatusCreated, phase25GistOKBody)
			te := setupPhase25Publish(t, stub)
			id := te.seedPhase25Insight(t, phase25DailyInsight())

			w := te.post(t, fmt.Sprintf(
				"/api/v1/insights/%d/publish%s", id, tt.query), "{}")

			require.Equal(t, http.StatusOK, w.Code,
				"body: %s", w.Body.String())
			assert.Equal(t, tt.wantPublic, stub.only(t).Payload.Public)
		})
	}
}

func TestPhase25InsightPublishResultURLs(t *testing.T) {
	stub := newPhase25GistStub(t, http.StatusCreated, phase25GistOKBody)
	te := setupPhase25Publish(t, stub)
	id := te.seedPhase25Insight(t, phase25DailyInsight())

	w := te.post(t, fmt.Sprintf(
		"/api/v1/insights/%d/publish", id), "{}")

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var got struct {
		GistID  string `json:"gist_id"`
		GistURL string `json:"gist_url"`
		ViewURL string `json:"view_url"`
		RawURL  string `json:"raw_url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	rawURL := "https://gist.githubusercontent.com/example-user/" +
		"gist-id-123/raw/insight-daily_activity-my-app-20250115.html"
	assert.Equal(t, "gist-id-123", got.GistID)
	assert.Equal(t,
		"https://gist.github.com/example-user/gist-id-123", got.GistURL)
	assert.Equal(t, rawURL, got.RawURL)
	assert.Equal(t, "https://htmlpreview.github.io/?"+rawURL, got.ViewURL)
}

func TestPhase25InsightPublishNoTokenIs401AndSendsNothing(t *testing.T) {
	stub := newPhase25GistStub(t, http.StatusCreated, phase25GistOKBody)
	te := setupWithServerOpts(t, []server.Option{
		server.WithGithubGistAPIURL(stub.server.URL),
	})
	id := te.seedPhase25Insight(t, phase25DailyInsight())

	w := te.post(t, fmt.Sprintf(
		"/api/v1/insights/%d/publish", id), "{}")

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"body: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), "GitHub token not configured")
	assert.Empty(t, stub.captured(),
		"unauthenticated publish must not reach the gist API")
}

func TestPhase25InsightPublishNotFoundIs404AndSendsNothing(t *testing.T) {
	stub := newPhase25GistStub(t, http.StatusCreated, phase25GistOKBody)
	te := setupPhase25Publish(t, stub)

	w := te.post(t, "/api/v1/insights/999999/publish", "{}")

	require.Equal(t, http.StatusNotFound, w.Code,
		"body: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), "insight not found")
	assert.Empty(t, stub.captured(),
		"a missing insight must not reach the gist API")
}

func TestPhase25InsightPublishGithubErrorIs502(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"Unauthorized", http.StatusUnauthorized, `{"message":"Bad credentials"}`},
		{"Forbidden", http.StatusForbidden, `{"message":"rate limited"}`},
		{"Unprocessable", http.StatusUnprocessableEntity, `{"message":"invalid"}`},
		{"ServerError", http.StatusInternalServerError, `{"message":"boom"}`},
		{"Unavailable", http.StatusServiceUnavailable, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newPhase25GistStub(t, tt.status, tt.body)
			te := setupPhase25Publish(t, stub)
			id := te.seedPhase25Insight(t, phase25DailyInsight())

			w := te.post(t, fmt.Sprintf(
				"/api/v1/insights/%d/publish", id), "{}")

			require.Equal(t, http.StatusBadGateway, w.Code,
				"body: %s", w.Body.String())
			assert.NotContains(t, w.Body.String(), phase25Token,
				"error response must not echo the token")
			assert.Len(t, stub.captured(), 1,
				"a failed publish must not be retried")
		})
	}
}

func TestPhase25InsightPublishIncompleteGistIs502(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"EmptyObject", `{}`},
		{"MissingHTMLURL", `{"id":"gist-id-123"}`},
		{"MissingID", `{"html_url":"https://gist.github.com/x/y"}`},
		{"BlankFields", `{"id":"","html_url":""}`},
		{"NotJSON", `<!doctype html><html></html>`},
		// The owner login is not decoration: it is a path segment of
		// raw_url, so a response without it yields a 200 carrying a
		// broken link instead of an error.
		{"MissingOwner", `{"id":"gist-id-123","html_url":"https://gist.github.com/example-user/gist-id-123"}`},
		{"EmptyOwnerObject", `{"id":"gist-id-123","html_url":"https://gist.github.com/example-user/gist-id-123","owner":{}}`},
		{"BlankOwnerLogin", `{"id":"gist-id-123","html_url":"https://gist.github.com/example-user/gist-id-123","owner":{"login":"   "}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newPhase25GistStub(t, http.StatusCreated, tt.body)
			te := setupPhase25Publish(t, stub)
			id := te.seedPhase25Insight(t, phase25DailyInsight())

			w := te.post(t, fmt.Sprintf(
				"/api/v1/insights/%d/publish", id), "{}")

			require.Equal(t, http.StatusBadGateway, w.Code,
				"body: %s", w.Body.String())
			assert.NotContains(t, w.Body.String(), phase25Token)
			assert.NotContains(t, w.Body.String(),
				"gist.githubusercontent.com",
				"an incomplete response must not produce a raw URL")
		})
	}
}

// TestPhase25InsightPublishGithubErrorBodyNeverReachesClient pins that the
// upstream error body never crosses the API boundary. GitHub (or any proxy in
// front of it) can echo the request back in an error shape, and this 502 body
// is what lands in the UI, the browser console and a user's screenshot, so the
// handler must report a sanitized error rather than relay what it received.
func TestPhase25InsightPublishGithubErrorBodyNeverReachesClient(t *testing.T) {
	tests := []struct {
		name string
		// body echoes the credential the way a careless upstream would.
		body string
		// marker is a distinctive fragment of body that must not appear
		// in the response either: proving only "token absent" would pass
		// for a redactor that misses any other encoding of it.
		marker string
	}{
		{
			"TokenInMessage",
			`{"message":"Bad credentials for token phase25-token"}`,
			"Bad credentials",
		},
		{
			"TokenInEchoedRequest",
			`{"message":"invalid","request":{"Authorization":"token phase25-token"}}`,
			"Authorization",
		},
		{
			"TokenInPlainText",
			"authorization: token phase25-token",
			"authorization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newPhase25GistStub(t, http.StatusUnauthorized, tt.body)
			te := setupPhase25Publish(t, stub)
			id := te.seedPhase25Insight(t, phase25DailyInsight())

			w := te.post(t, fmt.Sprintf(
				"/api/v1/insights/%d/publish", id), "{}")

			require.Equal(t, http.StatusBadGateway, w.Code,
				"body: %s", w.Body.String())
			assert.NotContains(t, w.Body.String(), phase25Token,
				"the response must never carry our credential")
			assert.NotContains(t, w.Body.String(), tt.marker,
				"the upstream body must not be relayed verbatim")
			assert.Contains(t, w.Body.String(), "401",
				"the upstream status is the diagnostic that may cross")
			assert.Len(t, stub.captured(), 1,
				"a failed publish must not be retried")
		})
	}
}

func TestPhase25InsightPublishCancelledRequestIsReported(t *testing.T) {
	reached := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			once.Do(func() { close(reached) })
			// Hold the response open so the cancellation is observed
			// rather than raced against a timer. `release` is what
			// unblocks the handler at teardown; the request context is
			// only an early exit, because an HTTP/1 server does not
			// reliably notice a cancelled client before then.
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
	t.Cleanup(func() {
		close(release)
		ts.Close()
	})

	te := setupWithServerOpts(t, []server.Option{
		server.WithGithubGistAPIURL(ts.URL),
	})
	te.srv.SetGithubToken(phase25Token)
	id := te.seedPhase25Insight(t, phase25DailyInsight())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-reached
		cancel()
	}()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf(
		"/api/v1/insights/%d/publish", id),
		strings.NewReader("{}")).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	// A cancelled request never reaches the publish handler's own error
	// mapping: humaTimeout's http.TimeoutHandler answers 503 with an
	// empty body for every route. What matters here is that the caller
	// cannot mistake this for a completed publish.
	assert.Equal(t, http.StatusServiceUnavailable, w.Code,
		"body: %s", w.Body.String())
	assert.NotEqual(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "gist_url")
	assert.NotContains(t, w.Body.String(), phase25Token)
	// The request did go out, so the cancellation is a real in-flight
	// one rather than a request that was never attempted. This is a
	// non-blocking check on purpose: by the time ServeHTTP returns the
	// stub has either been reached or never will be, and a blocking
	// receive here would hang the suite instead of failing it.
	select {
	case <-reached:
	default:
		assert.Fail(t, "publish never reached the gist stub")
	}
}
