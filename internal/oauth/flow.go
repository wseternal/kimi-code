package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

const (
	httpTimeout           = 30 * time.Second
	maxResponseSize       = 1 << 20 // 1 MB
	deviceCodeGrantType   = "urn:ietf:params:oauth:grant-type:device_code"
	refreshTokenGrantType = "refresh_token"
)

// httpClient is a dedicated HTTP client for OAuth requests.
var httpClient = &http.Client{Timeout: httpTimeout}

// PollResultKind represents the result of polling for a device token.
type PollResultKind int

const (
	PollSuccess PollResultKind = iota
	PollPending
	PollExpired
	PollDenied
)

// PollResult represents the result of a device token poll.
type PollResult struct {
	Kind        PollResultKind
	Token       *TokenInfo
	ErrorCode   string
	Description string
}

// postForm sends a form-encoded POST request and returns the status and JSON response.
func postForm(ctx context.Context, reqURL string, params map[string]string, headers DeviceHeaders) (int, map[string]interface{}, error) {
	form := neturl.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, NewOAuthErrorWithCause("failed to create request", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, NewOAuthErrorWithCause("request failed", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return resp.StatusCode, nil, NewOAuthErrorWithCause("failed to read response", err)
	}

	data := make(map[string]interface{})
	if len(body) > 0 {
		if err := json.Unmarshal(body, &data); err != nil {
			return resp.StatusCode, nil, NewOAuthErrorWithCause("invalid JSON response from server", err)
		}
	}

	return resp.StatusCode, data, nil
}

// tokenFromResponse extracts a TokenInfo from an OAuth response.
func tokenFromResponse(data map[string]interface{}) (*TokenInfo, error) {
	accessToken, ok := data["access_token"].(string)
	if !ok || accessToken == "" {
		return nil, NewOAuthError("OAuth response missing access_token")
	}

	refreshToken, _ := data["refresh_token"].(string)

	expiresInRaw, ok := data["expires_in"]
	if !ok {
		return nil, NewOAuthError("OAuth response missing expires_in")
	}

	var expiresIn int
	switch v := expiresInRaw.(type) {
	case float64:
		expiresIn = int(v)
	default:
		return nil, NewOAuthError("OAuth response has invalid expires_in")
	}

	if expiresIn <= 0 {
		return nil, NewOAuthError("OAuth response has invalid expires_in")
	}

	scope, _ := data["scope"].(string)
	tokenType, _ := data["token_type"].(string)
	if tokenType == "" {
		tokenType = "Bearer"
	}

	return &TokenInfo{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Unix() + int64(expiresIn),
		Scope:        scope,
		TokenType:    tokenType,
		ExpiresIn:    expiresIn,
	}, nil
}

// RequestDeviceAuthorization requests device authorization from the OAuth server.
func RequestDeviceAuthorization(ctx context.Context, config FlowConfig, headers DeviceHeaders) (*DeviceAuthorization, error) {
	url := strings.TrimSuffix(config.OAuthHost, "/") + "/api/oauth/device_authorization"

	status, data, err := postForm(ctx, url, map[string]string{
		"client_id": config.ClientID,
	}, headers)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		errMsg := "unknown"
		if msg, ok := data["error_description"].(string); ok {
			errMsg = msg
		}
		return nil, NewOAuthError(fmt.Sprintf("device authorization failed (HTTP %d): %s", status, errMsg))
	}

	userCode, ok := data["user_code"].(string)
	if !ok || userCode == "" {
		return nil, NewOAuthError("device authorization response missing user_code")
	}

	deviceCode, ok := data["device_code"].(string)
	if !ok || deviceCode == "" {
		return nil, NewOAuthError("device authorization response missing device_code")
	}

	verificationURIComplete, ok := data["verification_uri_complete"].(string)
	if !ok || verificationURIComplete == "" {
		return nil, NewOAuthError("device authorization response missing verification_uri_complete")
	}

	verificationURI, _ := data["verification_uri"].(string)
	var expiresIn int
	if v, ok := data["expires_in"].(float64); ok {
		expiresIn = int(v)
	}

	interval := 5
	if v, ok := data["interval"].(float64); ok {
		interval = int(v)
	}

	return &DeviceAuthorization{
		UserCode:                userCode,
		DeviceCode:              deviceCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

// PollDeviceToken polls for a device token.
func PollDeviceToken(ctx context.Context, config FlowConfig, deviceCode string, headers DeviceHeaders) (*PollResult, error) {
	url := strings.TrimSuffix(config.OAuthHost, "/") + "/api/oauth/token"

	status, data, err := postForm(ctx, url, map[string]string{
		"client_id":   config.ClientID,
		"device_code": deviceCode,
		"grant_type":  deviceCodeGrantType,
	}, headers)
	if err != nil {
		return nil, err
	}

	if status == http.StatusOK {
		if _, ok := data["access_token"].(string); ok {
			token, err := tokenFromResponse(data)
			if err != nil {
				return nil, err
			}
			return &PollResult{Kind: PollSuccess, Token: token}, nil
		}
	}

	if status >= 500 {
		// Per RFC 8628 §3.5, transient server errors should be retried.
		// Return PollPending so the caller continues polling.
		description, _ := data["error_description"].(string)
		return &PollResult{
			Kind:        PollPending,
			ErrorCode:   "server_error",
			Description: description,
		}, nil
	}

	errorCode, _ := data["error"].(string)
	description, _ := data["error_description"].(string)

	switch errorCode {
	case "authorization_pending", "slow_down":
		return &PollResult{Kind: PollPending, ErrorCode: errorCode, Description: description}, nil
	case "expired_token":
		return &PollResult{Kind: PollExpired}, nil
	case "access_denied":
		return &PollResult{Kind: PollDenied, Description: description}, nil
	default:
		return nil, NewOAuthError(fmt.Sprintf("device token polling failed (HTTP %d): %s %s", status, errorCode, description))
	}
}

// RefreshOptions configures refresh behavior.
type RefreshOptions struct {
	MaxRetries int
	BackoffMs  func(attempt int) int
}

// DefaultRefreshOptions returns default refresh options.
func DefaultRefreshOptions() RefreshOptions {
	return RefreshOptions{
		MaxRetries: 3,
		BackoffMs: func(attempt int) int {
			return (1 << attempt) * 1000 // 1s, 2s, 4s
		},
	}
}

// RefreshAccessToken refreshes an access token using a refresh token.
func RefreshAccessToken(ctx context.Context, config FlowConfig, refreshToken string, headers DeviceHeaders, opts RefreshOptions) (*TokenInfo, error) {
	url := strings.TrimSuffix(config.OAuthHost, "/") + "/api/oauth/token"

	if opts.MaxRetries == 0 {
		opts = DefaultRefreshOptions()
	}

	var lastError error
	for attempt := 0; attempt < opts.MaxRetries; attempt++ {
		status, data, err := postForm(ctx, url, map[string]string{
			"client_id":     config.ClientID,
			"grant_type":    refreshTokenGrantType,
			"refresh_token": refreshToken,
		}, headers)
		if err != nil {
			lastError = err
			if attempt < opts.MaxRetries-1 {
				if !contextSleep(ctx, time.Duration(opts.BackoffMs(attempt))*time.Millisecond) {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, lastError
		}

		if status == http.StatusOK {
			if _, ok := data["access_token"].(string); ok {
				return tokenFromResponse(data)
			}
		}

		errorCode, _ := data["error"].(string)
		errMsg := "unknown"
		if msg, ok := data["error_description"].(string); ok {
			errMsg = msg
		}

		// Unauthorized errors - don't retry
		if status == http.StatusUnauthorized || status == http.StatusForbidden || errorCode == "invalid_grant" {
			return nil, &OAuthError{Message: errMsg, Cause: ErrUnauthorized}
		}

		// Retryable errors
		if status == http.StatusTooManyRequests || status >= 500 {
			lastError = NewOAuthError(errMsg)
			if attempt < opts.MaxRetries-1 {
				if !contextSleep(ctx, time.Duration(opts.BackoffMs(attempt))*time.Millisecond) {
					return nil, ctx.Err()
				}
				continue
			}
		} else {
			// Non-retryable error
			return nil, NewOAuthError(errMsg)
		}
	}

	if lastError != nil {
		return nil, lastError
	}
	return nil, NewOAuthError("token refresh failed after retries")
}

// contextSleep sleeps for the given duration, but returns false if the context
// is cancelled before the sleep completes.
func contextSleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
