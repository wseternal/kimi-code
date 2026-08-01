// Package transcript implements an event-sourced session recording system.
// It provides operation-based recording of turns, steps, frames, interactions,
// attachments, todos, tasks, and metadata, with pagination and history grouping.
package transcript

import (
	"fmt"
	"time"
)

// ── IDs ──

// TurnID uniquely identifies a turn within a transcript.
type TurnID string

// StepID uniquely identifies a step within a turn.
type StepID string

// FrameID uniquely identifies a frame within a step.
type FrameID string

// TaskID uniquely identifies a task (shell/subagent/tool execution).
type TaskID string

// InteractionID uniquely identifies an interaction (approval/question).
type InteractionID string

// NewTurnID creates a new turn ID.
func NewTurnID(n int) TurnID { return TurnID(fmt.Sprintf("turn_%d", n)) }

// NewStepID creates a new step ID.
func NewStepID(turn TurnID, n int) StepID {
	return StepID(fmt.Sprintf("%s:step_%d", turn, n))
}

// NewFrameID creates a new frame ID.
func NewFrameID(step StepID, n int) FrameID {
	return FrameID(fmt.Sprintf("%s:frame_%d", step, n))
}

// NewTaskID creates a new task ID.
func NewTaskID(s string) TaskID { return TaskID(s) }

// NewInteractionID creates a new interaction ID.
func NewInteractionID(s string) InteractionID { return InteractionID(s) }

// ── Turn ──

// TurnOrigin describes how a turn was initiated.
type TurnOrigin struct {
	Kind   string `json:"kind"`   // "user", "system", "cron", "steering"
	Prompt string `json:"prompt"` // the original prompt text
}

// StepUsage records token usage for a single step.
type StepUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	CacheRead    int `json:"cacheRead,omitempty"`
	CacheWrite   int `json:"cacheWrite,omitempty"`
}

// StepTiming records timing for a single step.
type StepTiming struct {
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	DurationMs int       `json:"durationMs,omitempty"`
}

// StepRetry records retry information for a step.
type StepRetry struct {
	Count  int    `json:"count"`
	Reason string `json:"reason,omitempty"`
}

// TranscriptStep represents a single LLM step within a turn.
type TranscriptStep struct {
	ID          StepID     `json:"id"`
	TurnID      TurnID     `json:"turnId"`
	Index       int        `json:"index"`
	FinishReason string    `json:"finishReason,omitempty"` // "completed", "tool_calls", "truncated", "error"
	Usage       *StepUsage `json:"usage,omitempty"`
	Timing      *StepTiming `json:"timing,omitempty"`
	Retry       *StepRetry  `json:"retry,omitempty"`
	Error       string      `json:"error,omitempty"`
}

// TranscriptTurn represents a single user→assistant exchange.
type TranscriptTurn struct {
	ID         TurnID           `json:"id"`
	Index      int              `json:"index"`
	Origin     *TurnOrigin      `json:"origin,omitempty"`
	Steps      []TranscriptStep `json:"steps"`
	CreatedAt  time.Time        `json:"createdAt"`
	FinishedAt *time.Time       `json:"finishedAt,omitempty"`
}

// ── Frames ──

// FrameKind identifies the type of frame.
type FrameKind string

const (
	FrameText      FrameKind = "text"
	FrameThinking  FrameKind = "thinking"
	FrameToolCall  FrameKind = "tool_call"
	FrameToolResult FrameKind = "tool_result"
	FrameNotice    FrameKind = "notice"
)

// TextFrame represents assistant text output.
type TextFrame struct {
	Kind    FrameKind `json:"kind"`
	Content string    `json:"content"`
}

// ThinkingFrame represents model thinking/reasoning output.
type ThinkingFrame struct {
	Kind    FrameKind `json:"kind"`
	Content string    `json:"content"`
}

// ToolCallFrame represents a tool invocation.
type ToolCallFrame struct {
	Kind   FrameKind `json:"kind"`
	Name   string    `json:"name"`
	Args   string    `json:"args,omitempty"`
	Result string    `json:"result,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// NoticeFrame represents a system notice or message.
type NoticeFrame struct {
	Kind    FrameKind `json:"kind"`
	Content string    `json:"content"`
	Level   string    `json:"level,omitempty"` // "info", "warning", "error"
}

// Frame is a union type for all frame types.
type Frame struct {
	ID      FrameID   `json:"id"`
	StepID  StepID    `json:"stepId"`
	Index   int       `json:"index"`
	Kind    FrameKind `json:"kind"`
	// Only one of these is populated based on Kind
	Text      *TextFrame      `json:"text,omitempty"`
	Thinking  *ThinkingFrame  `json:"thinking,omitempty"`
	ToolCall  *ToolCallFrame  `json:"toolCall,omitempty"`
	Notice    *NoticeFrame    `json:"notice,omitempty"`
}

// ── Interaction ──

// InteractionKind is the type of interaction.
type InteractionKind string

const (
	InteractionApproval InteractionKind = "approval"
	InteractionQuestion InteractionKind = "question"
)

// TranscriptInteraction represents an approval or question interaction.
type TranscriptInteraction struct {
	ID        InteractionID   `json:"id"`
	Kind      InteractionKind `json:"kind"`
	Prompt    string          `json:"prompt"`
	Resolved  bool            `json:"resolved"`
	Response  string          `json:"response,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	ResolvedAt *time.Time     `json:"resolvedAt,omitempty"`
}

