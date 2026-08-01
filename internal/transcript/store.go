package transcript

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Store provides persistent storage for transcript snapshots.
// It uses an in-memory snapshot with optional file-based persistence.
type Store struct {
	mu        sync.RWMutex
	state     *Snapshot
	ops       []Operation
	dir       string
	sessionID string
}

// NewStore creates a transcript store.
func NewStore(dir, sessionID string) *Store {
	s := &Store{
		state:     NewSnapshot(),
		dir:       dir,
		sessionID: sessionID,
	}
	// Try to load persisted state
	if dir != "" && sessionID != "" {
		s.load()
	}
	return s
}

// Apply applies an operation to the store and optionally persists it.
func (s *Store) Apply(op Operation) ApplyResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := ApplyOperation(s.state, op)
	if result.Changed {
		s.state = result.State
		s.ops = append(s.ops, op)
	}
	return result
}

// State returns a copy of the current snapshot.
func (s *Store) State() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copySnapshot(s.state)
}

// Operations returns all recorded operations.
func (s *Store) Operations() []Operation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ops := make([]Operation, len(s.ops))
	copy(ops, s.ops)
	return ops
}

// Reset clears the transcript.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = NewSnapshot()
	s.ops = nil
}

// Persist writes the current state and operations to disk.
func (s *Store) Persist() error {
	if s.dir == "" || s.sessionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.dir, "transcripts", s.sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create transcript dir: %w", err)
	}

	// Write snapshot
	snapData, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), snapData, 0644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	// Write operations log
	opsData, err := json.Marshal(s.ops)
	if err != nil {
		return fmt.Errorf("marshal operations: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "operations.json"), opsData, 0644); err != nil {
		return fmt.Errorf("write operations: %w", err)
	}

	return nil
}

func (s *Store) load() {
	dir := filepath.Join(s.dir, "transcripts", s.sessionID)
	snapPath := filepath.Join(dir, "snapshot.json")
	data, err := os.ReadFile(snapPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("transcript: failed to read snapshot: %v", err)
		}
		return
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("transcript: failed to parse snapshot: %v", err)
		return
	}
	s.state = &snap

	opsPath := filepath.Join(dir, "operations.json")
	opsData, err := os.ReadFile(opsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("transcript: failed to read operations: %v", err)
		}
		return
	}
	var ops []Operation
	if err := json.Unmarshal(opsData, &ops); err != nil {
		log.Printf("transcript: failed to parse operations: %v", err)
		return
	}
	s.ops = ops
}
