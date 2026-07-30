// Package protocol provides ACP (Agent Client Protocol) adapter types (Gap #85).
package protocol

import "time"

// ACPVersion is the supported ACP version.
const ACPVersion = "0.1.0"

// ACPSession represents an ACP session.
type ACPSession struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"` // "active", "closed"
}

// ACPMessage represents an ACP message.
type ACPMessage struct {
	Role    string `json:"role"` // "user", "assistant", "system"
	Content string `json:"content"`
}

// ACPToolCall represents an ACP tool invocation.
type ACPToolCall struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Input  any    `json:"input"`
	Result any    `json:"result,omitempty"`
	Status string `json:"status"` // "pending", "running", "completed", "failed"
}

// ACPRequest is the base ACP request.
type ACPRequest struct {
	Method string `json:"method"`
	ID     string `json:"id"`
	Params any    `json:"params,omitempty"`
}

// ACPResponse is the base ACP response.
type ACPResponse struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ACPInitializeParams is the initialize request params.
type ACPInitializeParams struct {
	ClientInfo    ACPClientInfo    `json:"client_info"`
	Capabilities  ACPCapabilities  `json:"capabilities"`
}

// ACPClientInfo identifies the client.
type ACPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ACPCapabilities describes client capabilities.
type ACPCapabilities struct {
	Tools     bool `json:"tools"`
	Streaming bool `json:"streaming"`
}

// ACPInitializeResult is the initialize response.
type ACPInitializeResult struct {
	ServerInfo   ACPServerInfo   `json:"server_info"`
	Capabilities ACPServerCapabilities `json:"capabilities"`
}

// ACPServerInfo identifies the server.
type ACPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ACPServerCapabilities describes server capabilities.
type ACPServerCapabilities struct {
	Tools     []ACPToolDef `json:"tools,omitempty"`
	Streaming bool         `json:"streaming"`
}

// ACPToolDef defines a tool for ACP.
type ACPToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema,omitempty"`
}

// ACPAdapter translates between ACP and internal protocol.
type ACPAdapter struct {
	sessions map[string]*ACPSession
}

// NewACPAdapter creates an ACP adapter.
func NewACPAdapter() *ACPAdapter {
	return &ACPAdapter{
		sessions: make(map[string]*ACPSession),
	}
}

// CreateSession creates an ACP session.
func (a *ACPAdapter) CreateSession(id string) *ACPSession {
	sess := &ACPSession{
		ID:        id,
		CreatedAt: time.Now(),
		Status:    "active",
	}
	a.sessions[id] = sess
	return sess
}

// GetSession returns an ACP session.
func (a *ACPAdapter) GetSession(id string) (*ACPSession, bool) {
	sess, ok := a.sessions[id]
	return sess, ok
}

// CloseSession closes an ACP session.
func (a *ACPAdapter) CloseSession(id string) {
	if sess, ok := a.sessions[id]; ok {
		sess.Status = "closed"
	}
}
