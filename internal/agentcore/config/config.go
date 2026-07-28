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
	DefaultProvider string                      `toml:"default_provider"`
	DefaultModel    string                      `toml:"default_model"`
	Yolo            bool                        `toml:"yolo"`
	PlanMode        bool                        `toml:"plan_mode"`
	Telemetry       bool                        `toml:"telemetry"`
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
	APIKey         string            `toml:"api_key"`
	BaseURL        string            `toml:"base_url"`
	DefaultModel   string            `toml:"default_model"`
	CustomHeaders  map[string]string `toml:"custom_headers"`
	Env            map[string]string `toml:"env"`
	OAuth          *OAuthRef         `toml:"oauth"`
}

// OAuthRef references an OAuth credential store entry.
type OAuthRef struct {
	Storage  string `toml:"storage"`
	Key      string `toml:"key"`
	OAuthHost string `toml:"oauth_host"`
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
	MaxContextSize int      `toml:"max_context_size"`
	MaxInputSize   int      `toml:"max_input_size"`
	MaxOutputSize  int      `toml:"max_output_size"`
	DisplayName    string   `toml:"display_name"`
	Capabilities   []string `toml:"capabilities"`
}

// ThinkingConfig controls extended thinking behavior.
type ThinkingConfig struct {
	Enabled bool   `toml:"enabled"`
	Effort  string `toml:"effort"`
	Keep    string `toml:"keep"`
}

// LoopControlConfig controls agent loop limits.
type LoopControlConfig struct {
	MaxStepsPerTurn     int     `toml:"max_steps_per_turn"`
	MaxRetriesPerStep   int     `toml:"max_retries_per_step"`
	MaxRalphIterations  int     `toml:"max_ralph_iterations"`
	ReservedContextSize int     `toml:"reserved_context_size"`
	CompactionTriggerRatio float64 `toml:"compaction_trigger_ratio"`
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
			MaxStepsPerTurn: 50,
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

// SaveToFile saves config to a TOML file.
func (c *Config) SaveToFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(c)
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
