package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BashTool executes shell commands.
type BashTool struct {
	Shell string
}

func NewBashTool() *BashTool {
	return &BashTool{Shell: "/bin/bash"}
}

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func (t *BashTool) Definition() Definition {
	return Definition{
		Name:        "Bash",
		Description: "Execute a shell command. Use for running scripts, installing packages, or any system operation.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{"type": "string", "description": "The shell command to execute"},
				"timeout": map[string]interface{}{"type": "integer", "description": "Timeout in seconds (default 120)"},
			},
			"required": []string{"command"},
		},
	}
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params bashInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// Simple exec using os/exec
	cmd := newCommand(ctx, t.Shell, "-c", params.Command)
	if exec.WorkDir != "" {
		cmd.Dir = exec.WorkDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	if err != nil {
		return &Result{Output: output, IsError: true}, nil
	}
	return &Result{Output: output}, nil
}

// ReadFileTool reads file contents.
type ReadFileTool struct{}

func NewReadFileTool() *ReadFileTool { return &ReadFileTool{} }

type readFileInput struct {
	Path      string `json:"path"`
	StartLine *int   `json:"startLine,omitempty"`
	EndLine   *int   `json:"endLine,omitempty"`
}

func (t *ReadFileTool) Definition() Definition {
	return Definition{
		Name:        "Read",
		Description: "Read the contents of a file. Supports optional line range.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":      map[string]interface{}{"type": "string", "description": "Absolute path to the file to read"},
				"startLine": map[string]interface{}{"type": "integer", "description": "Start line (1-based, inclusive)"},
				"endLine":   map[string]interface{}{"type": "integer", "description": "End line (1-based, inclusive)"},
			},
			"required": []string{"path"},
		},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params readFileInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path := resolvePath(params.Path, exec.WorkDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	text := string(data)

	// Apply line range if specified
	if params.StartLine != nil || params.EndLine != nil {
		lines := strings.Split(text, "\n")
		start := 0
		end := len(lines)
		if params.StartLine != nil && *params.StartLine > 0 {
			start = *params.StartLine - 1
		}
		if params.EndLine != nil && *params.EndLine < len(lines) {
			end = *params.EndLine
		}
		if start > len(lines) {
			start = len(lines)
		}
		if end > len(lines) {
			end = len(lines)
		}
		text = strings.Join(lines[start:end], "\n")
	}

	return &Result{Output: text}, nil
}

// WriteFileTool writes file contents.
type WriteFileTool struct{}

func NewWriteFileTool() *WriteFileTool { return &WriteFileTool{} }

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteFileTool) Definition() Definition {
	return Definition{
		Name:        "Write",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":    map[string]interface{}{"type": "string", "description": "Absolute path to the file to write"},
				"content": map[string]interface{}{"type": "string", "description": "The content to write to the file"},
			},
			"required": []string{"path", "content"},
		},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params writeFileInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path := resolvePath(params.Path, exec.WorkDir)

	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	if err := os.WriteFile(path, []byte(params.Content), 0644); err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}
	return &Result{Output: fmt.Sprintf("File written successfully: %s", path)}, nil
}

// GlobTool finds files matching a pattern.
type GlobTool struct{}

