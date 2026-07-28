package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

// TestUpdateSuggestions_SlashOnlyShowsBuiltinCommands verifies that typing
// just "/" shows only builtin commands, NOT skills. Skills flood the list
// with 40+ entries, making builtin commands hard to find.
func TestUpdateSuggestions_SlashOnlyShowsBuiltinCommands(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{Name: "dev-cycle", Description: "Dev cycle skill"},
		{Name: "pr-review", Description: "PR review skill"},
		{Name: "test-driven-development", Description: "TDD skill"},
	})

	m := tuiModel{
		input:        "/",
		skillCatalog: cat,
	}
	m.updateSuggestions()

	if !m.showSuggestions {
		t.Fatal("expected showSuggestions to be true")
	}

	// All suggestions should be builtin commands (no "skill:" prefix)
	for _, s := range m.suggestions {
		if strings.HasPrefix(s.name, "skill:") {
			t.Errorf("slash-only suggestions should NOT include skills, got %q", s.name)
		}
	}

	// Should include at least some builtin commands
	if len(m.suggestions) == 0 {
		t.Error("expected at least some builtin command suggestions")
	}
}

// TestUpdateSuggestions_SkillPrefixShowsSkills verifies that typing "/sk"
// or more includes matching skills in the suggestion list.
func TestUpdateSuggestions_SkillPrefixShowsSkills(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{Name: "dev-cycle", Description: "Dev cycle skill"},
		{Name: "pr-review", Description: "PR review skill"},
	})

	m := tuiModel{
		input:        "/skill:",
		skillCatalog: cat,
	}
	m.updateSuggestions()

	if !m.showSuggestions {
		t.Fatal("expected showSuggestions to be true")
	}

	// Should include skills when "skill:" prefix is typed
	hasSkill := false
	for _, s := range m.suggestions {
		if strings.HasPrefix(s.name, "skill:") {
			hasSkill = true
			break
		}
	}
	if !hasSkill {
		t.Error("expected skill suggestions when '/skill:' is typed")
	}
}

// TestUpdateSuggestions_PartialBuiltinMatches verifies that typing "/sess"
// shows matching builtin commands (like "sessions") but not skills.
func TestUpdateSuggestions_PartialBuiltinMatches(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{Name: "dev-cycle", Description: "Dev cycle skill"},
	})

	m := tuiModel{
		input:        "/sess",
		skillCatalog: cat,
	}
	m.updateSuggestions()

	if !m.showSuggestions {
		t.Fatal("expected showSuggestions to be true")
	}

	// Should match "sessions" but not any skill
	found := false
	for _, s := range m.suggestions {
		if s.name == "sessions" {
			found = true
		}
		if strings.HasPrefix(s.name, "skill:") {
			t.Errorf("partial builtin match should NOT include skills, got %q", s.name)
		}
	}
	if !found {
		t.Error("expected 'sessions' in suggestions for '/sess'")
	}
}

// TestFormatSessionList_IncludesIDAndTitle verifies that the session list
// display includes both the session ID (for `kimi -S <id>`) and the title.
func TestFormatSessionList_IncludesIDAndTitle(t *testing.T) {
	sessions := []*session.SerializedSession{
		{
			ID:        "session_1234567890",
			Title:     "How do I fix the login bug?",
			UpdatedAt: time.Now().Add(-2 * time.Hour),
			Metadata: map[string]any{
				"turns":     float64(5),
				"tokens_in": float64(1200),
				"tokens_out": float64(800),
			},
		},
	}

	result := formatSessionList(sessions)

	// Must contain the session ID (so user can copy it for kimi -S)
	if !strings.Contains(result, "session_1234567890") {
		t.Error("session list should contain the session ID")
	}

	// Must contain the title
	if !strings.Contains(result, "How do I fix the login bug?") {
		t.Error("session list should contain the session title")
	}
}

