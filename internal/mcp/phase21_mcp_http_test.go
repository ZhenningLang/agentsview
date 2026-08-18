package mcp_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/mcp"
	"go.kenn.io/agentsview/internal/service"
)

// phase21HTTPServer builds an MCP server over a seeded temp archive.
func phase21HTTPServer(t *testing.T) (*mcpsdk.Server, *db.DB) {
	t.Helper()
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-http", "alpha", -5*time.Hour,
		phase21UserMsg(0, "served over http"))
	return mcp.NewServer(service.NewReadOnlyBackend(d),
		&mcp.Options{Now: phase21Clock}), d
}

// --- QA10: bind normalization -----------------------------------------

func TestPhase21MCPHTTPListenerNormalizesBindAddresses(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		want     string
		loopback bool
	}{
		{"bare port", "8085", "127.0.0.1:8085", true},
		{"colon port", ":8085", "127.0.0.1:8085", true},
		{"padded colon port", " :8085 ", "127.0.0.1:8085", true},
		{"explicit ipv4 loopback", "127.0.0.1:8085", "127.0.0.1:8085", true},
		{"other ipv4 loopback", "127.0.0.5:8085", "127.0.0.5:8085", true},
		{"ipv6 loopback", "[::1]:8085", "[::1]:8085", true},
		{"localhost", "localhost:8085", "localhost:8085", true},
		{"ephemeral port", "0", "127.0.0.1:0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{Addr: tt.addr})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Addr)
			assert.Equal(t, tt.loopback, got.Loopback)
			assert.Empty(t, got.Token,
				"loopback without require_auth needs no token")
		})
	}
}

func TestPhase21MCPHTTPListenerRejectsNonLoopbackByDefault(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"all interfaces ipv4", "0.0.0.0:8085"},
		{"all interfaces ipv6", "[::]:8085"},
		{"lan address", "192.168.1.10:8085"},
		{"public name", "archive.example.com:8085"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
				Addr:  tt.addr,
				Token: "phase21-listener-token",
			})
			require.Error(t, err, "non-loopback must be an explicit opt-in")
			assert.Contains(t, err.Error(), "--http-allow-insecure")
		})
	}

	for _, addr := range []string{"", "   ", "not-an-address", "127.0.0.1:http"} {
		_, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{Addr: addr})
		assert.Error(t, err, "address %q must be rejected", addr)
	}
}

// --- QA10: listener auth ----------------------------------------------

func TestPhase21MCPListenerAuthRequiresTokenOffLoopback(t *testing.T) {
	// The insecure flag says where to listen. It must not also decide
	// who may read: an exposed port with no token publishes every
	// transcript on the machine to the network.
	_, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
		Addr:          "0.0.0.0:8085",
		AllowInsecure: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth token")

	got, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
		Addr:          "0.0.0.0:8085",
		AllowInsecure: true,
		Token:         "phase21-listener-token",
	})
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8085", got.Addr)
	assert.False(t, got.Loopback)
	assert.Equal(t, "phase21-listener-token", got.Token)
	assert.False(t, got.TokenProvisioned)
}

func TestPhase21MCPListenerAuthProvisionsLoopbackTokenWhenRequired(
	t *testing.T,
) {
	got, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
		Addr:        ":8085",
		RequireAuth: true,
	})
	require.NoError(t, err)
	assert.True(t, got.Loopback)
	assert.True(t, got.TokenProvisioned)
	assert.GreaterOrEqual(t, len(got.Token), 32)

	again, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
		Addr:        ":8085",
		RequireAuth: true,
	})
	require.NoError(t, err)
	assert.NotEqual(t, got.Token, again.Token,
		"a provisioned token must be freshly random each time")

	configured, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
		Addr:        ":8085",
		RequireAuth: true,
		Token:       "phase21-configured-token",
	})
	require.NoError(t, err)
	assert.Equal(t, "phase21-configured-token", configured.Token)
	assert.False(t, configured.TokenProvisioned)
}

// --- QA10: bearer auth ------------------------------------------------

func TestPhase21MCPBearerAuthChallengesUnauthenticatedRequests(t *testing.T) {
	reached := 0
	handler := mcp.WithBearerAuth("phase21-bearer-token",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached++
			w.WriteHeader(http.StatusNoContent)
		}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic phase21-bearer-token", http.StatusUnauthorized},
		{"empty credentials", "Bearer ", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"shorter token", "Bearer phase21", http.StatusUnauthorized},
		{"correct token", "Bearer phase21-bearer-token", http.StatusNoContent},
		{"lowercase scheme", "bearer phase21-bearer-token", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				context.Background(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			res, err := srv.Client().Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, tt.want, res.StatusCode)
			if tt.want == http.StatusUnauthorized {
				assert.Equal(t, `Bearer realm="agentsview"`,
					strings.Split(res.Header.Get("WWW-Authenticate"), ",")[0],
					"a 401 must tell the client what to send")
			}
		})
	}
	assert.Equal(t, 2, reached, "only authenticated requests reach the server")
}

