package permission

import (
	"encoding/json"
	"sync"
)

// Mode is the permission operating mode.
type Mode string

const (
	ModeManual Mode = "manual" // ask for everything
	ModeYolo   Mode = "yolo"   // approve everything
	ModeAuto   Mode = "auto"   // approve safe, ask for risky
)

// ModeManager holds the current permission mode and rebuilds the chain on switch.
type ModeManager struct {
	mu       sync.RWMutex
	mode     Mode
	chain    *Chain
	onChange func(Mode) // optional callback when mode changes
}

// NewModeManager creates a mode manager starting in the given mode.
func NewModeManager(initial Mode) *ModeManager {
	m := &ModeManager{mode: initial}
	m.chain = m.buildChain(initial)
	return m
}

// Mode returns the current permission mode.
func (m *ModeManager) Mode() Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// Chain returns the current permission chain.
func (m *ModeManager) Chain() *Chain {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.chain
}

// SetMode switches the permission mode and rebuilds the chain.
func (m *ModeManager) SetMode(mode Mode) {
	m.mu.Lock()
	m.mode = mode
	m.chain = m.buildChain(mode)
	cb := m.onChange
	m.mu.Unlock()
	if cb != nil {
		cb(mode)
	}
}

// OnChange registers a callback invoked when the mode changes.
func (m *ModeManager) OnChange(fn func(Mode)) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// Evaluate delegates to the current chain.
func (m *ModeManager) Evaluate(toolName string, input json.RawMessage) Result {
	return m.Chain().Evaluate(toolName, input)
}

func (m *ModeManager) buildChain(mode Mode) *Chain {
	switch mode {
	case ModeYolo:
		return YoloChain()
	case ModeAuto:
		return AutoChain()
	default:
		return DefaultChain()
	}
}

// AutoChain returns a policy chain for auto mode:
// sensitive files denied, safe read-only tools approved, risky tools ask.
func AutoChain() *Chain {
	return NewChain(
		NewSensitiveFilePolicy(),
		NewSafeToolApprovePolicy(),
		NewFallbackAskPolicy(),
	)
}

// SafeToolApprovePolicy auto-approves known-safe read-only tools.
type SafeToolApprovePolicy struct{}

func NewSafeToolApprovePolicy() *SafeToolApprovePolicy { return &SafeToolApprovePolicy{} }

func (p *SafeToolApprovePolicy) Name() string { return "safe-tool-approve" }

// safeTools are tools that are read-only and non-destructive.
var safeTools = map[string]bool{
	"Read":      true,
	"Glob":      true,
	"Grep":      true,
	"FetchURL":  true,
	"WebSearch": true,
	"ReadMedia": true,
}

func (p *SafeToolApprovePolicy) Evaluate(toolName string, _ json.RawMessage) Result {
	if safeTools[toolName] {
		return Result{Decision: DecisionAllow, Reason: "safe read-only tool", Policy: p.Name()}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}
