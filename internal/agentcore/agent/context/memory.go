// Package context implements conversation context management.
// memory.go defines ContextMemory, the message history manager.
package context

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

// PromptOrigin identifies the source of a message.
type PromptOrigin struct {
	Variant string `json:"variant"` // "user", "injection", "shell_command", "compaction", "system_trigger", etc.
	Note    string `json:"note,omitempty"`
}

// ContextMessage extends kosong.Message with metadata for lifecycle management.
type ContextMessage struct {
	kosong.Message
	Origin    *PromptOrigin `json:"origin,omitempty"`
	IsError   bool          `json:"isError,omitempty"`
	Note      string        `json:"note,omitempty"`
	TokenCost int           `json:"-"` // cached per-message token estimate
}

// ContextMemory manages the full message history with append, undo, clear,
// import, token counting, and projector integration.
type ContextMemory struct {
	mu       sync.RWMutex
	history  []ContextMessage
	maxInput int // max input tokens before overflow (0 = unlimited)

	// Token tracking: real count up to coveredIdx, estimated beyond
	tokenCount    int
	coveredIdx    int // messages[:coveredIdx] have real token counts
	lastAssistant *int64
}

// NewContextMemory creates a new context memory with optional max input token limit.
func NewContextMemory(maxInputTokens int) *ContextMemory {
	return &ContextMemory{
		maxInput: maxInputTokens,
	}
}

// AppendUserMessage appends a user message with optional origin.
func (cm *ContextMemory) AppendUserMessage(content []kosong.ContentPart, origin *PromptOrigin) {
	msg := ContextMessage{
		Message: kosong.Message{
			Role:    kosong.RoleUser,
			Content: content,
		},
		Origin: origin,
	}
	msg.TokenCost = EstimateTokensForMessage(msg.Message)
	cm.mu.Lock()
	cm.history = append(cm.history, msg)
	cm.mu.Unlock()
}

// AppendUserText is a convenience to append a plain-text user message.
func (cm *ContextMemory) AppendUserText(text string, origin *PromptOrigin) {
	cm.AppendUserMessage([]kosong.ContentPart{kosong.NewTextPart(text)}, origin)
}

// AppendAssistantMessage appends an assistant message (with optional tool calls).
func (cm *ContextMemory) AppendAssistantMessage(content []kosong.ContentPart, toolCalls []kosong.ToolCall) {
	msg := ContextMessage{
		Message: kosong.Message{
			Role:      kosong.RoleAssistant,
			Content:   content,
			ToolCalls: toolCalls,
		},
		Origin: &PromptOrigin{Variant: "assistant"},
	}
	msg.TokenCost = EstimateTokensForMessage(msg.Message)
	cm.mu.Lock()
	cm.history = append(cm.history, msg)
	cm.mu.Unlock()
}

// AppendToolResult appends a tool result message.
func (cm *ContextMemory) AppendToolResult(toolCallID, output string, isError bool) {
	msg := ContextMessage{
		Message: kosong.CreateToolMessage(toolCallID, output),
		Origin:  &PromptOrigin{Variant: "tool_result"},
		IsError: isError,
	}
	msg.TokenCost = EstimateTokensForMessage(msg.Message)
	cm.mu.Lock()
	cm.history = append(cm.history, msg)
	cm.mu.Unlock()
}

// AppendSystemReminder appends a system reminder as a user-role message.
func (cm *ContextMemory) AppendSystemReminder(text string) {
	wrapped := fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>", text)
	cm.AppendUserMessage(
		[]kosong.ContentPart{kosong.NewTextPart(wrapped)},
		&PromptOrigin{Variant: "injection"},
	)
}

// AppendMessage appends a raw ContextMessage.
func (cm *ContextMemory) AppendMessage(msg ContextMessage) {
	if msg.TokenCost == 0 {
		msg.TokenCost = EstimateTokensForMessage(msg.Message)
	}
	cm.mu.Lock()
	cm.history = append(cm.history, msg)
	cm.mu.Unlock()
}

// History returns a copy of the message history.
func (cm *ContextMemory) History() []ContextMessage {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]ContextMessage, len(cm.history))
	copy(result, cm.history)
	return result
}

