package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const (
	inputHistoryFile = "input_history"
	maxHistorySize   = 1000
)

// InputHistory manages persistent command input history across sessions.
// History is stored as a simple text file, one entry per line.
type InputHistory struct {
	file    string
	entries []string
	index   int // current navigation position (-1 = not navigating)
}

// NewInputHistory creates a new history manager backed by a file
// in the given config directory (typically ~/.gkimi-code/).
func NewInputHistory(configDir string) *InputHistory {
	return &InputHistory{
		file:  filepath.Join(configDir, inputHistoryFile),
		index: -1,
	}
}

// Load reads history entries from the backing file.
func (h *InputHistory) Load() error {
	f, err := os.Open(h.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no history yet
		}
		return err
	}
	defer f.Close()

	var entries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			entries = append(entries, line)
		}
	}
	// Keep only the last maxHistorySize entries
	if len(entries) > maxHistorySize {
		entries = entries[len(entries)-maxHistorySize:]
	}
	h.entries = entries
	h.index = -1
	return scanner.Err()
}

// Save writes history entries to the backing file.
func (h *InputHistory) Save() error {
	dir := filepath.Dir(h.file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Keep only the last maxHistorySize entries
	if len(h.entries) > maxHistorySize {
		h.entries = h.entries[len(h.entries)-maxHistorySize:]
	}
	content := strings.Join(h.entries, "\n") + "\n"
	return os.WriteFile(h.file, []byte(content), 0644)
}

// Add appends a new entry to the history. Empty entries and exact
// duplicates of the last entry are skipped.
func (h *InputHistory) Add(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	// Skip exact duplicate of last entry
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		h.index = -1
		return
	}
	h.entries = append(h.entries, entry)
	h.index = -1
}

// Prev returns the previous (older) history entry. Returns ("", false)
// if there are no more entries to navigate backward.
func (h *InputHistory) Prev() (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.index == -1 {
		// Start navigating from the most recent entry
		h.index = len(h.entries) - 1
	} else if h.index > 0 {
		h.index--
	}
	return h.entries[h.index], true
}

// Next returns the next (newer) history entry. Returns ("", false)
// if we've navigated past the newest entry.
func (h *InputHistory) Next() (string, bool) {
	if h.index == -1 || len(h.entries) == 0 {
		return "", false
	}
	h.index++
	if h.index >= len(h.entries) {
		h.index = -1
		return "", false // past the end — return to empty input
	}
	return h.entries[h.index], true
}

// ResetNavigation resets the navigation index so the next Prev()
// starts from the most recent entry.
func (h *InputHistory) ResetNavigation() {
	h.index = -1
}
