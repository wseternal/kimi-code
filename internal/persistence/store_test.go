package persistence

import (
	"context"
	"errors"
	"os"
	"testing"
)

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestFileStoreGetSetDel(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Get nonexistent key
	_, err := store.Get(ctx, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Set and Get
	if err := store.Set(ctx, "key1", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	data, err := store.Get(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}

	// Overwrite
	if err := store.Set(ctx, "key1", []byte("world")); err != nil {
		t.Fatal(err)
	}
	data, _ = store.Get(ctx, "key1")
	if string(data) != "world" {
		t.Errorf("expected 'world', got %q", string(data))
	}

	// Has
	has, _ := store.Has(ctx, "key1")
	if !has {
		t.Error("expected Has=true for key1")
	}
	has, _ = store.Has(ctx, "missing")
	if has {
		t.Error("expected Has=false for missing")
	}

	// Del
	if err := store.Del(ctx, "key1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(ctx, "key1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Del, got %v", err)
	}

	// Del nonexistent is no-op
	if err := store.Del(ctx, "key1"); err != nil {
		t.Fatalf("Del nonexistent should be nil, got %v", err)
	}
}

func TestFileStoreNestedKeys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Set(ctx, "sessions/s1/meta", []byte("meta1"))
	store.Set(ctx, "sessions/s1/messages/m1", []byte("msg1"))
	store.Set(ctx, "sessions/s2/meta", []byte("meta2"))
	store.Set(ctx, "config/global", []byte("cfg"))

	// Keys with prefix
	keys, err := store.Keys(ctx, "sessions/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys with sessions/ prefix, got %d: %v", len(keys), keys)
	}

	keys, err = store.Keys(ctx, "sessions/s1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys with sessions/s1/ prefix, got %d", len(keys))
	}

	// All keys
	keys, err = store.Keys(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 4 {
		t.Fatalf("expected 4 total keys, got %d: %v", len(keys), keys)
	}
}

func TestFileStoreScan(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Set(ctx, "a/1", []byte("v1"))
	store.Set(ctx, "a/2", []byte("v2"))
	store.Set(ctx, "a/3", []byte("v3"))

	var collected []string
	err := store.Scan(ctx, "a/", func(key string, value []byte) error {
		collected = append(collected, key+"="+string(value))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(collected) != 3 {
		t.Fatalf("expected 3 items, got %d", len(collected))
	}
	if collected[0] != "a/1=v1" {
		t.Errorf("expected a/1=v1, got %s", collected[0])
	}
}

func TestFileStoreScanEarlyStop(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Set(ctx, "x/1", []byte("v1"))
	store.Set(ctx, "x/2", []byte("v2"))
	store.Set(ctx, "x/3", []byte("v3"))

	stopErr := errors.New("stop")
	count := 0
	err := store.Scan(ctx, "x/", func(key string, value []byte) error {
		count++
		if count == 2 {
			return stopErr
		}
		return nil
	})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stopErr, got %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 iterations, got %d", count)
	}
}

func TestFileStoreAtomicWrite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Verify no .tmp files remain after successful writes
	store.Set(ctx, "test", []byte("data"))

	entries, _ := os.ReadDir(store.root)
	for _, e := range entries {
		if len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] == ".tmp" {
			t.Errorf("found leftover .tmp file: %s", e.Name())
		}
	}
}

func TestQueryPrefixAndLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		store.Set(ctx, "items/"+zeroPad(i, 3), []byte("v"))
	}

	// Prefix + limit
	results, err := NewQuery(store).Prefix("items/").Limit(3).Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Key != "items/000" {
		t.Errorf("expected first key items/000, got %s", results[0].Key)
	}
}

func TestQueryCursor(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Set(ctx, "a/1", []byte("v1"))
	store.Set(ctx, "a/2", []byte("v2"))
	store.Set(ctx, "a/3", []byte("v3"))

	results, err := NewQuery(store).Prefix("a/").After("a/1").Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results after cursor a/1, got %d", len(results))
	}
	if results[0].Key != "a/2" {
		t.Errorf("expected first result a/2, got %s", results[0].Key)
	}
}

func TestQueryReverse(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Set(ctx, "r/1", []byte("v1"))
	store.Set(ctx, "r/2", []byte("v2"))
	store.Set(ctx, "r/3", []byte("v3"))

	results, err := NewQuery(store).Prefix("r/").Reverse().Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Key != "r/3" {
		t.Errorf("expected first key r/3, got %s", results[0].Key)
	}
}

func TestAppendLog(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	log := NewAppendLog(store, "journal")

	seq1, err := log.Append(ctx, []byte("entry-1"))
	if err != nil {
		t.Fatal(err)
	}
	if seq1 != 1 {
		t.Errorf("expected seq 1, got %d", seq1)
	}

	seq2, _ := log.Append(ctx, []byte("entry-2"))
	seq3, _ := log.Append(ctx, []byte("entry-3"))

	if seq2 != 2 || seq3 != 3 {
		t.Errorf("expected seqs 2,3, got %d,%d", seq2, seq3)
	}

	entries, err := log.ReadAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if string(entries[0].Value) != "entry-1" {
		t.Errorf("expected first entry 'entry-1', got %q", string(entries[0].Value))
	}
}

func TestZeroPad(t *testing.T) {
	tests := []struct{ n, w int; want string }{
		{0, 10, "0000000000"},
		{1, 10, "0000000001"},
		{42, 5, "00042"},
		{12345, 3, "345"}, // overflow — takes rightmost digits
	}
	for _, tt := range tests {
		got := zeroPad(tt.n, tt.w)
		if got != tt.want {
			t.Errorf("zeroPad(%d, %d) = %q, want %q", tt.n, tt.w, got, tt.want)
		}
	}
}
