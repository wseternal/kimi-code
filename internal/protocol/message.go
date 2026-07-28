package protocol

import "encoding/json"

// MessageRole enumerates who produced a message.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleSystem    MessageRole = "system"
)

// MessageContent is the discriminated union of message content parts.
// The Type field selects the variant.
type MessageContent struct {
	Type string `json:"type"`

	// TextContent (type: "text")
	Text string `json:"text,omitempty"`

	// ToolUseContent (type: "tool_use")
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`

	// ToolResultContent (type: "tool_result")
	Output  json.RawMessage `json:"output,omitempty"`
	IsError bool            `json:"is_error,omitempty"`

	// ImageContent (type: "image")
	Source *ImageSource `json:"source,omitempty"`

	// FileContent (type: "file")
	FileID    string `json:"file_id,omitempty"`
	Name      string `json:"name,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Size      int    `json:"size,omitempty"`

	// ThinkingContent (type: "thinking")
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// ImageSourceKind discriminates image/video source types.
type ImageSourceKind string

const (
	ImageSourceURL    ImageSourceKind = "url"
	ImageSourceBase64 ImageSourceKind = "base64"
	ImageSourceFile   ImageSourceKind = "file"
)

// ImageSource is the source for image or video content.
type ImageSource struct {
	Kind      ImageSourceKind `json:"kind"`
	URL       string          `json:"url,omitempty"`
	MediaType string          `json:"media_type,omitempty"`
	Data      string          `json:"data,omitempty"`
	FileID    string          `json:"file_id,omitempty"`
}

// Message is a chat message within a session.
type Message struct {
	ID              string            `json:"id"`
	SessionID       string            `json:"session_id"`
	Role            MessageRole       `json:"role"`
	Content         []MessageContent  `json:"content"`
	CreatedAt       string            `json:"created_at"`
	PromptID        string            `json:"prompt_id,omitempty"`
	ParentMessageID string            `json:"parent_message_id,omitempty"`
	Metadata        map[string]json.RawMessage `json:"metadata,omitempty"`
}
