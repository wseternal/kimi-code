package rest

import (
	"encoding/json"

	"github.com/visdomtech/kimi-code/internal/protocol"
)

// ── Config ──

// ConfigResponse is the response for GET /config.
type ConfigResponse struct {
	Model            string                 `json:"model"`
	Provider         string                 `json:"provider,omitempty"`
	BaseURL          string                 `json:"base_url,omitempty"`
	SystemPrompt     string                 `json:"system_prompt,omitempty"`
	PermissionMode   string                 `json:"permission_mode"`
	PlanMode         bool                   `json:"plan_mode"`
	SwarmMode        bool                   `json:"swarm_mode"`
	Thinking         *protocol.Thinking     `json:"thinking,omitempty"`
	Extra            map[string]interface{} `json:"extra,omitempty"`
}

// ConfigUpdateRequest is the body for PATCH /config.
type ConfigUpdateRequest struct {
	Model          *string            `json:"model,omitempty"`
	SystemPrompt   *string            `json:"system_prompt,omitempty"`
	PermissionMode *string            `json:"permission_mode,omitempty"`
	PlanMode       *bool              `json:"plan_mode,omitempty"`
	Thinking       *protocol.Thinking `json:"thinking,omitempty"`
}

// ── Prompts ──

// SubmitPromptRequest is the body for POST /sessions/{id}/prompts.
type SubmitPromptRequest struct {
	Text       string                `json:"text"`
	Origin     *protocol.PromptOrigin `json:"origin,omitempty"`
	Attachments []PromptAttachment   `json:"attachments,omitempty"`
}

// PromptAttachment is an attached file/image in a prompt.
type PromptAttachment struct {
	Type      string `json:"type"` // "image", "file"
	Path      string `json:"path,omitempty"`
	URL       string `json:"url,omitempty"`
	Data      string `json:"data,omitempty"` // base64
	MediaType string `json:"media_type,omitempty"`
	Name      string `json:"name,omitempty"`
}

// SubmitPromptResponse is the response for POST /sessions/{id}/prompts.
type SubmitPromptResponse struct {
	PromptID string `json:"prompt_id"`
}

// ── Approvals ──

// ListApprovalsResponse is the response for GET /sessions/{id}/approvals.
type ListApprovalsResponse struct {
	Items   []protocol.ApprovalRequest `json:"items"`
	HasMore bool                       `json:"has_more"`
}

// ResolveApprovalRequest is the body for POST /sessions/{id}/approvals/{approval_id}.
type ResolveApprovalRequest = protocol.ApprovalResponse

// ── Questions ──

// ListQuestionsResponse is the response for GET /sessions/{id}/questions.
type ListQuestionsResponse struct {
	Items   []protocol.QuestionRequest `json:"items"`
	HasMore bool                       `json:"has_more"`
}

// ResolveQuestionRequest is the body for POST /sessions/{id}/questions/{question_id}.
type ResolveQuestionRequest = protocol.QuestionResponse

// ── Tools ──

// ListToolsResponse is the response for GET /sessions/{id}/tools.
type ListToolsResponse struct {
	Items []ToolDescriptor `json:"items"`
}

// ToolDescriptor describes a registered tool.
type ToolDescriptor struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Source      string                 `json:"source,omitempty"` // "builtin", "mcp", "plugin"
	MCPServer   string                 `json:"mcp_server,omitempty"`
}

// ── Tasks ──

// ListTasksResponse is the response for GET /sessions/{id}/tasks.
type ListTasksResponse struct {
	Items []TaskInfo `json:"items"`
}

// TaskInfo describes a running or completed background task.
type TaskInfo struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Status    protocol.TaskLifecycleStatus `json:"status"`
	AgentID   string                  `json:"agent_id,omitempty"`
	CreatedAt string                  `json:"created_at"`
	EndedAt   string                  `json:"ended_at,omitempty"`
	Output    string                  `json:"output,omitempty"`
}

// ── Terminals ──

// ListTerminalsResponse is the response for GET /sessions/{id}/terminals.
type ListTerminalsResponse struct {
	Items []TerminalInfo `json:"items"`
}

// TerminalInfo describes an active terminal.
type TerminalInfo struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Status  string `json:"status"` // "running", "completed", "killed"
	PID     int    `json:"pid,omitempty"`
}

// ── Snapshot ──

