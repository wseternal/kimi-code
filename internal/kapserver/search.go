package kapserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)


// SearchService provides code search with snippet extraction.
type SearchService struct {
	workDir string
}

// NewSearchService creates a search service for a working directory.
func NewSearchService(workDir string) *SearchService {
	return &SearchService{workDir: workDir}
}

// SearchQuery describes a search request.
type SearchQuery struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Glob    string `json:"glob,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// SearchHit is a single search result with snippet.
type SearchHit struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Content   string `json:"content"`
	Snippet   string `json:"snippet,omitempty"`
}

// SearchResponse is the search result.
type SearchResponse struct {
	Hits       []SearchHit `json:"hits"`
	TotalCount int         `json:"total_count"`
}

// Search performs a ripgrep search and returns hits with snippets.
func (s *SearchService) Search(ctx context.Context, q SearchQuery) (*SearchResponse, error) {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	searchPath := q.Path
	if searchPath == "" {
		searchPath = s.workDir
	}
	// Reuse the shared path validation helper (abs + symlink containment).
	resolvedSearch, _, ok := resolveAndValidatePath(s.workDir, searchPath)
	if !ok {
		return nil, fmt.Errorf("search path %q is outside working directory", searchPath)
	}
	// Use the resolved path for rg to prevent symlink-based path traversal.

	args := []string{
		"--json", "--hidden",
		"--glob", "!.git",
		"--glob", "!node_modules",
		"-m", "1", // one match per file for initial listing
	}
	if q.Glob != "" {
		args = append(args, "--glob", q.Glob)
	}
	// W4 fix: insert "--" sentinel to prevent argument injection.
	args = append(args, "--", q.Pattern, resolvedSearch)

	// W2 fix: apply context with timeout to prevent hanging.
	searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(searchCtx, "rg", args...)
	out, err := cmd.Output()
	if err != nil {
		// rg returns exit 1 for no matches
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &SearchResponse{}, nil
		}
		return nil, err
	}

	hits := parseRgHits(string(out), q.Limit)

	// N5 fix: cache file reads to avoid reading same file N times for N hits.
	fileCache := make(map[string][]string)
	for i := range hits {
		hits[i].Snippet = extractSnippetCached(fileCache, hits[i].Path, hits[i].Line)
	}

	return &SearchResponse{
		Hits:       hits,
		TotalCount: len(hits),
	}, nil
}

func parseRgHits(output string, limit int) []SearchHit {
	var hits []SearchHit
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" || len(hits) >= limit {
			break
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
			hits = append(hits, SearchHit{
				Path:    msg.Data.Path.Text,
				Line:    msg.Data.LineNumber,
				Content: strings.TrimRight(msg.Data.Lines.Text, "\n"),
			})
		}
	}
	return hits
}

// extractSnippet returns a few lines of context around the match line.
func extractSnippet(path string, line int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	return joinSnippetLines(lines, line)
}

// extractSnippetCached uses a file-line cache to avoid re-reading the same file (N5 fix).
func extractSnippetCached(cache map[string][]string, path string, line int) string {
	lines, ok := cache[path]
	if !ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		lines = strings.Split(string(data), "\n")
		cache[path] = lines
	}
	return joinSnippetLines(lines, line)
}

func joinSnippetLines(lines []string, line int) string {
	start := line - 3
	if start < 0 {
		start = 0
	}
	end := line + 2
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

// SearchByGlob finds files matching a pattern and returns their info.
func (s *SearchService) SearchByGlob(pattern string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	// Reject patterns containing path traversal sequences.
	if strings.Contains(pattern, "..") {
		return nil, fmt.Errorf("glob pattern %q contains path traversal", pattern)
	}
	matches, err := filepath.Glob(filepath.Join(s.workDir, pattern))
	if err != nil {
		return nil, err
	}
	// Validate all matches are within the working directory.
	cleanWorkdir := filepath.Clean(s.workDir)
	var safe []string
	for _, m := range matches {
		clean := filepath.Clean(m)
		if clean == cleanWorkdir || strings.HasPrefix(clean, cleanWorkdir+string(filepath.Separator)) {
			safe = append(safe, m)
		}
	}
	if len(safe) > limit {
		safe = safe[:limit]
	}
	return safe, nil
}
