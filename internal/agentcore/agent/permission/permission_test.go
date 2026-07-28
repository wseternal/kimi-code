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
