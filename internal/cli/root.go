package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
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

	model := newTUIModel(a, sess)

	// Replay history if resuming
	if resumeID != "" || continueLast {
		model.replayHistory()
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithInput(&shiftEnterReader{r: os.Stdin}))
	_, err = p.Run()

	// Ensure modifyOtherKeys is disabled on exit (safety net).
	fmt.Fprint(os.Stdout, "\x1b[>4m")

	// Save session on exit
	if a.SessionStore != nil {
		sess.SetStatus(session.StatusIdle)
		_ = a.SessionStore.Save(context.Background(), sess)
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

// shiftEnterReader wraps an io.Reader (typically stdin) and translates the
// xterm modifyOtherKeys escape sequence for Shift+Enter (\x1b[13;2~) into a
// line-feed byte (\n / Ctrl+J), which bubbletea recognises as KeyCtrlJ.
//
// It also implements the term.File interface (Fd/Write/Close) so that
// bubbletea can put the underlying TTY into raw mode.
type shiftEnterReader struct {
	r   io.Reader
	buf []byte
}

// Fd returns the file descriptor of the underlying reader (if it is a
// *os.File). This satisfies the term.File interface required by bubbletea
// to enter raw mode.
func (s *shiftEnterReader) Fd() uintptr {
	if f, ok := s.r.(*os.File); ok {
		return f.Fd()
	}
	return 0
}

// Write delegates to the underlying reader if it supports writing.
func (s *shiftEnterReader) Write(p []byte) (int, error) {
	if w, ok := s.r.(io.Writer); ok {
		return w.Write(p)
	}
	return 0, fmt.Errorf("shiftEnterReader: underlying reader does not support Write")
}

// Close delegates to the underlying reader if it supports closing.
func (s *shiftEnterReader) Close() error {
	if c, ok := s.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// shiftEnterSeq is the CSI sequence emitted by terminals with modifyOtherKeys
// level 2 when Shift+Enter is pressed.
var shiftEnterSeq = []byte("\x1b[13;2~")

func (s *shiftEnterReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.buf = append(s.buf, p[:n]...)
	}
	if len(s.buf) == 0 {
		return 0, err
	}
	copyN := len(s.buf)
	if copyN > len(p) {
		copyN = len(p)
	}
	copy(p, s.buf[:copyN])
	s.buf = s.buf[copyN:]

	// Hold back trailing bytes that could be a partial prefix of the
	// Shift+Enter sequence. Without this, a 7-byte sequence split across
	// two reads would pass through untranslated.
	for prefixLen := len(shiftEnterSeq) - 1; prefixLen > 0; prefixLen-- {
		if copyN >= prefixLen && bytes.Equal(p[copyN-prefixLen:copyN], shiftEnterSeq[:prefixLen]) {
			s.buf = append(s.buf, p[copyN-prefixLen:copyN]...)
			copyN -= prefixLen
			break
		}
	}

	// Translate Shift+Enter sequence → LF (Ctrl+J).
	for {
		idx := bytes.Index(p[:copyN], shiftEnterSeq)
		if idx == -1 {
			break
		}
		// Replace the 7-byte sequence with a single LF byte.
		p[idx] = '\n'
		tail := copy(p[idx+1:], p[idx+len(shiftEnterSeq):copyN])
		copyN = idx + 1 + tail
	}
	return copyN, err
}
