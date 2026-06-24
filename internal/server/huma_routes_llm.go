package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/enrich"
)

const defaultHTTPEnrichLimit = 25

func (s *Server) registerLLMRoutes() {
	group := newRouteGroup(s.api, "/api/v1/llm", "LLM")

	post(s, group, "/enrich", "Trigger LLM enrichment", s.humaTriggerLLMEnrichment)
	get(s, group, "/enrich/status", "Get LLM enrichment status", s.humaLLMEnrichmentStatus)
	get(s, group, "/balance", "Get LLM provider balance", s.humaLLMBalance)
}

type llmEnrichInput struct {
	Body llmEnrichRequest
}

type llmEnrichRequest struct {
	Project string `json:"project,omitempty"`
	Force   bool   `json:"force,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type llmEnrichResponse struct {
	Enriched   int   `json:"enriched"`
	Skipped    int   `json:"skipped"`
	NoContent  int   `json:"no_content"`
	Errors     int   `json:"errors"`
	Candidates int   `json:"candidates"`
	ElapsedMS  int64 `json:"elapsed_ms"`
}

type llmBalanceResponse struct {
	Supported bool   `json:"supported"`
	Currency  string `json:"currency,omitempty"`
	Amount    string `json:"amount,omitempty"`
	Available bool   `json:"available"`
}

func (s *Server) humaTriggerLLMEnrichment(
	ctx context.Context,
	in *llmEnrichInput,
) (*jsonOutput[llmEnrichResponse], error) {
	if err := requireLocalLLMRequest(ctx); err != nil {
		return nil, err
	}
	if s.llmWriter == nil {
		return nil, apiError(http.StatusNotImplemented, "not available in remote mode")
	}
	if in.Body.Limit < 0 {
		return nil, apiError(http.StatusBadRequest, "limit must be >= 0")
	}
	llmCfg := s.cfg.ResolveLLM()
	if !llmCfg.Enabled {
		return nil, apiError(http.StatusConflict, "LLM enrichment is disabled")
	}
	if strings.TrimSpace(llmCfg.APIKey) == "" {
		return nil, apiError(http.StatusBadRequest, "LLM API key is not configured")
	}
	if strings.TrimSpace(llmCfg.BaseURL) == "" || strings.TrimSpace(llmCfg.Model) == "" {
		return nil, apiError(http.StatusBadRequest, "LLM base_url and model are required")
	}

	limit := in.Body.Limit
	if limit <= 0 {
		limit = defaultHTTPEnrichLimit
	}
	started := time.Now()
	runner := enrich.New(s.llmWriter, s.llmClient(llmCfg), llmCfg)
	stats, err := runner.Run(ctx, enrich.Options{
		Project: strings.TrimSpace(in.Body.Project),
		Force:   in.Body.Force,
		Limit:   limit,
	})
	if err != nil {
		if handled := handleHumaContextError(err); handled != nil {
			return nil, handled
		}
		return nil, internalError("LLM enrichment", err)
	}
	return &jsonOutput[llmEnrichResponse]{Body: llmEnrichResponse{
		Enriched:   stats.Succeeded,
		Skipped:    stats.SkippedTooShort,
		NoContent:  stats.NoContent,
		Errors:     stats.Failed,
		Candidates: stats.Candidates,
		ElapsedMS:  time.Since(started).Milliseconds(),
	}}, nil
}

func (s *Server) humaLLMEnrichmentStatus(
	ctx context.Context,
	_ *emptyInput,
) (*jsonOutput[db.EnrichmentStatusReport], error) {
	if err := requireLocalLLMRequest(ctx); err != nil {
		return nil, err
	}
	report, err := s.db.GetEnrichmentStatus(ctx)
	if err != nil {
		return nil, internalError("LLM enrichment status", err)
	}
	return &jsonOutput[db.EnrichmentStatusReport]{Body: report}, nil
}

func (s *Server) humaLLMBalance(
	ctx context.Context,
	_ *emptyInput,
) (*jsonOutput[llmBalanceResponse], error) {
	if err := requireLocalLLMRequest(ctx); err != nil {
		return nil, err
	}
	resp := s.fetchLLMBalance(ctx, s.cfg.ResolveLLM())
	return &jsonOutput[llmBalanceResponse]{Body: resp}, nil
}

func requireLocalLLMRequest(ctx context.Context) error {
	if !isLocalhostContext(ctx) {
		return apiError(http.StatusForbidden, "not available from remote clients")
	}
	return nil
}

func (s *Server) fetchLLMBalance(ctx context.Context, cfg config.LLMConfig) llmBalanceResponse {
	endpoint, ok := llmBalanceEndpoint(cfg)
	if !ok || strings.TrimSpace(cfg.APIKey) == "" || !cfg.Enabled {
		return llmBalanceResponse{Supported: false}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		log.Printf("LLM balance: invalid endpoint")
		return llmBalanceResponse{Supported: false}
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := s.llmHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("LLM balance: provider request failed: %v", err)
		return llmBalanceResponse{Supported: false}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Printf("LLM balance: reading response failed: %v", err)
		return llmBalanceResponse{Supported: false}
	}
	if resp.StatusCode >= 400 {
		log.Printf("LLM balance: provider returned status %d", resp.StatusCode)
		return llmBalanceResponse{Supported: false}
	}
	balance, err := parseLLMBalance(data)
	if err != nil {
		log.Printf("LLM balance: parsing response failed: %v", err)
		return llmBalanceResponse{Supported: false}
	}
	return balance
}

func llmBalanceEndpoint(cfg config.LLMConfig) (string, bool) {
	if strings.TrimSpace(cfg.BalanceURL) != "" {
		return strings.TrimSpace(cfg.BalanceURL), true
	}
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		return "", false
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	root := url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case strings.Contains(host, "deepseek"):
		root.Path = "/user/balance"
		return root.String(), true
	case strings.Contains(host, "moonshot"):
		root.Path = "/users/me/balance"
		return root.String(), true
	default:
		return "", false
	}
}

func parseLLMBalance(data []byte) (llmBalanceResponse, error) {
	var parsed struct {
		IsAvailable  bool `json:"is_available"`
		BalanceInfos []struct {
			Currency     string          `json:"currency"`
			TotalBalance json.RawMessage `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return llmBalanceResponse{}, err
	}
	if len(parsed.BalanceInfos) == 0 {
		return llmBalanceResponse{}, fmt.Errorf("missing balance_infos")
	}
	info := parsed.BalanceInfos[0]
	amount, err := parseBalanceAmount(info.TotalBalance)
	if err != nil {
		return llmBalanceResponse{}, err
	}
	if amount == "" || info.Currency == "" {
		return llmBalanceResponse{}, fmt.Errorf("missing balance fields")
	}
	return llmBalanceResponse{
		Supported: true,
		Currency:  info.Currency,
		Amount:    amount,
		Available: parsed.IsAvailable,
	}, nil
}

func parseBalanceAmount(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text), nil
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		return number.String(), nil
	}
	return "", errors.New("invalid balance amount")
}
