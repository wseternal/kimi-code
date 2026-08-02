package kapserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/protocol"
	"github.com/visdomtech/kimi-code/internal/protocol/rest"
)

// promptSeq is an atomic counter for unique prompt IDs.
var promptSeq atomic.Uint64

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

// ── Additional REST Route Handlers ──

// handleConfig returns the current agent configuration.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	permMode := "manual"
	if s.configProvider != nil {
		permMode = s.configProvider.PermissionMode()
	}
	respondJSON(w, 200, rest.ConfigResponse{
		PermissionMode: permMode,
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

	promptID := fmt.Sprintf("prompt_%d_%d", time.Now().UnixMilli(), promptSeq.Add(1))
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
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if s.onCompact != nil {
		if err := s.onCompact(r.Context(), id); err != nil {
			respondError(w, http.StatusInternalServerError, protocol.ErrorCodeInternalError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]string{"status": "compacted"})
		return
	}
	respondError(w, http.StatusNotImplemented, protocol.ErrorCodeInternalError, "compaction not configured")
}

// handleUndoSession undoes the last N messages.
func (s *Server) handleUndoSession(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var req struct {
		N int `json:"n"`
	}
	if err := decodeJSON(r, &req); err != nil || req.N <= 0 {
		req.N = 1 // default: undo last message
	}
	if s.onUndo != nil {
		if err := s.onUndo(r.Context(), id, req.N); err != nil {
			respondError(w, http.StatusInternalServerError, protocol.ErrorCodeInternalError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"status": "undone", "n": req.N})
		return
	}
	respondError(w, http.StatusNotImplemented, protocol.ErrorCodeInternalError, "undo not configured")
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
	// Trigger graceful shutdown after response flushes.
	go func() {
		time.Sleep(100 * time.Millisecond) // allow response to flush
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Stop(shutdownCtx); err != nil {
			s.logger.Error("graceful shutdown failed", "error", err)
		}
		// Fall back to context cancellation if Stop didn't trigger it.
		if s.cancelFunc != nil {
			s.cancelFunc()
		}
	}()
}

// resolveAndValidatePath validates that a requested path stays within the
// working directory after both absolute and symlink resolution.
// Returns the symlink-resolved path and the clean workdir on success.
func resolveAndValidatePath(workdir, path string) (resolvedReal string, cleanWorkdir string, ok bool) {
	absPath := filepath.Clean(filepath.Join(workdir, path))
	cleanWorkdir = filepath.Clean(workdir)
	if absPath != cleanWorkdir && !strings.HasPrefix(absPath, cleanWorkdir+string(os.PathSeparator)) {
		return "", "", false
	}
	resolvedReal, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", "", false
	}
	workdirReal, err := filepath.EvalSymlinks(cleanWorkdir)
	if err != nil {
		return "", "", false
	}
	if resolvedReal != workdirReal && !strings.HasPrefix(resolvedReal, workdirReal+string(os.PathSeparator)) {
		return "", "", false
	}
	return resolvedReal, cleanWorkdir, true
}

// respondSessionList is a generic helper for session sub-resource list endpoints.
// It validates the session, fetches items, wraps them via the wrap function,
// and responds with the typed envelope.
func respondSessionList[T any, R any](w http.ResponseWriter, r *http.Request, s *Server, fetch func(sessionID string) []T, wrap func([]T) R) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var items []T
	if s.sessionData != nil {
		items = fetch(id)
	}
	if items == nil {
		items = []T{}
	}
	respondJSON(w, 200, wrap(items))
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
	respondSessionList(w, r, s,
		func(id string) []protocol.ApprovalRequest { return s.sessionData.ListApprovals(id) },
		func(items []protocol.ApprovalRequest) any { return rest.ListApprovalsResponse{Items: items} })
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
	respondSessionList(w, r, s,
		func(id string) []protocol.QuestionRequest { return s.sessionData.ListQuestions(id) },
		func(items []protocol.QuestionRequest) any { return rest.ListQuestionsResponse{Items: items} })
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
	respondSessionList(w, r, s,
		func(id string) []rest.TaskInfo { return s.sessionData.ListTasks(id) },
		func(items []rest.TaskInfo) any { return rest.ListTasksResponse{Items: items} })
}

// handleListTools returns registered tools for a session.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	respondSessionList(w, r, s,
		func(id string) []rest.ToolDescriptor { return s.sessionData.ListTools(id) },
		func(items []rest.ToolDescriptor) any { return rest.ListToolsResponse{Items: items} })
}