// Messages returns the raw kosong.Message slice for provider calls.
func (cm *ContextMemory) Messages() []kosong.Message {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]kosong.Message, len(cm.history))
	for i, m := range cm.history {
		result[i] = m.Message
	}
	return result
}

// Len returns the number of messages.
func (cm *ContextMemory) Len() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.history)
}

// TokenCount returns the estimated total token count.
// Uses real counts for covered messages, estimates for the rest.
func (cm *ContextMemory) TokenCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.tokenCountWithPending()
}

func (cm *ContextMemory) tokenCountWithPending() int {
	total := cm.tokenCount
	for i := cm.coveredIdx; i < len(cm.history); i++ {
		total += cm.history[i].TokenCost
	}
	return total
}

// UpdateTokenCount updates the real token count from a provider response.
// coveredMessages is how many messages this count covers.
func (cm *ContextMemory) UpdateTokenCount(realTokens, coveredMessages int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if coveredMessages > len(cm.history) {
		coveredMessages = len(cm.history)
	}
	cm.tokenCount = realTokens
	cm.coveredIdx = coveredMessages
}

// Undo removes the last N real user messages (skipping injections).
// Returns the number of messages actually removed.
func (cm *ContextMemory) Undo(count int) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	removed := 0
	for count > 0 && len(cm.history) > 0 {
		// Find the last real user message
		idx := -1
		for i := len(cm.history) - 1; i >= 0; i-- {
			m := cm.history[i]
			if m.Role == kosong.RoleUser && (m.Origin == nil || m.Origin.Variant == "user") {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		// Remove from idx to end (including trailing tool results)
		// Subtract token costs
		for i := idx; i < len(cm.history); i++ {
			if i < cm.coveredIdx {
				cm.tokenCount -= cm.history[i].TokenCost
			}
		}
		removedCount := len(cm.history) - idx
		cm.history = cm.history[:idx]
		if cm.coveredIdx > idx {
			cm.coveredIdx = idx
		}
		removed += removedCount
		count--
	}
	return removed
}

// Clear resets all message history and token tracking.
func (cm *ContextMemory) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.history = nil
	cm.tokenCount = 0
	cm.coveredIdx = 0
	cm.lastAssistant = nil
}

// ImportContext appends external context as a user message with overflow check.
// Returns error if it would exceed maxInput tokens.
func (cm *ContextMemory) ImportContext(content, source string) error {
	est := EstimateTokensForText(content)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.maxInput > 0 {
		current := cm.tokenCountWithPending()
		if current+est > cm.maxInput {
			return fmt.Errorf("import would exceed max input tokens (%d + %d > %d)", current, est, cm.maxInput)
		}
	}

	msg := ContextMessage{
		Message: kosong.Message{
			Role:    kosong.RoleUser,
			Content: []kosong.ContentPart{kosong.NewTextPart(content)},
		},
		Origin:    &PromptOrigin{Variant: "import", Note: source},
		TokenCost: est,
	}
	cm.history = append(cm.history, msg)
	return nil
}

// Last returns the last message, or nil if empty.
func (cm *ContextMemory) Last() *ContextMessage {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if len(cm.history) == 0 {
		return nil
	}
	msg := cm.history[len(cm.history)-1]
	return &msg
}

// PopLastMessage removes and returns the last message.
func (cm *ContextMemory) PopLastMessage() *ContextMessage {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if len(cm.history) == 0 {
		return nil
	}
	msg := cm.history[len(cm.history)-1]
	cm.history = cm.history[:len(cm.history)-1]
	idx := len(cm.history)
	if idx < cm.coveredIdx {
		cm.tokenCount -= msg.TokenCost
		cm.coveredIdx = idx
	}
	return &msg
}

// ── Token Estimation ──

const mediaTokenEstimate = 2000

