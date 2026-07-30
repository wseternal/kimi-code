package protocol

// ApprovalRequest is sent when the agent needs user approval for a tool call.
type ApprovalRequest struct {
	ID         string `json:"id"`
	SessionID  string `json:"session_id"`
	TurnID     string `json:"turn_id,omitempty"`
	StepID     string `json:"step_id,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`

	ToolName   string `json:"tool_name"`
	ToolInput  string `json:"tool_input"`  // JSON string of the tool input
	Reason     string `json:"reason,omitempty"`

	// DiffPreview shows a proposed file change for approval
	DiffPreview *DiffPreview `json:"diff_preview,omitempty"`

	Status     ApprovalStatus `json:"status"`
	CreatedAt  string         `json:"created_at"`
	ResolvedAt string         `json:"resolved_at,omitempty"`
}

// ApprovalStatus is the lifecycle state of an approval request.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalDenied   ApprovalStatus = "denied"
	ApprovalExpired  ApprovalStatus = "expired"
)

// DiffPreview shows the proposed file change for approval.
type DiffPreview struct {
	Path     string `json:"path"`
	OldText  string `json:"old_text,omitempty"`
	NewText  string `json:"new_text"`
	Language string `json:"language,omitempty"`
}

// ApprovalResponse is the user's response to an approval request.
type ApprovalResponse struct {
	ID     string         `json:"id"`
	Status ApprovalStatus `json:"status"`
	Reason string         `json:"reason,omitempty"`

	// AllowAlways creates a permission rule for this tool pattern
	AllowAlways bool `json:"allow_always,omitempty"`
}

// QuestionRequest is sent when the agent asks the user a structured question.
type QuestionRequest struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	TurnID      string          `json:"turn_id,omitempty"`
	StepID      string          `json:"step_id,omitempty"`
	AgentID     string          `json:"agent_id,omitempty"`

	Question    string          `json:"question"`
	Options     []QuestionOption `json:"options,omitempty"`
	MultiSelect bool            `json:"multi_select,omitempty"`
	DefaultIdx  *int            `json:"default_index,omitempty"`

	Status      QuestionStatus  `json:"status"`
	CreatedAt   string          `json:"created_at"`
	ResolvedAt  string          `json:"resolved_at,omitempty"`
}

// QuestionOption is a single choice in a question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value,omitempty"`
}

// QuestionStatus is the lifecycle state of a question request.
type QuestionStatus string

const (
	QuestionPending  QuestionStatus = "pending"
	QuestionAnswered QuestionStatus = "answered"
	QuestionDismissed QuestionStatus = "dismissed"
	QuestionExpired  QuestionStatus = "expired"
)

// QuestionResponse is the user's answer to a question.
type QuestionResponse struct {
	ID              string `json:"id"`
	SelectedIndices []int  `json:"selected_indices,omitempty"`
	FreeText        string `json:"free_text,omitempty"`
	Dismissed       bool   `json:"dismissed,omitempty"`
}
