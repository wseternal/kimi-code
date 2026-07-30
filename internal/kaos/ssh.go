// Package kaos provides SSH remote execution environment (Gap #63).
package kaos

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SSHConfig holds SSH connection configuration.
type SSHConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	PrivateKey string `json:"private_key,omitempty"`
	Password   string `json:"password,omitempty"`
}

// SSHKaos implements the Kaos interface over SSH/SFTP.
type SSHKaos struct {
	config    SSHConfig
	connected bool
	mu        sync.Mutex
	wd        string
}

// NewSSHKaos creates an SSH Kaos instance.
func NewSSHKaos(cfg SSHConfig) *SSHKaos {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	return &SSHKaos{
		config: cfg,
		wd:     "/",
	}
}

// Connect establishes the SSH connection.
func (s *SSHKaos) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// In production, this would use golang.org/x/crypto/ssh
	s.connected = true
	return nil
}

// Close closes the SSH connection.
func (s *SSHKaos) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = false
	return nil
}

// IsConnected returns whether the SSH connection is active.
func (s *SSHKaos) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

// ReadFile reads a file from the remote host.
func (s *SSHKaos) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if !s.IsConnected() {
		return nil, fmt.Errorf("ssh: not connected")
	}
	// Placeholder — would use SFTP client
	return nil, fmt.Errorf("ssh: ReadFile not yet connected to real SSH client")
}

// WriteFile writes a file to the remote host.
func (s *SSHKaos) WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	if !s.IsConnected() {
		return fmt.Errorf("ssh: not connected")
	}
	return fmt.Errorf("ssh: WriteFile not yet connected to real SSH client")
}

// Stat returns file info on the remote host.
func (s *SSHKaos) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	if !s.IsConnected() {
		return nil, fmt.Errorf("ssh: not connected")
	}
	return nil, fmt.Errorf("ssh: Stat not yet connected to real SSH client")
}

// MkdirAll creates directories recursively on the remote host.
func (s *SSHKaos) MkdirAll(ctx context.Context, path string, mode os.FileMode) error {
	if !s.IsConnected() {
		return fmt.Errorf("ssh: not connected")
	}
	return fmt.Errorf("ssh: MkdirAll not yet connected to real SSH client")
}

// Remove removes a file or directory on the remote host.
func (s *SSHKaos) Remove(ctx context.Context, path string) error {
	if !s.IsConnected() {
		return fmt.Errorf("ssh: not connected")
	}
	return fmt.Errorf("ssh: Remove not yet connected to real SSH client")
}

// Glob performs glob matching on the remote host.
func (s *SSHKaos) Glob(ctx context.Context, pattern string) ([]string, error) {
	if !s.IsConnected() {
		return nil, fmt.Errorf("ssh: not connected")
	}
	return nil, fmt.Errorf("ssh: Glob not yet connected to real SSH client")
}

// ExecResult holds the result of a remote command execution.
type ExecResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// Exec executes a command on the remote host.
func (s *SSHKaos) Exec(ctx context.Context, command string, args []string, stdin io.Reader) (*ExecResult, error) {
	if !s.IsConnected() {
		return nil, fmt.Errorf("ssh: not connected")
	}
	return nil, fmt.Errorf("ssh: Exec not yet connected to real SSH client")
}

// ExecStream executes a command with streaming output.
func (s *SSHKaos) ExecStream(ctx context.Context, command string, args []string, stdout, stderr io.Writer) (int, error) {
	if !s.IsConnected() {
		return -1, fmt.Errorf("ssh: not connected")
	}
	return -1, fmt.Errorf("ssh: ExecStream not yet connected to real SSH client")
}

// SetWorkingDir sets the working directory for subsequent operations.
func (s *SSHKaos) SetWorkingDir(dir string) {
	s.mu.Lock()
	s.wd = dir
	s.mu.Unlock()
}

// WorkingDir returns the current working directory.
func (s *SSHKaos) WorkingDir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wd
}

// ResolvePath resolves a path relative to the working directory.
func (s *SSHKaos) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(s.WorkingDir(), path)
}

// Walk walks a directory tree on the remote host.
func (s *SSHKaos) Walk(ctx context.Context, root string, fn func(path string, info os.FileInfo, err error) error) error {
	if !s.IsConnected() {
		return fmt.Errorf("ssh: not connected")
	}
	return fmt.Errorf("ssh: Walk not yet connected to real SSH client")
}

// String returns a description of the SSH Kaos.
func (s *SSHKaos) String() string {
	return fmt.Sprintf("SSHKaos(%s@%s:%d)", s.config.User, s.config.Host, s.config.Port)
}

// ParseSSHURL parses an SSH URL into SSHConfig.
func ParseSSHURL(url string) (SSHConfig, error) {
	// user@host:port format or ssh://user@host:port
	if strings.HasPrefix(url, "ssh://") {
		url = strings.TrimPrefix(url, "ssh://")
	}
	parts := strings.SplitN(url, "@", 2)
	if len(parts) != 2 {
		return SSHConfig{}, fmt.Errorf("invalid SSH URL: %s", url)
	}
	user := parts[0]
	hostPort := parts[1]
	host := hostPort
	port := 22
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		host = hostPort[:idx]
		fmt.Sscanf(hostPort[idx+1:], "%d", &port)
	}
	return SSHConfig{
		Host: host,
		Port: port,
		User: user,
	}, nil
}