func TestPhase21MCPBearerAuthComparesInConstantTime(t *testing.T) {
	// Behavioral tests cannot observe comparison timing, so the
	// property is pinned structurally: the comparison must go through
	// crypto/subtle over fixed-size digests, never through a string
	// equality that returns at the first differing byte.
	source, err := os.ReadFile("http.go")
	require.NoError(t, err)
	text := string(source)

	for _, needle := range []string{
		"crypto/" + "subtle", "subtle." + "ConstantTimeCompare",
		"sha256." + "Sum256",
	} {
		assert.Contains(t, text, needle,
			"bearer comparison must use %s", needle)
	}
	for _, banned := range []string{
		"presented " + "== token", "token " + "== presented",
		"presented " + "!= token",
	} {
		assert.NotContains(t, text, banned,
			"a plain string comparison leaks the token through timing")
	}
}

func TestPhase21MCPBearerAuthGuardsRealToolCalls(t *testing.T) {
	srv, _ := phase21HTTPServer(t)
	listener, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{
		Addr:        "127.0.0.1:0",
		RequireAuth: true,
		Token:       "phase21-e2e-token",
	})
	require.NoError(t, err)
	httpSrv := httptest.NewServer(mcp.NewHTTPHandler(srv, listener))
	t.Cleanup(httpSrv.Close)

	ctx := context.Background()
	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "phase21-http", Version: "test"}, nil)

	_, err = client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		MaxRetries: -1,
	}, nil)
	require.Error(t, err, "an unauthenticated client must not initialize")

	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		MaxRetries: -1,
		HTTPClient: &http.Client{Transport: phase21BearerRoundTripper{
			token: "phase21-e2e-token",
			base:  http.DefaultTransport,
		}},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	var out phase21ListOut
	phase21Call(t, session, mcp.ToolListSessions, map[string]any{}, &out)
	require.Len(t, out.Sessions, 1)
	assert.Equal(t, "sess-http", out.Sessions[0].ID)
}

type phase21BearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (rt phase21BearerRoundTripper) RoundTrip(
	req *http.Request,
) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.base.RoundTrip(clone)
}

// --- QA10: DNS rebinding ----------------------------------------------

func TestPhase21MCPDNSRebindingGuardRejectsForeignHostHeader(t *testing.T) {
	srv, _ := phase21HTTPServer(t)
	listener, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{Addr: "127.0.0.1:0"})
	require.NoError(t, err)

	ln, err := net.Listen("tcp", listener.Addr)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- mcp.ServeHTTP(ctx, ln, mcp.NewHTTPHandler(srv, listener))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Error("http server did not stop")
		}
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
		`"params":{"protocolVersion":"2025-06-18","capabilities":{},` +
		`"clientInfo":{"name":"phase21","version":"test"}}}`
	post := func(host string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodPost, "http://"+ln.Addr().String(),
			bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if host != "" {
			req.Host = host
		}
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = res.Body.Close() })
		return res
	}

	// A browser page resolving its own hostname to 127.0.0.1 reaches a
	// loopback listener with a foreign Host header. That is the whole
	// rebinding attack, and the SDK guard this server leaves enabled
	// is what stops it.
	rebound := post("evil.example.com")
	assert.Equal(t, http.StatusForbidden, rebound.StatusCode)
	payload, err := io.ReadAll(rebound.Body)
	require.NoError(t, err)
	assert.Contains(t, string(payload), "Host header")

	direct := post("")
	assert.NotEqual(t, http.StatusForbidden, direct.StatusCode,
		"a genuine loopback client must still be served")
}

// --- QA10: shutdown ---------------------------------------------------

func TestPhase21MCPShutdownStdioEndsCleanlyOnEOFAndCancel(t *testing.T) {
	t.Run("client disconnect", func(t *testing.T) {
		srv, _ := phase21HTTPServer(t)
		clientReader, serverWriter := io.Pipe()
		serverReader, clientWriter := io.Pipe()
		done := make(chan error, 1)
		go func() {
			done <- mcp.RunStdio(context.Background(), srv,
				serverReader, serverWriter)
		}()
		// A client that exits closes the pipe. That is how every
		// normal session ends, so it must not be reported as failure.
		require.NoError(t, clientWriter.Close())
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("stdio server did not stop after client disconnect")
		}
		_ = clientReader.Close()
	})

	t.Run("interrupt", func(t *testing.T) {
		srv, _ := phase21HTTPServer(t)
		clientReader, serverWriter := io.Pipe()
		serverReader, clientWriter := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- mcp.RunStdio(ctx, srv, serverReader, serverWriter)
		}()
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err, "an interrupt is a clean stop")
		case <-time.After(5 * time.Second):
			t.Fatal("stdio server did not stop after cancel")
		}
		_ = clientWriter.Close()
		_ = clientReader.Close()
	})
}

func TestPhase21MCPShutdownHTTPStopsOnCancel(t *testing.T) {
	srv, _ := phase21HTTPServer(t)
	listener, err := mcp.ResolveHTTPListener(mcp.HTTPOptions{Addr: "127.0.0.1:0"})
	require.NoError(t, err)
	ln, err := net.Listen("tcp", listener.Addr)
	require.NoError(t, err)
	addr := ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mcp.ServeHTTP(ctx, ln, mcp.NewHTTPHandler(srv, listener))
	}()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 5*time.Second, 20*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err, "cancellation is a clean stop, not a failure")
	case <-time.After(10 * time.Second):
		t.Fatal("http server did not stop after cancel")
	}

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		_ = conn.Close()
	}
	assert.Error(t, err, "the listener must be closed after shutdown")
}
