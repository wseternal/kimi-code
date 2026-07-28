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
// TODO: Wire to agent loop.
func (h *Harness) SubmitPrompt(_ string, _ string) error {
	return nil
}

// Close shuts down the harness.
func (h *Harness) Close() {
	h.cancel()
	h.AppScope.Dispose()
}
