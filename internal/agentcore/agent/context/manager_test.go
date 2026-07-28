package context

import (
	"strings"
	"testing"
)

func TestTokenEstimate(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 2},          // 5 chars → (5+3)/4 = 2
		{"hello world", 3},    // 11 chars → (11+3)/4 = 3
		{"abcdefgh", 2},       // 8 chars → (8+3)/4 = 2
		{"abcdefghijkl", 3},   // 12 chars → (12+3)/4 = 3
	}
	for _, tt := range tests {
		got := TokenEstimate(tt.input)
		if got != tt.want {
			t.Errorf("TokenEstimate(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestContextManager(t *testing.T) {
	cm := NewContextManager(1000)

	if cm.MaxTokens() != 1000 {
		t.Errorf("MaxTokens = %d, want 1000", cm.MaxTokens())
	}
	if cm.CurrentUsage() != 0 {
		t.Errorf("CurrentUsage = %d, want 0", cm.CurrentUsage())
	}

	cm.AddTurnUsage(100)
	cm.AddTurnUsage(200)

	if cm.CurrentUsage() != 300 {
		t.Errorf("CurrentUsage = %d, want 300", cm.CurrentUsage())
	}
	if cm.UsagePercent() != 30.0 {
		t.Errorf("UsagePercent = %.1f, want 30.0", cm.UsagePercent())
	}

	cm.RemoveLastNTurns(1)
	if cm.CurrentUsage() != 100 {
		t.Errorf("after remove, CurrentUsage = %d, want 100", cm.CurrentUsage())
	}

	cm.Reset()
	if cm.CurrentUsage() != 0 {
		t.Errorf("after reset, CurrentUsage = %d, want 0", cm.CurrentUsage())
	}
}

func TestContextManagerNeedsCompaction(t *testing.T) {
	cm := NewContextManager(100)
	cm.AddTurnUsage(70)

	if cm.NeedsCompaction(0.8) {
		t.Error("should not need compaction at 70%")
	}

	cm.AddTurnUsage(20)
	if !cm.NeedsCompaction(0.8) {
		t.Error("should need compaction at 90%")
	}
}

func TestCompactMessages(t *testing.T) {
	// Use long messages so compaction actually reduces token count
	longUser1 := strings.Repeat("What is Go and how does it compare to other languages like Rust and Python? ", 5)
	longAsst1 := strings.Repeat("Go is a programming language created by Google focusing on simplicity and concurrency. ", 5)
	longUser2 := strings.Repeat("What is Rust and what are its main features for systems programming? ", 5)
	longAsst2 := strings.Repeat("Rust is a systems programming language focused on safety and performance. ", 5)
	messages := []CompactMessage{
		{Role: "user", Content: longUser1},
		{Role: "assistant", Content: longAsst1},
		{Role: "user", Content: longUser2},
		{Role: "assistant", Content: longAsst2},
		{Role: "user", Content: "Compare them"},
		{Role: "assistant", Content: "Go focuses on simplicity, Rust on safety."},
	}

	result, err := CompactMessages(messages, 1)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.RemovedTurns != 2 {
		t.Errorf("RemovedTurns = %d, want 2", result.RemovedTurns)
	}
	if result.KeptTurns != 1 {
		t.Errorf("KeptTurns = %d, want 1", result.KeptTurns)
	}
	if result.Summary == "" {
		t.Error("Summary should not be empty")
	}
	if result.CompactTokens >= result.OriginalTokens {
		t.Errorf("compaction did not reduce tokens: %d >= %d", result.CompactTokens, result.OriginalTokens)
	}
}

func TestCompactMessagesNotEnoughTurns(t *testing.T) {
	messages := []CompactMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}

	_, err := CompactMessages(messages, 3)
	if err == nil {
		t.Error("expected error for not enough turns")
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{500, "500"},
		{1500, "1.5K"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := FormatTokenCount(tt.input)
		if got != tt.want {
			t.Errorf("FormatTokenCount(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