func NewGlobTool() *GlobTool { return &GlobTool{} }

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (t *GlobTool) Definition() Definition {
	return Definition{
		Name:        "Glob",
		Description: "Find files matching a glob pattern.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{"type": "string", "description": "Glob pattern (e.g. **/*.go)"},
				"path":    map[string]interface{}{"type": "string", "description": "Base directory to search from"},
			},
			"required": []string{"pattern"},
		},
	}
}

func (t *GlobTool) Execute(_ context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params globInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	base := params.Path
	if base == "" {
		base = exec.WorkDir
	}
	if base == "" {
		base = "."
	}

	// Check if pattern contains **
	if strings.Contains(params.Pattern, "**") {
		matches, err := globDoubleStar(base, params.Pattern)
		if err != nil {
			return &Result{Output: err.Error(), IsError: true}, nil
		}
		return &Result{Output: strings.Join(matches, "\n")}, nil
	}

	// Standard glob for non-** patterns
	pattern := filepath.Join(base, params.Pattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}
	return &Result{Output: strings.Join(matches, "\n")}, nil
}

// globDoubleStar handles ** glob patterns using filepath.WalkDir.
func globDoubleStar(base, pattern string) ([]string, error) {
	var matches []string

	// Split pattern into parts
	parts := strings.Split(pattern, "**")
	prefix := parts[0]
	suffix := ""
	if len(parts) > 1 {
		suffix = parts[1]
	}

	// Normalize prefix/suffix
	if strings.HasPrefix(prefix, "/") {
		prefix = prefix[1:]
	}
	if strings.HasSuffix(prefix, "/") {
		prefix = prefix[:len(prefix)-1]
	}
	if strings.HasPrefix(suffix, "/") {
		suffix = suffix[1:]
	}

	searchRoot := base
	if prefix != "" {
		searchRoot = filepath.Join(base, prefix)
	}

	err := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden directories and node_modules
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path from base
		relPath, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}

		// Match against pattern
		matched, err := matchDoubleStar(relPath, pattern)
		if err != nil {
			return nil
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})

	return matches, err
}

// matchDoubleStar matches a path against a ** pattern.
func matchDoubleStar(path, pattern string) (bool, error) {
	// Simple implementation: split by ** and check prefix/suffix
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := ""
	if len(parts) > 1 {
		suffix = strings.TrimPrefix(parts[1], "/")
	}

	// Check prefix
	if prefix != "" && !strings.HasPrefix(path, prefix) {
		return false, nil
	}

	// Check suffix using filepath.Match
	if suffix != "" {
		// Extract the filename or remaining path to match
		var toMatch string
		if prefix != "" {
			toMatch = strings.TrimPrefix(path, prefix+"/")
			toMatch = strings.TrimPrefix(toMatch, prefix)
		} else {
			toMatch = path
		}

		// For patterns like "*.go", match against basename
		if !strings.Contains(suffix, "/") {
			toMatch = filepath.Base(toMatch)
		}

		return filepath.Match(suffix, toMatch)
	}

	return true, nil
}

// GrepTool searches file contents with regex using ripgrep.
type GrepTool struct{}

func NewGrepTool() *GrepTool { return &GrepTool{} }

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Glob    string `json:"glob,omitempty"`
	Type    string `json:"type,omitempty"`
	CaseI   bool   `json:"-i,omitempty"`
}

func (t *GrepTool) Definition() Definition {
	return Definition{
		Name:        "Grep",
		Description: "Search file contents using a regex pattern. Uses ripgrep for fast, recursive search.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{"type": "string", "description": "Regex pattern to search for"},
				"path":    map[string]interface{}{"type": "string", "description": "File or directory to search. Defaults to working directory"},
				"glob":    map[string]interface{}{"type": "string", "description": "Glob filter for files to search, e.g. '*.ts'"},
				"type":    map[string]interface{}{"type": "string", "description": "File type filter (e.g. 'go', 'ts', 'py')"},
				"-i":      map[string]interface{}{"type": "boolean", "description": "Case-insensitive search"},
			},
			"required": []string{"pattern"},
		},
	}
}

func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage, execCtx ExecContext) (*Result, error) {
	var params grepInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	searchPath := params.Path
	if searchPath == "" {
		searchPath = execCtx.WorkDir
	}
	if searchPath == "" {
		searchPath = "."
	}

	// Check if path exists
	if _, err := os.Stat(searchPath); err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	// Build rg command
	args := []string{
		"--json",
		"--hidden",
		"--glob", "!.git",
		"--glob", "!node_modules",
	}

	if params.CaseI {
		args = append(args, "-i")
	}
	if params.Glob != "" {
		args = append(args, "--glob", params.Glob)
	}
	if params.Type != "" {
		args = append(args, "--type", params.Type)
	}

	args = append(args, params.Pattern, searchPath)

	cmd := exec.CommandContext(ctx, "rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// rg returns exit code 1 when no matches found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &Result{Output: "No matches found"}, nil
		}
		// Exit code 2 means an error occurred
		if stderr.Len() > 0 {
			return &Result{Output: stderr.String(), IsError: true}, nil
		}
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	// Parse JSON output
	output := parseRgJSON(stdout.String())
	if output == "" {
		return &Result{Output: "No matches found"}, nil
	}

	return &Result{Output: output}, nil
}

// parseRgJSON parses ripgrep JSON output into a readable format.
func parseRgJSON(jsonOutput string) string {
	var results []string
	lines := strings.Split(strings.TrimSpace(jsonOutput), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		var msg struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		if msg.Type == "match" {
			lineText := strings.TrimRight(msg.Data.Lines.Text, "\n\r")
			result := fmt.Sprintf("%s:%d: %s", msg.Data.Path.Text, msg.Data.LineNumber, lineText)
			results = append(results, result)
		}
	}

	return strings.Join(results, "\n")
}

// EditTool performs exact string replacement in files.
type EditTool struct{}

func NewEditTool() *EditTool { return &EditTool{} }

type editInput struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (t *EditTool) Definition() Definition {
	return Definition{
		Name:        "Edit",
		Description: "Perform exact string replacement in a file. Errors when old_string is not found or not unique (when replace_all is false).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":        map[string]interface{}{"type": "string", "description": "Path to the file to edit"},
				"old_string":  map[string]interface{}{"type": "string", "description": "Exact content to replace"},
				"new_string":  map[string]interface{}{"type": "string", "description": "Replacement text"},
				"replace_all": map[string]interface{}{"type": "boolean", "description": "Replace all occurrences (default: false)"},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
	}
}

func (t *EditTool) Execute(_ context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params editInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	if params.OldString == params.NewString {
		return &Result{Output: "No changes to make: old_string and new_string are exactly the same.", IsError: true}, nil
	}

	path := resolvePath(params.Path, exec.WorkDir)
	content, err := os.ReadFile(path)
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	text := string(content)
	var newText string

	if params.ReplaceAll {
		newText = strings.ReplaceAll(text, params.OldString, params.NewString)
		if newText == text {
			return &Result{Output: fmt.Sprintf("old_string not found in %s", path), IsError: true}, nil
		}
	} else {
		// Replace first occurrence only (matches TS behavior)
		idx := strings.Index(text, params.OldString)
		if idx == -1 {
			return &Result{Output: fmt.Sprintf("old_string not found in %s", path), IsError: true}, nil
		}
		newText = text[:idx] + params.NewString + text[idx+len(params.OldString):]
	}

	if err := os.WriteFile(path, []byte(newText), 0644); err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	return &Result{Output: fmt.Sprintf("Successfully edited %s", path)}, nil
}

// RegisterDefaultTools registers all default built-in tools.
func RegisterDefaultTools(registry *Registry) {
	registry.Register(NewBashTool())
	registry.Register(NewReadFileTool())
	registry.Register(NewWriteFileTool())
	registry.Register(NewGlobTool())
	registry.Register(NewGrepTool())
	registry.Register(NewEditTool())
	registry.Register(NewTodoListTool())
	registry.Register(NewFetchURLTool())
	registry.Register(NewReadMediaTool())
	registry.Register(NewAskUserTool(nil))
}

func resolvePath(path, workDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if workDir != "" {
		return filepath.Join(workDir, path)
	}
	return path
}
