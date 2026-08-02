package kapserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/kapserver/transport"
	"github.com/visdomtech/kimi-code/internal/protocol"
	"github.com/visdomtech/kimi-code/internal/protocol/rest"
)

// sessionSeq is an atomic counter for unique session IDs.
var sessionSeq atomic.Uint64

// ConfigProvider provides configuration data for server endpoints.
type ConfigProvider interface {
	// ListModels returns all configured model entries.
	ListModels() []rest.ModelCatalogItem
	// ListProviders returns all configured provider names.
	ListProviders() []string
	// PermissionMode returns the current permission mode.
	PermissionMode() string
}

// PromptSubmitFunc is called when a prompt is submitted via REST.
// Returns a prompt ID and optional error.
type PromptSubmitFunc func(ctx context.Context, sessionID, prompt string) (string, error)

// CompactFunc triggers context compaction for a session.
type CompactFunc func(ctx context.Context, sessionID string) error

// UndoFunc undoes the last N messages in a session.
type UndoFunc func(ctx context.Context, sessionID string, n int) error

// MessageListFunc returns messages for a session.
type MessageListFunc func(ctx context.Context, sessionID string) ([]protocol.Message, error)

// OAuthLoginFunc triggers the OAuth device flow login.
type OAuthLoginFunc func(ctx context.Context) (string, error)

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
	workDir        string
	mux            *http.ServeMux

	// Optional callbacks wired by the caller.
	// When nil, the corresponding route returns an appropriate status
	// without executing agent actions.
	onPromptSubmit  PromptSubmitFunc
	onCompact       CompactFunc
	onUndo          UndoFunc
	onListMessages  MessageListFunc
	onOAuthLogin    OAuthLoginFunc
	onListTranscript TranscriptListFunc
	sessionData     SessionDataProvider
	cancelFunc      context.CancelFunc // for shutdown
	tokenStore      *TokenStore
	securityConfig  SecurityConfig
	security        *SecurityMiddleware
	snapshotSvc     *SnapshotService
	searchSvc       *SearchService

	// WebSocket transport
	wsRegistry    *transport.Registry
	broadcaster   *transport.Broadcaster

	// Config provider for model-catalog and config endpoints
	configProvider ConfigProvider
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

// WithCompact wires a compaction handler.
func WithCompact(fn CompactFunc) ServerOption {
	return func(s *Server) { s.onCompact = fn }
}

// WithUndo wires an undo handler.
func WithUndo(fn UndoFunc) ServerOption {
	return func(s *Server) { s.onUndo = fn }
}

// WithMessageList wires a message listing handler.
func WithMessageList(fn MessageListFunc) ServerOption {
	return func(s *Server) { s.onListMessages = fn }
}

// WithOAuthLogin wires an OAuth login handler.
func WithOAuthLogin(fn OAuthLoginFunc) ServerOption {
	return func(s *Server) { s.onOAuthLogin = fn }
}

// WithTranscriptList wires a transcript listing handler.
func WithTranscriptList(fn TranscriptListFunc) ServerOption {
	return func(s *Server) { s.onListTranscript = fn }
}

// WithCancelFunc provides a context cancel function for shutdown.
func WithCancelFunc(fn context.CancelFunc) ServerOption {
	return func(s *Server) { s.cancelFunc = fn }
}

// WithSessionDataProvider wires a session data provider for session-scoped routes.
func WithSessionDataProvider(p SessionDataProvider) ServerOption {
	return func(s *Server) { s.sessionData = p }
}

// WithTokenStore wires an auth token store for bearer authentication.
func WithTokenStore(ts *TokenStore) ServerOption {
	return func(s *Server) { s.tokenStore = ts }
}

// WithSecurityConfig configures the security middleware (CORS, rate limiting).
func WithSecurityConfig(cfg SecurityConfig) ServerOption {
	return func(s *Server) { s.securityConfig = cfg }
}

// WithWorkDir sets the server's working directory for file operations.
func WithWorkDir(dir string) ServerOption {
	return func(s *Server) { s.workDir = dir }
}

// WithConfigProvider wires a config provider for model-catalog and config endpoints.
func WithConfigProvider(cp ConfigProvider) ServerOption {
	return func(s *Server) { s.configProvider = cp }
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

	// Default workDir to current directory if not set.
	if s.workDir == "" {
		if wd, err := os.Getwd(); err == nil {
			s.workDir = wd
		}
	}

	// Default security config: bind address for loopback detection.
	if s.securityConfig.BindAddress == "" {
		s.securityConfig.BindAddress = cfg.Host
	}

	// Cache services to avoid per-request allocation (S2).
	s.snapshotSvc = NewSnapshotService(sessionMgr)
	s.searchSvc = NewSearchService(s.workDir)

	// Initialize WebSocket transport
	s.wsRegistry = transport.NewRegistry(logger)
	s.broadcaster = transport.NewBroadcaster(s.wsRegistry, 1000, logger)

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

	// WebSocket route
	s.mux.HandleFunc("GET /api/v1/ws", transport.HandleWebSocket(s.wsRegistry, s.broadcaster, s.logger))
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
	if s.security != nil {
		s.security.Stop()
	}
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	// Apply security middleware (CORS origin validation, host checks, rate limiting).
	sec := NewSecurityMiddleware(s.securityConfig)
	s.security = sec
	handler := sec.Wrap(next)

	// Apply bearer auth middleware when a token store is configured.
	if s.tokenStore != nil {
		skipPaths := []string{
			"/api/v1/health",
			"/api/v1/meta",
			"/api/v1/oauth/login",
			"/api/v1/oauth/status",
		}
		auth := BearerAuthMiddleware(s.tokenStore, skipPaths)
		handler = auth(handler)
	}

	// Wrap with request ID and content-type.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

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

		// S8 fix: access logging middleware.
		start := time.Now()
		rw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler.ServeHTTP(rw, r)
		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
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

// statusResponseWriter captures the HTTP status code for access logging (S8 fix).
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *statusResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.statusCode = code
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
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

	id := fmt.Sprintf("sess_%d_%d", time.Now().UnixMilli(), sessionSeq.Add(1))
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
	if s.onListMessages != nil {
		msgs, err := s.onListMessages(r.Context(), id)
		if err != nil {
			respondError(w, http.StatusInternalServerError, protocol.ErrorCodeInternalError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, msgs)
		return
	}
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
