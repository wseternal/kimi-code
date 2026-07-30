package kapserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthToken represents an API authentication token.
type AuthToken struct {
	ID        string    `json:"id"`
	Token     string    `json:"-"` // never sent in responses
	Hash      string    `json:"hash"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// IsExpired reports whether the token has expired.
func (t *AuthToken) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false // never expires
	}
	return time.Now().After(t.ExpiresAt)
}

// TokenStore manages authentication tokens.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*AuthToken // keyed by hash
}

// NewTokenStore creates a new token store.
func NewTokenStore() *TokenStore {
	return &TokenStore{tokens: make(map[string]*AuthToken)}
}

// GenerateToken creates a new API token.
func (s *TokenStore) GenerateToken(name string, ttl time.Duration) (*AuthToken, string, error) {
	// Generate random token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, "", err
	}
	rawToken := hex.EncodeToString(b)
	hash := hashToken(rawToken)

	token := &AuthToken{
		ID:        fmt.Sprintf("tok_%d", time.Now().UnixNano()),
		Token:     rawToken,
		Hash:      hash,
		Name:      name,
		CreatedAt: time.Now(),
	}
	if ttl > 0 {
		token.ExpiresAt = time.Now().Add(ttl)
	}

	s.mu.Lock()
	s.tokens[hash] = token
	s.mu.Unlock()

	return token, rawToken, nil
}

// ValidateToken checks a raw token and returns the AuthToken if valid.
func (s *TokenStore) ValidateToken(rawToken string) (*AuthToken, bool) {
	hash := hashToken(rawToken)
	s.mu.RLock()
	token, ok := s.tokens[hash]
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if token.IsExpired() {
		// Evict expired token to prevent unbounded map growth.
		s.mu.Lock()
		delete(s.tokens, hash)
		s.mu.Unlock()
		return nil, false
	}

	// Update last used
	s.mu.Lock()
	token.LastUsed = time.Now()
	s.mu.Unlock()

	return token, true
}

// RevokeToken removes a token by ID.
func (s *TokenStore) RevokeToken(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, token := range s.tokens {
		if token.ID == id {
			delete(s.tokens, hash)
			return true
		}
	}
	return false
}

// ListTokens returns all non-expired tokens.
func (s *TokenStore) ListTokens() []*AuthToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*AuthToken, 0, len(s.tokens))
	for _, token := range s.tokens {
		if !token.IsExpired() {
			result = append(result, token)
		}
	}
	return result
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// contextKey is an unexported type for context keys.
type contextKey int

const tokenContextKey contextKey = iota

// BearerAuthMiddleware validates Bearer tokens from the Authorization header.
func BearerAuthMiddleware(store *TokenStore, skipPaths []string) func(http.Handler) http.Handler {
	skipSet := make(map[string]bool)
	for _, p := range skipPaths {
		skipSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for certain paths
			if skipSet[r.URL.Path] || r.URL.Path == "/api/v1/health" {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" {
				// Allow unauthenticated access if no tokens are configured
				if store == nil || len(store.ListTokens()) == 0 {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, `{"code":40111,"msg":"authorization required"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, `{"code":40112,"msg":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			token, ok := store.ValidateToken(parts[1])
			if !ok {
				http.Error(w, `{"code":40112,"msg":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), tokenContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TokenFromContext extracts the auth token from the request context.
func TokenFromContext(ctx context.Context) *AuthToken {
	t, _ := ctx.Value(tokenContextKey).(*AuthToken)
	return t
}
