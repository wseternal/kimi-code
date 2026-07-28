package kaos

import (
	"context"
	"io"
	"io/fs"
)

// Kaos is the Kimi Agent Operating System interface.
// It provides a unified API for interacting with different execution environments.
type Kaos interface {
	// Name returns a human-readable name for this environment.
	Name() string
	// OsEnv returns the environment probe for this Kaos.
	OsEnv() Environment

	// ── Path operations ──

	// PathClass returns the path style ("posix" or "win32").
	PathClass() string
	// Normpath normalizes a path string.
	Normpath(path string) string
	// Gethome returns the home directory of the current user.
	Gethome() string
	// Getcwd returns the current working directory.
	Getcwd() string

	// ── Directory operations ──

	// Chdir changes the working directory.
	Chdir(ctx context.Context, path string) error
	// WithCwd returns a new Kaos with the given cwd.
	WithCwd(cwd string) Kaos
	// WithEnv returns a new Kaos that overlays env onto every spawned process.
	WithEnv(env map[string]string) Kaos
	// Stat returns stat metadata for path.
	Stat(ctx context.Context, path string, followSymlinks bool) (*StatResult, error)
	// Iterdir yields entry names in the directory at path.
	Iterdir(ctx context.Context, path string) (<-chan string, error)
	// Glob yields paths matching pattern under path.
	Glob(ctx context.Context, path, pattern string, caseSensitive bool) (<-chan string, error)

	// ── File operations ──

	// ReadBytes reads up to n bytes from path (all bytes if n <= 0).
	ReadBytes(ctx context.Context, path string, n int) ([]byte, error)
	// ReadText reads the file at path as a string.
	ReadText(ctx context.Context, path string, errors string) (string, error)
	// ReadLines yields lines from the file at path.
	ReadLines(ctx context.Context, path string, errors string) (<-chan string, error)
	// WriteBytes writes raw bytes to path, returning bytes written.
	WriteBytes(ctx context.Context, path string, data []byte) (int, error)
	// WriteText writes text to path, returning bytes written.
	WriteText(ctx context.Context, path, data string, mode string) (int, error)
	// Mkdir creates a directory at path.
	Mkdir(ctx context.Context, path string, parents, existOk bool) error

	// ── Process execution ──

	// Exec spawns a process with the given arguments.
	Exec(ctx context.Context, args ...string) (KaosProcess, error)
	// ExecWithEnv spawns a process with explicit environment variables.
	ExecWithEnv(ctx context.Context, args []string, env map[string]string) (KaosProcess, error)
}

// KaosProcess is a running process spawned by a Kaos environment.
type KaosProcess interface {
	// Stdin returns a writer connected to the process's standard input.
	Stdin() io.WriteCloser
	// Stdout returns a reader for the process's standard output.
	Stdout() io.ReadCloser
	// Stderr returns a reader for the process's standard error.
	Stderr() io.ReadCloser
	// Pid returns the operating-system process ID.
	Pid() int
	// ExitCode returns the exit code if terminated, or -1 if still running.
	ExitCode() int
	// Wait waits for the process to exit and returns its exit code.
	Wait(ctx context.Context) (int, error)
	// Kill sends a signal to the process (defaults to SIGTERM).
	Kill(ctx context.Context, signal string) error
	// Dispose releases stdin/stdout/stderr resources.
	Dispose() error
}

// StatResult mirrors Python's os.stat_result fields.
type StatResult struct {
	StMode  fs.FileMode `json:"stMode"`
	StIno   uint64      `json:"stIno"`
	StDev   uint64      `json:"stDev"`
	StNlink uint32      `json:"stNlink"`
	StUID   uint32      `json:"stUid"`
	StGID   uint32      `json:"stGid"`
	StSize  int64       `json:"stSize"`
	StAtime int64       `json:"stAtime"`
	StMtime int64       `json:"stMtime"`
	StCtime int64       `json:"stCtime"`
}

// OsKind is the OS platform kind.
type OsKind string

const (
	OsKindMacOS   OsKind = "macOS"
	OsKindLinux   OsKind = "Linux"
	OsKindWindows OsKind = "Windows"
)

// ShellName is the shell type.
type ShellName string

const (
	ShellBash ShellName = "bash"
	ShellSh   ShellName = "sh"
)

// Environment describes the OS and shell of a Kaos environment.
type Environment struct {
	OsKind    OsKind    `json:"osKind"`
	OsArch    string    `json:"osArch"`
	OsVersion string    `json:"osVersion"`
	ShellName ShellName `json:"shellName"`
	ShellPath string    `json:"shellPath"`
}
