// Package oauth provides managed provider provisioning (Gap #66).
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

// ManagedProviderInfo represents a provider from the managed platform.
type ManagedProviderInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	BaseURL     string            `json:"base_url"`
	APIKey      string            `json:"api_key,omitempty"`
	Models      []ManagedModel    `json:"models"`
	Capabilities map[string]bool  `json:"capabilities,omitempty"`
}

// ManagedModel represents a model from the managed platform.
type ManagedModel struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	ContextWindow  int               `json:"context_window"`
	InputCost      float64           `json:"input_cost_per_1m"`
	OutputCost     float64           `json:"output_cost_per_1m"`
	Capabilities   ModelCapabilities `json:"capabilities"`
}

// ModelCapabilities describes what a model can do.
type ModelCapabilities struct {
	Streaming     bool `json:"streaming"`
	FunctionCalls bool `json:"function_calls"`
	Vision        bool `json:"vision"`
	Reasoning     bool `json:"reasoning"`
	Caching       bool `json:"caching"`
}

// ManagedProvisioner fetches providers from the managed platform.
type ManagedProvisioner struct {
	endpoint string
	client   *http.Client
	token    string
}

// NewManagedProvisioner creates a managed provisioner.
func NewManagedProvisioner(endpoint, token string) *ManagedProvisioner {
	return &ManagedProvisioner{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: 30 * time.Second},
		token:    token,
	}
}

// FetchProviders fetches all managed providers.
func (p *ManagedProvisioner) FetchProviders(ctx context.Context) ([]ManagedProviderInfo, error) {
	url := p.endpoint + "/api/providers"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch providers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var providers []ManagedProviderInfo
	if err := json.Unmarshal(body, &providers); err != nil {
		return nil, fmt.Errorf("unmarshal providers: %w", err)
	}
	return providers, nil
}

// FetchModels fetches models for a specific provider.
func (p *ManagedProvisioner) FetchModels(ctx context.Context, providerID string) ([]ManagedModel, error) {
	url := p.endpoint + "/api/providers/" + providerID + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var models []ManagedModel
	if err := json.Unmarshal(body, &models); err != nil {
		return nil, fmt.Errorf("unmarshal models: %w", err)
	}
	return models, nil
}

// ParseCapabilities parses capability flags from a string list.
func ParseCapabilities(caps []string) map[string]bool {
	result := make(map[string]bool)
	for _, c := range caps {
		result[strings.ToLower(c)] = true
	}
	return result
}

// BuildProviderConfig builds a provider configuration from managed data.
func BuildProviderConfig(provider ManagedProviderInfo) map[string]any {
	return map[string]any{
		"id":       provider.ID,
		"name":     provider.Name,
		"base_url": provider.BaseURL,
		"api_key":  provider.APIKey,
		"models":   provider.Models,
	}
}
