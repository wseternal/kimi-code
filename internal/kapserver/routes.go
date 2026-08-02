package kapserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
// S5 fix: removed unused messageLimit parameter; message fetching should use the transcript store.
func (s *SnapshotService) CaptureWithMessages(sessionID string) (*rest.SessionSnapshot, error) {
	snapshot, err := s.Capture(sessionID)
	if err != nil {
		return nil, err
	}
	// Messages come from the transcript store, not session memory.
	return snapshot, nil
}

// ── Additional REST Route Handlers ──

// handleConfig returns the current agent configuration.
// TODO(W18): Wire Config to return real permission mode from config.
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

	results, err := s.searchSvc.Search(r.Context(), SearchQuery{
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
	snapshot, err := s.snapshotSvc.Capture(id)
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
// W5 fix: return 501 until actually wired to the agent loop.
func (s *Server) handleCompactSession(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	respondError(w, http.StatusNotImplemented, protocol.ErrorCodeInternalError, "compaction not yet implemented")
}

// handleUndoSession undoes the last N messages.
// W5 fix: return 501 until actually wired to the agent loop.
func (s *Server) handleUndoSession(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	respondError(w, http.StatusNotImplemented, protocol.ErrorCodeInternalError, "undo not yet implemented")
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

// requireSession validates that the session exists and returns its ID.
// Returns ("", false) and writes a 404 error if the session is not found.
func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if _, ok := s.sessionManager.Get(id); !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return "", false
	}
	return id, true
}

// requireSessionAndBody validates the session and decodes the JSON request body.
// Returns ("", zero-value, false) and writes an error on failure.
func requireSessionAndBody[T any](s *Server, w http.ResponseWriter, r *http.Request) (string, T, bool) {
	id, ok := s.requireSession(w, r)
	if !ok {
		var zero T
		return "", zero, false
	}
	var req T
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		var zero T
		return "", zero, false
	}
	return id, req, true
}

// handleListApprovals returns pending approval requests for a session.
func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var items []protocol.ApprovalRequest
	if s.sessionData != nil {
		items = s.sessionData.ListApprovals(id)
	}
	if items == nil {
		items = []protocol.ApprovalRequest{}
	}
	respondJSON(w, 200, rest.ListApprovalsResponse{Items: items})
}

// handleResolveApproval resolves a pending approval request.
func (s *Server) handleResolveApproval(w http.ResponseWriter, r *http.Request) {
	id, req, ok := requireSessionAndBody[rest.ResolveApprovalRequest](s, w, r)
	if !ok {
		return
	}
	approvalID := r.PathValue("approval_id")
	if s.sessionData != nil {
		if err := s.sessionData.ResolveApproval(id, approvalID, req); err != nil {
			respondError(w, 404, protocol.ErrorCodeApprovalNotFound, err.Error())
			return
		}
	} else {
		respondError(w, 501, protocol.ErrorCodeInternalError, "approval resolution not configured")
		return
	}
	respondJSON(w, 200, map[string]string{"status": "resolved"})
}

// handleListQuestions returns pending question requests for a session.
func (s *Server) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var items []protocol.QuestionRequest
	if s.sessionData != nil {
		items = s.sessionData.ListQuestions(id)
	}
	if items == nil {
		items = []protocol.QuestionRequest{}
	}
	respondJSON(w, 200, rest.ListQuestionsResponse{Items: items})
}

// handleResolveQuestion resolves a pending question.
func (s *Server) handleResolveQuestion(w http.ResponseWriter, r *http.Request) {
	id, req, ok := requireSessionAndBody[rest.ResolveQuestionRequest](s, w, r)
	if !ok {
		return
	}
	questionID := r.PathValue("question_id")
	if s.sessionData != nil {
		if err := s.sessionData.ResolveQuestion(id, questionID, req); err != nil {
			respondError(w, 404, protocol.ErrorCodeQuestionNotFound, err.Error())
			return
		}
	} else {
		respondError(w, 501, protocol.ErrorCodeInternalError, "question resolution not configured")
		return
	}
	respondJSON(w, 200, map[string]string{"status": "resolved"})
}

// ── Session Sub-resource Handlers ──

// handleListTasks returns background tasks for a session.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var items []rest.TaskInfo
	if s.sessionData != nil {
		items = s.sessionData.ListTasks(id)
	}
	if items == nil {
		items = []rest.TaskInfo{}
	}
	respondJSON(w, 200, rest.ListTasksResponse{Items: items})
}

// handleListTools returns registered tools for a session.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var items []rest.ToolDescriptor
	if s.sessionData != nil {
		items = s.sessionData.ListTools(id)
	}
	if items == nil {
		items = []rest.ToolDescriptor{}
	}
	respondJSON(w, 200, rest.ListToolsResponse{Items: items})
}

// handleListTerminals returns active terminals for a session.
func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var items []rest.TerminalInfo
	if s.sessionData != nil {
		items = s.sessionData.ListTerminals(id)
	}
	if items == nil {
		items = []rest.TerminalInfo{}
	}
	respondJSON(w, 200, rest.ListTerminalsResponse{Items: items})
}

