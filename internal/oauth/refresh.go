// Package oauth provides provider model refresh coordination (Gap #68).
package oauth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ProviderRefreshResult holds the result of a model refresh.
type ProviderRefreshResult struct {
	ProviderID string        `json:"provider_id"`
	Added      []string      `json:"added,omitempty"`
	Removed    []string      `json:"removed,omitempty"`
	Updated    []string      `json:"updated,omitempty"`
	Failures   []string      `json:"failures,omitempty"`
	Duration   time.Duration `json:"duration"`
	Timestamp  time.Time     `json:"timestamp"`
}

// RefreshCoordinator coordinates refreshing all provider models.
type RefreshCoordinator struct {
	mu         sync.Mutex
	provisioner *ManagedProvisioner
	registry   *CustomRegistry
	cache      map[string][]ManagedModel
	lastRefresh map[string]time.Time
	interval   time.Duration
}

// NewRefreshCoordinator creates a refresh coordinator.
func NewRefreshCoordinator(provisioner *ManagedProvisioner, registry *CustomRegistry, interval time.Duration) *RefreshCoordinator {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &RefreshCoordinator{
		provisioner: provisioner,
		registry:    registry,
		cache:       make(map[string][]ManagedModel),
		lastRefresh: make(map[string]time.Time),
		interval:    interval,
	}
}

// NeedsRefresh checks if a provider needs refreshing.
func (c *RefreshCoordinator) NeedsRefresh(providerID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	last, ok := c.lastRefresh[providerID]
	if !ok {
		return true
	}
	return time.Since(last) > c.interval
}

// RefreshProvider refreshes models for a single provider.
func (c *RefreshCoordinator) RefreshProvider(ctx context.Context, providerID string) (*ProviderRefreshResult, error) {
	start := time.Now()

	models, err := c.provisioner.FetchModels(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("refresh provider %s: %w", providerID, err)
	}

	c.mu.Lock()
	old := c.cache[providerID]
	c.cache[providerID] = models
	c.lastRefresh[providerID] = time.Now()
	c.mu.Unlock()

	result := &ProviderRefreshResult{
		ProviderID: providerID,
		Duration:   time.Since(start),
		Timestamp:  time.Now(),
	}

	// Diff old vs new
	oldSet := make(map[string]bool)
	for _, m := range old {
		oldSet[m.ID] = true
	}
	newSet := make(map[string]bool)
	for _, m := range models {
		newSet[m.ID] = true
		if !oldSet[m.ID] {
			result.Added = append(result.Added, m.ID)
		}
	}
	for _, m := range old {
		if !newSet[m.ID] {
			result.Removed = append(result.Removed, m.ID)
		}
	}

	return result, nil
}

// RefreshAll refreshes all known providers.
func (c *RefreshCoordinator) RefreshAll(ctx context.Context) ([]*ProviderRefreshResult, error) {
	providers, err := c.provisioner.FetchProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch providers: %w", err)
	}

	var results []*ProviderRefreshResult
	for _, p := range providers {
		result, err := c.RefreshProvider(ctx, p.ID)
		if err != nil {
			results = append(results, &ProviderRefreshResult{
				ProviderID: p.ID,
				Failures:   []string{err.Error()},
				Timestamp:  time.Now(),
			})
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// GetCachedModels returns cached models for a provider.
func (c *RefreshCoordinator) GetCachedModels(providerID string) ([]ManagedModel, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	models, ok := c.cache[providerID]
	return models, ok
}

// CacheSize returns the number of cached providers.
func (c *RefreshCoordinator) CacheSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cache)
}
