package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ── GoCallers Tool ─────────────────────────────────────────────────────

// GoCallersTool finds who calls a function/method using gograph.
type GoCallersTool struct {
	runner *GoGraphRunner
}

func NewGoCallersTool(runner *GoGraphRunner) *GoCallersTool {
	return &GoCallersTool{runner: runner}
}

type goCallersInput struct {
	Symbol string `json:"symbol"`
	Depth  int    `json:"depth,omitempty"`
}

func (t *GoCallersTool) Definition() Definition {
	return Definition{
		Name:        "GoCallers",
		Description: "Find all callers of a Go function/method using AST-aware call graph analysis. Returns precise call sites, not text matches.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{"type": "string", "description": "The function or method name (e.g., 'MyFunc' or 'pkg.Method')"},
				"depth":  map[string]interface{}{"type": "integer", "description": "Call depth to traverse (default 1)"},
			},
			"required": []string{"symbol"},
		},
	}
}

func (t *GoCallersTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "callers", input, func(params goCallersInput) []string {
		args := []string{params.Symbol}
		if params.Depth > 0 {
			args = append(args, "--depth", fmt.Sprintf("%d", params.Depth))
		}
		return args
	})
}

// ── GoCallees Tool ─────────────────────────────────────────────────────

// GoCalleesTool finds what a function calls using gograph.
type GoCalleesTool struct {
	runner *GoGraphRunner
}

func NewGoCalleesTool(runner *GoGraphRunner) *GoCalleesTool {
	return &GoCalleesTool{runner: runner}
}

type goCalleesInput struct {
	Symbol string `json:"symbol"`
	Depth  int    `json:"depth,omitempty"`
}

func (t *GoCalleesTool) Definition() Definition {
	return Definition{
		Name:        "GoCallees",
		Description: "Find all functions/methods called by a Go function using AST-aware call graph analysis.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{"type": "string", "description": "The function or method name (e.g., 'MyFunc' or 'pkg.Method')"},
				"depth":  map[string]interface{}{"type": "integer", "description": "Call depth to traverse (default 1)"},
			},
			"required": []string{"symbol"},
		},
	}
}

func (t *GoCalleesTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "callees", input, func(params goCalleesInput) []string {
		args := []string{params.Symbol}
		if params.Depth > 0 {
			args = append(args, "--depth", fmt.Sprintf("%d", params.Depth))
		}
		return args
	})
}

// ── GoContext Tool ─────────────────────────────────────────────────────

// GoContextTool provides a comprehensive context bundle for a symbol.
type GoContextTool struct {
	runner *GoGraphRunner
}

func NewGoContextTool(runner *GoGraphRunner) *GoContextTool {
	return &GoContextTool{runner: runner}
}

type goContextInput struct {
	Symbol string `json:"symbol"`
}

func (t *GoContextTool) Definition() Definition {
	return Definition{
		Name:        "GoContext",
		Description: "Get comprehensive context for a Go symbol: source code, callers, callees, and related tests in one call.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{"type": "string", "description": "The function or method name"},
			},
			"required": []string{"symbol"},
		},
	}
}

func (t *GoContextTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "context", input, func(params goContextInput) []string {
		return []string{params.Symbol}
	})
}

// ── GoSource Tool ──────────────────────────────────────────────────────

// GoSourceTool extracts function/method source code.
type GoSourceTool struct {
	runner *GoGraphRunner
}

func NewGoSourceTool(runner *GoGraphRunner) *GoSourceTool {
	return &GoSourceTool{runner: runner}
}

type goSourceInput struct {
	Symbol string `json:"symbol"`
}

func (t *GoSourceTool) Definition() Definition {
	return Definition{
		Name:        "GoSource",
		Description: "Extract the source code of a Go function/method (not the entire file). Returns only the function body.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{"type": "string", "description": "The function or method name"},
			},
			"required": []string{"symbol"},
		},
	}
}

func (t *GoSourceTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "source", input, func(params goSourceInput) []string {
		return []string{params.Symbol}
	})
}

// ── GoQuery Tool ───────────────────────────────────────────────────────

// GoQueryTool performs AST-aware symbol search.
type GoQueryTool struct {
	runner *GoGraphRunner
}

func NewGoQueryTool(runner *GoGraphRunner) *GoQueryTool {
	return &GoQueryTool{runner: runner}
}

type goQueryInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

func (t *GoQueryTool) Definition() Definition {
	return Definition{
		Name:        "GoQuery",
		Description: "Search for Go symbols using AST-aware queries. More precise than text grep for identifiers.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "The search query (symbol name, pattern, etc.)"},
				"limit": map[string]interface{}{"type": "integer", "description": "Maximum number of results (default 20)"},
			},
			"required": []string{"query"},
		},
	}
}

func (t *GoQueryTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "query", input, func(params goQueryInput) []string {
		args := []string{params.Query}
		if params.Limit > 0 {
			args = append(args, "--limit", fmt.Sprintf("%d", params.Limit))
		}
		return args
	})
}

// ── GoSummary Tool ─────────────────────────────────────────────────────

