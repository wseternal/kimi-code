package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepTool(t *testing.T) {
	// Skip if rg (ripgrep) is not installed
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg (ripgrep) not found in $PATH, skipping TestGrepTool")
	}

	// Create test directory with test files
	tmpDir := t.TempDir()

	// Create test files
	writeTestFile(t, filepath.Join(tmpDir, "file1.txt"), "hello world\nfoo bar\nhello again")
	writeTestFile(t, filepath.Join(tmpDir, "file2.go"), "package main\nfunc hello() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "file3.txt"), "no matches here")

	tests := []struct {
		name       string
		pattern    string
		path       string
		glob       string
		wantMatch  bool
		wantFiles  []string // expected files to contain matches
		wantErrMsg string
	}{
		{
			name:      "simple pattern match",
			pattern:   "hello",
			path:      tmpDir,
			wantMatch: true,
			wantFiles: []string{"file1.txt", "file2.go"},
		},
		{
			name:      "no match",
			pattern:   "xyz123",
			path:      tmpDir,
			wantMatch: false,
		},
		{
			name:      "regex pattern",
			pattern:   "func.*hello",
			path:      tmpDir,
			wantMatch: true,
			wantFiles: []string{"file2.go"},
		},
		{
			name:      "glob filter",
			pattern:   "hello",
			path:      tmpDir,
			glob:      "*.txt",
			wantMatch: true,
			wantFiles: []string{"file1.txt"},
		},
		{
			name:       "invalid path",
			pattern:    "hello",
			path:       "/nonexistent/path",
			wantErrMsg: "no such file or directory",
		},
	}

	tool := NewGrepTool()
	ctx := context.Background()
	exec := ExecContext{WorkDir: tmpDir}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]interface{}{
				"pattern": tt.pattern,
				"path":    tt.path,
			}
			if tt.glob != "" {
				input["glob"] = tt.glob
			}
			inputJSON, _ := json.Marshal(input)

			result, err := tool.Execute(ctx, inputJSON, exec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErrMsg != "" {
				if !result.IsError {
					t.Fatalf("expected error result, got success")
				}
				if !strings.Contains(result.Output, tt.wantErrMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErrMsg, result.Output)
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error result: %s", result.Output)
			}

			hasMatch := result.Output != "" && result.Output != "No matches found"
			if hasMatch != tt.wantMatch {
				t.Fatalf("wantMatch=%v, got output: %s", tt.wantMatch, result.Output)
			}

			// Verify expected files are in output
			for _, wantFile := range tt.wantFiles {
				if !strings.Contains(result.Output, wantFile) {
					t.Errorf("expected output to contain %q, got: %s", wantFile, result.Output)
				}
			}
		})
	}
}

func TestEditTool(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		content    string
		oldString  string
		newString  string
		replaceAll bool
		want       string
		wantErr    string
	}{
		{
			name:      "simple replace",
			content:   "hello world",
			oldString: "world",
			newString: "gopher",
			want:      "hello gopher",
		},
		{
			name:      "replace first occurrence only",
			content:   "foo bar foo baz",
			oldString: "foo",
			newString: "qux",
			want:      "qux bar foo baz",
		},
		{
			name:       "replace all occurrences",
			content:    "foo bar foo baz",
			oldString:  "foo",
			newString:  "qux",
			replaceAll: true,
			want:       "qux bar qux baz",
		},
		{
			name:      "not found error",
			content:   "hello world",
			oldString: "xyz",
			newString: "abc",
			wantErr:   "not found",
		},
		{
			name:      "multiple matches replaces first",
			content:   "foo foo foo",
			oldString: "foo",
			newString: "bar",
			want:      "bar foo foo",
		},
		{
			name:      "same content error",
			content:   "hello",
			oldString: "hello",
			newString: "hello",
			wantErr:   "same",
		},
	}

	tool := NewEditTool()
	ctx := context.Background()
	exec := ExecContext{WorkDir: tmpDir}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh file for each test
			testFile := filepath.Join(tmpDir, "test_"+sanitize(tt.name)+".txt")
			if err := os.WriteFile(testFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			input := map[string]interface{}{
				"path":       testFile,
				"old_string": tt.oldString,
				"new_string": tt.newString,
			}
			if tt.replaceAll {
				input["replace_all"] = true
			}
			inputJSON, _ := json.Marshal(input)

			result, err := tool.Execute(ctx, inputJSON, exec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr != "" {
				if !result.IsError {
					t.Fatalf("expected error result containing %q, got success", tt.wantErr)
				}
				if !strings.Contains(strings.ToLower(result.Output), strings.ToLower(tt.wantErr)) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, result.Output)
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error result: %s", result.Output)
			}

			// Read file and verify content
			got, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("want %q, got %q", tt.want, string(got))
			}
		})
	}
}

func TestGlobTool_DoubleStar(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure
	dirs := []string{
		filepath.Join(tmpDir, "src", "pkg1"),
		filepath.Join(tmpDir, "src", "pkg2"),
		filepath.Join(tmpDir, "test"),
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}

	files := []string{
		filepath.Join(tmpDir, "src", "pkg1", "a.go"),
		filepath.Join(tmpDir, "src", "pkg1", "b.go"),
		filepath.Join(tmpDir, "src", "pkg2", "c.go"),
		filepath.Join(tmpDir, "src", "main.go"),
		filepath.Join(tmpDir, "test", "main_test.go"),
		filepath.Join(tmpDir, "README.md"),
	}
	for _, f := range files {
		os.WriteFile(f, []byte("test"), 0644)
	}

	tool := NewGlobTool()
	ctx := context.Background()
	exec := ExecContext{WorkDir: tmpDir}

	tests := []struct {
		name      string
		pattern   string
		wantCount int
		wantFiles []string
	}{
		{
			name:      "double star go files",
			pattern:   "**/*.go",
			wantCount: 5,
		},
		{
			name:      "single star go files in src",
			pattern:   "src/*.go",
			wantCount: 1,
			wantFiles: []string{"main.go"},
		},
		{
			name:      "double star in subdirectory",
			pattern:   "src/**/*.go",
			wantCount: 4,
		},
		{
			name:      "markdown files",
			pattern:   "*.md",
			wantCount: 1,
			wantFiles: []string{"README.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]interface{}{
				"pattern": tt.pattern,
				"path":    tmpDir,
			}
			inputJSON, _ := json.Marshal(input)

			result, err := tool.Execute(ctx, inputJSON, exec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.IsError {
				t.Fatalf("unexpected error result: %s", result.Output)
			}

			matches := strings.Split(strings.TrimSpace(result.Output), "\n")
			if result.Output == "" {
				matches = nil
			}

			if len(matches) != tt.wantCount {
				t.Errorf("want %d matches, got %d: %v", tt.wantCount, len(matches), matches)
			}

			for _, wantFile := range tt.wantFiles {
				found := false
				for _, m := range matches {
					if strings.Contains(m, wantFile) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q in matches: %v", wantFile, matches)
				}
			}
		})
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
}
