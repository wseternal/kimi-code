package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// resolveEditorCommand returns the editor command to use.
// Priority: $VISUAL > $EDITOR > "vi"
func resolveEditorCommand() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	return "vi"
}

// openEditor opens an external text editor for composing a prompt.
// It creates a temporary markdown file with optional conversation context,
// launches the editor, and returns the user's input after editing.
//
// Parameters:
//   - currentInput: any existing input text to pre-fill
//   - recentContext: recent conversation messages for context (optional)
//
// Returns the edited content, or ("", nil) if the user left it empty.
func openEditor(currentInput, recentContext string) (string, error) {
	editor := resolveEditorCommand()

	// Build template content
	var template strings.Builder
	template.WriteString("# Your prompt:\n")
	template.WriteString("# Write your message below this line.\n")
	template.WriteString("# Lines starting with # will be removed.\n\n")

	if currentInput != "" {
		template.WriteString(currentInput)
		template.WriteString("\n\n")
	} else {
		template.WriteString("\n")
	}

	if recentContext != "" {
		template.WriteString("\n---\n")
		template.WriteString("# Recent conversation for context:\n")
		for _, line := range strings.Split(recentContext, "\n") {
			template.WriteString("# " + line + "\n")
		}
	}

	// Create temp file
	f, err := os.CreateTemp("", "kimi-code-prompt-*.md")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	if _, err := f.WriteString(template.String()); err != nil {
		f.Close()
		return "", fmt.Errorf("failed to write template: %w", err)
	}
	f.Close()

	// Launch editor
	editorArgs := strings.Fields(editor)
	cmd := exec.Command(editorArgs[0], append(editorArgs[1:], tmpPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	// Read back the edited content
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read edited file: %w", err)
	}

	// Extract user content: remove comment lines and template markers
	var result strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue // skip comment lines
		}
		if trimmed == "---" {
			break // stop at context separator
		}
		result.WriteString(line)
		result.WriteString("\n")
	}

	return strings.TrimSpace(result.String()), nil
}