// TestFormatSessionList_TruncatesLongTitle verifies that excessively long
// titles are truncated for display readability.
func TestFormatSessionList_TruncatesLongTitle(t *testing.T) {
	longTitle := strings.Repeat("a", 100)
	sessions := []*session.SerializedSession{
		{
			ID:        "sess_abc",
			Title:     longTitle,
			UpdatedAt: time.Now(),
		},
	}

	result := formatSessionList(sessions)

	// The full 100-char title should NOT appear (it should be truncated)
	if strings.Contains(result, longTitle) {
		t.Error("session list should truncate long titles")
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

// TestRenderSuggestions_Pagination verifies that renderSuggestions limits
// visible items to 10 and shows a pagination indicator when there are more.
func TestRenderSuggestions_Pagination(t *testing.T) {
	// Create 15 suggestions (more than maxVisible=10)
	var suggestions []slashCommand
	for i := 0; i < 15; i++ {
		suggestions = append(suggestions, slashCommand{
			name: fmt.Sprintf("cmd%d", i),
			desc: fmt.Sprintf("Description %d", i),
		})
	}

	m := tuiModel{
		suggestions:    suggestions,
		selectedSuggest: 0,
	}

	out := m.renderSuggestions()

	// Count lines: should be 10 items + 1 pagination = 11 lines
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 11 {
		t.Errorf("expected 11 lines (10 items + pagination), got %d", len(lines))
	}

	// Last line should be pagination indicator
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "(1/15)") {
		t.Errorf("expected pagination '(1/15)', got %q", lastLine)
	}

	// Scroll to item 12, window should show items 3-12
	m.selectedSuggest = 12
	out = m.renderSuggestions()
	lines = strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	// Should still be 11 lines
	if len(lines) != 11 {
		t.Errorf("expected 11 lines after scroll, got %d", len(lines))
	}

	// First visible item should be cmd3 (index 3)
	if !strings.Contains(lines[0], "cmd3") {
		t.Errorf("expected first visible item 'cmd3', got %q", lines[0])
	}

	// Pagination should show (13/15) since selectedSuggest=12 → 12+1=13
	lastLine = lines[len(lines)-1]
	if !strings.Contains(lastLine, "(13/15)") {
		t.Errorf("expected pagination '(13/15)', got %q", lastLine)
	}
}

// TestRenderSuggestions_NoPaginationForFewItems verifies that pagination
// indicator is NOT shown when all items fit in the visible window.
func TestRenderSuggestions_NoPaginationForFewItems(t *testing.T) {
	suggestions := []slashCommand{
		{name: "help", desc: "Show help"},
		{name: "clear", desc: "Clear screen"},
	}

	m := tuiModel{
		suggestions:    suggestions,
		selectedSuggest: 0,
	}

	out := m.renderSuggestions()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	// Should be exactly 2 lines (no pagination)
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}

	// Should NOT contain pagination indicator
	if strings.Contains(out, "(") {
		t.Errorf("unexpected pagination indicator in output: %q", out)
	}
}

// TestFormatSessionList_NilMetadata verifies no panic when Metadata is nil.
func TestFormatSessionList_NilMetadata(t *testing.T) {
	sessions := []*session.SerializedSession{
		{
			ID:        "sess_nil_meta",
			Title:     "Session with no metadata",
			UpdatedAt: time.Now().Add(-30 * time.Minute),
		},
	}

	result := formatSessionList(sessions)

	if !strings.Contains(result, "sess_nil_meta") {
		t.Error("expected session ID in output")
	}
	if !strings.Contains(result, "Session with no metadata") {
		t.Error("expected title in output")
	}
}

// TestFormatSessionList_ZeroUpdatedAt verifies that zero-value UpdatedAt
// displays "unknown" rather than a misleading date like "Jan 01".
func TestFormatSessionList_ZeroUpdatedAt(t *testing.T) {
	sessions := []*session.SerializedSession{
		{
			ID:        "sess_zero_time",
			Title:     "Old session",
			UpdatedAt: time.Time{},
		},
	}

	result := formatSessionList(sessions)

	if !strings.Contains(result, "unknown") {
		t.Errorf("expected 'unknown' for zero UpdatedAt, got: %s", result)
	}
	if strings.Contains(result, "Jan 01") {
		t.Error("should not show misleading 'Jan 01' for zero UpdatedAt")
	}
}

// TestFormatSessionList_SanitizesTitleNewlines verifies that newlines and
// tabs in titles are replaced with spaces to prevent broken display.
func TestFormatSessionList_SanitizesTitleNewlines(t *testing.T) {
	sessions := []*session.SerializedSession{
		{
			ID:        "sess_dirty",
			Title:     "Fix the\nlogin\tbug",
			UpdatedAt: time.Now(),
		},
	}

	result := formatSessionList(sessions)

	if strings.Contains(result, "\nlogin") {
		t.Error("newlines in title should be replaced with spaces")
	}
	if strings.Contains(result, "\tbug") {
		t.Error("tabs in title should be replaced with spaces")
	}
}

