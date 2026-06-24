package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/shlex"
	"go.kenn.io/agentsview/internal/config"
)

func (s *Server) registerConfigRoutes() {
	group := newRouteGroup(s.api, "/api/v1/config", "Config")

	get(s, group, "/github", "Get GitHub config", s.humaGetGithubConfig)
	post(s, group, "/github", "Set GitHub config", s.humaSetGithubConfig)
	get(s, group, "/terminal", "Get terminal config", s.humaGetTerminalConfig)
	post(s, group, "/terminal", "Set terminal config", s.humaSetTerminalConfig)
	get(s, group, "/llm", "Get LLM config", s.humaGetLLMConfig)
	post(s, group, "/llm", "Set LLM config", s.humaSetLLMConfig)
}

type terminalMode string

const (
	terminalModeAuto      terminalMode = "auto"
	terminalModeCustom    terminalMode = "custom"
	terminalModeClipboard terminalMode = "clipboard"
)

type githubConfigResponse struct {
	Configured bool `json:"configured"`
}

type setGithubConfigInput struct {
	Body struct {
		Token string `json:"token" required:"true" minLength:"1" doc:"GitHub token"`
	}
}

type setGithubConfigResponse struct {
	Success  bool   `json:"success"`
	Username string `json:"username"`
}

type terminalConfigInput struct {
	Body terminalConfigBody
}

type terminalConfigBody struct {
	Mode       terminalMode `json:"mode" enum:"auto,custom,clipboard" doc:"Terminal launch mode"`
	CustomBin  string       `json:"custom_bin,omitempty" doc:"Terminal binary path when mode is custom"`
	CustomArgs string       `json:"custom_args,omitempty" doc:"Argument template containing {cmd} when mode is custom"`
}

type llmConfigInput struct {
	Body llmConfigPatch
}

type llmEmbedConfigPatch struct {
	BaseURL *string `json:"base_url,omitempty"`
	APIKey  *string `json:"api_key,omitempty"`
	Model   *string `json:"model,omitempty"`
}

type llmConfigPatch struct {
	Enabled             *bool                `json:"enabled,omitempty"`
	BaseURL             *string              `json:"base_url,omitempty"`
	APIKey              *string              `json:"api_key,omitempty"`
	Model               *string              `json:"model,omitempty"`
	ReasoningEffort     *string              `json:"reasoning_effort,omitempty"`
	MinUserMessages     *int                 `json:"min_user_messages,omitempty"`
	ReenrichMsgDelta    *int                 `json:"reenrich_msg_delta,omitempty"`
	ReenrichIdleMinutes *int                 `json:"reenrich_idle_minutes,omitempty"`
	Concurrency         *int                 `json:"concurrency,omitempty"`
	Periodic            *bool                `json:"periodic,omitempty"`
	BalanceURL          *string              `json:"balance_url,omitempty"`
	Embed               *llmEmbedConfigPatch `json:"embed,omitempty"`
}

type llmEmbedConfigResponse struct {
	BaseURL       string `json:"base_url,omitempty"`
	Model         string `json:"model,omitempty"`
	HasAPIKey     bool   `json:"has_api_key"`
	APIKeyPreview string `json:"api_key_preview,omitempty"`
}

type llmConfigResponse struct {
	Enabled             bool                   `json:"enabled"`
	BaseURL             string                 `json:"base_url,omitempty"`
	Model               string                 `json:"model,omitempty"`
	ReasoningEffort     string                 `json:"reasoning_effort,omitempty"`
	MinUserMessages     int                    `json:"min_user_messages"`
	ReenrichMsgDelta    int                    `json:"reenrich_msg_delta"`
	ReenrichIdleMinutes int                    `json:"reenrich_idle_minutes"`
	Concurrency         int                    `json:"concurrency"`
	Periodic            bool                   `json:"periodic"`
	BalanceURL          string                 `json:"balance_url,omitempty"`
	HasAPIKey           bool                   `json:"has_api_key"`
	APIKeyPreview       string                 `json:"api_key_preview,omitempty"`
	Embed               llmEmbedConfigResponse `json:"embed"`
}

