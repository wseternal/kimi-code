package permission

import (
	"encoding/json"
	"testing"
)

func TestDecision_String(t *testing.T) {
	if DecisionAllow.String() != "allow" {
		t.Errorf("expected 'allow', got %s", DecisionAllow.String())
	}
	if DecisionDeny.String() != "deny" {
		t.Errorf("expected 'deny', got %s", DecisionDeny.String())
	}
	if DecisionAsk.String() != "ask" {
		t.Errorf("expected 'ask', got %s", DecisionAsk.String())
	}
}

func TestAutoApprovePolicy(t *testing.T) {
	p := NewAutoApprovePolicy()
	dec := p.Evaluate("Bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow, got %s", dec.Decision)
	}
}

func TestAutoDenyPolicy(t *testing.T) {
	p := NewAutoDenyPolicy("testing")
	dec := p.Evaluate("Bash", json.RawMessage(`{"command":"echo hi"}`))
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny, got %s", dec.Decision)
	}
	if dec.Reason != "testing" {
		t.Errorf("expected 'testing' reason, got %s", dec.Reason)
	}
}

func TestSensitiveFilePolicy_DeniesEnv(t *testing.T) {
	p := NewSensitiveFilePolicy()

	tests := []struct {
		tool     string
		input    string
		wantDec  Decision
	}{
		{"Read", `{"path": ".env"}`, DecisionDeny},
		{"Read", `{"path": "~/.aws/credentials"}`, DecisionDeny},
		{"Write", `{"path": "id_rsa"}`, DecisionDeny},
		{"Edit", `{"path": ".env.local"}`, DecisionDeny},
		{"Read", `{"path": "src/main.go"}`, DecisionAllow},
		{"Read", `{"path": ".env.example"}`, DecisionAllow},
		{"Bash", `{"command": "cat .env"}`, DecisionAsk}, // not a file tool → ask
		{"Glob", `{"pattern": "**/*.go"}`, DecisionAsk},
	}

	for _, tt := range tests {
		t.Run(tt.tool+"_"+tt.input, func(t *testing.T) {
			dec := p.Evaluate(tt.tool, json.RawMessage(tt.input))
			if dec.Decision != tt.wantDec {
				t.Errorf("Evaluate(%s, %s) = %s, want %s", tt.tool, tt.input, dec.Decision, tt.wantDec)
			}
		})
	}
}

