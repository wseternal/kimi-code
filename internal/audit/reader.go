package audit

import (
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

// Reader provides query access to the audit store: session listing,
// session metadata retrieval, and event scanning.
type Reader struct {
	db *badger.DB
}

// NewReader creates a Reader backed by the given BadgerDB.
func NewReader(db *badger.DB) *Reader {
	return &Reader{db: db}
}

// ListSessions returns all sessions sorted by UpdatedAt descending (newest first).
func (r *Reader) ListSessions() ([]SessionSummary, error) {
	var summaries []SessionSummary

	err := r.db.View(func(txn *badger.Txn) error {
		prefix := []byte(prefixUpdated)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false // values are empty

		it := txn.NewIterator(opts)
		defer it.Close()

		// Collect in forward order (oldest first), then reverse below.
		var ids []string
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			id := parseUpdatedKey(it.Item().Key())
			if id != "" {
				ids = append(ids, id)
			}
		}

		// Reverse to get newest first.
		for i := len(ids) - 1; i >= 0; i-- {
			rec, err := r.getSession(txn, ids[i])
			if err != nil {
				continue // skip sessions with missing metadata
			}
			summaries = append(summaries, SessionSummary{
				ID:        rec.ID,
				Title:     rec.Title,
				Status:    rec.Status,
				CreatedAt: rec.CreatedAt,
				UpdatedAt: rec.UpdatedAt,
			})
		}
		return nil
	})
	return summaries, err
}

// GetSession returns session metadata by ID.
func (r *Reader) GetSession(id string) (*SessionRecord, error) {
	var rec *SessionRecord
	err := r.db.View(func(txn *badger.Txn) error {
		var err error
		rec, err = r.getSession(txn, id)
		return err
	})
	return rec, err
}

// GetLatestSessionID returns the ID of the most recently updated session.
// Returns "" and no error if no sessions exist.
func (r *Reader) GetLatestSessionID() (string, error) {
	var id string
	err := r.db.View(func(txn *badger.Txn) error {
		prefix := []byte(prefixUpdated)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		// Forward scan, keep the last ID (newest).
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if parsed := parseUpdatedKey(it.Item().Key()); parsed != "" {
				id = parsed
			}
		}
		return nil
	})
	return id, err
}

// ReadEvents returns all audit events for a session in chronological order.
func (r *Reader) ReadEvents(sessionID string) ([]StoredEvent, error) {
	var events []StoredEvent

	err := r.db.View(func(txn *badger.Txn) error {
		prefix := []byte(prefixEvent + sessionID + ":")
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = true
		opts.PrefetchSize = 100

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			val, err := it.Item().ValueCopy(nil)
			if err != nil {
				return fmt.Errorf("read event value: %w", err)
			}
			var evt StoredEvent
			if err := json.Unmarshal(val, &evt); err != nil {
				continue // skip malformed events
			}
			events = append(events, evt)
		}
		return nil
	})
	return events, err
}

// getSession loads a session record within an existing transaction.
func (r *Reader) getSession(txn *badger.Txn, id string) (*SessionRecord, error) {
	item, err := txn.Get(keySession(id))
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, fmt.Errorf("session %s not found", id)
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		return nil, fmt.Errorf("copy session value: %w", err)
	}
	var rec SessionRecord
	if err := json.Unmarshal(val, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &rec, nil
}
