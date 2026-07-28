package audit

import (
	"encoding/json"
	"fmt"
	"time"
)

// Key prefixes for the BadgerDB schema.
//
// Session metadata:
//
//	s:session:{id}                    → JSON (SessionRecord)
//	s:updated:{unix_nano}:{id}        → "" (index for GetLatest / ListSessions)
//
// Audit events (append-only, chronologically ordered):
//
//	a:evt:{session_id}:{unix_nano}:{seq} → JSON (StoredEvent)
const (
	prefixSession = "s:session:"
	prefixUpdated = "s:updated:"
	prefixEvent   = "a:evt:"
)

// Event type constants for audit trail entries.
const (
	// LLM events
	EvtLLMRequest    = "llm.request"
	EvtLLMDeltaText  = "llm.delta.text"
	EvtLLMDeltaThink = "llm.delta.think"
	EvtLLMToolCall   = "llm.tool_call"
	EvtLLMToolResult = "llm.tool_result"
	EvtLLMUsage      = "llm.usage"
	EvtLLMStepDone   = "llm.step_done"
	EvtLLMError      = "llm.error"
	EvtLLMDone       = "llm.done"

	// Raw HTTP payload events — verbatim request/response bodies for
	// post-mortem diagnostics. These can be large and are stored
	// uncompressed so they can be fed back to an LLM for analysis.
	EvtLLMRawRequest  = "llm.raw_request"
	EvtLLMRawResponse = "llm.raw_response"

	// High-level turn event — recorded once per completed turn with the
	// full prompt, response, thinking, and tool calls. The facade uses
	// these to reconstruct session state without replaying deltas.
	EvtTurnCompleted = "turn.completed"

	// User events
	EvtUserInput        = "user.input"
	EvtUserCancel       = "user.cancel"
	EvtUserCommand      = "user.command"
	EvtUserSessionSwitch = "user.session.switch"
)

// AuditEvent is the in-memory representation of an event to be recorded.
type AuditEvent struct {
	SessionID string    `json:"sessionId"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Seq       uint64    `json:"seq"`
	Data      any       `json:"data,omitempty"`
}

// StoredEvent is the on-disk JSON representation in BadgerDB.
type StoredEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
	Ts   int64           `json:"ts"` // unix nano
	Seq  uint64          `json:"seq"`
}

// SessionRecord is the on-disk JSON for session metadata.
type SessionRecord struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// SessionSummary is a lightweight view for session listing.
type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TurnRecord captures the complete data for a single completed turn.
type TurnRecord struct {
	Prompt       string           `json:"prompt"`
	Response     string           `json:"response"`
	Thinking     string           `json:"thinking,omitempty"`
	Tools        []ToolCallRecord `json:"tools,omitempty"`
	Usage        *UsageRecord     `json:"usage,omitempty"`
	FinishReason string           `json:"finishReason,omitempty"`
}

// ToolCallRecord captures a tool invocation within a turn.
type ToolCallRecord struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Arguments string        `json:"arguments,omitempty"`
	Result    string        `json:"result,omitempty"`
	IsError   bool          `json:"isError,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
}

// UsageRecord captures token usage for a single turn.
type UsageRecord struct {
	InputOther         int `json:"inputOther"`
	Output             int `json:"output"`
	InputCacheRead     int `json:"inputCacheRead"`
	InputCacheCreation int `json:"inputCacheCreation"`
	ReasoningTokens    int `json:"reasoningTokens,omitempty"`
}

// Key builder functions — pure, no I/O.

func keySession(id string) []byte {
	return []byte(prefixSession + id)
}

func keyUpdated(ts time.Time, id string) []byte {
	// Fixed-width 20-digit nanosecond timestamp for lexicographic ordering.
	return []byte(fmt.Sprintf("%s%020d:%s", prefixUpdated, ts.UnixNano(), id))
}

func keyEvent(sessionID string, ts time.Time, seq uint64) []byte {
	// Fixed-width timestamp + sequence for chronological ordering within a session.
	return []byte(fmt.Sprintf("%s%s:%020d:%016x", prefixEvent, sessionID, ts.UnixNano(), seq))
}

// parseUpdatedKey extracts the session ID from an s:updated: key.
func parseUpdatedKey(key []byte) string {
	s := string(key)
	// Format: s:updated:{20-digit-nano}:{id}
	for i := len(prefixUpdated); i < len(s); i++ {
		if s[i] == ':' {
			return s[i+1:]
		}
	}
	return ""
}
