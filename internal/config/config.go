package config

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/pflag"
	"go.kenn.io/agentsview/internal/parser"
)

// TerminalConfig holds terminal launch preferences.
type TerminalConfig struct {
	// Mode: "auto" (detect terminal), "custom" (use CustomBin),
	// or "clipboard" (never launch, always copy).
	Mode string `json:"mode" toml:"mode"`
	// CustomBin is the terminal binary path (used when Mode == "custom").
	CustomBin string `json:"custom_bin,omitempty" toml:"custom_bin"`
	// CustomArgs is a template for terminal args. Use {cmd} as
	// placeholder for the resume command (e.g. "-- bash -c {cmd}").
	CustomArgs string `json:"custom_args,omitempty" toml:"custom_args"`
}

// ProxyConfig controls an optional managed reverse proxy.
type ProxyConfig struct {
	// Mode enables a managed proxy implementation.
	// Currently supported: "caddy".
	Mode string `json:"mode,omitempty" toml:"mode"`
	// Bin overrides the proxy executable path.
	Bin string `json:"bin,omitempty" toml:"bin"`
	// BindHost is the local interface/IP the proxy binds to.
	BindHost string `json:"bind_host,omitempty" toml:"bind_host"`
	// PublicPort is the external port exposed by the proxy.
	PublicPort int `json:"public_port,omitempty" toml:"public_port"`
	// TLSCert and TLSKey are used by managed HTTPS mode.
	TLSCert string `json:"tls_cert,omitempty" toml:"tls_cert"`
	TLSKey  string `json:"tls_key,omitempty" toml:"tls_key"`
	// AllowedSubnets restrict inbound clients to these CIDRs.
	AllowedSubnets []string `json:"allowed_subnets,omitempty" toml:"allowed_subnets"`
}

// PGConfig holds PostgreSQL connection settings.
type PGConfig struct {
	URL             string   `toml:"url" json:"url"`
	Schema          string   `toml:"schema" json:"schema"`
	MachineName     string   `toml:"machine_name" json:"machine_name"`
	AllowInsecure   bool     `toml:"allow_insecure" json:"allow_insecure"`
	Projects        []string `toml:"projects" json:"projects,omitempty"`
	ExcludeProjects []string `toml:"exclude_projects" json:"exclude_projects,omitempty"`
}

// DuckDBConfig holds DuckDB mirror and Quack connection settings.
type DuckDBConfig struct {
	Path            string   `toml:"path" json:"path"`
	URL             string   `toml:"url" json:"url"`
	Token           string   `toml:"token" json:"token,omitempty"`
	MachineName     string   `toml:"machine_name" json:"machine_name"`
	AllowInsecure   bool     `toml:"allow_insecure" json:"allow_insecure"`
	Projects        []string `toml:"projects" json:"projects,omitempty"`
	ExcludeProjects []string `toml:"exclude_projects" json:"exclude_projects,omitempty"`
}

// LLMEmbedConfig holds optional embedding-provider settings.
type LLMEmbedConfig struct {
	BaseURL    string `toml:"base_url" json:"base_url,omitempty"`
	APIKey     string `toml:"api_key" json:"-"`
	Model      string `toml:"model" json:"model,omitempty"`
	BalanceURL string `toml:"balance_url" json:"balance_url,omitempty"`
}

// LLMConfig holds OpenAI-compatible enrichment settings.
type LLMConfig struct {
	Enabled             bool                 `toml:"enabled" json:"enabled"`
	BaseURL             string               `toml:"base_url" json:"base_url,omitempty"`
	APIKey              string               `toml:"api_key" json:"-"`
	Model               string               `toml:"model" json:"model,omitempty"`
	ReasoningEffort     string               `toml:"reasoning_effort" json:"reasoning_effort,omitempty"`
	MinUserMessages     int                  `toml:"min_user_messages" json:"min_user_messages"`
	ReenrichMsgDelta    int                  `toml:"reenrich_msg_delta" json:"reenrich_msg_delta"`
	ReenrichIdleMinutes int                  `toml:"reenrich_idle_minutes" json:"reenrich_idle_minutes"`
	Concurrency         int                  `toml:"concurrency" json:"concurrency"`
	Periodic            bool                 `toml:"periodic" json:"periodic"`
	BalanceURL          string               `toml:"balance_url" json:"balance_url,omitempty"`
	Embed               LLMEmbedConfig       `toml:"embed" json:"embed,omitempty"`
	Providers           map[string]LLMConfig `toml:"providers" json:"providers,omitempty"`
	Usage               map[string]string    `toml:"usage" json:"usage,omitempty"`
	// UsageModel holds the per-usage model override. Providers carry only the
	// connection (base_url/api_key); the model a usage runs is stored here so a
	// single provider/key can serve different models across usages.
	UsageModel       map[string]string `toml:"usage_model" json:"usage_model,omitempty"`
	llmEnvEnabledSet bool
}

// AutomatedConfig holds user-supplied additions to the
// automated-session classifier. Parse-only; all semantic
// normalization (trim, dedupe, length cap, built-in overlap
// drop) happens inside db.SetUserAutomationPrefixes.
type AutomatedConfig struct {
	Prefixes []string `toml:"prefixes" json:"prefixes,omitempty"`
}

// AgentConfig holds per-agent runtime overrides.
type AgentConfig struct {
	Binary string `json:"binary,omitempty" toml:"binary"`
}

type CustomModelRate struct {
	Input         float64 `json:"input" toml:"input"`
	Output        float64 `json:"output" toml:"output"`
	CacheCreation float64 `json:"cache_creation,omitempty" toml:"cache_creation"`
	CacheRead     float64 `json:"cache_read,omitempty" toml:"cache_read"`
}

// RemoteHost describes one SSH target for config-driven
// `agentsview sync` fan-out. Host is required; User and Port are
// optional (Port 0 means the ssh default of 22).
type RemoteHost struct {
	Host string `toml:"host" json:"host"`
	User string `toml:"user,omitempty" json:"user,omitempty"`
	Port int    `toml:"port,omitempty" json:"port,omitempty"`
}

