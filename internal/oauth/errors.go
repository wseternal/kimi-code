package oauth

import "errors"

// Sentinel errors for OAuth operations.
var (
	ErrUnauthorized      = errors.New("oauth: unauthorized (token refresh rejected)")
	ErrDeviceCodeTimeout = errors.New("oauth: device code timeout")
	ErrAccessDenied      = errors.New("oauth: access denied")
)

// OAuthError represents a detailed OAuth error.
type OAuthError struct {
	Message string
	Cause   error
}

func (e *OAuthError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *OAuthError) Unwrap() error {
	return e.Cause
}

// NewOAuthError creates a new OAuth error with a message.
func NewOAuthError(message string) *OAuthError {
	return &OAuthError{Message: message}
}

// NewOAuthErrorWithCause creates a new OAuth error with a message and cause.
func NewOAuthErrorWithCause(message string, cause error) *OAuthError {
	return &OAuthError{Message: message, Cause: cause}
}
