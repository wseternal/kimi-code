package kapserver

import (
	"sort"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/protocol/rest"
)

// configProviderAdapter wraps config.Config to implement ConfigProvider.
type configProviderAdapter struct {
	cfg *config.Config
}

// NewConfigProvider creates a ConfigProvider backed by a config.Config.
func NewConfigProvider(cfg *config.Config) ConfigProvider {
	return &configProviderAdapter{cfg: cfg}
}

// ListModels returns all configured models as catalog items, sorted by ID.
func (a *configProviderAdapter) ListModels() []rest.ModelCatalogItem {
	items := make([]rest.ModelCatalogItem, 0, len(a.cfg.Models))
	for name, m := range a.cfg.Models {
		item := rest.ModelCatalogItem{
			ID:           name,
			Name:         name,
			Provider:     m.Provider,
			Capabilities: m.Capabilities,
		}
		if m.MaxContextSize > 0 {
			item.MaxTokens = m.MaxContextSize
		}
		if m.DisplayName != "" {
			item.Name = m.DisplayName
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

// ListProviders returns all configured provider names, sorted.
func (a *configProviderAdapter) ListProviders() []string {
	names := make([]string, 0, len(a.cfg.Providers))
	for name := range a.cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PermissionMode returns the current permission mode string.
func (a *configProviderAdapter) PermissionMode() string {
	if a.cfg.Yolo {
		return "yolo"
	}
	return "manual"
}
