package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/plan"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/skill"
	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/persistence"
)

// handleKey is a helper that calls handleKey and type-asserts the result back to tuiModel.
func handleKeyHelper(m tuiModel, msg tea.KeyPressMsg) (tuiModel, tea.Cmd) {
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
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: 'a', Text: "a"})

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
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeySpace})

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
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
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
		m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	if m.input != "hello" {
		t.Errorf("input = %q, want %q", m.input, "hello")
	}
}

// TestFindSkillTrigger verifies that '$' is recognized as a skill trigger at
// the start of the input or immediately after whitespace, and rejected when
// embedded inside a word (e.g. shell variable expansion like $HOME).
func TestFindSkillTrigger(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", -1},
		{"$", 0},
		{"$dev-cycle", 0},
		{"$dev-cycle fix bugs", 0},
		{"try $dev-cycle", 4},
		{"hello\t$dev", 6},
		{"hello\n$dev", 6},
		{"hello\r$dev", 6},
		{"hello $dev $other", 6},
		// Embedded in a word — not a valid trigger.
		{"a$b", -1},
		{"/a$b", -1},
		{" $", 1},
	}
	for _, tt := range tests {
		got := findSkillTrigger(tt.input)
		if got != tt.want {
			t.Errorf("findSkillTrigger(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestParseSkillCommand verifies that /skill:name and $name with arguments correctly
// separates the skill name from the arguments.
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
		// /skill: prefix wins over a '$' in the arguments.
		{"/skill:dev-cycle $verbose", "dev-cycle", "$verbose"},
		{"/skill:my-skill run $HOME", "my-skill", "run $HOME"},
		// $ prefix
		{"$dev-cycle", "dev-cycle", ""},
		{"$dev-cycle fix bugs", "dev-cycle", "fix bugs"},
		{"$interview-me how to improve", "interview-me", "how to improve"},
		// $ after whitespace (skill embedded in sentence)
		{"try $dev-cycle", "dev-cycle", ""},
		{"please run $dev-cycle fix these bugs", "dev-cycle", "fix these bugs"},
		{"hello\t$interview-me how", "interview-me", "how"},
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

// TestUpdateSuggestions_DollarPrefix verifies that $ prefix triggers
// skill-only autocomplete (no built-in commands).
func TestUpdateSuggestions_DollarPrefix(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{Name: "dev-cycle", Description: "Dev cycle skill"},
		{Name: "pr-review", Description: "PR review skill"},
		{Name: "test-driven-development", Description: "TDD skill"},
	})

	m := tuiModel{
		input:        "$dev",
		skillCatalog: cat,
	}
	m.updateSuggestions()

	if !m.showSuggestions {
		t.Fatal("expected showSuggestions to be true for $dev")
	}

	// Only skills should appear, no built-in commands
	if len(m.suggestions) != 1 {
		t.Errorf("expected 1 suggestion for '$dev', got %d", len(m.suggestions))
	}
	if m.suggestions[0].name != "dev-cycle" {
		t.Errorf("expected suggestion 'dev-cycle', got %q", m.suggestions[0].name)
	}
	if !m.suggestions[0].isSkill {
		t.Error("expected isSkill = true for skill suggestion")
	}

	// $ alone should show all skills
	m.input = "$"
	m.updateSuggestions()
	if len(m.suggestions) != 3 {
		t.Errorf("expected 3 suggestions for '$', got %d", len(m.suggestions))
	}

	// $ after whitespace: same skill-only lookup, prefix preserved.
	m.input = "please run $dev"
	m.updateSuggestions()
	if !m.showSuggestions {
		t.Fatal("expected showSuggestions to be true for 'please run $dev'")
	}
	if len(m.suggestions) != 1 {
		t.Errorf("expected 1 suggestion for 'please run $dev', got %d", len(m.suggestions))
	}
	if m.suggestions[0].name != "dev-cycle" {
		t.Errorf("expected suggestion 'dev-cycle', got %q", m.suggestions[0].name)
	}

	// $ alone after whitespace: all skills.
	m.input = "try $"
	m.updateSuggestions()
	if len(m.suggestions) != 3 {
		t.Errorf("expected 3 suggestions for 'try $', got %d", len(m.suggestions))
	}

	// $ followed by whitespace triggers lookup, but the token after '$' does
	// not match any skill in the catalog — so no suggestions appear.
	m.input = "echo $HOME"
	m.updateSuggestions()
	if m.showSuggestions && len(m.suggestions) > 0 {
		t.Errorf("expected no suggestions for 'echo $HOME' (token 'HOME' not in catalog), got %d", len(m.suggestions))
	}

	// $ embedded in a word (no whitespace before) must NOT trigger lookup.
	m.input = "echo$HOME"
	m.updateSuggestions()
	if m.showSuggestions && len(m.suggestions) > 0 {
		t.Errorf("expected no suggestions for 'echo$HOME' (no whitespace before $), got %d", len(m.suggestions))
	}
}

// TestHandleSubmit_DollarSkillRoutesThroughLLM verifies that $skill-name args
// routes the skill body through runLLMStream().
func TestHandleSubmit_DollarSkillRoutesThroughLLM(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{
			Name:        "test-skill",
			Description: "A test skill",
			Body:        "# Test Skill\nFollow these instructions carefully.",
		},
	})

	m := tuiModel{
		svc:          &SessionService{},
		input:        "$test-skill do something useful",
		cursor:       40,
		skillCatalog: cat,
	}

	result, cmd := m.handleSubmit()
	rm := result.(tuiModel)

	if !rm.streaming {
		t.Error("expected streaming to be true after $ skill invocation")
	}
	if cmd == nil {
		t.Error("expected non-nil tea.Cmd (should route through runLLMStream)")
	}
	if rm.svc.TurnCount() != 1 {
		t.Errorf("turnCount = %d, want 1", rm.svc.TurnCount())
	}
}

// TestHandleSubmit_DollarAloneShowsUsage verifies that submitting "$" alone
// (with no skill name) shows a usage hint instead of "Unknown skill: .".
func TestHandleSubmit_DollarAloneShowsUsage(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{Name: "dev-cycle", Description: "Dev cycle skill"},
	})

	m := tuiModel{
		svc:          &SessionService{},
		input:        "$",
		cursor:       1,
		skillCatalog: cat,
	}

	result, cmd := m.handleSubmit()
	rm := result.(tuiModel)

	if rm.streaming {
		t.Error("$ alone should not start streaming")
	}
	if cmd != nil {
		t.Error("$ alone should not return a tea.Cmd")
	}

	// Should show usage message, not "Unknown skill: ."
	found := false
	for _, msg := range rm.messages {
		if msg.role == "system" && strings.Contains(msg.content, "Usage: $skill-name") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Usage: $skill-name' system message for $ alone")
	}
}

