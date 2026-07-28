package providers

import (
	"context"
	"strings"

	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/oauth"
)

// OAuthProvider wraps a ChatProvider to inject OAuth access tokens.
type OAuthProvider struct {
	inner   kosong.ChatProvider
	manager *oauth.Manager
}

// NewOAuthProvider creates an OAuth-aware provider wrapper.
func NewOAuthProvider(inner kosong.ChatProvider, manager *oauth.Manager) *OAuthProvider {
	return &OAuthProvider{
		inner:   inner,
		manager: manager,
	}
}

func (p *OAuthProvider) Name() string                    { return p.inner.Name() }
func (p *OAuthProvider) ModelName() string                { return p.inner.ModelName() }
func (p *OAuthProvider) ThinkingEffort() kosong.ThinkingEffort { return p.inner.ThinkingEffort() }
func (p *OAuthProvider) MaxCompletionTokens() int         { return p.inner.MaxCompletionTokens() }

// injectAuth creates a deep copy of opts with the OAuth access token injected.
// This avoids mutating the caller's opts struct or its nested Auth pointer.
func injectAuth(opts *kosong.GenerateOptions, accessToken string) *kosong.GenerateOptions {
	var optsCopy kosong.GenerateOptions
	if opts != nil {
		optsCopy = *opts
	}
	// Deep-copy Auth to avoid mutating the caller's ProviderRequestAuth
	if optsCopy.Auth != nil {
		authCopy := *optsCopy.Auth
		optsCopy.Auth = &authCopy
	} else {
		optsCopy.Auth = &kosong.ProviderRequestAuth{}
	}
	optsCopy.Auth.APIKey = &accessToken
	return &optsCopy
}

func (p *OAuthProvider) Generate(ctx context.Context, systemPrompt string, tools []kosong.Tool, history []kosong.Message, opts *kosong.GenerateOptions) (*kosong.StreamedMessage, error) {
	// Ensure we have a fresh token
	accessToken, err := p.manager.EnsureFresh(ctx, false)
	if err != nil {
		return nil, err
	}

	// Inject auth into a copy of options to avoid mutating caller's struct
	authOpts := injectAuth(opts, accessToken)

	msg, err := p.inner.Generate(ctx, systemPrompt, tools, history, authOpts)
	if err != nil {
		// Check if it's a 401/unauthorized error - force refresh and retry once
		if isUnauthorizedError(err) {
			accessToken, retryErr := p.manager.EnsureFresh(ctx, true)
			if retryErr != nil {
				return nil, err // Return original error
			}
			retryOpts := injectAuth(opts, accessToken)
			return p.inner.Generate(ctx, systemPrompt, tools, history, retryOpts)
		}
		return nil, err
	}
	return msg, nil
}

func (p *OAuthProvider) WithThinking(effort kosong.ThinkingEffort) kosong.ChatProvider {
	return &OAuthProvider{
		inner:   p.inner.WithThinking(effort),
		manager: p.manager,
	}
}

func (p *OAuthProvider) WithMaxCompletionTokens(maxTokens int, opts *kosong.MaxCompletionTokensOptions) kosong.ChatProvider {
	return &OAuthProvider{
		inner:   p.inner.WithMaxCompletionTokens(maxTokens, opts),
		manager: p.manager,
	}
}

func (p *OAuthProvider) UploadVideo(ctx context.Context, input interface{}, opts *kosong.GenerateOptions) (*kosong.VideoURLPart, error) {
	// Ensure we have a fresh token and inject it
	accessToken, err := p.manager.EnsureFresh(ctx, false)
	if err != nil {
		return nil, err
	}

	authOpts := injectAuth(opts, accessToken)

	result, err := p.inner.UploadVideo(ctx, input, authOpts)
	if err != nil && isUnauthorizedError(err) {
		// Force refresh and retry once
		accessToken, retryErr := p.manager.EnsureFresh(ctx, true)
		if retryErr != nil {
			return nil, err
		}
		retryOpts := injectAuth(opts, accessToken)
		return p.inner.UploadVideo(ctx, input, retryOpts)
	}
	return result, err
}

// isUnauthorizedError checks if an error indicates an unauthorized response.
// It matches common patterns from HTTP provider errors (e.g., "API error 401: ...").
func isUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	return strings.Contains(lower, " 401") ||
		strings.Contains(lower, "status 401") ||
		strings.Contains(lower, "error 401") ||
		strings.Contains(lower, "unauthorized")
}
