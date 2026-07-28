package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// stubTool is a minimal Tool implementation for testing.
type stubTool struct {
	name   string
	output string
}

func (s *stubTool) Definition() Definition {
	return Definition{Name: s.name, Description: "stub: " + s.name}
}

func (s *stubTool) Execute(_ context.Context, _ json.RawMessage, _ ExecContext) (*Result, error) {
	return &Result{Output: s.output}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	tool := &stubTool{name: "Foo", output: "bar"}
	reg.Register(tool)

	got, ok := reg.Get("Foo")
	if !ok {
		t.Fatal("Get(Foo) returned false, want true")
	}
	if got.Definition().Name != "Foo" {
		t.Errorf("Get(Foo).Definition().Name = %q, want %q", got.Definition().Name, "Foo")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.Get("NonExistent")
	if ok {
		t.Error("Get(NonExistent) returned true, want false")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "A"})
	reg.Register(&stubTool{name: "B"})
	reg.Register(&stubTool{name: "C"})

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("List() returned %d tools, want 3", len(list))
	}

	names := map[string]bool{}
	for _, tool := range list {
		names[tool.Definition().Name] = true
	}
	for _, want := range []string{"A", "B", "C"} {
		if !names[want] {
			t.Errorf("List() missing tool %q", want)
		}
	}
}

func TestRegistry_Definitions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "X"})
	reg.Register(&stubTool{name: "Y"})

	defs := reg.Definitions()
	if len(defs) != 2 {
		t.Fatalf("Definitions() returned %d defs, want 2", len(defs))
	}
	for _, d := range defs {
		if d.Name != "X" && d.Name != "Y" {
			t.Errorf("unexpected definition name %q", d.Name)
		}
	}
}

func TestRegistry_RegisterOverwrite(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "T", output: "first"})
	reg.Register(&stubTool{name: "T", output: "second"})

	got, ok := reg.Get("T")
	if !ok {
		t.Fatal("Get(T) returned false after overwrite")
	}
	result, _ := got.Execute(context.Background(), nil, ExecContext{})
	if result.Output != "second" {
		t.Errorf("after overwrite, output = %q, want %q", result.Output, "second")
	}
}

// --- Hook tests ---

// testHook is a ToolHook that returns a wrapped tool.
type testHook struct {
	name    string
	wrapped Tool // if non-nil, Wrap returns this; if nil, returns nil (passthrough)
}

func (h *testHook) Name() string { return h.name }
func (h *testHook) Wrap(_ string, _ Tool) Tool {
	return h.wrapped
}

func TestRegistry_HookWrapsTool(t *testing.T) {
	reg := NewRegistry()
	original := &stubTool{name: "Grep", output: "original"}
	replacement := &stubTool{name: "Grep", output: "wrapped"}
	reg.Register(original)
	reg.RegisterHook("Grep", &testHook{name: "enhancer", wrapped: replacement})

	got, ok := reg.Get("Grep")
	if !ok {
		t.Fatal("Get(Grep) returned false")
	}
	result, _ := got.Execute(context.Background(), nil, ExecContext{})
	if result.Output != "wrapped" {
		t.Errorf("output = %q, want %q", result.Output, "wrapped")
	}
}

func TestRegistry_HookNilPassthrough(t *testing.T) {
	reg := NewRegistry()
	original := &stubTool{name: "Grep", output: "original"}
	reg.Register(original)
	reg.RegisterHook("Grep", &testHook{name: "noop", wrapped: nil})

	got, ok := reg.Get("Grep")
	if !ok {
		t.Fatal("Get(Grep) returned false")
	}
	result, _ := got.Execute(context.Background(), nil, ExecContext{})
	if result.Output != "original" {
		t.Errorf("output = %q, want %q", result.Output, "original")
	}
}

func TestRegistry_MultipleHooksChain(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "Grep", output: "base"})

	// First hook wraps with "first"
	reg.RegisterHook("Grep", &testHook{name: "first", wrapped: &stubTool{name: "Grep", output: "first"}})
	// Second hook wraps with "second" (applied to the result of first hook)
	reg.RegisterHook("Grep", &testHook{name: "second", wrapped: &stubTool{name: "Grep", output: "second"}})

	got, ok := reg.Get("Grep")
	if !ok {
		t.Fatal("Get(Grep) returned false")
	}
	result, _ := got.Execute(context.Background(), nil, ExecContext{})
	// Second hook wins (last in chain)
	if result.Output != "second" {
		t.Errorf("output = %q, want %q", result.Output, "second")
	}
}

func TestRegistry_NoHooksFastPath(t *testing.T) {
	reg := NewRegistry()
	original := &stubTool{name: "Bash", output: "original"}
	reg.Register(original)

	// No hooks registered for "Bash"
	got, ok := reg.Get("Bash")
	if !ok {
		t.Fatal("Get(Bash) returned false")
	}
	result, _ := got.Execute(context.Background(), nil, ExecContext{})
	if result.Output != "original" {
		t.Errorf("output = %q, want %q", result.Output, "original")
	}

	// Hook registered for different tool should not affect Bash
	reg.RegisterHook("Grep", &testHook{name: "other", wrapped: &stubTool{name: "Grep", output: "other"}})
	got2, ok := reg.Get("Bash")
	if !ok {
		t.Fatal("Get(Bash) returned false after unrelated hook")
	}
	result2, _ := got2.Execute(context.Background(), nil, ExecContext{})
	if result2.Output != "original" {
		t.Errorf("after unrelated hook, output = %q, want %q", result2.Output, "original")
	}
}
