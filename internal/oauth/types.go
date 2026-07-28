// Package oauth implements OAuth 2.0 Device Code Flow (RFC 8628) for Kimi Code.
package oauth

// TokenInfo represents a persisted OAuth token bundle.
type TokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64 // Unix seconds when access_token expires
	Scope        string
	TokenType    string
	ExpiresIn    int // Original expires_in from server (seconds)
}

// DeviceAuthorization represents the RFC 8628 §3.2 device authorization response.
type DeviceAuthorization struct {
	UserCode                string
	DeviceCode              string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int // Seconds until device_code expires (may be 0)
	Interval                int // Polling interval in seconds
}

// FlowConfig holds OAuth flow endpoint and client configuration.
type FlowConfig struct {
	Name      string // Logical provider name for storage (e.g., "kimi-code")
	OAuthHost string // Base URL of OAuth server, no trailing slash
	ClientID  string // Client ID registered with OAuth provider
}

// DeviceHeaders contains device identification headers for X-Msh-* headers.
type DeviceHeaders map[string]string

// tokenInfoWire is the JSON wire format for token persistence (snake_case).
type tokenInfoWire struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// toWire converts TokenInfo to wire format.
func (t *TokenInfo) toWire() *tokenInfoWire {
	return &tokenInfoWire{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		ExpiresAt:    t.ExpiresAt,
		Scope:        t.Scope,
		TokenType:    t.TokenType,
		ExpiresIn:    t.ExpiresIn,
	}
}

// fromWire converts wire format to TokenInfo.
func fromWire(w *tokenInfoWire) *TokenInfo {
	if w == nil {
		return nil
	}
	return &TokenInfo{
		AccessToken:  w.AccessToken,
		RefreshToken: w.RefreshToken,
		ExpiresAt:    w.ExpiresAt,
		Scope:        w.Scope,
		TokenType:    w.TokenType,
		ExpiresIn:    w.ExpiresIn,
	}
}
