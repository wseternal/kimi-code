// Package injection implements system-prompt injection management.
// The InjectionManager coordinates 6 independent injectors that provide
// context-aware content to the system prompt (goal status, plan mode,
// permission mode, todo reminders, tool set changes, plugin session start).
package injection

import (
	"fmt"
	"strings"
	"sync"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/goal"
)

// Variant identifies an injection source.
type Variant string

const (
	VariantGoal               Variant = "goal"
	VariantPlanMode           Variant = "plan_mode"
	VariantPermissionMode     Variant = "permission_mode"
	VariantTodoList           Variant = "todo_list"
	VariantToolsDiff          Variant = "tools_diff"
	VariantPluginSessionStart Variant = "plugin_session_start"
	VariantBackgroundTasks    Variant = "background_tasks"
)

// Injector is the interface for a single injection source.
type Injector interface {
	// Variant returns the injection source identifier.
	Variant() Variant
	// GetInjection returns the injection text, or empty string if nothing to inject.
	GetInjection() string
	// OnContextClear resets injection state when context is cleared.
	OnContextClear()
	// OnContextCompacted resets injection state after compaction.
	OnContextCompacted()
}

// Manager coordinates all injectors and produces combined injection text.
type Manager struct {
	mu       sync.RWMutex
	injectors []Injector
}

// NewManager creates an injection manager with the given injectors.
func NewManager(injectors ...Injector) *Manager {
	return &Manager{injectors: injectors}
}

// InjectAll returns the combined injection text from all per-step injectors.
func (m *Manager) InjectAll() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var parts []string
	for _, inj := range m.injectors {
		text := inj.GetInjection()
		if text != "" {
			parts = append(parts, wrapSystemReminder(string(inj.Variant()), text))
		}
	}
	return strings.Join(parts, "\n\n")
}

// InjectByVariant returns the injection text for a specific injector.
func (m *Manager) InjectByVariant(v Variant) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, inj := range m.injectors {
		if inj.Variant() == v {
			return inj.GetInjection()
		}
	}
	return ""
}

// OnContextClear forwards the clear signal to all injectors.
func (m *Manager) OnContextClear() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, inj := range m.injectors {
		inj.OnContextClear()
	}
}

// OnContextCompacted forwards the compaction signal to all injectors.
func (m *Manager) OnContextCompacted() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, inj := range m.injectors {
		inj.OnContextCompacted()
	}
}

// ── Goal Injector ──

// GoalInjector injects the current goal status.
type GoalInjector struct {
	tracker *goal.Tracker
}

func NewGoalInjector(tracker *goal.Tracker) *GoalInjector {
	return &GoalInjector{tracker: tracker}
}

func (g *GoalInjector) Variant() Variant { return VariantGoal }
func (g *GoalInjector) GetInjection() string {
	return g.tracker.SystemPromptSuffix()
}
func (g *GoalInjector) OnContextClear()     {}
func (g *GoalInjector) OnContextCompacted() {}

// ── Plan Mode Injector ──

// PlanModeInjector injects the current plan mode status.
type PlanModeInjector struct {
	mu       sync.RWMutex
	enabled  bool
	planText string
}

func NewPlanModeInjector() *PlanModeInjector {
	return &PlanModeInjector{}
}

func (p *PlanModeInjector) Variant() Variant { return VariantPlanMode }
func (p *PlanModeInjector) GetInjection() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.enabled {
		return ""
	}
	text := "You are currently in PLAN MODE. Do not make any code changes or execute any tools that modify files. " +
		"Focus on analyzing, planning, and discussing the approach. " +
		"Use ExitPlanMode when ready to switch back to implementation."
	if p.planText != "" {
		text += "\n\nCurrent plan:\n" + p.planText
	}
	return text
}

func (p *PlanModeInjector) SetPlanMode(enabled bool, planText string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
	p.planText = planText
}

func (p *PlanModeInjector) OnContextClear() {
	p.mu.Lock()
	p.planText = ""
	p.mu.Unlock()
}
func (p *PlanModeInjector) OnContextCompacted() {}

// ── Permission Mode Injector ──

// PermissionModeInjector injects the current permission mode.
type PermissionModeInjector struct {
	mu   sync.RWMutex
	mode string // "manual", "yolo", "auto"
}

func NewPermissionModeInjector() *PermissionModeInjector {
	return &PermissionModeInjector{mode: "manual"}
}