// handleListSkills returns discovered skills for a session.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var items []rest.SkillDescriptor
	if s.sessionData != nil {
		items = s.sessionData.ListSkills(id)
	}
	if items == nil {
		items = []rest.SkillDescriptor{}
	}
	respondJSON(w, 200, rest.ListSkillsResponse{Items: items})
}

// handleListTranscript returns transcript entries for a session.
// TODO(W17): Wire transcript.Store into the server to provide real transcript data.
// Currently returns empty until the transcript store is integrated with the agent loop.
func (s *Server) handleListTranscript(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	respondJSON(w, 200, rest.ListTranscriptResponse{Items: []rest.TranscriptEntry{}, HasMore: false})
}

// handleBrowseFS browses the filesystem for a session's working directory.
// Paths are restricted to the server's working directory to prevent traversal.
func (s *Server) handleBrowseFS(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	workdir := s.workDir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	// Resolve the requested path relative to workdir and enforce containment.
	resolved := filepath.Clean(filepath.Join(workdir, path))
	cleanWorkdir := filepath.Clean(workdir)
	if resolved != cleanWorkdir && !strings.HasPrefix(resolved, cleanWorkdir+string(os.PathSeparator)) {
		respondError(w, 403, protocol.ErrorCodeFSPermissionDenied, "path outside working directory")
		return
	}
	// C2b fix: resolve symlinks and re-validate containment.
	resolvedReal, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		respondError(w, 403, protocol.ErrorCodeFSPermissionDenied, "cannot resolve path")
		return
	}
	workdirReal, err := filepath.EvalSymlinks(cleanWorkdir)
	if err != nil {
		respondError(w, 500, protocol.ErrorCodeInternalError, "cannot resolve working directory")
		return
	}
	if resolvedReal != workdirReal && !strings.HasPrefix(resolvedReal, workdirReal+string(os.PathSeparator)) {
		respondError(w, 403, protocol.ErrorCodeFSPermissionDenied, "path resolves outside working directory")
		return
	}
	entries, err := os.ReadDir(resolvedReal)
	if err != nil {
		respondError(w, 500, protocol.ErrorCodeInternalError, err.Error())
		return
	}
	items := make([]rest.FileInfo, 0, len(entries))
	const maxEntries = 1000
	for _, e := range entries {
		if len(items) >= maxEntries {
			break
		}
		// N4 fix: use DirEntry.IsDir() directly (avoids stat for directory check).
		isDir := e.IsDir()
		entryPath := filepath.Join(resolved, e.Name())
		// S6 fix: return paths relative to workdir to avoid leaking server structure.
		relPath, err := filepath.Rel(cleanWorkdir, entryPath)
		if err != nil {
			relPath = entryPath
		}
		fi := rest.FileInfo{
			Path:  relPath,
			IsDir: isDir,
		}
		// Only stat if we need size/modtime (skip for directories).
		if !isDir {
			if info, err := e.Info(); err == nil {
				fi.Size = int(info.Size())
				fi.Modified = info.ModTime().Format(time.RFC3339)
			}
		}
		items = append(items, fi)
	}
	respondJSON(w, 200, rest.ListFilesResponse{Items: items})
}

// ── Global Handlers ──

// handleModelCatalog returns available models from configured providers.
// TODO(W18): Wire provider config to return real model catalog from ListProviderModels.
func (s *Server) handleModelCatalog(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, 200, rest.ListModelCatalogResponse{Items: []rest.ModelCatalogItem{}})
}

// handleOAuthStatus returns current OAuth authentication status.
// TODO(W18): Wire OAuth manager to return real auth status.
func (s *Server) handleOAuthStatus(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, 200, rest.OAuthStatusResponse{Authenticated: false})
}

// handleOAuthLogin initiates an OAuth device flow login.
// W5 fix: return 501 until OAuth manager is wired.
func (s *Server) handleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req rest.OAuthLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}
	respondError(w, http.StatusNotImplemented, protocol.ErrorCodeInternalError, "OAuth login not yet implemented")
}

// handleListConnections returns active WebSocket connections.
// TODO(W18): Wire Registry to return real connection list.
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
// W6 fix: validate path is within server workdir.
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req rest.CreateWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}
	// Containment check: workspace path must be within server workDir.
	workdir := s.workDir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid path")
		return
	}
	absWork, err := filepath.Abs(workdir)
	if err != nil {
		respondError(w, 500, protocol.ErrorCodeInternalError, "cannot resolve workdir")
		return
	}
	if absPath != absWork && !strings.HasPrefix(absPath, absWork+string(filepath.Separator)) {
		respondError(w, 403, protocol.ErrorCodeFSPermissionDenied, "workspace path outside working directory")
		return
	}
	respondJSON(w, 201, rest.Workspace{ID: req.Name, Name: req.Name, Path: req.Path})
}
