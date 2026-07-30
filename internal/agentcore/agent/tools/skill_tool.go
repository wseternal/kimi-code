package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/skill"
)

// SkillTool allows the model to invoke skills by name.
// The model decides when a skill is needed and calls SkillTool with the skill name.
type SkillTool struct {
	catalog  *skill.Catalog
	handler  SkillHandler
}

// SkillHandler is a callback invoked when a skill is activated via the tool.
// It receives the skill definition and any arguments from the model.
type SkillHandler func(ctx context.Context, sk *skill.Skill, args string) (string, error)

// NewSkillTool creates a SkillTool with the given catalog and handler.
func NewSkillTool(catalog *skill.Catalog, handler SkillHandler) *SkillTool {
	return &SkillTool{catalog: catalog, handler: handler}
}

type skillToolInput struct {
	Name string `json:"name"`
	Args string `json:"args,omitempty"`
}

// Definition returns the tool definition.
func (t *SkillTool) Definition() Definition {
	// Build the skill list for the description
	var skillNames []string
	if t.catalog != nil {
		for _, sk := range t.catalog.List() {
			if !sk.DisableModelInvocation && sk.IsUserActivatable() {
				desc := sk.Description
				if len(desc) > 60 {
					desc = desc[:57] + "..."
				}
				skillNames = append(skillNames, fmt.Sprintf("- %s: %s", sk.Name, desc))
			}
		}
	}

	skillList := ""
	if len(skillNames) > 0 {
		skillList = "\n\nAvailable skills:\n"
		for _, s := range skillNames {
			skillList += s + "\n"
		}
	}

	return Definition{
		Name:        "SkillTool",
		Description: fmt.Sprintf("Invoke a skill by name. Skills provide specialized workflows for specific tasks.%s", skillList),
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The skill name to invoke",
				},
				"args": map[string]interface{}{
					"type":        "string",
					"description": "Optional arguments to pass to the skill",
				},
			},
			"required": []string{"name"},
		},
	}
}

// Execute invokes the named skill.
func (t *SkillTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params skillToolInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	if t.catalog == nil {
		return &Result{Output: "No skill catalog available", IsError: true}, nil
	}

	sk := t.catalog.Get(params.Name)
	if sk == nil {
		return &Result{Output: fmt.Sprintf("Skill not found: %s", params.Name), IsError: true}, nil
	}

	if sk.DisableModelInvocation {
		return &Result{Output: fmt.Sprintf("Skill %s cannot be invoked by the model", params.Name), IsError: true}, nil
	}

	if t.handler == nil {
		// No handler — return the skill body as-is for injection
		return &Result{Output: sk.Body}, nil
	}

	output, err := t.handler(ctx, sk, params.Args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Skill %s failed: %v", params.Name, err), IsError: true}, nil
	}

	return &Result{Output: output}, nil
}

// Catalog returns the underlying skill catalog.
func (t *SkillTool) Catalog() *skill.Catalog {
	return t.catalog
}
