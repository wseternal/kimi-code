package klient

import (
	"context"
	"fmt"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
)

// Harness is the in-process engine facade for single-user CLI.
// It provides session CRUD, prompt submission, and approval/question handling
// without needing an HTTP server.
type Harness struct {
	Config         *config.Config
	AppScope       *di.Scope
	SessionManager *session.Manager
	ctx            context.Context
	cancel         context.CancelFunc

	// PromptHandler is called when SubmitPrompt is invoked.
	// When nil, SubmitPrompt is a no-op (caller wires the agent loop).
	PromptHandler func(ctx context.Context, sessionID, prompt string) error
}

// NewHarness creates a new in-process harness.
func NewHarness(cfg *config.Config) *Harness {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	ctx, cancel := context.WithCancel(context.Background())

	appScope := di.NewAppScope("kimi-harness")
	appScope.Register("config", cfg)

	mgr := session.NewManager(appScope)
	appScope.Register("sessionManager", mgr)

	return &Harness{
		Config:         cfg,
		AppScope:       appScope,
		SessionManager: mgr,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// CreateSession creates a new session.
func (h *Harness) CreateSession(title string) (*session.Session, error) {
	id := fmt.Sprintf("sess_%d", time.Now().UnixNano())
	sess, err := h.SessionManager.Create(h.ctx, id, title)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// GetSession returns a session by ID.
func (h *Harness) GetSession(id string) (*session.Session, bool) {
	return h.SessionManager.Get(id)
}

// ListSessions returns all sessions.
func (h *Harness) ListSessions() []*session.Session {
	return h.SessionManager.List()
}

// DeleteSession deletes a session.
func (h *Harness) DeleteSession(id string) {
	h.SessionManager.Delete(id)
}

// SubmitPrompt submits a prompt to a session.
// Delegates to the wired PromptHandler if set; otherwise marks session running.
func (h *Harness) SubmitPrompt(sessionID, prompt string) error {
	sess, ok := h.SessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	if h.PromptHandler != nil {
		return h.PromptHandler(h.ctx, sessionID, prompt)
	}
	sess.SetStatus(session.StatusRunning)
	return nil
}

// Close shuts down the harness.
func (h *Harness) Close() {
	h.cancel()
	h.AppScope.Dispose()
}
