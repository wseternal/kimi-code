package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/event"
	"github.com/visdomtech/kimi-code/internal/protocol"
)

// Status represents the lifecycle status of a session.
type Status string

const (
	StatusCreated   Status = "created"
	StatusRunning   Status = "running"
	StatusIdle      Status = "idle"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Session represents an active conversation session.
type Session struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Status      Status              `json:"status"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
	scope       *di.Scope
	eventBus    *event.Bus[Event]
	mu          sync.RWMutex
}

// Event is a session-level event.
type Event struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// NewSession creates a new session.
func NewSession(id, title string, parentScope *di.Scope) (*Session, error) {
	scope, err := parentScope.CreateChild(di.ScopeSession, id)
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID:        id,
		Title:     title,
		Status:    StatusCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]any),
		scope:     scope,
		eventBus:  event.NewBus[Event](),
	}
	scope.Register("session", s)
	return s, nil
}

// Scope returns the session's DI scope.
func (s *Session) Scope() *di.Scope {
	return s.scope
}

// EventBus returns the session's event bus.
func (s *Session) EventBus() *event.Bus[Event] {
	return s.eventBus
}

// SetStatus updates the session status.
func (s *Session) SetStatus(status Status) {
	s.mu.Lock()
	s.Status = status
	s.UpdatedAt = time.Now()
	s.mu.Unlock()

	s.eventBus.Publish(Event{
		Type:      "session.status.changed",
		SessionID: s.ID,
		Timestamp: time.Now(),
		Data:      map[string]any{"status": status},
	})
}

// SetTitle updates the session title.
func (s *Session) SetTitle(title string) {
	s.mu.Lock()
	s.Title = title
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
}

// ToProtocol converts the session to a protocol Session.
func (s *Session) ToProtocol() protocol.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return protocol.Session{
		ID:        s.ID,
		Title:     s.Title,
		CreatedAt: s.CreatedAt.Format(time.RFC3339),
		UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
	}
}

// Dispose disposes the session and its scope.
func (s *Session) Dispose() {
	s.eventBus.Unsubscribe()
	s.scope.Dispose()
}

// Manager manages multiple sessions.
type Manager struct {
	sessions map[string]*Session
	appScope *di.Scope
	store    *SessionStore
	mu       sync.RWMutex
}

// NewManager creates a new session manager.
func NewManager(appScope *di.Scope) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		appScope: appScope,
	}
}

// SetStore sets the persistent session store.
func (m *Manager) SetStore(store *SessionStore) {
	m.store = store
}

// Store returns the persistent session store (may be nil).
func (m *Manager) Store() *SessionStore {
	return m.store
}

// Create creates a new session.
func (m *Manager) Create(_ context.Context, id, title string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; exists {
		return nil, &protocol.APIError{Code: protocol.ErrorCodeSessionExists, Message: "session already exists"}
	}

	sess, err := NewSession(id, title, m.appScope)
	if err != nil {
		return nil, err
	}
	m.sessions[id] = sess

	// Auto-persist if store is available
	if m.store != nil {
		_ = m.store.Save(context.Background(), sess)
	}

	return sess, nil
}

// Get returns a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[id]
	return sess, ok
}

// List returns all sessions.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		result = append(result, sess)
	}
	return result
}

// Delete removes a session.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if ok {
		sess.Dispose()
		// Remove from persistent store
		if m.store != nil {
			_ = m.store.DeleteSession(context.Background(), id)
		}
	}
}

// GetLatest returns the most recently updated session from the store.
func (m *Manager) GetLatest() (*Session, error) {
	if m.store == nil {
		return nil, fmt.Errorf("no session store configured")
	}
	ctx := context.Background()
	id, err := m.store.GetLatest(ctx)
	if err != nil {
		return nil, err
	}
	// Check if already loaded
	if sess, ok := m.Get(id); ok {
		return sess, nil
	}
	return m.store.Load(ctx, id, m)
}

// Resume loads a session by ID from the store.
func (m *Manager) Resume(id string) (*Session, error) {
	if m.store == nil {
		return nil, fmt.Errorf("no session store configured")
	}
	if sess, ok := m.Get(id); ok {
		return sess, nil
	}
	return m.store.Load(context.Background(), id, m)
}
