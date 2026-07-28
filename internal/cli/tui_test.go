package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// handleKey is a helper that calls handleKey and type-asserts the result back to tuiModel.
func handleKeyHelper(m tuiModel, msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	result, cmd := m.handleKey(msg)
	return result.(tuiModel), cmd
}

// TestHandleKey_CursorExceedsInput reproduces the panic at tui.go:792:
// "slice bounds out of range [6:0]" — triggered when the cursor position
// exceeds the input length (e.g., after clearing input without resetting cursor).
func TestHandleKey_CursorExceedsInput(t *testing.T) {
	// Simulate the post-model-switch state: input cleared, cursor stale.
	m := tuiModel{
		input:  "",
		cursor: 6, // stale cursor from before input was cleared
	}

	// Typing a rune should NOT panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleKey panicked on KeyRunes with cursor > len(input): %v", r)
		}
	}()
	m, _ = handleKeyHelper(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if m.input != "a" {
		t.Errorf("after typing 'a', input = %q, want %q", m.input, "a")
	}
	if m.cursor != 1 {
		t.Errorf("after typing 'a', cursor = %d, want 1", m.cursor)
	}
}

// TestHandleKey_SpaceWithStaleCursor verifies KeySpace also handles
// cursor > len(input) gracefully.
func TestHandleKey_SpaceWithStaleCursor(t *testing.T) {
	m := tuiModel{
		input:  "",
		cursor: 3,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleKey panicked on KeySpace with cursor > len(input): %v", r)
		}
	}()
	m, _ = handleKeyHelper(m, tea.KeyMsg{Type: tea.KeySpace})

	if m.input != " " {
		t.Errorf("after typing space, input = %q, want %q", m.input, " ")
	}
}

// TestHandleKey_BackspaceWithStaleCursor verifies KeyBackspace handles
// cursor > len(input) gracefully.
func TestHandleKey_BackspaceWithStaleCursor(t *testing.T) {
	m := tuiModel{
		input:  "",
		cursor: 5,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleKey panicked on KeyBackspace with cursor > len(input): %v", r)
		}
	}()
	m, _ = handleKeyHelper(m, tea.KeyMsg{Type: tea.KeyBackspace})
	// Should not panic; cursor should be clamped.
}

// TestHandleKey_MultipleRunesAfterClear verifies typing multiple runes
// works correctly after a stale cursor state.
func TestHandleKey_MultipleRunesAfterClear(t *testing.T) {
	m := tuiModel{
		input:  "",
		cursor: 10,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleKey panicked typing runes after clear: %v", r)
		}
	}()

	for _, r := range "hello" {
		m, _ = handleKeyHelper(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if m.input != "hello" {
		t.Errorf("input = %q, want %q", m.input, "hello")
	}
}

// TestParseSkillCommand verifies that /skill:name with arguments correctly
// separates the skill name from the arguments. This reproduces the bug where
// "/skill:interview-me how to improve" tried to look up a skill named
// "interview-me how to improve" instead of "interview-me".
func TestParseSkillCommand(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs string
	}{
		{"/skill:interview-me", "interview-me", ""},
		{"/skill:interview-me how to improve the go kimi cli", "interview-me", "how to improve the go kimi cli"},
		{"/skill:dev-cycle", "dev-cycle", ""},
		{"/skill:dev-cycle --verbose", "dev-cycle", "--verbose"},
		{"/skill:my-skill arg1 arg2 arg3", "my-skill", "arg1 arg2 arg3"},
	}
	for _, tt := range tests {
		name, args := parseSkillCommand(tt.input)
		if name != tt.wantName {
			t.Errorf("parseSkillCommand(%q) name = %q, want %q", tt.input, name, tt.wantName)
		}
		if args != tt.wantArgs {
			t.Errorf("parseSkillCommand(%q) args = %q, want %q", tt.input, args, tt.wantArgs)
		}
	}
}
