// Package goal implements a simple goal tracker that injects
// progress-checking prompts into the system prompt.
package goal

import (
	"fmt"
	"strings"
	"time"
)

// Goal represents an autonomous goal for the agent.
type Goal struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	Status    string    `json:"status"` // "active", "completed", "abandoned"
}

// Tracker manages the current goal.
type Tracker struct {
	current *Goal
}

// NewTracker creates a new goal tracker.
func NewTracker() *Tracker {
	return &Tracker{}
}

// Set sets a new active goal, replacing any existing one.
func (t *Tracker) Set(text string) {
	t.current = &Goal{
		Text:      text,
		CreatedAt: time.Now(),
		Status:    "active",
	}
}

// Complete marks the current goal as completed.
func (t *Tracker) Complete() {
	if t.current != nil {
		t.current.Status = "completed"
	}
}

// Abandon marks the current goal as abandoned.
func (t *Tracker) Abandon() {
	if t.current != nil {
		t.current.Status = "abandoned"
	}
}

// Clear removes the current goal.
func (t *Tracker) Clear() {
	t.current = nil
}

// Current returns the current goal, or nil.
func (t *Tracker) Current() *Goal {
	return t.current
}

// IsActive returns true if there's an active goal.
func (t *Tracker) IsActive() bool {
	return t.current != nil && t.current.Status == "active"
}

// SystemPromptSuffix returns the goal injection text for the system prompt.
// Returns empty string if no active goal.
func (t *Tracker) SystemPromptSuffix() string {
	if !t.IsActive() {
		return ""
	}
	return fmt.Sprintf("\n\n## Current Goal\n%s\n\nPeriodically check your progress toward this goal and adjust your approach as needed.", t.current.Text)
}

// StatusString returns a human-readable status of the current goal.
func (t *Tracker) StatusString() string {
	if t.current == nil {
		return "No goal set"
	}
	elapsed := time.Since(t.current.CreatedAt).Round(time.Second)
	return fmt.Sprintf("Goal: %s\nStatus: %s (set %s ago)", t.current.Text, t.current.Status, elapsed)
}

// ParseGoalCommand parses "/goal <text>" or "/goal" (clear).
func ParseGoalCommand(args string) (text string, clear bool) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", true
	}
	return args, false
}
