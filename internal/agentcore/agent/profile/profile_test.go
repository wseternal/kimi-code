package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
)

func TestLoadTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	content := `
name = "coder"
model = "kimi-latest"
system_prompt = "You are a coding assistant."
permission_mode = "yolo"
tool_allow = ["Bash", "Read"]
tool_deny = ["Write"]
plan_mode = true
max_steps_per_turn = 30
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if p.Name != "coder" {
		t.Errorf("Name = %q, want %q", p.Name, "coder")
	}
	if p.Model != "kimi-latest" {
		t.Errorf("Model = %q, want %q", p.Model, "kimi-latest")
	}
	if p.SystemPrompt != "You are a coding assistant." {
		t.Errorf("SystemPrompt = %q", p.SystemPrompt)
	}
	if p.PermissionMode != "yolo" {
		t.Errorf("PermissionMode = %q", p.PermissionMode)
	}
	if len(p.ToolAllowList) != 2 || p.ToolAllowList[0] != "Bash" {
		t.Errorf("ToolAllowList = %v", p.ToolAllowList)
	}
	if !p.PlanMode {
		t.Error("PlanMode should be true")
	}
	if p.MaxStepsPerTurn != 30 {
		t.Errorf("MaxStepsPerTurn = %d, want 30", p.MaxStepsPerTurn)
	}
}

func TestLoadMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assistant.md")
	content := `---
name: my-agent
model: gpt-4
permission_mode: auto
---

You are a helpful assistant that focuses on code review.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if p.Name != "my-agent" {
		t.Errorf("Name = %q, want %q", p.Name, "my-agent")
	}
	if p.Model != "gpt-4" {
		t.Errorf("Model = %q", p.Model)
	}
	if p.CustomInstructions != "You are a helpful assistant that focuses on code review." {
		t.Errorf("CustomInstructions = %q", p.CustomInstructions)
	}
}

func TestLoadMarkdownNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "simple.md")
	content := "You are a simple coding assistant."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if p.Name != "simple" {
		t.Errorf("Name = %q, want %q", p.Name, "simple")
	}
	if p.SystemPrompt != "You are a simple coding assistant." {
		t.Errorf("SystemPrompt = %q", p.SystemPrompt)
	}
}

func TestApplyToSystemPrompt(t *testing.T) {
	p := &AgentProfile{
		SystemPrompt:       "Profile prompt",
		CustomInstructions: "Extra instructions",
	}

	result := p.ApplyToSystemPrompt("Base prompt")
	if result == "" {
		t.Error("result should not be empty")
	}

	// Should contain all three parts
	for _, part := range []string{"Profile prompt", "Base prompt", "Extra instructions"} {
		found := false
		for i := 0; i < len(result); i++ {
			if i+len(part) <= len(result) && result[i:i+len(part)] == part {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("result should contain %q, got %q", part, result)
		}
	}
}

func TestLoadNamed(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, config.DataDirName, "agents")
	os.MkdirAll(agentDir, 0755)

	content := `name = "test-agent"
model = "test-model"`
	os.WriteFile(filepath.Join(agentDir, "test-agent.toml"), []byte(content), 0644)

	p, err := LoadNamed("test-agent", dir)
	if err != nil {
		t.Fatalf("LoadNamed() error: %v", err)
	}
	if p.Name != "test-agent" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Model != "test-model" {
		t.Errorf("Model = %q", p.Model)
	}
}

func TestLoadNamedNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadNamed("nonexistent", dir)
	if err == nil {
		t.Error("expected error for nonexistent profile")
	}
}
