package cli

import (
	"sync"
	"time"

	agentctx "github.com/visdomtech/kimi-code/internal/agentcore/agent/context"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/goal"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/plan"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/audit"
	"github.com/visdomtech/kimi-code/internal/kosong"
)

// SessionService is the single source of truth for all session data.
// It is created at app startup and passed to the TUI model.
//
// Thread-safe: all public methods acquire mu, so the streaming goroutine
// in runLLMStream and the bubbletea Update loop can safely access session data.
//
// The TUI model holds a *SessionService pointer. Even when bubbletea copies
// the model (value receivers), the pointer survives copies, ensuring all
// mutations hit the same shared data — fixing the conversation history bug.
type SessionService struct {
	mu sync.RWMutex

	// Identity
	id          string
	sess        *session.Session
	auditWriter *audit.Writer

	// LLM conversation history (the critical hot path)
	history []kosong.Message

	// Completed turns (for collapsibles and persistence)
	completedTurns []turnData
	turnCount      int

	// Token usage
	turnUsage    kosong.TokenUsage
	sessionUsage kosong.TokenUsage

	// Context management
	contextMgr      *agentctx.ContextManager
	compactStrategy *agentctx.CompactionStrategy

	// Trackers
	planTracker *plan.Tracker
	goalTracker *goal.Tracker

	// Turn diagnostics
	lastPrompt       string
	overflowRetries  int
	lastFinishReason *string

	// Side query mode (/btw)
	btwMode       bool
	btwHistoryLen int
}

// SessionServiceConfig holds constructor dependencies for SessionService.
type SessionServiceConfig struct {
	MaxCtx             int
	TriggerRatio       float64
	ReservedContextSize int
}

// NewSessionService creates a new session service with the given dependencies.
func NewSessionService(sess *session.Session, app *App, cfg SessionServiceConfig) *SessionService {
	return &SessionService{
		id:          sess.ID,
		sess:        sess,
		auditWriter: app.AuditWriter,
		history:     make([]kosong.Message, 0),
		contextMgr: agentctx.NewContextManager(
			cfg.MaxCtx,
			cfg.TriggerRatio,
			cfg.ReservedContextSize,
		),
		compactStrategy: agentctx.NewCompactionStrategy(agentctx.CompactionConfig{
			TriggerRatio:         cfg.TriggerRatio,
			BlockRatio:           cfg.TriggerRatio,
			ReservedContextSize:  cfg.ReservedContextSize,
			MaxCompactionPerTurn: 3,
			MaxOverflowAttempts:  3,
		}),
		planTracker: plan.NewTracker(),
		goalTracker: goal.NewTracker(),
	}
}

// --- Identity (read-only after construction, except SetSession) ---

// ID returns the current session ID.
func (s *SessionService) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.id
}

// Session returns the underlying session object.
func (s *SessionService) Session() *session.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sess
}

// SetSession updates the session identity (for session switch).
func (s *SessionService) SetSession(sess *session.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = sess.ID
	s.sess = sess
}

// AuditWriter returns the audit writer (read-only, thread-safe via BadgerDB).
func (s *SessionService) AuditWriter() *audit.Writer {
	return s.auditWriter
}

// --- History (the critical hot path) ---

// History returns a snapshot copy of the LLM conversation history.
// The caller may freely modify the returned slice.
func (s *SessionService) History() []kosong.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]kosong.Message, len(s.history))
	copy(result, s.history)
	return result
}

// HistoryLen returns the current history length without copying.
func (s *SessionService) HistoryLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.history)
}

// AppendMessages appends messages to the conversation history.
func (s *SessionService) AppendMessages(msgs ...kosong.Message) {
	if len(msgs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, msgs...)
}

// TruncateHistory truncates the history to n messages.
// Used by /btw side-query to discard injected messages.
func (s *SessionService) TruncateHistory(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= 0 && n < len(s.history) {
		s.history = s.history[:n]
	}
}

// ClearHistory removes all messages from the history.
func (s *SessionService) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = s.history[:0]
}

// RemoveLastMessages removes the last n messages from the history.
// Used by /undo to remove messages corresponding to undone turns.
func (s *SessionService) RemoveLastMessages(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= len(s.history) {
		s.history = s.history[:0]
	} else if n > 0 {
		s.history = s.history[:len(s.history)-n]
	}
}

// RewriteHistory replaces the entire history with new messages.
// Used by compaction to install compacted context.
func (s *SessionService) RewriteHistory(msgs []kosong.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = msgs
}

// --- Turns ---

// CompletedTurns returns a snapshot copy of completed turn data.
func (s *SessionService) CompletedTurns() []turnData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]turnData, len(s.completedTurns))
	copy(result, s.completedTurns)
	return result
}

// AppendTurn adds a completed turn.
func (s *SessionService) AppendTurn(td turnData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedTurns = append(s.completedTurns, td)
}

// ClearTurns removes all completed turns.
func (s *SessionService) ClearTurns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedTurns = nil
}

