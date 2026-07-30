package context

import "testing"

func TestShouldCompactBasic(t *testing.T) {
	s := NewCompactionStrategy(CompactionConfig{
		TriggerRatio:         0.85,
		BlockRatio:           0.85,
		ReservedContextSize:  50000,
		MaxCompactionPerTurn: 3,
		MaxOverflowAttempts:  3,
	})

	// Fresh strategy: can compact, no last-compacted guard
	if !s.ShouldCompact(100) {
		t.Error("ShouldCompact(100) should be true for fresh strategy")
	}

	// After recording compaction at 100, same usage should be guarded
	s.RecordCompaction(100)
	if s.ShouldCompact(100) {
		t.Error("ShouldCompact should be false when usedSize <= lastCompactedTokens")
	}
	if s.ShouldCompact(50) {
		t.Error("ShouldCompact should be false when usedSize < lastCompactedTokens")
	}
	// Higher usage should pass the guard
	if !s.ShouldCompact(200) {
		t.Error("ShouldCompact(200) should be true when usage grew beyond lastCompactedTokens")
	}
}

func TestShouldCompactByRatio(t *testing.T) {
	cfg := CompactionConfig{
		TriggerRatio:         0.85,
		MaxCompactionPerTurn: 3,
		ReservedContextSize:  50000,
	}
	s := NewCompactionStrategy(cfg)

	// maxTokens=1000, usage=800: ratio check 800 < 850 → false
	if s.ShouldCompactByRatio(800, 1000) {
		t.Error("should not compact at 800/1000 (ratio=0.80 < 0.85)")
	}

	// usage=850: ratio check 850 >= 850 → true
	if !s.ShouldCompactByRatio(850, 1000) {
		t.Error("should compact at 850/1000 (ratio=0.85)")
	}

	// usage=900: ratio check 900 >= 850 → true
	if !s.ShouldCompactByRatio(900, 1000) {
		t.Error("should compact at 900/1000")
	}
}

func TestShouldCompactByRatioReservedContext(t *testing.T) {
	cfg := CompactionConfig{
		TriggerRatio:         0.85,
		MaxCompactionPerTurn: 3,
		ReservedContextSize:  200,
	}
	s := NewCompactionStrategy(cfg)

	// maxTokens=1000, reserved=200, usage=750: ratio 750<850, reserved 750+200=950<1000 → false
	if s.ShouldCompactByRatio(750, 1000) {
		t.Error("should not compact at 750 with reserved=200")
	}

	// usage=800: reserved 800+200=1000 >= 1000 → true
	if !s.ShouldCompactByRatio(800, 1000) {
		t.Error("should compact at 800 with reserved=200 (800+200>=1000)")
	}
}

func TestShouldBlock(t *testing.T) {
	cfg := CompactionConfig{BlockRatio: 0.85}
	s := NewCompactionStrategy(cfg)

	if s.ShouldBlock(800, 1000) {
		t.Error("should not block at 800/1000")
	}
	if !s.ShouldBlock(850, 1000) {
		t.Error("should block at 850/1000")
	}
	if !s.ShouldBlock(950, 1000) {
		t.Error("should block at 950/1000")
	}
	// Edge: maxTokens=0 → no block
	if s.ShouldBlock(100, 0) {
		t.Error("should not block with maxTokens=0")
	}
}

func TestPerTurnCompactionLimit(t *testing.T) {
	cfg := CompactionConfig{MaxCompactionPerTurn: 2}
	s := NewCompactionStrategy(cfg)

	if !s.CanCompact() {
		t.Error("should be able to compact initially")
	}

	s.RecordCompaction(100)
	if !s.CanCompact() {
		t.Error("should be able to compact once (limit=2)")
	}

	s.RecordCompaction(200)
	if s.CanCompact() {
		t.Error("should not be able to compact after reaching limit")
	}

	if s.ShouldCompact(500) {
		t.Error("ShouldCompact should be false when per-turn limit reached")
	}

	// Reset for next turn
	s.ResetForTurn()
	if !s.CanCompact() {
		t.Error("should be able to compact after ResetForTurn")
	}
	// lastCompactedTokens persists across turns (guard still active)
	if s.ShouldCompact(200) {
		t.Error("ShouldCompact should be false when usedSize <= lastCompactedTokens after reset")
	}
	if !s.ShouldCompact(300) {
		t.Error("ShouldCompact should be true when usedSize > lastCompactedTokens after reset")
	}
}

func TestReCompactionGuard(t *testing.T) {
	s := NewCompactionStrategy(DefaultCompactionConfig())

	// Simulate compaction that reduced usage to 50000
	s.RecordCompaction(50000)

	// Usage hasn't grown → should not compact
	if s.ShouldCompact(50000) {
		t.Error("should not re-compact when usage equals lastCompactedTokens")
	}

	// Usage grew → should compact
	if !s.ShouldCompact(60000) {
		t.Error("should compact when usage grew beyond lastCompactedTokens")
	}
}

func TestMaxOverflowAttempts(t *testing.T) {
	cfg := CompactionConfig{MaxOverflowAttempts: 5}
	s := NewCompactionStrategy(cfg)
	if s.MaxOverflowAttempts() != 5 {
		t.Errorf("MaxOverflowAttempts = %d, want 5", s.MaxOverflowAttempts())
	}
}

func TestDefaultCompactionConfig(t *testing.T) {
	cfg := DefaultCompactionConfig()
	if cfg.TriggerRatio != 0.85 {
		t.Errorf("TriggerRatio = %f, want 0.85", cfg.TriggerRatio)
	}
	if cfg.BlockRatio != 0.85 {
		t.Errorf("BlockRatio = %f, want 0.85", cfg.BlockRatio)
	}
	if cfg.ReservedContextSize != 50000 {
		t.Errorf("ReservedContextSize = %d, want 50000", cfg.ReservedContextSize)
	}
	if cfg.MaxCompactionPerTurn != 3 {
		t.Errorf("MaxCompactionPerTurn = %d, want 3", cfg.MaxCompactionPerTurn)
	}
	if cfg.MaxOverflowAttempts != 3 {
		t.Errorf("MaxOverflowAttempts = %d, want 3", cfg.MaxOverflowAttempts)
	}
}