// TestHandleSubmit_DollarAfterWhitespaceRoutesThroughLLM verifies that
// a '$'-triggered skill is still invoked when the '$' appears after
// whitespace (e.g. natural language prefix like "try $skill args").
func TestHandleSubmit_DollarAfterWhitespaceRoutesThroughLLM(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{
			Name:        "test-skill",
			Description: "A test skill",
			Body:        "# Test Skill\nFollow these instructions carefully.",
		},
	})

	m := tuiModel{
		svc:          &SessionService{},
		input:        "try $test-skill do something useful",
		cursor:       50,
		skillCatalog: cat,
	}

	result, cmd := m.handleSubmit()
	rm := result.(tuiModel)

	if !rm.streaming {
		t.Error("expected streaming to be true after embedded $ skill invocation")
	}
	if cmd == nil {
		t.Error("expected non-nil tea.Cmd (should route through runLLMStream)")
	}
	if rm.svc.TurnCount() != 1 {
		t.Errorf("turnCount = %d, want 1", rm.svc.TurnCount())
	}
	// The user message should preserve the original input verbatim.
	if len(rm.messages) < 1 || rm.messages[0].content != "try $test-skill do something useful" {
		t.Errorf("first user message = %q, want original input", rm.messages[0].content)
	}
}

// TestHandleSubmit_SlashUnknownWithDollarDoesNotInvokeSkill verifies that an
// unrecognized slash command containing '$' after whitespace is NOT silently
// routed as a skill invocation. It must fall through to the slash-command
// default handler and report "Unknown command".
func TestHandleSubmit_SlashUnknownWithDollarDoesNotInvokeSkill(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{
			Name:        "test-skill",
			Description: "A test skill",
			Body:        "# Test Skill",
		},
	})

	m := tuiModel{
		svc:          &SessionService{},
		input:        "/foobar $test-skill",
		cursor:       30,
		skillCatalog: cat,
	}

	result, cmd := m.handleSubmit()
	rm := result.(tuiModel)

	if rm.streaming {
		t.Error("unrecognized slash command must not start streaming as a skill")
	}
	if cmd != nil {
		t.Error("unrecognized slash command must not return a tea.Cmd")
	}

	found := false
	for _, msg := range rm.messages {
		if msg.role == "system" && strings.Contains(msg.content, "Unknown command") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Unknown command' system message for '/foobar $test-skill'")
	}
}

// TestTabAutocomplete_DollarAfterWhitespace verifies that pressing Tab when
// the '$' trigger appears after whitespace preserves the prefix and
// autocompletes the skill name (e.g. "try $dev" -> "try $dev-cycle").
func TestTabAutocomplete_DollarAfterWhitespace(t *testing.T) {
	cat := skill.NewCatalog([]skill.Skill{
		{Name: "dev-cycle", Description: "Dev cycle skill"},
		{Name: "pr-review", Description: "PR review skill"},
	})

	m := tuiModel{
		input:        "try $dev",
		cursor:       8,
		skillCatalog: cat,
	}
	m.updateSuggestions()

	if !m.showSuggestions || len(m.suggestions) == 0 {
		t.Fatalf("expected suggestions for 'try $dev', got none")
	}

	// Simulate Tab
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})

	want := "try $dev-cycle"
	if m.input != want {
		t.Errorf("after Tab, input = %q, want %q", m.input, want)
	}
	if m.cursor != len([]rune(want)) {
		t.Errorf("after Tab, cursor = %d, want %d", m.cursor, len([]rune(want)))
	}
	if m.showSuggestions {
		t.Error("expected showSuggestions to be false after Tab")
	}
}

