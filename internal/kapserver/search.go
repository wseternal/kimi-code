package kapserver

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
func (s *SearchService) Search(q SearchQuery) (*SearchResponse, error) {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	searchPath := q.Path
	if searchPath == "" {
		searchPath = s.workDir
	}

	args := []string{
		"--json", "--hidden",
		"--glob", "!.git",
		"--glob", "!node_modules",
		"-m", "1", // one match per file for initial listing
	}
	if q.Glob != "" {
		args = append(args, "--glob", q.Glob)
	}
	args = append(args, q.Pattern, searchPath)

	cmd := exec.Command("rg", args...)
	out, err := cmd.Output()
	if err != nil {
		// rg returns exit 1 for no matches
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &SearchResponse{}, nil
		}
		return nil, err
	}

	hits := parseRgHits(string(out), q.Limit)

	// Extract snippets for each hit
	for i := range hits {
		hits[i].Snippet = extractSnippet(hits[i].Path, hits[i].Line)
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
	matches, err := filepath.Glob(filepath.Join(s.workDir, pattern))
	if err != nil {
		return nil, err
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}
