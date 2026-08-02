package oauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	minRefreshThresholdSec = 300
	refreshThresholdRatio  = 0.5
	deviceCodeTimeoutSec   = 15 * 60 // 15 minutes
)

// LoginOptions configures the login flow.
type LoginOptions struct {
	OnDeviceCode func(auth *DeviceAuthorization) error
}

// Manager orchestrates the OAuth token lifecycle.
type Manager struct {
	config      FlowConfig
	storage     TokenStorage
	headers     DeviceHeaders
	configDir   string // for cross-process lock files
	mu          sync.Mutex
	inFlight    *inFlightRefresh
	now         func() int64 // injectable clock (unix seconds)
	sleepFunc   func(d time.Duration)
}

type inFlightRefresh struct {
	ch    chan struct{}
	force bool
	token string
	err   error
}

// ManagerOptions configures the Manager.
type ManagerOptions struct {
	Config    FlowConfig
	Storage   TokenStorage
	Headers   DeviceHeaders
	ConfigDir string
	Now       func() int64
	Sleep     func(d time.Duration)
}

// NewManager creates a new OAuth Manager.
func NewManager(opts ManagerOptions) *Manager {
	m := &Manager{
		config:    opts.Config,
		storage:   opts.Storage,
		headers:   opts.Headers,
		configDir: opts.ConfigDir,
		now:       opts.Now,
		sleepFunc: opts.Sleep,
	}
	if m.now == nil {
		m.now = func() int64 { return time.Now().Unix() }
	}
	if m.sleepFunc == nil {
		m.sleepFunc = time.Sleep
	}
	return m
}

