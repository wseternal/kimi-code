// Package permission implements a policy chain for tool call approval.
package permission

import (
	"encoding/json"
	"fmt"
)

// ApprovalRequest is sent when a tool needs user approval.
type ApprovalRequest struct {
	ToolName string          `json:"toolName"`
	Args     json.RawMessage `json:"args"`
	Reason   string          `json:"reason"`
	RespCh   chan ApprovalResponse
}

// ApprovalResponse is the user's decision on an approval request.
type ApprovalResponse struct {
	Approved     bool   `json:"approved"`
	AlwaysAllow  bool   `json:"alwaysAllow"` // approve for rest of session
	SessionScope bool   `json:"sessionScope"` // approve for this session
	Reason       string `json:"reason,omitempty"`
}

// Prompter handles interactive approval via a channel.
// The TUI layer reads from RequestCh and writes responses.
type Prompter struct {
	RequestCh chan ApprovalRequest
	// sessionApprovals tracks tool+pattern combos approved for this session.
	sessionApprovals map[string]bool
}

// NewPrompter creates a new interactive prompter.
func NewPrompter() *Prompter {
	return &Prompter{
		RequestCh:        make(chan ApprovalRequest, 16),
		sessionApprovals: make(map[string]bool),
	}
}

// Ask sends an approval request and waits for the user's response.
// Returns the decision result.
func (p *Prompter) Ask(toolName string, args json.RawMessage, reason string) Result {
	// Check session-scoped approvals first
	key := toolName
	if p.sessionApprovals[key] {
		return Result{Decision: DecisionAllow, Reason: "session-approved", Policy: "session"}
	}

	req := ApprovalRequest{
		ToolName: toolName,
		Args:     args,
		Reason:   reason,
		RespCh:   make(chan ApprovalResponse, 1),
	}

	select {
	case p.RequestCh <- req:
	default:
		// Channel full — deny by default for safety
		return Result{Decision: DecisionDeny, Reason: "approval channel full", Policy: "prompter"}
	}

	resp, ok := <-req.RespCh
	if !ok {
		return Result{Decision: DecisionDeny, Reason: "approval cancelled", Policy: "prompter"}
	}

	if resp.AlwaysAllow || resp.SessionScope {
		p.sessionApprovals[key] = true
	}

	if resp.Approved {
		return Result{Decision: DecisionAllow, Reason: "user approved", Policy: "prompter"}
	}
	return Result{Decision: DecisionDeny, Reason: fmt.Sprintf("user denied: %s", resp.Reason), Policy: "prompter"}
}

// ClearSessionApprovals removes all session-scoped approvals.
func (p *Prompter) ClearSessionApprovals() {
	p.sessionApprovals = make(map[string]bool)
}