func TestChain_AllowFirst(t *testing.T) {
	chain := NewChain(
		NewSensitiveFilePolicy(),
		NewAutoApprovePolicy(),
	)

	// Non-sensitive file → allow (from auto-approve)
	dec := chain.Evaluate("Read", json.RawMessage(`{"path": "main.go"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow, got %s", dec.Decision)
	}

	// Sensitive file → deny (from sensitive policy, before auto-approve)
	dec = chain.Evaluate("Read", json.RawMessage(`{"path": ".env"}`))
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny, got %s", dec.Decision)
	}
}

func TestChain_FallbackAsk(t *testing.T) {
	chain := NewChain(
		NewSensitiveFilePolicy(),
		NewFallbackAskPolicy(),
	)

	// Non-sensitive file → ask (fallback)
	dec := chain.Evaluate("Bash", json.RawMessage(`{"command": "ls"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask, got %s", dec.Decision)
	}
}

func TestIsSensitiveFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{".env.example", false},
		{".env.sample", false},
		{"id_rsa", true},
		{"id_rsa.pub", false},
		{"id_ed25519", true},
		{"credentials", true},
		{"~/.aws/credentials", true},
		{"src/main.go", false},
		{"package.json", false},
		{"id_rsa.pem", true},
		{"id_rsa.bak", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsSensitiveFile(tt.path); got != tt.want {
				t.Errorf("IsSensitiveFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestToolNeedsApproval_ReadOnlyTools(t *testing.T) {
	chain := NewChain(
		NewSensitiveFilePolicy(),
		NewFallbackAskPolicy(),
	)

	// Read-only tools that aren't sensitive should still ask (fallback)
	dec := chain.Evaluate("Glob", json.RawMessage(`{"pattern": "**/*.go"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for Glob, got %s", dec.Decision)
	}
}

func TestDefaultChain(t *testing.T) {
	chain := DefaultChain()

	// Bash should ask
	dec := chain.Evaluate("Bash", json.RawMessage(`{"command": "ls"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for Bash, got %s", dec.Decision)
	}

	// Sensitive read should deny
	dec = chain.Evaluate("Read", json.RawMessage(`{"path": ".env"}`))
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny for sensitive read, got %s", dec.Decision)
	}
}

func TestYoloChain(t *testing.T) {
	chain := YoloChain()

	// Everything should allow
	dec := chain.Evaluate("Bash", json.RawMessage(`{"command": "rm -rf /"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow in yolo mode, got %s", dec.Decision)
	}

	dec = chain.Evaluate("Read", json.RawMessage(`{"path": ".env"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow in yolo mode for sensitive file, got %s", dec.Decision)
	}
}

// ── Mode Manager Tests ──

func TestModeManager_Default(t *testing.T) {
	m := NewModeManager(ModeManual)
	if m.Mode() != ModeManual {
		t.Errorf("expected manual mode, got %s", m.Mode())
	}
	// Manual mode: Bash should ask
	dec := m.Evaluate("Bash", json.RawMessage(`{"command":"ls"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask in manual mode, got %s", dec.Decision)
	}
}

func TestModeManager_SwitchToYolo(t *testing.T) {
	m := NewModeManager(ModeManual)
	m.SetMode(ModeYolo)
	if m.Mode() != ModeYolo {
		t.Errorf("expected yolo mode, got %s", m.Mode())
	}
	dec := m.Evaluate("Bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow in yolo mode, got %s", dec.Decision)
	}
}

func TestModeManager_AutoMode(t *testing.T) {
	m := NewModeManager(ModeAuto)

	// Safe tool → allow
	dec := m.Evaluate("Read", json.RawMessage(`{"path":"main.go"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow for safe tool in auto mode, got %s", dec.Decision)
	}

	// Risky tool → ask
	dec = m.Evaluate("Bash", json.RawMessage(`{"command":"ls"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for risky tool in auto mode, got %s", dec.Decision)
	}

	// Sensitive file → deny
	dec = m.Evaluate("Read", json.RawMessage(`{"path":".env"}`))
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny for sensitive file in auto mode, got %s", dec.Decision)
	}
}

func TestModeManager_OnChange(t *testing.T) {
	m := NewModeManager(ModeManual)
	var called bool
	m.OnChange(func(mode Mode) {
		called = true
		if mode != ModeYolo {
			t.Errorf("expected yolo in callback, got %s", mode)
		}
	})
	m.SetMode(ModeYolo)
	if !called {
		t.Error("expected onChange callback to fire")
	}
}

// ── Policy Tests ──

func TestPlanModeGuardPolicy(t *testing.T) {
	p := NewPlanModeGuardPolicy()

	// Read should be allowed
	dec := p.Evaluate("Read", json.RawMessage(`{"path":"main.go"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow for Read in plan mode, got %s", dec.Decision)
	}

	// Bash should be denied
	dec = p.Evaluate("Bash", json.RawMessage(`{"command":"ls"}`))
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny for Bash in plan mode, got %s", dec.Decision)
	}

	// Write should be denied
	dec = p.Evaluate("Write", json.RawMessage(`{"path":"x.go","content":""}`))
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny for Write in plan mode, got %s", dec.Decision)
	}

	// ExitPlanMode should be allowed
	dec = p.Evaluate("ExitPlanMode", nil)
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow for ExitPlanMode in plan mode, got %s", dec.Decision)
	}
}

func TestUserConfiguredRulesPolicy(t *testing.T) {
	rules := []UserRule{
		{Tool: "Bash", Allow: true},
		{Tool: "Write", Path: "*.go", Allow: false},
	}
	p := NewUserConfiguredRulesPolicy(rules)

	// Bash should be allowed by rule
	dec := p.Evaluate("Bash", json.RawMessage(`{"command":"ls"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow for Bash by user rule, got %s", dec.Decision)
	}

	// Write to .go file should be denied
	dec = p.Evaluate("Write", json.RawMessage(`{"path":"main.go","content":""}`))
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny for Write to *.go by user rule, got %s", dec.Decision)
	}

	// Write to .md file should ask (no matching rule)
	dec = p.Evaluate("Write", json.RawMessage(`{"path":"README.md","content":""}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for Write to *.md (no matching rule), got %s", dec.Decision)
	}
}

func TestAutoModeAskUserQuestionDenyPolicy(t *testing.T) {
	p := NewAutoModeAskUserQuestionDenyPolicy()

	dec := p.Evaluate("AskUser", nil)
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny for AskUser, got %s", dec.Decision)
	}

	dec = p.Evaluate("Bash", json.RawMessage(`{"command":"ls"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for Bash, got %s", dec.Decision)
	}
}

func TestAgentSwarmExclusiveDenyPolicy(t *testing.T) {
	p := NewAgentSwarmExclusiveDenyPolicy()

	// Not a sub-agent → ask (pass-through)
	dec := p.Evaluate("AskUser", nil)
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for non-subagent, got %s", dec.Decision)
	}

	// Mark as sub-agent
	p.SetSubAgent(true)

	dec = p.Evaluate("AskUser", nil)
	if dec.Decision != DecisionDeny {
		t.Errorf("expected deny for AskUser as sub-agent, got %s", dec.Decision)
	}

	// Bash should still ask (not in denied list)
	dec = p.Evaluate("Bash", json.RawMessage(`{"command":"ls"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for Bash as sub-agent, got %s", dec.Decision)
	}
}

func TestGitCWDWriteApprovePolicy(t *testing.T) {
	p := NewGitCWDWriteApprovePolicy()

	// Safe git command → allow
	dec := p.Evaluate("Bash", json.RawMessage(`{"command":"git status"}`))
	if dec.Decision != DecisionAllow {
		t.Errorf("expected allow for git status, got %s", dec.Decision)
	}

	// Unsafe command → ask
	dec = p.Evaluate("Bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for rm, got %s", dec.Decision)
	}

	// Non-bash tool → ask
	dec = p.Evaluate("Read", json.RawMessage(`{"path":"x.go"}`))
	if dec.Decision != DecisionAsk {
		t.Errorf("expected ask for non-bash tool, got %s", dec.Decision)
	}
}

func TestSafeToolApprovePolicy(t *testing.T) {
	p := NewSafeToolApprovePolicy()

	tests := []struct {
		tool string
		want Decision
	}{
		{"Read", DecisionAllow},
		{"Glob", DecisionAllow},
		{"Grep", DecisionAllow},
		{"FetchURL", DecisionAllow},
		{"Bash", DecisionAsk},
		{"Write", DecisionAsk},
		{"Edit", DecisionAsk},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			dec := p.Evaluate(tt.tool, nil)
			if dec.Decision != tt.want {
				t.Errorf("SafeToolApprovePolicy(%s) = %s, want %s", tt.tool, dec.Decision, tt.want)
			}
		})
	}
}
