package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/skill"
	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
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

// TestHandleSubmit_SkillRoutesThroughLLM verifies that /skill:name args
// routes the skill body through runLLMStream() instead of just displaying it.
func TestHandleSubmit_SkillRoutesThroughLLM(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{
			Name:        "test-skill",
			Description: "A test skill",
			Body:        "# Test Skill\nFollow these instructions carefully.",
		},
	})

	m := tuiModel{
		input:        "/skill:test-skill do something useful",
		cursor:       40,
		skillCatalog: cat,
	}

	result, cmd := m.handleSubmit()
	rm := result.(tuiModel)

	if !rm.streaming {
		t.Error("expected streaming to be true after skill invocation")
	}
	if cmd == nil {
		t.Error("expected non-nil tea.Cmd (should route through runLLMStream)")
	}
	if rm.turnCount != 1 {
		t.Errorf("turnCount = %d, want 1", rm.turnCount)
	}
	// Verify a "Skill loaded" system message was added
	found := false
	for _, msg := range rm.messages {
		if msg.role == "system" && strings.Contains(msg.content, "Skill loaded: test-skill") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Skill loaded: test-skill' system message")
	}
}

// TestNewTUIModel_UsesDefaultModel verifies that the TUI model field is set
// from DefaultModel in config, not DefaultProvider. This ensures the CLI
// startup Model matches the default_model defined in ~/.kimi-code/config.toml.
func TestNewTUIModel_UsesDefaultModel(t *testing.T) {
	tests := []struct {
		name            string
		defaultModel    string
		defaultProvider string
		wantModel       string
	}{
		{
			name:            "default_model set to a specific alias",
			defaultModel:    "kimi-code/k3-256k",
			defaultProvider: "managed:kimi-code",
			wantModel:       "kimi-code/k3-256k",
		},
		{
			name:            "default_model empty, falls back to defaultProvider",
			defaultModel:    "",
			defaultProvider: "kimi",
			wantModel:       "kimi",
		},
		{
			name:            "both empty, falls back to hardcoded default",
			defaultModel:    "",
			defaultProvider: "",
			wantModel:       "kimi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.DefaultModel = tt.defaultModel
			cfg.DefaultProvider = tt.defaultProvider

			app := &App{Config: cfg}
			sess, _ := session.NewSession("test-session", "Test", di.NewAppScope("test"))

			m := newTUIModel(app, sess)

			if m.model != tt.wantModel {
				t.Errorf("model = %q, want %q", m.model, tt.wantModel)
			}
		})
	}
}

// TestHandleSubmit_SubSkillRoutesThroughLLM verifies that /subskill-name args
// routes the sub-skill body through runLLMStream().
func TestHandleSubmit_SubSkillRoutesThroughLLM(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{
			Name:        "review.slop",
			Description: "Sub-skill for slop review",
			IsSubSkill:  true,
			Body:        "# Slop Review\nCheck for slop patterns.",
		},
	})

	m := tuiModel{
		input:        "/review.slop check this code",
		cursor:       30,
		skillCatalog: cat,
	}

	result, cmd := m.handleSubmit()
	rm := result.(tuiModel)

	if !rm.streaming {
		t.Error("expected streaming to be true after sub-skill invocation")
	}
	if cmd == nil {
		t.Error("expected non-nil tea.Cmd for sub-skill")
	}
}
