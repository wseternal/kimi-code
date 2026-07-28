package kapserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/protocol"
)

func newTestServer() *Server {
	appScope := di.NewAppScope("test")
	mgr := session.NewManager(appScope)
	return NewServer(Config{}, mgr, nil)
}

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var env protocol.Envelope[map[string]string]
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 {
		t.Fatalf("expected code 0, got %d", env.Code)
	}
}

func TestMetaEndpoint(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/meta", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSessionCRUD(t *testing.T) {
	s := newTestServer()

	// Create session
	body := `{"title":"test session"}`
	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var createEnv protocol.Envelope[protocol.Session]
	if err := json.Unmarshal(w.Body.Bytes(), &createEnv); err != nil {
		t.Fatal(err)
	}
	if createEnv.Data == nil {
		t.Fatal("expected session data")
	}
	sessionID := createEnv.Data.ID

	// List sessions
	req = httptest.NewRequest("GET", "/api/v1/sessions", nil)
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Get session
	req = httptest.NewRequest("GET", "/api/v1/sessions/"+sessionID, nil)
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Update session
	body = `{"title":"updated title"}`
	req = httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID, strings.NewReader(body))
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Delete session
	req = httptest.NewRequest("DELETE", "/api/v1/sessions/"+sessionID, nil)
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/api/v1/sessions/"+sessionID, nil)
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
