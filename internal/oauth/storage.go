package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TokenStorage defines the interface for OAuth token persistence.
type TokenStorage interface {
	Load(ctx context.Context, name string) (*TokenInfo, error)
	Save(ctx context.Context, name string, token *TokenInfo) error
	Remove(ctx context.Context, name string) error
	List(ctx context.Context) ([]string, error)
}

// FileTokenStorage implements TokenStorage using files with atomic writes.
type FileTokenStorage struct {
	dir string
}

// NewFileTokenStorage creates a new FileTokenStorage at the given directory.
func NewFileTokenStorage(dir string) *FileTokenStorage {
	return &FileTokenStorage{dir: dir}
}

// NewDefaultTokenStorage creates a FileTokenStorage at ~/.kimi-code/credentials.
func NewDefaultTokenStorage() (*FileTokenStorage, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewFileTokenStorage(filepath.Join(home, ".kimi-code", "credentials")), nil
}

// ensureDir creates the storage directory with proper permissions.
func (s *FileTokenStorage) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create credentials dir: %w", err)
	}
	// Tighten permissions on existing directory
	if err := os.Chmod(s.dir, 0o700); err != nil && !os.IsPermission(err) {
		// Best-effort on Windows or read-only FS
	}
	return nil
}

// pathFor returns the file path for a token name, validating against path traversal.
func (s *FileTokenStorage) pathFor(name string) (string, error) {
	safe := filepath.Base(name)
	if safe == "" || safe != name || strings.HasPrefix(safe, ".") {
		return "", fmt.Errorf("invalid token name: %q", name)
	}
	return filepath.Join(s.dir, safe+".json"), nil
}

// Load reads a token from storage. Returns nil, nil if not found or corrupt JSON.
// Non-NotExist I/O errors are propagated to the caller.
func (s *FileTokenStorage) Load(_ context.Context, name string) (*TokenInfo, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token file: %w", err)
	}

	var wire tokenInfoWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, nil // Corrupt JSON, treat as missing
	}

	return fromWire(&wire), nil
}

// Save writes a token to storage with atomic semantics.
func (s *FileTokenStorage) Save(_ context.Context, name string, token *TokenInfo) error {
	if err := s.ensureDir(); err != nil {
		return err
	}

	path, err := s.pathFor(name)
	if err != nil {
		return err
	}

	// Generate random suffix for temp file
	randBytes := make([]byte, 4)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("generate random suffix: %w", err)
	}
	tmpPath := fmt.Sprintf("%s.tmp.%d.%s", path, os.Getpid(), hex.EncodeToString(randBytes))

	// Marshal to JSON
	data, err := json.MarshalIndent(token.toWire(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	data = append(data, '\n')

	// Write to temp file with restricted permissions
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	// fsync to ensure data is on disk before rename
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	// Ensure permissions are correct (umask may have stripped bits)
	if err := os.Chmod(tmpPath, 0o600); err != nil && !os.IsPermission(err) {
		// Best-effort
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}

	// fsync parent directory to persist the rename
	if dir, err := os.Open(s.dir); err == nil {
		dir.Sync()
		dir.Close()
	}

	return nil
}

// Remove deletes a token from storage.
func (s *FileTokenStorage) Remove(_ context.Context, name string) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove token: %w", err)
	}
	return nil
}

// List returns all token names in storage.
func (s *FileTokenStorage) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read credentials dir: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			names = append(names, strings.TrimSuffix(name, ".json"))
		}
	}
	return names, nil
}
