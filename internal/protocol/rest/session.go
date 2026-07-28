package rest

import "github.com/visdomtech/kimi-code/internal/protocol"

// SessionStatusResponse is the data for GET /sessions/{id}/status.
type SessionStatusResponse struct {
	Busy             bool    `json:"busy"`
	Model            string  `json:"model,omitempty"`
	ThinkingLevel    string  `json:"thinking_level"`
	Permission       string  `json:"permission"`
	PlanMode         bool    `json:"plan_mode"`
	SwarmMode        bool    `json:"swarm_mode"`
	ContextTokens    int     `json:"context_tokens"`
	MaxContextTokens int     `json:"max_context_tokens"`
	ContextUsage     float64 `json:"context_usage"`
}

// ListSessionsQuery is the query parameters for GET /sessions.
type ListSessionsQuery struct {
	protocol.CursorQuery
	Busy           *bool `json:"busy,omitempty"`
	IncludeArchive *bool `json:"include_archive,omitempty"`
	ArchivedOnly   *bool `json:"archived_only,omitempty"`
	ExcludeEmpty   *bool `json:"exclude_empty,omitempty"`
}

// ListSessionChildrenQuery is the query for GET /sessions/{id}/children.
type ListSessionChildrenQuery struct {
	protocol.CursorQuery
	Busy           *bool `json:"busy,omitempty"`
	IncludeArchive *bool `json:"include_archive,omitempty"`
}

// CompactSessionRequest is the body for POST /sessions/{id}:compact.
type CompactSessionRequest struct {
	Instruction string `json:"instruction,omitempty"`
}

// UndoSessionRequest is the body for POST /sessions/{id}:undo.
type UndoSessionRequest struct {
	Count    int `json:"count,omitempty"` // default 1
	PageSize int `json:"page_size,omitempty"`
}

// UndoSessionResponse is the response for POST /sessions/{id}:undo.
type UndoSessionResponse struct {
	Messages protocol.PageResponse[protocol.Message] `json:"messages"`
	Status   SessionStatusResponse                   `json:"status"`
}

// StartBtwSessionResponse is the response for POST /sessions/{id}:btw.
type StartBtwSessionResponse struct {
	AgentID string `json:"agent_id"`
}

// SessionWarning describes a session-level warning.
type SessionWarning struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // "info" | "warning" | "error"
}

// SessionWarningsResponse is the response for session warnings.
type SessionWarningsResponse struct {
	Warnings []SessionWarning `json:"warnings"`
}

// SessionAbortResponse is the response for aborting a session.
type SessionAbortResponse struct {
	Aborted bool `json:"aborted"`
}

// ArchiveSessionResponse is the response for POST /sessions/{id}:archive.
type ArchiveSessionResponse struct {
	Archived bool `json:"archived"`
}
