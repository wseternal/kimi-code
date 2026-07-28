package audit

import (
	"encoding/json"
	"fmt"
)

// ResumeData contains everything needed to reconstruct TUI state
// when resuming a session (equivalent to what replayHistory() builds).
type ResumeData struct {
	Session SessionRecord
	Turns   []TurnRecord
}

// Facade provides high-level session operations that reconstruct
// state from the audit event stream. It is the bridge between the
// raw BadgerDB audit trail and the session resume API.
type Facade struct {
	reader *Reader
}

// NewFacade creates a Facade backed by the given Reader.
func NewFacade(r *Reader) *Facade {
	return &Facade{reader: r}
}

// Reader returns the underlying Reader for direct queries.
func (f *Facade) Reader() *Reader {
	return f.reader
}

// LoadSession reconstructs full session state from the audit trail.
// It reads turn.completed events to rebuild the turn history.
func (f *Facade) LoadSession(id string) (*ResumeData, error) {
	rec, err := f.reader.GetSession(id)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	events, err := f.reader.ReadEvents(id)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}

	var turns []TurnRecord
	for _, evt := range events {
		if evt.Type != EvtTurnCompleted {
			continue
		}
		var tr TurnRecord
		if err := json.Unmarshal(evt.Data, &tr); err != nil {
			continue // skip malformed turn records
		}
		turns = append(turns, tr)
	}

	return &ResumeData{
		Session: *rec,
		Turns:   turns,
	}, nil
}

// ListSessions returns all sessions sorted by UpdatedAt descending.
func (f *Facade) ListSessions() ([]SessionSummary, error) {
	return f.reader.ListSessions()
}

// GetLatest loads the most recently updated session.
// Returns nil, nil if no sessions exist.
func (f *Facade) GetLatest() (*ResumeData, error) {
	id, err := f.reader.GetLatestSessionID()
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	return f.LoadSession(id)
}
