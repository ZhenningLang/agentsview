package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
)

func TestClient_ChatJSON(t *testing.T) {
	t.Run("request shape and response decode", func(t *testing.T) {
		var gotPath string
		var gotAuth string
		var gotBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"ok\"}"}}]}`))
		}))
		defer server.Close()

		client := New(config.LLMConfig{
			BaseURL:         server.URL + "/v1/",
			APIKey:          "secret-key",
			Model:           "deepseek-chat",
			ReasoningEffort: "medium",
		})
		got, err := client.ChatJSON(context.Background(), "sys", "user")

		require.NoError(t, err)
		assert.Equal(t, `{"title":"ok"}`, got)
		assert.Equal(t, "/v1/chat/completions", gotPath)
		assert.Equal(t, "Bearer secret-key", gotAuth)
		assert.Equal(t, "deepseek-chat", gotBody["model"])
		assert.Equal(t, "medium", gotBody["reasoning_effort"])
		assert.Equal(t, map[string]any{"type": "json_object"}, gotBody["response_format"])
	})

	t.Run("5xx retries", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 3 {
				http.Error(w, "temporary", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
		}))
		defer server.Close()

		client := New(config.LLMConfig{BaseURL: server.URL, Model: "m"})
		client.sleep = func(time.Duration) {}
		_, err := client.ChatJSON(context.Background(), "", "")

		require.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("transport errors retry", func(t *testing.T) {
		attempts := 0
		httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("temporary network failure")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{}"}}]}`)),
				Request:    req,
			}, nil
		})}

		client := NewWithHTTPClient(config.LLMConfig{BaseURL: "https://llm.example.test/v1", Model: "m"}, httpClient)
		client.sleep = func(time.Duration) {}
		_, err := client.ChatJSON(context.Background(), "", "")

		require.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("ordinary 4xx does not retry", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			http.Error(w, "bad key", http.StatusUnauthorized)
		}))
		defer server.Close()

		client := New(config.LLMConfig{BaseURL: server.URL, APIKey: "secret-key", Model: "m"})
		_, err := client.ChatJSON(context.Background(), "", "")

		require.Error(t, err)
		assert.Equal(t, 1, attempts)
		assert.NotContains(t, err.Error(), "secret-key")
	})

	t.Run("provider error body redacts api key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad token secret-key", http.StatusUnauthorized)
		}))
		defer server.Close()

		client := New(config.LLMConfig{BaseURL: server.URL, APIKey: "secret-key", Model: "m"})
		_, err := client.ChatJSON(context.Background(), "", "")

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret-key")
	})

	t.Run("reasoning rejection retries once without field", func(t *testing.T) {
		attempts := 0
		seenReasoning := []bool{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			_, ok := body["reasoning_effort"]
			seenReasoning = append(seenReasoning, ok)
			if attempts == 1 {
				http.Error(w, "unknown field reasoning_effort", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
		}))
		defer server.Close()

		client := New(config.LLMConfig{BaseURL: server.URL, Model: "m", ReasoningEffort: "high"})
		_, err := client.ChatJSON(context.Background(), "", "")

		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
		assert.Equal(t, []bool{true, false}, seenReasoning)
	})
}

func TestClient_Embed(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var gotPath string
		var gotBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
			_, _ = w.Write([]byte(`{"data":[{"embedding":[1.25,2.5]}]}`))
		}))
		defer server.Close()

		client := New(config.LLMConfig{Embed: config.LLMEmbedConfig{BaseURL: server.URL + "/v1", APIKey: "embed-key", Model: "embed-model"}})
		got, err := client.Embed(context.Background(), "hello")

		require.NoError(t, err)
		assert.Equal(t, []float32{1.25, 2.5}, got)
		assert.Equal(t, "/v1/embeddings", gotPath)
		assert.Equal(t, "embed-model", gotBody["model"])
		assert.Equal(t, "hello", gotBody["input"])
	})

	t.Run("falls back to chat endpoint settings", func(t *testing.T) {
		var gotAuth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"data":[{"embedding":[3]}]}`))
		}))
		defer server.Close()

		client := New(config.LLMConfig{
			BaseURL: server.URL,
			APIKey:  "chat-key",
			Embed: config.LLMEmbedConfig{
				Model: "embed-model",
			},
		})
		got, err := client.Embed(context.Background(), "hello")

		require.NoError(t, err)
		assert.Equal(t, []float32{3}, got)
		assert.Equal(t, "Bearer chat-key", gotAuth)
	})

	t.Run("empty model is disabled", func(t *testing.T) {
		client := New(config.LLMConfig{Embed: config.LLMEmbedConfig{BaseURL: "https://example.test"}})
		_, err := client.Embed(context.Background(), "hello")

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNotConfigured))
	})

	t.Run("404 is unsupported", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()

		client := New(config.LLMConfig{Embed: config.LLMEmbedConfig{BaseURL: server.URL, Model: "embed-model"}})
		_, err := client.Embed(context.Background(), "hello")

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrEmbeddingsUnsupported))
	})

	t.Run("empty vector is rejected", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"embedding":[]}]}`))
		}))
		defer server.Close()

		client := New(config.LLMConfig{Embed: config.LLMEmbedConfig{BaseURL: server.URL, Model: "embed-model"}})
		_, err := client.Embed(context.Background(), "hello")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty embedding")
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
