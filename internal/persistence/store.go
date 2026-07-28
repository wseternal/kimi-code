package persistence

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("persistence: key not found")

// Store is the primary key-value storage interface.
// Implementations must be safe for concurrent use.
type Store interface {
	// Get retrieves the value for a key. Returns ErrNotFound if absent.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value for a key, creating or overwriting as needed.
	Set(ctx context.Context, key string, value []byte) error

	// Del removes a key. Returns nil if the key did not exist.
	Del(ctx context.Context, key string) error

	// Has reports whether a key exists.
	Has(ctx context.Context, key string) (bool, error)

	// Keys returns all keys matching the given prefix, sorted lexicographically.
	// An empty prefix returns all keys.
	Keys(ctx context.Context, prefix string) ([]string, error)

	// Scan iterates over keys matching the prefix, calling fn for each.
	// If fn returns an error, iteration stops and the error is returned.
	Scan(ctx context.Context, prefix string, fn func(key string, value []byte) error) error

	// Close releases resources held by the store.
	Close() error
}

// FileStore implements Store using one file per key under a root directory.
// Keys are mapped to file paths by replacing "/" with the OS path separator.
type FileStore struct {
	root string
	mu   sync.RWMutex
}

// NewFileStore creates a new FileStore rooted at the given directory.
// The directory is created if it does not exist.
func NewFileStore(root string) (*FileStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("persistence: create root dir: %w", err)
	}
	return &FileStore{root: root}, nil
}

// keyToPath maps a logical key to a filesystem path.
func (s *FileStore) keyToPath(key string) string {
	return filepath.Join(s.root, filepath.FromSlash(key))
}

func (s *FileStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.keyToPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return data, err
}

func (s *FileStore) Set(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.keyToPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("persistence: mkdir for %s: %w", key, err)
	}
	// Write to temp file, then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, value, 0o644); err != nil {
		return fmt.Errorf("persistence: write %s: %w", key, err)
	}
	return os.Rename(tmp, path)
}

func (s *FileStore) Del(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.keyToPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s *FileStore) Has(_ context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, err := os.Stat(s.keyToPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *FileStore) Keys(_ context.Context, prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var keys []string
	err := filepath.Walk(s.root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *FileStore) Scan(ctx context.Context, prefix string, fn func(string, []byte) error) error {
	keys, err := s.Keys(ctx, prefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		data, err := s.Get(ctx, key)
		if err != nil {
			return err
		}
		if err := fn(key, data); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileStore) Close() error {
	return nil // FileStore holds no persistent resources
}
