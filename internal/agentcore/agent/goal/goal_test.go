package goal

import (
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestTrackerCreateAndCancel(t *testing.T) {
	tr := NewTracker()

	if tr.IsActive() {
		t.Error("new tracker should not be active")
	}

	snap, change, err := tr.CreateGoal("Build a REST API", "", BudgetLimits{}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if !tr.IsActive() {
		t.Error("should be active after CreateGoal")
	}
	if snap.Objective != "Build a REST API" {
		t.Errorf("Objective = %q", snap.Objective)
	}
	if change.Kind != "created" {
		t.Errorf("change.Kind = %q", change.Kind)
	}

	_, change, err = tr.CancelGoal("user")
	if err != nil {
		t.Fatal(err)
	}
	if tr.IsActive() {
		t.Error("should not be active after CancelGoal")
	}
	if tr.Current() != nil {
		t.Error("Current should be nil after CancelGoal")
	}
	if change.Kind != "cancelled" {
		t.Errorf("change.Kind = %q", change.Kind)
	}
}

func TestTrackerPauseAndResume(t *testing.T) {
	tr := NewTracker()
	tr.CreateGoal("Test goal", "", BudgetLimits{}, "user")

	_, change, err := tr.PauseGoal("user")
	if err != nil {
		t.Fatal(err)
	}
	if change.Kind != "paused" {
		t.Errorf("change.Kind = %q", change.Kind)
	}
	if tr.IsActive() {
		t.Error("paused goal should not be active")
	}

	_, change, err = tr.ResumeGoal("user")
	if err != nil {
		t.Fatal(err)
	}
	if change.Kind != "resumed" {
		t.Errorf("change.Kind = %q", change.Kind)
	}
	if !tr.IsActive() {
		t.Error("resumed goal should be active")
	}
}

func TestTrackerCompleteAndBlock(t *testing.T) {
	tr := NewTracker()

	tr.CreateGoal("Test goal", "", BudgetLimits{}, "user")
	snap, change, err := tr.MarkComplete("all done", "model")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != StatusComplete {
		t.Errorf("Status = %q, want complete", snap.Status)
	}
	if change.Kind != "completed" {
		t.Errorf("change.Kind = %q", change.Kind)
	}
	if tr.Current() != nil {
		t.Error("completed goal should be cleared")
	}

	// Test blocked
	tr.CreateGoal("Another goal", "", BudgetLimits{}, "user")
	_, change, err = tr.MarkBlocked("stuck", "model")
	if err != nil {
		t.Fatal(err)
	}
	if change.Kind != "blocked" {
		t.Errorf("change.Kind = %q", change.Kind)
	}
	if tr.IsActive() {
		t.Error("blocked goal should not be active")
	}

	// Can resume from blocked
	_, _, err = tr.ResumeGoal("user")
	if err != nil {
		t.Fatal(err)
	}
	if !tr.IsActive() {
		t.Error("resumed from blocked should be active")
	}
}

func TestBudgetTracking(t *testing.T) {
	tr := NewTracker()
	tr.CreateGoal("Budget goal", "", BudgetLimits{
		TokenBudget: intPtr(1000),
		TurnBudget:  intPtr(5),
	}, "user")

	if tr.IsOverBudget() {
		t.Error("should not be over budget initially")
	}

	tr.RecordTokenUsage(500)
	if tr.IsOverBudget() {
		t.Error("should not be over budget at 500/1000 tokens")
	}

	tr.RecordTokenUsage(600) // 1100 total > 1000 budget
	if !tr.IsOverBudget() {
		t.Error("should be over budget at 1100/1000 tokens")
	}

	snap := tr.Current()
	if snap.Budget == nil {
		t.Fatal("expected budget report")
	}
	if !snap.Budget.TokenBudgetReached {
		t.Error("expected token budget reached")
	}
	if snap.Budget.RemainingTurns == nil || *snap.Budget.RemainingTurns != 5 {
		t.Errorf("expected 5 remaining turns, got %v", snap.Budget.RemainingTurns)
	}
}

func TestTurnCounting(t *testing.T) {
	tr := NewTracker()
	tr.CreateGoal("Turn goal", "", BudgetLimits{TurnBudget: intPtr(3)}, "user")

	tr.IncrementTurn()
	tr.IncrementTurn()
	snap := tr.Current()
	if snap.TurnsUsed != 2 {
		t.Errorf("TurnsUsed = %d, want 2", snap.TurnsUsed)
	}

	tr.IncrementTurn()
	if !tr.IsOverBudget() {
		t.Error("should be over budget at 3/3 turns")
	}
}

func TestSystemPromptSuffix(t *testing.T) {
	tr := NewTracker()

	if s := tr.SystemPromptSuffix(); s != "" {
		t.Errorf("empty tracker should return empty suffix, got %q", s)
	}

	tr.CreateGoal("Deploy to production", "All tests pass", BudgetLimits{}, "user")
	suffix := tr.SystemPromptSuffix()
	if suffix == "" {
		t.Error("active goal should return non-empty suffix")
	}
	if !strings.Contains(suffix, "Deploy to production") {
		t.Errorf("suffix should contain goal text: %q", suffix)
	}
	if !strings.Contains(suffix, "All tests pass") {
		t.Errorf("suffix should contain completion criterion: %q", suffix)
	}

	// Paused goal should not appear in prompt
	tr.PauseGoal("user")
	if s := tr.SystemPromptSuffix(); s != "" {
		t.Errorf("paused goal should return empty suffix, got %q", s)
	}
}

func TestCreateGoalValidation(t *testing.T) {
	tr := NewTracker()

	_, _, err := tr.CreateGoal("", "", BudgetLimits{}, "user")
	if err == nil {
		t.Error("empty objective should return error")
	}

	_, _, err = tr.CreateGoal(strings.Repeat("x", 4001), "", BudgetLimits{}, "user")
	if err == nil {
		t.Error("objective > 4000 chars should return error")
	}
}

func TestParseGoalCommand(t *testing.T) {
	tests := []struct {
		input string
		text  string
		clear bool
	}{
		{"", "", true},
		{"Build API", "Build API", false},
		{"  ", "", true},
	}

	for _, tt := range tests {
		text, clear := ParseGoalCommand(tt.input)
		if text != tt.text || clear != tt.clear {
			t.Errorf("ParseGoalCommand(%q) = (%q, %v), want (%q, %v)",
				tt.input, text, clear, tt.text, tt.clear)
		}
	}
}

func TestStatusString(t *testing.T) {
	tr := NewTracker()
	if s := tr.StatusString(); s != "No goal set" {
		t.Errorf("StatusString = %q", s)
	}

	tr.CreateGoal("My goal", "", BudgetLimits{}, "user")
	s := tr.StatusString()
	if !strings.Contains(s, "My goal") {
		t.Errorf("StatusString should contain goal text: %q", s)
	}
}

func TestPauseNonActive(t *testing.T) {
	tr := NewTracker()
	_, _, err := tr.PauseGoal("user")
	if err == nil {
		t.Error("pausing with no goal should return error")
	}

	tr.CreateGoal("Test", "", BudgetLimits{}, "user")
	tr.PauseGoal("user")
	_, _, err = tr.PauseGoal("user")
	if err == nil {
		t.Error("pausing already paused goal should return error")
	}
}

func TestSetBudgetLimits(t *testing.T) {
	tr := NewTracker()
	tr.CreateGoal("Budget", "", BudgetLimits{}, "user")
	tr.SetBudgetLimits(BudgetLimits{TokenBudget: intPtr(5000)})

	snap := tr.Current()
	if snap.Budget.TokenBudget == nil || *snap.Budget.TokenBudget != 5000 {
		t.Errorf("expected token budget 5000, got %v", snap.Budget.TokenBudget)
	}
}
