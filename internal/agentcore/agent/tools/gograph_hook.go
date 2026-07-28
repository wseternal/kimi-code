package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
)

// ── GoGraph Hook ───────────────────────────────────────────────────────

// GoGraphHook is a ToolHook that enhances the Grep tool with gograph-backed
// AST-aware symbol search when working in Go projects.
type GoGraphHook struct {
	runner *GoGraphRunner
}

// NewGoGraphHook creates a new hook backed by the given runner.
func NewGoGraphHook(runner *GoGraphRunner) *GoGraphHook {
	return &GoGraphHook{runner: runner}
}

func (h *GoGraphHook) Name() string { return "gograph" }

func (h *GoGraphHook) Wrap(toolName string, original Tool) Tool {
	if toolName == "Grep" {
		return &goEnhancedGrepTool{original: original, runner: h.runner}
	}
	return nil // pass through for other tools
}

// ── Enhanced Grep Tool ─────────────────────────────────────────────────

// goSymbolPattern matches Go exported symbols (PascalCase identifiers).
// Examples: MyFunc, HTTPClient, pkg.SomeType
var goSymbolPattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$|^[a-z][A-Za-z0-9]*\.[A-Z][A-Za-z0-9]*$`)

// goEnhancedGrepTool wraps the original Grep tool with gograph enhancement.
type goEnhancedGrepTool struct {
	original Tool
	runner   *GoGraphRunner
}

func (t *goEnhancedGrepTool) Definition() Definition {
	// Return the original definition — the LLM doesn't need to know about the enhancement.
	return t.original.Definition()
}

func (t *goEnhancedGrepTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	// 1. Parse the pattern from input
	var params struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		// If we can't parse, just delegate to original
		return t.original.Execute(ctx, input, exec)
	}

	// 2. Check if this looks like a Go symbol
	if !goSymbolPattern.MatchString(params.Pattern) {
		// Not a Go symbol pattern — use original grep
		return t.original.Execute(ctx, input, exec)
	}

	// 3. Check if gograph is available and this is a Go project
	if !IsGoGraphAvailable() || !IsGoProject(exec.WorkDir) {
		// Fall through to original grep
		return t.original.Execute(ctx, input, exec)
	}

	// 4. Ensure graph is built
	workDir := exec.WorkDir
	if !t.runner.IsGraphBuilt(workDir) {
		if err := t.runner.EnsureGraph(ctx, workDir); err != nil {
			// Graph build failed — fall through to original grep
			return t.original.Execute(ctx, input, exec)
		}
	}

	// 5. Try gograph query
	output, err := t.runner.RunJSON(ctx, workDir, "query", []string{params.Pattern}...)
	if err != nil || output == "" {
		// gograph returned no results or error — fall through to original grep
		return t.original.Execute(ctx, input, exec)
	}

	// 6. Success — return gograph results with precision tag
	precision := t.runner.PrecisionMode(ctx, workDir)
	if precision != "" {
		output = fmt.Sprintf("[gograph: %s]\n%s", precision, output)
	} else {
		output = "[gograph]\n" + output
	}

	return &Result{Output: output}, nil
}
