// Package goal implements goal lifecycle management with budget tracking,
// status transitions, and system prompt injection.
package goal

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Status represents the lifecycle state of a goal.
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusBlocked  Status = "blocked"
	StatusComplete Status = "complete"
)

// BudgetLimits defines optional resource caps for a goal.
type BudgetLimits struct {
	TokenBudget       *int `json:"tokenBudget,omitempty"`
	TurnBudget        *int `json:"turnBudget,omitempty"`
	WallClockBudgetMs *int `json:"wallClockBudgetMs,omitempty"`
}

// BudgetReport is the computed budget status at a point in time.
type BudgetReport struct {
	TokenBudget       *int `json:"tokenBudget,omitempty"`
	TurnBudget        *int `json:"turnBudget,omitempty"`
	WallClockBudgetMs *int `json:"wallClockBudgetMs,omitempty"`
	RemainingTokens   *int `json:"remainingTokens,omitempty"`
	RemainingTurns    *int `json:"remainingTurns,omitempty"`
	RemainingWallClockMs *int `json:"remainingWallClockMs,omitempty"`
	TokenBudgetReached     bool `json:"tokenBudgetReached"`
	TurnBudgetReached      bool `json:"turnBudgetReached"`
	WallClockBudgetReached bool `json:"wallClockBudgetReached"`
	OverBudget             bool `json:"overBudget"`
}

// State holds the full internal state of a goal.
type State struct {
	GoalID              string       `json:"goalId"`
	Objective           string       `json:"objective"`
	CompletionCriterion string       `json:"completionCriterion,omitempty"`
	Status              Status       `json:"status"`
	TurnsUsed           int          `json:"turnsUsed"`
	TokensUsed          int          `json:"tokensUsed"`
	WallClockMs         int          `json:"wallClockMs"`
	BudgetLimits        BudgetLimits `json:"budgetLimits"`
	TerminalReason      string       `json:"terminalReason,omitempty"`
	CreatedAt           time.Time    `json:"createdAt"`
	wallClockResumedAt  *time.Time
}

// Snapshot is the public read-only view of a goal.
type Snapshot struct {
	GoalID              string        `json:"goalId"`
	Objective           string        `json:"objective"`
	CompletionCriterion string        `json:"completionCriterion,omitempty"`
	Status              Status        `json:"status"`
	TurnsUsed           int           `json:"turnsUsed"`
	TokensUsed          int           `json:"tokensUsed"`
	WallClockMs         int           `json:"wallClockMs"`
	Budget              *BudgetReport `json:"budget,omitempty"`
	TerminalReason      string        `json:"terminalReason,omitempty"`
}

