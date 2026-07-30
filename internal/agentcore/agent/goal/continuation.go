// Package goal implements goal lifecycle management.
// continuation.go provides the goal continuation driver.
package goal

import (
	"fmt"
	"sync"
	"time"
)

// ContinuationConfig configures the continuation driver.
type ContinuationConfig struct {
	MaxContinuationTurns int // max auto-continuation turns (0 = unlimited)
	MaxStepsPerTurn      int // max steps per turn (for step-cap detection)
}

// DefaultContinuationConfig returns sensible defaults.
func DefaultContinuationConfig() ContinuationConfig {
	return ContinuationConfig{
		MaxContinuationTurns: 0, // unlimited
		MaxStepsPerTurn:      50,
	}
}

// ContinuationPrompt is a request to submit a new turn for goal continuation.
type ContinuationPrompt struct {
	GoalID     string
	Objective  string
	Message    string // the continuation prompt text
	StepCapped bool   // true if previous turn hit step limit
}

// ContinuationDriver manages automatic goal continuation after turn completion.
// It checks goal status and budget, then emits continuation prompts.
type ContinuationDriver struct {
	mu       sync.RWMutex
	tracker  *Tracker
	config   ContinuationConfig
	pending  *ContinuationPrompt
	history  []ContinuationRecord
	disabled bool
}

// ContinuationRecord tracks a continuation that was launched.
type ContinuationRecord struct {
	GoalID     string    `json:"goalId"`
	TurnID     string    `json:"turnId,omitempty"`
	StepCapped bool      `json:"stepCapped"`
	LaunchedAt time.Time `json:"launchedAt"`
}

// NewContinuationDriver creates a continuation driver.
func NewContinuationDriver(tracker *Tracker, config ContinuationConfig) *ContinuationDriver {
	return &ContinuationDriver{
		tracker: tracker,
		config:  config,
	}
}

// OnTurnEnded is called when a turn completes. It checks if the goal should
// continue and returns a ContinuationPrompt if so, or nil if not.
// stepCapped indicates whether the turn ended due to hitting the step limit.
// turnFailed indicates whether the turn failed (error, abort, etc.).
func (d *ContinuationDriver) OnTurnEnded(turnID string, stepCapped, turnFailed bool) *ContinuationPrompt {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.disabled {
		return nil
	}

	snap := d.tracker.Current()
	if snap == nil {
		return nil
	}

	// If turn failed (non-step-cap), check if goal should be paused/blocked
	if turnFailed && !stepCapped {
		return nil
	}

	// Check goal status
	switch snap.Status {
	case StatusActive:
		// Continue
	case StatusPaused, StatusBlocked, StatusComplete:
		return nil
	default:
		return nil
	}

	// Check budget
	if d.tracker.IsOverBudget() {
		// Block the goal if budget is exhausted
		d.tracker.MarkBlocked("budget exhausted", "system")
		return nil
	}

	// Check max continuation turns
	if d.config.MaxContinuationTurns > 0 && len(d.history) >= d.config.MaxContinuationTurns {
		d.tracker.MarkBlocked("max continuation turns reached", "system")
		return nil
	}

	// Check for pending continuation (prevent double-enqueue)
	if d.pending != nil {
		return nil
	}

	// Build continuation prompt
	msg := GoalContinuationPrompt
	if stepCapped {
		msg = GoalStepCapContinuationPrompt + "\n\n" + msg
	}

	prompt := &ContinuationPrompt{
		GoalID:     snap.GoalID,
		Objective:  snap.Objective,
		Message:    msg,
		StepCapped: stepCapped,
	}
	d.pending = prompt
	return prompt
}

// ConfirmLaunched is called when the continuation turn was actually submitted.
func (d *ContinuationDriver) ConfirmLaunched(turnID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.pending != nil {
		d.history = append(d.history, ContinuationRecord{
			GoalID:     d.pending.GoalID,
			TurnID:     turnID,
			StepCapped: d.pending.StepCapped,
			LaunchedAt: time.Now(),
		})
		d.pending = nil
	}
}

// ClearPending clears any pending continuation (e.g., user submitted a new turn).
func (d *ContinuationDriver) ClearPending() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = nil
}

// HasPending returns true if there's a pending continuation.
func (d *ContinuationDriver) HasPending() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.pending != nil
}

// History returns the continuation history.
func (d *ContinuationDriver) History() []ContinuationRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]ContinuationRecord, len(d.history))
	copy(result, d.history)
	return result
}

// SetDisabled enables or disables the continuation driver.
func (d *ContinuationDriver) SetDisabled(disabled bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.disabled = disabled
}

// ContinuationCount returns how many continuations have been launched.
func (d *ContinuationDriver) ContinuationCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.history)
}

// ── Continuation Prompts ──

// GoalContinuationPrompt is the standard continuation prompt.
var GoalContinuationPrompt = `Continue working toward the active goal. Keep the self-audit brief. Do not explore unrelated interpretations once the goal can be decided. If the objective is simple, already answered, impossible, unsafe, or contradictory, do not run another goal turn. Explain briefly if useful, then call UpdateGoal with complete or blocked in the same turn. Otherwise, weigh the objective and any completion criteria against the work done so far, choose one bounded, useful slice of work, and use the existing conversation context and your tools. Do not try to finish a broad goal in one turn unless the whole goal is genuinely small. Most goal turns should not call UpdateGoal: after completing a useful slice, if material work remains, end the turn normally without calling UpdateGoal so the runtime can continue the goal in the next turn. Call UpdateGoal with complete only when all required work is done, any stated validation has passed, and there is no useful next action.`

// GoalStepCapContinuationPrompt is used when the previous turn hit the step limit.
var GoalStepCapContinuationPrompt = `The previous goal turn reached the per-turn step limit before finishing its work, so a new turn was started for you. Pick up where that turn stopped and keep each slice of work small enough to fit the limit.`

// GoalBudgetStopReminder is injected when budget is reached mid-turn.
var GoalBudgetStopReminder = `Your goal budget has been reached. Please wrap up your current action and call UpdateGoal with complete or blocked. Do not start new work.`

// GoalBudgetToolsRejectedMessage is returned when tool calls are vetoed after budget.
var GoalBudgetToolsRejectedMessage = fmt.Sprintf("Tool execution rejected: goal budget exhausted. %s", GoalBudgetStopReminder)
