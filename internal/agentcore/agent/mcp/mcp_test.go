package mcp

import (
	"context"
	"testing"
)

func TestQualifyToolName(t *testing.T) {
	tests := []struct {
		server, tool, want string
	}{
		{"myserver", "read_file", "mcp__myserver__read_file"},
		{"my-server", "list-dir", "mcp__my_server__list_dir"},
		{"server.name", "tool.v2", "mcp__server_name__tool_v2"},
		{"simple", "simple", "mcp__simple__simple"},
	}
	for _, tc := range tests {
		got := QualifyToolName(tc.server, tc.tool)
		if got != tc.want {
			t.Errorf("QualifyToolName(%q, %q) = %q, want %q", tc.server, tc.tool, got, tc.want)
		}
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"hello-world", "hello_world"},
		{"foo.bar", "foo_bar"},
		{"abc123", "abc123"},
		{"has spaces", "has_spaces"},
	}
	for _, tc := range tests {
		got := sanitize(tc.input)
		if got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestToolResultText(t *testing.T) {
	tests := []struct {
		name   string
		result ToolResult
		want   string
	}{
		{
			"single text",
			ToolResult{Content: []ToolResultContent{{Type: "text", Text: "hello"}}},
			"hello",
		},
		{
			"multiple text",
			ToolResult{Content: []ToolResultContent{{Type: "text", Text: "a"}, {Type: "text", Text: "b"}}},
			"a\nb",
		},
		{
			"error no content",
			ToolResult{IsError: true},
			"error (no text content)",
		},
		{
			"empty",
			ToolResult{},
			"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.result.Text()
			if got != tc.want {
				t.Errorf("Text() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToolAdapterDefinition(t *testing.T) {
	client := &mockClient{}
	adapter := NewToolAdapter(client, "testserver", ToolDefinition{
		Name:        "read",
		Description: "Read a file",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{"type": "string"},
			},
		},
	})

	def := adapter.Definition()
	if def.Name != "mcp__testserver__read" {
		t.Errorf("Definition().Name = %q, want %q", def.Name, "mcp__testserver__read")
	}
	if def.Description != "Read a file" {
		t.Errorf("Definition().Description = %q", def.Description)
	}
}

// mockClient is a test double for the Client interface.
type mockClient struct {
	tools []ToolDefinition
	err   error
}

func (m *mockClient) Initialize(_ context.Context) error { return nil }
func (m *mockClient) ListTools(_ context.Context) ([]ToolDefinition, error) {
	return m.tools, m.err
}
func (m *mockClient) CallTool(_ context.Context, _ string, _ map[string]any) (*ToolResult, error) {
	return &ToolResult{Content: []ToolResultContent{{Type: "text", Text: "mock result"}}}, nil
}
func (m *mockClient) Ping(_ context.Context) error { return nil }
func (m *mockClient) Close() error            { return nil }
