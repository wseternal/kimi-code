package oauth

import "os"

// ClientVersion is the version string sent in OAuth device headers.
// Set by the CLI package at startup.
var ClientVersion = "unknown"

// OAuth constants for Kimi Code.
const (
	DefaultOAuthHost  = "https://auth.kimi.com"
	ClientID          = "17e5f671-d194-4dfb-9706-5516cb48c098"
	ProviderName      = "kimi-code"
	ManagedProvider   = "managed:kimi-code"
	OAuthKey          = "oauth/kimi-code"
	KimiCodePlatform  = "kimi_code_cli"
	DefaultBaseURL    = "https://api.kimi.com/coding/v1"
)

// ResolveBaseURL returns the API base URL, checking environment variables first.
func ResolveBaseURL() string {
	if envURL := os.Getenv("KIMI_CODE_BASE_URL"); envURL != "" {
		return envURL
	}
	return DefaultBaseURL
}

// GetOAuthHost returns the OAuth host, checking environment variables first.
func GetOAuthHost() string {
	if host := os.Getenv("KIMI_CODE_OAUTH_HOST"); host != "" {
		return host
	}
	if host := os.Getenv("KIMI_OAUTH_HOST"); host != "" {
		return host
	}
	return DefaultOAuthHost
}

// DefaultFlowConfig returns the default OAuth flow configuration for Kimi Code.
func DefaultFlowConfig() FlowConfig {
	return FlowConfig{
		Name:      ProviderName,
		OAuthHost: GetOAuthHost(),
		ClientID:  ClientID,
	}
}
