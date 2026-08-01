package providers

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

// DiscoveredModel represents a model discovered from a provider's API.
type DiscoveredModel struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// ListProviderModels queries a provider's /models endpoint and returns
// the list of available models. Works with OpenAI-compatible APIs.
func ListProviderModels(ctx context.Context, cfg *config.Config) ([]DiscoveredModel, error) {
	name, prov := cfg.ResolveProvider()
	if name == "" || prov == nil {
		return nil, fmt.Errorf("no provider configured")
	}

	baseURL := prov.BaseURL
	if baseURL == "" {
		provType := prov.Type
		if provType == "" {
			provType = name
		}
		if known, ok := wellKnownBaseURLs[provType]; ok {
			baseURL = known
		} else {
			return nil, fmt.Errorf("provider %q has no base_url", name)
		}
	}

	// Ensure base URL ends properly for /models endpoint
	baseURL = strings.TrimRight(baseURL, "/")

	modelsURL := baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set auth header
	if prov.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+prov.APIKey)
	}
	for k, v := range prov.CustomHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []DiscoveredModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Data, nil
}
