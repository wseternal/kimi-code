package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGoCallersTool_Definition(t *testing.T) {
	tool := NewGoCallersTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoCallers" {
		t.Errorf("Name = %q, want %q", def.Name, "GoCallers")
	}
	if def.Description == "" {
		t.Error("Description should not be empty")
	}
	props := def.Parameters["properties"].(map[string]interface{})
	if _, ok := props["symbol"]; !ok {
		t.Error("missing 'symbol' parameter")
	}
}

func TestGoCalleesTool_Definition(t *testing.T) {
	tool := NewGoCalleesTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoCallees" {
		t.Errorf("Name = %q, want %q", def.Name, "GoCallees")
	}
}

func TestGoContextTool_Definition(t *testing.T) {
	tool := NewGoContextTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoContext" {
		t.Errorf("Name = %q, want %q", def.Name, "GoContext")
	}
}

func TestGoSourceTool_Definition(t *testing.T) {
	tool := NewGoSourceTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoSource" {
		t.Errorf("Name = %q, want %q", def.Name, "GoSource")
	}
}

func TestGoQueryTool_Definition(t *testing.T) {
	tool := NewGoQueryTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoQuery" {
		t.Errorf("Name = %q, want %q", def.Name, "GoQuery")
	}
}

func TestGoSummaryTool_Definition(t *testing.T) {
	tool := NewGoSummaryTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoSummary" {
		t.Errorf("Name = %q, want %q", def.Name, "GoSummary")
	}
}

func TestGoPlanTool_Definition(t *testing.T) {
	tool := NewGoPlanTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoPlan" {
		t.Errorf("Name = %q, want %q", def.Name, "GoPlan")
	}
}

func TestGoImpactTool_Definition(t *testing.T) {
	tool := NewGoImpactTool(NewGoGraphRunner())
	def := tool.Definition()
	if def.Name != "GoImpact" {
		t.Errorf("Name = %q, want %q", def.Name, "GoImpact")
	}
}

func TestGoCallersTool_Execute_GographUnavailable(t *testing.T) {
	// Use a runner with a nonexistent binary to simulate gograph being unavailable
	runner := &GoGraphRunner{binaryPath: "", timeout: gographDefaultTimeout}
	tool := NewGoCallersTool(runner)

	input, _ := json.Marshal(map[string]string{"symbol": "MyFunc"})
	result, err := tool.Execute(context.Background(), input, ExecContext{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when gograph unavailable")
	}
	if result.Output == "" {
		t.Error("expected non-empty error message")
	}
}

func TestGoCalleesTool_Execute_NotGoProject(t *testing.T) {
	runner := NewGoGraphRunner()
	tool := NewGoCalleesTool(runner)

	nonGoDir := t.TempDir()
	goProjectCache.Delete(nonGoDir)

	input, _ := json.Marshal(map[string]string{"symbol": "MyFunc"})
	result, err := tool.Execute(context.Background(), input, ExecContext{WorkDir: nonGoDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Either gograph unavailable or not a Go project — both should return IsError
	if !result.IsError {
		t.Error("expected IsError=true when not in a Go project")
	}
}

func TestRegisterGoGraphTools(t *testing.T) {
	reg := NewRegistry()
	runner := NewGoGraphRunner()
	RegisterGoGraphTools(reg, runner)

	expectedTools := []string{
		"GoCallers", "GoCallees", "GoContext", "GoSource",
		"GoQuery", "GoSummary", "GoPlan", "GoImpact",
	}

	for _, name := range expectedTools {
		tool, ok := reg.Get(name)
		if !ok {
			t.Errorf("tool %q not registered", name)
			continue
		}
		if tool.Definition().Name != name {
			t.Errorf("tool %q has definition name %q", name, tool.Definition().Name)
		}
	}
}

func TestGoQueryTool_Execute_InvalidInput(t *testing.T) {
	runner := NewGoGraphRunner()
	tool := NewGoQueryTool(runner)

	// Invalid JSON
	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`), ExecContext{WorkDir: t.TempDir()})
	if err == nil && result != nil && !result.IsError {
		t.Error("expected error or IsError for invalid input")
	}
}
