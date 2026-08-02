package kapserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/protocol"
	"github.com/visdomtech/kimi-code/internal/protocol/rest"
)

// PromptSubmitFunc is called when a prompt is submitted via REST.
// Returns a prompt ID and optional error.
type PromptSubmitFunc func(ctx context.Context, sessionID, prompt string) (string, error)

// SessionDataProvider provides session-scoped data for route handlers.
// Implementations wire real agent subsystems (permissions, background tasks,
// tool registry, etc.) to the server. Methods return empty slices when
// the subsystem is not available.
type SessionDataProvider interface {
	// ListApprovals returns pending approval requests for a session.
	ListApprovals(sessionID string) []protocol.ApprovalRequest
	// ListQuestions returns pending questions for a session.
	ListQuestions(sessionID string) []protocol.QuestionRequest
	// ListTasks returns background tasks for a session.
	ListTasks(sessionID string) []rest.TaskInfo
	// ListTools returns registered tool descriptors for a session.
	ListTools(sessionID string) []rest.ToolDescriptor
	// ListTerminals returns active terminals for a session.
	ListTerminals(sessionID string) []rest.TerminalInfo
	// ListSkills returns discovered skills for a session.
	ListSkills(sessionID string) []rest.SkillDescriptor
	// ResolveApproval resolves a pending approval. Returns error if not found.
	ResolveApproval(sessionID, approvalID string, response protocol.ApprovalResponse) error
	// ResolveQuestion resolves a pending question. Returns error if not found.
	ResolveQuestion(sessionID, questionID string, response protocol.QuestionResponse) error
}

// Server is the HTTP + WebSocket server.
type Server struct {
	httpServer     *http.Server
	sessionManager *session.Manager
	logger         *slog.Logger
	host           string
	port           int
	mux            *http.ServeMux

	// Optional callbacks wired by the caller.
	// When nil, the corresponding route returns an appropriate status
	// without executing agent actions.
	onPromptSubmit  PromptSubmitFunc
	sessionData     SessionDataProvider
	cancelFunc      context.CancelFunc // for shutdown
}

// Config holds server configuration.
type Config struct {
	Host string
	Port int
}

// ServerOption configures optional server features.
type ServerOption func(*Server)

// WithPromptSubmit wires a prompt submission handler.
func WithPromptSubmit(fn PromptSubmitFunc) ServerOption {
	return func(s *Server) { s.onPromptSubmit = fn }
}

// WithCancelFunc provides a context cancel function for shutdown.
func WithCancelFunc(fn context.CancelFunc) ServerOption {
	return func(s *Server) { s.cancelFunc = fn }
}

// WithSessionDataProvider wires a session data provider for session-scoped routes.
func WithSessionDataProvider(p SessionDataProvider) ServerOption {
	return func(s *Server) { s.sessionData = p }
}

// NewServer creates a new server.
func NewServer(cfg Config, sessionMgr *session.Manager, logger *slog.Logger, opts ...ServerOption) *Server {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 9876
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		sessionManager: sessionMgr,
		logger:         logger,
		host:           cfg.Host,
		port:           cfg.Port,
		mux:            http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(s)
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Middleware
	handler := s.withMiddleware(s.mux)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.host, s.port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// API v1 routes
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/meta", s.handleMeta)
	s.mux.HandleFunc("GET /api/v1/config", s.handleConfig)
	s.mux.HandleFunc("POST /api/v1/shutdown", s.handleShutdown)

	// Session routes
	s.mux.HandleFunc("GET /api/v1/sessions", s.handleListSessions)
	s.mux.HandleFunc("POST /api/v1/sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("PATCH /api/v1/sessions/{id}", s.handleUpdateSession)
	s.mux.HandleFunc("DELETE /api/v1/sessions/{id}", s.handleDeleteSession)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/messages", s.handleListMessages)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/messages", s.handlePostMessage)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/abort", s.handleAbortSession)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/status", s.handleSessionStatus)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/prompts", s.handleSubmitPrompt)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/compact", s.handleCompactSession)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/undo", s.handleUndoSession)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/archive", s.handleArchiveSession)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/snapshot", s.handleSnapshot)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/export", s.handleSessionExport)

	// Approval & question routes
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/approvals", s.handleListApprovals)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/approvals/{approval_id}", s.handleResolveApproval)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/questions", s.handleListQuestions)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/questions/{question_id}", s.handleResolveQuestion)

	// Session sub-resource routes
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/tasks", s.handleListTasks)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/tools", s.handleListTools)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/terminals", s.handleListTerminals)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/skills", s.handleListSkills)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/transcript", s.handleListTranscript)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}/fs", s.handleBrowseFS)

	// Global routes
	s.mux.HandleFunc("GET /api/v1/model-catalog", s.handleModelCatalog)
	s.mux.HandleFunc("GET /api/v1/oauth/status", s.handleOAuthStatus)
	s.mux.HandleFunc("POST /api/v1/oauth/login", s.handleOAuthLogin)
	s.mux.HandleFunc("GET /api/v1/connections", s.handleListConnections)
	s.mux.HandleFunc("GET /api/v1/workspaces", s.handleListWorkspaces)
	s.mux.HandleFunc("POST /api/v1/workspaces", s.handleCreateWorkspace)
}