func (p *PermissionModeInjector) Variant() Variant { return VariantPermissionMode }
func (p *PermissionModeInjector) GetInjection() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch p.mode {
	case "yolo":
		return "Permission mode: YOLO — all tool executions are auto-approved."
	case "auto":
		return "Permission mode: AUTO — safe operations are auto-approved, risky operations require confirmation."
	default:
		return "" // manual is the default, no injection needed
	}
}

func (p *PermissionModeInjector) SetMode(mode string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = mode
}
func (p *PermissionModeInjector) OnContextClear()     {}
func (p *PermissionModeInjector) OnContextCompacted() {}

// ── Todo List Injector ──

// TodoListInjector injects pending todo items as a reminder.
type TodoListInjector struct {
	mu    sync.RWMutex
	items []TodoItem
}

// TodoItem is a single task in the todo list.
type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"` // "pending", "in_progress", "complete", "cancelled"
}

func NewTodoListInjector() *TodoListInjector {
	return &TodoListInjector{}
}

func (t *TodoListInjector) Variant() Variant { return VariantTodoList }
func (t *TodoListInjector) GetInjection() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var pending []TodoItem
	for _, item := range t.items {
		if item.Status == "pending" || item.Status == "in_progress" {
			pending = append(pending, item)
		}
	}
	if len(pending) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Current tasks:\n")
	for _, item := range pending {
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", item.Status, item.ID, item.Content))
	}
	return sb.String()
}

func (t *TodoListInjector) SetItems(items []TodoItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items = items
}

func (t *TodoListInjector) OnContextClear() {
	t.mu.Lock()
	t.items = nil
	t.mu.Unlock()
}
func (t *TodoListInjector) OnContextCompacted() {}

// ── Tools Diff Injector ──

// ToolsDiffInjector injects information about dynamically loaded tools.
type ToolsDiffInjector struct {
	mu          sync.RWMutex
	loadedTools []string
}

func NewToolsDiffInjector() *ToolsDiffInjector {
	return &ToolsDiffInjector{}
}

func (t *ToolsDiffInjector) Variant() Variant { return VariantToolsDiff }
func (t *ToolsDiffInjector) GetInjection() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.loadedTools) == 0 {
		return ""
	}
	return "Newly available tools: " + strings.Join(t.loadedTools, ", ")
}

func (t *ToolsDiffInjector) SetLoadedTools(tools []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadedTools = tools
}

func (t *ToolsDiffInjector) OnContextClear() {
	t.mu.Lock()
	t.loadedTools = nil
	t.mu.Unlock()
}
func (t *ToolsDiffInjector) OnContextCompacted() {
	t.mu.Lock()
	t.loadedTools = nil
	t.mu.Unlock()
}

// ── Plugin Session Start Injector ──

// PluginSessionStartInjector injects plugin session start instructions.
type PluginSessionStartInjector struct {
	mu       sync.RWMutex
	instructions string
}

func NewPluginSessionStartInjector() *PluginSessionStartInjector {
	return &PluginSessionStartInjector{}
}

func (p *PluginSessionStartInjector) Variant() Variant { return VariantPluginSessionStart }
func (p *PluginSessionStartInjector) GetInjection() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.instructions
}

func (p *PluginSessionStartInjector) SetInstructions(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instructions = text
}

func (p *PluginSessionStartInjector) OnContextClear() {
	p.mu.Lock()
	p.instructions = ""
	p.mu.Unlock()
}
func (p *PluginSessionStartInjector) OnContextCompacted() {}

// ── Background Tasks Injector ──

// BackgroundTasksInjector injects information about running background tasks.
type BackgroundTasksInjector struct {
	mu    sync.RWMutex
	tasks []string
}

func NewBackgroundTasksInjector() *BackgroundTasksInjector {
	return &BackgroundTasksInjector{}
}

func (b *BackgroundTasksInjector) Variant() Variant { return VariantBackgroundTasks }
func (b *BackgroundTasksInjector) GetInjection() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.tasks) == 0 {
		return ""
	}
	return "Running background tasks:\n" + strings.Join(b.tasks, "\n")
}

func (b *BackgroundTasksInjector) SetTasks(tasks []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tasks = tasks
}

func (b *BackgroundTasksInjector) OnContextClear() {
	b.mu.Lock()
	b.tasks = nil
	b.mu.Unlock()
}
func (b *BackgroundTasksInjector) OnContextCompacted() {}

// ── Helpers ──

// wrapSystemReminder wraps injection text in a system-reminder tag.
func wrapSystemReminder(variant, text string) string {
	return fmt.Sprintf("<system-reminder source=\"%s\">\n%s\n</system-reminder>", variant, text)
}
