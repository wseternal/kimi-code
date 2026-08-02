package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/tools"
)

// ToolAdapter wraps an MCP tool as a tools.Tool for the agent registry.
type ToolAdapter struct {
	client     Client
	serverName string
	def        ToolDefinition
	toolName   string // qualified name: mcp__<server>__<tool>
}

// NewToolAdapter creates an adapter for a single MCP tool.
func NewToolAdapter(client Client, serverName string, def ToolDefinition) *ToolAdapter {
	return &ToolAdapter{
		client:     client,
		serverName: serverName,
		def:        def,
		toolName:   QualifyToolName(serverName, def.Name),
	}
}

// Definition returns the tool definition for the LLM.
func (a *ToolAdapter) Definition() tools.Definition {
	params := a.def.InputSchema
	if params == nil {
		params = map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
	return tools.Definition{
		Name:        a.toolName,
		Description: a.def.Description,
		Parameters:  params,
	}
}

// Execute calls the MCP tool via the client.
func (a *ToolAdapter) Execute(ctx context.Context, input json.RawMessage, _ tools.ExecContext) (*tools.Result, error) {
	var args map[string]any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("mcp adapter: parse input: %w", err)
		}
	}

	result, err := a.client.CallTool(ctx, a.def.Name, args)
	if err != nil {
		return &tools.Result{Output: err.Error(), IsError: true}, nil
	}

	return &tools.Result{
		Output:  result.Text(),
		IsError: result.IsError,
	}, nil
}

// QualifyToolName creates a qualified MCP tool name.
// Format: mcp__<server>__<tool>
func QualifyToolName(serverName, toolName string) string {
	// Sanitize names: replace non-alphanumeric chars with underscore
	server := sanitize(serverName)
	tool := sanitize(toolName)
	return fmt.Sprintf("mcp__%s__%s", server, tool)
}

// sanitize replaces non-alphanumeric characters with underscores.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// RegisterServerTools discovers tools from an MCP server and registers
// them into the given tool registry. Returns the number of tools registered.
func RegisterServerTools(ctx context.Context, client Client, serverName string, registry *tools.Registry, enabledTools []string) (int, error) {
	defs, err := client.ListTools(ctx)
	if err != nil {
		return 0, fmt.Errorf("list tools from %s: %w", serverName, err)
	}

	// Build enabled set for filtering
	enabledSet := make(map[string]bool)
	for _, t := range enabledTools {
		enabledSet[t] = true
	}

	count := 0
	for _, def := range defs {
		// Filter by enabled tools list (empty = all enabled)
		if len(enabledSet) > 0 && !enabledSet[def.Name] {
			continue
		}
		adapter := NewToolAdapter(client, serverName, def)
		registry.Register(adapter)
		count++
	}
	return count, nil
}