// Config holds all application configuration.
type Config struct {
	Host                 string                 `json:"host" toml:"host"`
	Port                 int                    `json:"port" toml:"port"`
	DataDir              string                 `json:"data_dir" toml:"data_dir"`
	DBPath               string                 `json:"-" toml:"-"`
	PublicURL            string                 `json:"public_url,omitempty" toml:"public_url"`
	PublicOrigins        []string               `json:"public_origins,omitempty" toml:"public_origins"`
	Proxy                ProxyConfig            `json:"proxy,omitempty" toml:"proxy"`
	WatchExcludePatterns []string               `json:"watch_exclude_patterns,omitempty" toml:"watch_exclude_patterns"`
	CursorSecret         string                 `json:"cursor_secret" toml:"cursor_secret"`
	GithubToken          string                 `json:"github_token,omitempty" toml:"github_token"`
	Terminal             TerminalConfig         `json:"terminal,omitempty" toml:"terminal"`
	AuthToken            string                 `json:"auth_token,omitempty" toml:"auth_token"`
	RequireAuth          bool                   `json:"require_auth" toml:"require_auth"`
	NoBrowser            bool                   `json:"no_browser" toml:"no_browser"`
	DisableUpdateCheck   bool                   `json:"disable_update_check" toml:"disable_update_check"`
	NoSync               bool                   `json:"-" toml:"-"`
	PG                   PGConfig               `json:"pg,omitempty" toml:"pg"`
	DuckDB               DuckDBConfig           `json:"duckdb,omitempty" toml:"duckdb"`
	LLM                  LLMConfig              `json:"llm,omitempty" toml:"llm"`
	Automated            AutomatedConfig        `json:"automated,omitempty" toml:"automated"`
	Agent                map[string]AgentConfig `json:"agent,omitempty" toml:"agent"`
	WriteTimeout         time.Duration          `json:"-" toml:"-"`

	// AgentDirs maps each AgentType to its configured
	// directories. Single-dir agents store a one-element
	// slice; unconfigured agents use nil.
	AgentDirs map[parser.AgentType][]string `json:"-" toml:"-"`

	// agentDirSource tracks how each agent's dirs were
	// set so loadFile doesn't override env-set values.
	agentDirSource map[parser.AgentType]dirSource

	// envBackupEnabledSet records that AGENTSVIEW_BACKUP_ENABLED was present in
	// the environment, so the config file does not override the env's choice.
	envBackupEnabledSet bool

	// envConsolidateEnabledSet records that AGENTSVIEW_CONSOLIDATE_ENABLED was
	// present in the environment, so the config file does not override it.
	envConsolidateEnabledSet bool

	// envConsolidateIntervalSet records that AGENTSVIEW_CONSOLIDATE_INTERVAL was
	// present in the environment, so the config file does not override it.
	envConsolidateIntervalSet bool

	// envExtractEnabledSet records that AGENTSVIEW_EXTRACT_ENABLED was present
	// in the environment, so the config file does not override the env's choice.
	envExtractEnabledSet bool

	ResultContentBlockedCategories []string `json:"result_content_blocked_categories,omitempty" toml:"result_content_blocked_categories"`

	// EventsCoalesceInterval is the minimum wall-clock time between
	// SSE data_changed broadcasts to connected clients. Emits that
	// arrive within this window after a prior broadcast are coalesced
	// into a single trailing broadcast, bounding dashboard refetch
	// work during bursts of sync activity. Zero disables coalescing.
	EventsCoalesceInterval time.Duration `json:"events_coalesce_interval,omitempty" toml:"events_coalesce_interval"`

	CustomModelPricing map[string]CustomModelRate `json:"custom_model_pricing,omitempty" toml:"custom_model_pricing"`

	// RemoteHosts is the config-file list of SSH targets that
	// `agentsview sync` (with no --host) syncs after the local
	// pass. CLI/config-file only; never serialized to the
	// settings API, so there is no web-UI editing of this list.
	RemoteHosts []RemoteHost `json:"-" toml:"-"`

	// HostExplicit is true when the user passed --host on the CLI.
	// Used to prevent auto-bind to 0.0.0.0 when the user
	// explicitly requested a specific host.
	HostExplicit bool `json:"-" toml:"-"`

	// SkillsCatalogDir is the coding-skills catalog directory (the one
	// containing catalog.json) used by the skill-governance views. When
	// empty, New() probes the default ~/.dotfiles/coding-skills; when
	// nothing is found the skills feature stays empty and fail-open.
	SkillsCatalogDir string `json:"skills_catalog_dir,omitempty" toml:"skills_catalog_dir"`

	// MemoryDir is the user-memory SSOT directory containing the *.md
	// notes used by the read-only memory views. When empty, New() probes
	// the default ~/.dotfiles/memory/user; when nothing is found the
	// memory feature stays empty and fail-open.
	MemoryDir string `json:"memory_dir,omitempty" toml:"memory_dir"`

	// CCMemoryDir is the root of CC-native auto-memory: a directory whose
	// immediate children are project dirs each holding a memory/ subdir
	// (<project>/memory/*.md). When empty, ResolveCCMemoryDir probes the
	// default ~/.claude/projects; when nothing is found the CC-native memory
	// source stays empty and fail-open. Override via AGENTSVIEW_CC_MEMORY_DIR.
	CCMemoryDir string `json:"cc_memory_dir,omitempty" toml:"cc_memory_dir"`

	// VaultRoots are the roots scanned for dev-workflow run records under
	// `.long-loop/<slug>/`. When empty, ResolveVaultRoots probes the
	// default ~/.dotfiles. Multiple roots may be configured; via env they
	// are comma-separated (AGENTSVIEW_VAULT_ROOTS).
	VaultRoots []string `json:"vault_roots,omitempty" toml:"vault_roots"`

	// Consolidate holds the optional independent LLM settings used by the
	// background staging->memory/user consolidation worker. Any unset field
	// falls back to the corresponding LLM field (see ConsolidateLLM). Env:
	// AGENTSVIEW_CONSOLIDATE_BASE_URL / _API_KEY / _MODEL.
	Consolidate LLMConfig `json:"consolidate,omitempty" toml:"consolidate"`

	// ConsolidateEnabled gates the background consolidation worker. It
	// defaults to OFF: auto-writing LLM-decided notes into memory/user is a
	// side-effecting action, so the first run must be an explicit opt-in
	// (UI/config/env AGENTSVIEW_CONSOLIDATE_ENABLED). Once enabled the worker
	// runs fully automatically on the configured interval.
	ConsolidateEnabled bool `json:"consolidate_enabled" toml:"consolidate_enabled"`

	// ConsolidateInterval is the period between consolidation runs once
	// enabled. Zero selects the default (24h). Env:
	// AGENTSVIEW_CONSOLIDATE_INTERVAL (a Go duration string).
	ConsolidateInterval time.Duration `json:"consolidate_interval,omitempty" toml:"consolidate_interval"`

	// ConsolidateBatchSize caps how many candidates one consolidation cycle
	// processes, so a burst backlog is worked off over successive cycles instead
	// of one oversized LLM prompt + a flood of recall calls. Zero selects the
	// default (20).
	ConsolidateBatchSize int `json:"consolidate_batch_size,omitempty" toml:"consolidate_batch_size"`

	// SynthesizeEnabled gates the background topic-synthesis worker (atomic
	// notes -> coherent topic notes). Default OFF: it auto-writes into
	// memory/user, so the first run is an explicit opt-in.
	SynthesizeEnabled bool `json:"synthesize_enabled" toml:"synthesize_enabled"`

	// SynthesizeInterval is the period between synthesis runs once enabled. Zero
	// selects the default (24h).
	SynthesizeInterval time.Duration `json:"synthesize_interval,omitempty" toml:"synthesize_interval"`

	// ExtractEnabled gates the background LLM extraction worker. It defaults to
	// OFF so no LLM/file side effect happens without explicit opt-in.
	ExtractEnabled bool `json:"extract_enabled" toml:"extract_enabled"`

	// ExtractInterval is the period between extraction runs once enabled. Zero
	// selects the default (24h). Env: AGENTSVIEW_EXTRACT_INTERVAL.
	ExtractInterval time.Duration `json:"extract_interval,omitempty" toml:"extract_interval"`

	// DotfilesRoot is the dotfiles repository root passed to
	// assist_consolidate.py as --root. When empty it is derived from the
	// resolved memory dir (its two-levels-up parent), so the script writes
	// into the same memory/user the existing syncer scans. Env:
	// AGENTSVIEW_DOTFILES_ROOT (override only; normally derived).
	DotfilesRoot string `json:"dotfiles_root,omitempty" toml:"dotfiles_root"`

	// MemoryBackupRepo is the resolved `<owner>/<name>` full name of the
	// PRIVATE GitHub repo claimed for memory backup (Phase 04 gh-connect).
	// Phase 05 reads it to push. Empty means no backup target is configured.
	MemoryBackupRepo string `json:"memory_backup_repo,omitempty" toml:"memory_backup_repo"`

	// MemoryBackupLinked reports whether MemoryBackupRepo was successfully
	// validated/claimed (private + marker) so the UI can show a connected
	// status. It is set together with MemoryBackupRepo on a successful connect.
	MemoryBackupLinked bool `json:"memory_backup_linked" toml:"memory_backup_linked"`

	// BackupEnabled gates the background backup-push worker (Phase 05). It
	// defaults to OFF: pushing memory to a remote is a side-effecting action, so
	// the first run is an explicit opt-in (UI/config/env
	// AGENTSVIEW_BACKUP_ENABLED). Once enabled the worker pushes automatically
	// on the configured interval while agentsview runs (timer bound to the
	// process, no system cron).
	BackupEnabled bool `json:"backup_enabled" toml:"backup_enabled"`

	// BackupInterval is the period between backup pushes once enabled. Zero
	// selects the default (1h). Env: AGENTSVIEW_BACKUP_INTERVAL (a Go duration
	// string).
	BackupInterval time.Duration `json:"backup_interval,omitempty" toml:"backup_interval"`

	// BackupWorkspaceDir is the isolated git working dir the backup worker owns
	// (its .git, remote, and pushes). When empty it defaults to
	// <DataDir>/memory-backup. Env: AGENTSVIEW_BACKUP_WORKSPACE.
	BackupWorkspaceDir string `json:"backup_workspace_dir,omitempty" toml:"backup_workspace_dir"`
}

// defaultBackupInterval is the period between backup pushes when BackupInterval
// is left at its zero value.
const defaultBackupInterval = time.Hour

// ResolveBackupInterval returns the effective backup-push interval, substituting
// the 1h default for a non-positive configured value.
func (c *Config) ResolveBackupInterval() time.Duration {
	if c.BackupInterval > 0 {
		return c.BackupInterval
	}
	return defaultBackupInterval
}

// ResolveBackupWorkspaceDir returns the isolated backup working dir. It honors
// an explicit BackupWorkspaceDir, otherwise defaults to <DataDir>/memory-backup
// — a directory entirely outside any source repo, so the backup's git never
// touches memory/user's .git, the main repo, or the dotfiles repo.
func (c *Config) ResolveBackupWorkspaceDir() string {
	if strings.TrimSpace(c.BackupWorkspaceDir) != "" {
		return c.BackupWorkspaceDir
	}
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.DataDir, "memory-backup")
}

// defaultConsolidateInterval is the period between consolidation runs when
// ConsolidateInterval is left at its zero value.
const defaultConsolidateInterval = 24 * time.Hour

// defaultConsolidateBatchSize caps candidates per consolidation cycle when
// ConsolidateBatchSize is left at its zero value.
const defaultConsolidateBatchSize = 20

// defaultExtractInterval is the period between extraction runs when
// ExtractInterval is left at its zero value.
const defaultExtractInterval = 24 * time.Hour

// ResolveConsolidateInterval returns the effective consolidation interval,
// substituting the 24h default for a non-positive configured value.
func (c *Config) ResolveConsolidateInterval() time.Duration {
	if c.ConsolidateInterval > 0 {
		return c.ConsolidateInterval
	}
	return defaultConsolidateInterval
}

// ResolveConsolidateBatchSize returns the effective per-cycle candidate cap,
// substituting the default for a non-positive configured value.
func (c *Config) ResolveConsolidateBatchSize() int {
	if c.ConsolidateBatchSize > 0 {
		return c.ConsolidateBatchSize
	}
	return defaultConsolidateBatchSize
}

// ResolveSynthesizeInterval returns the effective synthesis interval (default 24h).
func (c *Config) ResolveSynthesizeInterval() time.Duration {
	if c.SynthesizeInterval > 0 {
		return c.SynthesizeInterval
	}
	return 24 * time.Hour
}

// ResolveExtractInterval returns the effective extraction interval,
// substituting the 24h default for a non-positive configured value.
func (c *Config) ResolveExtractInterval() time.Duration {
	if c.ExtractInterval > 0 {
		return c.ExtractInterval
	}
	return defaultExtractInterval
}

// ConsolidateLLM returns the effective LLM settings for the consolidation
// worker: the independent Consolidate config with each unset connection field
// filled from the main LLM config. Enabled/periodic gating is handled
// separately via ConsolidateEnabled, so only the connection fields are merged.
func (c *Config) ConsolidateLLM() LLMConfig {
	out := c.resolveLegacyConsolidateLLM()
	if provider, ok := c.resolveBoundProvider("consolidate"); ok {
		out = overlayProvider(out, provider)
	}
	return out
}

