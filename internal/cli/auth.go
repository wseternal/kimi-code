package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
)

// authWizard runs the interactive API key setup.
func authWizard(cfg *config.Config, configPath string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("🔑 API Key Setup")
	fmt.Println()
	fmt.Println("Available providers: kimi, openai, anthropic, google")
	fmt.Print("Enter provider name: ")

	provName, _ := reader.ReadString('\n')
	provName = strings.TrimSpace(strings.ToLower(provName))
	if provName == "" {
		return fmt.Errorf("provider name required")
	}

	fmt.Printf("Enter API key for %s: ", provName)
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("API key required")
	}

	// Update config
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}

	prov, exists := cfg.Providers[provName]
	if !exists {
		prov = config.ProviderConfig{Type: provName}
	}
	prov.APIKey = apiKey
	cfg.Providers[provName] = prov

	if cfg.DefaultProvider == "" {
		cfg.DefaultProvider = provName
	}

	// Save to config file
	if err := cfg.SaveToFile(configPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("✓ API key saved for %s\n", provName)
	return nil
}

// logoutProvider removes the API key for a provider.
func logoutProvider(cfg *config.Config, configPath string, provName string) error {
	if provName == "" {
		provName = cfg.ResolveProviderName()
	}
	if provName == "" {
		return fmt.Errorf("no provider configured")
	}

	prov, exists := cfg.Providers[provName]
	if !exists {
		return fmt.Errorf("provider %s not found", provName)
	}

	prov.APIKey = ""
	prov.OAuth = nil
	cfg.Providers[provName] = prov

	if err := cfg.SaveToFile(configPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("✓ Removed API key for %s\n", provName)
	return nil
}

// hasAnyAPIKey checks if any provider has an API key configured.
func hasAnyAPIKey(cfg *config.Config) bool {
	for _, prov := range cfg.Providers {
		if prov.APIKey != "" {
			return true
		}
	}
	return false
}

// sessionsDir returns the sessions storage directory.
func sessionsDir(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code", "sessions")
}
