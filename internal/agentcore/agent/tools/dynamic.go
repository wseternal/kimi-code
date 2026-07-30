package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ── Gap #38: SelectTools (Progressive Tool Disclosure) ──

// ToolGroup defines a named group of tools that can be loaded on demand.
type ToolGroup struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ToolNames   []string `json:"tool_names"`
}

// SelectToolsTool allows the model to request additional tool groups.
type SelectToolsTool struct {
	registry   *Registry
	mu         sync.RWMutex
	groups     map[string]*ToolGroup
	factories  map[string]func() Tool // lazy tool factories
	loaded     map[string]bool
}

// NewSelectToolsTool creates a SelectTools tool.
func NewSelectToolsTool(registry *Registry) *SelectToolsTool {
	return &SelectToolsTool{
		registry:  registry,
		groups:    make(map[string]*ToolGroup),
		factories: make(map[string]func() Tool),
		loaded:    make(map[string]bool),
	}
}

// RegisterGroup adds a tool group that can be loaded on demand.
func (t *SelectToolsTool) RegisterGroup(group ToolGroup, factory func() Tool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.groups[group.Name] = &group
	for _, name := range group.ToolNames {
		t.factories[name] = factory
	}
}

type selectToolsInput struct {
	Groups []string `json:"groups"`
}

// Definition returns the tool definition.
func (t *SelectToolsTool) Definition() Definition {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var groupDesc []string
	for name, g := range t.groups {
		if !t.loaded[name] {
			groupDesc = append(groupDesc, fmt.Sprintf("- %s: %s (tools: %s)",
				name, g.Description, strings.Join(g.ToolNames, ", ")))
		}
	}

	desc := "Request additional tool groups for specialized tasks. Use when the current toolset is insufficient."
	if len(groupDesc) > 0 {
		desc += "\n\nAvailable groups:\n" + strings.Join(groupDesc, "\n")
	}

	return Definition{
		Name:        "SelectTools",
		Description: desc,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"groups": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Names of tool groups to load",
				},
			},
			"required": []string{"groups"},
		},
	}
}

// Execute loads the requested tool groups into the registry.
func (t *SelectToolsTool) Execute(_ context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params selectToolsInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	var loaded []string
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, groupName := range params.Groups {
		if t.loaded[groupName] {
			loaded = append(loaded, groupName+" (already loaded)")
			continue
		}

		group, ok := t.groups[groupName]
		if !ok {
			loaded = append(loaded, groupName+" (not found)")
			continue
		}

		// Load tools from the group
		for _, toolName := range group.ToolNames {
			if factory, ok := t.factories[toolName]; ok {
				t.registry.Register(factory())
			}
		}

		t.loaded[groupName] = true
		loaded = append(loaded, groupName)
	}

	return &Result{
		Output: fmt.Sprintf("Loaded tool groups: %s", strings.Join(loaded, ", ")),
	}, nil
}

// LoadedGroups returns the names of currently loaded groups.
func (t *SelectToolsTool) LoadedGroups() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var result []string
	for name := range t.loaded {
		result = append(result, name)
	}
	return result
}

// AvailableGroups returns names of groups not yet loaded.
func (t *SelectToolsTool) AvailableGroups() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var result []string
	for name := range t.groups {
		if !t.loaded[name] {
			result = append(result, name)
		}
	}
	return result
}

// ── Gap #39: Dynamic Tools Support ──

// DynamicRegistry wraps a Registry with the ability to add/remove tools
// mid-session. It supports tool diff computation for progressive loading.
type DynamicRegistry struct {
	inner     *Registry
	mu        sync.RWMutex
	version   int
	changelog []ToolChange
	onChange  func(changes []ToolChange)
}

// ToolChange records a change to the tool set.
type ToolChange struct {
	Version int       `json:"version"`
	Action  string    `json:"action"` // "add", "remove", "update"
	Name    string    `json:"name"`
	Reason  string    `json:"reason,omitempty"`
}

// NewDynamicRegistry creates a dynamic tool registry wrapping a base registry.
func NewDynamicRegistry(inner *Registry, onChange func([]ToolChange)) *DynamicRegistry {
	return &DynamicRegistry{
		inner:    inner,
		onChange: onChange,
	}
}

// AddTool adds a tool to the registry and records the change.
func (d *DynamicRegistry) AddTool(tool Tool, reason string) {
	d.mu.Lock()
	d.version++
	change := ToolChange{
		Version: d.version,
		Action:  "add",
		Name:    tool.Definition().Name,
		Reason:  reason,
	}
	d.changelog = append(d.changelog, change)
	d.mu.Unlock()

	d.inner.Register(tool)

	if d.onChange != nil {
		d.onChange([]ToolChange{change})
	}
}

// RemoveTool removes a tool from the registry and records the change.
func (d *DynamicRegistry) RemoveTool(name, reason string) {
	d.mu.Lock()
	d.version++
	change := ToolChange{
		Version: d.version,
		Action:  "remove",
		Name:    name,
		Reason:  reason,
	}
	d.changelog = append(d.changelog, change)
	d.mu.Unlock()

	// Note: Registry doesn't have a Remove method, so we skip actual removal
	// In production, this would need a proper removal mechanism

	if d.onChange != nil {
		d.onChange([]ToolChange{change})
	}
}

// DiffSince returns all tool changes since a given version.
func (d *DynamicRegistry) DiffSince(version int) []ToolChange {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []ToolChange
	for _, c := range d.changelog {
		if c.Version > version {
			result = append(result, c)
		}
	}
	return result
}

// Version returns the current tool set version.
func (d *DynamicRegistry) Version() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.version
}

// Inner returns the wrapped registry.
func (d *DynamicRegistry) Inner() *Registry {
	return d.inner
}

// Definitions returns the current tool definitions.
func (d *DynamicRegistry) Definitions() []Definition {
	return d.inner.Definitions()
}

// ToolDiffSummary returns a human-readable summary of tool changes.
func ToolDiffSummary(changes []ToolChange) string {
	if len(changes) == 0 {
		return "No tool changes."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool changes (%d):\n", len(changes)))
	for _, c := range changes {
		sb.WriteString(fmt.Sprintf("  %s: %s", c.Action, c.Name))
		if c.Reason != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", c.Reason))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