func (c *Config) resolveLegacyConsolidateLLM() LLMConfig {
	out := c.LLM
	// Consolidation is a semantic-triage classifier, not a reasoning task. Do
	// NOT inherit the base [llm] reasoning_effort: deepseek honors it (adds a
	// thinking pass that ~tripled call latency in practice), which pushed cycles
	// past the LLM client timeout and failed the whole cycle. An explicit
	// [consolidate] or bound-provider reasoning_effort below still wins.
	out.ReasoningEffort = ""
	if c.Consolidate.BaseURL != "" {
		out.BaseURL = c.Consolidate.BaseURL
	}
	if c.Consolidate.APIKey != "" {
		out.APIKey = c.Consolidate.APIKey
	}
	if c.Consolidate.Model != "" {
		out.Model = c.Consolidate.Model
	}
	if c.Consolidate.ReasoningEffort != "" {
		out.ReasoningEffort = c.Consolidate.ReasoningEffort
	}
	return out
}

// ResolveUsageLLM returns the effective LLM settings for a named usage. A
// usage binding selects a named provider from [llm.providers]; unbound usages
// preserve the legacy resolver behavior so existing config.toml files keep
// their previous effective values.
func (c *Config) ResolveUsageLLM(usage string) LLMConfig {
	usage = strings.TrimSpace(usage)
	if usage == "consolidate" {
		base := c.ConsolidateLLM()
		if m := c.usageModelOverride(usage); m != "" {
			base.Model = m
		}
		return base
	}
	base := c.ResolveLLM()
	if provider, ok := c.resolveBoundProvider(usage); ok {
		base = overlayProvider(base, provider)
	}
	if usage == "embed" {
		base = c.resolveUsageEmbed(base, usage)
		if m := c.usageModelOverride(usage); m != "" {
			base.Embed.Model = m
		}
		return base
	}
	if m := c.usageModelOverride(usage); m != "" {
		base.Model = m
	}
	return base
}

// usageModelOverride returns the per-usage model override, or "" when unset.
// This is the model a usage runs; the bound provider supplies only the
// connection (base_url/api_key).
func (c *Config) usageModelOverride(usage string) string {
	if c == nil || len(c.LLM.UsageModel) == 0 {
		return ""
	}
	return strings.TrimSpace(c.LLM.UsageModel[strings.TrimSpace(usage)])
}

