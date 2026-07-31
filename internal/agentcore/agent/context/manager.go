// Package context implements conversation context management:
// compaction (summarize old messages), undo, and token tracking.
package context

import (
	"fmt"
	"strings"
	"sync"
)

// PerTurnOverhead is the estimated token cost of system prompt + tool
// definitions included in every API request. Tuned empirically.
const PerTurnOverhead = 1800

// TurnEstimate returns a token estimate for a full API turn, including
// the fixed overhead from system prompt and tool definitions.
func TurnEstimate(text string) int {
	return TokenEstimate(text) + PerTurnOverhead
}

// TokenEstimate is defined in tokens.go (CJK-aware heuristic).

// turnRecord stores token usage for a single turn.
type turnRecord struct {
	tokens   int
	measured bool // true if from real API usage, false if estimated
}

// ContextManager tracks context window usage using a two-tier model:
// measured (confirmed by API responses) and pending (estimated for in-flight turns).
// It also stores compaction thresholds from config.
// Thread-safe: all public methods acquire mu.
type ContextManager struct {
	mu              sync.RWMutex
	maxTokens       int
	measuredTotal   int // confirmed by API usage responses
	pendingEstimate int // estimated tokens for in-flight turn (transient)
	turnRecords     []turnRecord
	// Compaction thresholds (from config)
	triggerRatio    float64 // default 0.85
	reservedContext int     // default 50000
}

// NewContextManager creates a new context manager with the given max tokens
// and compaction thresholds. If triggerRatio <= 0, defaults to 0.85.
// If reservedContext <= 0, defaults to 50000.
func NewContextManager(maxTokens int, triggerRatio float64, reservedContext int) *ContextManager {
	if maxTokens <= 0 {
		maxTokens = 262144 // default 256K
	}
	if triggerRatio <= 0 {
		triggerRatio = 0.85
	}
	if reservedContext <= 0 {
		reservedContext = 50000
	}
	return &ContextManager{
		maxTokens:       maxTokens,
		triggerRatio:    triggerRatio,
		reservedContext: reservedContext,
	}
}

// AddTurnUsage records measured token usage for a completed turn.
// This clears the pending estimate and promotes it to measured.
func (cm *ContextManager) AddTurnUsage(tokens int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.turnRecords = append(cm.turnRecords, turnRecord{tokens: tokens, measured: true})
	cm.measuredTotal += tokens
	cm.pendingEstimate = 0
}

// SetPendingEstimate sets the transient token estimate for the current
// in-flight turn (e.g. during streaming). This does not affect measuredTotal.
func (cm *ContextManager) SetPendingEstimate(tokens int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.pendingEstimate = tokens
}

// CurrentUsage returns measured total plus any pending estimate.
func (cm *ContextManager) CurrentUsage() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.measuredTotal + cm.pendingEstimate
}

// MaxTokens returns the context window size.
func (cm *ContextManager) MaxTokens() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.maxTokens
}

// SetMaxTokens updates the context window size. If n <= 0, falls back to the
// default (262144). This is called when the model is changed at runtime (e.g.
// via /model) so the status bar reflects the correct limit.
func (cm *ContextManager) SetMaxTokens(n int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if n <= 0 {
		n = 262144 // default 256K
	}
	cm.maxTokens = n
}

// UsagePercent returns the percentage of context window used.
func (cm *ContextManager) UsagePercent() float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.maxTokens == 0 {
		return 0
	}
	return float64(cm.measuredTotal+cm.pendingEstimate) / float64(cm.maxTokens) * 100
}

// UsageDisplay returns a formatted string like "12.3K / 128K tokens (9.6%)".
// If currentTurn > 0, those tokens are added to the used count (for live
// streaming display where the current turn hasn't been committed yet).
func (cm *ContextManager) UsageDisplay(currentTurn ...int) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	used := cm.measuredTotal + cm.pendingEstimate
	for _, t := range currentTurn {
		used += t
	}
	max := cm.maxTokens
	pct := float64(0)
	if max > 0 {
		pct = float64(used) / float64(max) * 100
	}

	usedStr := FormatTokenCount(used)
	maxStr := FormatTokenCount(max)
	return fmt.Sprintf("%s / %s tokens (%.1f%%)", usedStr, maxStr, pct)
}

