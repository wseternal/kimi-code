package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultMaxCodeBlockLines = 50
	defaultTruncatedLines   = 20
)

// MarkdownBuffer accumulates streaming markdown chunks and determines
// safe points to flush content for rendering. It tracks open markdown
// constructs (code fences, emphasis) to ensure we only output complete,
// well-formed markdown.
//
// This is a simplified Go port of goose's streaming_buffer.rs,
// focusing on the most common constructs: code fences and emphasis.
type MarkdownBuffer struct {
	buffer string
}

// NewMarkdownBuffer creates an empty streaming markdown buffer.
func NewMarkdownBuffer() MarkdownBuffer {
	return MarkdownBuffer{}
}

// Push adds a chunk of markdown text to the buffer.
// Returns the content that is safe to render, or ("", false) if the buffer
// contains only incomplete constructs that should be held back.
func (b *MarkdownBuffer) Push(chunk string) (string, bool) {
	b.buffer += chunk
	safeEnd := b.findSafeEnd()

	if safeEnd > 0 {
		safe := b.buffer[:safeEnd]
		b.buffer = b.buffer[safeEnd:]
		return truncateCodeBlocks(safe), true
	}
	return "", false
}

// Flush returns all remaining buffered content. Call this when the
// stream is complete (e.g., on "done" event). Returns raw content
// without truncation since the stream is finished.
func (b *MarkdownBuffer) Flush() string {
	if b.buffer == "" {
		return ""
	}
	remaining := b.buffer
	b.buffer = ""
	return remaining
}

// findSafeEnd scans the buffer to find the last byte position where all
// markdown constructs are closed. Returns 0 if no safe point exists.
func (b *MarkdownBuffer) findSafeEnd() int {
	content := b.buffer
	if content == "" {
		return 0
	}

	inCodeBlock := false
	var fenceChar byte
	fenceLen := 0
	inInlineCode := false
	inlineCodeLen := 0
	inBold := false
	inItalic := false

	lastSafe := 0
	i := 0

	for i < len(content) {
		c := content[i]

		// Code fence detection (``` or ~~~)
		if !inInlineCode && !inCodeBlock && (c == '`' || c == '~') {
			run := 1
			for i+run < len(content) && content[i+run] == c {
				run++
			}
			if run >= 3 {
				// Opening code fence
				inCodeBlock = true
				fenceChar = c
				fenceLen = run
				i += run
				// Skip to end of fence line (language tag)
				for i < len(content) && content[i] != '\n' {
					i++
				}
				continue
			}
		}

		if inCodeBlock {
			if c == '\n' {
				// Check if next line starts with a closing fence
				next := i + 1
				if next < len(content) {
					run := 0
					for next+run < len(content) && content[next+run] == fenceChar {
						run++
					}
					if run >= fenceLen {
						// Check that rest of line is whitespace
						afterFence := next + run
						eol := afterFence
						for eol < len(content) && content[eol] != '\n' {
							if content[eol] != ' ' && content[eol] != '\t' {
								break
							}
							eol++
						}
						if eol == afterFence || (eol < len(content) && eol == afterFence) {
							inCodeBlock = false
							// Safe point is after the closing fence line
							if eol < len(content) && content[eol] == '\n' {
								lastSafe = eol + 1
							} else {
								lastSafe = eol
							}
							i = eol
							continue
						}
					}
				}
				// Don't set lastSafe inside code blocks — only the closing
				// fence boundary is a safe flush point.
			}
			i++
			continue
		}

		// Inline code
		if c == '`' && !inCodeBlock {
			if inInlineCode {
				// Check if closing backticks match opening count
				run := 1
				for i+run < len(content) && content[i+run] == '`' {
					run++
				}
				if run == inlineCodeLen {
					inInlineCode = false
					i += run
					lastSafe = i
					continue
				}
				i += run
				continue
			}
			// Opening inline code
			run := 1
			for i+run < len(content) && content[i+run] == '`' {
				run++
			}
			inInlineCode = true
			inlineCodeLen = run
			i += run
			continue
		}

		if inInlineCode {
			i++
			continue
		}

		// Bold/italic detection (** and *)
		if c == '*' {
			run := 1
			for i+run < len(content) && content[i+run] == '*' {
				run++
			}
			if run >= 2 {
				inBold = !inBold
				i += run
				lastSafe = i
				continue
			}
			inItalic = !inItalic
			i++
			lastSafe = i
			continue
		}

		// Newline is always a safe point if no constructs are open
		if c == '\n' && !inBold && !inItalic {
			lastSafe = i + 1
		}

		i++
	}

	// If nothing is open, everything is safe
	if !inCodeBlock && !inInlineCode && !inBold && !inItalic {
		return len(content)
	}

	return lastSafe
}