// handleListTerminals returns active terminals for a session.
func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	respondSessionList(w, r, s,
		func(id string) []rest.TerminalInfo { return s.sessionData.ListTerminals(id) },
		func(items []rest.TerminalInfo) any { return rest.ListTerminalsResponse{Items: items} })
}

// handleListSkills returns discovered skills for a session.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	respondSessionList(w, r, s,
		func(id string) []rest.SkillDescriptor { return s.sessionData.ListSkills(id) },
		func(items []rest.SkillDescriptor) any { return rest.ListSkillsResponse{Items: items} })
}

// TranscriptListFunc returns transcript entries for a session.
type TranscriptListFunc func(ctx context.Context, sessionID string) ([]rest.TranscriptEntry, error)

// handleListTranscript returns transcript entries for a session.
func (s *Server) handleListTranscript(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if s.onListTranscript != nil {
		entries, err := s.onListTranscript(r.Context(), r.PathValue("id"))
		if err != nil {
			respondError(w, http.StatusInternalServerError, protocol.ErrorCodeInternalError, err.Error())
			return
		}
		respondJSON(w, 200, rest.ListTranscriptResponse{Items: entries, HasMore: false})
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
	resolvedReal, cleanWorkdir, ok := resolveAndValidatePath(workdir, path)
	if !ok {
		respondError(w, 403, protocol.ErrorCodeFSPermissionDenied, "path outside working directory")
		return
	}
	// Compute display base (non-symlink-resolved) for relative paths.
	resolvedDisplay := filepath.Clean(filepath.Join(workdir, path))
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
		isDir := e.IsDir()
		relPath, err := filepath.Rel(cleanWorkdir, filepath.Join(resolvedDisplay, e.Name()))
		if err != nil {
			continue // skip entries that can't be made relative
		}
		fi := rest.FileInfo{Path: relPath, IsDir: isDir}
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
func (s *Server) handleModelCatalog(w http.ResponseWriter, _ *http.Request) {
	var items []rest.ModelCatalogItem
	if s.configProvider != nil {
		items = s.configProvider.ListModels()
	}
	if items == nil {
		items = []rest.ModelCatalogItem{}
	}
	respondJSON(w, 200, rest.ListModelCatalogResponse{Items: items})
}

// handleOAuthStatus returns current OAuth authentication status.
// TODO(W18): Wire OAuth manager to return real auth status.
func (s *Server) handleOAuthStatus(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, 200, rest.OAuthStatusResponse{Authenticated: false})
}

// handleOAuthLogin initiates an OAuth device flow login.
func (s *Server) handleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.onOAuthLogin != nil {
		verificationURI, err := s.onOAuthLogin(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, protocol.ErrorCodeInternalError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]string{
			"status":           "started",
			"verification_uri": verificationURI,
		})
		return
	}
	respondError(w, http.StatusNotImplemented, protocol.ErrorCodeInternalError, "OAuth login not configured")
}

// handleListConnections returns active WebSocket connections.
func (s *Server) handleListConnections(w http.ResponseWriter, _ *http.Request) {
	items := make([]rest.ConnectionInfo, 0)
	if s.wsRegistry != nil {
		count := s.wsRegistry.Count()
		// Return aggregate count; individual connection details
		// would require Registry to expose a List method.
		if count > 0 {
			items = append(items, rest.ConnectionInfo{
				ID:   fmt.Sprintf("aggregate_%d", count),
				Type: "ws",
			})
		}
	}
	respondJSON(w, 200, rest.ListConnectionsResponse{Items: items})
}

// handleListWorkspaces returns known workspaces.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, _ *http.Request) {
	cwd, _ := os.Getwd()
	respondJSON(w, 200, rest.ListWorkspacesResponse{
		Items: []rest.Workspace{{ID: "default", Name: "default", Path: cwd}},
	})
}

// handleCreateWorkspace creates a new workspace.
// W6 fix: validate path is within server workDir.
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req rest.CreateWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, 400, protocol.ErrorCodeValidationFailed, "invalid request")
		return
	}
	workdir := s.workDir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	if _, _, ok := resolveAndValidatePath(workdir, req.Path); !ok {
		respondError(w, 403, protocol.ErrorCodeFSPermissionDenied, "workspace path outside working directory")
		return
	}
	respondJSON(w, 201, rest.Workspace{ID: req.Name, Name: req.Name, Path: req.Path})
}
