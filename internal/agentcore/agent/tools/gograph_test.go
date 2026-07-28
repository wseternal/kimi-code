package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsGoProject(t *testing.T) {
	// Create a temp directory with go.mod
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Subdirectory should also be detected (parent walk)
	subDir := filepath.Join(tmpDir, "internal", "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		workDir string
		want    bool
	}{
		{"with go.mod", tmpDir, true},
		{"subdirectory of go.mod", subDir, true},
		{"empty string", "", false},
		{"temp dir without go.mod", t.TempDir(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache for each test to avoid cross-contamination
			goProjectCache.Delete(tt.workDir)
			got := IsGoProject(tt.workDir)
			if got != tt.want {
				t.Errorf("IsGoProject(%q) = %v, want %v", tt.workDir, got, tt.want)
			}
		})
	}
}

func TestIsGoGraphAvailable(t *testing.T) {
	// Reset the once for testability (can't truly reset sync.Once, so just
	// test the current state — it may be true or false depending on the env).
	available := IsGoGraphAvailable()
	t.Logf("gograph available: %v (binary: %q)", available, gographPath)

	// If gograph is not installed, just log and pass.
	if !available {
		t.Log("gograph not installed; skipping tests that require it")
	}
}

func TestGoGraphRunner_Run_GographMissing(t *testing.T) {
	runner := &GoGraphRunner{binaryPath: "/nonexistent/gograph", timeout: 5 * time.Second}
	_, err := runner.Run(context.Background(), t.TempDir(), "stats")
	if err == nil {
		t.Fatal("expected error when gograph binary is missing")
	}
}

func TestGoGraphRunner_Run_Timeout(t *testing.T) {
	// Use "sleep" as a stand-in binary that takes too long
	sleepBin, err := os.Executable() // use self as a slow binary
	if err != nil {
		// Fallback: use "sleep" from PATH
		sleepBin = "sleep"
	}

	runner := &GoGraphRunner{binaryPath: sleepBin, timeout: 100 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// sleep command that takes longer than timeout
	_, err = runner.Run(ctx, "", "10")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestGoGraphRunner_IsGraphBuilt(t *testing.T) {
	tmpDir := t.TempDir()

	runner := NewGoGraphRunner()

	// No graph yet
	if runner.IsGraphBuilt(tmpDir) {
		t.Error("IsGraphBuilt returned true for empty dir")
	}

	// Create .gograph/graph.json
	graphDir := filepath.Join(tmpDir, ".gograph")
	if err := os.MkdirAll(graphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "graph.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if !runner.IsGraphBuilt(tmpDir) {
		t.Error("IsGraphBuilt returned false after creating graph.json")
	}
}

func TestGoGraphRunner_IsGraphBuilt_EmptyWorkDir(t *testing.T) {
	runner := NewGoGraphRunner()
	if runner.IsGraphBuilt("") {
		t.Error("IsGraphBuilt should return false for empty workDir")
	}
}
