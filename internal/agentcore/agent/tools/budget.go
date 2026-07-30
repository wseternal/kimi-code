package tools

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ResultBudget controls how tool results are truncated to fit within context budget.
type ResultBudget struct {
	// MaxChars is the maximum number of characters allowed in a tool result.
	// 0 means no limit.
	MaxChars int
	// MaxLines is the maximum number of lines allowed. 0 means no limit.
	MaxLines int
}

// DefaultBudget returns the default tool result budget.
func DefaultBudget() ResultBudget {
	return ResultBudget{
		MaxChars: 100_000, // ~25K tokens
		MaxLines: 2000,
	}
}

// Apply truncates the result output if it exceeds the budget.
// Returns the (possibly truncated) output and whether truncation occurred.
func (b ResultBudget) Apply(output string) (string, bool) {
	if b.MaxChars <= 0 && b.MaxLines <= 0 {
		return output, false
	}

	// Check character limit
	truncated := false
	if b.MaxChars > 0 && utf8.RuneCountInString(output) > b.MaxChars {
		runes := []rune(output)
		half := b.MaxChars / 2
		output = string(runes[:half]) +
			"\n\n... [truncated " + formatNumber(utf8.RuneCountInString(output)-b.MaxChars) + " characters] ...\n\n" +
			string(runes[len(runes)-half:])
		truncated = true
	}

	// Check line limit
	if b.MaxLines > 0 {
		lines := strings.Split(output, "\n")
		if len(lines) > b.MaxLines {
			half := b.MaxLines / 2
			head := strings.Join(lines[:half], "\n")
			tail := strings.Join(lines[len(lines)-half:], "\n")
			output = head +
				"\n\n... [truncated " + formatNumber(len(lines)-b.MaxLines) + " lines] ...\n\n" +
				tail
			truncated = true
		}
	}

	return output, truncated
}

// formatNumber formats a number for display.
func formatNumber(n int) string {
	return fmt.Sprintf("%d", n)
}

// TruncateResult applies budget truncation to a tool Result.
func TruncateResult(result *Result, budget ResultBudget) {
	if result == nil {
		return
	}
	truncated, wasTruncated := budget.Apply(result.Output)
	result.Output = truncated
	result.Truncate = wasTruncated
}
