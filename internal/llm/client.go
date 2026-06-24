package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
)

var (
	ErrNotConfigured         = errors.New("llm not configured")
	ErrEmbeddingsUnsupported = errors.New("embeddings unsupported")
)

type Client struct {
	cfg        config.LLMConfig
	httpClient *http.Client
	sleep      func(time.Duration)
}

func New(cfg config.LLMConfig) *Client {
	return NewWithHTTPClient(cfg, nil)
}

func NewWithHTTPClient(cfg config.LLMConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	cfg = resolveEmbedFallback(cfg)
	return &Client{
		cfg:        cfg,
		httpClient: httpClient,
		sleep:      time.Sleep,
	}
}

func (c *Client) ChatJSON(ctx context.Context, system, user string) (string, error) {
	if strings.TrimSpace(c.cfg.BaseURL) == "" || strings.TrimSpace(c.cfg.Model) == "" {
		return "", ErrNotConfigured
	}
	endpoint := joinEndpoint(c.cfg.BaseURL, "chat/completions")
	withReasoning := strings.TrimSpace(c.cfg.ReasoningEffort) != ""
	triedWithoutReasoning := false

	for attempt := 0; attempt < 3; attempt++ {
		content, status, err := c.postChat(ctx, endpoint, system, user, withReasoning)
		if err == nil {
			return content, nil
		}
		if status >= 400 && status < 500 {
			if withReasoning && !triedWithoutReasoning && isReasoningRejection(err.Error()) {
				withReasoning = false
				triedWithoutReasoning = true
				continue
			}
			return "", err
		}
		if attempt == 2 || !isRetryable(status, err) {
			return "", err
		}
		c.sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return "", fmt.Errorf("chat completions: exhausted retries")
}

func (c *Client) Embed(ctx context.Context, input string) ([]float32, error) {
	if strings.TrimSpace(c.cfg.Embed.Model) == "" {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(c.cfg.Embed.BaseURL) == "" {
		return nil, ErrNotConfigured
	}
	body := map[string]any{
		"model": c.cfg.Embed.Model,
		"input": input,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinEndpoint(c.cfg.Embed.BaseURL, "embeddings"), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	setHeaders(req, c.cfg.Embed.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("embeddings: reading response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrEmbeddingsUnsupported
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("embeddings: provider returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("embeddings: decoding response: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embeddings: empty embedding")
	}
	out := make([]float32, len(parsed.Data[0].Embedding))
	for i, v := range parsed.Data[0].Embedding {
		out[i] = float32(v)
	}
	return out, nil
}

func (c *Client) postChat(ctx context.Context, endpoint, system, user string, withReasoning bool) (string, int, error) {
	body := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.2,
	}
	if withReasoning {
		body["reasoning_effort"] = c.cfg.ReasoningEffort
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", 0, fmt.Errorf("chat completions: encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", 0, fmt.Errorf("chat completions: building request: %w", err)
	}
	setHeaders(req, c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("chat completions: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("chat completions: reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", resp.StatusCode, fmt.Errorf("chat completions: provider returned status %d: %s", resp.StatusCode, sanitizeProviderError(respBody, c.cfg.APIKey))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", resp.StatusCode, fmt.Errorf("chat completions: decoding response: %w", err)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return "", resp.StatusCode, fmt.Errorf("chat completions: missing message content")
	}
	return parsed.Choices[0].Message.Content, resp.StatusCode, nil
}

func joinEndpoint(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func setHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

func resolveEmbedFallback(cfg config.LLMConfig) config.LLMConfig {
	if cfg.Embed.BaseURL == "" {
		cfg.Embed.BaseURL = cfg.BaseURL
		if cfg.Embed.APIKey == "" {
			cfg.Embed.APIKey = cfg.APIKey
		}
	}
	return cfg
}

func isRetryable(status int, err error) bool {
	if err == nil {
		return false
	}
	return status == 0 || status >= 500
}

func isReasoningRejection(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "reasoning_effort") || strings.Contains(message, "reasoning")
}

func sanitizeProviderError(body []byte, secrets ...string) string {
	text := strings.TrimSpace(string(body))
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}
