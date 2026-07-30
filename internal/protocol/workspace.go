// Package protocol provides workspace wire types (Gap #75).
package protocol

import "time"

// WorkspaceIDRegex is the pattern for valid workspace IDs.
const WorkspaceIDRegex = `^[a-zA-Z0-9_-]+$`

// Workspace represents a workspace.
type Workspace struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	IsActive    bool              `json:"is_active"`
}

// WorkspaceCreate is a request to create a new workspace.
type WorkspaceCreate struct {
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// WorkspaceUpdate is a request to update a workspace.
type WorkspaceUpdate struct {
	ID       string            `json:"id"`
	Name     string            `json:"name,omitempty"`
	Path     string            `json:"path,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// WorkspaceList is a response listing workspaces.
type WorkspaceList struct {
	Workspaces []Workspace `json:"workspaces"`
	Total      int         `json:"total"`
}

// WorkspaceEvent is a workspace lifecycle event.
type WorkspaceEvent struct {
	Type      string    `json:"type"` // "created", "updated", "deleted", "activated"
	Workspace Workspace `json:"workspace"`
	Timestamp time.Time `json:"timestamp"`
}