// EstimateTokensForText estimates tokens for a text string.
// ASCII chars ÷ 4, non-ASCII chars counted 1:1 (CJK ≈ 1 token/char).
func EstimateTokensForText(text string) int {
	if text == "" {
		return 0
	}
	asciiChars := 0
	nonASCII := 0
	for _, r := range text {
		if r <= unicode.MaxASCII {
			asciiChars++
		} else {
			nonASCII++
		}
	}
	result := (asciiChars + 3) / 4 // round up
	result += nonASCII
	if result == 0 {
		result = 1
	}
	return result
}

// EstimateTokensForMessage estimates the token cost of a message.
func EstimateTokensForMessage(msg kosong.Message) int {
	total := 4 // role overhead
	total += EstimateTokensForText(string(msg.Role))

	for _, part := range msg.Content {
		switch part.Type {
		case "text":
			total += EstimateTokensForText(part.Text)
		case "think":
			total += EstimateTokensForText(part.Think)
		case "image_url", "audio_url", "video_url":
			total += mediaTokenEstimate
		}
	}

	for _, tc := range msg.ToolCalls {
		total += 4 // overhead
		if tc.Name != "" {
			total += EstimateTokensForText(tc.Name)
		}
		if tc.Arguments != nil {
			total += EstimateTokensForText(*tc.Arguments)
		}
	}

	if msg.ToolCallID != nil {
		total += 4 // tool_call_id overhead
	}

	for _, tool := range msg.Tools {
		total += EstimateTokensForText(tool.Name)
		total += EstimateTokensForText(tool.Description)
		if tool.Parameters != nil {
			total += EstimateTokensForText(fmt.Sprintf("%v", tool.Parameters))
		}
	}

	return total
}

// EstimateTokensForMessages sums per-message estimates.
func EstimateTokensForMessages(msgs []kosong.Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokensForMessage(m)
	}
	return total
}

// EstimateTokensForTools estimates the token cost of tool definitions.
func EstimateTokensForTools(tools []kosong.Tool) int {
	total := 0
	for _, t := range tools {
		total += EstimateTokensForText(t.Name)
		total += EstimateTokensForText(t.Description)
		if t.Parameters != nil {
			total += EstimateTokensForText(fmt.Sprintf("%v", t.Parameters))
		}
	}
	return total
}

// CloseAbandonedToolExchange synthesizes error results for any dangling
// tool calls at the end of history. Returns the number of results added.
func (cm *ContextMemory) CloseAbandonedToolExchange(errorOutput string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Find trailing assistant messages with unresolved tool calls
	var pendingIDs []string
	for i := len(cm.history) - 1; i >= 0; i-- {
		m := cm.history[i]
		if m.Role == kosong.RoleAssistant && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				// Check if a result already exists
				found := false
				for j := i + 1; j < len(cm.history); j++ {
					if cm.history[j].ToolCallID != nil && *cm.history[j].ToolCallID == tc.ID {
						found = true
						break
					}
				}
				if !found {
					pendingIDs = append(pendingIDs, tc.ID)
				}
			}
			break
		}
		if m.Role != kosong.RoleAssistant && m.ToolCallID == nil {
			break
		}
	}

	added := 0
	for _, id := range pendingIDs {
		msg := ContextMessage{
			Message: kosong.CreateToolMessage(id, errorOutput),
			Origin:  &PromptOrigin{Variant: "system_trigger", Note: "abandoned tool exchange"},
			IsError: true,
		}
		msg.TokenCost = EstimateTokensForMessage(msg.Message)
		cm.history = append(cm.history, msg)
		added++
	}
	return added
}

// FinishResume closes any pending tool results at end of session resume.
func (cm *ContextMemory) FinishResume() {
	cm.CloseAbandonedToolExchange("(session was interrupted; this tool call was not completed)")
}

// Project returns wire-ready messages using the projector.
func (cm *ContextMemory) Project(opts *ProjectOptions) []kosong.Message {
	return Project(cm.Messages(), opts)
}

// Strings returns a debug representation.
func (cm *ContextMemory) String() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var sb strings.Builder
	for i, m := range cm.history {
		origin := ""
		if m.Origin != nil {
			origin = m.Origin.Variant
		}
		sb.WriteString(fmt.Sprintf("[%d] %s (%s) tokens=%d\n", i, m.Role, origin, m.TokenCost))
	}
	return sb.String()
}
