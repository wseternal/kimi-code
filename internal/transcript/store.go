package transcript

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// maxOps is the maximum number of operations kept in memory.
// Older operations are discarded to prevent unbounded growth.
const maxOps = 10000

// Store provides persistent storage for transcript snapshots.
// It uses an in-memory snapshot with optional file-based persistence.
type Store struct {
	mu        sync.RWMutex
	state     *Snapshot
	ops       []Operation
	dir       string
	sessionID string
	logger    *slog.Logger
}

// NewStore creates a transcript store.
func NewStore(dir, sessionID string) *Store {
	return NewStoreWithLogger(dir, sessionID, nil)
}

// NewStoreWithLogger creates a transcript store with a custom logger.
func NewStoreWithLogger(dir, sessionID string, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Store{
		state:     NewSnapshot(),
		dir:       dir,
		sessionID: sessionID,
		logger:    logger,
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
		// Bound ops in memory to prevent unbounded growth.
		if len(s.ops) > maxOps {
			s.ops = s.ops[len(s.ops)-maxOps:]
		}
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

// Persist writes the current state and operations to disk atomically (W3 fix).
// Uses write-to-tmp + rename to prevent inconsistent state on crash.
func (s *Store) Persist() error {
	if s.dir == "" || s.sessionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.dir, "transcripts", s.sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create transcript dir: %w", err)
	}

	snapPath := filepath.Join(dir, "snapshot.json")
	opsPath := filepath.Join(dir, "operations.json")
	snapTmp := snapPath + ".tmp"
	opsTmp := opsPath + ".tmp"

	// Write snapshot to tmp file
	snapData, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(snapTmp, snapData, 0600); err != nil {
		return fmt.Errorf("write snapshot tmp: %w", err)
	}

	// Write operations to tmp file
	opsData, err := json.Marshal(s.ops)
	if err != nil {
		os.Remove(snapTmp) // cleanup on failure
		return fmt.Errorf("marshal operations: %w", err)
	}
	if err := os.WriteFile(opsTmp, opsData, 0600); err != nil {
		os.Remove(snapTmp) // cleanup on failure
		return fmt.Errorf("write operations tmp: %w", err)
	}

	// Rename operations first, then snapshot. If snapshot rename fails,
	// the old snapshot + new operations can be recovered on reload by
	// re-applying operations to the old snapshot (COW is idempotent).
	if err := os.Rename(opsTmp, opsPath); err != nil {
		os.Remove(snapTmp)
		os.Remove(opsTmp)
		return fmt.Errorf("rename operations: %w", err)
	}
	if err := os.Rename(snapTmp, snapPath); err != nil {
		os.Remove(snapTmp)
		return fmt.Errorf("rename snapshot: %w", err)
	}

	return nil
}

func (s *Store) load() {
	dir := filepath.Join(s.dir, "transcripts", s.sessionID)
	snapPath := filepath.Join(dir, "snapshot.json")
	data, err := os.ReadFile(snapPath)
	if err != nil {
		if !os.IsNotExist(err) {
			s.logger.Error("transcript: failed to read snapshot", "error", err)
		}
		return
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		s.logger.Error("transcript: failed to parse snapshot", "error", err)
		return
	}
	s.state = &snap

	opsPath := filepath.Join(dir, "operations.json")
	opsData, err := os.ReadFile(opsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			s.logger.Error("transcript: failed to read operations", "error", err)
		}
		return
	}
	var ops []Operation
	if err := json.Unmarshal(opsData, &ops); err != nil {
		s.logger.Error("transcript: failed to parse operations", "error", err)
		return
	}
	s.ops = ops
}
