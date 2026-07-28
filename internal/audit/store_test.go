package audit

import (
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// openTestDB creates a temporary BadgerDB for testing.
func openTestDB(t *testing.T) *badger.DB {
	t.Helper()
	dir, err := os.MkdirTemp("", "audit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	opts := badger.DefaultOptions(dir).
		WithSyncWrites(false).
		WithLoggingLevel(badger.ERROR).
		WithNumVersionsToKeep(1).
		WithValueLogFileSize(16 << 20).
		WithDetectConflicts(false)

	db, err := badger.Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStoreOpenClose(t *testing.T) {
	dir, err := os.MkdirTemp("", "audit-store-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if store.DB() == nil {
		t.Fatal("DB() returned nil")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestKeyEncoding(t *testing.T) {
	// Session key
	k := keySession("abc123")
	if string(k) != "s:session:abc123" {
		t.Errorf("keySession: got %q", k)
	}

	// Updated key — lexicographic ordering
	ts1 := time.Unix(0, 1000000000) // 1s
	ts2 := time.Unix(0, 2000000000) // 2s
	k1 := string(keyUpdated(ts1, "a"))
	k2 := string(keyUpdated(ts2, "a"))
	if k1 >= k2 {
		t.Errorf("keyUpdated ordering: %q should be < %q", k1, k2)
	}

	// parseUpdatedKey
	id := parseUpdatedKey(keyUpdated(ts1, "sess-42"))
	if id != "sess-42" {
		t.Errorf("parseUpdatedKey: got %q, want %q", id, "sess-42")
	}

	// Event key — chronological ordering within session
	ek1 := string(keyEvent("s1", ts1, 1))
	ek2 := string(keyEvent("s1", ts1, 2))
	ek3 := string(keyEvent("s1", ts2, 1))
	if ek1 >= ek2 {
		t.Errorf("event key seq ordering: %q should be < %q", ek1, ek2)
	}
	if ek2 >= ek3 {
		t.Errorf("event key time ordering: %q should be < %q", ek2, ek3)
	}
}

func TestWriterSaveSessionAndReader(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(db)
	defer w.Close()

	rec := SessionRecord{
		ID:        "sess-1",
		Title:     "Test Session",
		Status:    "idle",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now(),
		Metadata:  map[string]any{"tokens_in": float64(5000)},
	}
	if err := w.SaveSession(rec); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Read back
	r := NewReader(db)
	got, err := r.GetSession("sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != "sess-1" || got.Title != "Test Session" {
		t.Errorf("GetSession: got %+v", got)
	}
	if got.Metadata["tokens_in"] != float64(5000) {
		t.Errorf("metadata tokens_in: got %v", got.Metadata["tokens_in"])
	}
}

func TestWriterRecordAndReaderEvents(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(db)

	sessionID := "sess-events"
	now := time.Now()

	// Record several events
	w.Record(AuditEvent{
		SessionID: sessionID,
		Type:      EvtUserInput,
		Timestamp: now,
		Data:      map[string]any{"text": "hello"},
	})
	w.Record(AuditEvent{
		SessionID: sessionID,
		Type:      EvtLLMDeltaText,
		Timestamp: now.Add(1 * time.Millisecond),
		Data:      map[string]any{"text": "Hi there"},
	})
	w.Record(AuditEvent{
		SessionID: sessionID,
		Type:      EvtTurnCompleted,
		Timestamp: now.Add(2 * time.Millisecond),
		Data: TurnRecord{
			Prompt:   "hello",
			Response: "Hi there",
			Usage:    &UsageRecord{InputOther: 100, Output: 50},
		},
	})

	// Close writer to flush remaining events
	w.Close()
	time.Sleep(50 * time.Millisecond) // let goroutine finish

	// Read events back
	r := NewReader(db)
	events, err := r.ReadEvents(sessionID)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != EvtUserInput {
		t.Errorf("event[0] type: got %q", events[0].Type)
	}
	if events[1].Type != EvtLLMDeltaText {
		t.Errorf("event[1] type: got %q", events[1].Type)
	}
	if events[2].Type != EvtTurnCompleted {
		t.Errorf("event[2] type: got %q", events[2].Type)
	}
}

func TestReaderListSessions(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(db)
	defer w.Close()

	now := time.Now()
	// Save 3 sessions with different UpdatedAt times
	sessions := []SessionRecord{
		{ID: "s1", Title: "First", Status: "idle", CreatedAt: now, UpdatedAt: now},
		{ID: "s2", Title: "Second", Status: "running", CreatedAt: now, UpdatedAt: now.Add(time.Second)},
		{ID: "s3", Title: "Third", Status: "completed", CreatedAt: now, UpdatedAt: now.Add(2 * time.Second)},
	}
	for _, sess := range sessions {
		if err := w.SaveSession(sess); err != nil {
			t.Fatalf("SaveSession %s: %v", sess.ID, err)
		}
	}

	r := NewReader(db)
	got, err := r.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(got))
	}

	// Should be sorted by UpdatedAt descending: s3 (newest), s2, s1 (oldest)
	if got[0].ID != "s3" {
		t.Errorf("sessions[0] should be s3 (newest), got %s", got[0].ID)
	}
	if got[2].ID != "s1" {
		t.Errorf("sessions[2] should be s1 (oldest), got %s", got[2].ID)
	}
}

func TestReaderGetLatestSessionID(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(db)
	defer w.Close()

	r := NewReader(db)

	// Empty store
	id, err := r.GetLatestSessionID()
	if err != nil {
		t.Fatalf("GetLatestSessionID empty: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty ID, got %q", id)
	}

	// Add sessions
	now := time.Now()
	_ = w.SaveSession(SessionRecord{ID: "old", UpdatedAt: now})
	_ = w.SaveSession(SessionRecord{ID: "new", UpdatedAt: now.Add(time.Second)})

	id, err = r.GetLatestSessionID()
	if err != nil {
		t.Fatalf("GetLatestSessionID: %v", err)
	}
	if id != "new" {
		t.Errorf("expected 'new', got %q", id)
	}
}

func TestFacadeLoadSession(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(db)

	now := time.Now()
	sessionID := "sess-facade"

	// Save session
	_ = w.SaveSession(SessionRecord{
		ID:        sessionID,
		Title:     "Facade Test",
		Status:    "idle",
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Record a user input
	w.Record(AuditEvent{
		SessionID: sessionID,
		Type:      EvtUserInput,
		Timestamp: now.Add(1 * time.Millisecond),
		Data:      map[string]any{"text": "What is Go?"},
	})

	// Record a turn.completed event
	w.Record(AuditEvent{
		SessionID: sessionID,
		Type:      EvtTurnCompleted,
		Timestamp: now.Add(2 * time.Millisecond),
		Data: TurnRecord{
			Prompt:   "What is Go?",
			Response: "Go is a programming language.",
			Thinking: "Let me explain Go...",
			Tools: []ToolCallRecord{
				{Name: "search", Arguments: `{"q":"Go language"}`, Result: "found", Duration: 100 * time.Millisecond},
			},
			Usage: &UsageRecord{InputOther: 200, Output: 80, InputCacheRead: 50},
		},
	})

	// Record a second turn
	w.Record(AuditEvent{
		SessionID: sessionID,
		Type:      EvtTurnCompleted,
		Timestamp: now.Add(3 * time.Millisecond),
		Data: TurnRecord{
			Prompt:   "Tell me more",
			Response: "Go was created at Google.",
		},
	})

	// Close writer to flush
	w.Close()
	time.Sleep(50 * time.Millisecond)

	// Load via facade
	r := NewReader(db)
	f := NewFacade(r)

	data, err := f.LoadSession(sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if data.Session.Title != "Facade Test" {
		t.Errorf("session title: got %q", data.Session.Title)
	}
	if len(data.Turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(data.Turns))
	}
	if data.Turns[0].Prompt != "What is Go?" {
		t.Errorf("turn[0] prompt: got %q", data.Turns[0].Prompt)
	}
	if data.Turns[0].Response != "Go is a programming language." {
		t.Errorf("turn[0] response: got %q", data.Turns[0].Response)
	}
	if data.Turns[0].Thinking != "Let me explain Go..." {
		t.Errorf("turn[0] thinking: got %q", data.Turns[0].Thinking)
	}
	if len(data.Turns[0].Tools) != 1 {
		t.Fatalf("turn[0] tools: expected 1, got %d", len(data.Turns[0].Tools))
	}
	if data.Turns[0].Tools[0].Name != "search" {
		t.Errorf("turn[0] tool name: got %q", data.Turns[0].Tools[0].Name)
	}
	if data.Turns[0].Usage == nil || data.Turns[0].Usage.InputOther != 200 {
		t.Errorf("turn[0] usage: got %+v", data.Turns[0].Usage)
	}
	if data.Turns[1].Prompt != "Tell me more" {
		t.Errorf("turn[1] prompt: got %q", data.Turns[1].Prompt)
	}
}

func TestFacadeGetLatest(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(db)

	now := time.Now()
	_ = w.SaveSession(SessionRecord{ID: "s1", Title: "Old", UpdatedAt: now})
	_ = w.SaveSession(SessionRecord{ID: "s2", Title: "New", UpdatedAt: now.Add(time.Second)})

	// Add a turn to s2
	w.Record(AuditEvent{
		SessionID: "s2",
		Type:      EvtTurnCompleted,
		Timestamp: now.Add(2 * time.Second),
		Data:      TurnRecord{Prompt: "hi", Response: "hello"},
	})

	w.Close()
	time.Sleep(50 * time.Millisecond)

	f := NewFacade(NewReader(db))
	data, err := f.GetLatest()
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if data == nil {
		t.Fatal("GetLatest returned nil")
	}
	if data.Session.ID != "s2" {
		t.Errorf("expected s2, got %s", data.Session.ID)
	}
	if len(data.Turns) != 1 || data.Turns[0].Prompt != "hi" {
		t.Errorf("turns: %+v", data.Turns)
	}
}

func TestFacadeGetLatestEmpty(t *testing.T) {
	db := openTestDB(t)
	f := NewFacade(NewReader(db))

	data, err := f.GetLatest()
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil for empty store, got %+v", data)
	}
}

func TestWriterCloseDrains(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(db)

	// Record events then immediately close
	for i := 0; i < 10; i++ {
		w.Record(AuditEvent{
			SessionID: "drain-test",
			Type:      EvtLLMDeltaText,
			Timestamp: time.Now(),
			Data:      map[string]any{"text": "chunk"},
		})
	}
	w.Close()
	time.Sleep(100 * time.Millisecond)

	r := NewReader(db)
	events, err := r.ReadEvents("drain-test")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 10 {
		t.Errorf("expected 10 events after drain, got %d", len(events))
	}
}
