// Package oauth provides custom API registry support (Gap #67).
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CustomRegistryEntry represents a provider entry from a custom api.json registry.
type CustomRegistryEntry struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	BaseURL  string            `json:"base_url"`
	APIKey   string            `json:"api_key,omitempty"`
	Models   []CustomModel     `json:"models,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Provider string            `json:"provider,omitempty"` // openai, anthropic, etc.
}

// CustomModel is a model definition from a custom registry.
type CustomModel struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	ContextWindow int     `json:"context_window,omitempty"`
	InputCost     float64 `json:"input_cost_per_1m,omitempty"`
	OutputCost    float64 `json:"output_cost_per_1m,omitempty"`
}

// CustomRegistry fetches and parses custom api.json registries.
type CustomRegistry struct {
	client *http.Client
}

// NewCustomRegistry creates a custom registry client.
func NewCustomRegistry() *CustomRegistry {
	return &CustomRegistry{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchRegistry fetches a custom api.json from a URL.
func (r *CustomRegistry) FetchRegistry(ctx context.Context, url string) ([]CustomRegistryEntry, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return ParseRegistryJSON(body)
}

// ParseRegistryJSON parses a custom api.json payload.
func ParseRegistryJSON(data []byte) ([]CustomRegistryEntry, error) {
	// Try array format first
	var entries []CustomRegistryEntry
	if err := json.Unmarshal(data, &entries); err == nil {
		return entries, nil
	}

	// Try object format with "providers" key
	var wrapper struct {
		Providers []CustomRegistryEntry `json:"providers"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return wrapper.Providers, nil
}

// NormalizeEntry normalizes a registry entry with defaults.
func NormalizeEntry(entry CustomRegistryEntry) CustomRegistryEntry {
	if entry.Provider == "" {
		entry.Provider = "openai" // default
	}
	if !strings.HasSuffix(entry.BaseURL, "/") {
		entry.BaseURL += "/"
	}
	return entry
}

// ValidateEntry validates a registry entry.
func ValidateEntry(entry CustomRegistryEntry) error {
	if entry.ID == "" {
		return fmt.Errorf("entry missing id")
	}
	if entry.BaseURL == "" {
		return fmt.Errorf("entry %s missing base_url", entry.ID)
	}
	return nil
}
