package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/visdomtech/kimi-code/internal/persistence"
)

// SerializedSession is the JSON representation persisted to disk.
type SerializedSession struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Status    Status         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Persist handles saving and loading session state to a FileStore.
type Persist struct {
	store persistence.Store
}

// NewPersist creates a new persistence handler.
func NewPersist(store persistence.Store) *Persist {
	return &Persist{store: store}
}

// sessionKey returns the store key for a session's metadata.
func sessionKey(id string) string {
	return fmt.Sprintf("sessions/%s/session.json", id)
}

// Save persists a session's metadata to the store.
func (p *Persist) Save(ctx context.Context, s *Session) error {
	s.mu.RLock()
	data := SerializedSession{
		ID:        s.ID,
		Title:     s.Title,
		Status:    s.Status,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Metadata:  s.Metadata,
	}
	s.mu.RUnlock()

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return p.store.Set(ctx, sessionKey(s.ID), raw)
}

// Load restores a session's metadata from the store.
func (p *Persist) Load(ctx context.Context, id string) (*SerializedSession, error) {
	raw, err := p.store.Get(ctx, sessionKey(id))
	if err != nil {
		return nil, fmt.Errorf("load session %s: %w", id, err)
	}
	var s SerializedSession
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("unmarshal session %s: %w", id, err)
	}
	return &s, nil
}

// Delete removes a session's persisted data.
func (p *Persist) Delete(ctx context.Context, id string) error {
	return p.store.Del(ctx, sessionKey(id))
}

// ListIDs returns all persisted session IDs.
func (p *Persist) ListIDs(ctx context.Context) ([]string, error) {
	keys, err := p.store.Keys(ctx, "sessions/")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, key := range keys {
		// Extract session ID from "sessions/<id>/..."
		parts := splitKey(key)
		if len(parts) >= 2 && parts[0] == "sessions" && !seen[parts[1]] {
			seen[parts[1]] = true
			ids = append(ids, parts[1])
		}
	}
	return ids, nil
}

// splitKey splits a "/" separated store key into parts.
func splitKey(key string) []string {
	var parts []string
	current := ""
	for _, c := range key {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
