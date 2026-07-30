package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// SteeringMessage represents a user message queued for steering the agent.
type SteeringMessage struct {
	Content string `json:"content"`
}

// SteeringTool is a system-injected tool that delivers queued user messages
// to the agent at step boundaries. The LLM does not call this tool directly;
// the streaming loop invokes Execute() between steps and injects the result
// as a tool message in the conversation.
type SteeringTool struct {
	mu       sync.Mutex
	queue    []SteeringMessage
	signaled atomic.Bool
}

// NewSteeringTool creates a new steering tool with an empty queue.
func NewSteeringTool() *SteeringTool {
	return &SteeringTool{}
}

// Enqueue adds a message to the steering queue.
func (t *SteeringTool) Enqueue(content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queue = append(t.queue, SteeringMessage{Content: content})
}

// HasMessages reports whether there are queued messages.
func (t *SteeringTool) HasMessages() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.queue) > 0
}

// Len returns the number of queued messages.
func (t *SteeringTool) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.queue)
}

// DrainAll returns and clears all queued messages.
func (t *SteeringTool) DrainAll() []SteeringMessage {
	t.mu.Lock()
	msgs := t.queue
	t.queue = nil
	t.mu.Unlock()
	return msgs
}

// Signal sets the steering priority flag, indicating the user wants the
// agent to process queued messages at its next breakpoint.
func (t *SteeringTool) Signal() {
	t.signaled.Store(true)
}

// IsSignaled checks and clears the priority signal (atomic swap).
func (t *SteeringTool) IsSignaled() bool {
	return t.signaled.Swap(false)
}

// Definition returns the tool definition for the LLM.
func (t *SteeringTool) Definition() Definition {
	return Definition{
		Name:        "Steering",
		Description: "System tool: delivers user steering messages. These are mid-conversation instructions from the user that should be considered before proceeding with further actions.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

// Execute drains queued messages and returns them formatted as context for the LLM.
func (t *SteeringTool) Execute(_ context.Context, _ json.RawMessage, _ ExecContext) (*Result, error) {
	msgs := t.DrainAll()
	if len(msgs) == 0 {
		return &Result{Output: "No steering messages."}, nil
	}
	var sb strings.Builder
	sb.WriteString("The user has sent the following steering messages. ")
	sb.WriteString("Please consider these instructions before proceeding with further actions:\n")
	for i, m := range msgs {
		sb.WriteString(fmt.Sprintf("\n%d. %s", i+1, m.Content))
	}
	return &Result{Output: sb.String()}, nil
}
