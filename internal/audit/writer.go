package audit

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
)

const (
	writerChanSize = 512 // Buffer for burst protection
	flushInterval  = 100 * time.Millisecond
	flushBatchSize = 32
	sessionAckTimeout = 5 * time.Second
)

// sessionOp is an internal message for routing SaveSession through the
// background goroutine so that all BadgerDB writes are single-threaded.
type sessionOp struct {
	record SessionRecord
	ack    chan error
}

// Writer provides non-blocking audit event recording. Events are buffered
// in a channel and drained into BadgerDB by a background goroutine using
// WriteBatch for efficiency. Record() never blocks the caller.
//
// All BadgerDB writes (events and session saves) are performed by the
// background goroutine to avoid concurrent write conflicts and prevent
// the TUI goroutine from blocking on database operations.
type Writer struct {
	db       *badger.DB
	ch       chan AuditEvent
	sessionCh chan sessionOp
	done     chan struct{}
	wg       sync.WaitGroup
	closed   atomic.Bool
	seq      atomic.Uint64

	// Tracks the last UpdatedAt nanos per session ID so SaveSession
	// can delete the stale s:updated: index key before writing a new one.
	sessions map[string]int64
}

// NewWriter creates a Writer and starts the background drain goroutine.
func NewWriter(db *badger.DB) *Writer {
	w := &Writer{
		db:        db,
		ch:        make(chan AuditEvent, writerChanSize),
		sessionCh: make(chan sessionOp, 4),
		done:      make(chan struct{}),
		sessions:  make(map[string]int64),
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.run()
	}()
	return w
}

// Record enqueues an event for async persistence. If the channel is full,
// the event is dropped (with a warning log) rather than blocking the caller.
// The event's Seq field is assigned automatically.
func (w *Writer) Record(evt AuditEvent) {
	if w.closed.Load() {
		return
	}
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
// The write is routed through the background goroutine so that all
// BadgerDB writes are single-threaded and the caller's goroutine is
// not blocked by database I/O.
func (w *Writer) SaveSession(s SessionRecord) error {
	if w.closed.Load() {
		return fmt.Errorf("writer closed")
	}
	ack := make(chan error, 1)
	select {
	case w.sessionCh <- sessionOp{record: s, ack: ack}:
	case <-w.done:
		return fmt.Errorf("writer closed")
	}
	select {
	case err := <-ack:
		return err
	case <-time.After(sessionAckTimeout):
		return fmt.Errorf("session save timed out")
	}
}

// Close drains remaining events, processes pending session saves,
// and blocks until the background goroutine has finished.
// Safe to call multiple times.
func (w *Writer) Close() error {
	if w.closed.Swap(true) {
		return nil // already closed
	}
	close(w.done)
	w.wg.Wait()
	return nil
}

// run is the background goroutine that drains events and session saves
// into BadgerDB. All database writes happen here.
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
		case op := <-w.sessionCh:
			err := w.saveSessionDirect(op.record)
			op.ack <- err
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
					// Drain pending session saves
					for {
						select {
						case op := <-w.sessionCh:
							err := w.saveSessionDirect(op.record)
							op.ack <- err
						default:
							return
						}
					}
				}
			}
		}
	}
}

// saveSessionDirect writes a session record and updates the time index.
// Must only be called from the background goroutine.
func (w *Writer) saveSessionDirect(s SessionRecord) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	return w.db.Update(func(txn *badger.Txn) error {
		// Delete stale updated-time index key for this session
		if prevNano, ok := w.sessions[s.ID]; ok {
			oldKey := fmt.Sprintf("%s%020d:%s", prefixUpdated, prevNano, s.ID)
			_ = txn.Delete([]byte(oldKey))
		}
		// Write session record
		if err := txn.Set(keySession(s.ID), data); err != nil {
			return err
		}
		// Write updated-time index
		if err := txn.Set(keyUpdated(s.UpdatedAt, s.ID), nil); err != nil {
			return err
		}
		w.sessions[s.ID] = s.UpdatedAt.UnixNano()
		return nil
	})
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