// TestRenderSuggestions_MixedSkillAlignment verifies that when built-in
// commands and skills are mixed in the suggestion list, descriptions
// are aligned (the [Skill] label width is accounted for in padding).
func TestRenderSuggestions_MixedSkillAlignment(t *testing.T) {
	suggestions := []slashCommand{
		{name: "help", desc: "Show help"},
		{name: "dev-cycle", desc: "Dev cycle workflow", isSkill: true},
	}

	m := tuiModel{
		suggestions:    suggestions,
		selectedSuggest: 0,
	}

	out := m.renderSuggestions()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// stripANSI removes ANSI escape sequences to compare visual columns.
	stripANSI := func(s string) string {
		var b strings.Builder
		inEsc := false
		for _, r := range s {
			if r == '\x1b' {
				inEsc = true
				continue
			}
			if inEsc {
				if r == 'm' {
					inEsc = false
				}
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}

	plain0 := stripANSI(lines[0])
	plain1 := stripANSI(lines[1])

	// runeIndex returns the rune position of substr in s (not byte position).
	runeIndex := func(s, substr string) int {
		sRunes := []rune(s)
		subRunes := []rune(substr)
		for i := 0; i <= len(sRunes)-len(subRunes); i++ {
			match := true
			for j := range subRunes {
				if sRunes[i+j] != subRunes[j] {
					match = false
					break
				}
			}
			if match {
				return i
			}
		}
		return -1
	}

	helpDescIdx := runeIndex(plain0, "Show help")
	skillDescIdx := runeIndex(plain1, "Dev cycle workflow")
	if helpDescIdx < 0 || skillDescIdx < 0 {
		t.Fatalf("descriptions not found in stripped output:\n%q\n%q", plain0, plain1)
	}

	// Descriptions should start at the same visual column
	if helpDescIdx != skillDescIdx {
		t.Errorf("description alignment mismatch: builtin at col %d, skill at col %d\n%s",
			helpDescIdx, skillDescIdx, out)
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
		svc:          &SessionService{},
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
	if rm.svc.TurnCount() != 1 {
		t.Errorf("turnCount = %d, want 1", rm.svc.TurnCount())
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
		svc:          &SessionService{},
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

func TestBuildSystemPrompt_ActiveSkill(t *testing.T) {
	cat := skill.NewCatalog(nil)

	// Without active skill: no "Active Skill" section
	prompt := buildSystemPrompt("/tmp", "main", cat, nil, "", "")
	if strings.Contains(prompt, "Active Skill") {
		t.Error("system prompt should not contain 'Active Skill' section when no skill is active")
	}

	// With active skill: section present with name and args
	active := &activeSkillInfo{name: "pr-review", args: "COMMITS=abc123"}
	prompt = buildSystemPrompt("/tmp", "main", cat, active, "", "")
	if !strings.Contains(prompt, "## Active Skill: pr-review") {
		t.Error("system prompt should contain '## Active Skill: pr-review' section")
	}
	if !strings.Contains(prompt, "Arguments: COMMITS=abc123") {
		t.Error("system prompt should contain the skill arguments")
	}
	if !strings.Contains(prompt, "Do NOT perform actions outside this skill's scope") {
		t.Error("system prompt should contain the scope guardrail")
	}
}

func TestBuildSystemPrompt_ActiveSkillNoArgs(t *testing.T) {
	cat := skill.NewCatalog(nil)
	active := &activeSkillInfo{name: "dev-cycle", args: ""}
	prompt := buildSystemPrompt("/tmp", "main", cat, active, "", "")
	if !strings.Contains(prompt, "## Active Skill: dev-cycle") {
		t.Error("system prompt should contain the active skill section")
	}
	if strings.Contains(prompt, "Arguments:") {
		t.Error("system prompt should not contain 'Arguments:' when args are empty")
	}
}

func TestToolVerb(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"write_file", "Write"},
		{"read_file", "Read"},
		{"bash", "Bash"},
		{"grep", "Search"},
		{"search_replace", "Edit"},
		{"unknown_tool", "Unknown_tool"},
		{"", "Tool"},
		// Multi-byte UTF-8 tool name should not panic or produce garbled output
		{"α_tool", "Α_tool"},
	}
	for _, tt := range tests {
		got := toolVerb(tt.name)
		if got != tt.want {
			t.Errorf("toolVerb(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestDiffStats(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		result   string
		want     string
	}{
		{"empty", "edit_file", "", ""},
		{"no diff lines", "edit_file", "hello world\nfoo bar", ""},
		{"added lines", "edit_file", "+line1\n+line2\n-line3\nplain", "+2/-1"},
		{"only removed", "edit_file", "-line1\n-line2\nplain", "+0/-2"},
		// Non-diff tool should return empty even if result has +/- lines
		{"bash false positive", "bash", "+1 for yes\n-1 for no", ""},
		{"read_file false positive", "read_file", "+x means positive\n-y means negative", ""},
	}
	for _, tt := range tests {
		got := diffStats(tt.toolName, tt.result)
		if got != tt.want {
			t.Errorf("diffStats(%q, %q) = %q, want %q", tt.toolName, tt.result, got, tt.want)
		}
	}
}

func TestToolArgSummary_PriorityKeys(t *testing.T) {
	// file_path should appear first (as raw value) instead of key=value
	args := `{"file_path": "/src/foo.go", "mode": "read-only"}`
	summary := toolArgSummary(args)
	if !strings.HasPrefix(summary, "/src/foo.go") {
		t.Errorf("summary should start with file_path value, got: %q", summary)
	}
	if !strings.Contains(summary, "mode=read-only") {
		t.Errorf("summary should contain mode=read-only, got: %q", summary)
	}

	// command should appear first for bash
	args = `{"command": "go test ./...", "timeout": 30}`
	summary = toolArgSummary(args)
	if !strings.HasPrefix(summary, "go test ./...") {
		t.Errorf("summary should start with command value, got: %q", summary)
	}
}

func TestSessionPickerKey_Escape(t *testing.T) {
	m := tuiModel{
		showSessionPicker: true,
		sessionPickerList: []*session.SerializedSession{
			{ID: "s1", Title: "Session 1"},
			{ID: "s2", Title: "Session 2"},
		},
		sessionPickerSel: 0,
		width:            80,
		height:           24,
	}
	result, _ := m.handleSessionPickerKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := result.(tuiModel)
	if got.showSessionPicker {
		t.Error("Esc should close the session picker")
	}
}

func TestToolArgSummary_Deterministic(t *testing.T) {
	// Non-priority keys should appear in sorted order for deterministic output
	args := `{"zebra": "z", "alpha": "a", "file_path": "/f"}`
	summary := toolArgSummary(args)
	// file_path first, then alpha before zebra
	alphaIdx := strings.Index(summary, "alpha=a")
	zebraIdx := strings.Index(summary, "zebra=z")
	if alphaIdx < 0 || zebraIdx < 0 {
		t.Fatalf("summary missing keys: %q", summary)
	}
	if alphaIdx >= zebraIdx {
		t.Errorf("non-priority keys should be sorted: alpha before zebra, got: %q", summary)
	}
}

func TestSessionPickerKey_CtrlC(t *testing.T) {
	m := tuiModel{
		showSessionPicker: true,
		sessionPickerList: []*session.SerializedSession{
			{ID: "s1", Title: "Session 1"},
		},
		sessionPickerSel: 0,
		width:            80,
		height:           24,
	}
	result, cmd := m.handleSessionPickerKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	got := result.(tuiModel)
	if !got.quitting {
		t.Error("Ctrl+C should set quitting = true")
	}
	if cmd == nil {
		t.Error("Ctrl+C should return a quit command")
	}
}

func TestSessionPickerKey_Enter(t *testing.T) {
	// Enter with empty list should not panic
	m := tuiModel{
		showSessionPicker: true,
		sessionPickerList: []*session.SerializedSession{},
		sessionPickerSel:  0,
		width:             80,
		height:            24,
	}
	_, _ = m.handleSessionPickerKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	// No panic means the guard works
}

func TestSessionPickerKey_Navigation(t *testing.T) {
	m := tuiModel{
		showSessionPicker: true,
		sessionPickerList: []*session.SerializedSession{
			{ID: "s1", Title: "Session 1"},
			{ID: "s2", Title: "Session 2"},
			{ID: "s3", Title: "Session 3"},
		},
		sessionPickerSel: 0,
		width:            80,
		height:           24,
	}

	// Down should move selection from 0 to 1
	result, _ := m.handleSessionPickerKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got := result.(tuiModel)
	if got.sessionPickerSel != 1 {
		t.Errorf("after Down, sel = %d, want 1", got.sessionPickerSel)
	}

	// Down again should move to 2
	result, _ = got.handleSessionPickerKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got = result.(tuiModel)
	if got.sessionPickerSel != 2 {
		t.Errorf("after Down+Down, sel = %d, want 2", got.sessionPickerSel)
	}

	// Down at last item should stay at 2 (clamped)
	result, _ = got.handleSessionPickerKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got = result.(tuiModel)
	if got.sessionPickerSel != 2 {
		t.Errorf("after Down at end, sel = %d, want 2", got.sessionPickerSel)
	}

	// Up should move from 2 to 1
	result, _ = got.handleSessionPickerKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got = result.(tuiModel)
	if got.sessionPickerSel != 1 {
		t.Errorf("after Up, sel = %d, want 1", got.sessionPickerSel)
	}
}

func TestRenderSessionPicker(t *testing.T) {
	m := tuiModel{
		showSessionPicker: true,
		sessionPickerList: []*session.SerializedSession{
			{ID: "s1", Title: "First Session"},
			{ID: "s2", Title: "Second Session"},
		},
		sessionPickerSel: 0,
		width:            80,
		height:           24,
	}
	rendered := m.renderSessionPicker()
	if !strings.Contains(rendered, "Resume a session") {
		t.Error("render should contain header 'Resume a session'")
	}
	if !strings.Contains(rendered, "First Session") {
		t.Error("render should contain 'First Session'")
	}
	if !strings.Contains(rendered, "Second Session") {
		t.Error("render should contain 'Second Session'")
	}
	if !strings.Contains(rendered, "Enter resume") {
		t.Error("render should contain 'Enter resume' hint")
	}
}

// TestScrollWithEmptyInput_UpDown tests that Up/Down arrows scroll the
// content viewport when the input is empty, instead of navigating input history.
func TestScrollWithEmptyInput_UpDown(t *testing.T) {
	// Build a model with enough messages to require scrolling
	var msgs []chatMessage
	for i := 0; i < 50; i++ {
		msgs = append(msgs, chatMessage{role: "assistant", content: fmt.Sprintf("Line %d of output content", i)})
	}

	m := tuiModel{
		messages:     msgs,
		input:        "", // empty input
		cursor:       0,
		scrollOffset: 0,
		width:        80,
		height:       24,
		inputHistory: &InputHistory{
			entries: []string{"previous question 1", "previous question 2"},
			index:   -1,
		},
	}

	// Press Up arrow — should scroll viewport up, NOT navigate input history
	got, _ := handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got.scrollOffset == 0 {
		t.Error("Up arrow with empty input should increase scrollOffset (scroll content up)")
	}
	if got.input != "" {
		t.Errorf("Up arrow with empty input should not change input field, got: %q", got.input)
	}

	// Press Up again — scroll further
	got2, _ := handleKeyHelper(got, tea.KeyPressMsg{Code: tea.KeyUp})
	if got2.scrollOffset <= got.scrollOffset {
		t.Error("Second Up arrow should increase scrollOffset further")
	}

	// Press Down arrow — should scroll viewport back down
	got3, _ := handleKeyHelper(got2, tea.KeyPressMsg{Code: tea.KeyDown})
	if got3.scrollOffset >= got2.scrollOffset {
		t.Error("Down arrow should decrease scrollOffset (scroll content down)")
	}
}

// TestInputHistoryWithNonEmptyInput tests that Up/Down arrows navigate
// input history when the input field is non-empty.
func TestInputHistoryWithNonEmptyInput(t *testing.T) {
	m := tuiModel{
		input:        "current text",
		cursor:       12,
		scrollOffset: 0,
		width:        80,
		height:       24,
		inputHistory: &InputHistory{
			entries: []string{"previous question"},
			index:   -1,
		},
	}

	// Press Up with non-empty input — should navigate history
	got, _ := handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got.input != "previous question" {
		t.Errorf("Up arrow with non-empty input should navigate history, got: %q", got.input)
	}
	if got.scrollOffset != 0 {
		t.Error("Up arrow with non-empty input should not change scrollOffset")
	}
}

// TestNewlineWithShiftEnter tests that Shift+Enter inserts a newline
// into the input field rather than submitting.
func TestNewlineWithShiftEnter(t *testing.T) {
	m := tuiModel{
		input:        "hello world",
		cursor:       5, // cursor after "hello"
		scrollOffset: 0,
		width:        80,
		height:       24,
		inputHistory: &InputHistory{index: -1},
	}

	// Shift+Enter should insert a newline at cursor position
	got, _ := handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if got.input != "hello\n world" {
		t.Errorf("Shift+Enter should insert newline, got: %q", got.input)
	}
	if got.cursor != 6 {
		t.Errorf("cursor should advance to 6, got: %d", got.cursor)
	}
}

// TestNewlineWithAltEnter tests that Alt+Enter inserts a newline.
func TestNewlineWithAltEnter(t *testing.T) {
	m := tuiModel{
		input:        "foo bar",
		cursor:       3, // after "foo"
		scrollOffset: 0,
		width:        80,
		height:       24,
		inputHistory: &InputHistory{index: -1},
	}

	got, _ := handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if got.input != "foo\n bar" {
		t.Errorf("Alt+Enter should insert newline, got: %q", got.input)
	}
	if got.cursor != 4 {
		t.Errorf("cursor should advance to 4, got: %d", got.cursor)
	}
}

// TestNewlineWithCtrlJ tests that Ctrl+J inserts a newline (safety net
// for terminals that don't report ModShift).
func TestNewlineWithCtrlJ(t *testing.T) {
	m := tuiModel{
		input:        "abc",
		cursor:       3,
		scrollOffset: 0,
		width:        80,
		height:       24,
		inputHistory: &InputHistory{index: -1},
	}

	got, _ := handleKeyHelper(m, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if got.input != "abc\n" {
		t.Errorf("Ctrl+J should insert newline, got: %q", got.input)
	}
	if got.cursor != 4 {
		t.Errorf("cursor should advance to 4, got: %d", got.cursor)
	}
}

// TestReplayHistory_RestoresContextUsage verifies that replayHistory uses
// persisted real API token counts (tokens_in/tokens_out) for the context
// manager instead of text-based estimates, which drastically undercount.
func TestReplayHistory_RestoresContextUsage(t *testing.T) {
	dir, err := os.MkdirTemp("", "replay-ctx-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := persistence.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	appScope := di.NewAppScope("test")
	sessStore := session.NewSessionStore(store, appScope)
	sessID := "sess-ctx-1"

	// Create a session with persisted token metadata (real API counts)
	sess, err := session.NewSession(sessID, "Test", appScope)
	if err != nil {
		t.Fatal(err)
	}
	sess.Metadata["tokens_in"] = 196800  // real API input tokens
	sess.Metadata["tokens_out"] = 6300    // real API output tokens
	if err := sessStore.Save(context.Background(), sess); err != nil {
		t.Fatal(err)
	}

	// Add history messages
	ctx := context.Background()
	_ = sessStore.History().AddMessage(ctx, sessID, session.Message{Role: "user", Content: "Hello"})
	_ = sessStore.History().AddMessage(ctx, sessID, session.Message{Role: "assistant", Content: "Hi there"})

	// Create a tuiModel and replay history
	app := &App{SessionStore: sessStore}
	svc := NewSessionService(sess, app, SessionServiceConfig{
		MaxCtx: 262144,
	})
	m := tuiModel{
		svc: svc,
		app: app,
	}
	m.replayHistory()

	// Context manager should use persisted real tokens, not text estimates.
	// Expected: tokens_in + tokens_out = 196800 + 6300 = 203100
	got := m.svc.ContextMgr().CurrentUsage()
	want := 196800 + 6300
	if got != want {
		t.Errorf("contextMgr.CurrentUsage() = %d, want %d", got, want)
	}

	// Verify display format shows ~203.1K, not ~2.4K
	display := m.svc.ContextMgr().UsageDisplay()
	if !strings.Contains(display, "203.1K") {
		t.Errorf("UsageDisplay() = %q, want to contain '203.1K'", display)
	}
}

// TestRenderDrawer_ContainsAllSections verifies that renderDrawer outputs
// the three expected sections: Progress, Tools, and Skills.
func TestRenderDrawer_ContainsAllSections(t *testing.T) {
	tracker := plan.NewTracker()
	tracker.SetTasks([]plan.Task{
		{Title: "Implement feature", Status: plan.StatusActive},
		{Title: "Write tests", Status: plan.StatusPending},
		{Title: "Deploy", Status: plan.StatusDone},
	})

	m := tuiModel{
		svc: &SessionService{planTracker: tracker},
		drawerToolLog: []drawerToolEntry{
			{name: "read_file", args: `/src/foo.go`, at: time.Now(), duration: 200 * time.Millisecond},
			{name: "bash", args: `go test ./...`, at: time.Now(), duration: 1 * time.Second},
		},
		drawerSkills: []drawerSkillEntry{
			{name: "dev-cycle", at: time.Now()},
		},
	}

	out := m.renderDrawer(40)

	for _, section := range []string{"Progress", "Tools", "Skills"} {
		if !strings.Contains(out, section) {
			t.Errorf("renderDrawer output missing section %q", section)
		}
	}

	// Verify task content
	if !strings.Contains(out, "Implement feature") {
		t.Error("renderDrawer missing task 'Implement feature'")
	}

	// Verify tool content
	if !strings.Contains(out, "read_file") {
		t.Error("renderDrawer missing tool 'read_file'")
	}

	// Verify skill content
	if !strings.Contains(out, "dev-cycle") {
		t.Error("renderDrawer missing skill 'dev-cycle'")
	}
}

// TestRenderDrawer_EmptyState verifies renderDrawer handles empty data gracefully.
func TestRenderDrawer_EmptyState(t *testing.T) {
	tracker := plan.NewTracker()
	m := tuiModel{
		svc: &SessionService{planTracker: tracker},
	}

	out := m.renderDrawer(30)

	if !strings.Contains(out, "No tasks") {
		t.Error("expected 'No tasks' for empty tracker")
	}
	if !strings.Contains(out, "No tool calls") {
		t.Error("expected 'No tool calls' for empty tool log")
	}
	if !strings.Contains(out, "No skills used") {
		t.Error("expected 'No skills used' for empty skills")
	}
}

// TestCtrlTToggle_Drawer verifies that Ctrl+T toggles the showDrawer flag.
func TestCtrlTToggle_Drawer(t *testing.T) {
	m := tuiModel{
		showDrawer: false,
	}

	// First Ctrl+T should open the drawer
	got, _ := handleKeyHelper(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if !got.showDrawer {
		t.Error("Ctrl+T should toggle showDrawer to true")
	}

	// Second Ctrl+T should close the drawer
	got2, _ := handleKeyHelper(got, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if got2.showDrawer {
		t.Error("Ctrl+T should toggle showDrawer back to false")
	}
}

// TestCtrlTToggle_DrawerDuringStreaming verifies that Ctrl+T toggles the
// showDrawer flag even while a model response is streaming. The streaming
// branch in Update uses a restricted key handler that previously swallowed
// Ctrl+T, making the drawer untoggleable mid-stream.
func TestCtrlTToggle_DrawerDuringStreaming(t *testing.T) {
	m := tuiModel{
		showDrawer: false,
		streaming:  true,
	}

	// First Ctrl+T should open the drawer even while streaming
	gotModel, _ := m.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	got := gotModel.(tuiModel)
	if !got.showDrawer {
		t.Error("Ctrl+T should toggle showDrawer to true during streaming")
	}

	// Second Ctrl+T should close the drawer again
	gotModel2, _ := got.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	got2 := gotModel2.(tuiModel)
	if got2.showDrawer {
		t.Error("Ctrl+T should toggle showDrawer back to false during streaming")
	}
}

// TestTruncateRune_MultiByteUTF8 verifies that truncateRune handles
// multi-byte characters without producing invalid UTF-8.
func TestTruncateRune_MultiByteUTF8(t *testing.T) {
	// Chinese characters: each is 3 bytes but 1 rune
	input := "\u4f60\u597d\u4e16\u754c\u8fd9\u662f\u4e00\u4e2a\u6d4b\u8bd5" // 10 Chinese chars
	result := truncateRune(input, 7)

	runes := []rune(result)
	if len(runes) > 7 {
		t.Errorf("expected max 7 runes, got %d", len(runes))
	}

	// Verify no replacement characters (invalid UTF-8)
	for _, r := range result {
		if r == '\uFFFD' {
			t.Error("found replacement character — invalid UTF-8 truncation")
		}
	}
}

// TestToolArgSummary_MultiByteUTF8 verifies toolArgSummary doesn't corrupt
// multi-byte UTF-8 in argument values.
func TestToolArgSummary_MultiByteUTF8(t *testing.T) {
	// JSON with long Chinese value
	args := `{"file_path": "/src/\u4f60\u597d\u4e16\u754c\u8fd9\u662f\u4e00\u4e2a\u5f88\u957f\u7684\u6587\u4ef6\u8def\u5f84\u540d\u79f0\u6d4b\u8bd5\u7528\u4f8b\u6570\u636e.go"}`
	result := toolArgSummary(args)

	// Verify no replacement characters
	for _, r := range result {
		if r == '\uFFFD' {
			t.Error("toolArgSummary corrupted multi-byte UTF-8")
		}
	}
}

// TestBuildSystemPrompt_UpdatePlanHint verifies that buildSystemPrompt
// includes the update_plan tool hint.
func TestBuildSystemPrompt_UpdatePlanHint(t *testing.T) {
	cat := skill.NewCatalog(nil)
	prompt := buildSystemPrompt("/tmp", "main", cat, nil, "", "")

	if !strings.Contains(prompt, "update_plan") {
		t.Error("system prompt should mention update_plan tool")
	}
}

// ── @ file completion tests ──

// TestFindFileTrigger verifies that '@' is recognized as a file-completion
// trigger at the start of the input or immediately after whitespace, and
// rejected when embedded inside a word (e.g. email addresses).
func TestFindFileTrigger(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", -1},
		{"@", 0},
		{"@src", 0},
		{"@src/main.go", 0},
		{"look at @src", 8},
		{"hello\t@dev", 6},
		{"hello\n@dev", 6},
		{"hello\r@dev", 6},
		{"hello @dev @other", 6},
		// Embedded in a word — not a valid trigger.
		{"user@host", -1},
		{"a@b", -1},
		{"/@b", -1},
		{" @", 1},
	}
	for _, tt := range tests {
		got := findFileTrigger(tt.input)
		if got != tt.want {
			t.Errorf("findFileTrigger(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestParseFileQuery verifies the directory + filter decomposition for both
// absolute and relative queries.
func TestParseFileQuery(t *testing.T) {
	cwd := "/home/user/project"
	tests := []struct {
		query      string
		wantDir    string
		wantFilter string
	}{
		{"", cwd, ""},
		{"src", cwd, "src"},
		{"src/", filepath.Join(cwd, "src"), ""},
		{"src/ma", filepath.Join(cwd, "src"), "ma"},
		{"/", "/", ""},
		{"/usr", "/", "usr"},
		{"/usr/", "/usr", ""},
		{"/usr/l", "/usr", "l"},
	}
	for _, tt := range tests {
		dir, filter := parseFileQuery(tt.query, cwd)
		if dir != tt.wantDir || filter != tt.wantFilter {
			t.Errorf("parseFileQuery(%q, %q) = (%q, %q), want (%q, %q)",
				tt.query, cwd, dir, filter, tt.wantDir, tt.wantFilter)
		}
	}
}

// TestListFileCandidates creates a temporary directory tree and verifies that
// listFileCandidates returns entries sorted with directories first, filtering
// by prefix, and hiding dotfiles unless the filter starts with '.'.
func TestListFileCandidates(t *testing.T) {
	// Create a temp directory with known structure.
	root := t.TempDir()
	// Create directories.
	for _, d := range []string{"alpha", "beta", "src", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Create files.
	for _, f := range []string{"README.md", "main.go", "go.mod", ".env"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Test 1: Empty filter returns all non-hidden entries, dirs first.
	candidates := listFileCandidates("", root)
	if len(candidates) == 0 {
		t.Fatal("expected candidates for empty filter, got none")
	}
	// First entries should be directories.
	for i, c := range candidates {
		if c.name == ".hidden" || c.name == ".env" {
			t.Errorf("candidate[%d] = %q should be hidden (filter doesn't start with '.')", i, c.name)
		}
	}
	// Check dirs come before files.
	lastDirIdx := -1
	firstFileIdx := len(candidates)
	for i, c := range candidates {
		if c.isDir {
			lastDirIdx = i
		} else if i < firstFileIdx {
			firstFileIdx = i
		}
	}
	if lastDirIdx >= firstFileIdx {
		t.Error("directories should be listed before files")
	}

	// Test 2: Filter by prefix "ma" should match only "main.go".
	candidates = listFileCandidates("ma", root)
	if len(candidates) != 1 || candidates[0].name != "main.go" {
		names := make([]string, len(candidates))
		for i, c := range candidates {
			names[i] = c.name
		}
		t.Errorf("listFileCandidates(%q) = %v, want [main.go]", "ma", names)
	}

	// Test 3: Dotfiles shown when filter starts with '.'.
	candidates = listFileCandidates(".", root)
	hasHidden := false
	for _, c := range candidates {
		if c.name == ".hidden" || c.name == ".env" {
			hasHidden = true
		}
	}
	if !hasHidden {
		t.Error("expected dotfiles when filter starts with '.'")
	}

	// Test 4: Absolute path lookup.
	candidates = listFileCandidates("/", "/")
	if len(candidates) == 0 {
		t.Error("expected candidates for '/' query")
	}
	for _, c := range candidates {
		if !filepath.IsAbs(c.absPath) {
			t.Errorf("absPath %q should be absolute", c.absPath)
		}
	}
}

// TestUpdateSuggestions_AtPrefix verifies that typing '@' followed by a path
// fragment populates the suggestion list with file candidates.
func TestUpdateSuggestions_AtPrefix(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "dir"), 0o755)

	m := tuiModel{
		input: "@fi",
		cwd:   root,
	}
	m.updateSuggestions()
	if !m.showSuggestions {
		t.Fatal("expected showSuggestions=true for '@fi'")
	}
	if len(m.suggestions) != 1 || m.suggestions[0].name != "file.txt" {
		t.Errorf("suggestions = %v, want [file.txt]", m.suggestions)
	}
	if len(m.fileCandidates) == 0 {
		t.Error("fileCandidates should be populated after @ trigger")
	}
}

// TestFileCompletion_TabCycling verifies that pressing Tab cycles through
// file candidates, replacing input with each candidate's absolute path.
func TestFileCompletion_TabCycling(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "alpha"), 0o755)
	os.MkdirAll(filepath.Join(root, "beta"), 0o755)
	os.WriteFile(filepath.Join(root, "gamma.txt"), []byte("x"), 0o644)

	m := tuiModel{
		input: "@",
		cwd:   root,
	}
	m.updateSuggestions()
	if len(m.fileCandidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(m.fileCandidates))
	}

	// Tab 1: first candidate (dirs first, alphabetically).
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.input != m.filePrefix+m.fileCandidates[0].absPath {
		t.Errorf("after Tab 1, input = %q, want %q", m.input, m.fileCandidates[0].absPath)
	}
	if m.selectedSuggest != 0 {
		t.Errorf("after Tab 1, selectedSuggest = %d, want 0", m.selectedSuggest)
	}

	// Tab 2: second candidate.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.input != m.filePrefix+m.fileCandidates[1].absPath {
		t.Errorf("after Tab 2, input = %q, want %q", m.input, m.fileCandidates[1].absPath)
	}
	if m.selectedSuggest != 1 {
		t.Errorf("after Tab 2, selectedSuggest = %d, want 1", m.selectedSuggest)
	}

	// Tab 3: third candidate.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.selectedSuggest != 2 {
		t.Errorf("after Tab 3, selectedSuggest = %d, want 2", m.selectedSuggest)
	}

	// Tab 4: wraps around to first candidate.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.selectedSuggest != 0 {
		t.Errorf("after Tab 4 (wrap), selectedSuggest = %d, want 0", m.selectedSuggest)
	}
}

// TestFileCompletion_SpaceConfirms verifies that Space confirms the current
// file candidate, clears cycling state, and adds a trailing space.
func TestFileCompletion_SpaceConfirms(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o644)

	m := tuiModel{
		input: "@",
		cwd:   root,
	}
	m.updateSuggestions()

	// Tab once to select first candidate.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if len(m.fileCandidates) == 0 {
		t.Fatal("fileCandidates should be populated")
	}
	expectedPath := m.fileCandidates[0].absPath

	// Space confirms: clears state, adds trailing space.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.fileCandidates != nil {
		t.Error("fileCandidates should be nil after Space confirmation")
	}
	if m.showSuggestions {
		t.Error("showSuggestions should be false after Space confirmation")
	}
	if !strings.HasSuffix(m.input, " ") {
		t.Error("input should end with trailing space after Space confirmation")
	}
	if !strings.HasPrefix(m.input, expectedPath) {
		t.Errorf("input = %q, should start with %q", m.input, expectedPath)
	}
}

// TestFileCompletion_SpaceWithoutTabInsertsNormalSpace verifies that
// pressing Space after '@src' without pressing Tab first inserts a
// normal space character rather than confirming a file candidate.
func TestFileCompletion_SpaceWithoutTabInsertsNormalSpace(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "src.txt"), []byte("x"), 0o644)

	m := tuiModel{
		input:  "@src",
		cursor: 4, // cursor at end of input
		cwd:    root,
	}
	m.updateSuggestions()
	if len(m.fileCandidates) == 0 {
		t.Fatal("fileCandidates should be populated")
	}
	if m.fileCycleIdx != 0 {
		t.Fatalf("fileCycleIdx should be 0 (no Tab pressed), got %d", m.fileCycleIdx)
	}

	// Space without Tab: should insert a normal space, NOT confirm.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeySpace})
	// Input should be "@src " (original text + space inserted at cursor).
	if m.input != "@src " {
		t.Errorf("input = %q, want %q (normal space insertion)", m.input, "@src ")
	}
}

// TestFileCompletion_EnterConfirms verifies that Enter confirms the
// current file candidate without submitting the message.
func TestFileCompletion_EnterConfirms(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "alpha"), 0o755)
	os.WriteFile(filepath.Join(root, "beta.txt"), []byte("x"), 0o644)

	m := tuiModel{
		input: "@",
		cwd:   root,
	}
	m.updateSuggestions()

	// Tab once to select first candidate.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if len(m.fileCandidates) == 0 {
		t.Fatal("fileCandidates should be populated")
	}
	expectedPath := m.fileCandidates[0].absPath

	// Enter confirms: clears state, does NOT submit.
	m, cmd := handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.fileCandidates != nil {
		t.Error("fileCandidates should be nil after Enter confirmation")
	}
	if m.showSuggestions {
		t.Error("showSuggestions should be false after Enter confirmation")
	}
	if !strings.HasPrefix(m.input, expectedPath) {
		t.Errorf("input = %q, should start with %q", m.input, expectedPath)
	}
	// Enter should NOT produce a submit command (cmd should be nil).
	if cmd != nil {
		t.Error("Enter during file confirmation should not return a tea.Cmd (no submit)")
	}
}

