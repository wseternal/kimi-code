// Package oauth provides the OAuth toolkit facade (Gap #69).
package oauth

import (
	"context"
	"fmt"
	"sync"
)

// ToolkitConfig holds the OAuth toolkit configuration.
type ToolkitConfig struct {
	ManagedEndpoint string `json:"managed_endpoint"`
	RegistryURL     string `json:"registry_url,omitempty"`
	Token           string `json:"token,omitempty"`
	DataDir         string `json:"data_dir"`
}

// Toolkit is the high-level facade tying together identity, storage,
// manager, provisioning, usage, and feedback.
type Toolkit struct {
	mu          sync.RWMutex
	config      ToolkitConfig
	storage     TokenStorage
	manager     *Manager
	provisioner *ManagedProvisioner
	registry    *CustomRegistry
	coordinator *RefreshCoordinator
	usage       *UsageTracker
	feedback    *FeedbackClient
}

// NewToolkit creates a fully wired OAuth toolkit.
func NewToolkit(cfg ToolkitConfig) (*Toolkit, error) {
	storage := NewFileTokenStorage(cfg.DataDir)

	manager := NewManager(ManagerOptions{
		Storage:   storage,
		ConfigDir: cfg.DataDir,
	})

	var provisioner *ManagedProvisioner
	if cfg.ManagedEndpoint != "" {
		provisioner = NewManagedProvisioner(cfg.ManagedEndpoint, cfg.Token)
	}

	registry := NewCustomRegistry()

	var coordinator *RefreshCoordinator
	if provisioner != nil {
		coordinator = NewRefreshCoordinator(provisioner, registry, 0)
	}

	return &Toolkit{
		config:      cfg,
		storage:     storage,
		manager:     manager,
		provisioner: provisioner,
		registry:    registry,
		coordinator: coordinator,
		usage:       NewUsageTracker(cfg.ManagedEndpoint, cfg.Token),
		feedback:    NewFeedbackClient(cfg.ManagedEndpoint, cfg.Token),
	}, nil
}

// Storage returns the token storage.
func (t *Toolkit) Storage() TokenStorage { return t.storage }

// Manager returns the manager component.
func (t *Toolkit) Manager() *Manager { return t.manager }

// Provisioner returns the managed provisioner.
func (t *Toolkit) Provisioner() *ManagedProvisioner { return t.provisioner }

// Coordinator returns the refresh coordinator.
func (t *Toolkit) Coordinator() *RefreshCoordinator { return t.coordinator }

// Usage returns the usage tracker.
func (t *Toolkit) Usage() *UsageTracker { return t.usage }

// Feedback returns the feedback client.
func (t *Toolkit) Feedback() *FeedbackClient { return t.feedback }

// RefreshModels refreshes all provider models.
func (t *Toolkit) RefreshModels(ctx context.Context) ([]*ProviderRefreshResult, error) {
	if t.coordinator == nil {
		return nil, fmt.Errorf("no coordinator configured")
	}
	return t.coordinator.RefreshAll(ctx)
}
