// Package mcp implements the Model Context Protocol client for connecting
// to external MCP servers. Supports stdio, HTTP, and SSE transports.
package mcp

import (
	"context"
	"encoding/json"
)

// ToolDefinition describes a tool exposed by an MCP server.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ToolResult is the result of calling an MCP tool.
type ToolResult struct {
	Content []ToolResultContent `json:"content,omitempty"`
	IsError bool                `json:"isError,omitempty"`
}

// ToolResultContent is a single content block in a tool result.
type ToolResultContent struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// Text returns the concatenated text content of the result.
func (r *ToolResult) Text() string {
	var parts []string
	for _, c := range r.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	if len(parts) == 0 && r.IsError {
		return "error (no text content)"
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}

// Client is the interface for an MCP server connection.
type Client interface {
	// Initialize performs the MCP handshake with the server.
	Initialize(ctx context.Context) error
	// ListTools returns all tools the server exposes.
	ListTools(ctx context.Context) ([]ToolDefinition, error)
	// CallTool invokes a tool on the server.
	CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error)
	// Ping checks the server is alive.
	Ping(ctx context.Context) error
	// Close shuts down the connection.
	Close() error
}

// ServerInfo is metadata returned during initialization.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}
