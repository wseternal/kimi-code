package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/hooks"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/mcp"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/permission"
	promptpkg "github.com/visdomtech/kimi-code/internal/agentcore/agent/prompt"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/profile"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/skill"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/tools"
	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/di"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/audit"
	"github.com/visdomtech/kimi-code/internal/kapserver"
	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/kosong/providers"
	"github.com/visdomtech/kimi-code/internal/persistence"
	"github.com/visdomtech/kimi-code/internal/trace"
)

// CLIOptions holds all parsed CLI flags.
type CLIOptions struct {
	ResumeID     string
	ContinueLast bool
	Prompt       string
	OutputFormat string // "text" or "stream-json"
	Model        string
	Yolo         bool
	Auto         bool
	Plan         bool
	AddDirs      []string
	TracePath    string
	Agent        string // named agent profile
	AgentFile    string // path to agent profile file
}

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
		return a.runTUI(CLIOptions{})
	}

	// Parse flags and subcommands
	opts := CLIOptions{}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-S" || arg == "-r" || arg == "--resume":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				opts.ResumeID = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--resume requires a session ID argument")
			}
		case arg == "-c" || arg == "--continue":
			opts.ContinueLast = true
			i++
		case arg == "-p" || arg == "--prompt":
			if i+1 < len(args) {
				opts.Prompt = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--prompt requires an argument")
			}
		case arg == "--output-format":
			if i+1 < len(args) {
				opts.OutputFormat = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--output-format requires an argument (text or stream-json)")
			}
		case arg == "-m" || arg == "--model":
			if i+1 < len(args) {
				opts.Model = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--model requires a model name argument")
			}
		case arg == "-y" || arg == "--yolo" || arg == "--yes":
			opts.Yolo = true
			i++
		case arg == "--auto" || arg == "--auto-approve":
			opts.Auto = true
			i++
		case arg == "--plan":
			opts.Plan = true
			i++
		case arg == "--add-dir":
			if i+1 < len(args) {
				opts.AddDirs = append(opts.AddDirs, args[i+1])
				i += 2
			} else {
				return fmt.Errorf("--add-dir requires a directory argument")
			}
		case arg == "--trace":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				opts.TracePath = args[i+1]
				i += 2
			} else {
				opts.TracePath = filepath.Join(home, ".kimi-code", fmt.Sprintf("trace_%d.jsonl", time.Now().Unix()))
				i++
			}
		case arg == "--agent":
			if i+1 < len(args) {
				opts.Agent = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--agent requires a profile name argument")
			}
		case arg == "--agent-file":
			if i+1 < len(args) {
				opts.AgentFile = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("--agent-file requires a file path argument")
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
		case arg == "convert":
			return a.parseConvert(args[i+1:])
		case arg == "sessions":
			return a.runSessions()
		default:
			if len(arg) > 0 && arg[0] != '-' {
				opts.Prompt = arg
				i++
			} else {
				return fmt.Errorf("unknown flag: %s", arg)
			}
		}
	}

	// Enable tracing if requested
	if opts.TracePath != "" {
		if err := trace.Enable(opts.TracePath); err != nil {
			return fmt.Errorf("failed to enable trace: %w", err)
		}
		defer trace.Disable()
		fmt.Fprintf(os.Stderr, "Trace enabled: %s\n", opts.TracePath)
	}

	// Apply model override to config
	if opts.Model != "" {
		a.Config.DefaultModel = opts.Model
	}

	// Determine mode
	if opts.Prompt != "" {
		return a.runHeadless(opts)
	}

	return a.runTUI(opts)
}

func (a *App) printHelp() error {
	fmt.Println("kimi-code \u2014 AI-powered coding assistant")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  kimi                          Start interactive TUI")
	fmt.Println("  kimi -c                       Continue last session")
	fmt.Println("  kimi -S <session-id>          Resume a specific session")
	fmt.Println("  kimi -p <prompt>              Run headless (non-interactive)")
	fmt.Println("  kimi -m, --model <model>      Override LLM model for this invocation")
	fmt.Println("  kimi -y, --yolo               Auto-approve regular tool calls")
	fmt.Println("  kimi --auto                   Start in fully autonomous auto mode")
	fmt.Println("  kimi --plan                   Start in plan mode")
	fmt.Println("  kimi --output-format <fmt>    Headless output format: text, stream-json")
	fmt.Println("  kimi --add-dir <dir>          Add additional workspace directory (repeatable)")
	fmt.Println("  kimi server                   Start HTTP server")
	fmt.Println("  kimi doctor                   Run diagnostics")
	fmt.Println("  kimi login                    Set up API key")
	fmt.Println("  kimi export <session-id>      Export session as markdown")
	fmt.Println("  kimi sessions                 List all sessions")
	fmt.Println("  kimi convert -s <id> -o <file> Convert session audit to DuckDB")
	fmt.Println("  kimi version                  Show version")
	fmt.Println("  kimi --trace [file]           Enable event tracing to JSONL file")
	fmt.Println("  kimi --agent <name>           Use a named agent profile")
	fmt.Println("  kimi --agent-file <path>      Load agent profile from file")
	fmt.Println("  kimi help                     Show this help")
	return nil
}

