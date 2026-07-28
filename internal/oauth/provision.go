package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
)

// ModelInfo represents a model returned by the managed Kimi Code API.
type ModelInfo struct {
	ID                   string
	ContextLength        int
	SupportsReasoning    bool
	SupportsThinkingType string // "only", "no", "both", or ""
	SupportsImageIn      bool
	SupportsVideoIn      bool
	SupportsToolUse      bool
	DisplayName          string
	Protocol             string // "kimi" or "anthropic"
	SupportEfforts       []string
	DefaultEffort        string
}

// modelsHTTPClient is a dedicated HTTP client for model fetching.
var modelsHTTPClient = &http.Client{Timeout: 30 * time.Second}

// FetchManagedModels fetches available models from the Kimi Code API.
func FetchManagedModels(ctx context.Context, accessToken, baseURL string, headers map[string]string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := modelsHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, &OAuthError{
				Message: fmt.Sprintf("models endpoint rejected (HTTP %d)", resp.StatusCode),
				Cause:   ErrUnauthorized,
			}
		}
		return nil, fmt.Errorf("fetch models failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	var models []ModelInfo
	for _, item := range payload.Data {
		id, ok := item["id"].(string)
		if !ok || id == "" {
			continue
		}
		ctxLen := 0
		if v, ok := item["context_length"].(float64); ok {
			ctxLen = int(v)
		}
		if ctxLen <= 0 {
			continue
		}

		supportsToolUse := true
		if v, ok := item["supports_tool_use"].(bool); ok {
			supportsToolUse = v
		}

		displayName, _ := item["display_name"].(string)
		protocol, _ := item["protocol"].(string)
		if protocol != "anthropic" {
			protocol = ""
		}

		thinkingType := ""
		if v, ok := item["supports_thinking_type"].(string); ok {
			if v == "only" || v == "no" || v == "both" {
				thinkingType = v
			}
		}

		// Parse think_efforts: { support: bool, valid_efforts: [...], default_effort: "..." }
		var supportEfforts []string
		var defaultEffort string
		if te, ok := item["think_efforts"].(map[string]interface{}); ok {
			if support, _ := te["support"].(bool); support {
				if efforts, ok := te["valid_efforts"].([]interface{}); ok {
					for _, e := range efforts {
						if s, ok := e.(string); ok && s != "" {
							supportEfforts = append(supportEfforts, s)
						}
					}
				}
				if def, ok := te["default_effort"].(string); ok && def != "" {
					defaultEffort = def
				}
			}
		}

		models = append(models, ModelInfo{
			ID:                   id,
			ContextLength:        ctxLen,
			SupportsReasoning:    boolVal(item["supports_reasoning"]),
			SupportsThinkingType: thinkingType,
			SupportsImageIn:      boolVal(item["supports_image_in"]),
			SupportsVideoIn:      boolVal(item["supports_video_in"]),
			SupportsToolUse:      supportsToolUse,
			DisplayName:          displayName,
			Protocol:             protocol,
			SupportEfforts:       supportEfforts,
			DefaultEffort:        defaultEffort,
		})
	}

	return models, nil
}

func boolVal(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

// modelCapabilities returns capability tags for a model.
func modelCapabilities(m ModelInfo) []string {
	var caps []string
	switch m.SupportsThinkingType {
	case "only":
		caps = append(caps, "thinking", "always_thinking")
	case "both":
		caps = append(caps, "thinking")
	case "no":
		// no thinking caps
	default:
		if m.SupportsReasoning {
			caps = append(caps, "thinking")
		}
	}
	if m.SupportsImageIn {
		caps = append(caps, "image_in")
	}
	if m.SupportsVideoIn {
		caps = append(caps, "video_in")
	}
	if m.SupportsToolUse {
		caps = append(caps, "tool_use")
	}
	return caps
}

// ProvisionConfig provisions the managed:kimi-code provider in the config.
func ProvisionConfig(cfg *config.Config, models []ModelInfo, baseURL, oauthHost string) error {
	if len(models) == 0 {
		return fmt.Errorf("no models available for Kimi Code")
	}

	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}
	if cfg.Models == nil {
		cfg.Models = make(map[string]config.ModelConfig)
	}

	// Build OAuth ref
	oauthRef := &config.OAuthRef{
		Storage: "file",
		Key:     OAuthKey,
	}
	if oauthHost != "" && oauthHost != DefaultOAuthHost {
		oauthRef.OAuthHost = oauthHost
	}

	// Set the managed provider provider
	cfg.Providers[ManagedProvider] = config.ProviderConfig{
		Type:     "kimi",
		BaseURL:  baseURL,
		APIKey:   "",
		OAuth:    oauthRef,
	}

	// Remove stale managed models (check both old and new key prefix formats)
	upstreamKeys := make(map[string]bool)
	for _, m := range models {
		key := ModelKeyPrefix + "/" + m.ID
		upstreamKeys[key] = true
	}
	for key, mc := range cfg.Models {
		if mc.Provider == ManagedProvider && !upstreamKeys[key] {
			delete(cfg.Models, key)
		}
	}

	// Add/update managed models using TS-compatible key prefix (kimi-code/X, not managed:kimi-code/X)
	for _, m := range models {
		key := ModelKeyPrefix + "/" + m.ID
		caps := modelCapabilities(m)
		mc := config.ModelConfig{
			Provider:       ManagedProvider,
			Model:          m.ID,
			MaxContextSize: m.ContextLength,
			DisplayName:    m.DisplayName,
			Capabilities:   caps,
		}
		if len(m.SupportEfforts) > 0 {
			mc.SupportEfforts = m.SupportEfforts
		}
		if m.DefaultEffort != "" {
			mc.DefaultEffort = m.DefaultEffort
		}
		cfg.Models[key] = mc
	}

	// Select default model: first model, prefer thinking-capable
	selectedKey := ModelKeyPrefix + "/" + models[0].ID
	thinkingEnabled := false
	for _, m := range models {
		key := ModelKeyPrefix + "/" + m.ID
		caps := modelCapabilities(m)
		for _, c := range caps {
			if c == "thinking" || c == "always_thinking" {
				selectedKey = key
				thinkingEnabled = true
				break
			}
		}
		if thinkingEnabled {
			break
		}
	}
	_ = thinkingEnabled

	// Only set defaults if they're currently empty or already point to the managed provider
	if cfg.DefaultModel == "" || isManagedModel(cfg.DefaultModel) {
		cfg.DefaultModel = selectedKey
	}
	if cfg.DefaultProvider == "" || cfg.DefaultProvider == ManagedProvider {
		cfg.DefaultProvider = ManagedProvider
	}

	return nil
}

// isManagedModel checks if a model key belongs to the managed provider.
// Accepts both new format (kimi-code/X) and old format (managed:kimi-code/X).
func isManagedModel(modelKey string) bool {
	return strings.HasPrefix(modelKey, ModelKeyPrefix+"/") ||
		strings.HasPrefix(modelKey, ManagedProvider+"/")
}

// ClearManagedConfig removes the managed:kimi-code provider and its models.
func ClearManagedConfig(cfg *config.Config) {
	delete(cfg.Providers, ManagedProvider)

	removedDefault := false
	for key, mc := range cfg.Models {
		if mc.Provider == ManagedProvider {
			if cfg.DefaultModel == key {
				removedDefault = true
			}
			delete(cfg.Models, key)
		}
	}

	if removedDefault {
		cfg.DefaultModel = ""
	}

	if cfg.DefaultProvider == ManagedProvider {
		cfg.DefaultProvider = ""
	}
}
