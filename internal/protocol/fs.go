// Package protocol provides FS browsing/search/git-status wire types (Gap #74).
package protocol

import "time"

// FsEntry represents a file system entry.
type FsEntry struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Type     string    `json:"type"` // "file", "directory", "symlink"
	Size     int64     `json:"size"`
	Mode     string    `json:"mode"`
	Modified time.Time `json:"modified"`
	IsDir    bool      `json:"is_dir"`
	IsHidden bool      `json:"is_hidden"`
}

// FsSearchHit represents a file search result.
type FsSearchHit struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Match   string `json:"match,omitempty"` // matched text
	Score   int    `json:"score,omitempty"` // relevance score
	IsDir   bool   `json:"is_dir"`
}

// FsGrepHit represents a single grep match within a line.
type FsGrepHit struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// FsGrepLine represents a matching line in a file.
type FsGrepLine struct {
	LineNumber int         `json:"line_number"`
	Content    string      `json:"content"`
	Hits       []FsGrepHit `json:"hits"`
}

// FsGrepFileHit represents grep results for a single file.
type FsGrepFileHit struct {
	Path  string       `json:"path"`
	Lines []FsGrepLine `json:"lines"`
}

// FsGitStatusEntry represents a git status entry.
type FsGitStatusEntry struct {
	Path      string `json:"path"`
	Status    string `json:"status"` // "modified", "added", "deleted", "untracked", "renamed"
	Staged    bool   `json:"staged"`
	Unstaged  bool   `json:"unstaged"`
	OldPath   string `json:"old_path,omitempty"` // for renames
}

// FsChangeEvent represents a file system change event.
type FsChangeEvent struct {
	Type   string    `json:"type"` // "create", "modify", "delete", "rename"
	Path   string    `json:"path"`
	OldPath string   `json:"old_path,omitempty"` // for renames
	Time   time.Time `json:"time"`
}

// FsBrowseRequest is a request to browse a directory.
type FsBrowseRequest struct {
	Path     string `json:"path"`
	Pattern  string `json:"pattern,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

// FsBrowseResponse is the response to a browse request.
type FsBrowseResponse struct {
	Entries []FsEntry `json:"entries"`
	Path    string    `json:"path"`
}

// FsSearchRequest is a request to search files.
type FsSearchRequest struct {
	Query   string `json:"query"`
	Path    string `json:"path,omitempty"`
	Pattern string `json:"pattern,omitempty"` // file glob filter
	Limit   int    `json:"limit,omitempty"`
}

// FsSearchResponse is the response to a search request.
type FsSearchResponse struct {
	Hits    []FsSearchHit `json:"hits"`
	Query   string        `json:"query"`
	Total   int           `json:"total"`
}

// FsGrepRequest is a request to grep files.
type FsGrepRequest struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Glob    string `json:"glob,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// FsGrepResponse is the response to a grep request.
type FsGrepResponse struct {
	Files   []FsGrepFileHit `json:"files"`
	Pattern string          `json:"pattern"`
	Total   int             `json:"total"`
}

// FsGitStatusResponse is the response to a git status request.
type FsGitStatusResponse struct {
	Entries   []FsGitStatusEntry `json:"entries"`
	Branch    string             `json:"branch"`
	IsClean   bool               `json:"is_clean"`
}
