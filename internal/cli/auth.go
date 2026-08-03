package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/oauth"
)

// authWizard runs the interactive login wizard (OAuth or API key).
func authWizard(cfg *config.Config, configPath string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("🔑 Authentication Setup")
	fmt.Println()
	fmt.Println("  1. Kimi Code (OAuth)")
	fmt.Println("  2. API Key (manual)")
	fmt.Println()
	fmt.Print("Choose authentication method [1]: ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	if choice == "" {
		choice = "1"
	}

	switch choice {
	case "1":
		return oauthLogin(cfg, configPath)
	case "2":
		return apiKeyLogin(cfg, configPath)
	default:
		return fmt.Errorf("invalid choice: %s", choice)
	}
}

// oauthLogin runs the OAuth device code flow for Kimi Code.
func oauthLogin(cfg *config.Config, configPath string) error {
	manager, err := oauth.NewDefaultManager()
	if err != nil {
		return fmt.Errorf("initialize OAuth: %w", err)
	}

	// Use signal-aware context so Ctrl+C cancels the polling loop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	token, err := manager.Login(ctx, oauth.LoginOptions{
		OnDeviceCode: func(auth *oauth.DeviceAuthorization) error {
			url := auth.VerificationURIComplete
			if url == "" {
				url = auth.VerificationURI
			}
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "Opening browser for Kimi device login: %s\n", url)
			fmt.Fprintf(os.Stderr, "If the browser did not open, visit the URL above and enter code: %s\n", auth.UserCode)
			if auth.ExpiresIn > 0 {
				fmt.Fprintf(os.Stderr, "Code expires in %ds.\n", auth.ExpiresIn)
			}
			fmt.Fprintf(os.Stderr, "Waiting for authorization to complete...\n\n")

			// Best-effort browser open
			oauth.OpenURL(url)
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Provision managed config
	baseURL := oauth.ResolveBaseURL()
	oauthHost := oauth.GetOAuthHost()

	models, err := oauth.FetchManagedModels(ctx, token.AccessToken, baseURL, nil)
	if err != nil {
		return fmt.Errorf("fetch models after login: %w", err)
	}

	if err := oauth.ProvisionConfig(cfg, models, baseURL, oauthHost); err != nil {
		return fmt.Errorf("provision config: %w", err)
	}

	if err := cfg.SaveToFile(configPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Logged in to %s (%d models available)\n", oauth.ManagedProvider, len(models))
	return nil
}

// apiKeyLogin runs the interactive API key setup.
func apiKeyLogin(cfg *config.Config, configPath string) error {
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

// logoutProvider removes authentication for a provider.
func logoutProvider(cfg *config.Config, configPath string, provName string) error {
	if provName == "" {
		provName = cfg.ResolveProviderName()
	}
	if provName == "" {
		return fmt.Errorf("no provider configured")
	}

	// Check if this is the managed OAuth provider
	if provName == oauth.ManagedProvider || (provName == cfg.ResolveProviderName() && cfg.Providers[provName].OAuth != nil) {
		// Clear managed config
		oauth.ClearManagedConfig(cfg)

		// Delete stored token
		manager, err := oauth.NewDefaultManager()
		if err == nil {
			ctx := context.Background()
			if logoutErr := manager.Logout(ctx); logoutErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: token file could not be deleted: %v\n", logoutErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: could not initialize OAuth manager to delete token: %v\n", err)
		}

		if err := cfg.SaveToFile(configPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Printf("✓ Logged out from %s\n", provName)
		return nil
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

// hasAnyAuth checks if any provider has authentication configured.
func hasAnyAuth(cfg *config.Config) bool {
	for _, prov := range cfg.Providers {
		if prov.APIKey != "" || prov.OAuth != nil {
			return true
		}
	}
	return false
}

// sessionsDir returns the sessions storage directory.
func sessionsDir(homeDir string) string {
	return filepath.Join(homeDir, config.DataDirName, "sessions")
}
