package injection

import (
	"strings"
	"testing"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/goal"
)

func TestManagerInjectAll(t *testing.T) {
	tracker := goal.NewTracker()
	gi := NewGoalInjector(tracker)
	pm := NewPlanModeInjector()
	td := NewToolsDiffInjector()

	mgr := NewManager(gi, pm, td)

	// Nothing active → empty
	if got := mgr.InjectAll(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// Enable plan mode
	pm.SetPlanMode(true, "1. Do X\n2. Do Y")
	got := mgr.InjectAll()
	if !strings.Contains(got, "PLAN MODE") {
		t.Error("expected PLAN MODE in injection")
	}
	if !strings.Contains(got, "Do X") {
		t.Error("expected plan text in injection")
	}

	// Add a goal
	tracker.CreateGoal("Build REST API", "", goal.BudgetLimits{}, "user")
	got = mgr.InjectAll()
	if !strings.Contains(got, "Build REST API") {
		t.Error("expected goal objective in injection")
	}

	// Add tools
	td.SetLoadedTools([]string{"SearchCodebase", "ReadFile"})
	got = mgr.InjectAll()
	if !strings.Contains(got, "SearchCodebase") {
		t.Error("expected loaded tool name in injection")
	}
}

func TestManagerInjectByVariant(t *testing.T) {
	pm := NewPlanModeInjector()
	mgr := NewManager(pm)

	pm.SetPlanMode(true, "plan text")
	got := mgr.InjectByVariant(VariantPlanMode)
	if !strings.Contains(got, "plan text") {
		t.Errorf("expected plan text, got %q", got)
	}

	got = mgr.InjectByVariant(VariantGoal)
	if got != "" {
		t.Errorf("expected empty for missing variant, got %q", got)
	}
}

func TestManagerOnContextClear(t *testing.T) {
	pm := NewPlanModeInjector()
	td := NewToolsDiffInjector()
	tl := NewTodoListInjector()

	mgr := NewManager(pm, td, tl)

	pm.SetPlanMode(true, "some plan")
	td.SetLoadedTools([]string{"A"})
	tl.SetItems([]TodoItem{{ID: "1", Content: "task", Status: "pending"}})

	mgr.OnContextClear()

	// PlanMode keeps enabled but clears planText
	pmGot := pm.GetInjection()
	if !strings.Contains(pmGot, "PLAN MODE") {
		t.Error("plan mode should still be enabled after clear")
	}
	if strings.Contains(pmGot, "some plan") {
		t.Error("plan text should be cleared")
	}

	// ToolsDiff cleared
	if got := td.GetInjection(); got != "" {
		t.Errorf("tools diff should be empty after clear, got %q", got)
	}

	// TodoList cleared
	if got := tl.GetInjection(); got != "" {
		t.Errorf("todo list should be empty after clear, got %q", got)
	}
}

func TestManagerOnContextCompacted(t *testing.T) {
	td := NewToolsDiffInjector()
	mgr := NewManager(td)
	td.SetLoadedTools([]string{"X"})
	mgr.OnContextCompacted()
	if got := td.GetInjection(); got != "" {
		t.Errorf("tools diff should be empty after compaction, got %q", got)
	}
}

func TestTodoListInjector(t *testing.T) {
	tl := NewTodoListInjector()

	// Empty
	if got := tl.GetInjection(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	tl.SetItems([]TodoItem{
		{ID: "1", Content: "done task", Status: "complete"},
		{ID: "2", Content: "pending task", Status: "pending"},
		{ID: "3", Content: "wip task", Status: "in_progress"},
	})
	got := tl.GetInjection()
	if !strings.Contains(got, "pending task") {
		t.Error("expected pending task")
	}
	if !strings.Contains(got, "wip task") {
		t.Error("expected wip task")
	}
	if strings.Contains(got, "done task") {
		t.Error("should not contain completed task")
	}
}

func TestPermissionModeInjector(t *testing.T) {
	pm := NewPermissionModeInjector()

	// Default (manual) → no injection
	if got := pm.GetInjection(); got != "" {
		t.Errorf("manual mode should produce no injection, got %q", got)
	}

	pm.SetMode("yolo")
	if got := pm.GetInjection(); !strings.Contains(got, "YOLO") {
		t.Errorf("expected YOLO, got %q", got)
	}

	pm.SetMode("auto")
	if got := pm.GetInjection(); !strings.Contains(got, "AUTO") {
		t.Errorf("expected AUTO, got %q", got)
	}
}

func TestGoalInjector(t *testing.T) {
	tracker := goal.NewTracker()
	gi := NewGoalInjector(tracker)

	// No goal → empty
	if got := gi.GetInjection(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	tracker.CreateGoal("Test goal", "", goal.BudgetLimits{}, "user")
	got := gi.GetInjection()
	if !strings.Contains(got, "Test goal") {
		t.Errorf("expected goal text, got %q", got)
	}
}

func TestPluginSessionStartInjector(t *testing.T) {
	ps := NewPluginSessionStartInjector()

	if got := ps.GetInjection(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	ps.SetInstructions("Start the MCP server")
	if got := ps.GetInjection(); !strings.Contains(got, "MCP server") {
		t.Errorf("expected MCP server text, got %q", got)
	}

	ps.OnContextClear()
	if got := ps.GetInjection(); got != "" {
		t.Errorf("expected empty after clear, got %q", got)
	}
}
