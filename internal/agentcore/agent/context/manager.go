// Package context implements conversation context management:
// compaction (summarize old messages), undo, and token tracking.
package context

import (
	"fmt"
	"strings"
)

// PerTurnOverhead is the estimated token cost of system prompt + tool
// definitions included in every API request. Tuned empirically.
const PerTurnOverhead = 1800

// TurnEstimate returns a token estimate for a full API turn, including
// the fixed overhead from system prompt and tool definitions.
func TurnEstimate(text string) int {
	return TokenEstimate(text) + PerTurnOverhead
}

// TokenEstimate estimates token count from text using the ~4 chars/token heuristic.
func TokenEstimate(text string) int {
	if text == "" {
		return 0
	}
	// Approximate: 1 token ≈ 4 characters for English, ~3 for code
	chars := len(text)
	return (chars + 3) / 4
}

// ContextManager tracks context window usage and manages compaction.
type ContextManager struct {
	maxTokens    int
	currentUsage int
	turnUsages   []int // per-turn token counts
}

// NewContextManager creates a new context manager with the given max tokens.
func NewContextManager(maxTokens int) *ContextManager {
	if maxTokens <= 0 {
		maxTokens = 131072 // default 128K
	}
	return &ContextManager{maxTokens: maxTokens}
}

// AddTurnUsage records token usage for a completed turn.
func (cm *ContextManager) AddTurnUsage(tokens int) {
	cm.turnUsages = append(cm.turnUsages, tokens)
	cm.recalculate()
}

// recalculate recomputes total usage from per-turn data.
func (cm *ContextManager) recalculate() {
	cm.currentUsage = 0
	for _, u := range cm.turnUsages {
		cm.currentUsage += u
	}
}

// CurrentUsage returns the estimated current token usage.
func (cm *ContextManager) CurrentUsage() int {
	return cm.currentUsage
}

// MaxTokens returns the context window size.
func (cm *ContextManager) MaxTokens() int {
	return cm.maxTokens
}

// UsagePercent returns the percentage of context window used.
func (cm *ContextManager) UsagePercent() float64 {
	if cm.maxTokens == 0 {
		return 0
	}
	return float64(cm.currentUsage) / float64(cm.maxTokens) * 100
}

// UsageDisplay returns a formatted string like "12.3K / 128K tokens (9.6%)".
func (cm *ContextManager) UsageDisplay() string {
	used := cm.currentUsage
	max := cm.maxTokens
	pct := cm.UsagePercent()

	usedStr := FormatTokenCount(used)
	maxStr := FormatTokenCount(max)
	return fmt.Sprintf("%s / %s tokens (%.1f%%)", usedStr, maxStr, pct)
}

// RemoveLastNTurns removes the last N turn usages.
func (cm *ContextManager) RemoveLastNTurns(n int) {
	if n >= len(cm.turnUsages) {
		cm.turnUsages = nil
	} else {
		cm.turnUsages = cm.turnUsages[:len(cm.turnUsages)-n]
	}
	cm.recalculate()
}

// Reset clears all usage tracking.
func (cm *ContextManager) Reset() {
	cm.turnUsages = nil
	cm.currentUsage = 0
}

// NeedsCompaction returns true if context usage exceeds the trigger ratio.
func (cm *ContextManager) NeedsCompaction(triggerRatio float64) bool {
	if triggerRatio <= 0 {
		triggerRatio = 0.8
	}
	return cm.UsagePercent() >= triggerRatio*100
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
