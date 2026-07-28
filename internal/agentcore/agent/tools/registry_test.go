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
