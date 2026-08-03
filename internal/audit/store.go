package audit

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

// Store manages a BadgerDB instance for session audit data.
// Each session gets its own Store backed by a directory under
// the session's data path (e.g. ~/.gkimi-code/sessions/{id}/badger/).
type Store struct {
	db *badger.DB
}

// Open opens or creates a BadgerDB at dir with options tuned for
// a CLI audit trail: async writes, small value log, no versioning.
func Open(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithSyncWrites(false).           // Crash may lose last ~1s; acceptable for audit
		WithLoggingLevel(badger.WARNING). // Reduce noise
		WithNumVersionsToKeep(1).        // Append-only; no MVCC needed
		WithCompactL0OnClose(true).      // Keep reads fast across restarts
		WithValueLogFileSize(16 << 20).  // 16 MB — sessions are small
		WithDetectConflicts(false)       // Single-writer goroutine

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger at %s: %w", dir, err)
	}
	return &Store{db: db}, nil
}

// Close runs value log GC and closes the database.
func (s *Store) Close() error {
	// Run GC to reclaim space from deleted/overwritten values.
	for {
		if err := s.db.RunValueLogGC(0.5); err != nil {
			break // no more GC needed
		}
	}
	return s.db.Close()
}

// DB returns the underlying BadgerDB for advanced use (tests, migrations).
func (s *Store) DB() *badger.DB {
	return s.db
}
