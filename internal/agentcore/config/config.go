package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the application configuration, compatible with the TS CLI's
// ~/.kimi-code/config.toml format. Fields use TOML tags matching the
// snake_case keys used by the TS core's transformTomlData pipeline.
type Config struct {
	DefaultProvider string                      `toml:"default_provider,omitempty"`
	DefaultModel    string                      `toml:"default_model,omitempty"`
	Yolo            bool                        `toml:"yolo,omitempty"`
	PlanMode        bool                        `toml:"plan_mode,omitempty"`
	Telemetry       bool                        `toml:"telemetry,omitempty"`
	Providers       map[string]ProviderConfig    `toml:"providers"`
	Models          map[string]ModelConfig       `toml:"models"`
	Thinking        ThinkingConfig               `toml:"thinking"`
	LoopControl     LoopControlConfig            `toml:"loop_control"`
	Permission      PermissionConfig             `toml:"permission"`
	Experimental    map[string]bool              `toml:"experimental"`
	Server          ServerConfig                 `toml:"server"`

	// Raw holds the original parsed TOML data for forward-compatible
	// round-tripping of sections the Go CLI does not yet consume.
	Raw map[string]any `toml:"-"`
}

// ProviderConfig is a named provider entry, matching TS schema:
//
//	[providers.kimi]
//	type = "kimi"
//	api_key = "sk-..."
//	base_url = "https://..."
//	default_model = "kimi-latest"
type ProviderConfig struct {
	Type           string            `toml:"type"`
	APIKey         string            `toml:"api_key,omitempty"`
	BaseURL        string            `toml:"base_url,omitempty"`
	DefaultModel   string            `toml:"default_model,omitempty"`
	CustomHeaders  map[string]string `toml:"custom_headers,omitempty"`
	Env            map[string]string `toml:"env,omitempty"`
	OAuth          *OAuthRef         `toml:"oauth,omitempty"`
}

// OAuthRef references an OAuth credential store entry.
type OAuthRef struct {
	Storage   string `toml:"storage"`
	Key       string `toml:"key"`
	OAuthHost string `toml:"oauth_host,omitempty"`
}

// ModelConfig is a named model alias, matching TS schema:
//
//	[models.kimi-latest]
//	provider = "kimi"
//	model = "kimi-latest"
//	max_context_size = 131072
type ModelConfig struct {
	Provider       string   `toml:"provider"`
	Model          string   `toml:"model"`
	MaxContextSize int      `toml:"max_context_size,omitempty"`
	MaxInputSize   int      `toml:"max_input_size,omitempty"`
	MaxOutputSize  int      `toml:"max_output_size,omitempty"`
	DisplayName    string   `toml:"display_name,omitempty"`
	Capabilities   []string `toml:"capabilities,omitempty"`
	SupportEfforts []string `toml:"support_efforts,omitempty"`
	DefaultEffort  string   `toml:"default_effort,omitempty"`
}

// ThinkingConfig controls extended thinking behavior.
type ThinkingConfig struct {
	Enabled bool   `toml:"enabled,omitempty"`
	Effort  string `toml:"effort,omitempty"`
	Keep    string `toml:"keep,omitempty"`
}

// LoopControlConfig controls agent loop limits.
type LoopControlConfig struct {
	MaxStepsPerTurn        int     `toml:"max_steps_per_turn,omitempty"`
	MaxRetriesPerStep      int     `toml:"max_retries_per_step,omitempty"`
	MaxRalphIterations     int     `toml:"max_ralph_iterations,omitempty"`
	ReservedContextSize    int     `toml:"reserved_context_size,omitempty"`
	CompactionTriggerRatio float64 `toml:"compaction_trigger_ratio,omitempty"`
}

// PermissionConfig holds permission rules.
type PermissionConfig struct {
	Rules []PermissionRule `toml:"rules"`
}

// PermissionRule is a single allow/deny/ask rule.
type PermissionRule struct {
	Decision string `toml:"decision"`
	Scope    string `toml:"scope"`
	Pattern  string `toml:"pattern"`
	Reason   string `toml:"reason"`
}

// ServerConfig holds the kap-server settings (Go CLI local-only, not in TS TOML).
type ServerConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Providers: make(map[string]ProviderConfig),
		Models:    make(map[string]ModelConfig),
		LoopControl: LoopControlConfig{
			MaxStepsPerTurn:        50,
			CompactionTriggerRatio: 0.85,
			ReservedContextSize:    50000,
		},
		Experimental: make(map[string]bool),
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 9876,
		},
	}
}

// LoadFromFile loads config from a TOML file (config.toml).
// Returns default config if the file does not exist.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	return parseConfigString(string(data), path)
}

