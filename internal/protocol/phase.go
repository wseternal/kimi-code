package protocol

// AgentPhase represents the current state of the agent within a session turn.
// The phase transitions follow a state machine:
//
//	idle → running → streaming → tool_call → running → ... → idle
//	                                    ↘ retrying → running
//	                any → awaiting_approval → running
//	                any → interrupted → idle
//	                any → ended
type AgentPhase string

const (
	PhaseIdle             AgentPhase = "idle"
	PhaseRunning          AgentPhase = "running"
	PhaseStreaming        AgentPhase = "streaming"
	PhaseToolCall         AgentPhase = "tool_call"
	PhaseRetrying         AgentPhase = "retrying"
	PhaseAwaitingApproval AgentPhase = "awaiting_approval"
	PhaseInterrupted      AgentPhase = "interrupted"
	PhaseEnded            AgentPhase = "ended"
)

// IsTerminal reports whether the phase represents a terminal state.
func (p AgentPhase) IsTerminal() bool {
	return p == PhaseIdle || p == PhaseEnded
}

// IsActive reports whether the agent is actively processing.
func (p AgentPhase) IsActive() bool {
	switch p {
	case PhaseRunning, PhaseStreaming, PhaseToolCall, PhaseRetrying:
		return true
	}
	return false
}

// PhaseTransition describes a transition between phases.
type PhaseTransition struct {
	From      AgentPhase `json:"from"`
	To        AgentPhase `json:"to"`
	Reason    string     `json:"reason,omitempty"`
	Timestamp string     `json:"timestamp"`
}
