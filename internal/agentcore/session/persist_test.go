package session

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/persistence"
)

func TestPersistRoundTrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "session-persist-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	appScope := di.NewAppScope("test")
	sess, err := NewSession("test-123", "My Session", appScope)
	if err != nil {
		t.Fatal(err)
	}
	sess.Status = StatusRunning
	sess.Metadata["key"] = "value"

	persist := NewPersist(store)
	ctx := context.Background()

	// Save
	if err := persist.Save(ctx, sess); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Load
	loaded, err := persist.Load(ctx, "test-123")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.ID != "test-123" {
		t.Errorf("ID = %q, want %q", loaded.ID, "test-123")
	}
	if loaded.Title != "My Session" {
		t.Errorf("Title = %q, want %q", loaded.Title, "My Session")
	}
	if loaded.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", loaded.Status, StatusRunning)
	}
	if v, ok := loaded.Metadata["key"]; !ok || v != "value" {
		t.Errorf("Metadata[key] = %v, want %q", v, "value")
	}
}

func TestPersistListIDs(t *testing.T) {
	dir, err := os.MkdirTemp("", "session-list-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	appScope := di.NewAppScope("test")
	persist := NewPersist(store)
	ctx := context.Background()

	// Create multiple sessions
	for _, id := range []string{"s1", "s2", "s3"} {
		sess, _ := NewSession(id, "Session "+id, appScope)
		if err := persist.Save(ctx, sess); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	ids, err := persist.ListIDs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("got %d IDs, want 3", len(ids))
	}
}

func TestPersistDelete(t *testing.T) {
	dir, err := os.MkdirTemp("", "session-delete-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	appScope := di.NewAppScope("test")
	persist := NewPersist(store)
	ctx := context.Background()

	sess, _ := NewSession("del-1", "To Delete", appScope)
	_ = persist.Save(ctx, sess)

	if err := persist.Delete(ctx, "del-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = persist.Load(ctx, "del-1")
	if err == nil {
		t.Error("expected error loading deleted session")
	}
}

func TestHistoryAddAndLoad(t *testing.T) {
	dir, err := os.MkdirTemp("", "history-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	history := NewHistory(store)
	ctx := context.Background()
	sessID := "h-1"

	// Add messages
	_ = history.AddMessage(ctx, sessID, Message{Role: "user", Content: "Hello"})
	_ = history.AddMessage(ctx, sessID, Message{Role: "assistant", Content: "Hi there", Timestamp: time.Now()})

	// Load into fresh history
	h2 := NewHistory(store)
	if err := h2.Load(ctx, sessID); err != nil {
		t.Fatalf("load: %v", err)
	}

	msgs := h2.Messages()
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("msg[0] = %+v, want user/Hello", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Hi there" {
		t.Errorf("msg[1] = %+v, want assistant/Hi there", msgs[1])
	}
}

func TestHistoryTurns(t *testing.T) {
	dir, err := os.MkdirTemp("", "history-turns-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	history := NewHistory(store)
	ctx := context.Background()
	sessID := "turns-1"

	_ = history.AddMessage(ctx, sessID, Message{Role: "user", Content: "Q1"})
	_ = history.AddMessage(ctx, sessID, Message{Role: "assistant", Content: "A1"})
	_ = history.AddMessage(ctx, sessID, Message{Role: "user", Content: "Q2"})
	_ = history.AddMessage(ctx, sessID, Message{Role: "assistant", Content: "A2"})

	turns := history.Turns()
	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if turns[0].Prompt != "Q1" || turns[0].Response != "A1" {
		t.Errorf("turn[0] = %+v", turns[0])
	}
	if turns[1].Prompt != "Q2" || turns[1].Response != "A2" {
		t.Errorf("turn[1] = %+v", turns[1])
	}
}

func TestHistoryRemoveLastTurns(t *testing.T) {
	dir, err := os.MkdirTemp("", "history-remove-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	history := NewHistory(store)
	ctx := context.Background()
	sessID := "remove-1"

	_ = history.AddMessage(ctx, sessID, Message{Role: "user", Content: "Q1"})
	_ = history.AddMessage(ctx, sessID, Message{Role: "assistant", Content: "A1"})
	_ = history.AddMessage(ctx, sessID, Message{Role: "user", Content: "Q2"})
	_ = history.AddMessage(ctx, sessID, Message{Role: "assistant", Content: "A2"})

	if err := history.RemoveLastTurns(ctx, sessID, 1); err != nil {
		t.Fatalf("remove: %v", err)
	}

	turns := history.Turns()
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if turns[0].Prompt != "Q1" {
		t.Errorf("remaining turn = %+v", turns[0])
	}
}

func TestSessionStoreFork(t *testing.T) {
	dir, err := os.MkdirTemp("", "session-fork-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	appScope := di.NewAppScope("test")
	mgr := NewManager(appScope)
	ss := NewSessionStore(store, appScope)
	mgr.SetStore(ss)
	ctx := context.Background()

	// Create source session
	sess, _ := mgr.Create(ctx, "original", "Original Session")
	_ = ss.History().AddMessage(ctx, "original", Message{Role: "user", Content: "Hello"})
	_ = ss.History().AddMessage(ctx, "original", Message{Role: "assistant", Content: "World"})
	_ = ss.Save(ctx, sess)

	// Fork
	forked, err := ss.Fork(ctx, "original", "Forked Session", mgr)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}

	if forked.Title != "Forked Session" {
		t.Errorf("forked title = %q", forked.Title)
	}

	// Load forked messages
	if err := ss.History().Load(ctx, forked.ID); err != nil {
		t.Fatalf("load forked: %v", err)
	}
	msgs := ss.History().Messages()
	if len(msgs) != 2 {
		t.Errorf("forked messages = %d, want 2", len(msgs))
	}
}

func TestPurgeEmptySessions(t *testing.T) {
	dir, err := os.MkdirTemp("", "session-purge-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	appScope := di.NewAppScope("test")
	mgr := NewManager(appScope)
	ss := NewSessionStore(store, appScope)
	mgr.SetStore(ss)
	ctx := context.Background()

	// Create 3 sessions: one empty, one with user messages, one with only assistant messages
	empty, _ := mgr.Create(ctx, "empty", "Empty Session")
	_ = ss.Save(ctx, empty)

	withUser, _ := mgr.Create(ctx, "with-user", "With User")
	_ = ss.History().AddMessage(ctx, "with-user", Message{Role: "user", Content: "Hello"})
	_ = ss.History().AddMessage(ctx, "with-user", Message{Role: "assistant", Content: "Hi"})
	_ = ss.Save(ctx, withUser)

	onlyAssistant, _ := mgr.Create(ctx, "only-assistant", "Only Assistant")
	_ = ss.History().AddMessage(ctx, "only-assistant", Message{Role: "assistant", Content: "Solo"})
	_ = ss.Save(ctx, onlyAssistant)

	// Purge empty sessions
	if err := ss.PurgeEmptySessions(ctx); err != nil {
		t.Fatalf("purge: %v", err)
	}

	// Verify: only "with-user" should remain
	remaining, err := ss.ListAll(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("got %d sessions, want 1", len(remaining))
	}
	if len(remaining) > 0 && remaining[0].ID != "with-user" {
		t.Errorf("remaining session ID = %q, want 'with-user'", remaining[0].ID)
	}
}