// parseConfigString parses a TOML config string into Config.
func parseConfigString(text string, filePath string) (*Config, error) {
	cfg := DefaultConfig()
	if text == "" || len(trimSpace(text)) == 0 {
		return cfg, nil
	}

	// First decode into raw map to preserve unknown fields
	var raw map[string]any
	if _, err := toml.Decode(text, &raw); err != nil {
		return nil, fmt.Errorf("invalid TOML in %s: %w", filePath, err)
	}
	cfg.Raw = raw

	// Now decode into typed struct (extra fields are silently ignored)
	if _, err := toml.Decode(text, cfg); err != nil {
		return nil, fmt.Errorf("invalid TOML in %s: %w", filePath, err)
	}

	// Ensure maps are non-nil
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
	if cfg.Models == nil {
		cfg.Models = make(map[string]ModelConfig)
	}
	if cfg.Experimental == nil {
		cfg.Experimental = make(map[string]bool)
	}

	return cfg, nil
}

// SaveToFile saves config to a TOML file, preserving unknown sections
// (e.g. [services]) from the original Raw data by merging typed fields
// on top of the raw map before encoding.
func (c *Config) SaveToFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Start with the raw TOML data (preserves unknown sections like [services]).
	raw := make(map[string]any)
	for k, v := range c.Raw {
		raw[k] = v
	}

	// Overlay typed fields (non-zero values take precedence over raw).
	c.overlayTyped(raw)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(raw)
}

// overlayTyped writes the typed Config fields into raw, only setting
// non-zero values so that we don't overwrite preserved raw data with
// Go zero values.
//
// Limitation: Zero-value typed fields do not override Raw. If a field
// was set in the loaded config (e.g. yolo = true) and then cleared via
// the typed API (cfg.Yolo = false), SaveToFile will still emit the
// original value because overlayTyped skips zero-value fields. Clearing
// previously-set values through the typed API is not supported; callers
// that need this must delete the key from c.Raw directly before saving.
func (c *Config) overlayTyped(raw map[string]any) {
	if c.DefaultProvider != "" {
		raw["default_provider"] = c.DefaultProvider
	}
	if c.DefaultModel != "" {
		raw["default_model"] = c.DefaultModel
	}
	if c.Yolo {
		raw["yolo"] = c.Yolo
	}
	if c.PlanMode {
		raw["plan_mode"] = c.PlanMode
	}
	if c.Telemetry {
		raw["telemetry"] = c.Telemetry
	}
	if len(c.Providers) > 0 {
		raw["providers"] = providersToMap(c.Providers)
	}
	if len(c.Models) > 0 {
		raw["models"] = modelsToMap(c.Models)
	}
	if c.Thinking.Enabled || c.Thinking.Effort != "" || c.Thinking.Keep != "" {
		raw["thinking"] = thinkingToMap(c.Thinking)
	}
	if !isLoopControlZero(c.LoopControl) {
		raw["loop_control"] = loopControlToMap(c.LoopControl)
	}
	if len(c.Permission.Rules) > 0 {
		raw["permission"] = permissionToMap(c.Permission)
	}
	if len(c.Experimental) > 0 {
		raw["experimental"] = c.Experimental
	}
	if c.Server.Host != "" || c.Server.Port != 0 {
		raw["server"] = serverToMap(c.Server)
	}
}

func providersToMap(ps map[string]ProviderConfig) map[string]any {
	out := make(map[string]any, len(ps))
	for k, p := range ps {
		m := map[string]any{"type": p.Type}
		if p.APIKey != "" {
			m["api_key"] = p.APIKey
		}
		if p.BaseURL != "" {
			m["base_url"] = p.BaseURL
		}
		if p.DefaultModel != "" {
			m["default_model"] = p.DefaultModel
		}
		if len(p.CustomHeaders) > 0 {
			m["custom_headers"] = p.CustomHeaders
		}
		if len(p.Env) > 0 {
			m["env"] = p.Env
		}
		if p.OAuth != nil {
			o := map[string]any{"storage": p.OAuth.Storage, "key": p.OAuth.Key}
			if p.OAuth.OAuthHost != "" {
				o["oauth_host"] = p.OAuth.OAuthHost
			}
			m["oauth"] = o
		}
		out[k] = m
	}
	return out
}