func terminalConfigBodyFromConfig(tc config.TerminalConfig) terminalConfigBody {
	mode := terminalMode(tc.Mode)
	if mode == "" {
		mode = terminalModeAuto
	}
	return terminalConfigBody{
		Mode:       mode,
		CustomBin:  tc.CustomBin,
		CustomArgs: tc.CustomArgs,
	}
}

func (b terminalConfigBody) config() config.TerminalConfig {
	return config.TerminalConfig{
		Mode:       string(b.Mode),
		CustomBin:  b.CustomBin,
		CustomArgs: b.CustomArgs,
	}
}

func (s *Server) humaGetGithubConfig(
	_ context.Context,
	_ *emptyInput,
) (*jsonOutput[githubConfigResponse], error) {
	return &jsonOutput[githubConfigResponse]{
		Body: githubConfigResponse{Configured: s.githubToken() != ""},
	}, nil
}

func (s *Server) humaSetGithubConfig(
	ctx context.Context,
	in *setGithubConfigInput,
) (*jsonOutput[setGithubConfigResponse], error) {
	token := strings.TrimSpace(in.Body.Token)
	if token == "" {
		return nil, apiError(http.StatusBadRequest, "token required")
	}
	username, err := validateGithubToken(ctx, token)
	if err != nil {
		return nil, apiError(http.StatusUnauthorized, err.Error())
	}
	s.mu.Lock()
	err = s.cfg.SaveGithubToken(token)
	s.mu.Unlock()
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, "failed to save token")
	}
	return &jsonOutput[setGithubConfigResponse]{
		Body: setGithubConfigResponse{Success: true, Username: username},
	}, nil
}

func (s *Server) humaGetTerminalConfig(
	_ context.Context,
	_ *emptyInput,
) (*jsonOutput[terminalConfigBody], error) {
	s.mu.RLock()
	tc := s.cfg.Terminal
	s.mu.RUnlock()
	return &jsonOutput[terminalConfigBody]{
		Body: terminalConfigBodyFromConfig(tc),
	}, nil
}

func (s *Server) humaSetTerminalConfig(
	_ context.Context,
	in *terminalConfigInput,
) (*jsonOutput[terminalConfigBody], error) {
	body := in.Body
	tc := body.config()
	switch terminalMode(tc.Mode) {
	case terminalModeAuto, terminalModeCustom, terminalModeClipboard:
	default:
		return nil, apiError(http.StatusBadRequest,
			`mode must be "auto", "custom", or "clipboard"`)
	}
	if tc.Mode == string(terminalModeCustom) && tc.CustomBin == "" {
		return nil, apiError(http.StatusBadRequest,
			`custom_bin is required when mode is "custom"`)
	}
	if tc.Mode == string(terminalModeCustom) {
		if tc.CustomArgs != "" && !strings.Contains(tc.CustomArgs, "{cmd}") {
			return nil, apiError(http.StatusBadRequest,
				`custom_args must contain the {cmd} placeholder so the resume command is passed to the terminal`)
		}
		if tc.CustomArgs != "" {
			if _, splitErr := shlex.Split(tc.CustomArgs); splitErr != nil {
				return nil, apiError(http.StatusBadRequest,
					fmt.Sprintf("custom_args has invalid shell syntax: %v", splitErr))
			}
		}
	}
	s.mu.Lock()
	err := s.cfg.SaveTerminalConfig(tc)
	s.mu.Unlock()
	if err != nil {
		return nil, internalError("save terminal config", err)
	}
	return &jsonOutput[terminalConfigBody]{
		Body: terminalConfigBodyFromConfig(tc),
	}, nil
}

