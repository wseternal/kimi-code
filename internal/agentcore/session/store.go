package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/persistence"
)

// SessionStore wraps persistence.Store with session-specific operations.
type SessionStore struct {
	persist *Persist
	history *History
	store   persistence.Store
	appScope *di.Scope
}

// NewSessionStore creates a new session store.
func NewSessionStore(store persistence.Store, appScope *di.Scope) *SessionStore {
	return &SessionStore{
		persist:  NewPersist(store),
		history:  NewHistory(store),
		store:    store,
		appScope: appScope,
	}
}

// Persist returns the underlying session persistence handler.
func (ss *SessionStore) Persist() *Persist {
	return ss.persist
}

// History returns the history handler.
func (ss *SessionStore) History() *History {
	return ss.history
}

// Save persists a session and optionally its history.
func (ss *SessionStore) Save(ctx context.Context, s *Session) error {
	return ss.persist.Save(ctx, s)
}

// Load loads a session by ID, restoring it into the manager.
func (ss *SessionStore) Load(ctx context.Context, id string, mgr *Manager) (*Session, error) {
	data, err := ss.persist.Load(ctx, id)
	if err != nil {
		return nil, err
	}

	sess, err := NewSession(data.ID, data.Title, mgr.appScope)
	if err != nil {
		return nil, err
	}
	sess.Status = data.Status
	sess.CreatedAt = data.CreatedAt
	sess.UpdatedAt = data.UpdatedAt
	if data.Metadata != nil {
		sess.Metadata = data.Metadata
	}

	// Load message history
	if err := ss.history.Load(ctx, id); err != nil {
		return nil, fmt.Errorf("load history for %s: %w", id, err)
	}

	mgr.mu.Lock()
	mgr.sessions[id] = sess
	mgr.mu.Unlock()

	return sess, nil
}

// ListAll returns metadata for all persisted sessions, sorted by update time (newest first).
func (ss *SessionStore) ListAll(ctx context.Context) ([]*SerializedSession, error) {
	ids, err := ss.persist.ListIDs(ctx)
	if err != nil {
		return nil, err
	}

	var sessions []*SerializedSession
	for _, id := range ids {
		s, err := ss.persist.Load(ctx, id)
		if err != nil {
			continue // skip corrupt sessions
		}
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// GetLatest returns the most recently updated session ID.
func (ss *SessionStore) GetLatest(ctx context.Context) (string, error) {
	sessions, err := ss.ListAll(ctx)
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions found")
	}
	return sessions[0].ID, nil
}

// Fork creates a deep copy of a session with a new ID and optional title.
func (ss *SessionStore) Fork(ctx context.Context, sourceID, newTitle string, mgr *Manager) (*Session, error) {
	// Load source session data
	srcData, err := ss.persist.Load(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("load source session: %w", err)
	}

	// Load source messages
	if err := ss.history.Load(ctx, sourceID); err != nil {
		return nil, fmt.Errorf("load source history: %w", err)
	}
	srcMessages := ss.history.Messages()

	// Create new session
	newID := fmt.Sprintf("session_%d", time.Now().UnixNano())
	if newTitle == "" {
		newTitle = srcData.Title + " (fork)"
	}

	newSess, err := NewSession(newID, newTitle, mgr.appScope)
	if err != nil {
		return nil, err
	}
	newSess.Status = StatusIdle
	newSess.Metadata = make(map[string]any)
	if srcData.Metadata != nil {
		for k, v := range srcData.Metadata {
			newSess.Metadata[k] = v
		}
	}

	// Persist new session metadata
	if err := ss.persist.Save(ctx, newSess); err != nil {
		return nil, err
	}

	// Copy messages
	for _, msg := range srcMessages {
		if err := ss.history.AddMessage(ctx, newID, msg); err != nil {
			return nil, fmt.Errorf("copy message: %w", err)
		}
	}

	mgr.mu.Lock()
	mgr.sessions[newID] = newSess
	mgr.mu.Unlock()

	return newSess, nil
}

// DeleteSession removes a session and its history from the store.
// Order matters: history (messages) is deleted first so the session
// directory still contains session.json when FileStore.Del cleans up
// empty parent directories. Reversing the order could leave orphan files.
func (ss *SessionStore) DeleteSession(ctx context.Context, id string) error {
	if err := ss.history.Clear(ctx, id); err != nil {
		return err
	}
	return ss.persist.Delete(ctx, id)
}

// PurgeEmptySessions removes all sessions that have no user messages.
// A session is considered empty if the user never sent any messages.
func (ss *SessionStore) PurgeEmptySessions(ctx context.Context) error {
	ids, err := ss.persist.ListIDs(ctx)
	if err != nil {
		return err
	}

	var errs []error
	for _, id := range ids {
		hasUser, err := ss.hasUserMessages(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("check session %s: %w", id, err))
			continue
		}
		if !hasUser {
			if err := ss.DeleteSession(ctx, id); err != nil {
				errs = append(errs, fmt.Errorf("delete session %s: %w", id, err))
			}
		}
	}
	return errors.Join(errs...)
}

// hasUserMessages checks if a session has any user messages by reading the JSONL directly.
func (ss *SessionStore) hasUserMessages(ctx context.Context, sessionID string) (bool, error) {
	data, err := ss.store.Get(ctx, messagesKey(sessionID))
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	// Scan JSONL for user messages
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Role == "user" {
			return true, nil
		}
	}
	return false, nil
}
