// Package profile implements agent profile loading and application.
// Profiles customize agent behavior via Markdown or TOML files.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
)

// AgentProfile defines an agent's behavioral configuration.
type AgentProfile struct {
	Name               string   `toml:"name"`
	SystemPrompt       string   `toml:"system_prompt,omitempty"`
	Model              string   `toml:"model,omitempty"`
	SecondaryModel     string   `toml:"secondary_model,omitempty"`
	ToolAllowList      []string `toml:"tool_allow,omitempty"`
	ToolDenyList       []string `toml:"tool_deny,omitempty"`
	PermissionMode     string   `toml:"permission_mode,omitempty"`
	PlanMode           bool     `toml:"plan_mode,omitempty"`
	CustomInstructions string   `toml:"custom_instructions,omitempty"`
	MaxStepsPerTurn    int      `toml:"max_steps_per_turn,omitempty"`
}

// Load loads an agent profile from a file path.
// Supports TOML format (.toml) and Markdown format (.md).
func Load(path string) (*AgentProfile, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		return loadTOML(path)
	case ".md", ".markdown":
		return loadMarkdown(path)
	default:
		// Try TOML first, fall back to Markdown
		if p, err := loadTOML(path); err == nil {
			return p, nil
		}
		return loadMarkdown(path)
	}
}

// loadTOML loads a profile from a TOML file.
func loadTOML(path string) (*AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}
	var p AgentProfile
	if _, err := toml.Decode(string(data), &p); err != nil {
		return nil, fmt.Errorf("parse profile TOML %s: %w", path, err)
	}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return &p, nil
}

// loadMarkdown loads a profile from a Markdown file.
// The frontmatter (YAML between --- markers) is parsed for structured fields.
// The body becomes the system prompt / custom instructions.
func loadMarkdown(path string) (*AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}

	content := string(data)
	p := &AgentProfile{
		Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
	}

	// Check for YAML frontmatter
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			frontmatter := strings.TrimSpace(parts[1])
			body := strings.TrimSpace(parts[2])

			// Simple YAML-like parsing for known fields
			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				// Skip comments and empty lines
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.Index(line, ":"); idx > 0 {
					key := strings.TrimSpace(line[:idx])
					value := strings.TrimSpace(line[idx+1:])
					// Strip inline comments
					if ci := strings.Index(value, " #"); ci > 0 {
						value = strings.TrimSpace(value[:ci])
					}
					value = strings.Trim(value, "\"'")
					switch key {
					case "name":
						p.Name = value
					case "model":
						p.Model = value
					case "secondary_model":
						p.SecondaryModel = value
					case "permission_mode":
						p.PermissionMode = value
					case "plan_mode":
						p.PlanMode = value == "true"
					case "max_steps_per_turn":
						fmt.Sscanf(value, "%d", &p.MaxStepsPerTurn)
					}
				}
			}

			if body != "" {
				p.CustomInstructions = body
			}
			return p, nil
		}
	}

	// No frontmatter — entire content is the system prompt
	p.SystemPrompt = content
	return p, nil
}

// LoadNamed loads a named agent profile from standard directories.
// Searches: ~/.gkimi-code/agents/<name>.toml, then .md
func LoadNamed(name string, homeDir string) (*AgentProfile, error) {
	agentDir := filepath.Join(homeDir, config.DataDirName, "agents")

	// Try TOML first
	tomlPath := filepath.Join(agentDir, name+".toml")
	if p, err := Load(tomlPath); err == nil {
		return p, nil
	}

	// Try Markdown
	mdPath := filepath.Join(agentDir, name+".md")
	if p, err := Load(mdPath); err == nil {
		return p, nil
	}

	return nil, fmt.Errorf("agent profile %q not found in %s", name, agentDir)
}

// ApplyToSystemPrompt injects the profile's instructions into the system prompt.
// Returns the modified system prompt.
func (p *AgentProfile) ApplyToSystemPrompt(basePrompt string) string {
	var parts []string

	if p.SystemPrompt != "" {
		parts = append(parts, p.SystemPrompt)
	}

	if basePrompt != "" {
		parts = append(parts, basePrompt)
	}

	if p.CustomInstructions != "" {
		parts = append(parts, "## Custom Instructions\n\n"+p.CustomInstructions)
	}

	return strings.Join(parts, "\n\n")
}