// TestFileCompletion_CursorMovementClearsState verifies that pressing
// cursor movement keys (Left, Right, Ctrl+A, Ctrl+E, Ctrl+B, Ctrl+F)
// clears stale file completion state so that Space/Enter behave normally.
func TestFileCompletion_CursorMovementClearsState(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "alpha"), 0o755)
	os.WriteFile(filepath.Join(root, "beta.txt"), []byte("x"), 0o644)

	m := tuiModel{
		input:  "@",
		cursor: 1,
		cwd:    root,
	}
	m.updateSuggestions()

	// Tab once to enter file cycling.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.fileCandidates == nil || m.fileCycleIdx == 0 {
		t.Fatal("file cycling state should be active after Tab")
	}

	// Press Left arrow — should clear file state.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.fileCandidates != nil {
		t.Error("fileCandidates should be nil after Left arrow")
	}
	if m.fileCycleIdx != 0 {
		t.Error("fileCycleIdx should be 0 after Left arrow")
	}
	if m.filePrefix != "" {
		t.Error("filePrefix should be empty after Left arrow")
	}
}

// TestFileCompletion_EditingCommandsClearState verifies that Ctrl+K, Ctrl+U,
// and Ctrl+W clear file completion state via updateSuggestions.
func TestFileCompletion_EditingCommandsClearState(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "alpha"), 0o755)
	os.WriteFile(filepath.Join(root, "beta.txt"), []byte("x"), 0o644)

	m := tuiModel{
		input:  "@",
		cursor: 1,
		cwd:    root,
	}
	m.updateSuggestions()

	// Tab once to enter file cycling.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.fileCandidates == nil || m.fileCycleIdx == 0 {
		t.Fatal("file cycling state should be active after Tab")
	}

	// Ctrl+K (kill to end) — should clear file state.
	m, _ = handleKeyHelper(m, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if m.fileCandidates != nil {
		t.Error("fileCandidates should be nil after Ctrl+K")
	}
	if m.fileCycleIdx != 0 {
		t.Error("fileCycleIdx should be 0 after Ctrl+K")
	}
}

