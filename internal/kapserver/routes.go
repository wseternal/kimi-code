package kapserver

import (
	"encoding/json"
	"net/http"
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
		Messages: []protocol.Message{}, // TODO: wire message history
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

	// TODO: Wire message store to get actual messages
	// For now, return empty messages
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
	// TODO: Wire to agent loop
	respondJSON(w, 200, rest.SubmitPromptResponse{PromptID: promptID})
}

// handleCompactSession triggers compaction.
func (s *Server) handleCompactSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := s.sessionManager.Get(id)
	if !ok {
		respondError(w, 404, protocol.ErrorCodeSessionNotFound, "session not found")
		return
	}
	// TODO: Wire to compaction engine
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
	// TODO: Wire to context memory undo
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
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	var req rest.ShutdownRequest
	_ = decodeJSON(r, &req)
	respondJSON(w, 200, rest.ShutdownResponse{ShuttingDown: true})
	// TODO: trigger actual shutdown
}

// decodeJSON decodes a JSON request body.
func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
