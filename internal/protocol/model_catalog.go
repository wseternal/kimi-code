// Package protocol provides model catalog wire types (Gap #76).
package protocol

// ModelCatalogItem represents a model in the catalog.
type ModelCatalogItem struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	ProviderID     string            `json:"provider_id"`
	ContextWindow  int               `json:"context_window"`
	InputCost      float64           `json:"input_cost_per_1m,omitempty"`
	OutputCost     float64           `json:"output_cost_per_1m,omitempty"`
	Capabilities   []string          `json:"capabilities,omitempty"`
	IsDefault      bool              `json:"is_default"`
	IsDeprecated   bool              `json:"is_deprecated"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ProviderCatalogItem represents a provider in the catalog.
type ProviderCatalogItem struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	BaseURL  string             `json:"base_url,omitempty"`
	Models   []ModelCatalogItem `json:"models,omitempty"`
	AuthType string             `json:"auth_type,omitempty"` // "api_key", "oauth", "none"
	IsCustom bool               `json:"is_custom"`
}

// ProviderRefreshChange represents a change detected during provider refresh.
type ProviderRefreshChange struct {
	Type       string `json:"type"` // "added", "removed", "updated"
	ModelID    string `json:"model_id"`
	ModelName  string `json:"model_name,omitempty"`
	OldValue   string `json:"old_value,omitempty"`
	NewValue   string `json:"new_value,omitempty"`
}

// ProviderRefreshFailure represents a failure during provider refresh.
type ProviderRefreshFailure struct {
	ProviderID string `json:"provider_id"`
	Error      string `json:"error"`
	Retryable  bool   `json:"retryable"`
}

// ModelCatalogResponse is the response to a model catalog request.
type ModelCatalogResponse struct {
	Providers []ProviderCatalogItem `json:"providers"`
	Total     int                   `json:"total"`
}

// ModelCatalogRefreshRequest triggers a model catalog refresh.
type ModelCatalogRefreshRequest struct {
	ProviderID string `json:"provider_id,omitempty"` // empty = refresh all
}

// ModelCatalogRefreshResponse is the response to a refresh request.
type ModelCatalogRefreshResponse struct {
	Changes  []ProviderRefreshChange  `json:"changes,omitempty"`
	Failures []ProviderRefreshFailure `json:"failures,omitempty"`
}