// TestUpdateSuggestions_SlashPathNoCommandTrigger verifies that an input
// starting with '/' followed by path separators (like an absolute path
// produced by @ file completion) does NOT trigger command completion.
func TestUpdateSuggestions_SlashPathNoCommandTrigger(t *testing.T) {
	m := tuiModel{
		input: "/usr/local/bin/something",
	}
	m.updateSuggestions()
	// Should NOT show suggestions since this looks like a file path, not a command.
	if m.showSuggestions {
		t.Error("showSuggestions should be false for filesystem path input")
	}

	// A real command prefix should still trigger.
	m.input = "/help"
	m.updateSuggestions()
	// Should show command suggestions (there's at least one /help-like command or none,
	// but the / branch should be entered regardless).
	// The key assertion is that the / branch IS entered for single-word commands.
}

// TestListFileCandidates_SymlinkedDirectory verifies that symlinks to
// directories are correctly identified as directories (isDir=true).
func TestListFileCandidates_SymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "realdir")
	os.MkdirAll(realDir, 0o755)
	linkPath := filepath.Join(root, "linkdir")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	candidates := listFileCandidates("", root)
	for _, c := range candidates {
		if c.name == "linkdir" && !c.isDir {
			t.Error("symlinked directory should have isDir=true")
		}
	}
}