// NewDefaultManager creates a Manager with default settings for Kimi Code.
func NewDefaultManager() (*Manager, error) {
	storage, err := NewDefaultTokenStorage()
	if err != nil {
		return nil, fmt.Errorf("create token storage: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	configDir := filepath.Join(homeDir, ".kimi-code")

	return NewManager(ManagerOptions{
		Config:    DefaultFlowConfig(),
		Storage:   storage,
		Headers:   CreateDeviceHeaders(ClientVersion, configDir),
		ConfigDir: configDir,
	}), nil
}

// HasToken reports whether a valid token is stored.
func (m *Manager) HasToken(ctx context.Context) bool {
	token, err := m.storage.Load(ctx, m.config.Name)
	if err != nil || token == nil {
		return false
	}
	return token.AccessToken != ""
}

// GetCachedAccessToken returns the stored access token without refreshing.
func (m *Manager) GetCachedAccessToken(ctx context.Context) (string, error) {
	token, err := m.storage.Load(ctx, m.config.Name)
	if err != nil {
		return "", err
	}
	if token == nil || token.AccessToken == "" {
		return "", nil
	}
	return token.AccessToken, nil
}

// Logout removes the stored token.
func (m *Manager) Logout(ctx context.Context) error {
	return m.storage.Remove(ctx, m.config.Name)
}

// refreshThreshold returns the seconds before expiry when refresh should trigger.
func refreshThreshold(expiresIn int) int {
	if expiresIn > 0 {
		threshold := int(float64(expiresIn) * refreshThresholdRatio)
		if threshold < minRefreshThresholdSec {
			return minRefreshThresholdSec
		}
		return threshold
	}
	return minRefreshThresholdSec
}

// shouldRefresh checks if a token needs refreshing.
func (m *Manager) shouldRefresh(token *TokenInfo, force bool) bool {
	if force {
		return true
	}
	if token.ExpiresAt == 0 {
		return false
	}
	remaining := token.ExpiresAt - m.now()
	return remaining < int64(refreshThreshold(token.ExpiresIn))
}

// EnsureFresh returns a valid access token, refreshing if needed.
func (m *Manager) EnsureFresh(ctx context.Context, force bool) (string, error) {
	// Check in-flight refresh
	m.mu.Lock()
	if m.inFlight != nil {
		inflight := m.inFlight
		m.mu.Unlock()
		<-inflight.ch
		if !force || inflight.force {
			return inflight.token, inflight.err
		}
		// Force on top of non-force: check if peer's refresh made the token fresh enough
		if inflight.err == nil && inflight.token != "" {
			token, err := m.storage.Load(ctx, m.config.Name)
			if err == nil && token != nil && !m.shouldRefresh(token, false) {
				return token.AccessToken, nil
			}
		}
		// Still stale, do our own refresh
		return m.doEnsureFresh(ctx, force)
	}

	// Start new refresh
	ch := make(chan struct{})
	m.inFlight = &inFlightRefresh{ch: ch, force: force}
	m.mu.Unlock()

	token, err := m.doEnsureFresh(ctx, force)

	m.mu.Lock()
	if m.inFlight != nil && m.inFlight.ch == ch {
		m.inFlight.token = token
		m.inFlight.err = err
		close(ch)
		m.inFlight = nil
	}
	m.mu.Unlock()

	return token, err
}

func (m *Manager) doEnsureFresh(ctx context.Context, force bool) (string, error) {
	token, err := m.storage.Load(ctx, m.config.Name)
	if err != nil {
		return "", err
	}
	if token == nil {
		return "", &OAuthError{Message: fmt.Sprintf("no token for %q, run /login to authenticate", m.config.Name), Cause: ErrUnauthorized}
	}
	if token.AccessToken == "" {
		return "", &OAuthError{Message: fmt.Sprintf("stored token for %q was rejected, re-login required", m.config.Name), Cause: ErrUnauthorized}
	}

	if !m.shouldRefresh(token, force) {
		return token.AccessToken, nil
	}

	// Acquire cross-process lock
	release, err := m.acquireLock(ctx)
	if err != nil {
		return "", fmt.Errorf("acquire lock: %w", err)
	}
	defer release()

	// Post-lock re-read
	afterLock, err := m.storage.Load(ctx, m.config.Name)
	if err != nil {
		return "", err
	}
	if afterLock != nil && afterLock.AccessToken == "" {
		return "", &OAuthError{Message: "token was rejected while waiting for lock", Cause: ErrUnauthorized}
	}

	activeToken := token
	if afterLock != nil {
		if !m.shouldRefresh(afterLock, force) {
			return afterLock.AccessToken, nil
		}
		if force && afterLock.RefreshToken != token.RefreshToken {
			return afterLock.AccessToken, nil
		}
		activeToken = afterLock
	}

	if activeToken.RefreshToken == "" {
		return "", &OAuthError{Message: "token has no refresh_token", Cause: ErrUnauthorized}
	}

	// Attempt refresh
	refreshed, err := RefreshAccessToken(ctx, m.config, activeToken.RefreshToken, m.headers, DefaultRefreshOptions())
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			// Wait and re-read: peer may have rotated
			if !contextSleep(ctx, 100*time.Millisecond) {
				return "", ctx.Err()
			}
			recovery, _ := m.storage.Load(ctx, m.config.Name)
			if recovery != nil && recovery.RefreshToken != activeToken.RefreshToken && recovery.AccessToken != "" {
				return recovery.AccessToken, nil
			}
			// Write revoked tombstone
			tombstone := &TokenInfo{
				AccessToken:  "",
				RefreshToken: activeToken.RefreshToken,
				ExpiresAt:    activeToken.ExpiresAt,
				Scope:        activeToken.Scope,
				TokenType:    activeToken.TokenType,
				ExpiresIn:    activeToken.ExpiresIn,
			}
			if saveErr := m.storage.Save(ctx, m.config.Name, tombstone); saveErr != nil {
				return "", fmt.Errorf("save revoked tombstone: %w (original error: %v)", saveErr, err)
			}
		}
		return "", err
	}

	// Save refreshed token
	if err := m.storage.Save(ctx, m.config.Name, refreshed); err != nil {
		return "", fmt.Errorf("save refreshed token: %w", err)
	}

	return refreshed.AccessToken, nil
}

