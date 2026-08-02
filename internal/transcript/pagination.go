package transcript

// Page represents a paginated view of transcript turns.
type Page struct {
	Turns    []TranscriptTurn `json:"turns"`
	Frames   []Frame          `json:"frames"`
	HasMore  bool             `json:"hasMore"`
	Cursor   string           `json:"cursor,omitempty"` // opaque cursor for next page
	Total    int              `json:"total"`
	PageNum  int              `json:"pageNum"`
	PageSize int              `json:"pageSize"`
}

// Paginate extracts a page of turns from the snapshot.
// pageSize is the number of turns per page. pageNumber is 0-based.
// Returns turns in reverse chronological order (newest first).
func Paginate(state *Snapshot, pageNumber, pageSize int) Page {
	if pageSize <= 0 {
		pageSize = 20
	}
	if state == nil || len(state.Turns) == 0 {
		return Page{
			Turns:    nil,
			Frames:   nil,
			HasMore:  false,
			Total:    0,
			PageNum:  pageNumber,
			PageSize: pageSize,
		}
	}

	total := len(state.Turns)
	totalPages := (total + pageSize - 1) / pageSize
	if pageNumber >= totalPages {
		pageNumber = totalPages - 1
	}
	if pageNumber < 0 {
		pageNumber = 0
	}

	// Reverse chronological: newest turns first
	start := total - (pageNumber+1)*pageSize
	if start < 0 {
		start = 0
	}
	end := total - pageNumber*pageSize
	if end > total {
		end = total
	}

	turns := make([]TranscriptTurn, end-start)
	copy(turns, state.Turns[start:end])

	// Build frame index by StepID once, then collect frames for these turns.
	frameIndex := buildFrameIndex(state.Frames)
	var frames []Frame
	for _, turn := range turns {
		for _, step := range turn.Steps {
			frames = append(frames, frameIndex[step.ID]...)
		}
	}

	cursor := ""
	if pageNumber+1 < totalPages {
		cursor = string(NewTurnID(pageNumber + 1))
	}

	return Page{
		Turns:    turns,
		Frames:   frames,
		HasMore:  pageNumber+1 < totalPages,
		Cursor:   cursor,
		Total:    total,
		PageNum:  pageNumber,
		PageSize: pageSize,
	}
}

// buildFrameIndex indexes frames by StepID for O(1) lookup per step.
func buildFrameIndex(frames []Frame) map[StepID][]Frame {
	idx := make(map[StepID][]Frame, len(frames)/4)
	for _, f := range frames {
		idx[f.StepID] = append(idx[f.StepID], f)
	}
	return idx
}

// PaginateByCursor extracts turns after a given turn ID cursor.
// TODO(S4): Consider switching to true cursor semantics (opaque token → index)
// instead of using TurnID as cursor, for better forward-compatibility.
func PaginateByCursor(state *Snapshot, cursor TurnID, limit int) Page {
	if state == nil || len(state.Turns) == 0 || limit <= 0 {
		return Page{Turns: nil, PageSize: limit}
	}

	// Find the cursor position
	startIdx := 0
	if cursor != "" {
		for i, t := range state.Turns {
			if t.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(state.Turns) {
		endIdx = len(state.Turns)
	}

	turns := make([]TranscriptTurn, endIdx-startIdx)
	copy(turns, state.Turns[startIdx:endIdx])
	hasMore := endIdx < len(state.Turns)

	nextCursor := ""
	if hasMore && len(turns) > 0 {
		nextCursor = string(turns[len(turns)-1].ID)
	}

	return Page{
		Turns:    turns,
		HasMore:  hasMore,
		Cursor:   nextCursor,
		Total:    len(state.Turns),
		PageNum:  startIdx / limit,
		PageSize: limit,
	}
}
