package protocol

import "errors"

// CursorQuery is the cursor-based pagination query parameters.
// before_id and after_id are mutually exclusive.
type CursorQuery struct {
	BeforeID string `json:"before_id,omitempty"`
	AfterID  string `json:"after_id,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// Validate checks mutual exclusivity and range constraints.
func (q CursorQuery) Validate() error {
	if q.BeforeID != "" && q.AfterID != "" {
		return errors.New("before_id and after_id are mutually exclusive")
	}
	if q.PageSize < 0 || q.PageSize > 100 {
		return errors.New("page_size must be between 1 and 100")
	}
	return nil
}

// PageResponse is a generic paginated response wrapper.
type PageResponse[T any] struct {
	Items   []T  `json:"items"`
	HasMore bool `json:"has_more"`
}
