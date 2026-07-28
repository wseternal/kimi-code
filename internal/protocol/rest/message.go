package rest

import "github.com/visdomtech/kimi-code/internal/protocol"

// ListMessagesQuery is the query for GET /sessions/{session_id}/messages.
type ListMessagesQuery struct {
	protocol.CursorQuery
	Role string `json:"role,omitempty"` // "user" | "assistant" | "tool" | "system"
}

// ListMessagesResponse is the paginated response for message listing.
type ListMessagesResponse struct {
	Items   []protocol.Message `json:"items"`
	HasMore bool               `json:"has_more"`
}
