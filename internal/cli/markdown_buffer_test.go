package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestMarkdownBuffer_PlainText(t *testing.T) {
	buf := NewMarkdownBuffer()

	safe, ok := buf.Push("Hello world\n")
	if !ok {
		t.Error("expected plain text to flush immediately")
	}
	if safe != "Hello world\n" {
		t.Errorf("got %q, want %q", safe, "Hello world\n")
	}
}

func TestMarkdownBuffer_MultipleLines(t *testing.T) {
	buf := NewMarkdownBuffer()

	safe, ok := buf.Push("line 1\nline 2\nline 3\n")
	if !ok {
		t.Error("expected multiple complete lines to flush")
	}
	if safe != "line 1\nline 2\nline 3\n" {
		t.Errorf("got %q, want full content", safe)
	}
}

func TestMarkdownBuffer_UnclosedCodeBlock(t *testing.T) {
	buf := NewMarkdownBuffer()

	// Push text before code block
	safe, ok := buf.Push("Some text\n")
	if !ok {
		t.Error("expected text before code block to flush")
	}
	if safe != "Some text\n" {
		t.Errorf("got %q, want %q", safe, "Some text\n")
	}

	// Push opening fence — should be held back
	safe, ok = buf.Push("```go\n")
	if ok {
		t.Errorf("expected unclosed code fence to hold content, got %q", safe)
	}

	// Push code content — should still be held
	safe, ok = buf.Push("func main() {}\n")
	if ok {
		t.Errorf("expected content inside code block to hold, got %q", safe)
	}

	// Push closing fence — should now flush everything
	safe, ok = buf.Push("```\n")
	if !ok {
		t.Error("expected content to flush after closing fence")
	}
	if !strings.Contains(safe, "```go\n") {
		t.Errorf("flushed content should contain opening fence, got %q", safe)
	}
	if !strings.Contains(safe, "func main() {}\n") {
		t.Errorf("flushed content should contain code, got %q", safe)
	}
}

func TestMarkdownBuffer_ClosedCodeBlock(t *testing.T) {
	buf := NewMarkdownBuffer()

	// Push a complete code block in one chunk
	content := "Here's code:\n```go\nfmt.Println(\"hi\")\n```\nMore text\n"
	safe, ok := buf.Push(content)
	if !ok {
		t.Error("expected complete code block to flush")
	}
	if safe != content {
		t.Errorf("got %q, want %q", safe, content)
	}
}

func TestMarkdownBuffer_Flush(t *testing.T) {
	buf := NewMarkdownBuffer()

	// Push unclosed content
	buf.Push("```go\nsome code")

	// Flush should return everything
	remaining := buf.Flush()
	if remaining != "```go\nsome code" {
		t.Errorf("Flush() = %q, want all remaining content", remaining)
	}

	// Second flush should be empty
	remaining = buf.Flush()
	if remaining != "" {
		t.Errorf("second Flush() = %q, want empty", remaining)
	}
}

func TestMarkdownBuffer_Bold(t *testing.T) {
	buf := NewMarkdownBuffer()

	// Push partial bold
	safe, ok := buf.Push("Hello **wor")
	if ok && strings.Contains(safe, "**wor") {
		t.Error("expected unclosed bold to be held back")
	}

	// Close bold
	safe, ok = buf.Push("ld**!\n")
	if !ok {
		t.Error("expected content to flush after closing bold")
	}
}

func TestMarkdownBuffer_StreamingChunks(t *testing.T) {
	buf := NewMarkdownBuffer()

	// Simulate realistic LLM streaming
	chunks := []string{
		"Here is ",
		"the code:\n\n",
		"```python\n",
		"def hello(",
		"):\n    print(",
		`"hello world")`,
		"\n```\n",
		"\nThat's it!",
	}

	var result strings.Builder
	for _, chunk := range chunks {
		if safe, ok := buf.Push(chunk); ok {
			result.WriteString(safe)
		}
	}
	result.WriteString(buf.Flush())

	output := result.String()
	if !strings.Contains(output, "```python\n") {
		t.Error("output should contain opening code fence")
	}
	if !strings.Contains(output, "```\n") {
		t.Error("output should contain closing code fence")
	}
	if !strings.Contains(output, "That's it!") {
		t.Error("output should contain trailing text")
	}
}

func TestTruncateCodeBlocks_ShortBlock(t *testing.T) {
	// Blocks under the limit should not be truncated
	content := "```go\nline1\nline2\nline3\n```\n"
	result := truncateCodeBlocksWith(content, 50, 20)
	if result != content {
		t.Errorf("short block should not be truncated, got %q", result)
	}
}

func TestTruncateCodeBlocks_LongBlock(t *testing.T) {
	// Build a block with 60 lines
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	content := "```go\n" + strings.Join(lines, "\n") + "\n```\n"

	result := truncateCodeBlocksWith(content, 50, 20)
	if strings.Contains(result, "line 25") {
		t.Error("truncated output should not contain lines beyond show limit")
	}
	if !strings.Contains(result, "line 0") {
		t.Error("truncated output should contain first lines")
	}
	if !strings.Contains(result, "more lines") {
		t.Error("truncated output should contain 'more lines' indicator")
	}
}

func TestTruncateCodeBlocks_NoFence(t *testing.T) {
	content := "Just plain text with no code blocks"
	result := truncateCodeBlocksWith(content, 50, 20)
	if result != content {
		t.Errorf("plain text should not be modified, got %q", result)
	}
}

// Need fmt for Sprintf in test
func init() {
	// Override env vars for tests to ensure consistent truncation behavior
}
