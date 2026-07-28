package di

import (
	"testing"
)

func TestNewAppScope(t *testing.T) {
	scope := NewAppScope("test-app")
	if scope.ID() != "test-app" {
		t.Fatalf("expected id 'test-app', got %q", scope.ID())
	}
	if scope.Kind() != ScopeApp {
		t.Fatalf("expected kind App, got %d", scope.Kind())
	}
}

func TestScope_RegisterResolve(t *testing.T) {
	scope := NewAppScope("app")
	scope.Register("config", "my-config")

	val, err := scope.Resolve("config")
	if err != nil {
		t.Fatal(err)
	}
	if val != "my-config" {
		t.Fatalf("expected 'my-config', got %v", val)
	}
}

func TestScope_ResolveNotFound(t *testing.T) {
	scope := NewAppScope("app")
	_, err := scope.Resolve("missing")
	if err == nil {
		t.Fatal("expected error for missing service")
	}
}

func TestScope_CreateChild(t *testing.T) {
	app := NewAppScope("app")
	app.Register("global", "global-value")

	session, err := app.CreateChild(ScopeSession, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if session.Kind() != ScopeSession {
		t.Fatalf("expected Session kind, got %d", session.Kind())
	}

	// Child can resolve parent services
	val, err := session.Resolve("global")
	if err != nil {
		t.Fatal(err)
	}
	if val != "global-value" {
		t.Fatalf("expected 'global-value', got %v", val)
	}

	// Child can register its own services
	session.Register("local", "session-value")
	val, err = session.Resolve("local")
	if err != nil {
		t.Fatal(err)
	}
	if val != "session-value" {
		t.Fatalf("expected 'session-value', got %v", val)
	}
}

func TestScope_CreateChild_InvalidKind(t *testing.T) {
	app := NewAppScope("app")
	_, err := app.CreateChild(ScopeApp, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid scope kind")
	}
}

func TestScope_CreateChild_DuplicateID(t *testing.T) {
	app := NewAppScope("app")
	_, err := app.CreateChild(ScopeSession, "s1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.CreateChild(ScopeSession, "s1")
	if err == nil {
		t.Fatal("expected error for duplicate child id")
	}
}

func TestScope_Dispose(t *testing.T) {
	app := NewAppScope("app")
	s1, _ := app.CreateChild(ScopeSession, "s1")
	s1.CreateChild(ScopeAgent, "a1")

	app.Dispose()

	// After dispose, creating children should fail
	_, err := app.CreateChild(ScopeSession, "s2")
	if err == nil {
		t.Fatal("expected error after dispose")
	}
}

func TestScope_Grandchild(t *testing.T) {
	app := NewAppScope("app")
	session, _ := app.CreateChild(ScopeSession, "s1")
	agent, err := session.CreateChild(ScopeAgent, "a1")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Kind() != ScopeAgent {
		t.Fatalf("expected Agent kind, got %d", agent.Kind())
	}

	// Agent can resolve app-level services
	app.Register("global", "global-value")
	val, err := agent.Resolve("global")
	if err != nil {
		t.Fatal(err)
	}
	if val != "global-value" {
		t.Fatalf("expected 'global-value', got %v", val)
	}
}

func TestServicesAccessor(t *testing.T) {
	scope := NewAppScope("app")
	scope.Register("key", "value")
	accessor := NewServicesAccessor(scope)

	val, err := accessor.Get("key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value" {
		t.Fatalf("expected 'value', got %v", val)
	}
}