// Login drives the device code flow end-to-end.
func (m *Manager) Login(ctx context.Context, opts LoginOptions) (*TokenInfo, error) {
	startedAt := m.now()
	deadlineAt := startedAt + deviceCodeTimeoutSec

	for {
		// Request device authorization
		auth, err := RequestDeviceAuthorization(ctx, m.config, m.headers)
		if err != nil {
			return nil, fmt.Errorf("request device authorization: %w", err)
		}

		// Notify caller about device code
		if opts.OnDeviceCode != nil {
			if err := opts.OnDeviceCode(auth); err != nil {
				return nil, err
			}
		}

		// Poll for token
		interval := auth.Interval
		if interval < 1 {
			interval = 1
		}

		deviceExpired := false
		for {
			// Check context cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			// Check timeout
			if m.now() >= deadlineAt {
				return nil, ErrDeviceCodeTimeout
			}

			result, err := PollDeviceToken(ctx, m.config, auth.DeviceCode, m.headers)
			if err != nil {
				return nil, fmt.Errorf("poll device token: %w", err)
			}

			switch result.Kind {
			case PollSuccess:
				if err := m.storage.Save(ctx, m.config.Name, result.Token); err != nil {
					return nil, fmt.Errorf("save token: %w", err)
				}
				return result.Token, nil
			case PollDenied:
				desc := result.Description
				if desc == "" {
					desc = "authorization denied"
				}
				return nil, &OAuthError{Message: desc, Cause: ErrAccessDenied}
			case PollExpired:
				deviceExpired = true
			case PollPending:
				if result.ErrorCode == "slow_down" {
					interval += 5
				}
				// Use context-aware sleep so cancellation is respected immediately
				if !contextSleep(ctx, time.Duration(interval)*time.Second) {
					return nil, ctx.Err()
				}
			}

			if deviceExpired {
				break
			}
		}

		// If device didn't expire, we exited the loop abnormally
		if !deviceExpired {
			break
		}

		// Check timeout before retrying outer loop
		if m.now() >= deadlineAt {
			return nil, ErrDeviceCodeTimeout
		}
	}

	return nil, NewOAuthError("device flow ended unexpectedly")
}

// staleLockThreshold is the maximum age of a lock directory before it is
// considered stale. Token refresh typically completes in <10s, so 2 minutes
// is a very generous margin.
const staleLockThreshold = 2 * time.Minute

// acquireLock acquires a cross-process lock using directory creation (mkdir).
// This is compatible with the TypeScript kimi CLI which uses mkdir/rmdir for
// the same lock path. Stale locks are detected via the directory's mtime.
// No files are written inside the lock directory, ensuring that rmdir-based
// release (used by both CLIs) works without ENOTEMPTY errors.
// Returns a release function and an error.
func (m *Manager) acquireLock(ctx context.Context) (func(), error) {
	if m.configDir == "" {
		return func() {}, nil
	}

	lockDir := filepath.Join(m.configDir, "oauth")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	lockPath := filepath.Join(lockDir, m.config.Name+".lock")

	for i := 0; i < 120; i++ {
		if err := os.Mkdir(lockPath, 0o700); err == nil {
			// Acquired. Release via os.Remove (rmdir) — compatible with TS CLI.
			return func() {
				os.Remove(lockPath)
			}, nil
		}

		// Lock directory exists — check mtime for stale detection.
		// A lock older than the threshold is assumed orphaned.
		if info, err := os.Stat(lockPath); err == nil {
			if time.Since(info.ModTime()) > staleLockThreshold {
				os.Remove(lockPath)
				continue
			}
		}

		if !contextSleep(ctx, 500*time.Millisecond) {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("failed to acquire lock after 60s")
}
