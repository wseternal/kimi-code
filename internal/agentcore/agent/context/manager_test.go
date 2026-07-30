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
	cm := NewContextManager(1000, 0.85, 50000)

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
	cm := NewContextManager(100, 0.8, 50000)
	cm.AddTurnUsage(70)

	if cm.NeedsCompaction() {
		t.Error("should not need compaction at 70%")
	}

	cm.AddTurnUsage(20)
	if !cm.NeedsCompaction() {
		t.Error("should need compaction at 90%")
	}
}

func TestTwoTierTracking(t *testing.T) {
	cm := NewContextManager(1000, 0.85, 50000)

	// Add measured turns
	cm.AddTurnUsage(200)
	cm.AddTurnUsage(300)
	if cm.CurrentUsage() != 500 {
		t.Errorf("CurrentUsage = %d, want 500", cm.CurrentUsage())
	}

	// Set pending estimate (simulates streaming)
	cm.SetPendingEstimate(150)
	if cm.CurrentUsage() != 650 {
		t.Errorf("CurrentUsage with pending = %d, want 650", cm.CurrentUsage())
	}

	// Commit turn: pending clears, measured increases
	cm.AddTurnUsage(150)
	if cm.CurrentUsage() != 650 {
		t.Errorf("CurrentUsage after commit = %d, want 650", cm.CurrentUsage())
	}

	// Pending should be cleared after AddTurnUsage
	cm.SetPendingEstimate(0)
	if cm.CurrentUsage() != 650 {
		t.Errorf("CurrentUsage after clearing pending = %d, want 650", cm.CurrentUsage())
	}
}

func TestNeedsCompactionReservedContext(t *testing.T) {
	// maxTokens=1000, triggerRatio=0.85, reservedContext=200
	cm := NewContextManager(1000, 0.85, 200)

	// At 750 tokens: ratio check 750 < 850 (no), reserved check 750+200=950 < 1000 (no)
	cm.AddTurnUsage(750)
	if cm.NeedsCompaction() {
		t.Error("should not need compaction at 750 with reserved=200")
	}

	// At 800 tokens: ratio check 800 < 850 (no), reserved check 800+200=1000 >= 1000 (yes)
	cm.AddTurnUsage(50)
	if !cm.NeedsCompaction() {
		t.Error("should need compaction at 800 with reserved=200 (800+200>=1000)")
	}
}

func TestNeedsCompactionRatioTrigger(t *testing.T) {
	// maxTokens=100, triggerRatio=0.85, reservedContext=50000 (default)
	// Ratio check fires at 85; reserved check fires at max(0, 100-50000)=0, always true.
	// Use large maxTokens so reserved check doesn't dominate.
	cm := NewContextManager(200000, 0.85, 50000)
	// Ratio fires at 170000; reserved fires at 150000. Reserved fires first.
	cm.AddTurnUsage(140000)
	if cm.NeedsCompaction() {
		t.Error("should not need compaction at 140000/200000 with reserved=50000")
	}
	// At 150000: reserved check fires (150000+50000 >= 200000)
	cm.AddTurnUsage(10000)
	if !cm.NeedsCompaction() {
		t.Error("should need compaction at 150000/200000 (reserved context exhausted)")
	}
}

func TestSetMaxTokensResetThresholds(t *testing.T) {
	cm := NewContextManager(1000, 0.85, 100)
	cm.AddTurnUsage(900)
	if !cm.NeedsCompaction() {
		t.Error("should need compaction at 900/1000")
	}

	// Expand window — same usage should no longer trigger
	cm.SetMaxTokens(2000)
	if cm.NeedsCompaction() {
		t.Error("should not need compaction at 900/2000")
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

func TestSetMaxTokens(t *testing.T) {
	cm := NewContextManager(1000, 0.85, 50000)
	cm.AddTurnUsage(500)

	// Verify initial state
	if cm.MaxTokens() != 1000 {
		t.Errorf("MaxTokens = %d, want 1000", cm.MaxTokens())
	}
	if cm.UsagePercent() != 50.0 {
		t.Errorf("UsagePercent = %.1f, want 50.0", cm.UsagePercent())
	}

	// Update max tokens — usage should stay the same, percent should change
	cm.SetMaxTokens(2000)
	if cm.MaxTokens() != 2000 {
		t.Errorf("after SetMaxTokens, MaxTokens = %d, want 2000", cm.MaxTokens())
	}
	if cm.CurrentUsage() != 500 {
		t.Errorf("after SetMaxTokens, CurrentUsage = %d, want 500", cm.CurrentUsage())
	}
	if cm.UsagePercent() != 25.0 {
		t.Errorf("after SetMaxTokens, UsagePercent = %.1f, want 25.0", cm.UsagePercent())
	}

	// Display should reflect the new max
	display := cm.UsageDisplay()
	if !strings.Contains(display, "2.0K") {
		t.Errorf("UsageDisplay = %q, expected to contain '2.0K'", display)
	}

	// SetMaxTokens(0) should fall back to default (262144)
	cm.SetMaxTokens(0)
	if cm.MaxTokens() != 262144 {
		t.Errorf("SetMaxTokens(0) → MaxTokens = %d, want 262144 (default)", cm.MaxTokens())
	}

	// SetMaxTokens(-100) should also fall back to default
	cm.SetMaxTokens(-100)
	if cm.MaxTokens() != 262144 {
		t.Errorf("SetMaxTokens(-100) → MaxTokens = %d, want 262144 (default)", cm.MaxTokens())
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
