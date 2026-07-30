package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// AskUserHandler is called when the agent asks the user a question.
// It blocks until the user responds. Returns the selected option index
// and optional free-text response.
type AskUserHandler func(question AskUserQuestion) (AskUserResponse, error)

// AskUserQuestion is a structured question from the agent to the user.
type AskUserQuestion struct {
	Question    string           `json:"question"`
	Options     []AskUserOption  `json:"options,omitempty"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
}

// AskUserOption is a single choice in a question.
type AskUserOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskUserResponse is the user's answer.
type AskUserResponse struct {
	SelectedIndices []int  `json:"selectedIndices,omitempty"`
	FreeText        string `json:"freeText,omitempty"`
}

// AskUserTool lets the agent ask the user structured questions.
type AskUserTool struct {
	mu      sync.Mutex
	handler AskUserHandler
}

func NewAskUserTool(handler AskUserHandler) *AskUserTool {
	return &AskUserTool{handler: handler}
}

// SetHandler replaces the ask handler (useful for late binding).
func (t *AskUserTool) SetHandler(h AskUserHandler) {
	t.mu.Lock()
	t.handler = h
	t.mu.Unlock()
}

func (t *AskUserTool) Definition() Definition {
	return Definition{
		Name:        "AskUser",
		Description: "Ask the user a structured question with optional choices. Use when you need clarification, preference, or a decision from the user before proceeding.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"question": map[string]interface{}{
					"type":        "string",
					"description": "The question to ask the user",
				},
				"options": map[string]interface{}{
					"type":        "array",
					"description": "Available choices for the user",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"label":       map[string]interface{}{"type": "string", "description": "Short label for the option"},
							"description": map[string]interface{}{"type": "string", "description": "Detailed description of this choice"},
						},
						"required": []string{"label"},
					},
				},
				"multiSelect": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, the user can select multiple options",
				},
			},
			"required": []string{"question"},
		},
	}
}

func (t *AskUserTool) Execute(ctx context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var q AskUserQuestion
	if err := json.Unmarshal(input, &q); err != nil {
		return nil, fmt.Errorf("parse AskUser input: %w", err)
	}
	if q.Question == "" {
		return &Result{Output: "Question is required", IsError: true}, nil
	}

	t.mu.Lock()
	handler := t.handler
	t.mu.Unlock()

	if handler == nil {
		// No handler: format question as text for the user
		return &Result{Output: formatQuestionAsText(q)}, nil
	}

	resp, err := handler(q)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get user response: %s", err), IsError: true}, nil
	}

	return &Result{Output: formatResponse(q, resp)}, nil
}

// formatQuestionAsText formats the question as readable text when no handler is available.
func formatQuestionAsText(q AskUserQuestion) string {
	var sb strings.Builder
	sb.WriteString("Question for the user: ")
	sb.WriteString(q.Question)
	sb.WriteString("\n")

	if len(q.Options) > 0 {
		sb.WriteString("\nOptions:\n")
		for i, opt := range q.Options {
			sb.WriteString(fmt.Sprintf("  %d. %s", i+1, opt.Label))
			if opt.Description != "" {
				sb.WriteString(fmt.Sprintf(" — %s", opt.Description))
			}
			sb.WriteString("\n")
		}
		if q.MultiSelect {
			sb.WriteString("(Multiple selections allowed)\n")
		}
	}

	sb.WriteString("\nPlease respond with your choice or free-form answer.")
	return sb.String()
}

// formatResponse formats the user's response into a readable result.
func formatResponse(q AskUserQuestion, resp AskUserResponse) string {
	if resp.FreeText != "" {
		return fmt.Sprintf("User responded: %s", resp.FreeText)
	}

	if len(resp.SelectedIndices) == 0 {
		return "User did not select any option."
	}

	var parts []string
	for _, idx := range resp.SelectedIndices {
		if idx >= 0 && idx < len(q.Options) {
			parts = append(parts, q.Options[idx].Label)
		} else if idx == len(q.Options) {
			parts = append(parts, "Other (custom response)")
		}
	}

	if len(parts) == 0 {
		return "User did not select any valid option."
	}
	return fmt.Sprintf("User selected: %s", strings.Join(parts, ", "))
}
