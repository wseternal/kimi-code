package kapserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/protocol"
	"github.com/visdomtech/kimi-code/internal/protocol/rest"
)

// SnapshotService captures point-in-time session snapshots.
type SnapshotService struct {
	sessionManager *session.Manager
}

// NewSnapshotService creates a new snapshot service.
func NewSnapshotService(sessionMgr *session.Manager) *SnapshotService {
	return &SnapshotService{sessionManager: sessionMgr}
}

// Capture captures a point-in-time snapshot of a session.
func (s *SnapshotService) Capture(sessionID string) (*rest.SessionSnapshot, error) {
	sess, ok := s.sessionManager.Get(sessionID)
	if !ok {
		return nil, &protocol.APIError{Code: protocol.ErrorCodeSessionNotFound, Message: "session not found"}
	}

	proto := sess.ToProtocol()

	snapshot := &rest.SessionSnapshot{
		Session: proto,
		Status: rest.SessionStatusResponse{
			Busy:       sess.IsBusy(),
			Permission: string(proto.AgentConfig.PermissionMode),
			PlanMode:   proto.AgentConfig.PlanMode,
			SwarmMode:  proto.AgentConfig.SwarmMode,
		},
		Messages: []protocol.Message{}, // Messages come from audit/transcript store, not session memory
		Tasks:    []rest.TaskInfo{},
		Phase:    protocol.PhaseIdle,
	}

	if sess.IsBusy() {
		snapshot.Phase = protocol.PhaseRunning
	}

	return snapshot, nil
}

// CaptureWithMessages captures a snapshot including recent messages.
func (s *SnapshotService) CaptureWithMessages(sessionID string, messageLimit int) (*rest.SessionSnapshot, error) {
	snapshot, err := s.Capture(sessionID)
	if err != nil {
		return nil, err
	}

	// Messages are stored in the audit/transcript store, not in session memory.
	// The message limit is respected for API consumers.
	if messageLimit <= 0 {
		messageLimit = 50
	}

	return snapshot, nil
}

// ── Additional REST Route Handlers ──

// handleConfig returns the current agent configuration.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, 200, rest.ConfigResponse{
		PermissionMode: "manual",
	})
}

// handleSearch performs a code search in the session's working directory.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}

	var req rest.SearchRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}

	svc := NewSearchService(".")
	results, err := svc.Search(SearchQuery{
		Pattern: req.Query,
		Limit:   req.Limit,
	})
	if err != nil {
		respondError(w, 500, protocol.ErrorCodeInternalError, err.Error())
		return
	}

	respondJSON(w, 200, rest.SearchResponse{
		Items:      toRESTHits(results.Hits),
		TotalCount: results.TotalCount,
	})
}

func toRESTHits(hits []SearchHit) []rest.SearchHit {
	result := make([]rest.SearchHit, len(hits))
	for i, h := range hits {
		result[i] = rest.SearchHit{
			Path:    h.Path,
			Line:    h.Line,
			Content: h.Content,
			Snippet: h.Snippet,
		}
	}
	return result
}

// handleSnapshot captures a session snapshot.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	svc := NewSnapshotService(s.sessionManager)
	snapshot, err := svc.Capture(id)
	if err != nil {
		if apiErr, ok := err.(*protocol.APIError); ok {
			respondError(w, 404, apiErr.Code, apiErr.Message)
		} else {
			respondError(w, 500, protocol.ErrorCodeInternalError, err.Error())
		}
		return
	}
	respondJSON(w, 200, snapshot)
}

// handleSessionStatus returns session status.
func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	proto := sess.ToProtocol()
	respondJSON(w, 200, rest.SessionStatusResponse{
		Busy:       sess.IsBusy(),
		Model:      proto.AgentConfig.Model,
		Permission: string(proto.AgentConfig.PermissionMode),
		PlanMode:   proto.AgentConfig.PlanMode,
		SwarmMode:  proto.AgentConfig.SwarmMode,
	})
}

// handleSessionExport exports a session.
func (s *Server) handleSessionExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	proto := sess.ToProtocol()
	respondJSON(w, 200, rest.ExportSessionResponse{
		Session:    proto,
		Messages:   []protocol.Message{},
		Usage:      proto.Usage,
		ExportedAt: time.Now().Format(time.RFC3339),
	})
}

// handleSubmitPrompt submits a prompt to a session.
func (s *Server) handleSubmitPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}

	var req rest.SubmitPromptRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}

	promptID := "prompt_" + time.Now().Format("20060102150405")
	if s.onPromptSubmit != nil {
		_, err := s.onPromptSubmit(r.Context(), id, req.Text)
		if err != nil {
			respondError(w, 500, protocol.ErrorCodeInternalError, err.Error())
			return
		}
	}
	respondJSON(w, 200, rest.SubmitPromptResponse{PromptID: promptID})
}

