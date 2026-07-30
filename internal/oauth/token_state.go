// Package oauth provides OAuth token state machine (Gap #88).
package oauth

import (
	"fmt"
	"sync"
	"time"
)

// TokenState represents the lifecycle state of an OAuth token.
type TokenState string

const (
	TokenStateAbsent   TokenState = "absent"
	TokenStatePending  TokenState = "pending"
	TokenStateActive   TokenState = "active"
	TokenStateExpired  TokenState = "expired"
	TokenStateRevoked  TokenState = "revoked"
	TokenStateError    TokenState = "error"
)

// TokenStateEvent represents a state transition event.
type TokenStateEvent struct {
	From      TokenState `json:"from"`
	To        TokenState `json:"to"`
	Reason    string     `json:"reason"`
	Timestamp time.Time  `json:"timestamp"`
}

// TokenStateMachine manages the token lifecycle state.
type TokenStateMachine struct {
	mu       sync.RWMutex
	state    TokenState
	history  []TokenStateEvent
	onChange func(event TokenStateEvent)
}

// NewTokenStateMachine creates a state machine starting in absent state.
func NewTokenStateMachine() *TokenStateMachine {
	return &TokenStateMachine{
		state: TokenStateAbsent,
	}
}

// State returns the current token state.
func (m *TokenStateMachine) State() TokenState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// SetChangeCallback sets a callback for state transitions.
func (m *TokenStateMachine) SetChangeCallback(fn func(TokenStateEvent)) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// validTransitions defines allowed state transitions.
var validTransitions = map[TokenState][]TokenState{
	TokenStateAbsent:  {TokenStatePending},
	TokenStatePending: {TokenStateActive, TokenStateError, TokenStateAbsent},
	TokenStateActive:  {TokenStateExpired, TokenStateRevoked, TokenStateAbsent},
	TokenStateExpired: {TokenStatePending, TokenStateAbsent},
	TokenStateRevoked: {TokenStatePending, TokenStateAbsent},
	TokenStateError:   {TokenStatePending, TokenStateAbsent},
}

// Transition attempts a state transition.
func (m *TokenStateMachine) Transition(to TokenState, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	allowed := validTransitions[m.state]
	for _, s := range allowed {
		if s == to {
			event := TokenStateEvent{
				From:      m.state,
				To:        to,
				Reason:    reason,
				Timestamp: time.Now(),
			}
			m.state = to
			m.history = append(m.history, event)
			if m.onChange != nil {
				go m.onChange(event)
			}
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s -> %s", m.state, to)
}

// MarkPending transitions to pending (start login flow).
func (m *TokenStateMachine) MarkPending() error {
	return m.Transition(TokenStatePending, "login started")
}

// MarkActive transitions to active (token obtained).
func (m *TokenStateMachine) MarkActive() error {
	return m.Transition(TokenStateActive, "token obtained")
}

// MarkExpired transitions to expired (token expired).
func (m *TokenStateMachine) MarkExpired() error {
	return m.Transition(TokenStateExpired, "token expired")
}

// MarkRevoked transitions to revoked (user logged out).
func (m *TokenStateMachine) MarkRevoked() error {
	return m.Transition(TokenStateRevoked, "token revoked")
}

// MarkError transitions to error state.
func (m *TokenStateMachine) MarkError(reason string) error {
	return m.Transition(TokenStateError, reason)
}

// Reset transitions back to absent.
func (m *TokenStateMachine) Reset() error {
	return m.Transition(TokenStateAbsent, "reset")
}

// IsActive returns true if the token is active.
func (m *TokenStateMachine) IsActive() bool {
	return m.State() == TokenStateActive
}

// History returns the state transition history.
func (m *TokenStateMachine) History() []TokenStateEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]TokenStateEvent, len(m.history))
	copy(result, m.history)
	return result
}