// truncateCodeBlocks replaces code blocks exceeding maxLines with a
// truncated version showing the first showLines lines plus a pointer
// to a temp file containing the full content.
func truncateCodeBlocks(content string) string {
	maxLines := getMaxCodeBlockLines()
	if maxLines <= 0 {
		return content // truncation disabled
	}
	return truncateCodeBlocksWith(content, maxLines, getTruncatedShowLines())
}

func truncateCodeBlocksWith(content string, maxLines, showLines int) string {
	// Find opening code fence
	openIdx := strings.Index(content, "```")
	if openIdx == -1 {
		openIdx = strings.Index(content, "~~~")
	}
	if openIdx == -1 {
		return content
	}

	// Determine fence character and length
	fenceChar := content[openIdx]
	fenceLen := 0
	for i := openIdx; i < len(content) && content[i] == fenceChar; i++ {
		fenceLen++
	}

	// Find end of fence line (language tag)
	fenceLineEnd := strings.IndexByte(content[openIdx+fenceLen:], '\n')
	if fenceLineEnd == -1 {
		return content
	}
	codeStart := openIdx + fenceLen + fenceLineEnd + 1

	// Find closing fence
	rest := content[codeStart:]
	closeIdx := findClosingFence(rest, fenceChar, fenceLen)
	if closeIdx == -1 {
		return content
	}

	codeContent := rest[:closeIdx]
	lines := strings.Split(codeContent, "\n")

	if len(lines) <= maxLines {
		return content
	}

	// Truncate
	show := showLines
	if show > maxLines {
		show = maxLines
	}
	if show > len(lines) {
		show = len(lines)
	}
	truncated := strings.Join(lines[:show], "\n")
	remaining := len(lines) - show

	// Save full content to temp file
	var fileMsg string
	if path, err := saveToTempFile(codeContent); err == nil {
		fileMsg = fmt.Sprintf(" → %s", path)
	}

	// Reconstruct: prefix + truncated code + remaining message + suffix
	prefix := content[:codeStart]
	suffixStart := codeStart + closeIdx
	if suffixStart < len(content) && content[suffixStart] == '\n' {
		suffixStart++
	}
	suffix := ""
	if suffixStart < len(content) {
		suffix = content[suffixStart:]
	}

	return fmt.Sprintf("%s%s\n... (%d more lines%s)\n%s", prefix, truncated, remaining, fileMsg, suffix)
}

// findClosingFence finds a closing code fence in content.
// Returns the byte offset of the newline before the closing fence, or -1.
func findClosingFence(content string, fenceChar byte, minLen int) int {
	lines := strings.Split(content, "\n")
	offset := 0
	for _, line := range lines {
		run := 0
		for run < len(line) && line[run] == fenceChar {
			run++
		}
		if run >= minLen {
			// Check rest of line is whitespace
			rest := strings.TrimSpace(line[run:])
			if rest == "" {
				return offset - 1 // offset of \n before this line
			}
		}
		offset += len(line) + 1 // +1 for \n
	}
	return -1
}

// saveToTempFile writes content to a temp file and returns its path.
func saveToTempFile(content string) (string, error) {
	f, err := os.CreateTemp("", "kimi-code-*.txt")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

func getMaxCodeBlockLines() int {
	if v := os.Getenv("KIMI_NO_CODE_TRUNCATION"); v == "1" || strings.EqualFold(v, "true") {
		return 0 // disabled
	}
	if v := os.Getenv("KIMI_MAX_CODE_BLOCK_LINES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxCodeBlockLines
}

func getTruncatedShowLines() int {
	if v := os.Getenv("KIMI_TRUNCATED_SHOW_LINES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultTruncatedLines
}
