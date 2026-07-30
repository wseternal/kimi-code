package protocol

import "encoding/json"

// ToolInputDisplay is a discriminated union for rich TUI rendering of tool call inputs.
// The Variant field selects the rendering shape.
type ToolInputDisplay struct {
	Variant ToolInputVariant `json:"variant"`

	// Common fields
	ToolName string `json:"tool_name"`

	// Bash variant
	Command   string `json:"command,omitempty"`
	Timeout   int    `json:"timeout,omitempty"`

	// File variant (Read, Write, Edit)
	Path      string `json:"path,omitempty"`
	StartLine *int   `json:"start_line,omitempty"`
	EndLine   *int   `json:"end_line,omitempty"`
	Content   string `json:"content,omitempty"`
	OldString string `json:"old_string,omitempty"`
	NewString string `json:"new_string,omitempty"`

	// Glob variant
	Pattern   string `json:"pattern,omitempty"`

	// Grep variant
	SearchPattern string `json:"search_pattern,omitempty"`
	SearchPath    string `json:"search_path,omitempty"`
	SearchGlob    string `json:"search_glob,omitempty"`

	// FetchURL variant
	URL string `json:"url,omitempty"`

	// WebSearch variant
	Query string `json:"query,omitempty"`

	// Generic/unknown variant
	RawInput json.RawMessage `json:"raw_input,omitempty"`
}

// ToolInputVariant identifies the display shape for a tool input.
type ToolInputVariant string

const (
	ToolInputBash      ToolInputVariant = "bash"
	ToolInputRead      ToolInputVariant = "read"
	ToolInputWrite     ToolInputVariant = "write"
	ToolInputEdit      ToolInputVariant = "edit"
	ToolInputGlob      ToolInputVariant = "glob"
	ToolInputGrep      ToolInputVariant = "grep"
	ToolInputFetchURL  ToolInputVariant = "fetch_url"
	ToolInputWebSearch ToolInputVariant = "web_search"
	ToolInputReadMedia ToolInputVariant = "read_media"
	ToolInputAskUser   ToolInputVariant = "ask_user"
	ToolInputUpdatePlan ToolInputVariant = "update_plan"
	ToolInputGeneric   ToolInputVariant = "generic"
)

// ToolResultDisplay is a discriminated union for rich TUI rendering of tool results.
type ToolResultDisplay struct {
	Variant  ToolResultVariant `json:"variant"`
	ToolName string            `json:"tool_name"`

	// Common
	Output   string `json:"output,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
	Truncated bool  `json:"truncated,omitempty"`

	// Bash result
	ExitCode  int    `json:"exit_code,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`

	// File result
	Path      string `json:"path,omitempty"`
	LineCount int    `json:"line_count,omitempty"`
	CharCount int    `json:"char_count,omitempty"`

	// Search result (Grep/Glob)
	MatchCount int    `json:"match_count,omitempty"`

	// Edit result
	EditApplied bool   `json:"edit_applied,omitempty"`

	// Media result
	MediaType string `json:"media_type,omitempty"`
	FileSize  int    `json:"file_size,omitempty"`

	// Generic
	RawOutput json.RawMessage `json:"raw_output,omitempty"`
}

// ToolResultVariant identifies the display shape for a tool result.
type ToolResultVariant string

const (
	ToolResultBash      ToolResultVariant = "bash"
	ToolResultRead      ToolResultVariant = "read"
	ToolResultWrite     ToolResultVariant = "write"
	ToolResultEdit      ToolResultVariant = "edit"
	ToolResultGlob      ToolResultVariant = "glob"
	ToolResultGrep      ToolResultVariant = "grep"
	ToolResultFetchURL  ToolResultVariant = "fetch_url"
	ToolResultWebSearch ToolResultVariant = "web_search"
	ToolResultReadMedia ToolResultVariant = "read_media"
	ToolResultAskUser   ToolResultVariant = "ask_user"
	ToolResultUpdatePlan ToolResultVariant = "update_plan"
	ToolResultGeneric   ToolResultVariant = "generic"
)