func modelsToMap(ms map[string]ModelConfig) map[string]any {
	out := make(map[string]any, len(ms))
	for k, mc := range ms {
		m := map[string]any{
			"provider": mc.Provider,
			"model":    mc.Model,
		}
		if mc.MaxContextSize != 0 {
			m["max_context_size"] = mc.MaxContextSize
		}
		if mc.MaxInputSize != 0 {
			m["max_input_size"] = mc.MaxInputSize
		}
		if mc.MaxOutputSize != 0 {
			m["max_output_size"] = mc.MaxOutputSize
		}
		if mc.DisplayName != "" {
			m["display_name"] = mc.DisplayName
		}
		if len(mc.Capabilities) > 0 {
			m["capabilities"] = mc.Capabilities
		}
		if len(mc.SupportEfforts) > 0 {
			m["support_efforts"] = mc.SupportEfforts
		}
		if mc.DefaultEffort != "" {
			m["default_effort"] = mc.DefaultEffort
		}
		out[k] = m
	}
	return out
}

func thinkingToMap(t ThinkingConfig) map[string]any {
	m := make(map[string]any)
	if t.Enabled {
		m["enabled"] = t.Enabled
	}
	if t.Effort != "" {
		m["effort"] = t.Effort
	}
	if t.Keep != "" {
		m["keep"] = t.Keep
	}
	return m
}

func isLoopControlZero(lc LoopControlConfig) bool {
	return lc.MaxStepsPerTurn == 0 && lc.MaxRetriesPerStep == 0 &&
		lc.MaxRalphIterations == 0 && lc.ReservedContextSize == 0 &&
		lc.CompactionTriggerRatio == 0
}

func loopControlToMap(lc LoopControlConfig) map[string]any {
	m := make(map[string]any)
	if lc.MaxStepsPerTurn != 0 {
		m["max_steps_per_turn"] = lc.MaxStepsPerTurn
	}
	if lc.MaxRetriesPerStep != 0 {
		m["max_retries_per_step"] = lc.MaxRetriesPerStep
	}
	if lc.MaxRalphIterations != 0 {
		m["max_ralph_iterations"] = lc.MaxRalphIterations
	}
	if lc.ReservedContextSize != 0 {
		m["reserved_context_size"] = lc.ReservedContextSize
	}
	if lc.CompactionTriggerRatio != 0 {
		m["compaction_trigger_ratio"] = lc.CompactionTriggerRatio
	}
	return m
}

func permissionToMap(p PermissionConfig) map[string]any {
	m := make(map[string]any)
	if len(p.Rules) > 0 {
		rules := make([]any, len(p.Rules))
		for i, r := range p.Rules {
			rm := map[string]any{}
			if r.Decision != "" {
				rm["decision"] = r.Decision
			}
			if r.Scope != "" {
				rm["scope"] = r.Scope
			}
			if r.Pattern != "" {
				rm["pattern"] = r.Pattern
			}
			if r.Reason != "" {
				rm["reason"] = r.Reason
			}
			rules[i] = rm
		}
		m["rules"] = rules
	}
	return m
}

func serverToMap(s ServerConfig) map[string]any {
	m := make(map[string]any)
	if s.Host != "" {
		m["host"] = s.Host
	}
	if s.Port != 0 {
		m["port"] = s.Port
	}
	return m
}

// ConfigPath returns the default config.toml path under the given home dir.
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code", "config.toml")
}

func trimSpace(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
			return s[i:]
		}
	}
	return ""
}

// ResolveModel returns the resolved default model name and its config entry.
// Resolution order:
//  1. default_model looked up in [models]
//  2. first model entry whose provider matches the default provider
//  3. empty if nothing matches
func (c *Config) ResolveModel() (string, *ModelConfig) {
	// 1. Direct model lookup by name
	if c.DefaultModel != "" {
		if m, ok := c.Models[c.DefaultModel]; ok {
			return c.DefaultModel, &m
		}
	}
	// 2. First model whose provider matches default provider
	prov := c.ResolveProviderName()
	for name, m := range c.Models {
		if m.Provider == prov {
			return name, &m
		}
	}
	return "", nil
}

// ResolveProviderName returns the resolved default provider name.
// Resolution order:
//  1. default_model → [models.<name>].provider → look up in [providers]
//  2. default_provider
//  3. first available provider in the map
func (c *Config) ResolveProviderName() string {
	// 1. Model-driven resolution
	if c.DefaultModel != "" {
		if m, ok := c.Models[c.DefaultModel]; ok {
			if _, ok := c.Providers[m.Provider]; ok {
				return m.Provider
			}
		}
	}
	// 2. Explicit default_provider
	if c.DefaultProvider != "" {
		if _, ok := c.Providers[c.DefaultProvider]; ok {
			return c.DefaultProvider
		}
	}
	// 3. First available provider
	for name := range c.Providers {
		return name
	}
	return ""
}

// ResolveProvider returns the resolved default provider config entry.
func (c *Config) ResolveProvider() (string, *ProviderConfig) {
	name := c.ResolveProviderName()
	if name == "" {
		return "", nil
	}
	prov := c.Providers[name]
	return name, &prov
}
