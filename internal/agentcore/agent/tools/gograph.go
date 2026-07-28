package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	// gographOutputLimit is the maximum bytes captured from gograph stdout.
	gographOutputLimit = 32 * 1024 // 32 KiB

	// gographDefaultTimeout is the default per-command timeout.
	gographDefaultTimeout = 30 * time.Second

	// gographBuildTimeout is the timeout for graph build operations.
	gographBuildTimeout = 120 * time.Second
)

// ── Project Detection ─────────────────────────────────────────────────

var (
	goProjectCache sync.Map // map[string]bool: workDir → isGoProject
	gographOnce    sync.Once
	gographPath    string
	gographFound   bool
)

// IsGoProject reports whether workDir (or an ancestor) contains a go.mod file.
// Results are cached per workDir.
func IsGoProject(workDir string) bool {
	if workDir == "" {
		return false
	}
	if v, ok := goProjectCache.Load(workDir); ok {
		return v.(bool)
	}
	result := findGoMod(workDir)
	goProjectCache.Store(workDir, result)
	return result
}

// findGoMod walks from dir upward looking for go.mod.
func findGoMod(dir string) bool {
	dir = filepath.Clean(dir)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// IsGoGraphAvailable reports whether the gograph binary is on $PATH.
// The result is computed once and cached for the process lifetime.
func IsGoGraphAvailable() bool {
	gographOnce.Do(func() {
		p, err := exec.LookPath("gograph")
		if err == nil {
			gographPath = p
			gographFound = true
		}
	})
	return gographFound
}

// GoGraphBinaryPath returns the cached gograph binary path, or "" if not found.
func GoGraphBinaryPath() string {
	IsGoGraphAvailable() // ensure lookup ran
	return gographPath
}

// ── CLI Runner ─────────────────────────────────────────────────────────

// GoGraphRunner wraps exec calls to the gograph CLI.
type GoGraphRunner struct {
	binaryPath string
	timeout    time.Duration
}

// NewGoGraphRunner creates a runner using the cached gograph binary path.
// Returns a runner even if gograph is unavailable (calls will return errors).
func NewGoGraphRunner() *GoGraphRunner {
	return &GoGraphRunner{
		binaryPath: GoGraphBinaryPath(),
		timeout:    gographDefaultTimeout,
	}
}

// Run executes a gograph subcommand and returns its stdout as a string.
// Stderr is captured and included in the error on failure.
// Output is truncated to gographOutputLimit bytes.
func (r *GoGraphRunner) Run(ctx context.Context, workDir string, args ...string) (string, error) {
	if r.binaryPath == "" {
		return "", fmt.Errorf("gograph binary not found on $PATH")
	}

	// Only apply the default timeout if the caller hasn't set one.
	// This lets EnsureGraph pass a longer deadline for slow builds.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, r.binaryPath, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{buf: &stdout, limit: gographOutputLimit}
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out, fmt.Errorf("gograph timed out")
		}
		// Exit code 1 often means "no results" for query commands — not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return out, nil
		}
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("gograph error: %s", msg)
	}

	return out, nil
}

// RunJSON executes a gograph subcommand with --json and returns the output.
func (r *GoGraphRunner) RunJSON(ctx context.Context, workDir string, subcmd string, args ...string) (string, error) {
	fullArgs := make([]string, 0, 2+len(args))
	fullArgs = append(fullArgs, subcmd)
	fullArgs = append(fullArgs, "--json")
	fullArgs = append(fullArgs, args...)
	return r.Run(ctx, workDir, fullArgs...)
}

// IsGraphBuilt reports whether a gograph graph exists for the given workDir.
func (r *GoGraphRunner) IsGraphBuilt(workDir string) bool {
	if workDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(workDir, ".gograph", "graph.json"))
	return err == nil
}

// EnsureGraph runs "gograph build ." if no graph exists yet.
// Uses a longer timeout since graph building can be slow.
func (r *GoGraphRunner) EnsureGraph(ctx context.Context, workDir string) error {
	if r.IsGraphBuilt(workDir) {
		return nil
	}

	buildCtx, cancel := context.WithTimeout(ctx, gographBuildTimeout)
	defer cancel()

	_, err := r.Run(buildCtx, workDir, "build", ".")
	return err
}

// PrecisionMode returns "precise" or "ast" based on how the graph was built.
// Returns "" if the graph is not built or stats cannot be read.
func (r *GoGraphRunner) PrecisionMode(ctx context.Context, workDir string) string {
	out, err := r.RunJSON(ctx, workDir, "stats")
	if err != nil || out == "" {
		return ""
	}
	// Quick string check to avoid a full JSON parse for a display tag.
	if contains(out, `"precise"`) || contains(out, `"precise_fallback"`) {
		return "precise"
	}
	if contains(out, `"ast"`) {
		return "ast"
	}
	return ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// limitWriter wraps a bytes.Buffer and silently drops writes beyond the limit.
type limitWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // silently discard
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, err := w.buf.Write(p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