// TestFormatSessionList_MaxDisplayCap verifies that only 20 sessions are
// displayed and the remainder is indicated with "... and N more".
func TestFormatSessionList_MaxDisplayCap(t *testing.T) {
	var sessions []*session.SerializedSession
	for i := 0; i < 25; i++ {
		sessions = append(sessions, &session.SerializedSession{
			ID:        fmt.Sprintf("sess_%d", i),
			Title:     fmt.Sprintf("Session %d", i),
			UpdatedAt: time.Now(),
		})
	}

	result := formatSessionList(sessions)

	if !strings.Contains(result, "... and 5 more") {
		t.Error("expected '... and 5 more' for 25 sessions")
	}
	// The 21st session should NOT appear
	if strings.Contains(result, "sess_20") {
		t.Error("session 21 (sess_20) should not be displayed")
	}
}

// TestFormatSessionList_MetaIntTypes verifies that metadata stored as
// int or int64 (not just float64) is correctly extracted.
func TestFormatSessionList_MetaIntTypes(t *testing.T) {
	sessions := []*session.SerializedSession{
		{
			ID:    "sess_int_meta",
			Title: "Int metadata",
			Metadata: map[string]any{
				"turns":      5,        // Go int
				"tokens_in":  int64(1200), // int64
				"tokens_out": float64(800), // JSON float64
			},
			UpdatedAt: time.Now(),
		},
	}

	result := formatSessionList(sessions)

	if !strings.Contains(result, "5 turns") {
		t.Errorf("expected '5 turns' for int metadata, got: %s", result)
	}
	if !strings.Contains(result, "1.2K in") {
		t.Errorf("expected '1.2K in' for int64 tokens_in, got: %s", result)
	}
	if !strings.Contains(result, "800 out") {
		t.Errorf("expected '800 out' for float64 tokens_out, got: %s", result)
	}
}

// TestRenderSuggestions_ExactlyTenItems verifies no pagination for exactly 10 items.
func TestRenderSuggestions_ExactlyTenItems(t *testing.T) {
	var suggestions []slashCommand
	for i := 0; i < 10; i++ {
		suggestions = append(suggestions, slashCommand{
			name: fmt.Sprintf("cmd%d", i),
			desc: fmt.Sprintf("Desc %d", i),
		})
	}

	m := tuiModel{
		suggestions:    suggestions,
		selectedSuggest: 0,
	}

	out := m.renderSuggestions()

	if strings.Contains(out, "(") {
		t.Errorf("10 items should not show pagination, got: %s", out)
	}

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}
}

// TestRenderSuggestions_OutOfBoundsSelection verifies clamping when
// selectedSuggest exceeds the list length.
func TestRenderSuggestions_OutOfBoundsSelection(t *testing.T) {
	suggestions := []slashCommand{
		{name: "a", desc: "A"},
		{name: "b", desc: "B"},
	}

	m := tuiModel{
		suggestions:    suggestions,
		selectedSuggest: 99, // way out of bounds
	}

	out := m.renderSuggestions()

	// Should not panic and should show last item as selected (with arrow)
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "b") && strings.Contains(line, "\u2192") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'b' to be the selected item (with \u2192 indicator)")
	}
}

// TestTruncateDesc_MultiByteUTF8 verifies that truncateDesc handles
// multi-byte UTF-8 characters without producing invalid output.
func TestTruncateDesc_MultiByteUTF8(t *testing.T) {
	// Each emoji is 4 bytes but 1 rune
	desc := strings.Repeat("\U0001F600", 20) // 20 emoji, 80 bytes, 20 runes

	result := truncateDesc(desc, 10)

	// Should truncate by runes, not bytes
	runes := []rune(result)
	if len(runes) > 10 {
		t.Errorf("expected max 10 runes, got %d", len(runes))
	}

	// Result should be valid UTF-8 (no partial byte sequences)
	if strings.HasSuffix(result, "...") {
		trimmed := result[:len(result)-3]
		for _, r := range trimmed {
			if r == '\uFFFD' {
				t.Error("found replacement character — invalid UTF-8 truncation")
			}
		}
	}
}
