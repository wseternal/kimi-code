package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/visdomtech/kimi-code/internal/persistence"
)

// Message represents a single message in session history.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Timestamp time.Time  `json:"timestamp"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
	Thinking  string     `json:"thinking,omitempty"`
}

// ToolCall records a tool invocation within a message.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
	IsError   bool   `json:"isError,omitempty"`
}

// Turn groups a user prompt with the assistant's response.
type Turn struct {
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response"`
	Thinking  string    `json:"thinking,omitempty"`
	Tools     []ToolCall `json:"tools,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// History manages message history for a session, backed by a JSONL append log.
type History struct {
	store    persistence.Store
	messages []Message
}

// NewHistory creates a new history manager.
func NewHistory(store persistence.Store) *History {
	return &History{store: store}
}

// messagesKey returns the store key for a session's message log.
func messagesKey(id string) string {
	return fmt.Sprintf("sessions/%s/messages.jsonl", id)
}

// AddMessage appends a message to the in-memory list and persists it.
func (h *History) AddMessage(ctx context.Context, sessionID string, msg Message) error {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	h.messages = append(h.messages, msg)

	line, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	line = append(line, '\n')

	// Read existing content, append, and write back
	existing, err := h.store.Get(ctx, messagesKey(sessionID))
	if err != nil && err != persistence.ErrNotFound {
		return fmt.Errorf("read messages: %w", err)
	}
	updated := append(existing, line...)
	return h.store.Set(ctx, messagesKey(sessionID), updated)
}

// AddTurn adds a user prompt and assistant response as a pair of messages.
func (h *History) AddTurn(ctx context.Context, sessionID string, prompt, response, thinking string, tools []ToolCall) error {
	now := time.Now()
	if err := h.AddMessage(ctx, sessionID, Message{
		Role:      "user",
		Content:   prompt,
		Timestamp: now,
	}); err != nil {
		return err
	}
	return h.AddMessage(ctx, sessionID, Message{
		Role:      "assistant",
		Content:   response,
		Thinking:  thinking,
		ToolCalls: tools,
		Timestamp: now,
	})
}

// Load restores message history from the JSONL file.
func (h *History) Load(ctx context.Context, sessionID string) error {
	data, err := h.store.Get(ctx, messagesKey(sessionID))
	if err != nil {
		if err == persistence.ErrNotFound {
			h.messages = nil
			return nil
		}
		return fmt.Errorf("load messages: %w", err)
	}

	h.messages = nil
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // skip malformed lines
		}
		h.messages = append(h.messages, msg)
	}
	return scanner.Err()
}

// Messages returns the loaded message history.
func (h *History) Messages() []Message {
	return h.messages
}

// Turns reconstructs turns from the message list.
func (h *History) Turns() []Turn {
	var turns []Turn
	var current *Turn
	for _, msg := range h.messages {
		switch msg.Role {
		case "user":
			if current != nil {
				turns = append(turns, *current)
			}
			current = &Turn{
				Prompt:    msg.Content,
				Timestamp: msg.Timestamp,
			}
		case "assistant":
			if current != nil {
				current.Response = msg.Content
				current.Thinking = msg.Thinking
				current.Tools = msg.ToolCalls
				turns = append(turns, *current)
				current = nil
			}
		}
	}
	if current != nil {
		turns = append(turns, *current)
	}
	return turns
}

// RemoveLastTurns removes the last N turns from history and rewrites the store.
func (h *History) RemoveLastTurns(ctx context.Context, sessionID string, n int) error {
	turns := h.Turns()
	if n >= len(turns) {
		h.messages = nil
	} else {
		keep := turns[:len(turns)-n]
		h.messages = nil
		for _, t := range keep {
			h.messages = append(h.messages, Message{
				Role:      "user",
				Content:   t.Prompt,
				Timestamp: t.Timestamp,
			})
			h.messages = append(h.messages, Message{
				Role:      "assistant",
				Content:   t.Response,
				Thinking:  t.Thinking,
				ToolCalls: t.Tools,
				Timestamp: t.Timestamp,
			})
		}
	}
	return h.rewriteAll(ctx, sessionID)
}

// rewriteAll rewrites the entire JSONL file from current messages.
func (h *History) rewriteAll(ctx context.Context, sessionID string) error {
	var buf bytes.Buffer
	for _, msg := range h.messages {
		line, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return h.store.Set(ctx, messagesKey(sessionID), buf.Bytes())
}

// Clear removes all message history for a session.
func (h *History) Clear(ctx context.Context, sessionID string) error {
	h.messages = nil
	return h.store.Del(ctx, messagesKey(sessionID))
}
