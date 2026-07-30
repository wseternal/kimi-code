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
)

// Server is the HTTP + WebSocket server.
type Server struct {
	httpServer     *http.Server
	sessionManager *session.Manager
	logger         *slog.Logger
	host           string
	port           int
	mux            *http.ServeMux
}

// Config holds server configuration.
type Config struct {
	Host string
	Port int
}

// NewServer creates a new server.
func NewServer(cfg Config, sessionMgr *session.Manager, logger *slog.Logger) *Server {
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
	// Messages not yet wired — return empty
	respondJSON(w, http.StatusOK, []protocol.Message{})
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
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

	// TODO: wire to agent loop
	respondJSON(w, http.StatusCreated, map[string]string{"status": "queued"})
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