func (s *Server) humaGetLLMConfig(
	ctx context.Context,
	_ *emptyInput,
) (*jsonOutput[llmConfigResponse], error) {
	if err := s.requireLocalWritableLLMRequest(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	llm := s.cfg.LLM
	s.mu.RUnlock()
	return &jsonOutput[llmConfigResponse]{Body: llmConfigResponseFromConfig(llm)}, nil
}

func (s *Server) humaSetLLMConfig(
	ctx context.Context,
	in *llmConfigInput,
) (*jsonOutput[llmConfigResponse], error) {
	if err := s.requireLocalWritableLLMRequest(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	llm := applyLLMConfigPatch(s.cfg.LLM, in.Body)
	err := s.cfg.SaveLLMConfig(llm)
	if err == nil {
		llm = s.cfg.LLM
	}
	s.mu.Unlock()
	if err != nil {
		return nil, internalError("save LLM config", err)
	}
	return &jsonOutput[llmConfigResponse]{Body: llmConfigResponseFromConfig(llm)}, nil
}

func applyLLMConfigPatch(llm config.LLMConfig, patch llmConfigPatch) config.LLMConfig {
	if patch.Enabled != nil {
		llm.Enabled = *patch.Enabled
	}
	if patch.BaseURL != nil {
		llm.BaseURL = strings.TrimSpace(*patch.BaseURL)
	}
	if patch.APIKey != nil && shouldReplaceLLMAPIKey(*patch.APIKey) {
		llm.APIKey = strings.TrimSpace(*patch.APIKey)
	}
	if patch.Model != nil {
		llm.Model = strings.TrimSpace(*patch.Model)
	}
	if patch.ReasoningEffort != nil {
		llm.ReasoningEffort = strings.TrimSpace(*patch.ReasoningEffort)
	}
	if patch.MinUserMessages != nil {
		llm.MinUserMessages = *patch.MinUserMessages
	}
	if patch.ReenrichMsgDelta != nil {
		llm.ReenrichMsgDelta = *patch.ReenrichMsgDelta
	}
	if patch.ReenrichIdleMinutes != nil {
		llm.ReenrichIdleMinutes = *patch.ReenrichIdleMinutes
	}
	if patch.Concurrency != nil {
		llm.Concurrency = *patch.Concurrency
	}
	if patch.Periodic != nil {
		llm.Periodic = *patch.Periodic
	}
	if patch.BalanceURL != nil {
		llm.BalanceURL = strings.TrimSpace(*patch.BalanceURL)
	}
	if patch.Embed != nil {
		if patch.Embed.BaseURL != nil {
			llm.Embed.BaseURL = strings.TrimSpace(*patch.Embed.BaseURL)
		}
		if patch.Embed.APIKey != nil && shouldReplaceLLMAPIKey(*patch.Embed.APIKey) {
			llm.Embed.APIKey = strings.TrimSpace(*patch.Embed.APIKey)
		}
		if patch.Embed.Model != nil {
			llm.Embed.Model = strings.TrimSpace(*patch.Embed.Model)
		}
	}
	return llm
}

func shouldReplaceLLMAPIKey(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return strings.Trim(value, "*") != ""
}

func llmConfigResponseFromConfig(llm config.LLMConfig) llmConfigResponse {
	return llmConfigResponse{
		Enabled:             llm.Enabled,
		BaseURL:             llm.BaseURL,
		Model:               llm.Model,
		ReasoningEffort:     llm.ReasoningEffort,
		MinUserMessages:     llm.MinUserMessages,
		ReenrichMsgDelta:    llm.ReenrichMsgDelta,
		ReenrichIdleMinutes: llm.ReenrichIdleMinutes,
		Concurrency:         llm.Concurrency,
		Periodic:            llm.Periodic,
		BalanceURL:          llm.BalanceURL,
		HasAPIKey:           strings.TrimSpace(llm.APIKey) != "",
		APIKeyPreview:       apiKeyPreview(llm.APIKey),
		Embed: llmEmbedConfigResponse{
			BaseURL:       llm.Embed.BaseURL,
			Model:         llm.Embed.Model,
			HasAPIKey:     strings.TrimSpace(llm.Embed.APIKey) != "",
			APIKeyPreview: apiKeyPreview(llm.Embed.APIKey),
		},
	}
}

func apiKeyPreview(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return ""
	}
	return value[len(value)-4:]
}
