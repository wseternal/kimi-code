// Package hooks implements lifecycle hook management for the agent system.
// Hooks are user-defined shell commands that execute at specific lifecycle
// points (PreToolUse, PostToolUse, SessionStart, etc.) and can optionally
// block tool execution.
package hooks

import "time"

// EventType identifies a lifecycle hook event.
type EventType string

const (
	PreToolUse         EventType = "PreToolUse"
	PostToolUse        EventType = "PostToolUse"
	PostToolUseFailure EventType = "PostToolUseFailure"
	UserPromptSubmit   EventType = "UserPromptSubmit"
	Stop               EventType = "Stop"
	Interrupt          EventType = "Interrupt"
	SessionStart       EventType = "SessionStart"
	SessionEnd         EventType = "SessionEnd"
	SubagentStart      EventType = "SubagentStart"
	SubagentStop       EventType = "SubagentStop"
	PreCompact         EventType = "PreCompact"
	PostCompact        EventType = "PostCompact"
	Notification       EventType = "Notification"
)

// AllEventTypes lists all supported hook event types.
var AllEventTypes = []EventType{
	PreToolUse, PostToolUse, PostToolUseFailure,
	UserPromptSubmit, Stop, Interrupt,
	SessionStart, SessionEnd,
	SubagentStart, SubagentStop,
	PreCompact, PostCompact,
	Notification,
}

// HookDef defines a single hook from config.toml [[hooks]] sections.
//
//	[[hooks]]
//	event = "PreToolUse"
//	matcher = "Bash"        # optional regex/glob on the event subject
//	command = "echo checking" # shell command to execute
//	timeout = 30             # seconds, default 30
type HookDef struct {
	Event   EventType `toml:"event" json:"event"`
	Matcher string    `toml:"matcher,omitempty" json:"matcher,omitempty"`
	Command string    `toml:"command" json:"command"`
	Timeout int       `toml:"timeout,omitempty" json:"timeout,omitempty"`
}

// HookInput is the JSON payload sent to the hook command on stdin.
type HookInput struct {
	Event   EventType      `json:"event"`
	Matcher string         `json:"matcher,omitempty"`
	Tool    *HookToolInput `json:"tool,omitempty"`
	Session *HookSession   `json:"session,omitempty"`
}

// HookToolInput provides tool-specific context for PreToolUse/PostToolUse.
type HookToolInput struct {
	Name      string `json:"name"`
	Input     string `json:"input,omitempty"`
	Output    string `json:"output,omitempty"`
	IsError   bool   `json:"isError,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// HookSession provides session-level context.
type HookSession struct {
	ID      string `json:"id"`
	WorkDir string `json:"workDir,omitempty"`
}

// HookResult is the outcome of running a hook.
type HookResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	Blocked  bool   // true if exit code 2 or stdout has action:"block"
	Reason   string // human-readable block reason
	Err      error  // execution error (timeout, spawn failure, etc.)
}

// BlockDecision represents the result of a blocking hook trigger.
type BlockDecision struct {
	Blocked bool
	Reason  string
	Results []HookResult
}

// DefaultTimeout is the default hook execution timeout in seconds.
const DefaultTimeout = 30