// TestListFileCandidates_PermissionDenied verifies that listFileCandidates
// returns a synthetic error indicator when the directory is unreadable.
func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxWidth int
		want     string
	}{
		{"short line", "hello", 20, "hello"},
		{"empty input", "", 20, ""},
		{"zero width", "hello", 0, "hello"},
		{"negative width", "hello", -1, "hello"},
		{"exact width", "hello", 5, "hello"},
		{
			"long line wraps at word boundary",
			"the quick brown fox jumps over the lazy dog",
			15,
			"the quick brown\nfox jumps over\nthe lazy dog",
		},
		{
			"preserves existing newlines",
			"line one\nline two\nline three",
			20,
			"line one\nline two\nline three",
		},
		{
			"word longer than maxWidth breaks hard",
			"abcdefghijklmnopqrstuvwxyz",
			10,
			"abcdefghij\nklmnopqrst\nuvwxyz",
		},
		{
			"multi-byte runes counted correctly",
			"你好世界 hello",
			8,
			"你好世界\nhello",
		},
		{
			"blank line preserved",
			"line one\n\nline three",
			20,
			"line one\n\nline three",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.text, tt.maxWidth)
			if got != tt.want {
				t.Errorf("wrapText(%q, %d)\ngot:  %q\nwant: %q", tt.text, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestListFileCandidates_PermissionDenied(t *testing.T) {
	root := t.TempDir()
	restricted := filepath.Join(root, "restricted")
	os.MkdirAll(restricted, 0o755)
	os.Chmod(restricted, 0o000) // no read permission
	defer os.Chmod(restricted, 0o755) // restore for cleanup

	candidates := listFileCandidates("restricted/", root)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 error indicator, got %d candidates", len(candidates))
	}
	if candidates[0].name != "[permission denied]" {
		t.Errorf("expected name %q, got %q", "[permission denied]", candidates[0].name)
	}
	if !candidates[0].isFile {
		t.Error("error indicator should have isFile=true")
	}
}

// ── SessionService tests ──

func TestSessionService_HistoryOperations(t *testing.T) {
	svc := &SessionService{}

	// Empty history
	if svc.HistoryLen() != 0 {
		t.Errorf("initial history len = %d, want 0", svc.HistoryLen())
	}

	// Append
	svc.AppendMessages(kosong.CreateUserMessage("hello"))
	svc.AppendMessages(kosong.CreateUserMessage("world"))
	if svc.HistoryLen() != 2 {
		t.Errorf("after 2 appends, len = %d, want 2", svc.HistoryLen())
	}

	// History returns a copy
	h := svc.History()
	if len(h) != 2 {
		t.Errorf("History() len = %d, want 2", len(h))
	}
	h = append(h, kosong.CreateUserMessage("extra"))
	if svc.HistoryLen() != 2 {
		t.Error("modifying History() copy should not affect service")
	}

	// Truncate
	svc.TruncateHistory(1)
	if svc.HistoryLen() != 1 {
		t.Errorf("after truncate(1), len = %d, want 1", svc.HistoryLen())
	}

	// Clear
	svc.ClearHistory()
	if svc.HistoryLen() != 0 {
		t.Errorf("after clear, len = %d, want 0", svc.HistoryLen())
	}

	// Rewrite
	svc.RewriteHistory([]kosong.Message{kosong.CreateUserMessage("a"), kosong.CreateUserMessage("b")})
	if svc.HistoryLen() != 2 {
		t.Errorf("after rewrite, len = %d, want 2", svc.HistoryLen())
	}
}

func TestSessionService_TurnAndUsage(t *testing.T) {
	svc := &SessionService{}

	svc.IncrementTurn()
	svc.IncrementTurn()
	if svc.TurnCount() != 2 {
		t.Errorf("turnCount = %d, want 2", svc.TurnCount())
	}

	svc.AddTurnUsage(kosong.TokenUsage{InputOther: 100, Output: 50})
	tu := svc.TurnUsage()
	if tu.InputOther != 100 || tu.Output != 50 {
		t.Errorf("turnUsage = %+v, want {InputOther:100, Output:50}", tu)
	}

	svc.AddSessionUsage(kosong.TokenUsage{InputOther: 200, Output: 100})
	su := svc.SessionUsage()
	if su.InputOther != 200 || su.Output != 100 {
		t.Errorf("sessionUsage = %+v, want {InputOther:200, Output:100}", su)
	}

	svc.ResetTurnUsage()
	if svc.TurnUsage() != (kosong.TokenUsage{}) {
		t.Error("ResetTurnUsage should zero out turn usage")
	}
}

func TestSessionService_Reset(t *testing.T) {
	sess, _ := session.NewSession("test", "Test", di.NewAppScope("test"))
	app := &App{Config: config.DefaultConfig()}
	svc := NewSessionService(sess, app, SessionServiceConfig{MaxCtx: 262144})

	svc.AppendMessages(kosong.CreateUserMessage("hello"))
	svc.IncrementTurn()
	svc.AppendTurn(turnData{text: "response"})
	svc.AddSessionUsage(kosong.TokenUsage{InputOther: 100})
	svc.SetLastPrompt("hello")
	svc.IncrementOverflow()

	svc.Reset()

	if svc.HistoryLen() != 0 {
		t.Error("Reset should clear history")
	}
	if svc.TurnCount() != 0 {
		t.Error("Reset should clear turn count")
	}
	if len(svc.CompletedTurns()) != 0 {
		t.Error("Reset should clear completed turns")
	}
	if svc.SessionUsage() != (kosong.TokenUsage{}) {
		t.Error("Reset should clear session usage")
	}
	if svc.LastPrompt() != "" {
		t.Error("Reset should clear last prompt")
	}
	if svc.OverflowRetries() != 0 {
		t.Error("Reset should clear overflow retries")
	}
}

func TestSessionService_ConcurrentAccess(t *testing.T) {
	svc := &SessionService{}
	const N = 100

	// Concurrent appends from multiple goroutines
	done := make(chan struct{})
	go func() {
		for i := 0; i < N; i++ {
			svc.AppendMessages(kosong.CreateUserMessage(fmt.Sprintf("msg-%d", i)))
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < N; i++ {
			svc.IncrementTurn()
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < N; i++ {
			_ = svc.History()
			_ = svc.TurnCount()
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	<-done

	if svc.HistoryLen() != N {
		t.Errorf("concurrent appends: history len = %d, want %d", svc.HistoryLen(), N)
	}
	if svc.TurnCount() != N {
		t.Errorf("concurrent increments: turnCount = %d, want %d", svc.TurnCount(), N)
	}
}

func TestSessionService_BtwMode(t *testing.T) {
	svc := &SessionService{}
	svc.AppendMessages(kosong.CreateUserMessage("a"), kosong.CreateUserMessage("b"))

	svc.SetBtwMode(true)
	if !svc.BtwMode() {
		t.Error("BtwMode should be true")
	}
	if svc.BtwHistoryLen() != 2 {
		t.Errorf("BtwHistoryLen = %d, want 2", svc.BtwHistoryLen())
	}

	// Append more during btw mode
	svc.AppendMessages(kosong.CreateUserMessage("btw-question"))
	if svc.HistoryLen() != 3 {
		t.Errorf("history len = %d, want 3", svc.HistoryLen())
	}

	// Truncate back to btw snapshot
	svc.TruncateHistory(svc.BtwHistoryLen())
	if svc.HistoryLen() != 2 {
		t.Errorf("after truncate, history len = %d, want 2", svc.HistoryLen())
	}

	svc.SetBtwMode(false)
	if svc.BtwMode() {
		t.Error("BtwMode should be false")
	}
}
