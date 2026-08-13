package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	spaFixtureIndex = `<!doctype html><html><head><title>SPA Fixture</title></head><body><div id="app">fixture index</div><script type="module" src="/assets/app-deadbeef.js"></script></body></html>`
	spaFixtureJS    = `export const spaFixture = true;`
)

func newSPATestServer(t *testing.T, opts ...Option) *Server {
	t.Helper()

	s := testServer(t, 0, opts...)
	assets := fstest.MapFS{
		"index.html": {
			Data: []byte(spaFixtureIndex),
		},
		"assets/app-deadbeef.js": {
			Data: []byte(spaFixtureJS),
		},
	}
	s.spaFS = assets
	s.spaHandler = http.FileServerFS(assets)
	return s
}

func getSPAResponse(t *testing.T, s *Server, path string) (int, http.Header, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, string(body)
}

func assertSecurityHeaders(t *testing.T, header http.Header) {
	t.Helper()

	require.NotEmpty(t, header.Get("Content-Security-Policy"))
	assert.Equal(t, "DENY", header.Get("X-Frame-Options"))
}

func TestSPAMissingAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		opts []Option
	}{
		{name: "root", path: "/assets/obsolete-deadbeef.js"},
		{name: "base path", path: "/viewer/assets/obsolete-deadbeef.js", opts: []Option{WithBasePath("/viewer")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSPATestServer(t, tt.opts...)
			status, header, body := getSPAResponse(t, s, tt.path)

			require.Equal(t, http.StatusNotFound, status)
			assert.NotContains(t, body, "fixture index")
			assert.NotContains(t, header.Get("Content-Type"), "text/html")
			assertSecurityHeaders(t, header)
		})
	}
}

func TestSPAFingerprintedAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		opts []Option
	}{
		{name: "root", path: "/assets/app-deadbeef.js"},
		{name: "base path", path: "/viewer/assets/app-deadbeef.js", opts: []Option{WithBasePath("/viewer")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSPATestServer(t, tt.opts...)
			status, header, body := getSPAResponse(t, s, tt.path)

			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, spaFixtureJS, body)
			assert.Equal(t, "public, max-age=31536000, immutable", header.Get("Cache-Control"))
			assertSecurityHeaders(t, header)
		})
	}
}

func TestSPAIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		opts         []Option
		wantBaseHref bool
	}{
		{name: "root slash", path: "/"},
		{name: "root index", path: "/index.html"},
		{name: "base path slash", path: "/viewer/", opts: []Option{WithBasePath("/viewer")}, wantBaseHref: true},
		{name: "base path index", path: "/viewer/index.html", opts: []Option{WithBasePath("/viewer")}, wantBaseHref: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSPATestServer(t, tt.opts...)
			status, header, body := getSPAResponse(t, s, tt.path)

			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, "no-cache", header.Get("Cache-Control"))
			assert.Contains(t, body, "fixture index")
			if tt.wantBaseHref {
				assert.Contains(t, body, `<base href="/viewer/">`)
			} else {
				assert.NotContains(t, body, `<base href="/viewer/">`)
			}
			assertSecurityHeaders(t, header)
		})
	}
}

func TestSPAClientRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		opts         []Option
		wantBaseHref bool
	}{
		{name: "root", path: "/sessions/example"},
		{name: "base path", path: "/viewer/sessions/example", opts: []Option{WithBasePath("/viewer")}, wantBaseHref: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSPATestServer(t, tt.opts...)
			status, header, body := getSPAResponse(t, s, tt.path)

			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, "no-cache", header.Get("Cache-Control"))
			assert.Contains(t, body, "fixture index")
			if tt.wantBaseHref {
				assert.Contains(t, body, `<base href="/viewer/">`)
			} else {
				assert.NotContains(t, body, `<base href="/viewer/">`)
			}
			assertSecurityHeaders(t, header)
		})
	}
}
