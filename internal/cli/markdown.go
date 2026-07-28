package cli

import (
	"os"
	"sync"

	"github.com/charmbracelet/glamour"
	"charm.land/lipgloss/v2"
)

// markdownRenderer is a cached glamour renderer for a given theme.
// Creating glamour renderers is expensive, so we cache one per
// dark/light mode and word-wrap width.
type markdownRenderer struct {
	mu       sync.RWMutex
	dark     *glamour.TermRenderer
	light    *glamour.TermRenderer
	lastDark bool
	lastW    int
}

var mdRenderer markdownRenderer

// renderMarkdown renders markdown content with syntax highlighting and
// proper terminal styling. It falls back to plain text on error.
func renderMarkdown(content string, width int) string {
	if width <= 0 {
		width = 80
	}
	dark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	r := mdRenderer.get(dark, width)
	if r == nil {
		return content
	}

	out, err := r.Render(content)
	if err != nil {
		return content
	}
	return out
}

// get returns a cached glamour renderer for the given theme and width,
// creating one if necessary.
func (mr *markdownRenderer) get(dark bool, width int) *glamour.TermRenderer {
	mr.mu.RLock()
	if mr.lastDark == dark && mr.lastW == width {
		var r *glamour.TermRenderer
		if dark {
			r = mr.dark
		} else {
			r = mr.light
		}
		mr.mu.RUnlock()
		if r != nil {
			return r
		}
	} else {
		mr.mu.RUnlock()
	}

	mr.mu.Lock()
	defer mr.mu.Unlock()

	// Double-check after acquiring write lock
	if mr.lastDark == dark && mr.lastW == width {
		if dark && mr.dark != nil {
			return mr.dark
		}
		if !dark && mr.light != nil {
			return mr.light
		}
	}

	var styleOpt glamour.TermRendererOption
	if dark {
		styleOpt = glamour.WithStylePath("dark")
	} else {
		styleOpt = glamour.WithStylePath("light")
	}

	r, err := glamour.NewTermRenderer(
		styleOpt,
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}

	if dark {
		mr.dark = r
	} else {
		mr.light = r
	}
	mr.lastDark = dark
	mr.lastW = width

	return r
}
