package providers

import (
	"fmt"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/kosong"
)

// Well-known base URLs by provider type.
var wellKnownBaseURLs = map[string]string{
	"kimi":             "https://api.moonshot.cn/v1",
	"openai":           "https://api.openai.com/v1",
	"anthropic":        "https://api.anthropic.com",
	"google-genai":     "https://generativelanguage.googleapis.com/v1beta",
	"openai_responses": "https://api.openai.com/v1",
	"vertexai":         "https://aiplatform.googleapis.com/v1",
}

// defaultModels by provider type.
var defaultModels = map[string]string{
	"kimi":             "kimi-latest",
	"openai":           "gpt-4o",
	"anthropic":        "claude-sonnet-4-20250514",
	"google-genai":     "gemini-2.5-pro",
	"openai_responses": "gpt-4o",
	"vertexai":         "gemini-2.5-pro",
}

// NewFromConfig creates a ChatProvider from the TOML config.
// It resolves the default provider via model→provider lookup (matching the
// TS CLI: default_model → [models].provider → [providers]) and creates
// the appropriate provider adapter.
func NewFromConfig(cfg *config.Config) (kosong.ChatProvider, error) {
	name, prov := cfg.ResolveProvider()
	if name == "" {
		return nil, fmt.Errorf("no provider found in config.toml")
	}

	provType := prov.Type
	if provType == "" {
		provType = name
	}

	baseURL := prov.BaseURL
	if baseURL == "" {
		if known, ok := wellKnownBaseURLs[provType]; ok {
			baseURL = known
		} else {
			return nil, fmt.Errorf("provider %q has no base_url and type %q is not a well-known provider", name, provType)
		}
	}

	// Resolve model: prefer default_model → [models], then provider default
	model := ""
	if _, mc := cfg.ResolveModel(); mc != nil {
		model = mc.Model
	}
	if model == "" {
		model = prov.DefaultModel
	}
	if model == "" {
		if def, ok := defaultModels[provType]; ok {
			model = def
		} else {
			model = "gpt-4o"
		}
	}

	headers := prov.CustomHeaders

	return NewOpenAIProvider(OpenAIProviderConfig{
		Name:           name,
		APIKey:         prov.APIKey,
		BaseURL:        baseURL,
		Model:          model,
		DefaultHeaders: headers,
	}), nil
}

// IsConfigured checks whether the default provider has an API key or OAuth.
func IsConfigured(cfg *config.Config) bool {
	_, prov := cfg.ResolveProvider()
	if prov == nil {
		return false
	}
	return prov.APIKey != "" || prov.OAuth != nil
}
