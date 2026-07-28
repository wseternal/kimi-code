package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGoGraphHook_WrapGrep(t *testing.T) {
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)

	original := &stubTool{name: "Grep", output: "original"}
	wrapped := hook.Wrap("Grep", original)

	if wrapped == nil {
		t.Fatal("Wrap(Grep) returned nil, want non-nil")
	}
	if wrapped.Definition().Name != "Grep" {
		t.Errorf("wrapped tool name = %q, want %q", wrapped.Definition().Name, "Grep")
	}
}

func TestGoGraphHook_PassThroughNonGrep(t *testing.T) {
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)

	original := &stubTool{name: "Bash", output: "original"}
	wrapped := hook.Wrap("Bash", original)

	if wrapped != nil {
		t.Error("Wrap(Bash) returned non-nil, want nil (passthrough)")
	}
}

func TestGoGraphHook_Name(t *testing.T) {
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)

	if hook.Name() != "gograph" {
		t.Errorf("Name() = %q, want %q", hook.Name(), "gograph")
	}
}

func TestGoEnhancedGrep_NonSymbol(t *testing.T) {
	// Pattern that doesn't look like a Go symbol should go to original grep
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)
	original := &stubTool{name: "Grep", output: "original grep result"}
	wrapped := hook.Wrap("Grep", original)

	input, _ := json.Marshal(map[string]string{"pattern": "lowercase_pattern"})
	result, err := wrapped.Execute(context.Background(), input, ExecContext{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "original grep result" {
		t.Errorf("output = %q, want %q", result.Output, "original grep result")
	}
}

func TestGoEnhancedGrep_RegexPattern(t *testing.T) {
	// Regex patterns should go to original grep
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)
	original := &stubTool{name: "Grep", output: "regex result"}
	wrapped := hook.Wrap("Grep", original)

	input, _ := json.Marshal(map[string]string{"pattern": "func.*Error"})
	result, err := wrapped.Execute(context.Background(), input, ExecContext{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "regex result" {
		t.Errorf("output = %q, want %q", result.Output, "regex result")
	}
}

func TestGoEnhancedGrep_FallbackOnNotGoProject(t *testing.T) {
	// Go symbol pattern but not a Go project should fall through to original
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)
	original := &stubTool{name: "Grep", output: "fallback result"}
	wrapped := hook.Wrap("Grep", original)

	// Use a temp dir without go.mod
	nonGoDir := t.TempDir()
	goProjectCache.Delete(nonGoDir) // ensure clean cache

	input, _ := json.Marshal(map[string]string{"pattern": "MyFunc"})
	result, err := wrapped.Execute(context.Background(), input, ExecContext{WorkDir: nonGoDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall through to original grep since not a Go project
	if result.Output != "fallback result" {
		t.Errorf("output = %q, want %q", result.Output, "fallback result")
	}
}

func TestGoEnhancedGrep_FallbackOnInvalidJSON(t *testing.T) {
	// Invalid JSON should delegate to original
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)
	original := &stubTool{name: "Grep", output: "original on error"}
	wrapped := hook.Wrap("Grep", original)

	result, err := wrapped.Execute(context.Background(), json.RawMessage(`{invalid`), ExecContext{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "original on error" {
		t.Errorf("output = %q, want %q", result.Output, "original on error")
	}
}

func TestGoEnhancedGrep_DefinitionPassthrough(t *testing.T) {
	// The enhanced tool should return the original's definition
	runner := NewGoGraphRunner()
	hook := NewGoGraphHook(runner)
	original := &stubTool{name: "Grep", output: "x"}
	wrapped := hook.Wrap("Grep", original)

	def := wrapped.Definition()
	if def.Name != "Grep" {
		t.Errorf("Definition().Name = %q, want %q", def.Name, "Grep")
	}
}

func TestGoSymbolPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"MyFunc", true},
		{"HTTPClient", true},
		{"pkg.SomeType", true},
		{"lowercase", false},
		{"func.*Error", false},
		{"123Bad", false},
		{"", false},
		{"pkg.lower", false},
		{"A", true},
		{"a1b2C3", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := goSymbolPattern.MatchString(tt.pattern)
			if got != tt.want {
				t.Errorf("goSymbolPattern.MatchString(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}
