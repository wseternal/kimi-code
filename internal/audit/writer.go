package audit

import (
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
)

const (
	writerChanSize  = 512           // Buffer for burst protection
	flushInterval   = 100 * time.Millisecond
	flushBatchSize  = 32
)

// Writer provides non-blocking audit event recording. Events are buffered
// in a channel and drained into BadgerDB by a background goroutine using
// WriteBatch for efficiency. Record() never blocks the caller.
type Writer struct {
	db   *badger.DB
	ch   chan AuditEvent
	done chan struct{}
	seq  atomic.Uint64
}

// NewWriter creates a Writer and starts the background drain goroutine.
func NewWriter(db *badger.DB) *Writer {
	w := &Writer{
		db:   db,
		ch:   make(chan AuditEvent, writerChanSize),
		done: make(chan struct{}),
	}
	go w.run()
	return w
}

// Record enqueues an event for async persistence. If the channel is full,
// the event is dropped (with a warning log) rather than blocking the caller.
// The event's Seq field is assigned automatically.
func (w *Writer) Record(evt AuditEvent) {
	evt.Seq = w.seq.Add(1)
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	select {
	case w.ch <- evt:
	default:
		log.Printf("[audit] WARNING: dropped event %s (channel full)", evt.Type)
	}
}

// SaveSession upserts session metadata and the updated-time index.
// Unlike Record, this is synchronous — the caller needs confirmation
// that the session was persisted.
func (w *Writer) SaveSession(s SessionRecord) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	return w.db.Update(func(txn *badger.Txn) error {
		// Write session record
		if err := txn.Set(keySession(s.ID), data); err != nil {
			return err
		}
		// Write updated-time index (overwrite is fine — latest wins)
		return txn.Set(keyUpdated(s.UpdatedAt, s.ID), nil)
	})
}

// Close drains remaining events and stops the background goroutine.
func (w *Writer) Close() error {
	close(w.done)
	return nil
}

// run is the background goroutine that drains events into BadgerDB.
func (w *Writer) run() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	var buffer []AuditEvent

	for {
		select {
		case evt := <-w.ch:
			buffer = append(buffer, evt)
			if len(buffer) >= flushBatchSize {
				w.flush(buffer)
				buffer = buffer[:0]
			}
		case <-ticker.C:
			if len(buffer) > 0 {
				w.flush(buffer)
				buffer = buffer[:0]
			}
		case <-w.done:
			// Drain remaining events from channel
			for {
				select {
				case evt := <-w.ch:
					buffer = append(buffer, evt)
				default:
					if len(buffer) > 0 {
						w.flush(buffer)
					}
					return
				}
			}
		}
	}
}

// flush writes a batch of events to BadgerDB using WriteBatch.
func (w *Writer) flush(events []AuditEvent) {
	if len(events) == 0 {
		return
	}

	wb := w.db.NewWriteBatch()
	for _, evt := range events {
		data, err := json.Marshal(StoredEvent{
			Type: evt.Type,
			Data: marshalData(evt.Data),
			Ts:   evt.Timestamp.UnixNano(),
			Seq:  evt.Seq,
		})
		if err != nil {
			log.Printf("[audit] marshal error for %s: %v", evt.Type, err)
			continue
		}
		if err := wb.Set(keyEvent(evt.SessionID, evt.Timestamp, evt.Seq), data); err != nil {
			log.Printf("[audit] write error for %s: %v", evt.Type, err)
		}
	}
	if err := wb.Flush(); err != nil {
		log.Printf("[audit] flush error: %v", err)
	}
}

// marshalData converts Data to json.RawMessage. Returns nil if Data is nil.
func marshalData(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
