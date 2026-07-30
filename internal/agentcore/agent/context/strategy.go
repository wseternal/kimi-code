package context

// CompactionConfig holds the tunable parameters for the compaction strategy.
type CompactionConfig struct {
	TriggerRatio         float64 // usage ratio that triggers auto-compaction (default 0.85)
	BlockRatio           float64 // usage ratio that blocks new LLM calls (default 0.85)
	ReservedContextSize  int     // tokens reserved for response (default 50000)
	MaxCompactionPerTurn int     // max compaction attempts per turn (default 3)
	MaxOverflowAttempts  int     // max retries on context overflow error (default 3)
}

// DefaultCompactionConfig returns a CompactionConfig with production defaults.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		TriggerRatio:         0.85,
		BlockRatio:           0.85,
		ReservedContextSize:  50000,
		MaxCompactionPerTurn: 3,
		MaxOverflowAttempts:  3,
	}
}

// CompactionStrategy tracks compaction state and decides when to trigger
// auto-compaction or block new LLM calls. It prevents redundant compaction
// by recording the token count after each compaction and comparing against it.
type CompactionStrategy struct {
	config              CompactionConfig
	compactionCount     int // compactions performed in the current turn
	lastCompactedTokens int // usage recorded after the most recent compaction
}

// NewCompactionStrategy creates a strategy with the given config.
func NewCompactionStrategy(config CompactionConfig) *CompactionStrategy {
	return &CompactionStrategy{config: config}
}

// ShouldCompact returns true if compaction is allowed this turn (under the
// per-turn limit) and there is new content since the last compaction.
// Callers should combine this with ContextManager.NeedsCompaction() to
// check the actual usage threshold:
//
//	if cm.NeedsCompaction() && strategy.ShouldCompact(cm.CurrentUsage()) {
//	    performCompaction()
//	}
func (s *CompactionStrategy) ShouldCompact(usedSize int) bool {
	if !s.CanCompact() {
		return false
	}
	// Guard: nothing new since last compaction
	if s.lastCompactedTokens > 0 && usedSize <= s.lastCompactedTokens {
		return false
	}
	return true
}

// ShouldCompactByRatio returns true if usedSize exceeds the trigger ratio
// against the given maxTokens and compaction is allowed this turn.
func (s *CompactionStrategy) ShouldCompactByRatio(usedSize, maxTokens int) bool {
	if maxTokens <= 0 {
		return false
	}
	if !s.CanCompact() {
		return false
	}
	// Guard: nothing new since last compaction
	if s.lastCompactedTokens > 0 && usedSize <= s.lastCompactedTokens {
		return false
	}
	// Ratio trigger
	if float64(usedSize) >= float64(maxTokens)*s.config.TriggerRatio {
		return true
	}
	// Reserved context trigger
	if s.config.ReservedContextSize > 0 && s.config.ReservedContextSize < maxTokens {
		if usedSize+s.config.ReservedContextSize >= maxTokens {
			return true
		}
	}
	return false
}

// ShouldBlock returns true if usage exceeds the block ratio, meaning the
// agent should not start a new LLM call until compaction completes.
func (s *CompactionStrategy) ShouldBlock(usedSize, maxTokens int) bool {
	if maxTokens <= 0 {
		return false
	}
	return float64(usedSize) >= float64(maxTokens)*s.config.BlockRatio
}

// CanCompact returns true if the per-turn compaction limit has not been reached.
func (s *CompactionStrategy) CanCompact() bool {
	return s.compactionCount < s.config.MaxCompactionPerTurn
}

// RecordCompaction records that a compaction just completed with the given
// post-compaction token count. This prevents re-compacting the same content.
func (s *CompactionStrategy) RecordCompaction(tokensAfter int) {
	s.compactionCount++
	s.lastCompactedTokens = tokensAfter
}

// ResetForTurn resets the per-turn compaction counter. Call this at the
// start of each new user turn.
func (s *CompactionStrategy) ResetForTurn() {
	s.compactionCount = 0
}

// CompactionCount returns how many compactions have been performed this turn.
func (s *CompactionStrategy) CompactionCount() int {
	return s.compactionCount
}

// LastCompactedTokens returns the usage recorded after the most recent compaction.
func (s *CompactionStrategy) LastCompactedTokens() int {
	return s.lastCompactedTokens
}

// MaxOverflowAttempts returns the configured max retries on context overflow.
func (s *CompactionStrategy) MaxOverflowAttempts() int {
	return s.config.MaxOverflowAttempts
}