// GoSummaryTool provides a codebase briefing.
type GoSummaryTool struct {
	runner *GoGraphRunner
}

func NewGoSummaryTool(runner *GoGraphRunner) *GoSummaryTool {
	return &GoSummaryTool{runner: runner}
}

func (t *GoSummaryTool) Definition() Definition {
	return Definition{
		Name:        "GoSummary",
		Description: "Get a high-level briefing of the Go codebase: hotspots, complexity metrics, orphan functions, and architectural insights.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t *GoSummaryTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "summary", input, func(_ json.RawMessage) []string {
		return []string{}
	})
}

// ── GoPlan Tool ────────────────────────────────────────────────────────

// GoPlanTool performs pre-edit impact analysis.
type GoPlanTool struct {
	runner *GoGraphRunner
}

func NewGoPlanTool(runner *GoGraphRunner) *GoPlanTool {
	return &GoPlanTool{runner: runner}
}

type goPlanInput struct {
	Symbol string `json:"symbol"`
}

func (t *GoPlanTool) Definition() Definition {
	return Definition{
		Name:        "GoPlan",
		Description: "Analyze the impact of changing a Go function/method before making edits. Shows affected callers and dependencies.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{"type": "string", "description": "The function or method to analyze"},
			},
			"required": []string{"symbol"},
		},
	}
}

func (t *GoPlanTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "plan", input, func(params goPlanInput) []string {
		return []string{params.Symbol}
	})
}

// ── GoImpact Tool ──────────────────────────────────────────────────────

// GoImpactTool analyzes the blast radius of a change.
type GoImpactTool struct {
	runner *GoGraphRunner
}

func NewGoImpactTool(runner *GoGraphRunner) *GoImpactTool {
	return &GoImpactTool{runner: runner}
}

type goImpactInput struct {
	Symbol string `json:"symbol"`
	Since  string `json:"since,omitempty"`
}

func (t *GoImpactTool) Definition() Definition {
	return Definition{
		Name:        "GoImpact",
		Description: "Analyze the blast radius of changing a Go symbol. Shows all affected code paths and test coverage.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{"type": "string", "description": "The function or method to analyze"},
				"since":  map[string]interface{}{"type": "string", "description": "Git ref to compare changes (e.g., 'HEAD~1', 'main')"},
			},
			"required": []string{"symbol"},
		},
	}
}

func (t *GoImpactTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	return executeGoGraphTool(ctx, t.runner, exec, "impact", input, func(params goImpactInput) []string {
		args := []string{params.Symbol}
		if params.Since != "" {
			args = append(args, "--since", params.Since)
		}
		return args
	})
}

// ── Common Execution Logic ─────────────────────────────────────────────

// executeGoGraphTool is the common execution path for all gograph tools.
// It handles validation, availability checks, graph building, and precision tagging.
func executeGoGraphTool[P any](
	ctx context.Context,
	runner *GoGraphRunner,
	exec ExecContext,
	subcmd string,
	input json.RawMessage,
	buildArgs func(P) []string,
) (*Result, error) {
	// 1. Check if gograph is available
	if !IsGoGraphAvailable() {
		return &Result{
			Output:  "gograph not installed. Use Grep/Glob/Read instead for text-based search.",
			IsError: true,
		}, nil
	}

	// 2. Check if this is a Go project
	workDir := exec.WorkDir
	if !IsGoProject(workDir) {
		return &Result{
			Output:  "not a Go project (no go.mod found). Use Grep/Glob/Read instead.",
			IsError: true,
		}, nil
	}

	// 3. Parse input parameters
	var params P
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// 4. Ensure graph is built
	if !runner.IsGraphBuilt(workDir) {
		if err := runner.EnsureGraph(ctx, workDir); err != nil {
			return &Result{
				Output:  fmt.Sprintf("failed to build gograph: %s", err),
				IsError: true,
			}, nil
		}
	}

	// 5. Execute gograph command
	args := buildArgs(params)
	output, err := runner.RunJSON(ctx, workDir, subcmd, args...)
	if err != nil {
		return &Result{
			Output:  fmt.Sprintf("gograph %s failed: %s", subcmd, err),
			IsError: true,
		}, nil
	}

	// 6. Append precision tag
	precision := runner.PrecisionMode(ctx, workDir)
	if precision != "" {
		output = fmt.Sprintf("[gograph: %s]\n%s", precision, output)
	}

	return &Result{Output: output}, nil
}

// ── Registration ───────────────────────────────────────────────────────

// RegisterGoGraphTools registers all Go-specific tools with the registry.
func RegisterGoGraphTools(registry *Registry, runner *GoGraphRunner) {
	registry.Register(NewGoCallersTool(runner))
	registry.Register(NewGoCalleesTool(runner))
	registry.Register(NewGoContextTool(runner))
	registry.Register(NewGoSourceTool(runner))
	registry.Register(NewGoQueryTool(runner))
	registry.Register(NewGoSummaryTool(runner))
	registry.Register(NewGoPlanTool(runner))
	registry.Register(NewGoImpactTool(runner))
}
