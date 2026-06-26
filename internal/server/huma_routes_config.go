package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

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
	get(s, group, "/llm/providers", "Get LLM providers and usage", s.humaGetLLMProviders)
	patch(s, group, "/llm/providers", "Set LLM providers and usage", s.humaPatchLLMProviders)
	get(s, group, "/consolidate", "Get consolidate config", s.humaGetConsolidateConfig)
	patch(s, group, "/consolidate", "Set consolidate config", s.humaPatchConsolidateConfig)
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

type llmProvidersInput struct {
	Body llmProvidersPatch
}

type consolidateConfigInput struct {
	Body consolidateConfigPatch
}

type llmEmbedConfigPatch struct {
	BaseURL    *string `json:"base_url,omitempty"`
	APIKey     *string `json:"api_key,omitempty"`
	Model      *string `json:"model,omitempty"`
	BalanceURL *string `json:"balance_url,omitempty"`
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

	// Test-only routing hints, ignored by applyLLMConfigPatch and the config
	// save path. They let POST /llm/test target a usage's effective config, a
	// stored named provider's real secret, or a single transport channel.
	Usage    *string `json:"usage,omitempty" doc:"(test only) Resolve and test this usage's effective config"`
	Provider *string `json:"provider,omitempty" doc:"(test only) Resolve and test this named registry provider's stored secret"`
	Channel  *string `json:"channel,omitempty" doc:"(test only) Restrict to one channel: \"chat\" or \"embed\""`
}

type llmProvidersPatch struct {
	Providers       map[string]llmProviderConfigPatch `json:"providers,omitempty"`
	Usage           map[string]string                 `json:"usage,omitempty"`
	DeleteProviders []string                          `json:"delete_providers,omitempty"`
}

type consolidateConfigPatch struct {
	Interval *string `json:"interval,omitempty"`
}

type llmProviderConfigPatch struct {
	Enabled         *bool   `json:"enabled,omitempty"`
	BaseURL         *string `json:"base_url,omitempty"`
	APIKey          *string `json:"api_key,omitempty"`
	Model           *string `json:"model,omitempty"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
	BalanceURL      *string `json:"balance_url,omitempty"`
}

type llmEmbedConfigResponse struct {
	BaseURL       string `json:"base_url,omitempty"`
	Model         string `json:"model,omitempty"`
	HasAPIKey     bool   `json:"has_api_key"`
	APIKeyPreview string `json:"api_key_preview,omitempty"`
	BalanceURL    string `json:"balance_url,omitempty"`
}

type llmConfigResponse struct {
	Enabled             bool                                 `json:"enabled"`
	BaseURL             string                               `json:"base_url,omitempty"`
	Model               string                               `json:"model,omitempty"`
	ReasoningEffort     string                               `json:"reasoning_effort,omitempty"`
	MinUserMessages     int                                  `json:"min_user_messages"`
	ReenrichMsgDelta    int                                  `json:"reenrich_msg_delta"`
	ReenrichIdleMinutes int                                  `json:"reenrich_idle_minutes"`
	Concurrency         int                                  `json:"concurrency"`
	Periodic            bool                                 `json:"periodic"`
	BalanceURL          string                               `json:"balance_url,omitempty"`
	HasAPIKey           bool                                 `json:"has_api_key"`
	APIKeyPreview       string                               `json:"api_key_preview,omitempty"`
	Embed               llmEmbedConfigResponse               `json:"embed"`
	Providers           map[string]llmProviderConfigResponse `json:"providers,omitempty"`
	Usage               map[string]string                    `json:"usage,omitempty"`
	UsageWarnings       []string                             `json:"usage_warnings,omitempty"`
}

type llmProviderConfigResponse struct {
	Enabled         bool   `json:"enabled"`
	BaseURL         string `json:"base_url,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	BalanceURL      string `json:"balance_url,omitempty"`
	HasAPIKey       bool   `json:"has_api_key"`
	APIKeyPreview   string `json:"api_key_preview,omitempty"`
}

type consolidateConfigResponse struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval"`
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

func (s *Server) humaGetLLMProviders(
	ctx context.Context,
	_ *emptyInput,
) (*jsonOutput[llmConfigResponse], error) {
	return s.humaGetLLMConfig(ctx, &emptyInput{})
}