// Change describes what triggered a goal state transition.
type Change struct {
	Kind   string `json:"kind"`
	Actor  string `json:"actor,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Tracker manages the current goal lifecycle.
type Tracker struct {
	mu      sync.RWMutex
	current *State
	queue   []QueuedGoal
}

// QueuedGoal is a pending goal in the queue.
type QueuedGoal struct {
	ID                  string `json:"id"`
	Objective           string `json:"objective"`
	CompletionCriterion string `json:"completionCriterion,omitempty"`
	BudgetLimits        BudgetLimits `json:"budgetLimits"`
	QueuedAt            time.Time    `json:"queuedAt"`
}

// NewTracker creates a new goal tracker.
func NewTracker() *Tracker { return &Tracker{} }

// ── Queue Operations ──

// QueueGoal adds a goal to the pending queue.
func (t *Tracker) QueueGoal(objective, completionCriterion string, budget BudgetLimits) *QueuedGoal {
	t.mu.Lock()
	defer t.mu.Unlock()
	q := QueuedGoal{
		ID:                  fmt.Sprintf("queued_%d", time.Now().UnixNano()),
		Objective:           objective,
		CompletionCriterion: completionCriterion,
		BudgetLimits:        budget,
		QueuedAt:            time.Now(),
	}
	t.queue = append(t.queue, q)
	return &t.queue[len(t.queue)-1]
}

// ListQueue returns all queued goals.
func (t *Tracker) ListQueue() []QueuedGoal {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]QueuedGoal, len(t.queue))
	copy(result, t.queue)
	return result
}

// PopQueue removes and returns the first queued goal, or nil.
func (t *Tracker) PopQueue() *QueuedGoal {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.queue) == 0 {
		return nil
	}
	q := t.queue[0]
	t.queue = t.queue[1:]
	return &q
}

// RemoveFromQueue removes a queued goal by ID.
func (t *Tracker) RemoveFromQueue(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, q := range t.queue {
		if q.ID == id {
			t.queue = append(t.queue[:i], t.queue[i+1:]...)
			return true
		}
	}
	return false
}

// ClearQueue removes all queued goals.
func (t *Tracker) ClearQueue() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(t.queue)
	t.queue = nil
	return n
}

// CreateGoal creates a new active goal. Objective max 4000 chars.
func (t *Tracker) CreateGoal(objective, completionCriterion string, budget BudgetLimits, actor string) (*Snapshot, *Change, error) {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return nil, nil, fmt.Errorf("goal objective must be non-empty")
	}
	if len(objective) > 4000 {
		return nil, nil, fmt.Errorf("goal objective exceeds 4000 character limit")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.current = &State{
		GoalID: fmt.Sprintf("goal_%d", now.UnixNano()), Objective: objective,
		CompletionCriterion: completionCriterion, Status: StatusActive,
		BudgetLimits: budget, CreatedAt: now, wallClockResumedAt: &now,
	}
	return t.snapshotLocked(), &Change{Kind: "created", Actor: actor}, nil
}

// PauseGoal pauses the active goal.
func (t *Tracker) PauseGoal(actor string) (*Snapshot, *Change, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil || t.current.Status != StatusActive {
		return nil, nil, fmt.Errorf("no active goal to pause")
	}
	t.foldWallClock()
	t.current.Status = StatusPaused
	return t.snapshotLocked(), &Change{Kind: "paused", Actor: actor}, nil
}

// ResumeGoal resumes a paused or blocked goal.
func (t *Tracker) ResumeGoal(actor string) (*Snapshot, *Change, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil || (t.current.Status != StatusPaused && t.current.Status != StatusBlocked) {
		return nil, nil, fmt.Errorf("no paused/blocked goal to resume")
	}
	now := time.Now()
	t.current.Status = StatusActive
	t.current.wallClockResumedAt = &now
	return t.snapshotLocked(), &Change{Kind: "resumed", Actor: actor}, nil
}

// MarkBlocked transitions the goal to blocked.
func (t *Tracker) MarkBlocked(reason, actor string) (*Snapshot, *Change, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return nil, nil, fmt.Errorf("no goal to block")
	}
	t.foldWallClock()
	t.current.Status = StatusBlocked
	t.current.TerminalReason = reason
	return t.snapshotLocked(), &Change{Kind: "blocked", Actor: actor, Reason: reason}, nil
}

// MarkComplete completes the goal and clears it.
func (t *Tracker) MarkComplete(reason, actor string) (*Snapshot, *Change, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return nil, nil, fmt.Errorf("no goal to complete")
	}
	t.foldWallClock()
	t.current.Status = StatusComplete
	t.current.TerminalReason = reason
	snap := t.snapshotLocked()
	ch := &Change{Kind: "completed", Actor: actor, Reason: reason}
	t.current = nil
	return snap, ch, nil
}

// CancelGoal removes the goal entirely.
func (t *Tracker) CancelGoal(actor string) (*Snapshot, *Change, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return nil, nil, fmt.Errorf("no goal to cancel")
	}
	t.foldWallClock()
	snap := t.snapshotLocked()
	ch := &Change{Kind: "cancelled", Actor: actor}
	t.current = nil
	return snap, ch, nil
}

// RecordTokenUsage adds token usage to the current goal.
func (t *Tracker) RecordTokenUsage(delta int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current != nil {
		t.current.TokensUsed += delta
	}
}

// IncrementTurn increments the turn counter.
func (t *Tracker) IncrementTurn() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current != nil {
		t.current.TurnsUsed++
	}
}

// SetBudgetLimits merges new limits into the current goal.
func (t *Tracker) SetBudgetLimits(limits BudgetLimits) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return
	}
	if limits.TokenBudget != nil {
		t.current.BudgetLimits.TokenBudget = limits.TokenBudget
	}
	if limits.TurnBudget != nil {
		t.current.BudgetLimits.TurnBudget = limits.TurnBudget
	}
	if limits.WallClockBudgetMs != nil {
		t.current.BudgetLimits.WallClockBudgetMs = limits.WallClockBudgetMs
	}
}

// Current returns a snapshot or nil.
func (t *Tracker) Current() *Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.current == nil {
		return nil
	}
	return t.snapshotLocked()
}

// IsActive returns true if there's an active goal.
func (t *Tracker) IsActive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.current != nil && t.current.Status == StatusActive
}

// IsOverBudget returns true if any budget limit is exceeded.
func (t *Tracker) IsOverBudget() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.current == nil {
		return false
	}
	return t.computeBudgetLocked().OverBudget
}

// SystemPromptSuffix returns the goal injection text.
func (t *Tracker) SystemPromptSuffix() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.current == nil || t.current.Status != StatusActive {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n## Current Goal\n")
	sb.WriteString(t.current.Objective)
	if t.current.CompletionCriterion != "" {
		sb.WriteString("\n\nCompletion criterion: ")
		sb.WriteString(t.current.CompletionCriterion)
	}
	budget := t.computeBudgetLocked()
	if budget.TokenBudget != nil || budget.TurnBudget != nil {
		sb.WriteString("\n\nBudget: ")
		if budget.RemainingTokens != nil {
			fmt.Fprintf(&sb, "%d tokens remaining. ", *budget.RemainingTokens)
		}
		if budget.RemainingTurns != nil {
			fmt.Fprintf(&sb, "%d turns remaining. ", *budget.RemainingTurns)
		}
	}
	sb.WriteString("\n\nPeriodically check your progress toward this goal and adjust your approach as needed.")
	return sb.String()
}

// StatusString returns a human-readable status.
func (t *Tracker) StatusString() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.current == nil {
		return "No goal set"
	}
	elapsed := time.Since(t.current.CreatedAt).Round(time.Second)
	return fmt.Sprintf("Goal: %s\nStatus: %s (set %s ago, %d turns, %d tokens)",
		t.current.Objective, t.current.Status, elapsed, t.current.TurnsUsed, t.current.TokensUsed)
}

// ParseGoalCommand parses "/goal <text>" or "/goal" (clear).
func ParseGoalCommand(args string) (text string, clear bool) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", true
	}
	return args, false
}

// ── Internal ──

func (t *Tracker) foldWallClock() {
	if t.current != nil && t.current.wallClockResumedAt != nil {
		t.current.WallClockMs += int(time.Since(*t.current.wallClockResumedAt).Milliseconds())
		t.current.wallClockResumedAt = nil
	}
}

func (t *Tracker) snapshotLocked() *Snapshot {
	c := t.current
	s := &Snapshot{
		GoalID: c.GoalID, Objective: c.Objective, CompletionCriterion: c.CompletionCriterion,
		Status: c.Status, TurnsUsed: c.TurnsUsed, TokensUsed: c.TokensUsed,
		WallClockMs: c.WallClockMs, TerminalReason: c.TerminalReason,
	}
	if c.wallClockResumedAt != nil {
		s.WallClockMs += int(time.Since(*c.wallClockResumedAt).Milliseconds())
	}
	s.Budget = t.computeBudgetLocked()
	return s
}

func (t *Tracker) computeBudgetLocked() *BudgetReport {
	c := t.current
	r := &BudgetReport{TokenBudget: c.BudgetLimits.TokenBudget, TurnBudget: c.BudgetLimits.TurnBudget, WallClockBudgetMs: c.BudgetLimits.WallClockBudgetMs}
	wallMs := c.WallClockMs
	if c.wallClockResumedAt != nil {
		wallMs += int(time.Since(*c.wallClockResumedAt).Milliseconds())
	}
	if r.TokenBudget != nil {
		rem := *r.TokenBudget - c.TokensUsed
		r.RemainingTokens = &rem
		r.TokenBudgetReached = rem <= 0
	}
	if r.TurnBudget != nil {
		rem := *r.TurnBudget - c.TurnsUsed
		r.RemainingTurns = &rem
		r.TurnBudgetReached = rem <= 0
	}
	if r.WallClockBudgetMs != nil {
		rem := *r.WallClockBudgetMs - wallMs
		r.RemainingWallClockMs = &rem
		r.WallClockBudgetReached = rem <= 0
	}
	r.OverBudget = r.TokenBudgetReached || r.TurnBudgetReached || r.WallClockBudgetReached
	return r
}