// RemoveLastNTurns removes the last N turn records and recalculates measured total.
func (cm *ContextManager) RemoveLastNTurns(n int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if n >= len(cm.turnRecords) {
		cm.turnRecords = nil
	} else {
		cm.turnRecords = cm.turnRecords[:len(cm.turnRecords)-n]
	}
	cm.recalculateLocked()
}

// recalculateLocked recomputes measuredTotal from turn records.
// Caller must hold cm.mu.
func (cm *ContextManager) recalculateLocked() {
	cm.measuredTotal = 0
	for _, r := range cm.turnRecords {
		cm.measuredTotal += r.tokens
	}
}

// Reset clears all usage tracking.
func (cm *ContextManager) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.turnRecords = nil
	cm.measuredTotal = 0
	cm.pendingEstimate = 0
}

// NeedsCompaction returns true if context usage exceeds the configured
// trigger ratio or if usage plus reserved context would exhaust the window.
func (cm *ContextManager) NeedsCompaction() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.maxTokens <= 0 {
		return false
	}
	used := cm.measuredTotal + cm.pendingEstimate
	// Trigger if usage exceeds ratio threshold
	if float64(used) >= float64(cm.maxTokens)*cm.triggerRatio {
		return true
	}
	// Trigger if reserved context would be exhausted
	if cm.reservedContext > 0 && cm.reservedContext < cm.maxTokens {
		if used+cm.reservedContext >= cm.maxTokens {
			return true
		}
	}
	return false
}

// TriggerRatio returns the configured compaction trigger ratio.
func (cm *ContextManager) TriggerRatio() float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.triggerRatio
}

// ReservedContext returns the configured reserved context size.
func (cm *ContextManager) ReservedContext() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.reservedContext
}

// FormatTokenCount formats a token count with K/M suffix.
func FormatTokenCount(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// CompactionResult holds the output of a compaction operation.
type CompactionResult struct {
	Summary       string // compacted summary of old messages
	KeptTurns     int    // number of recent turns kept verbatim
	RemovedTurns  int    // number of turns summarized
	OriginalTokens int
	CompactTokens  int
}

// CompactMessages generates a compaction summary from conversation messages.
// It keeps the last keepN turns verbatim and summarizes older ones.
func CompactMessages(messages []CompactMessage, keepN int) (*CompactionResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages to compact")
	}

	// Group into turns (user+assistant pairs)
	type turn struct {
		user     string
		assistant string
	}
	var turns []turn
	var current *turn
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			if current != nil {
				turns = append(turns, *current)
			}
			current = &turn{user: msg.Content}
		case "assistant":
			if current != nil {
				current.assistant = msg.Content
				turns = append(turns, *current)
				current = nil
			}
		}
	}
	if current != nil {
		turns = append(turns, *current)
	}

	if len(turns) <= keepN {
		return nil, fmt.Errorf("not enough turns to compact (%d turns, keeping %d)", len(turns), keepN)
	}

	// Summarize old turns
	oldTurns := turns[:len(turns)-keepN]
	var summaryParts []string
	originalTokens := 0
	for i, t := range oldTurns {
		originalTokens += TokenEstimate(t.user) + TokenEstimate(t.assistant)
		// Create a compact summary of each turn
		userSummary := truncateForSummary(t.user, 100)
		assistantSummary := truncateForSummary(t.assistant, 150)
		summaryParts = append(summaryParts, fmt.Sprintf("Turn %d: User asked about %s. Assistant responded about %s.", i+1, userSummary, assistantSummary))
	}

	summary := "Previous conversation summary:\n" + strings.Join(summaryParts, "\n")
	compactTokens := TokenEstimate(summary)

	return &CompactionResult{
		Summary:        summary,
		KeptTurns:      keepN,
		RemovedTurns:   len(oldTurns),
		OriginalTokens: originalTokens,
		CompactTokens:  compactTokens,
	}, nil
}

// CompactMessage is a simplified message for compaction input.
type CompactMessage struct {
	Role    string
	Content string
}

func truncateForSummary(text string, maxLen int) string {
	// Take first line or first maxLen chars
	lines := strings.SplitN(text, "\n", 2)
	text = lines[0]
	if len(text) > maxLen {
		return text[:maxLen-3] + "..."
	}
	return text
}
