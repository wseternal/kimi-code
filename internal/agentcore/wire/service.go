package wire

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/visdomtech/kimi-code/internal/persistence"
	"github.com/visdomtech/kimi-code/internal/protocol"
)

// Journal is an append-only log of operations for audit and replay.
type Journal struct {
	log     *persistence.AppendLog
	seq     atomic.Int64
	mu      sync.RWMutex
	entries []Entry
}

// Entry is a single journal entry.
type Entry struct {
	Seq       int64           `json:"seq"`
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Data      json.RawMessage `json:"data"`
}

// NewJournal creates a new journal backed by the append log.
func NewJournal(log *persistence.AppendLog) *Journal {
	return &Journal{log: log}
}

// Append adds an entry to the journal.
func (j *Journal) Append(ctx context.Context, entryType, sessionID string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	seq := j.seq.Add(1)
	entry := Entry{
		Seq:       seq,
		Timestamp: time.Now(),
		Type:      entryType,
		SessionID: sessionID,
		Data:      raw,
	}
	rawEntry, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	j.mu.Lock()
	j.entries = append(j.entries, entry)
	j.mu.Unlock()
	_, err = j.log.Append(ctx, rawEntry)
	return err
}

// Entries returns all journal entries.
func (j *Journal) Entries() []Entry {
	j.mu.RLock()
	defer j.mu.RUnlock()
	result := make([]Entry, len(j.entries))
	copy(result, j.entries)
	return result
}

// Seq returns the current sequence number.
func (j *Journal) Seq() int64 {
	return j.seq.Load()
}

// Service manages wire protocol state (journal + model snapshots).
type Service struct {
	journal *Journal
	states  map[string]*ModelState
	mu      sync.RWMutex
}

// ModelState is a snapshot of agent state for a session.
type ModelState struct {
	SessionID  string              `json:"sessionId"`
	Messages   []protocol.Message  `json:"messages"`
	Tools      []string            `json:"tools"`
	Seq        int64               `json:"seq"`
	UpdatedAt  time.Time           `json:"updatedAt"`
}

// NewService creates a new wire service.
func NewService(journal *Journal) *Service {
	return &Service{
		journal: journal,
		states:  make(map[string]*ModelState),
	}
}

// Journal returns the underlying journal.
func (s *Service) Journal() *Journal {
	return s.journal
}

// GetState returns the model state for a session.
func (s *Service) GetState(sessionID string) *ModelState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.states[sessionID]
}

// SetState updates the model state for a session.
func (s *Service) SetState(state *ModelState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.UpdatedAt = time.Now()
	state.Seq = s.journal.Seq()
	s.states[state.SessionID] = state
}

// DeleteState removes the model state for a session.
func (s *Service) DeleteState(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, sessionID)
}