// ── Attachment ──

// TranscriptAttachment represents a media/file attachment.
type TranscriptAttachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	FetchRef string `json:"fetchRef,omitempty"`
	URL      string `json:"url,omitempty"`
}

// ── Todo ──

// TodoItem is a single item in a todo list.
type TodoItem struct {
	Title  string `json:"title"`
	Status string `json:"status"` // "pending", "in_progress", "done", "cancelled"
}

// TranscriptTodo represents a whole-document todo list replacement.
type TranscriptTodo struct {
	Items []TodoItem `json:"items"`
}

// ── Task ──

// TaskKind identifies what kind of task this is.
type TaskKind string

const (
	TaskShell    TaskKind = "shell"
	TaskSubagent TaskKind = "subagent"
	TaskTool     TaskKind = "tool"
)

// TranscriptTask represents a shell/subagent/tool execution entity.
type TranscriptTask struct {
	ID        TaskID   `json:"id"`
	Kind      TaskKind `json:"kind"`
	Label     string   `json:"label"`
	Status    string   `json:"status"` // "running", "completed", "failed", "cancelled"
	Output    string   `json:"output,omitempty"`
	Error     string   `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// ── Meta ──

// GoalMeta records goal state.
type GoalMeta struct {
	Objective string `json:"objective,omitempty"`
	Status    string `json:"status,omitempty"`
}

// ModesMeta records mode states.
type ModesMeta struct {
	PlanMode  bool `json:"planMode,omitempty"`
	SwarmMode bool `json:"swarmMode,omitempty"`
	YoloMode  bool `json:"yoloMode,omitempty"`
}

// AgentPhaseMeta records the current agent phase.
type AgentPhaseMeta struct {
	Phase string `json:"phase"` // "idle", "thinking", "acting", "streaming"
}

// AgentStatusMeta records high-level agent status.
type AgentStatusMeta struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// TranscriptMeta aggregates all metadata.
type TranscriptMeta struct {
	Goal        *GoalMeta        `json:"goal,omitempty"`
	Modes       *ModesMeta       `json:"modes,omitempty"`
	AgentPhase  *AgentPhaseMeta  `json:"agentPhase,omitempty"`
	AgentStatus *AgentStatusMeta `json:"agentStatus,omitempty"`
	Model       string           `json:"model,omitempty"`
	Provider    string           `json:"provider,omitempty"`
	Custom      map[string]any   `json:"custom,omitempty"`
}

// ── Marker & TaskRef (items) ──

// TranscriptMarker is a visual marker in the transcript.
type TranscriptMarker struct {
	Kind  string `json:"kind"` // "divider", "bookmark"
	Label string `json:"label,omitempty"`
}

// TranscriptTaskRef references a task from a turn.
type TranscriptTaskRef struct {
	TaskID TaskID `json:"taskId"`
}

// TranscriptItem is a union of marker and taskref, associated with a turn.
type TranscriptItem struct {
	TurnID  TurnID             `json:"turnId"`
	Marker  *TranscriptMarker  `json:"marker,omitempty"`
	TaskRef *TranscriptTaskRef `json:"taskRef,omitempty"`
}

// ── Prompt ──

// TranscriptPrompt represents a prompt in the queue.
type TranscriptPrompt struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Status    string    `json:"status"` // "queued", "processing", "processed"
	CreatedAt time.Time `json:"createdAt"`
}

// ── Snapshot (the full transcript state) ──

// Snapshot is the complete transcript state.
type Snapshot struct {
	Turns        []TranscriptTurn        `json:"turns"`
	Frames       []Frame                 `json:"frames"`
	Interactions []TranscriptInteraction  `json:"interactions"`
	Attachments  []TranscriptAttachment   `json:"attachments"`
	Todos        *TranscriptTodo          `json:"todos,omitempty"`
	Tasks        []TranscriptTask         `json:"tasks"`
	Items        []TranscriptItem         `json:"items"`
	Prompts      []TranscriptPrompt       `json:"prompts"`
	Meta         TranscriptMeta           `json:"meta"`
}

// NewSnapshot creates an empty transcript snapshot.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		Turns:        make([]TranscriptTurn, 0),
		Frames:       make([]Frame, 0),
		Interactions: make([]TranscriptInteraction, 0),
		Attachments:  make([]TranscriptAttachment, 0),
		Tasks:        make([]TranscriptTask, 0),
		Items:        make([]TranscriptItem, 0),
		Prompts:      make([]TranscriptPrompt, 0),
	}
}
