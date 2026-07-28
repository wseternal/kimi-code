package goal

import (
	"testing"
)

func TestTrackerSetAndClear(t *testing.T) {
	tr := NewTracker()

	if tr.IsActive() {
		t.Error("new tracker should not be active")
	}

	tr.Set("Build a REST API")
	if !tr.IsActive() {
		t.Error("should be active after Set")
	}
	if tr.Current().Text != "Build a REST API" {
		t.Errorf("Text = %q", tr.Current().Text)
	}

	tr.Clear()
	if tr.IsActive() {
		t.Error("should not be active after Clear")
	}
	if tr.Current() != nil {
		t.Error("Current should be nil after Clear")
	}
}

func TestTrackerCompleteAndAbandon(t *testing.T) {
	tr := NewTracker()

	tr.Set("Test goal")
	tr.Complete()
	if tr.Current().Status != "completed" {
		t.Errorf("Status = %q, want completed", tr.Current().Status)
	}
	if tr.IsActive() {
		t.Error("completed goal should not be active")
	}

	tr.Set("Another goal")
	tr.Abandon()
	if tr.Current().Status != "abandoned" {
		t.Errorf("Status = %q, want abandoned", tr.Current().Status)
	}
}

func TestSystemPromptSuffix(t *testing.T) {
	tr := NewTracker()

	if s := tr.SystemPromptSuffix(); s != "" {
		t.Errorf("empty tracker should return empty suffix, got %q", s)
	}

	tr.Set("Deploy to production")
	suffix := tr.SystemPromptSuffix()
	if suffix == "" {
		t.Error("active goal should return non-empty suffix")
	}
	if !contains(suffix, "Deploy to production") {
		t.Errorf("suffix should contain goal text: %q", suffix)
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

	tr.Set("My goal")
	s := tr.StatusString()
	if !contains(s, "My goal") {
		t.Errorf("StatusString should contain goal text: %q", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
