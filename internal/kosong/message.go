package kosong

import "encoding/json"

// Role represents the sender of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// TextPart is a text content block.
type TextPart struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// ThinkPart is a model reasoning block.
type ThinkPart struct {
	Type      string  `json:"type"` // "think"
	Think     string  `json:"think"`
	Encrypted *string `json:"encrypted,omitempty"` // Provider-specific reasoning signature
}

// ImageURLPart is an image content block.
type ImageURLPart struct {
	Type     string `json:"type"` // "image_url"
	ImageURL struct {
		URL string  `json:"url"`
		ID  *string `json:"id,omitempty"`
	} `json:"imageUrl"`
}

// AudioURLPart is an audio content block.
type AudioURLPart struct {
	Type     string `json:"type"` // "audio_url"
	AudioURL struct {
		URL string  `json:"url"`
		ID  *string `json:"id,omitempty"`
	} `json:"audioUrl"`
}

// VideoURLPart is a video content block.
type VideoURLPart struct {
	Type     string `json:"type"` // "video_url"
	VideoURL struct {
		URL string  `json:"url"`
		ID  *string `json:"id,omitempty"`
	} `json:"videoUrl"`
}

// ContentPart is a discriminated union of content blocks.
type ContentPart struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Think     string          `json:"think,omitempty"`
	Encrypted *string         `json:"encrypted,omitempty"`
	ImageURL  *ImageURLInner  `json:"imageUrl,omitempty"`
	AudioURL  *AudioURLInner  `json:"audioUrl,omitempty"`
	VideoURL  *VideoURLInner  `json:"videoUrl,omitempty"`
}

type ImageURLInner struct {
	URL string  `json:"url"`
	ID  *string `json:"id,omitempty"`
}

type AudioURLInner struct {
	URL string  `json:"url"`
	ID  *string `json:"id,omitempty"`
}

type VideoURLInner struct {
	URL string  `json:"url"`
	ID  *string `json:"id,omitempty"`
}

// NewTextPart creates a text content part.
func NewTextPart(text string) ContentPart {
	return ContentPart{Type: "text", Text: text}
}

// NewThinkPart creates a thinking content part.
func NewThinkPart(think string) ContentPart {
	return ContentPart{Type: "think", Think: think}
}

// NewImageURLPart creates an image URL content part.
func NewImageURLPart(url string, id *string) ContentPart {
	return ContentPart{Type: "image_url", ImageURL: &ImageURLInner{URL: url, ID: id}}
}

// ToolCall represents a tool invocation requested by the assistant.
type ToolCall struct {
	Type      string                 `json:"type"` // "function"
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments *string                `json:"arguments"`
	Extras    map[string]interface{} `json:"extras,omitempty"`
}

// ToolCallPart is a streaming delta for tool call arguments.
type ToolCallPart struct {
	Type          string      `json:"type"` // "tool_call_part"
	ArgumentsPart *string     `json:"argumentsPart"`
	Index         interface{} `json:"index,omitempty"` // number | string
}

// StreamedMessagePart is a discriminated union of parts yielded during streaming.
type StreamedMessagePart struct {
	Type          string                 `json:"type"`
	Text          string                 `json:"text,omitempty"`
	Think         string                 `json:"think,omitempty"`
	Encrypted     *string                `json:"encrypted,omitempty"`
	ImageURL      *ImageURLInner         `json:"imageUrl,omitempty"`
	AudioURL      *AudioURLInner         `json:"audioUrl,omitempty"`
	VideoURL      *VideoURLInner         `json:"videoUrl,omitempty"`
	ID            string                 `json:"id,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Arguments     *string                `json:"arguments,omitempty"`
	ArgumentsPart *string                `json:"argumentsPart,omitempty"`
	Index         interface{}            `json:"index,omitempty"`
	Extras        map[string]interface{} `json:"extras,omitempty"`
}

// IsContentPart returns true if the part is a content block (text, think, image_url, etc.).
func (p *StreamedMessagePart) IsContentPart() bool {
	switch p.Type {
	case "text", "think", "image_url", "audio_url", "video_url":
		return true
	}
	return false
}

// IsToolCall returns true if the part is a complete tool call.
func (p *StreamedMessagePart) IsToolCall() bool {
	return p.Type == "function"
}

// IsToolCallPart returns true if the part is a streaming tool call argument delta.
func (p *StreamedMessagePart) IsToolCallPart() bool {
	return p.Type == "tool_call_part"
}

// MergeInPlace merges source into target for streaming accumulation.
// Returns true if merge was performed, false otherwise.
func MergeInPlace(target, source *StreamedMessagePart) bool {
	// TextPart + TextPart
	if target.Type == "text" && source.Type == "text" {
		target.Text += source.Text
		return true
	}

	// ThinkPart + ThinkPart
	if target.Type == "think" && source.Type == "think" {
		if target.Encrypted != nil {
			return false
		}
		target.Think += source.Think
		if source.Encrypted != nil {
			target.Encrypted = source.Encrypted
		}
		return true
	}

	// ToolCall + ToolCallPart
	if target.Type == "function" && source.Type == "tool_call_part" {
		if source.ArgumentsPart != nil {
			if target.Arguments == nil {
				target.Arguments = source.ArgumentsPart
			} else {
				combined := *target.Arguments + *source.ArgumentsPart
				target.Arguments = &combined
			}
		}
		return true
	}

	return false
}

// Message represents a single message in a conversation.
type Message struct {
	Role       Role          `json:"role"`
	Name       *string       `json:"name,omitempty"`
	Content    []ContentPart `json:"content"`
	ToolCalls  []ToolCall    `json:"toolCalls"`
	ToolCallID *string       `json:"toolCallId,omitempty"`
	Partial    *bool         `json:"partial,omitempty"`
	Tools      []Tool        `json:"tools,omitempty"`
}

// IsToolDeclarationOnlyMessage returns true for a message whose only payload is tools.
func IsToolDeclarationOnlyMessage(m *Message) bool {
	return len(m.Tools) > 0 && len(m.Content) == 0 && len(m.ToolCalls) == 0
}

// ExtractText extracts concatenated text from a message's content parts.
func ExtractText(m *Message, sep string) string {
	var result string
	first := true
	for _, part := range m.Content {
		if part.Type == "text" {
			if !first && sep != "" {
				result += sep
			}
			result += part.Text
			first = false
		}
	}
	return result
}

// CreateUserMessage creates a simple user message with a single text part.
func CreateUserMessage(content string) Message {
	return Message{
		Role:      RoleUser,
		Content:   []ContentPart{NewTextPart(content)},
		ToolCalls: []ToolCall{},
	}
}

// CreateAssistantMessage creates an assistant message from content parts and optional tool calls.
func CreateAssistantMessage(content []ContentPart, toolCalls []ToolCall) Message {
	if toolCalls == nil {
		toolCalls = []ToolCall{}
	}
	return Message{
		Role:      RoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
	}
}

// CreateToolMessage creates a tool result message.
func CreateToolMessage(toolCallID string, output interface{}) Message {
	var content []ContentPart
	switch v := output.(type) {
	case string:
		content = []ContentPart{NewTextPart(v)}
	case []ContentPart:
		content = v
	default:
		// Fallback: marshal to JSON
		data, _ := json.Marshal(v)
		content = []ContentPart{NewTextPart(string(data))}
	}
	return Message{
		Role:       RoleTool,
		Content:    content,
		ToolCalls:  []ToolCall{},
		ToolCallID: &toolCallID,
	}
}
