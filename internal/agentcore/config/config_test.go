package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.Providers == nil {
		t.Error("Providers map should be non-nil")
	}
	if cfg.Models == nil {
		t.Error("Models map should be non-nil")
	}
	if cfg.Experimental == nil {
		t.Error("Experimental map should be non-nil")
	}
	if cfg.LoopControl.MaxStepsPerTurn != 50 {
		t.Errorf("expected MaxStepsPerTurn=50, got %d", cfg.LoopControl.MaxStepsPerTurn)
	}
}

func TestParseEmptyTOML(t *testing.T) {
	cfg, err := parseConfigString("", "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("config should not be nil")
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("expected empty providers, got %d", len(cfg.Providers))
	}
}

func TestParseProviderConfig(t *testing.T) {
	toml := `
default_provider = "kimi"
default_model = "kimi-latest"

[providers.kimi]
type = "kimi"
api_key = "sk-test-key-123"
base_url = "https://api.moonshot.cn/v1"
default_model = "kimi-latest"

[providers.my-openai]
type = "openai"
api_key = "sk-openai-key"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultProvider != "kimi" {
		t.Errorf("expected default_provider=kimi, got %s", cfg.DefaultProvider)
	}
	if cfg.DefaultModel != "kimi-latest" {
		t.Errorf("expected default_model=kimi-latest, got %s", cfg.DefaultModel)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.Providers))
	}

	kimi := cfg.Providers["kimi"]
	if kimi.Type != "kimi" {
		t.Errorf("expected type=kimi, got %s", kimi.Type)
	}
	if kimi.APIKey != "sk-test-key-123" {
		t.Errorf("expected api_key=sk-test-key-123, got %s", kimi.APIKey)
	}
	if kimi.BaseURL != "https://api.moonshot.cn/v1" {
		t.Errorf("expected base_url, got %s", kimi.BaseURL)
	}

	myOpenAI := cfg.Providers["my-openai"]
	if myOpenAI.Type != "openai" {
		t.Errorf("expected type=openai, got %s", myOpenAI.Type)
	}
	if myOpenAI.APIKey != "sk-openai-key" {
		t.Errorf("expected api_key=sk-openai-key, got %s", myOpenAI.APIKey)
	}
}

func TestParseModelConfig(t *testing.T) {
	toml := `
[models.kimi-latest]
provider = "kimi"
model = "kimi-latest"
max_context_size = 131072

[models.gpt4o]
provider = "my-openai"
model = "gpt-4o"
max_context_size = 128000
max_output_size = 16384
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(cfg.Models))
	}

	kimiModel := cfg.Models["kimi-latest"]
	if kimiModel.Provider != "kimi" {
		t.Errorf("expected provider=kimi, got %s", kimiModel.Provider)
	}
	if kimiModel.MaxContextSize != 131072 {
		t.Errorf("expected max_context_size=131072, got %d", kimiModel.MaxContextSize)
	}

	gpt4o := cfg.Models["gpt4o"]
	if gpt4o.MaxOutputSize != 16384 {
		t.Errorf("expected max_output_size=16384, got %d", gpt4o.MaxOutputSize)
	}
}

func TestParseThinkingConfig(t *testing.T) {
	toml := `
[thinking]
enabled = true
effort = "high"
keep = "all"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Thinking.Enabled {
		t.Error("expected thinking.enabled=true")
	}
	if cfg.Thinking.Effort != "high" {
		t.Errorf("expected effort=high, got %s", cfg.Thinking.Effort)
	}
	if cfg.Thinking.Keep != "all" {
		t.Errorf("expected keep=all, got %s", cfg.Thinking.Keep)
	}
}

func TestParseLoopControlConfig(t *testing.T) {
	toml := `
[loop_control]
max_steps_per_turn = 100
max_retries_per_step = 3
compaction_trigger_ratio = 0.8
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LoopControl.MaxStepsPerTurn != 100 {
		t.Errorf("expected max_steps_per_turn=100, got %d", cfg.LoopControl.MaxStepsPerTurn)
	}
	if cfg.LoopControl.MaxRetriesPerStep != 3 {
		t.Errorf("expected max_retries_per_step=3, got %d", cfg.LoopControl.MaxRetriesPerStep)
	}
	if cfg.LoopControl.CompactionTriggerRatio != 0.8 {
		t.Errorf("expected compaction_trigger_ratio=0.8, got %f", cfg.LoopControl.CompactionTriggerRatio)
	}
}

