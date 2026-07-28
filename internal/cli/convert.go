package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/visdomtech/kimi-code/internal/audit"
)

// parseConvert parses flags for the convert subcommand.
// Usage: kimi convert -s <session-id> -o <output.duckdb>
func (a *App) parseConvert(args []string) error {
	var sessionID, outputPath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-s", "--session":
			if i+1 < len(args) {
				sessionID = args[i+1]
				i++
			} else {
				return fmt.Errorf("convert: -s requires a session ID")
			}
		case "-o", "--output":
			if i+1 < len(args) {
				outputPath = args[i+1]
				i++
			} else {
				return fmt.Errorf("convert: -o requires an output path")
			}
		default:
			return fmt.Errorf("convert: unknown flag %s", args[i])
		}
	}

	if sessionID == "" {
		return fmt.Errorf("convert: -s <session-id> is required")
	}
	if outputPath == "" {
		return fmt.Errorf("convert: -o <output-path> is required")
	}

	return a.runConvert(sessionID, outputPath)
}

// runConvert converts a session's BadgerDB audit trail to a DuckDB file.
// Usage: kimi convert -s <session-id> -o <output.duckdb>
func (a *App) runConvert(sessionID, outputPath string) error {
	home := homeDir()
	if home == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	// Open the session's BadgerDB audit store.
	badgerDir := filepath.Join(sessionsDir(home), sessionID, "badger")
	store, err := audit.Open(badgerDir)
	if err != nil {
		return fmt.Errorf("open audit store for session %s: %w", sessionID, err)
	}
	defer store.Close()

	reader := audit.NewReader(store.DB())

	// Read session metadata.
	rec, err := reader.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("read session metadata: %w", err)
	}

	// Read all events.
	events, err := reader.ReadEvents(sessionID)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}

	// Open DuckDB output file.
	db, err := sql.Open("duckdb", outputPath)
	if err != nil {
		return fmt.Errorf("open duckdb: %w", err)
	}
	defer db.Close()

	// Create sessions table and insert metadata.
	if err := createSessionsTable(db, rec); err != nil {
		return fmt.Errorf("create sessions table: %w", err)
	}

	// Create events table and insert all events.
	if err := createEventsTable(db, sessionID, events); err != nil {
		return fmt.Errorf("create events table: %w", err)
	}

	fmt.Printf("Converted session %s (%d events) → %s\n", sessionID, len(events), outputPath)
	return nil
}

// createSessionsTable creates the sessions table in DuckDB and inserts the record.
func createSessionsTable(db *sql.DB, rec *audit.SessionRecord) error {
	_, err := db.Exec(`
		CREATE TABLE sessions (
			id          VARCHAR PRIMARY KEY,
			title       VARCHAR,
			status      VARCHAR,
			created_at  TIMESTAMP,
			updated_at  TIMESTAMP,
			metadata    VARCHAR
		)
	`)
	if err != nil {
		return err
	}

	var metaJSON string
	if rec.Metadata != nil {
		b, _ := json.Marshal(rec.Metadata)
		metaJSON = string(b)
	}

	_, err = db.Exec(
		`INSERT INTO sessions (id, title, status, created_at, updated_at, metadata) VALUES (?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Title, rec.Status, rec.CreatedAt, rec.UpdatedAt, metaJSON,
	)
	return err
}

// createEventsTable creates the events table in DuckDB and inserts all events.
func createEventsTable(db *sql.DB, sessionID string, events []audit.StoredEvent) error {
	_, err := db.Exec(`
		CREATE TABLE events (
			session_id  VARCHAR,
			type        VARCHAR,
			ts          TIMESTAMP,
			seq         UBIGINT,
			data        VARCHAR
		)
	`)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO events (session_id, type, ts, seq, data) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, evt := range events {
		ts := time.Unix(0, evt.Ts)
		dataStr := ""
		if evt.Data != nil {
			dataStr = string(evt.Data)
		}
		if _, err := stmt.Exec(sessionID, evt.Type, ts, evt.Seq, dataStr); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert event %s seq %d: %w", evt.Type, evt.Seq, err)
		}
	}

	return tx.Commit()
}

// sessionInfo holds session metadata for listing.
type sessionInfo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// runSessions lists all sessions from the file-based session store.
// Usage: kimi sessions
func (a *App) runSessions() error {
	home := homeDir()
	if home == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	dir := sessionsDir(home)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No sessions found.")
			return nil
		}
		return fmt.Errorf("read sessions dir: %w", err)
	}

	var sessions []sessionInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(dir, entry.Name(), "session.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue // skip sessions without metadata
		}
		var s sessionInfo
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	// Sort by UpdatedAt descending (newest first).
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tCREATED\tUPDATED")
	for _, s := range sessions {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.ID,
			truncate(s.Title, 40),
			s.Status,
			s.CreatedAt.Format("2006-01-02 15:04"),
			s.UpdatedAt.Format("2006-01-02 15:04"),
		)
	}
	w.Flush()
	return nil
}

// truncate shortens a string to maxLen, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}
