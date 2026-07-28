package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/audit"
	"github.com/visdomtech/kimi-code/internal/trace"
	"github.com/visdomtech/kimi-code/internal/kapserver"
	"github.com/visdomtech/kimi-code/internal/persistence"
)

// App holds the application state.
type App struct {
	Config         *config.Config
	AppScope       *di.Scope
	SessionManager *session.Manager
	SessionStore   *session.SessionStore
	ConfigPath     string

	// Audit trail (BadgerDB-backed)
	AuditStore   *audit.Store
	AuditWriter  *audit.Writer
	AuditFacade  *audit.Facade
}

// Execute runs the root command.
func Execute() error {
	app := &App{}
	return app.run()
}

func (a *App) run() error {
	home := homeDir()

	// Load config
	cfg := config.DefaultConfig()
	configPath := config.ConfigPath(home)
	a.ConfigPath = configPath
	if loaded, err := config.LoadFromFile(configPath); err == nil {
		cfg = loaded
	}
	a.Config = cfg

	// Create app scope
	a.AppScope = di.NewAppScope("kimi-app")
	a.AppScope.Register("config", cfg)

	// Create session manager
	a.SessionManager = session.NewManager(a.AppScope)
	a.AppScope.Register("sessionManager", a.SessionManager)

	// Set up persistent session store
	sessDir := sessionsDir(home)
	if fileStore, err := persistence.NewFileStore(sessDir); err == nil {
		store := session.NewSessionStore(fileStore, a.AppScope)
		a.SessionStore = store
		a.SessionManager.SetStore(store)
		a.AppScope.Register("sessionStore", store)
	}

	// Parse args
	args := os.Args[1:]

	if len(args) == 0 {
		return a.runTUI("", false)
	}

	// Parse flags and subcommands
	var resumeID string
	var continueLast bool
	var promptArg string
	var tracePath string

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-S" || arg == "-r" || arg == "--resume":
			if i+1 < len(args) {
				resumeID = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--resume requires a session ID argument")
			}
		case arg == "-c" || arg == "--continue":
			continueLast = true
			i++
		case arg == "-p" || arg == "--prompt":
			if i+1 < len(args) {
				promptArg = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--prompt requires an argument")
			}
		case arg == "--trace":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				tracePath = args[i+1]
				i += 2
			} else {
				// Default trace path
				tracePath = filepath.Join(home, ".kimi-code", fmt.Sprintf("trace_%d.jsonl", time.Now().Unix()))
				i++
			}
		case arg == "server":
			return a.runServer()
		case arg == "version" || arg == "--version" || arg == "-v":
			fmt.Println("kimi-code " + BuildVersion())
			return nil
		case arg == "help" || arg == "--help" || arg == "-h":
			return a.printHelp()
		case arg == "doctor":
			return a.runDoctor()
		case arg == "login":
			return authWizard(a.Config, a.ConfigPath)
		case arg == "export":
			if i+1 < len(args) {
				return ExportSession(a.SessionStore, args[i+1], "")
			}
			return fmt.Errorf("export requires a session ID")
		default:
			if arg[0] != '-' {
				promptArg = arg
				i++
			} else {
				return fmt.Errorf("unknown flag: %s", arg)
			}
		}
	}

	// Enable tracing if requested
	if tracePath != "" {
		if err := trace.Enable(tracePath); err != nil {
			return fmt.Errorf("failed to enable trace: %w", err)
		}
		defer trace.Disable()
		fmt.Fprintf(os.Stderr, "Trace enabled: %s\n", tracePath)
	}

	// Determine mode
	if promptArg != "" {
		return a.runHeadless(promptArg)
	}

	return a.runTUI(resumeID, continueLast)
}

func (a *App) printHelp() error {
	fmt.Println("kimi-code — AI-powered coding assistant")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  kimi                          Start interactive TUI")
	fmt.Println("  kimi -c                       Continue last session")
	fmt.Println("  kimi -S <session-id>          Resume a specific session")
	fmt.Println("  kimi -p <prompt>              Run headless (non-interactive)")
	fmt.Println("  kimi server                   Start HTTP server")
	fmt.Println("  kimi doctor                   Run diagnostics")
	fmt.Println("  kimi login                    Set up API key")
	fmt.Println("  kimi export <session-id>      Export session as markdown")
	fmt.Println("  kimi version                  Show version")
	fmt.Println("  kimi --trace [file]           Enable event tracing to JSONL file")
	fmt.Println("  kimi help                     Show this help")
	return nil
}