func TestParsePermissionRules(t *testing.T) {
	toml := `
[[permission.rules]]
decision = "allow"
scope = "user"
pattern = "read_file"

[[permission.rules]]
decision = "deny"
scope = "project"
pattern = "bash(rm -rf *)"
reason = "safety"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Permission.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.Permission.Rules))
	}
	if cfg.Permission.Rules[0].Decision != "allow" {
		t.Errorf("expected decision=allow, got %s", cfg.Permission.Rules[0].Decision)
	}
	if cfg.Permission.Rules[1].Reason != "safety" {
		t.Errorf("expected reason=safety, got %s", cfg.Permission.Rules[1].Reason)
	}
}

func TestParseExperimentalFlags(t *testing.T) {
	toml := `
[experimental]
new-feature = true
old-feature = false
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Experimental["new-feature"] {
		t.Error("expected new-feature=true")
	}
	if cfg.Experimental["old-feature"] {
		t.Error("expected old-feature=false")
	}
}

func TestParseFullConfig(t *testing.T) {
	// This is what a real config.toml from the TS CLI looks like
	toml := `
default_provider = "kimi"
default_model = "kimi-k2"
yolo = false
telemetry = true

[providers.kimi]
type = "kimi"
api_key = "sk-abc123"

[providers.custom-gpt]
type = "openai"
api_key = "sk-custom"
base_url = "https://my-proxy.example.com/v1"
default_model = "gpt-4o-mini"

[models.kimi-k2]
provider = "kimi"
model = "kimi-k2"
max_context_size = 131072
display_name = "Kimi K2"

[thinking]
enabled = true
effort = "high"

[loop_control]
max_steps_per_turn = 75

[server]
host = "0.0.0.0"
port = 8080
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultProvider != "kimi" {
		t.Errorf("expected default_provider=kimi, got %s", cfg.DefaultProvider)
	}
	if cfg.Yolo {
		t.Error("expected yolo=false")
	}
	if !cfg.Telemetry {
		t.Error("expected telemetry=true")
	}
	if len(cfg.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(cfg.Providers))
	}
	if cfg.Providers["custom-gpt"].BaseURL != "https://my-proxy.example.com/v1" {
		t.Errorf("unexpected custom base URL: %s", cfg.Providers["custom-gpt"].BaseURL)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port=8080, got %d", cfg.Server.Port)
	}
	if cfg.LoopControl.MaxStepsPerTurn != 75 {
		t.Errorf("expected max_steps=75, got %d", cfg.LoopControl.MaxStepsPerTurn)
	}
}

func TestInvalidTOML(t *testing.T) {
	_, err := parseConfigString("[invalid toml {{{", "test.toml")
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadFromFileNotExist(t *testing.T) {
	cfg, err := LoadFromFile("/tmp/nonexistent_config_test_12345.toml")
	if err != nil {
		t.Fatalf("should not error for missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("should return default config")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `
default_provider = "openai"
[providers.openai]
type = "openai"
api_key = "sk-from-file"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("expected default_provider=openai, got %s", cfg.DefaultProvider)
	}
	if cfg.Providers["openai"].APIKey != "sk-from-file" {
		t.Errorf("expected api_key=sk-from-file, got %s", cfg.Providers["openai"].APIKey)
	}
}

func TestSaveToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.DefaultProvider = "kimi"
	cfg.Providers["kimi"] = ProviderConfig{
		Type:   "kimi",
		APIKey: "sk-save-test",
	}

	if err := cfg.SaveToFile(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if loaded.DefaultProvider != "kimi" {
		t.Errorf("expected default_provider=kimi, got %s", loaded.DefaultProvider)
	}
	if loaded.Providers["kimi"].APIKey != "sk-save-test" {
		t.Errorf("expected api_key=sk-save-test, got %s", loaded.Providers["kimi"].APIKey)
	}
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath("/home/testuser")
	expected := filepath.Join("/home/testuser", DataDirName, "config.toml")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestEnsureDataDir_CreatesAndMigrates(t *testing.T) {
	home := t.TempDir()

	// Create legacy directory with config, tui.toml, input_history, and credentials.
	legacyDir := filepath.Join(home, LegacyDataDirName)
	os.MkdirAll(legacyDir, 0755)
	os.WriteFile(filepath.Join(legacyDir, "config.toml"), []byte("[providers.kimi]\ntype = \"kimi\"\n"), 0644)
	os.WriteFile(filepath.Join(legacyDir, "tui.toml"), []byte("theme = \"dark\"\n"), 0644)
	os.WriteFile(filepath.Join(legacyDir, "input_history"), []byte("cmd1\ncmd2\n"), 0644)
	credsDir := filepath.Join(legacyDir, "credentials")
	os.MkdirAll(credsDir, 0700)
	os.WriteFile(filepath.Join(credsDir, "token1.json"), []byte("{\"access_token\":\"abc\"}"), 0600)

	// EnsureDataDir should create the new directory and copy all data.
	EnsureDataDir(home)

	newDir := filepath.Join(home, DataDirName)

	// Verify config.toml migrated.
	data, err := os.ReadFile(filepath.Join(newDir, "config.toml"))
	if err != nil {
		t.Fatalf("expected config to be migrated: %v", err)
	}
	if !strings.Contains(string(data), "[providers.kimi]") {
		t.Errorf("expected migrated config to contain provider section, got: %s", string(data))
	}

	// Verify tui.toml migrated.
	tuiData, err := os.ReadFile(filepath.Join(newDir, "tui.toml"))
	if err != nil {
		t.Fatalf("expected tui.toml to be migrated: %v", err)
	}
	if !strings.Contains(string(tuiData), "theme") {
		t.Errorf("expected tui.toml to contain theme, got: %s", string(tuiData))
	}

	// Verify input_history migrated.
	histData, err := os.ReadFile(filepath.Join(newDir, "input_history"))
	if err != nil {
		t.Fatalf("expected input_history to be migrated: %v", err)
	}
	if !strings.Contains(string(histData), "cmd1") {
		t.Errorf("expected input_history to contain cmd1, got: %s", string(histData))
	}

	// Verify credentials migrated.
	tokenData, err := os.ReadFile(filepath.Join(newDir, "credentials", "token1.json"))
	if err != nil {
		t.Fatalf("expected credentials to be migrated: %v", err)
	}
	if !strings.Contains(string(tokenData), "access_token") {
		t.Errorf("expected token file to contain access_token, got: %s", string(tokenData))
	}
}

func TestEnsureDataDir_SkipsIfExists(t *testing.T) {
	home := t.TempDir()

	// Create the new directory already.
	newDir := filepath.Join(home, DataDirName)
	os.MkdirAll(newDir, 0755)
	os.WriteFile(filepath.Join(newDir, "config.toml"), []byte("existing"), 0644)

	// Also create legacy with different content.
	legacyDir := filepath.Join(home, LegacyDataDirName)
	os.MkdirAll(legacyDir, 0755)
	os.WriteFile(filepath.Join(legacyDir, "config.toml"), []byte("legacy"), 0644)

	EnsureDataDir(home)

	// Should not overwrite existing config.
	data, _ := os.ReadFile(filepath.Join(newDir, "config.toml"))
	if string(data) != "existing" {
		t.Errorf("expected existing config to be preserved, got: %s", string(data))
	}
}

func TestParseOAuthProvider(t *testing.T) {
	toml := `
[providers.kimi]
type = "kimi"

[providers.kimi.oauth]
storage = "file"
key = "managed:kimi-code"
oauth_host = "https://auth.example.com"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prov := cfg.Providers["kimi"]
	if prov.OAuth == nil {
		t.Fatal("expected OAuth ref to be set")
	}
	if prov.OAuth.Storage != "file" {
		t.Errorf("expected storage=file, got %s", prov.OAuth.Storage)
	}
	if prov.OAuth.Key != "managed:kimi-code" {
		t.Errorf("expected key=managed:kimi-code, got %s", prov.OAuth.Key)
	}
}

func TestParseCustomHeaders(t *testing.T) {
	toml := `
[providers.custom]
type = "openai"
api_key = "sk-test"

[providers.custom.custom_headers]
X-Custom-Header = "value1"
X-Request-Id = "value2"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prov := cfg.Providers["custom"]
	if len(prov.CustomHeaders) != 2 {
		t.Fatalf("expected 2 custom headers, got %d", len(prov.CustomHeaders))
	}
	if prov.CustomHeaders["X-Custom-Header"] != "value1" {
		t.Errorf("expected X-Custom-Header=value1, got %s", prov.CustomHeaders["X-Custom-Header"])
	}
}

func TestParseRawPreserved(t *testing.T) {
	toml := `
default_provider = "kimi"
[providers.kimi]
type = "kimi"
api_key = "sk-test"

[unknown_future_section]
some_field = "value"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Raw == nil {
		t.Fatal("Raw should be set")
	}
	if _, ok := cfg.Raw["unknown_future_section"]; !ok {
		t.Error("Raw should preserve unknown sections")
	}
}

func TestResolveProviderFromModel(t *testing.T) {
	// Config like the user's real config.toml:
	// default_model = "kimi-code/k3-256k" (no default_provider)
	toml := `
default_model = "kimi-code/k3-256k"

[providers."managed:kimi-code"]
type = "kimi"
api_key = ""
base_url = "https://api.kimi.com/coding/v1"

[providers."managed:kimi-code".oauth]
storage = "file"
key = "oauth/kimi-code"

[providers.deepseek]
type = "openai"
api_key = "sk-test"
base_url = "https://api.deepseek.com"

[models."kimi-code/k3-256k"]
provider = "managed:kimi-code"
model = "k3-256k"
max_context_size = 262144
display_name = "k3-256k"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ResolveProviderName should follow default_model → models → providers
	name := cfg.ResolveProviderName()
	if name != "managed:kimi-code" {
		t.Errorf("expected managed:kimi-code, got %s", name)
	}

	provName, prov := cfg.ResolveProvider()
	if prov == nil {
		t.Fatal("expected provider to be resolved")
	}
	if provName != "managed:kimi-code" {
		t.Errorf("expected managed:kimi-code, got %s", provName)
	}
	if prov.Type != "kimi" {
		t.Errorf("expected type=kimi, got %s", prov.Type)
	}

	// ResolveModel should find the model entry
	modelName, mc := cfg.ResolveModel()
	if modelName != "kimi-code/k3-256k" {
		t.Errorf("expected kimi-code/k3-256k, got %s", modelName)
	}
	if mc == nil {
		t.Fatal("expected model config to be resolved")
	}
	if mc.Model != "k3-256k" {
		t.Errorf("expected model=k3-256k, got %s", mc.Model)
	}
	if mc.DisplayName != "k3-256k" {
		t.Errorf("expected display_name=k3-256k, got %s", mc.DisplayName)
	}
}

func TestResolveProviderExplicitDefault(t *testing.T) {
	// Config with explicit default_provider (no default_model)
	toml := `
default_provider = "deepseek"

[providers.deepseek]
type = "openai"
api_key = "sk-test"

[providers.other]
type = "kimi"
api_key = "sk-other"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	name := cfg.ResolveProviderName()
	if name != "deepseek" {
		t.Errorf("expected deepseek, got %s", name)
	}
}

func TestResolveProviderFallback(t *testing.T) {
	// Config with neither default_model nor default_provider
	toml := `
[providers.only-one]
type = "openai"
api_key = "sk-test"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	name := cfg.ResolveProviderName()
	if name != "only-one" {
		t.Errorf("expected only-one (fallback), got %s", name)
	}
}

func TestResolveProviderEmpty(t *testing.T) {
	cfg := DefaultConfig()
	name := cfg.ResolveProviderName()
	if name != "" {
		t.Errorf("expected empty, got %s", name)
	}
	_, prov := cfg.ResolveProvider()
	if prov != nil {
		t.Error("expected nil provider")
	}
}

func TestResolveProviderOAuthCountsAsConfigured(t *testing.T) {
	// Provider with OAuth but no api_key should still be "configured"
	toml := `
default_model = "kimi-code/k3"

[providers."managed:kimi-code"]
type = "kimi"
api_key = ""
base_url = "https://api.kimi.com/coding/v1"

[providers."managed:kimi-code".oauth]
storage = "file"
key = "oauth/kimi-code"

[models."kimi-code/k3"]
provider = "managed:kimi-code"
model = "k3"
max_context_size = 1048576
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, prov := cfg.ResolveProvider()
	if prov == nil {
		t.Fatal("expected provider to be resolved")
	}
	if prov.APIKey != "" {
		t.Error("expected empty api_key")
	}
	if prov.OAuth == nil {
		t.Fatal("expected OAuth to be set")
	}
	// OAuth-only should count as configured
	if prov.APIKey == "" && prov.OAuth == nil {
		t.Error("provider with OAuth should be considered configured")
	}
}

func TestSaveToFile_PreservesUnknownSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	input := `default_model = "kimi-code/k3"

[providers."managed:kimi-code"]
type = "kimi"
base_url = "https://api.kimi.com/coding/v1"

[providers."managed:kimi-code".oauth]
storage = "file"
key = "oauth/kimi-code"

[models."kimi-code/k3"]
provider = "managed:kimi-code"
model = "k3"
max_context_size = 1048576
support_efforts = ["low", "high", "max"]
default_effort = "high"

[thinking]
enabled = true
effort = "high"

[services.moonshot_search]
base_url = "https://api.kimi.com/coding/v1/search"

[services.moonshot_fetch]
base_url = "https://api.kimi.com/coding/v1/fetch"
`
	cfg, err := parseConfigString(input, "test.toml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cfg.SaveToFile(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(data)

	// [services] sections must be preserved
	if !strings.Contains(text, "moonshot_search") {
		t.Error("services.moonshot_search lost during save")
	}
	if !strings.Contains(text, "moonshot_fetch") {
		t.Error("services.moonshot_fetch lost during save")
	}

	// support_efforts and default_effort must survive round-trip
	if !strings.Contains(text, "support_efforts") {
		t.Error("support_efforts lost during save")
	}
	if !strings.Contains(text, "default_effort") {
		t.Error("default_effort lost during save")
	}

	// Zero-value noise must NOT be present
	if strings.Contains(text, "yolo = false") {
		t.Error("yolo = false should be omitted")
	}
	if strings.Contains(text, "plan_mode = false") {
		t.Error("plan_mode = false should be omitted")
	}
	if strings.Contains(text, "max_input_size = 0") {
		t.Error("max_input_size = 0 should be omitted")
	}
	if strings.Contains(text, "default_provider =") && !strings.Contains(input, "default_provider") {
		t.Error("default_provider should not appear when empty")
	}
}

func TestSaveToFile_OmitsZeroValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.DefaultModel = "test-model"
	cfg.Providers["test"] = ProviderConfig{
		Type:   "openai",
		APIKey: "sk-test",
	}
	cfg.Models["test-model"] = ModelConfig{
		Provider:       "test",
		Model:          "test-model",
		MaxContextSize: 8192,
	}

	if err := cfg.SaveToFile(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(data)

	// These zero-value fields must not appear
	for _, bad := range []string{"yolo = false", "plan_mode = false", "telemetry = false", "max_input_size = 0", "max_output_size = 0"} {
		if strings.Contains(text, bad) {
			t.Errorf("zero-value field should be omitted: %s", bad)
		}
	}

	// Empty sections must not appear
	for _, bad := range []string{"[permission]", "[experimental]"} {
		if strings.Contains(text, bad) {
			t.Errorf("empty section should be omitted: %s", bad)
		}
	}
}

func TestParseMcpServerBearerTokenEnv(t *testing.T) {
	toml := `
[mcp_servers.myserver]
transport = "http"
url = "https://mcp.example.com"
bearer_token_env = "MY_SECRET_TOKEN"

[mcp_servers.legacy]
transport = "sse"
url = "https://mcp2.example.com"

[mcp_servers.legacy.env]
BEARER_TOKEN_ENV = "LEGACY_TOKEN"
`
	cfg, err := parseConfigString(toml, "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Explicit bearer_token_env field
	srv := cfg.McpServers["myserver"]
	if srv.BearerTokenEnv != "MY_SECRET_TOKEN" {
		t.Errorf("expected bearer_token_env=MY_SECRET_TOKEN, got %s", srv.BearerTokenEnv)
	}
	if srv.Transport != "http" {
		t.Errorf("expected transport=http, got %s", srv.Transport)
	}

	// Legacy env map approach
	legacy := cfg.McpServers["legacy"]
	if legacy.BearerTokenEnv != "" {
		t.Errorf("expected empty bearer_token_env for legacy server, got %s", legacy.BearerTokenEnv)
	}
	if legacy.Env["BEARER_TOKEN_ENV"] != "LEGACY_TOKEN" {
		t.Errorf("expected legacy env BEARER_TOKEN_ENV=LEGACY_TOKEN, got %s", legacy.Env["BEARER_TOKEN_ENV"])
	}
}

func TestRoundTrip_SupportEfforts(t *testing.T) {
	input := `[models.k3]
provider = "kimi"
model = "k3"
max_context_size = 1048576
support_efforts = ["low", "high", "max"]
default_effort = "high"
`
	cfg, err := parseConfigString(input, "test.toml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := cfg.Models["k3"]
	if len(m.SupportEfforts) != 3 {
		t.Errorf("expected 3 support_efforts, got %d", len(m.SupportEfforts))
	}
	if m.DefaultEffort != "high" {
		t.Errorf("expected default_effort=high, got %s", m.DefaultEffort)
	}

	// Save and reload
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := cfg.SaveToFile(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	cfg2, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	m2 := cfg2.Models["k3"]
	if len(m2.SupportEfforts) != 3 {
		t.Errorf("after reload: expected 3 support_efforts, got %d", len(m2.SupportEfforts))
	}
	if m2.DefaultEffort != "high" {
		t.Errorf("after reload: expected default_effort=high, got %s", m2.DefaultEffort)
	}
}
