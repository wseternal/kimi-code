package permission

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
)

// ── FileAccessAskPolicy ──

// FileAccessAskPolicy asks for all file-modifying operations (Write, Edit)
// and allows read-only operations. Used in manual/strict modes.
type FileAccessAskPolicy struct{}

func NewFileAccessAskPolicy() *FileAccessAskPolicy { return &FileAccessAskPolicy{} }

func (p *FileAccessAskPolicy) Name() string { return "file-access-ask" }

var fileModifyTools = map[string]bool{
	"Write": true,
	"Edit":  true,
}

func (p *FileAccessAskPolicy) Evaluate(toolName string, _ json.RawMessage) Result {
	if fileModifyTools[toolName] {
		return Result{Decision: DecisionAsk, Reason: "file modification requires approval", Policy: p.Name()}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}

// ── PlanModeGuardPolicy ──

// PlanModeGuardPolicy denies all tools that modify state when in plan mode.
// Only read-only / analysis tools are allowed.
type PlanModeGuardPolicy struct{}

func NewPlanModeGuardPolicy() *PlanModeGuardPolicy { return &PlanModeGuardPolicy{} }

func (p *PlanModeGuardPolicy) Name() string { return "plan-mode-guard" }

// planModeAllowedTools are tools permitted in plan mode.
var planModeAllowedTools = map[string]bool{
	"Read":        true,
	"Glob":        true,
	"Grep":        true,
	"FetchURL":    true,
	"WebSearch":   true,
	"ReadMedia":   true,
	"ExitPlanMode": true,
	"update_plan": true,
}

func (p *PlanModeGuardPolicy) Evaluate(toolName string, _ json.RawMessage) Result {
	if planModeAllowedTools[toolName] {
		return Result{Decision: DecisionAllow, Reason: "allowed in plan mode", Policy: p.Name()}
	}
	return Result{
		Decision: DecisionDeny,
		Reason:   "tool " + toolName + " is not allowed in plan mode",
		Policy:   p.Name(),
	}
}

// ── UserConfiguredRulesPolicy ──

// UserRule is a single user-configured allow/deny rule.
type UserRule struct {
	Tool    string `json:"tool"`    // tool name or "*" for all
	Path    string `json:"path"`    // optional path glob for file tools
	Allow   bool   `json:"allow"`   // true = allow, false = deny
}

// UserConfiguredRulesPolicy evaluates user-defined allow/deny rules.
type UserConfiguredRulesPolicy struct {
	mu    sync.RWMutex
	rules []UserRule
}

func NewUserConfiguredRulesPolicy(rules []UserRule) *UserConfiguredRulesPolicy {
	return &UserConfiguredRulesPolicy{rules: rules}
}

func (p *UserConfiguredRulesPolicy) Name() string { return "user-configured-rules" }

func (p *UserConfiguredRulesPolicy) SetRules(rules []UserRule) {
	p.mu.Lock()
	p.rules = rules
	p.mu.Unlock()
}

func (p *UserConfiguredRulesPolicy) Evaluate(toolName string, input json.RawMessage) Result {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, rule := range p.rules {
		if rule.Tool != "*" && rule.Tool != toolName {
			continue
		}
		// If rule has a path glob, check it against file tools
		if rule.Path != "" {
			path := extractPath(input)
			if path == "" {
				continue
			}
			matched, err := filepath.Match(rule.Path, filepath.Base(path))
			if err != nil || !matched {
				// Try full path match
				matched, _ = filepath.Match(rule.Path, path)
			}
			if !matched {
				continue
			}
		}
		if rule.Allow {
			return Result{Decision: DecisionAllow, Reason: "user rule: allow " + rule.Tool, Policy: p.Name()}
		}
		return Result{Decision: DecisionDeny, Reason: "user rule: deny " + rule.Tool, Policy: p.Name()}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}

// extractPath extracts the "path" field from tool input JSON.
func extractPath(input json.RawMessage) string {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(input, &args); err != nil {
		return ""
	}
	raw, ok := args["path"]
	if !ok {
		return ""
	}
	var path string
	if err := json.Unmarshal(raw, &path); err != nil {
		return ""
	}
	return path
}

// ── ExitPlanModeReviewPolicy ──

// ExitPlanModeReviewPolicy asks before allowing ExitPlanMode tool,
// giving the user a chance to review the plan before switching to implementation.
type ExitPlanModeReviewPolicy struct{}

func NewExitPlanModeReviewPolicy() *ExitPlanModeReviewPolicy { return &ExitPlanModeReviewPolicy{} }

func (p *ExitPlanModeReviewPolicy) Name() string { return "exit-plan-mode-review" }

func (p *ExitPlanModeReviewPolicy) Evaluate(toolName string, _ json.RawMessage) Result {
	if toolName == "ExitPlanMode" {
		return Result{Decision: DecisionAsk, Reason: "plan mode exit requires review", Policy: p.Name()}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}

// ── GoalStartReviewPolicy ──

// GoalStartReviewPolicy asks before allowing goal-starting operations.
type GoalStartReviewPolicy struct{}

func NewGoalStartReviewPolicy() *GoalStartReviewPolicy { return &GoalStartReviewPolicy{} }

func (p *GoalStartReviewPolicy) Name() string { return "goal-start-review" }

func (p *GoalStartReviewPolicy) Evaluate(toolName string, input json.RawMessage) Result {
	// Check if this is a goal-starting Bash command (e.g., running long processes)
	if toolName == "Bash" {
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(input, &args); err == nil {
			cmd := strings.TrimSpace(args.Command)
			// Flag long-running or background processes for review
			if strings.Contains(cmd, "nohup") || strings.Contains(cmd, "&") ||
				strings.HasPrefix(cmd, "screen ") || strings.HasPrefix(cmd, "tmux ") {
				return Result{
					Decision: DecisionAsk,
					Reason:   "background/long-running process requires review",
					Policy:   p.Name(),
				}
			}
		}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}

// ── AutoModeAskUserQuestionDenyPolicy ──

// AutoModeAskUserQuestionDenyPolicy denies the AskUser tool in auto mode
// to prevent the agent from blocking on user interaction for safe operations.
type AutoModeAskUserQuestionDenyPolicy struct{}

func NewAutoModeAskUserQuestionDenyPolicy() *AutoModeAskUserQuestionDenyPolicy {
	return &AutoModeAskUserQuestionDenyPolicy{}
}

func (p *AutoModeAskUserQuestionDenyPolicy) Name() string { return "auto-mode-ask-user-deny" }

func (p *AutoModeAskUserQuestionDenyPolicy) Evaluate(toolName string, _ json.RawMessage) Result {
	if toolName == "AskUser" {
		return Result{
			Decision: DecisionDeny,
			Reason:   "AskUser is disabled in auto mode",
			Policy:   p.Name(),
		}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}

// ── AgentSwarmExclusiveDenyPolicy ──

// AgentSwarmExclusiveDenyPolicy denies tools that should not be available
// to sub-agents spawned by the swarm system.
type AgentSwarmExclusiveDenyPolicy struct {
	mu          sync.RWMutex
	isSubAgent  bool
	deniedTools map[string]bool
}

func NewAgentSwarmExclusiveDenyPolicy() *AgentSwarmExclusiveDenyPolicy {
	return &AgentSwarmExclusiveDenyPolicy{
		deniedTools: map[string]bool{
			"AskUser":      true,
			"EnterPlanMode": true,
		},
	}
}

func (p *AgentSwarmExclusiveDenyPolicy) Name() string { return "agent-swarm-exclusive-deny" }

func (p *AgentSwarmExclusiveDenyPolicy) SetSubAgent(isSub bool) {
	p.mu.Lock()
	p.isSubAgent = isSub
	p.mu.Unlock()
}

func (p *AgentSwarmExclusiveDenyPolicy) Evaluate(toolName string, _ json.RawMessage) Result {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.isSubAgent {
		return Result{Decision: DecisionAsk, Policy: p.Name()}
	}
	if p.deniedTools[toolName] {
		return Result{
			Decision: DecisionDeny,
			Reason:   toolName + " is not available to sub-agents",
			Policy:   p.Name(),
		}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}

// ── GitCWDWriteApprovePolicy ──

// GitCWDWriteApprovePolicy auto-approves git operations that write to the
// current working directory (git add, commit, etc.) since they're expected.
type GitCWDWriteApprovePolicy struct{}

func NewGitCWDWriteApprovePolicy() *GitCWDWriteApprovePolicy { return &GitCWDWriteApprovePolicy{} }

func (p *GitCWDWriteApprovePolicy) Name() string { return "git-cwd-write-approve" }

func (p *GitCWDWriteApprovePolicy) Evaluate(toolName string, input json.RawMessage) Result {
	if toolName != "Bash" {
		return Result{Decision: DecisionAsk, Policy: p.Name()}
	}
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{Decision: DecisionAsk, Policy: p.Name()}
	}
	cmd := strings.TrimSpace(args.Command)
	// Auto-approve common safe git operations
	safeGitPrefixes := []string{
		"git status", "git log", "git diff", "git show", "git branch",
		"git remote", "git stash list", "git tag",
	}
	for _, prefix := range safeGitPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return Result{Decision: DecisionAllow, Reason: "safe git operation", Policy: p.Name()}
		}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}

// ── DefaultToolApprovePolicy ──

// DefaultToolApprovePolicy auto-approves tools that are always safe.
// This is a superset of SafeToolApprovePolicy used in auto mode chains.
type DefaultToolApprovePolicy struct{}

func NewDefaultToolApprovePolicy() *DefaultToolApprovePolicy { return &DefaultToolApprovePolicy{} }

func (p *DefaultToolApprovePolicy) Name() string { return "default-tool-approve" }

var defaultApprovedTools = map[string]bool{
	"Read":        true,
	"Glob":        true,
	"Grep":        true,
	"FetchURL":    true,
	"WebSearch":   true,
	"ReadMedia":   true,
	"update_plan": true,
	"TodoList":    true,
}

func (p *DefaultToolApprovePolicy) Evaluate(toolName string, _ json.RawMessage) Result {
	if defaultApprovedTools[toolName] {
		return Result{Decision: DecisionAllow, Reason: "default approved tool", Policy: p.Name()}
	}
	return Result{Decision: DecisionAsk, Policy: p.Name()}
}