func (a *App) runServer() error {
	srv := kapserver.NewServer(
		kapserver.Config{Host: a.Config.Server.Host, Port: a.Config.Server.Port},
		a.SessionManager,
		nil,
	)
	fmt.Printf("Starting server on %s\n", srv.Addr())
	return srv.Start(nil)
}

func (a *App) runTUI(resumeID string, continueLast bool) error {
	var sess *session.Session
	var err error

	// Try to resume or continue
	if resumeID != "" {
		sess, err = a.SessionManager.Resume(resumeID)
		if err != nil {
			return fmt.Errorf("failed to resume session %s: %w", resumeID, err)
		}
	} else if continueLast {
		sess, err = a.SessionManager.GetLatest()
		if err != nil {
			// Fall back to new session
			sess = nil
		}
	}

	if sess == nil {
		// Create a new session
		sessionID := fmt.Sprintf("session_%d", os.Getpid())
		sess, err = a.SessionManager.Create(context.Background(), sessionID, "Interactive Session")
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}
	sess.SetStatus(session.StatusIdle)

	// If stdin is not a terminal (piped), use simple fallback mode
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return a.runSimpleTUI(sess)
	}

	// Open audit store for the session
	home, _ := os.UserHomeDir()
	if home != "" {
		a.openAuditStore(sess.ID, home)
	}
	defer a.closeAuditStore()

	model := newTUIModel(a, sess)

	// Replay history if resuming
	if resumeID != "" || continueLast {
		model.replayHistory()
	}

	p := tea.NewProgram(model)
	_, err = p.Run()

	// Save session on exit
	if a.SessionStore != nil {
		sess.SetStatus(session.StatusIdle)
		_ = a.SessionStore.Save(context.Background(), sess)
		// Purge empty sessions (including current if user never sent messages)
		if err := a.SessionStore.PurgeEmptySessions(context.Background()); err != nil {
			slog.Debug("purge empty sessions", "error", err)
		}
	}
	// Also persist to audit trail on exit
	if a.AuditWriter != nil {
		a.AuditWriter.SaveSession(audit.SessionRecord{
			ID:        sess.ID,
			Title:     sess.Title,
			Status:    string(sess.Status),
			CreatedAt: sess.CreatedAt,
			UpdatedAt: time.Now(),
			Metadata:  sess.Metadata,
		})
	}

	return err
}

func (a *App) runHeadless(prompt string) error {
	fmt.Printf("Running headless with prompt: %s\n", prompt)
	// TODO: Wire to agent loop
	fmt.Println("Headless mode not yet fully wired (requires provider adapter)")
	return nil
}

func (a *App) runDoctor() error {
	results := RunDoctor(a.Config)
	fmt.Print(FormatDoctorResults(results))
	return nil
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// resolveSessionsDir returns the path to the sessions directory, creating it if needed.
func resolveSessionsDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kimi-code", "sessions")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// unused but available for future subcommand routing
var _ = strings.TrimSpace

// openAuditStore opens a per-session BadgerDB for the audit trail.
// Data is stored at ~/.kimi-code/sessions/{sessionID}/badger/.
func (a *App) openAuditStore(sessionID, home string) {
	dir := filepath.Join(sessionsDir(home), sessionID, "badger")
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Debug("create audit dir", "error", err)
		return
	}
	store, err := audit.Open(dir)
	if err != nil {
		slog.Debug("open audit store", "error", err)
		return
	}
	a.AuditStore = store
	w := audit.NewWriter(store.DB())
	a.AuditWriter = w
	reader := audit.NewReader(store.DB())
	a.AuditFacade = audit.NewFacade(reader)
}

// closeAuditStore closes the audit writer and BadgerDB store.
func (a *App) closeAuditStore() {
	if a.AuditWriter != nil {
		a.AuditWriter.Close()
		a.AuditWriter = nil
	}
	if a.AuditStore != nil {
		a.AuditStore.Close()
		a.AuditStore = nil
	}
	a.AuditFacade = nil
}

// switchAuditStore closes the current audit store and opens a new one
// for a different session (e.g. after /new or session resume).
func (a *App) switchAuditStore(sessionID, home string) {
	a.closeAuditStore()
	a.openAuditStore(sessionID, home)
}