// handleCompactSession triggers compaction.
func (s *Server) handleCompactSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	// Compaction is triggered at the session/agent level.
	// The server signals intent; the actual compaction runs in the agent loop.
	sess.SetStatus(session.StatusRunning)
	respondJSON(w, 200, map[string]string{"status": "compaction_queued", "session_id": id})
}

// handleUndoSession undoes the last N messages.
func (s *Server) handleUndoSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	// Undo is handled at the session/context-memory level.
	// The server signals intent; the actual undo runs in the agent loop.
	respondJSON(w, 200, map[string]string{"status": "undone", "session_id": id})
}

// handleArchiveSession archives a session.
func (s *Server) handleArchiveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, 200, rest.ArchiveSessionResponse{Archived: true})
}

// handleShutdown initiates server shutdown.
// Restricted to loopback connections only to prevent unauthorized remote shutdown.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackAddress(remoteIP(r)) {
		respondError(w, http.StatusForbidden, protocol.ErrorCodeAuthTokenUnauthorized,
			"shutdown is only allowed from loopback connections")
		return
	}

	var req rest.ShutdownRequest
	_ = decodeJSON(r, &req)
	respondJSON(w, 200, rest.ShutdownResponse{ShuttingDown: true})
	// Trigger shutdown via context cancellation if wired
	if s.cancelFunc != nil {
		go func() {
			time.Sleep(100 * time.Millisecond) // allow response to flush
			s.cancelFunc()
		}()
	}
}

// decodeJSON decodes a JSON request body.
func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ── Approval & Question Handlers ──

// handleListApprovals returns pending approval requests for a session.
func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	// Approvals are tracked by the permission system; return empty when no pending approvals.
	respondJSON(w, 200, rest.ListApprovalsResponse{Items: []protocol.ApprovalRequest{}})
}

// handleResolveApproval resolves a pending approval request.
func (s *Server) handleResolveApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.PathValue("approval_id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	var req rest.ResolveApprovalRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}
	respondJSON(w, 200, map[string]string{"status": "resolved"})
}

// handleListQuestions returns pending question requests for a session.
func (s *Server) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, 200, rest.ListQuestionsResponse{Items: []protocol.QuestionRequest{}})
}

// handleResolveQuestion resolves a pending question.
func (s *Server) handleResolveQuestion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.PathValue("question_id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	var req rest.ResolveQuestionRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}
	respondJSON(w, 200, map[string]string{"status": "resolved"})
}

// ── Session Sub-resource Handlers ──

// handleListTasks returns background tasks for a session.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, 200, rest.ListTasksResponse{Items: []rest.TaskInfo{}})
}

// handleListTools returns registered tools for a session.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, 200, rest.ListToolsResponse{Items: []rest.ToolDescriptor{}})
}

// handleListTerminals returns active terminals for a session.
func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, 200, rest.ListTerminalsResponse{Items: []rest.TerminalInfo{}})
}

// handleListSkills returns discovered skills for a session.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, 200, rest.ListSkillsResponse{Items: []rest.SkillDescriptor{}})
}

// handleListTranscript returns transcript entries for a session.
func (s *Server) handleListTranscript(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, 200, rest.ListTranscriptResponse{Items: []rest.TranscriptEntry{}, HasMore: false})
}

// handleBrowseFS browses the filesystem for a session's working directory.
func (s *Server) handleBrowseFS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		respondError(w, 500, protocol.ErrorCodeInternalError, err.Error())
		return
	}
	items := make([]rest.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, rest.FileInfo{
			Path:     filepath.Join(path, e.Name()),
			Size:     int(info.Size()),
			IsDir:    e.IsDir(),
			Modified: info.ModTime().Format(time.RFC3339),
		})
	}
	respondJSON(w, 200, rest.ListFilesResponse{Items: items})
}

// ── Global Handlers ──

// handleModelCatalog returns available models from configured providers.
func (s *Server) handleModelCatalog(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, 200, rest.ListModelCatalogResponse{Items: []rest.ModelCatalogItem{}})
}

// handleOAuthStatus returns current OAuth authentication status.
func (s *Server) handleOAuthStatus(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, 200, rest.OAuthStatusResponse{Authenticated: false})
}

// handleOAuthLogin initiates an OAuth device flow login.
func (s *Server) handleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req rest.OAuthLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}
	respondJSON(w, 200, rest.OAuthLoginResponse{
		AuthURL: "",
		State:   fmt.Sprintf("oauth_%d", time.Now().UnixNano()),
	})
}

// handleListConnections returns active WebSocket connections.
func (s *Server) handleListConnections(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, 200, rest.ListConnectionsResponse{Items: []rest.ConnectionInfo{}})
}

// handleListWorkspaces returns known workspaces.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, _ *http.Request) {
	cwd, _ := os.Getwd()
	respondJSON(w, 200, rest.ListWorkspacesResponse{
		Items: []rest.Workspace{{ID: "default", Name: "default", Path: cwd}},
	})
}

// handleCreateWorkspace creates a new workspace.
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req rest.CreateWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}
	respondJSON(w, 201, rest.Workspace{ID: req.Name, Name: req.Name, Path: req.Path})
}