// DanglingLLMUsageBindings returns usage names that point to missing providers.
// This keeps fail-open fallback behavior while making configuration errors
// observable to callers that render the config surface.
func (c *Config) DanglingLLMUsageBindings() []string {
	if c == nil || len(c.LLM.Usage) == 0 {
		return nil
	}
	out := make([]string, 0)
	for usage, providerName := range c.LLM.Usage {
		usage = strings.TrimSpace(usage)
		providerName = strings.TrimSpace(providerName)
		if usage == "" || providerName == "" {
			continue
		}
		if _, ok := c.LLM.Providers[providerName]; !ok {
			out = append(out, usage)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}

func (c *Config) resolveUsageEmbed(base LLMConfig, usage string) LLMConfig {
	provider, ok := c.resolveBoundProvider(usage)
	if !ok {
		return base
	}
	base.Embed = LLMEmbedConfig{
		BaseURL:    provider.BaseURL,
		APIKey:     provider.APIKey,
		Model:      provider.Model,
		BalanceURL: provider.BalanceURL,
	}
	return base
}

func (c *Config) resolveBoundProvider(usage string) (LLMConfig, bool) {
	if c == nil || len(c.LLM.Usage) == 0 || len(c.LLM.Providers) == 0 {
		return LLMConfig{}, false
	}
	providerName := strings.TrimSpace(c.LLM.Usage[strings.TrimSpace(usage)])
	if providerName == "" {
		return LLMConfig{}, false
	}
	provider, ok := c.LLM.Providers[providerName]
	if !ok {
		return LLMConfig{}, false
	}
	return provider, true
}

func overlayProvider(base, provider LLMConfig) LLMConfig {
	if provider.Enabled {
		base.Enabled = true
	}
	if provider.BaseURL != "" {
		base.BaseURL = provider.BaseURL
	}
	if provider.APIKey != "" {
		base.APIKey = provider.APIKey
	}
	if provider.Model != "" {
		base.Model = provider.Model
	}
	if provider.ReasoningEffort != "" {
		base.ReasoningEffort = provider.ReasoningEffort
	}
	if provider.BalanceURL != "" {
		base.BalanceURL = provider.BalanceURL
	}
	return base
}

// ResolveDotfilesRoot returns the dotfiles repository root used as
// assist_consolidate.py --root. It honors an explicit DotfilesRoot override,
// otherwise derives it from the resolved memory dir: memory/user lives at
// <dotfiles>/memory/user, so the root is the dir's two-levels-up parent. This
// keeps the script's write target bound to the exact memory dir the existing
// syncer scans (locked decision B1). Returns "" when neither is resolvable.
func (c *Config) ResolveDotfilesRoot() string {
	if strings.TrimSpace(c.DotfilesRoot) != "" {
		return c.DotfilesRoot
	}
	dir := c.ResolveMemoryDir()
	if dir == "" {
		return ""
	}
	// <dotfiles>/memory/user -> <dotfiles>
	return filepath.Dir(filepath.Dir(dir))
}

type dirSource int

const (
	dirDefault dirSource = iota
	dirFile
	dirEnv
)

// ResolveDirs returns the effective directories for an agent.
func (c *Config) ResolveDirs(
	agent parser.AgentType,
) []string {
	return c.AgentDirs[agent]
}

// IsUserConfigured reports whether the agent's directories
// were explicitly set by the user (via env var or config file)
// rather than populated from defaults.
func (c *Config) IsUserConfigured(
	agent parser.AgentType,
) bool {
	return c.agentDirSource[agent] != dirDefault
}

// ValidateRemoteHosts checks the configured remote_hosts entries
// for semantic errors: a non-empty host and a port within 0..65535
// (0 means the ssh default). It checks the trimmed values that
// loadFile already normalized, so what is validated here is exactly
// what is passed to ssh. Returns an aggregated error naming every
// offending entry, or nil when all entries are valid.
func (c Config) ValidateRemoteHosts() error {
	var problems []string
	seen := make(map[string]int, len(c.RemoteHosts))
	for i, h := range c.RemoteHosts {
		if h.Host == "" {
			problems = append(problems,
				fmt.Sprintf("entry %d: host is required", i+1))
		}
		if h.Port < 0 || h.Port > 65535 {
			problems = append(problems,
				fmt.Sprintf("entry %d (%q): invalid port %d",
					i+1, h.Host, h.Port))
		}
		// Remote sync namespaces sessions and the skip cache by
		// host alone (see ssh.RemoteSync), so two entries sharing a
		// host collide regardless of user/port. Reject duplicates
		// rather than silently share or overwrite cached state.
		if h.Host != "" {
			if first, ok := seen[h.Host]; ok {
				problems = append(problems,
					fmt.Sprintf("entry %d: duplicate host %q (already at entry %d)",
						i+1, h.Host, first))
			} else {
				seen[h.Host] = i + 1
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("remote_hosts: %s",
			strings.Join(problems, "; "))
	}
	return nil
}

// Default returns a Config with default values.
func Default() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf(
			"determining home directory: %w", err,
		)
	}
	dataDir := filepath.Join(home, ".agentsview")

	agentDirs := make(map[parser.AgentType][]string)
	agentDirSource := make(map[parser.AgentType]dirSource)
	for _, def := range parser.Registry {
		dirs := make([]string, len(def.DefaultDirs))
		for i, rel := range def.DefaultDirs {
			dirs[i] = filepath.Join(home, rel)
		}
		agentDirs[def.Type] = dirs
		agentDirSource[def.Type] = dirDefault
	}

	return Config{
		Host:                           "127.0.0.1",
		Port:                           8080,
		DataDir:                        dataDir,
		DBPath:                         filepath.Join(dataDir, "sessions.db"),
		WriteTimeout:                   30 * time.Second,
		AgentDirs:                      agentDirs,
		agentDirSource:                 agentDirSource,
		WatchExcludePatterns:           []string{".git", "node_modules", "__pycache__", ".venv", "venv", "vendor", ".next"},
		ResultContentBlockedCategories: []string{"Read", "Glob"},
		EventsCoalesceInterval:         10 * time.Second,
		Agent:                          map[string]AgentConfig{},
		LLM: LLMConfig{
			ReasoningEffort:     "medium",
			MinUserMessages:     3,
			ReenrichMsgDelta:    20,
			ReenrichIdleMinutes: 30,
			Concurrency:         3,
		},
	}, nil
}

// Load builds a Config by layering: defaults < config file < env < flags.
// The provided FlagSet must already be parsed by the caller.
// Only flags that were explicitly set override the lower layers.
func Load(fs *flag.FlagSet) (Config, error) {
	cfg, err := LoadMinimal()
	if err != nil {
		return cfg, err
	}
	applyFlags(&cfg, fs)
	if err := finalize(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// LoadPFlags builds a Config from a parsed Cobra/pflag FlagSet.
func LoadPFlags(fs *pflag.FlagSet) (Config, error) {
	cfg, err := LoadMinimal()
	if err != nil {
		return cfg, err
	}
	applyPFlags(&cfg, fs)
	if err := finalize(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// LoadPGServe builds a Config for `pg serve` by preserving
// shared and PG settings from defaults/env/config file while
// resetting serve-specific network/browser settings to defaults.
// Only explicitly provided serve flags are applied on top.
func LoadPGServe(fs *flag.FlagSet) (Config, error) {
	cfg, err := loadPGServeBase()
	if err != nil {
		return cfg, err
	}
	applyFlags(&cfg, fs)
	if err := finalize(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// LoadPGServePFlags builds a PG serve config from a parsed Cobra/pflag FlagSet.
func LoadPGServePFlags(fs *pflag.FlagSet) (Config, error) {
	cfg, err := loadPGServeBase()
	if err != nil {
		return cfg, err
	}
	applyPFlags(&cfg, fs)
	if err := finalize(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// LoadDuckDBServePFlags builds a DuckDB serve config from a parsed Cobra/pflag
// FlagSet. It intentionally uses the same isolated serve defaults as pg serve.
func LoadDuckDBServePFlags(fs *pflag.FlagSet) (Config, error) {
	cfg, err := loadPGServeBase()
	if err != nil {
		return cfg, err
	}
	applyPFlags(&cfg, fs)
	if err := finalize(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loadPGServeBase() (Config, error) {
	cfg, err := Default()
	if err != nil {
		return cfg, err
	}
	cfg.loadEnv()
	if err := cfg.loadFile(); err != nil {
		return cfg, fmt.Errorf("loading config file: %w", err)
	}
	if err := cfg.ensureCursorSecret(); err != nil {
		return cfg, fmt.Errorf("ensuring cursor secret: %w", err)
	}
	cfg.DBPath = filepath.Join(cfg.DataDir, "sessions.db")

	// pg serve intentionally ignores persisted normal serve/public/proxy
	// settings so an existing SQLite-backed serve deployment cannot silently
	// reconfigure the PG-backed server. Until a dedicated pg-serve config
	// namespace exists, only explicit pg-serve flags should shape its
	// network/proxy behavior.
	cfg.Host = "127.0.0.1"
	cfg.Port = 8080
	cfg.PublicURL = ""
	cfg.PublicOrigins = nil
	cfg.Proxy = ProxyConfig{}
	cfg.NoBrowser = false
	cfg.HostExplicit = false
	return cfg, nil
}

// LoadMinimal builds a Config from defaults, env, and config file,
// without parsing CLI flags. Use this for subcommands that manage
// their own flag sets.
func LoadMinimal() (Config, error) {
	cfg, err := Default()
	if err != nil {
		return cfg, err
	}
	cfg.loadEnv()

	if err := cfg.loadFile(); err != nil {
		return cfg, fmt.Errorf("loading config file: %w", err)
	}
	if err := finalize(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.ensureCursorSecret(); err != nil {
		return cfg, fmt.Errorf("ensuring cursor secret: %w", err)
	}
	cfg.DBPath = filepath.Join(cfg.DataDir, "sessions.db")
	return cfg, nil
}

func (c *Config) configPath() string {
	return filepath.Join(c.DataDir, "config.toml")
}

func (c *Config) jsonConfigPath() string {
	return filepath.Join(c.DataDir, "config.json")
}

// migrateJSONToTOML converts config.json to config.toml if
// config.json exists and config.toml does not. The original
// JSON file is renamed to config.json.bak.
func (c *Config) migrateJSONToTOML() error {
	jsonPath := c.jsonConfigPath()
	tomlPath := c.configPath()

	if _, err := os.Stat(tomlPath); err == nil {
		return nil // TOML already exists
	}
	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return nil // no JSON to migrate
	}
	if err != nil {
		return fmt.Errorf("reading config.json for migration: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parsing config.json for migration: %w", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return fmt.Errorf("encoding config.toml: %w", err)
	}
	if err := os.WriteFile(tomlPath, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("writing config.toml: %w", err)
	}
	if err := os.Rename(jsonPath, jsonPath+".bak"); err != nil {
		return fmt.Errorf("renaming config.json to .bak: %w", err)
	}
	return nil
}

func (c *Config) loadFile() error {
	if err := c.migrateJSONToTOML(); err != nil {
		return err
	}

	path := c.configPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	var file struct {
		GithubToken                    string                     `toml:"github_token"`
		CursorSecret                   string                     `toml:"cursor_secret"`
		PublicURL                      string                     `toml:"public_url"`
		PublicOrigins                  []string                   `toml:"public_origins"`
		Proxy                          ProxyConfig                `toml:"proxy"`
		WatchExcludePatterns           []string                   `toml:"watch_exclude_patterns"`
		ResultContentBlockedCategories []string                   `toml:"result_content_blocked_categories"`
		Terminal                       TerminalConfig             `toml:"terminal"`
		AuthToken                      string                     `toml:"auth_token"`
		RequireAuth                    bool                       `toml:"require_auth"`
		RemoteAccess                   bool                       `toml:"remote_access"`
		DisableUpdateCheck             bool                       `toml:"disable_update_check"`
		PG                             PGConfig                   `toml:"pg"`
		DuckDB                         DuckDBConfig               `toml:"duckdb"`
		LLM                            LLMConfig                  `toml:"llm"`
		Automated                      AutomatedConfig            `toml:"automated"`
		Agent                          map[string]AgentConfig     `toml:"agent"`
		EventsCoalesceInterval         time.Duration              `toml:"events_coalesce_interval"`
		CustomModelPricing             map[string]CustomModelRate `toml:"custom_model_pricing"`
		RemoteHosts                    []RemoteHost               `toml:"remote_hosts"`
		SkillsCatalogDir               string                     `toml:"skills_catalog_dir"`
		MemoryDir                      string                     `toml:"memory_dir"`
		CCMemoryDir                    string                     `toml:"cc_memory_dir"`
		VaultRoots                     []string                   `toml:"vault_roots"`
		MemoryBackupRepo               string                     `toml:"memory_backup_repo"`
		MemoryBackupLinked             bool                       `toml:"memory_backup_linked"`
		BackupEnabled                  bool                       `toml:"backup_enabled"`
		BackupInterval                 time.Duration              `toml:"backup_interval"`
		BackupWorkspaceDir             string                     `toml:"backup_workspace_dir"`
		ExtractEnabled                 bool                       `toml:"extract_enabled"`
		ExtractInterval                time.Duration              `toml:"extract_interval"`
		ConsolidateEnabled             bool                       `toml:"consolidate_enabled"`
		ConsolidateInterval            time.Duration              `toml:"consolidate_interval"`
		ConsolidateBatchSize           int                        `toml:"consolidate_batch_size"`
		SynthesizeEnabled              bool                       `toml:"synthesize_enabled"`
		SynthesizeInterval             time.Duration              `toml:"synthesize_interval"`
	}
	meta, err := toml.DecodeFile(path, &file)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}
	if file.GithubToken != "" {
		c.GithubToken = file.GithubToken
	}
	if file.CursorSecret != "" {
		c.CursorSecret = file.CursorSecret
	}
	if file.PublicURL != "" {
		c.PublicURL = file.PublicURL
	}
	if file.PublicOrigins != nil {
		c.PublicOrigins = file.PublicOrigins
	}
	if file.Proxy.Mode != "" || file.Proxy.Bin != "" ||
		file.Proxy.BindHost != "" || file.Proxy.PublicPort != 0 ||
		file.Proxy.TLSCert != "" || file.Proxy.TLSKey != "" ||
		file.Proxy.AllowedSubnets != nil {
		c.Proxy = file.Proxy
	}
	if file.WatchExcludePatterns != nil {
		c.WatchExcludePatterns = file.WatchExcludePatterns
	}
	if file.ResultContentBlockedCategories != nil {
		c.ResultContentBlockedCategories = file.ResultContentBlockedCategories
	}
	if file.Terminal.Mode != "" {
		c.Terminal = file.Terminal
	}
	if file.AuthToken != "" {
		c.AuthToken = file.AuthToken
	}
	c.RequireAuth = file.RequireAuth || file.RemoteAccess
	c.DisableUpdateCheck = file.DisableUpdateCheck
	// env (loadEnv, AGENTSVIEW_SKILLS_DIR) runs first and wins; the
	// config file only fills the value when env left it unset.
	if file.SkillsCatalogDir != "" && c.SkillsCatalogDir == "" {
		c.SkillsCatalogDir = file.SkillsCatalogDir
	}
	// env (loadEnv, AGENTSVIEW_MEMORY_DIR) runs first and wins; the
	// config file only fills the value when env left it unset.
	if file.MemoryDir != "" && c.MemoryDir == "" {
		c.MemoryDir = file.MemoryDir
	}
	// env (loadEnv, AGENTSVIEW_CC_MEMORY_DIR) runs first and wins; the
	// config file only fills the value when env left it unset.
	if file.CCMemoryDir != "" && c.CCMemoryDir == "" {
		c.CCMemoryDir = file.CCMemoryDir
	}
	// env (loadEnv, AGENTSVIEW_VAULT_ROOTS) runs first and wins; the
	// config file only fills the value when env left it unset.
	if len(file.VaultRoots) > 0 && len(c.VaultRoots) == 0 {
		c.VaultRoots = file.VaultRoots
	}
	// Memory backup target (Phase 04 gh-connect). Persisted via SaveSettings
	// after a successful connect; the in-memory value (set at runtime) wins,
	// so the file only fills it on a fresh load.
	if file.MemoryBackupRepo != "" && c.MemoryBackupRepo == "" {
		c.MemoryBackupRepo = file.MemoryBackupRepo
		c.MemoryBackupLinked = file.MemoryBackupLinked
	}
	// Backup-push settings (Phase 05). env (loadEnv) runs first and wins; the
	// config file only fills a value the env left unset. BackupEnabled is set
	// from the file unless an env override already pinned it (see envBackupSet).
	if !c.envBackupEnabledSet && meta.IsDefined("backup_enabled") {
		c.BackupEnabled = file.BackupEnabled
	}
	if file.BackupInterval > 0 && c.BackupInterval == 0 {
		c.BackupInterval = file.BackupInterval
	}
	if file.BackupWorkspaceDir != "" && c.BackupWorkspaceDir == "" {
		c.BackupWorkspaceDir = file.BackupWorkspaceDir
	}
	if !c.envConsolidateEnabledSet && meta.IsDefined("consolidate_enabled") {
		c.ConsolidateEnabled = file.ConsolidateEnabled
	}
	if meta.IsDefined("consolidate_interval") && !c.envConsolidateIntervalSet {
		c.ConsolidateInterval = file.ConsolidateInterval
	}
	if file.ConsolidateBatchSize > 0 && c.ConsolidateBatchSize == 0 {
		c.ConsolidateBatchSize = file.ConsolidateBatchSize
	}
	if meta.IsDefined("synthesize_enabled") {
		c.SynthesizeEnabled = file.SynthesizeEnabled
	}
	if file.SynthesizeInterval > 0 && c.SynthesizeInterval == 0 {
		c.SynthesizeInterval = file.SynthesizeInterval
	}
	if !c.envExtractEnabledSet && meta.IsDefined("extract_enabled") {
		c.ExtractEnabled = file.ExtractEnabled
	}
	if file.ExtractInterval > 0 && c.ExtractInterval == 0 {
		c.ExtractInterval = file.ExtractInterval
	}
	// Merge pg field-by-field so env vars override only
	// the fields they set, preserving config-file settings.
	if file.PG.URL != "" && c.PG.URL == "" {
		c.PG.URL = file.PG.URL
	}
	if file.PG.Schema != "" && c.PG.Schema == "" {
		c.PG.Schema = file.PG.Schema
	}
	if file.PG.MachineName != "" && c.PG.MachineName == "" {
		c.PG.MachineName = file.PG.MachineName
	}
	if file.PG.AllowInsecure {
		c.PG.AllowInsecure = true
	}
	if file.PG.Projects != nil && c.PG.Projects == nil {
		c.PG.Projects = file.PG.Projects
	}
	if file.PG.ExcludeProjects != nil && c.PG.ExcludeProjects == nil {
		c.PG.ExcludeProjects = file.PG.ExcludeProjects
	}
	// Merge duckdb field-by-field so env vars override only
	// the fields they set, preserving config-file settings.
	if file.DuckDB.Path != "" && c.DuckDB.Path == "" {
		c.DuckDB.Path = file.DuckDB.Path
	}
	if file.DuckDB.URL != "" && c.DuckDB.URL == "" {
		c.DuckDB.URL = file.DuckDB.URL
	}
	if file.DuckDB.Token != "" && c.DuckDB.Token == "" {
		c.DuckDB.Token = file.DuckDB.Token
	}
	if file.DuckDB.MachineName != "" && c.DuckDB.MachineName == "" {
		c.DuckDB.MachineName = file.DuckDB.MachineName
	}
	if file.DuckDB.AllowInsecure {
		c.DuckDB.AllowInsecure = true
	}
	if file.DuckDB.Projects != nil && c.DuckDB.Projects == nil {
		c.DuckDB.Projects = file.DuckDB.Projects
	}
	if file.DuckDB.ExcludeProjects != nil && c.DuckDB.ExcludeProjects == nil {
		c.DuckDB.ExcludeProjects = file.DuckDB.ExcludeProjects
	}
	mergeLLMConfig(c, file.LLM, meta)
	// IsDefined distinguishes "unset" (leave default 10s) from an
	// explicit "0s" (disable coalescing). Checking != 0 would silently
	// ignore the latter.
	if meta.IsDefined("events_coalesce_interval") {
		c.EventsCoalesceInterval = file.EventsCoalesceInterval
	}
	if file.Automated.Prefixes != nil {
		c.Automated.Prefixes = file.Automated.Prefixes
	}
	if len(file.Agent) > 0 {
		if c.Agent == nil {
			c.Agent = map[string]AgentConfig{}
		}
		for name, cfg := range file.Agent {
			name = strings.TrimSpace(strings.ToLower(name))
			if name == "" {
				continue
			}
			cfg.Binary = strings.TrimSpace(cfg.Binary)
			c.Agent[name] = cfg
		}
	}
	if len(file.CustomModelPricing) > 0 {
		c.CustomModelPricing = file.CustomModelPricing
	}
	if len(file.RemoteHosts) > 0 {
		hosts := make([]RemoteHost, len(file.RemoteHosts))
		for i, h := range file.RemoteHosts {
			hosts[i] = RemoteHost{
				Host: strings.TrimSpace(h.Host),
				User: strings.TrimSpace(h.User),
				Port: h.Port,
			}
		}
		c.RemoteHosts = hosts
	}

	// Parse config-file dir arrays for agents that have a
	// ConfigKey. Only apply when not already set by env var.
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return fmt.Errorf("parsing config raw: %w", err)
	}
	for _, def := range parser.Registry {
		if def.ConfigKey == "" {
			continue
		}
		rawVal, exists := raw[def.ConfigKey]
		if !exists {
			continue
		}
		if c.agentDirSource[def.Type] == dirEnv {
			continue
		}
		rawSlice, ok := rawVal.([]any)
		if !ok {
			log.Printf(
				"config: %s: expected string array: got %T",
				def.ConfigKey, rawVal,
			)
			continue
		}
		dirs := make([]string, 0, len(rawSlice))
		for _, v := range rawSlice {
			s, ok := v.(string)
			if !ok {
				log.Printf(
					"config: %s: expected string array: element is %T",
					def.ConfigKey, v,
				)
				dirs = nil
				break
			}
			dirs = append(dirs, s)
		}
		if len(dirs) > 0 {
			c.AgentDirs[def.Type] = dirs
			c.agentDirSource[def.Type] = dirFile
		}
	}
	return nil
}

func (c *Config) ensureCursorSecret() error {
	if c.CursorSecret != "" {
		return nil
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("generating secret: %w", err)
	}
	secret := base64.StdEncoding.EncodeToString(b)
	c.CursorSecret = secret

	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing, err := c.readConfigMap()
	if err != nil {
		return err
	}

	existing["cursor_secret"] = secret
	return c.writeConfigMap(existing)
}

// readConfigMap reads the TOML config file into a map. Returns
// an empty map if the file does not exist.
func (c *Config) readConfigMap() (map[string]any, error) {
	existing := make(map[string]any)
	data, err := os.ReadFile(c.configPath())
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err == nil {
		if _, err := toml.Decode(string(data), &existing); err != nil {
			return nil, fmt.Errorf("existing config invalid: %w", err)
		}
	}
	return existing, nil
}

// writeConfigMap encodes a map as TOML and writes it to the
// config file.
func (c *Config) writeConfigMap(m map[string]any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(c.configPath(), buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// dataDirFromEnv returns the data directory from the environment, preferring
// AGENTSVIEW_DATA_DIR and falling back to the legacy AGENT_VIEWER_DATA_DIR.
// Returns "" when neither is set.
func dataDirFromEnv() string {
	if v := os.Getenv("AGENTSVIEW_DATA_DIR"); v != "" {
		return v
	}
	return os.Getenv("AGENT_VIEWER_DATA_DIR")
}

func (c *Config) loadEnv() {
	for _, def := range parser.Registry {
		if v := os.Getenv(def.EnvVar); v != "" {
			c.AgentDirs[def.Type] = []string{v}
			c.agentDirSource[def.Type] = dirEnv
		}
	}
	if v := dataDirFromEnv(); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("AGENTSVIEW_PG_URL"); v != "" {
		c.PG.URL = v
	}
	if v := os.Getenv("AGENTSVIEW_PG_SCHEMA"); v != "" {
		c.PG.Schema = v
	}
	if v := os.Getenv("AGENTSVIEW_PG_MACHINE"); v != "" {
		c.PG.MachineName = v
	}
	if v := os.Getenv("AGENTSVIEW_DUCKDB_PATH"); v != "" {
		c.DuckDB.Path = v
	}
	if v := os.Getenv("AGENTSVIEW_DUCKDB_URL"); v != "" {
		c.DuckDB.URL = v
	}
	if v := os.Getenv("AGENTSVIEW_DUCKDB_TOKEN"); v != "" {
		c.DuckDB.Token = v
	}
	if v := os.Getenv("AGENTSVIEW_DUCKDB_MACHINE"); v != "" {
		c.DuckDB.MachineName = v
	}
	if v := os.Getenv("AGENTSVIEW_DISABLE_UPDATE_CHECK"); v != "" {
		c.DisableUpdateCheck = v == "1" || v == "true"
	}
	if v := os.Getenv("AGENTSVIEW_SKILLS_DIR"); v != "" {
		c.SkillsCatalogDir = v
	}
	if v, ok := os.LookupEnv("AGENTSVIEW_LLM_ENABLED"); ok {
		c.LLM.Enabled = v == "1" || strings.EqualFold(v, "true")
		c.LLM.llmEnvEnabledSet = true
	}
	if v := os.Getenv("AGENTSVIEW_LLM_BASE_URL"); v != "" {
		c.LLM.BaseURL = v
	}
	if v := os.Getenv("AGENTSVIEW_LLM_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("AGENTSVIEW_LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("AGENTSVIEW_MEMORY_DIR"); v != "" {
		c.MemoryDir = v
	}
	if v := os.Getenv("AGENTSVIEW_CC_MEMORY_DIR"); v != "" {
		c.CCMemoryDir = v
	}
	if v := os.Getenv("AGENTSVIEW_VAULT_ROOTS"); v != "" {
		roots := make([]string, 0, 2)
		for part := range strings.SplitSeq(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				roots = append(roots, p)
			}
		}
		if len(roots) > 0 {
			c.VaultRoots = roots
		}
	}
	if v := os.Getenv("AGENTSVIEW_CONSOLIDATE_BASE_URL"); v != "" {
		c.Consolidate.BaseURL = v
	}
	if v := os.Getenv("AGENTSVIEW_CONSOLIDATE_API_KEY"); v != "" {
		c.Consolidate.APIKey = v
	}
	if v := os.Getenv("AGENTSVIEW_CONSOLIDATE_MODEL"); v != "" {
		c.Consolidate.Model = v
	}
	if v, ok := os.LookupEnv("AGENTSVIEW_CONSOLIDATE_ENABLED"); ok {
		c.ConsolidateEnabled = v == "1" || strings.EqualFold(v, "true")
		c.envConsolidateEnabledSet = true
	}
	if v := os.Getenv("AGENTSVIEW_CONSOLIDATE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.ConsolidateInterval = d
			c.envConsolidateIntervalSet = true
		} else {
			log.Printf("config: invalid AGENTSVIEW_CONSOLIDATE_INTERVAL %q: %v", v, err)
		}
	}
	if v, ok := os.LookupEnv("AGENTSVIEW_EXTRACT_ENABLED"); ok {
		c.ExtractEnabled = v == "1" || strings.EqualFold(v, "true")
		c.envExtractEnabledSet = true
	}
	if v := os.Getenv("AGENTSVIEW_EXTRACT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.ExtractInterval = d
		} else {
			log.Printf("config: invalid AGENTSVIEW_EXTRACT_INTERVAL %q: %v", v, err)
		}
	}
	if v := os.Getenv("AGENTSVIEW_DOTFILES_ROOT"); v != "" {
		c.DotfilesRoot = v
	}
	if v, ok := os.LookupEnv("AGENTSVIEW_BACKUP_ENABLED"); ok {
		c.BackupEnabled = v == "1" || strings.EqualFold(v, "true")
		c.envBackupEnabledSet = true
	}
	if v := os.Getenv("AGENTSVIEW_BACKUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.BackupInterval = d
		} else {
			log.Printf("config: invalid AGENTSVIEW_BACKUP_INTERVAL %q: %v", v, err)
		}
	}
	if v := os.Getenv("AGENTSVIEW_BACKUP_WORKSPACE"); v != "" {
		c.BackupWorkspaceDir = v
	}
}

func mergeLLMConfig(c *Config, file LLMConfig, meta toml.MetaData) {
	if meta.IsDefined("llm", "enabled") && !c.LLM.llmEnvEnabledSet {
		c.LLM.Enabled = file.Enabled
	}
	if file.BaseURL != "" && c.LLM.BaseURL == "" {
		c.LLM.BaseURL = file.BaseURL
	}
	if file.APIKey != "" && c.LLM.APIKey == "" {
		c.LLM.APIKey = file.APIKey
	}
	if file.Model != "" && c.LLM.Model == "" {
		c.LLM.Model = file.Model
	}
	if meta.IsDefined("llm", "reasoning_effort") {
		c.LLM.ReasoningEffort = file.ReasoningEffort
	}
	if meta.IsDefined("llm", "min_user_messages") {
		c.LLM.MinUserMessages = file.MinUserMessages
	}
	if meta.IsDefined("llm", "reenrich_msg_delta") {
		c.LLM.ReenrichMsgDelta = file.ReenrichMsgDelta
	}
	if meta.IsDefined("llm", "reenrich_idle_minutes") {
		c.LLM.ReenrichIdleMinutes = file.ReenrichIdleMinutes
	}
	if meta.IsDefined("llm", "concurrency") {
		c.LLM.Concurrency = file.Concurrency
	}
	if meta.IsDefined("llm", "periodic") {
		c.LLM.Periodic = file.Periodic
	}
	if file.BalanceURL != "" && c.LLM.BalanceURL == "" {
		c.LLM.BalanceURL = file.BalanceURL
	}
	if file.Embed.BaseURL != "" && c.LLM.Embed.BaseURL == "" {
		c.LLM.Embed.BaseURL = file.Embed.BaseURL
	}
	if file.Embed.APIKey != "" && c.LLM.Embed.APIKey == "" {
		c.LLM.Embed.APIKey = file.Embed.APIKey
	}
	if file.Embed.Model != "" && c.LLM.Embed.Model == "" {
		c.LLM.Embed.Model = file.Embed.Model
	}
	if file.Embed.BalanceURL != "" && c.LLM.Embed.BalanceURL == "" {
		c.LLM.Embed.BalanceURL = file.Embed.BalanceURL
	}
	if len(file.Providers) > 0 {
		c.LLM.Providers = normalizeLLMProviders(file.Providers)
	}
	if len(file.Usage) > 0 {
		c.LLM.Usage = normalizeLLMUsage(file.Usage)
	}
	if len(file.UsageModel) > 0 {
		c.LLM.UsageModel = normalizeLLMUsage(file.UsageModel)
	}
}

func normalizeLLMProviders(in map[string]LLMConfig) map[string]LLMConfig {
	out := make(map[string]LLMConfig, len(in))
	for name, provider := range in {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		provider.BaseURL = strings.TrimSpace(provider.BaseURL)
		provider.APIKey = strings.TrimSpace(provider.APIKey)
		provider.Model = strings.TrimSpace(provider.Model)
		provider.ReasoningEffort = strings.TrimSpace(provider.ReasoningEffort)
		provider.BalanceURL = strings.TrimSpace(provider.BalanceURL)
		provider.Embed = LLMEmbedConfig{}
		provider.Providers = nil
		provider.Usage = nil
		provider.UsageModel = nil
		out[name] = provider
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeLLMUsage(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for usage, provider := range in {
		usage = strings.TrimSpace(usage)
		provider = strings.TrimSpace(provider)
		if usage == "" || provider == "" {
			continue
		}
		out[usage] = provider
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ResolveSkillsCatalogDir returns the effective coding-skills catalog
// directory. It prefers an explicit config/env value, then probes the
// default ~/.dotfiles/coding-skills. It returns "" (fail-open: skills
// feature disabled) when no directory with a catalog.json is found.
func (c *Config) ResolveSkillsCatalogDir() string {
	candidates := make([]string, 0, 2)
	if c.SkillsCatalogDir != "" {
		candidates = append(candidates, c.SkillsCatalogDir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".dotfiles", "coding-skills"))
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "catalog.json")); err == nil {
			return dir
		}
	}
	return ""
}

// ResolveMemoryDir returns the effective user-memory directory. It
// prefers an explicit config/env value, then probes the default
// ~/.dotfiles/memory/user. It returns "" (fail-open: memory feature
// disabled) when no candidate directory exists on disk.
func (c *Config) ResolveMemoryDir() string {
	candidates := make([]string, 0, 2)
	if c.MemoryDir != "" {
		candidates = append(candidates, c.MemoryDir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".dotfiles", "memory", "user"))
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// ResolveCCMemoryDir returns the effective root for CC-native auto-memory
// (the directory whose children are project dirs each holding a memory/
// subdir). It prefers an explicit config/env value, then probes the default
// ~/.claude/projects. It returns "" (fail-open: CC-native source disabled)
// when no candidate directory exists on disk.
func (c *Config) ResolveCCMemoryDir() string {
	candidates := make([]string, 0, 2)
	if c.CCMemoryDir != "" {
		candidates = append(candidates, c.CCMemoryDir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".claude", "projects"))
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// ResolveVaultRoots returns the effective roots scanned for dev-workflow
// run records (`<root>/.long-loop/<slug>/`). It prefers explicit
// config/env values, then falls back to the default ~/.dotfiles. Roots are
// returned even when they do not yet contain a `.long-loop` directory; the
// VaultSyncer is fail-soft and simply finds no runs. Returns an empty slice
// (vault feature disabled) only when no candidate can be resolved.
func (c *Config) ResolveVaultRoots() []string {
	if len(c.VaultRoots) > 0 {
		out := make([]string, 0, len(c.VaultRoots))
		for _, r := range c.VaultRoots {
			if r = strings.TrimSpace(r); r != "" {
				out = append(out, r)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return []string{filepath.Join(home, ".dotfiles")}
	}
	return nil
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		*f = append(*f, part)
	}
	return nil
}

func (f *stringListFlag) Type() string {
	return "stringList"
}

// RegisterServeFlags registers serve-command flags on fs.
// The caller must call fs.Parse before passing fs to Load.
func RegisterServeFlags(fs *flag.FlagSet) {
	fs.String("host", "127.0.0.1", "Host to bind to")
	fs.Int("port", 8080, "Port to listen on")
	fs.String(
		"public-url", "",
		"Public URL to trust and open for hostname or proxy access",
	)
	fs.Var(
		&stringListFlag{},
		"public-origin",
		"Trusted browser origin to allow for remote or proxied access (repeatable or comma-separated)",
	)
	fs.String(
		"proxy", "",
		"Managed reverse proxy mode (currently: caddy)",
	)
	fs.String(
		"caddy-bin", "",
		"Caddy binary to use when -proxy=caddy (default: caddy)",
	)
	fs.String(
		"proxy-bind-host", "",
		"Local interface/IP for managed Caddy to bind (default: 0.0.0.0)",
	)
	fs.Int(
		"public-port", 0,
		"External port for the public URL in managed Caddy mode (default: 8443)",
	)
	fs.String(
		"tls-cert", "",
		"TLS certificate path for managed Caddy HTTPS mode",
	)
	fs.String(
		"tls-key", "",
		"TLS key path for managed Caddy HTTPS mode",
	)
	fs.Var(
		&stringListFlag{},
		"allowed-subnet",
		"Client CIDR allowed to connect to the managed proxy (repeatable or comma-separated)",
	)
	fs.Bool(
		"no-browser", false,
		"Don't open browser on startup",
	)
	fs.Bool(
		"no-sync", false,
		"Skip initial sync and disable background sync/file watching",
	)
	fs.Bool(
		"no-update-check", false,
		"Disable the update check API endpoint",
	)
	fs.Bool(
		"require-auth", false,
		"Require a bearer token for all API requests",
	)
	fs.Duration(
		"events-coalesce-interval", 10*time.Second,
		"Minimum interval between SSE data_changed broadcasts (0 disables coalescing)",
	)
}

// RegisterServePFlags registers serve-command flags on fs.
func RegisterServePFlags(fs *pflag.FlagSet) {
	fs.String("host", "127.0.0.1", "Host to bind to")
	fs.Int("port", 8080, "Port to listen on")
	fs.String(
		"public-url", "",
		"Public URL to trust and open for hostname or proxy access",
	)
	fs.Var(
		&stringListFlag{},
		"public-origin",
		"Trusted browser origin to allow for remote or proxied access (repeatable or comma-separated)",
	)
	fs.String(
		"proxy", "",
		"Managed reverse proxy mode (currently: caddy)",
	)
	fs.String(
		"caddy-bin", "",
		"Caddy binary to use when -proxy=caddy (default: caddy)",
	)
	fs.String(
		"proxy-bind-host", "",
		"Local interface/IP for managed Caddy to bind (default: 0.0.0.0)",
	)
	fs.Int(
		"public-port", 0,
		"External port for the public URL in managed Caddy mode (default: 8443)",
	)
	fs.String(
		"tls-cert", "",
		"TLS certificate path for managed Caddy HTTPS mode",
	)
	fs.String(
		"tls-key", "",
		"TLS key path for managed Caddy HTTPS mode",
	)
	fs.Var(
		&stringListFlag{},
		"allowed-subnet",
		"Client CIDR allowed to connect to the managed proxy (repeatable or comma-separated)",
	)
	fs.Bool(
		"no-browser", false,
		"Don't open browser on startup",
	)
	fs.Bool(
		"no-sync", false,
		"Skip initial sync and disable background sync/file watching",
	)
	fs.Bool(
		"no-update-check", false,
		"Disable the update check API endpoint",
	)
	fs.Bool(
		"require-auth", false,
		"Require a bearer token for all API requests",
	)
	fs.Duration(
		"events-coalesce-interval", 10*time.Second,
		"Minimum interval between SSE data_changed broadcasts (0 disables coalescing)",
	)
}

// applyFlags copies explicitly-set flags from fs into cfg.
func applyFlags(cfg *Config, fs *flag.FlagSet) {
	if fs == nil {
		return
	}
	fs.Visit(func(f *flag.Flag) {
		applyFlagValue(cfg, f.Name, f.Value.String())
	})
}

// applyPFlags copies explicitly-set pflags from fs into cfg.
func applyPFlags(cfg *Config, fs *pflag.FlagSet) {
	if fs == nil {
		return
	}
	fs.Visit(func(f *pflag.Flag) {
		applyFlagValue(cfg, f.Name, f.Value.String())
	})
}

func applyFlagValue(cfg *Config, name, value string) {
	switch name {
	case "host":
		cfg.Host = value
		cfg.HostExplicit = true
	case "port":
		cfg.Port, _ = strconv.Atoi(value)
	case "public-url":
		cfg.PublicURL = value
	case "public-origin":
		cfg.PublicOrigins = splitFlagList(value)
	case "proxy":
		cfg.Proxy.Mode = value
	case "caddy-bin":
		cfg.Proxy.Bin = value
	case "proxy-bind-host":
		cfg.Proxy.BindHost = value
	case "public-port":
		cfg.Proxy.PublicPort, _ = strconv.Atoi(value)
	case "tls-cert":
		cfg.Proxy.TLSCert = value
	case "tls-key":
		cfg.Proxy.TLSKey = value
	case "allowed-subnet":
		cfg.Proxy.AllowedSubnets = splitFlagList(value)
	case "no-browser":
		cfg.NoBrowser = value == "true"
	case "no-sync":
		cfg.NoSync = value == "true"
	case "no-update-check":
		cfg.DisableUpdateCheck = value == "true"
	case "require-auth":
		cfg.RequireAuth = value == "true"
	case "events-coalesce-interval":
		if d, err := time.ParseDuration(value); err == nil {
			cfg.EventsCoalesceInterval = d
		}
	}
}

func splitFlagList(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func finalize(cfg *Config) error {
	var err error
	if err := normalizeProxyConfig(&cfg.Proxy); err != nil {
		return err
	}
	cfg.PublicURL, err = resolvePublicURL(cfg.PublicURL, cfg.Proxy)
	if err != nil {
		return fmt.Errorf("invalid public url: %w", err)
	}
	cfg.PublicOrigins, err = normalizePublicOrigins(cfg.PublicOrigins)
	if err != nil {
		return fmt.Errorf("invalid public origins: %w", err)
	}
	if cfg.PublicURL != "" {
		cfg.PublicOrigins, err = normalizePublicOrigins(
			append(cfg.PublicOrigins, cfg.PublicURL),
		)
		if err != nil {
			return fmt.Errorf("invalid public url: %w", err)
		}
	}
	return nil
}

func resolvePublicURL(value string, proxyCfg ProxyConfig) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if u == nil || u.Host == "" {
		return "", fmt.Errorf("%q must include a host", value)
	}
	if u.User != nil {
		return "", fmt.Errorf("%q must not include user info", value)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%q must not include query or fragment", value)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("%q must not include a path", value)
	}
	if proxyCfg.Mode != "caddy" {
		return normalizePublicOrigin(value)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%q must use http or https", value)
	}
	resolvedPort := proxyCfg.PublicPort
	if resolvedPort == 0 {
		resolvedPort = 8443
	}
	if rawPort := u.Port(); rawPort != "" {
		explicitPort, err := strconv.Atoi(rawPort)
		if err != nil || explicitPort < 1 || explicitPort > 65535 {
			return "", fmt.Errorf("%q has an invalid port", value)
		}
		if proxyCfg.PublicPort != 0 && explicitPort != proxyCfg.PublicPort {
			return "", fmt.Errorf(
				"%q conflicts with configured public port %d",
				value, proxyCfg.PublicPort,
			)
		}
		resolvedPort = explicitPort
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("%q must include a host", value)
	}
	if resolvedPort == defaultPortForScheme(scheme) {
		return scheme + "://" + hostLiteral(host), nil
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(resolvedPort)), nil
}

func normalizePublicOrigins(origins []string) ([]string, error) {
	if len(origins) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(origins))
	seen := make(map[string]bool, len(origins))
	for _, origin := range origins {
		if strings.TrimSpace(origin) == "" {
			continue
		}
		norm, err := normalizePublicOrigin(origin)
		if err != nil {
			return nil, err
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		normalized = append(normalized, norm)
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func normalizePublicOrigin(origin string) (string, error) {
	origin = strings.TrimSpace(origin)
	u, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("parsing %q: %w", origin, err)
	}
	if u == nil || u.Host == "" {
		return "", fmt.Errorf("%q must include a host", origin)
	}
	if u.User != nil {
		return "", fmt.Errorf("%q must not include user info", origin)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%q must not include query or fragment", origin)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("%q must not include a path", origin)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%q must use http or https", origin)
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("%q must include a host", origin)
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("%q has an invalid port", origin)
		}
		if n == defaultPortForScheme(scheme) {
			port = ""
		}
	}

	if port == "" {
		return scheme + "://" + hostLiteral(host), nil
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}

func normalizeProxyConfig(cfg *ProxyConfig) error {
	if cfg == nil {
		return nil
	}
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	switch cfg.Mode {
	case "", "caddy":
	default:
		return fmt.Errorf("invalid proxy mode %q", cfg.Mode)
	}
	if cfg.Mode == "caddy" && strings.TrimSpace(cfg.Bin) == "" {
		cfg.Bin = "caddy"
	}
	if cfg.Mode == "caddy" {
		cfg.BindHost = strings.TrimSpace(cfg.BindHost)
		if cfg.BindHost == "" {
			cfg.BindHost = "127.0.0.1"
		}
		if cfg.PublicPort < 0 || cfg.PublicPort > 65535 {
			return fmt.Errorf("invalid public port %d", cfg.PublicPort)
		}
	}
	var err error
	cfg.AllowedSubnets, err = normalizeAllowedSubnets(cfg.AllowedSubnets)
	if err != nil {
		return fmt.Errorf("invalid allowed subnets: %w", err)
	}
	return nil
}

func normalizeAllowedSubnets(subnets []string) ([]string, error) {
	if len(subnets) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(subnets))
	seen := make(map[string]bool, len(subnets))
	for _, subnet := range subnets {
		subnet = strings.TrimSpace(subnet)
		if subnet == "" {
			continue
		}
		network, err := parseAllowedSubnet(subnet)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", subnet, err)
		}
		value := network.String()
		if seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func parseAllowedSubnet(value string) (*net.IPNet, error) {
	_, network, err := net.ParseCIDR(value)
	if err == nil {
		return network, nil
	}
	expanded, ok := expandIPv4CIDRShorthand(value)
	if !ok {
		return nil, err
	}
	_, network, err = net.ParseCIDR(expanded)
	if err != nil {
		return nil, err
	}
	return network, nil
}

func expandIPv4CIDRShorthand(value string) (string, bool) {
	addr, mask, ok := strings.Cut(value, "/")
	if !ok || strings.Contains(addr, ":") {
		return "", false
	}
	parts := strings.Split(addr, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return "", false
	}
	if slices.Contains(parts, "") {
		return "", false
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".") + "/" + mask, true
}

func defaultPortForScheme(scheme string) int {
	if scheme == "https" {
		return 443
	}
	return 80
}

func hostLiteral(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// ResolveDataDir returns the effective data directory by applying
// defaults and environment overrides, without reading any files.
// Use this to determine where migration should target before
// calling Load or LoadMinimal.
func ResolveDataDir() (string, error) {
	cfg, err := Default()
	if err != nil {
		return "", err
	}
	if v := dataDirFromEnv(); v != "" {
		cfg.DataDir = v
	}
	return cfg.DataDir, nil
}

// ResolvePG returns a copy of PG config with defaults applied
// and environment variables expanded in URL.
func (c *Config) ResolvePG() (PGConfig, error) {
	pg := c.PG
	if pg.URL != "" {
		expanded, err := expandBracedEnv(pg.URL)
		if err != nil {
			return pg, fmt.Errorf("expanding url: %w", err)
		}
		pg.URL = expanded
	}
	if pg.Schema == "" {
		pg.Schema = "agentsview"
	}
	if pg.MachineName == "" {
		h, err := os.Hostname()
		if err != nil {
			return pg, fmt.Errorf("os.Hostname failed (%w); set machine_name explicitly in config", err)
		}
		pg.MachineName = h
	}
	return pg, nil
}

// ResolveDuckDB returns a copy of DuckDB config with defaults applied
// and environment variables expanded in path, URL, and token.
func (c *Config) ResolveDuckDB() (DuckDBConfig, error) {
	duck := c.DuckDB
	if duck.Path != "" {
		expanded, err := expandBracedEnv(duck.Path)
		if err != nil {
			return duck, fmt.Errorf("expanding path: %w", err)
		}
		duck.Path = expanded
	}
	if duck.URL != "" {
		expanded, err := expandBracedEnv(duck.URL)
		if err != nil {
			return duck, fmt.Errorf("expanding url: %w", err)
		}
		duck.URL = expanded
	}
	if duck.Token != "" {
		expanded, err := expandBracedEnv(duck.Token)
		if err != nil {
			return duck, fmt.Errorf("expanding token: %w", err)
		}
		duck.Token = expanded
	}
	if duck.Path == "" {
		duck.Path = filepath.Join(c.DataDir, "sessions.duckdb")
	}
	if duck.MachineName == "" {
		h, err := os.Hostname()
		if err != nil {
			return duck, fmt.Errorf("os.Hostname failed (%w); set machine_name explicitly in config", err)
		}
		duck.MachineName = h
	}
	return duck, nil
}

// ResolveLLM returns LLM config with derived embedding defaults applied.
func (c *Config) ResolveLLM() LLMConfig {
	llm := c.LLM
	if llm.Embed.BaseURL == "" {
		llm.Embed.BaseURL = llm.BaseURL
		if llm.Embed.APIKey == "" {
			llm.Embed.APIKey = llm.APIKey
		}
	}
	return llm
}

var (
	bracedEnvPattern      = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	bareEnvPattern        = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)$`)
	partialBareEnvPattern = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
)

// IsEnvDependentURL reports whether s would have environment variables
// expanded by expandBracedEnv: it contains any ${VAR} reference, or the
// whole string is a single bare $VAR shortcut. Embedded bare $VAR
// references (e.g. "postgres://$USER@host") are deliberately NOT expanded
// and so do not count. Callers that must persist a literal URL into a
// context without the shell environment (e.g. a background service) use
// this to reject env-dependent values. It shares the exact patterns
// expandBracedEnv uses so the rejection check cannot drift from the
// expansion semantics.
func IsEnvDependentURL(s string) bool {
	return bracedEnvPattern.MatchString(s) ||
		bareEnvPattern.MatchString(strings.TrimSpace(s))
}

// bareEnvWarned tracks which bare $VAR names have already been warned
// about, so each distinct variable triggers a warning at most once.
var bareEnvWarned sync.Map

// ResetBareEnvWarned clears the warning dedup state. Exported for tests.
func ResetBareEnvWarned() {
	bareEnvWarned.Range(func(k, _ any) bool { bareEnvWarned.Delete(k); return true })
}

// expandBracedEnv expands ${VAR} references in s. As a convenience,
// if the entire string is a single bare $VAR (e.g. "$PGURL"), it is
// expanded as a whole-string shortcut. Bare $VAR references embedded
// in a larger string (e.g. "postgres://$USER@host") are NOT expanded;
// use ${VAR} syntax instead.
func expandBracedEnv(s string) (string, error) {
	if parts := bareEnvPattern.FindStringSubmatch(s); parts != nil {
		val, ok := os.LookupEnv(parts[1])
		if !ok {
			return "", fmt.Errorf("environment variable %s is not set", parts[1])
		}
		return val, nil
	}

	// Warn about bare $VAR references that won't be expanded.
	if remaining := bracedEnvPattern.ReplaceAllString(s, ""); partialBareEnvPattern.MatchString(remaining) {
		for _, m := range partialBareEnvPattern.FindAllStringSubmatch(remaining, -1) {
			if _, set := os.LookupEnv(m[1]); set {
				if _, warned := bareEnvWarned.LoadOrStore(m[1], true); !warned {
					log.Printf("warning: pg.url contains bare $%s which will NOT be expanded; use ${%s} syntax instead", m[1], m[1])
				}
			}
		}
	}

	var missingVars []string
	result := bracedEnvPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := bracedEnvPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(name)
		if !ok {
			missingVars = append(missingVars, name)
			return ""
		}
		return val
	})
	if len(missingVars) > 0 {
		return "", fmt.Errorf("environment variable(s) not set: %s",
			strings.Join(missingVars, ", "))
	}
	return result, nil
}

// SaveTerminalConfig persists terminal settings to the config file.
func (c *Config) SaveTerminalConfig(tc TerminalConfig) error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing, err := c.readConfigMap()
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	existing["terminal"] = tc
	if err := c.writeConfigMap(existing); err != nil {
		return err
	}
	c.Terminal = tc
	return nil
}

// SaveLLMConfig persists LLM settings to the config file.
func (c *Config) SaveLLMConfig(llm LLMConfig) error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing, err := c.readConfigMap()
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	if llm.Providers == nil {
		llm.Providers = c.LLM.Providers
	}
	if llm.Usage == nil {
		llm.Usage = c.LLM.Usage
	}
	if llm.UsageModel == nil {
		llm.UsageModel = c.LLM.UsageModel
	}
	existing["llm"] = llm
	if err := c.writeConfigMap(existing); err != nil {
		return err
	}
	c.LLM = llm
	return nil
}

// SaveLLMProviders persists named provider and usage bindings without touching
// legacy LLM fields. Existing provider API keys survive masked or empty patch
// values so the config API can round-trip redacted responses safely.
func (c *Config) SaveLLMProviders(providers map[string]LLMConfig, usage, usageModel map[string]string) error {
	llm := c.LLM
	// SaveLLMProviders is the authoritative writer for these three maps, so use
	// non-nil empties when cleared: SaveLLMConfig's nil-guard would otherwise
	// restore the stale values (a nil map reads as "field not specified").
	llm.Providers = normalizeLLMProviders(providers)
	if llm.Providers == nil {
		llm.Providers = map[string]LLMConfig{}
	}
	llm.Usage = normalizeLLMUsage(usage)
	if llm.Usage == nil {
		llm.Usage = map[string]string{}
	}
	llm.UsageModel = normalizeLLMUsage(usageModel)
	if llm.UsageModel == nil {
		llm.UsageModel = map[string]string{}
	}
	return c.SaveLLMConfig(llm)
}

// SaveSettings persists a partial settings update to the config file.
// The patch map contains config keys mapped to their new values. Only
// the keys present in patch are written; other config keys are preserved.
func (c *Config) SaveSettings(patch map[string]any) error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing, err := c.readConfigMap()
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	maps.Copy(existing, patch)

	// When require_auth is written, remove the legacy
	// remote_access key so it cannot override on next load.
	if _, ok := patch["require_auth"]; ok {
		delete(existing, "remote_access")
	}

	if err := c.writeConfigMap(existing); err != nil {
		return err
	}

	// Update in-memory config for known keys.
	if v, ok := patch["terminal"]; ok {
		if tc, ok := v.(TerminalConfig); ok {
			c.Terminal = tc
		} else if m, ok := v.(map[string]any); ok {
			if s, ok := m["mode"].(string); ok {
				c.Terminal.Mode = s
			}
			if s, ok := m["custom_bin"].(string); ok {
				c.Terminal.CustomBin = s
			}
			if s, ok := m["custom_args"].(string); ok {
				c.Terminal.CustomArgs = s
			}
		}
	}
	if v, ok := patch["github_token"]; ok {
		if s, ok := v.(string); ok {
			c.GithubToken = s
		}
	}
	if v, ok := patch["auth_token"]; ok {
		if s, ok := v.(string); ok {
			c.AuthToken = s
		}
	}
	if v, ok := patch["require_auth"]; ok {
		if b, ok := v.(bool); ok {
			c.RequireAuth = b
		}
	}
	if v, ok := patch["consolidate_enabled"]; ok {
		if b, ok := v.(bool); ok {
			c.ConsolidateEnabled = b
		}
	}
	if v, ok := patch["consolidate_interval"]; ok {
		if d, ok := v.(time.Duration); ok {
			c.ConsolidateInterval = d
		}
	}
	if v, ok := patch["backup_enabled"]; ok {
		if b, ok := v.(bool); ok {
			c.BackupEnabled = b
		}
	}
	if v, ok := patch["extract_enabled"]; ok {
		if b, ok := v.(bool); ok {
			c.ExtractEnabled = b
		}
	}
	if v, ok := patch["synthesize_enabled"]; ok {
		if b, ok := v.(bool); ok {
			c.SynthesizeEnabled = b
		}
	}
	if v, ok := patch["extract_interval"]; ok {
		if d, ok := v.(time.Duration); ok {
			c.ExtractInterval = d
		}
	}
	if v, ok := patch["memory_backup_repo"]; ok {
		if s, ok := v.(string); ok {
			c.MemoryBackupRepo = s
		}
	}
	if v, ok := patch["memory_backup_linked"]; ok {
		if b, ok := v.(bool); ok {
			c.MemoryBackupLinked = b
		}
	}
	return nil
}

// EnsureAuthToken generates and persists an auth token if one does
// not already exist. Called when require_auth is enabled.
func (c *Config) EnsureAuthToken() error {
	if c.AuthToken != "" {
		return nil
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("generating auth token: %w", err)
	}
	token := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)
	c.AuthToken = token

	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing, err := c.readConfigMap()
	if err != nil {
		return err
	}

	existing["auth_token"] = token
	return c.writeConfigMap(existing)
}

// SaveGithubToken persists the GitHub token to the config file.
func (c *Config) SaveGithubToken(token string) error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing, err := c.readConfigMap()
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	existing["github_token"] = token
	if err := c.writeConfigMap(existing); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	c.GithubToken = token
	return nil
}