// SessionSnapshot is a point-in-time session state snapshot.
type SessionSnapshot struct {
	Session  protocol.Session          `json:"session"`
	Status   SessionStatusResponse     `json:"status"`
	Messages []protocol.Message        `json:"messages,omitempty"`
	Tasks    []TaskInfo                `json:"tasks,omitempty"`
	Phase    protocol.AgentPhase       `json:"phase,omitempty"`
}

// ── Skills ──

// ListSkillsResponse is the response for GET /sessions/{id}/skills.
type ListSkillsResponse struct {
	Items []SkillDescriptor `json:"items"`
}

// SkillDescriptor describes a discovered skill.
type SkillDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      protocol.SkillSource `json:"source"`
	Version     string `json:"version,omitempty"`
	Active      bool   `json:"active,omitempty"`
}

// ── Workspaces ──

// Workspace is a workspace descriptor.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListWorkspacesResponse is the response for GET /workspaces.
type ListWorkspacesResponse struct {
	Items []Workspace `json:"items"`
}

// CreateWorkspaceRequest is the body for POST /workspaces.
type CreateWorkspaceRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ── Search ──

// SearchRequest is the body for POST /sessions/{id}/search.
type SearchRequest struct {
	Query     string `json:"query"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

// SearchResponse is the response for search.
type SearchResponse struct {
	Items      []SearchHit `json:"items"`
	TotalCount int         `json:"total_count"`
}

// SearchHit is a single search result.
type SearchHit struct {
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Content   string `json:"content,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
}

// ── Session Export ──

// ExportSessionResponse is the response for GET /sessions/{id}/export.
type ExportSessionResponse struct {
	Session   protocol.Session   `json:"session"`
	Messages  []protocol.Message `json:"messages"`
	Usage     protocol.SessionUsage `json:"usage"`
	ExportedAt string            `json:"exported_at"`
}

// ── Meta ──

// MetaResponse is the response for GET /meta.
type MetaResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
}

// ── OAuth ──

// OAuthStatusResponse is the response for GET /oauth/status.
type OAuthStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	Provider      string `json:"provider,omitempty"`
	Email         string `json:"email,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

// OAuthLoginRequest is the body for POST /oauth/login.
type OAuthLoginRequest struct {
	Provider string `json:"provider"`
	Redirect string `json:"redirect_url,omitempty"`
}

// OAuthLoginResponse is the response for POST /oauth/login.
type OAuthLoginResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

// ── Files ──

// FileInfo describes a file in the workspace.
type FileInfo struct {
	Path      string `json:"path"`
	Size      int    `json:"size"`
	IsDir     bool   `json:"is_dir"`
	Modified  string `json:"modified,omitempty"`
	MediaType string `json:"media_type,omitempty"`
}

// ListFilesQuery is the query for GET /sessions/{id}/fs.
type ListFilesQuery struct {
	Path    string `json:"path"`
	Depth   int    `json:"depth,omitempty"`
	Include string `json:"include,omitempty"` // glob pattern
}

// ListFilesResponse is the response for file listing.
type ListFilesResponse struct {
	Items []FileInfo `json:"items"`
}

// ── Model Catalog ──

// ModelCatalogItem describes a model available to the agent.
type ModelCatalogItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	Capabilities []string `json:"capabilities,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
}

// ListModelCatalogResponse is the response for GET /model-catalog.
type ListModelCatalogResponse struct {
	Items []ModelCatalogItem `json:"items"`
}

// ── Connections ──

// ConnectionInfo describes a connected client.
type ConnectionInfo struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "ws", "api"
	CreatedAt string `json:"created_at"`
	UserAgent string `json:"user_agent,omitempty"`
}

// ListConnectionsResponse is the response for GET /connections.
type ListConnectionsResponse struct {
	Items []ConnectionInfo `json:"items"`
}

// ── Shutdown ──

// ShutdownRequest is the body for POST /shutdown.
type ShutdownRequest struct {
	Reason  string `json:"reason,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds
}

// ShutdownResponse is the response for POST /shutdown.
type ShutdownResponse struct {
	ShuttingDown bool `json:"shutting_down"`
}

// ── Transcript ──

// TranscriptEntry is a single transcript record.
type TranscriptEntry struct {
	Seq       int             `json:"seq"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp string          `json:"timestamp"`
}

// ListTranscriptQuery is the query for GET /sessions/{id}/transcript.
type ListTranscriptQuery struct {
	protocol.CursorQuery
	EventType string `json:"event_type,omitempty"`
}

// ListTranscriptResponse is the response for transcript listing.
type ListTranscriptResponse struct {
	Items   []TranscriptEntry `json:"items"`
	HasMore bool              `json:"has_more"`
}