// Start starts the HTTP server.
func (s *Server) Start(_ context.Context) error {
	s.logger.Info("server starting", "addr", s.httpServer.Addr)
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	return s.httpServer.Serve(ln)
}

// Stop gracefully stops the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
		}
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r)
	})
}

// respondJSON writes a JSON envelope response.
func respondJSON[T any](w http.ResponseWriter, status int, data T) {
	env := protocol.OkEnvelope(data, w.Header().Get("X-Request-ID"))
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(env)
}

// respondError writes a JSON error envelope response.
func respondError(w http.ResponseWriter, status int, code int, msg string) {
	env := protocol.ErrEnvelope(code, msg, w.Header().Get("X-Request-ID"))
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(env)
}

// Route handlers

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"version": "0.1.0",
		"name":    "kimi-code",
	})
}

func (s *Server) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	sessions := s.sessionManager.List()
	result := make([]protocol.Session, 0, len(sessions))
	for _, sess := range sessions {
		result = append(result, sess.ToProtocol())
	}
	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, protocol.ErrorCodeValidationFailed, "invalid request body")
		return
	}

	id := fmt.Sprintf("sess_%d", time.Now().UnixNano())
	sess, err := s.sessionManager.Create(r.Context(), id, req.Title)
	if err != nil {
		respondError(w, http.StatusInternalServerError, protocol.ErrorCodeInternalError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, sess.ToProtocol())
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	respondJSON(w, http.StatusOK, sess.ToProtocol())
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}

	var req struct {
		Title *string `json:"title,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, protocol.ErrorCodeValidationFailed, "invalid request body")
		return
	}

	if req.Title != nil {
		sess.SetTitle(*req.Title)
	}

	respondJSON(w, http.StatusOK, sess.ToProtocol())
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	s.sessionManager.Delete(id)
	respondJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	// Messages are stored in the audit/transcript store.
	// A full message listing requires wiring the transcript reader; return empty for now.
	respondJSON(w, http.StatusOK, []protocol.Message{})
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, protocol.ErrorCodeValidationFailed, "invalid request body")
		return
	}

	if s.onPromptSubmit != nil {
		promptID, err := s.onPromptSubmit(r.Context(), id, req.Content)
		if err != nil {
			respondError(w, http.StatusInternalServerError, protocol.ErrorCodeInternalError, err.Error())
			return
		}
		sess.SetStatus(session.StatusRunning)
		respondJSON(w, http.StatusCreated, map[string]string{"status": "queued", "prompt_id": promptID})
		return
	}
	// No prompt handler wired — accept and mark session busy
	sess.SetStatus(session.StatusRunning)
	respondJSON(w, http.StatusCreated, map[string]string{"status": "accepted", "note": "no agent loop wired"})
}

func (s *Server) handleAbortSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	sess.SetStatus(session.StatusIdle)
	respondJSON(w, http.StatusOK, map[string]string{"status": "aborted"})
}