func (a *App) runServer() error {
	srv := kapserver.NewServer(
		kapserver.Config{Host: a.Config.Server.Host, Port: a.Config.Server.Port},
		a.SessionManager,
		nil,
		kapserver.WithConfigProvider(kapserver.NewConfigProvider(a.Config)),
	)
	fmt.Printf("Starting server on %s\n", srv.Addr())
	return srv.Start(nil)
}

func (a *App) runTUI(opts CLIOptions) error {
	var sess *session.Session
	var err error

	// Try to resume or continue
	if opts.ResumeID != "" {
		sess, err = a.SessionManager.Resume(opts.ResumeID)
		if err != nil {
			return fmt.Errorf("failed to resume session %s: %w", opts.ResumeID, err)
		}
	} else if opts.ContinueLast {
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

	// Apply CLI options to TUI model
	if opts.Yolo {
		model.permChain = permission.YoloChain()
	}
	if opts.Auto {
		model.permChain = permission.AutoChain()
	}
	if opts.Plan {
		model.planMode = true
	}

	// Replay history if resuming
	if opts.ResumeID != "" || opts.ContinueLast {
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
		if err := a.AuditWriter.SaveSession(audit.SessionRecord{
			ID:        sess.ID,
			Title:     sess.Title,
			Status:    string(sess.Status),
			CreatedAt: sess.CreatedAt,
			UpdatedAt: time.Now(),
			Metadata:  sess.Metadata,
		}); err != nil {
			slog.Debug("audit save session on exit", "error", err)
		}
	}

	return err
}

func (a *App) runHeadless(opts CLIOptions) error {
	cwd, _ := os.Getwd()

	// Load agent profile if specified
	agentProfile, err := a.loadAgentProfile(opts)
	if err != nil {
		return err
	}

	// Resolve provider from config
	if !providers.IsConfigured(a.Config) {
		return fmt.Errorf("no provider configured. Add to ~/.kimi-code/config.toml or run 'kimi login'")
	}

	// Apply profile model override
	if agentProfile != nil && agentProfile.Model != "" {
		a.Config.DefaultModel = agentProfile.Model
	}

	provider, err := providers.NewFromConfig(a.Config)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Build tool registry
	toolReg := tools.NewRegistry()
	tools.RegisterDefaultTools(toolReg)
	tools.RegisterBackgroundTools(toolReg, nil)

	// Connect MCP servers
	var mcpMgr *mcp.Manager
	if len(a.Config.McpServers) > 0 {
		mcpMgr = mcp.NewManager(slog.Default())
		if count, err := mcpMgr.ConnectAll(context.Background(), a.Config.McpServers, toolReg); err != nil {
			slog.Warn("mcp: some servers failed to connect", "connected", count, "error", err)
		} else if count > 0 {
			slog.Info("mcp: servers connected", "tools", count)
		}
	}
	defer func() {
		if mcpMgr != nil {
			mcpMgr.DisconnectAll()
		}
	}()

	// Permission chain
	var permChain *permission.Chain
	switch {
	case opts.Auto:
		permChain = permission.AutoChain()
	case opts.Yolo:
		permChain = permission.YoloChain()
	default:
		permChain = permission.DefaultChain()
	}

	// Hook engine (from config)
	var hookEng *hooks.Engine
	if len(a.Config.Hooks) > 0 {
		hookEng = hooks.NewEngine(a.Config.Hooks, nil)
	}

	// Discover skills
	var skillCat *skill.Catalog
	if cat, err := skill.Discover(cwd); err == nil {
		skillCat = cat
		toolReg.Register(tools.NewSkillTool(skillCat, nil))
	}

	// Build system prompt
	homeDir, _ := os.UserHomeDir()
	agentsMd, _ := promptpkg.LoadAgentsMd(cwd, homeDir)
	systemPrompt := buildSystemPrompt(cwd, getGitBranch(skill.FindProjectRoot(cwd)), skillCat, nil, agentsMd, "")

	// Apply agent profile to system prompt
	if agentProfile != nil {
		systemPrompt = agentProfile.ApplyToSystemPrompt(systemPrompt)
		if agentProfile.PlanMode {
			systemPrompt += "\n\nYou are in plan mode. Focus on planning before implementation."
		}
	}

	// Determine output format
	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		outputFormat = "text"
	}

	// Convert tool definitions
	var kosongTools []kosong.Tool
	for _, def := range toolReg.Definitions() {
		kosongTools = append(kosongTools, kosong.Tool{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var history []kosong.Message
	history = append(history, kosong.CreateUserMessage(opts.Prompt))

	maxSteps := 25
	for step := 0; step < maxSteps; step++ {
		stream, err := provider.Generate(ctx, systemPrompt, kosongTools, history, nil)
		if err != nil {
			if outputFormat == "stream-json" {
				emitJSON(os.Stdout, "error", map[string]string{"message": err.Error()})
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			}
			return err
		}

		msg, err := kosong.Generate(ctx, stream)
		if err != nil {
			if outputFormat == "stream-json" {
				emitJSON(os.Stdout, "error", map[string]string{"message": err.Error()})
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			}
			return err
		}

		// Output text
		for _, part := range msg.Content {
			if part.Type == "text" {
				if outputFormat == "stream-json" {
					emitJSON(os.Stdout, "text", map[string]string{"content": part.Text})
				} else {
					fmt.Print(part.Text)
				}
			}
		}

		if len(msg.ToolCalls) == 0 {
			history = append(history, *msg)
			break
		}

		history = append(history, *msg)

		// Execute tool calls
		for _, tc := range msg.ToolCalls {
			tool, ok := toolReg.Get(tc.Name)
			if !ok {
				history = append(history, kosong.CreateToolMessage(tc.ID, fmt.Sprintf("tool %q not found", tc.Name)))
				continue
			}

			var toolInput json.RawMessage
			if tc.Arguments != nil {
				toolInput = json.RawMessage(*tc.Arguments)
			} else {
				toolInput = json.RawMessage("{}")
			}

			// Permission check
			permResult := permChain.Evaluate(tc.Name, toolInput)
			if permResult.Decision == permission.DecisionDeny {
				history = append(history, kosong.CreateToolMessage(tc.ID, fmt.Sprintf("Permission denied: %s", permResult.Reason)))
				continue
			}

			// PreToolUse hook
			if hookEng != nil {
				hookInput := hooks.HookInput{
					Tool:    &hooks.HookToolInput{Name: tc.Name, Input: string(toolInput)},
					Session: &hooks.HookSession{WorkDir: cwd},
				}
				decision := hookEng.TriggerBlock(ctx, hooks.PreToolUse, hookInput)
				if decision.Blocked {
					history = append(history, kosong.CreateToolMessage(tc.ID, fmt.Sprintf("Blocked by hook: %s", decision.Reason)))
					continue
				}
			}

			result, execErr := tool.Execute(ctx, toolInput, tools.ExecContext{WorkDir: cwd})
			if execErr != nil {
				result = &tools.Result{Output: execErr.Error(), IsError: true}
			}

			if outputFormat == "stream-json" {
				emitJSON(os.Stdout, "tool_call", map[string]any{"name": tc.Name, "result": result.Output, "is_error": result.IsError})
			}

			history = append(history, kosong.CreateToolMessage(tc.ID, result.Output))
		}
	}

	if outputFormat == "stream-json" {
		emitJSON(os.Stdout, "done", nil)
	} else {
		fmt.Println()
	}
	return nil
}

// emitJSON writes a newline-delimited JSON event to the writer.
func emitJSON(w *os.File, eventType string, data any) {
	event := map[string]any{"type": eventType}
	if data != nil {
		event["data"] = data
	}
	b, _ := json.Marshal(event)
	fmt.Fprintln(w, string(b))
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
	if err := os.MkdirAll(dir, 0700); err != nil {
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

// loadAgentProfile loads an agent profile from CLI flags.
// Returns nil if no profile is specified. Returns an error if the
// specified profile cannot be loaded.
func (a *App) loadAgentProfile(opts CLIOptions) (*profile.AgentProfile, error) {
	if opts.AgentFile != "" {
		p, err := profile.Load(opts.AgentFile)
		if err != nil {
			return nil, fmt.Errorf("load agent profile from %s: %w", opts.AgentFile, err)
		}
		slog.Info("loaded agent profile", "name", p.Name, "source", opts.AgentFile)
		return p, nil
	}
	if opts.Agent != "" {
		home, _ := os.UserHomeDir()
		p, err := profile.LoadNamed(opts.Agent, home)
		if err != nil {
			return nil, fmt.Errorf("load agent profile %q: %w", opts.Agent, err)
		}
		slog.Info("loaded agent profile", "name", p.Name)
		return p, nil
	}
	return nil, nil
}