// RemoveLastTurns removes the last n completed turns.
func (s *SessionService) RemoveLastTurns(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= len(s.completedTurns) {
		s.completedTurns = nil
	} else {
		s.completedTurns = s.completedTurns[:len(s.completedTurns)-n]
	}
}

// RewriteTurns replaces all completed turns. Used by compaction.
func (s *SessionService) RewriteTurns(turns []turnData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedTurns = turns
}

// TurnCount returns the total number of turns.
func (s *SessionService) TurnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turnCount
}

// IncrementTurn increments the turn counter.
func (s *SessionService) IncrementTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnCount++
}

// ResetTurnCount resets the turn counter to zero.
func (s *SessionService) ResetTurnCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnCount = 0
}

// --- Usage ---

// TurnUsage returns the current turn's token usage.
func (s *SessionService) TurnUsage() kosong.TokenUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turnUsage
}

// SetTurnUsage sets the current turn's token usage.
func (s *SessionService) SetTurnUsage(u kosong.TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnUsage = u
}

// AddTurnUsage accumulates usage for the current turn.
func (s *SessionService) AddTurnUsage(u kosong.TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnUsage = kosong.AddUsage(s.turnUsage, u)
}

// ResetTurnUsage clears the per-turn usage accumulator.
func (s *SessionService) ResetTurnUsage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnUsage = kosong.TokenUsage{}
}

// SessionUsage returns the cumulative session token usage.
func (s *SessionService) SessionUsage() kosong.TokenUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionUsage
}

// AddSessionUsage accumulates usage into the session total.
func (s *SessionService) AddSessionUsage(u kosong.TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionUsage = kosong.AddUsage(s.sessionUsage, u)
}

// ResetSessionUsage clears the cumulative session usage.
func (s *SessionService) ResetSessionUsage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionUsage = kosong.TokenUsage{}
}

// SetSessionUsage sets the session usage directly (for replay).
func (s *SessionService) SetSessionUsage(u kosong.TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionUsage = u
}

// --- Context ---

// ContextMgr returns the context manager for direct access.
// The context manager is also mutex-protected internally.
func (s *SessionService) ContextMgr() *agentctx.ContextManager {
	return s.contextMgr
}

// CompactStrategy returns the compaction strategy.
func (s *SessionService) CompactStrategy() *agentctx.CompactionStrategy {
	return s.compactStrategy
}

// --- Trackers ---

// PlanTracker returns the plan tracker.
func (s *SessionService) PlanTracker() *plan.Tracker {
	return s.planTracker
}

// GoalTracker returns the goal tracker.
func (s *SessionService) GoalTracker() *goal.Tracker {
	return s.goalTracker
}

// --- Turn diagnostics ---

// LastPrompt returns the last prompt sent (for overflow retry).
func (s *SessionService) LastPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastPrompt
}

// SetLastPrompt records the last prompt sent.
func (s *SessionService) SetLastPrompt(p string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastPrompt = p
}

// OverflowRetries returns the number of overflow retry attempts.
func (s *SessionService) OverflowRetries() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.overflowRetries
}

// IncrementOverflow increments the overflow retry counter.
func (s *SessionService) IncrementOverflow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overflowRetries++
}

// ResetOverflow clears the overflow retry counter.
func (s *SessionService) ResetOverflow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overflowRetries = 0
}

// LastFinishReason returns the raw finish_reason from the last LLM step.
func (s *SessionService) LastFinishReason() *string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastFinishReason
}

// SetFinishReason records the finish_reason from an LLM step.
func (s *SessionService) SetFinishReason(r *string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFinishReason = r
}

// --- Side query (/btw) ---

// BtwMode returns whether side-query mode is active.
func (s *SessionService) BtwMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.btwMode
}

// SetBtwMode enables or disables side-query mode.
// When enabling, captures the current history length for later truncation.
func (s *SessionService) SetBtwMode(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.btwMode = enabled
	if enabled {
		s.btwHistoryLen = len(s.history)
	}
}

// BtwHistoryLen returns the history length captured when btw mode was enabled.
func (s *SessionService) BtwHistoryLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.btwHistoryLen
}

// --- Bulk operations ---

// Reset clears all mutable session state for a fresh start (/new, /init).
// Does NOT clear planTracker (callers decide whether to clear it).
func (s *SessionService) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = s.history[:0]
	s.completedTurns = nil
	s.turnCount = 0
	s.turnUsage = kosong.TokenUsage{}
	s.sessionUsage = kosong.TokenUsage{}
	s.lastPrompt = ""
	s.overflowRetries = 0
	s.lastFinishReason = nil
	s.btwMode = false
	s.btwHistoryLen = 0
	s.contextMgr.Reset()
	if s.compactStrategy != nil {
		s.compactStrategy.ResetForTurn()
	}
	s.goalTracker.CancelGoal("user")
}

// MaxOverflowAttempts returns the configured maximum overflow attempts.
func (s *SessionService) MaxOverflowAttempts() int {
	return s.compactStrategy.MaxOverflowAttempts()
}

// SessionTurnData is a helper that returns turn data and timestamp pairs
// for session replay/persistence.
type SessionTurnData struct {
	Turn      turnData
	Timestamp time.Time
}
