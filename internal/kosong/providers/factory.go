package providers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/kosong/providers/anthropic"
	"github.com/visdomtech/kimi-code/internal/kosong/providers/google"
	"github.com/visdomtech/kimi-code/internal/kosong/providers/kimi"
	"github.com/visdomtech/kimi-code/internal/oauth"
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

// ClientVersion is the version string sent in OAuth device headers.
// Set by the CLI package at startup.
var ClientVersion = "unknown"

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

	// Create the appropriate provider based on type
	var inner kosong.ChatProvider
	switch provType {
	case "kimi":
		inner = kimi.NewProvider(kimi.ProviderConfig{
			APIKey:         prov.APIKey,
			BaseURL:        baseURL,
			Model:          model,
			DefaultHeaders: headers,
		})

	case "anthropic":
		inner = anthropic.NewProvider(anthropic.Config{
			APIKey:         prov.APIKey,
			BaseURL:        baseURL,
			Model:          model,
			DefaultHeaders: headers,
		})

	case "google-genai", "vertexai":
		inner = google.NewProvider(google.Config{
			APIKey:         prov.APIKey,
			BaseURL:        baseURL,
			Model:          model,
			DefaultHeaders: headers,
		})

	case "openai_responses":
		inner = NewOpenAIResponsesProvider(OpenAIResponsesConfig{
			Name:           name,
			APIKey:         prov.APIKey,
			BaseURL:        baseURL,
			Model:          model,
			DefaultHeaders: headers,
		})

	default:
		// Default to OpenAI-compatible provider
		inner = NewOpenAIProvider(OpenAIProviderConfig{
			Name:           name,
			APIKey:         prov.APIKey,
			BaseURL:        baseURL,
			Model:          model,
			DefaultHeaders: headers,
		})
	}

	// Wrap with OAuth if configured
	if prov.OAuth != nil {
		manager, err := createOAuthManager(prov.OAuth)
		if err != nil {
			return nil, fmt.Errorf("create OAuth manager for %q: %w", name, err)
		}
		return NewOAuthProvider(inner, manager), nil
	}

	return inner, nil
}

// createOAuthManager creates an OAuth manager from an OAuthRef config.
func createOAuthManager(ref *config.OAuthRef) (*oauth.Manager, error) {
	storage, err := oauth.NewDefaultTokenStorage()
	if err != nil {
		return nil, err
	}

	// Determine storage name from key (e.g., "oauth/kimi-code" -> "kimi-code")
	storageName := oauth.ProviderName
	if ref.Key != "" {
		// Use the last path component as the storage name
		storageName = filepath.Base(ref.Key)
	}

	oauthHost := oauth.GetOAuthHost()
	if ref.OAuthHost != "" {
		oauthHost = ref.OAuthHost
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(homeDir, config.DataDirName)

	return oauth.NewManager(oauth.ManagerOptions{
		Config: oauth.FlowConfig{
			Name:      storageName,
			OAuthHost: oauthHost,
			ClientID:  oauth.ClientID,
		},
		Storage:   storage,
		Headers:   oauth.CreateDeviceHeaders(ClientVersion, configDir),
		ConfigDir: configDir,
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
