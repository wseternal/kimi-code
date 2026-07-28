package tools

import (
	"context"
	"encoding/json"
)

// Definition describes a tool for the LLM.
type Definition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Result is the result of a tool execution.
type Result struct {
	Output   string `json:"output"`
	IsError  bool   `json:"isError"`
	Truncate bool   `json:"truncate,omitempty"`
}

// ExecContext provides context for tool execution.
type ExecContext struct {
	SessionID string
	AgentID   string
	WorkDir   string
}

// Tool is the interface that all built-in tools implement.
type Tool interface {
	// Definition returns the tool definition for the LLM.
	Definition() Definition
	// Execute runs the tool with the given input.
	Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error)
}

// ToolHook intercepts tool dispatch for a named tool.
// Mirrors the permission.Policy pattern.
type ToolHook interface {
	// Name identifies this hook for logging/debugging.
	Name() string
	// Wrap returns a replacement Tool that delegates to original,
	// or nil to pass through unchanged.
	Wrap(toolName string, original Tool) Tool
}

// Registry manages available tools.
type Registry struct {
	tools map[string]Tool
	hooks map[string][]ToolHook
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Definition().Name] = tool
}

// Get returns a tool by name. If hooks are registered for this tool,
// they are applied in registration order (fast path: no hooks = return original).
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	if !ok {
		return nil, false
	}
	if hooks := r.hooks[name]; len(hooks) > 0 {
		for _, h := range hooks {
			if wrapped := h.Wrap(name, t); wrapped != nil {
				t = wrapped
			}
		}
	}
	return t, true
}

// RegisterHook registers a hook for a named tool. Hooks are applied in
// registration order when Get is called for that tool.
func (r *Registry) RegisterHook(toolName string, hook ToolHook) {
	if r.hooks == nil {
		r.hooks = make(map[string][]ToolHook)
	}
	r.hooks[toolName] = append(r.hooks[toolName], hook)
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// Definitions returns all tool definitions.
func (r *Registry) Definitions() []Definition {
	result := make([]Definition, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t.Definition())
	}
	return result
}