func (s *Server) humaPatchLLMProviders(
	ctx context.Context,
	in *llmProvidersInput,
) (*jsonOutput[llmConfigResponse], error) {
	if err := s.requireLocalWritableLLMRequest(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	providers := applyLLMProvidersPatch(s.cfg.LLM.Providers, in.Body.Providers, in.Body.DeleteProviders)
	usage := applyLLMUsagePatch(s.cfg.LLM.Usage, in.Body.Usage, in.Body.DeleteProviders)
	err := s.cfg.SaveLLMProviders(providers, usage)
	llm := s.cfg.LLM
	s.mu.Unlock()
	if err != nil {
		return nil, internalError("save LLM providers", err)
	}
	return &jsonOutput[llmConfigResponse]{Body: llmConfigResponseFromConfig(llm)}, nil
}

func (s *Server) humaGetConsolidateConfig(
	ctx context.Context,
	_ *emptyInput,
) (*jsonOutput[consolidateConfigResponse], error) {
	if err := s.requireLocalWritableLLMRequest(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	resp := consolidateConfigResponseFromConfig(s.cfg)
	s.mu.RUnlock()
	return &jsonOutput[consolidateConfigResponse]{Body: resp}, nil
}

func (s *Server) humaPatchConsolidateConfig(
	ctx context.Context,
	in *consolidateConfigInput,
) (*jsonOutput[consolidateConfigResponse], error) {
	if err := s.requireLocalWritableLLMRequest(ctx); err != nil {
		return nil, err
	}
	patch := make(map[string]any)
	if in.Body.Interval != nil {
		d, err := time.ParseDuration(strings.TrimSpace(*in.Body.Interval))
		if err != nil || d <= 0 {
			return nil, apiError(http.StatusBadRequest, "invalid consolidate interval")
		}
		patch["consolidate_interval"] = d
	}
	s.mu.Lock()
	if len(patch) > 0 {
		if err := s.cfg.SaveSettings(patch); err != nil {
			s.mu.Unlock()
			return nil, internalError("save consolidate config", err)
		}
	}
	resp := consolidateConfigResponseFromConfig(s.cfg)
	s.mu.Unlock()
	return &jsonOutput[consolidateConfigResponse]{Body: resp}, nil
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
		if patch.Embed.BalanceURL != nil {
			llm.Embed.BalanceURL = strings.TrimSpace(*patch.Embed.BalanceURL)
		}
		if patch.Embed.Model != nil {
			llm.Embed.Model = strings.TrimSpace(*patch.Embed.Model)
		}
	}
	return llm
}

func applyLLMProvidersPatch(
	current map[string]config.LLMConfig,
	patch map[string]llmProviderConfigPatch,
	deleteProviders []string,
) map[string]config.LLMConfig {
	out := make(map[string]config.LLMConfig, len(current)+len(patch))
	for name, provider := range current {
		if name = strings.TrimSpace(name); name != "" {
			out[name] = provider
		}
	}
	for name, p := range patch {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		provider := out[name]
		if p.Enabled != nil {
			provider.Enabled = *p.Enabled
		}
		if p.BaseURL != nil {
			provider.BaseURL = strings.TrimSpace(*p.BaseURL)
		}
		if p.APIKey != nil && shouldReplaceLLMAPIKey(*p.APIKey) {
			provider.APIKey = strings.TrimSpace(*p.APIKey)
		}
		if p.Model != nil {
			provider.Model = strings.TrimSpace(*p.Model)
		}
		if p.ReasoningEffort != nil {
			provider.ReasoningEffort = strings.TrimSpace(*p.ReasoningEffort)
		}
		if p.BalanceURL != nil {
			provider.BalanceURL = strings.TrimSpace(*p.BalanceURL)
		}
		out[name] = provider
	}
	for _, name := range deleteProviders {
		if name = strings.TrimSpace(name); name != "" {
			delete(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyLLMUsagePatch(current, patch map[string]string, deleteProviders []string) map[string]string {
	out := make(map[string]string, len(current)+len(patch))
	deleted := make(map[string]struct{}, len(deleteProviders))
	for _, provider := range deleteProviders {
		if provider = strings.TrimSpace(provider); provider != "" {
			deleted[provider] = struct{}{}
		}
	}
	for usage, provider := range current {
		usage = strings.TrimSpace(usage)
		provider = strings.TrimSpace(provider)
		_, isDeleted := deleted[provider]
		if usage != "" && provider != "" && !isDeleted {
			out[usage] = provider
		}
	}
	for usage, provider := range patch {
		usage = strings.TrimSpace(usage)
		provider = strings.TrimSpace(provider)
		if usage == "" {
			continue
		}
		if provider == "" {
			delete(out, usage)
			continue
		}
		if _, isDeleted := deleted[provider]; isDeleted {
			delete(out, usage)
			continue
		}
		out[usage] = provider
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldReplaceLLMAPIKey(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return strings.Trim(value, "*") != ""
}

func llmConfigResponseFromConfig(llm config.LLMConfig) llmConfigResponse {
	resp := llmConfigResponse{
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
			BalanceURL:    llm.Embed.BalanceURL,
		},
	}
	if len(llm.Providers) > 0 {
		resp.Providers = make(map[string]llmProviderConfigResponse, len(llm.Providers))
		for name, provider := range llm.Providers {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			resp.Providers[name] = llmProviderConfigResponse{
				Enabled:         provider.Enabled,
				BaseURL:         provider.BaseURL,
				Model:           provider.Model,
				ReasoningEffort: provider.ReasoningEffort,
				BalanceURL:      provider.BalanceURL,
				HasAPIKey:       strings.TrimSpace(provider.APIKey) != "",
				APIKeyPreview:   apiKeyPreview(provider.APIKey),
			}
		}
	}
	if len(llm.Usage) > 0 {
		resp.Usage = make(map[string]string, len(llm.Usage))
		for usage, provider := range llm.Usage {
			usage = strings.TrimSpace(usage)
			provider = strings.TrimSpace(provider)
			if usage != "" && provider != "" {
				resp.Usage[usage] = provider
			}
		}
	}
	resp.UsageWarnings = danglingLLMUsageWarnings(llm)
	return resp
}

func danglingLLMUsageWarnings(llm config.LLMConfig) []string {
	dangling := (&config.Config{LLM: llm}).DanglingLLMUsageBindings()
	if len(dangling) == 0 {
		return nil
	}
	warnings := make([]string, 0, len(dangling))
	for _, usage := range dangling {
		provider := strings.TrimSpace(llm.Usage[usage])
		warnings = append(warnings, fmt.Sprintf("usage %q references unknown provider %q", usage, provider))
	}
	slices.Sort(warnings)
	return warnings
}

func consolidateConfigResponseFromConfig(cfg config.Config) consolidateConfigResponse {
	return consolidateConfigResponse{
		Enabled:  cfg.ConsolidateEnabled,
		Interval: cfg.ResolveConsolidateInterval().String(),
	}
}

func apiKeyPreview(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return ""
	}
	return value[len(value)-4:]
}
