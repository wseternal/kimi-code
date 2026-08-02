package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/background"
	agentctx "github.com/visdomtech/kimi-code/internal/agentcore/agent/context"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/cron"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/goal"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/hooks"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/injection"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/permission"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/plan"
	promptpkg "github.com/visdomtech/kimi-code/internal/agentcore/agent/prompt"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/skill"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/swarm"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/tools"
	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/audit"
	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/kosong/providers"
	"github.com/visdomtech/kimi-code/internal/oauth"
	"github.com/visdomtech/kimi-code/internal/trace"
)

// ── Theme (auto-detects light/dark terminal background) ──

type themeColors struct {
	primary     string
	accent      string
	text        string
	textStrong  string
	textDim     string
	textMuted   string
	border      string
	success     string
	warning     string
	error       string
	roleUser    string
	statusBarBg string
}

var darkTheme = themeColors{
	primary:     "#4FA8FF",
	accent:      "#5BC0BE",
	text:        "#E0E0E0",
	textStrong:  "#F5F5F5",
	textDim:     "#888888",
	textMuted:   "#6B6B6B",
	border:      "#5A5A5A",
	success:     "#4EC87E",
	warning:     "#E8A838",
	error:       "#E85454",
	roleUser:    "#FFCB6B",
	statusBarBg: "#1A1A2E",
}

var lightTheme = themeColors{
	primary:     "#1A5FB4",
	accent:      "#26A269",
	text:        "#3D3846",
	textStrong:  "#1E1E2E",
	textDim:     "#77767B",
	textMuted:   "#9A9A9A",
	border:      "#C0BFBC",
	success:     "#26A269",
	warning:     "#E5A50A",
	error:       "#C01C28",
	roleUser:    "#A2734C",
	statusBarBg: "#DEDDDA",
}

var (
	// Logo block (2 rows)
	logoLines = []string{
		"▐█▛█▛█▌",
		"▐█████▌",
	}
)

// ── Styles (initialized by initTheme) ──

var (
	primaryStyle      lipgloss.Style
	boldPrimary       lipgloss.Style
	dimStyle          lipgloss.Style
	textStyle         lipgloss.Style
	strongStyle       lipgloss.Style
	successStyle      lipgloss.Style
	warningStyle      lipgloss.Style
	userStyle         lipgloss.Style
	mutedStyle        lipgloss.Style
	borderStyle       lipgloss.Style
	inputBorderStyle  lipgloss.Style
	inputFocusedStyle lipgloss.Style
	statusBarStyle    lipgloss.Style
)

func init() {
	// Cache terminal background detection ONCE before any rendering.
	// Calling HasDarkBackground during the event loop blocks for seconds
	// because the terminal is busy with screen updates.
	initDarkBgCache()
	initTheme()
	// Set the client version for OAuth device headers
	providers.ClientVersion = Version
	oauth.ClientVersion = Version
}

func initTheme() {
	t := darkTheme
	if !cachedDarkBg {
		t = lightTheme
	}

	primary := lipgloss.Color(t.primary)
	border := lipgloss.Color(t.border)

	primaryStyle = lipgloss.NewStyle().Foreground(primary)
	boldPrimary = lipgloss.NewStyle().Foreground(primary).Bold(true)
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.textDim))
	textStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.text))
	strongStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.textStrong)).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.success))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.warning))
	userStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.roleUser)).Bold(true)
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.textMuted))

	borderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primary).
		Padding(0, 1)

	inputBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1)

	inputFocusedStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primary).
		Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.textDim)).
		Background(lipgloss.Color(t.statusBarBg)).
		Padding(0, 1)
}

// ── Slash commands ──

type slashCommand struct {
	name    string
	desc    string
	isSkill bool   // true if this is a skill (shown with [Skill] prefix)
	isFile  bool   // true for file/directory completions
	isDir   bool   // true when the completion is a directory (isFile must also be true)
	absPath string // absolute filesystem path (populated for file completions)
}

// commandReg is the global command registry.
var commandReg = NewCommandRegistry()

// slashCommands is auto-generated from the registry for autocomplete.
var slashCommands = commandReg.toSlashCommands()

// ── Chat message ──

type chatMessage struct {
	role    string // "user", "assistant", "system"
	content string
}

// ── Bubbletea model ──

// activeSkillInfo tracks the currently executing skill so that its constraints
// can be injected into the system prompt across all tool steps within a turn.
type activeSkillInfo struct {
	name string
	args string
}

type tuiModel struct {
	// Session service (thread-safe, shared across goroutines)
	svc *SessionService

	// State
	version      string
	cwd          string
	model        string
	branch       string
	messages     []chatMessage
	input        string
	cursor       int
	width        int
	height       int
	yoloMode     bool
	planMode     bool
	swarmEnabled bool // whether swarm sub-agent spawning is active
	quitting     bool
	ctrlCPending bool // true after first Ctrl+C; second press quits
	streaming    bool
	app          *App

	// LLM provider
	provider kosong.ChatProvider

	// Agent tools
	toolRegistry *tools.Registry
	bgManager    *background.Manager
	permChain    *permission.Chain
	cronManager  *cron.CronManager
	swarmRoster  *swarm.Roster
	hookEngine   *hooks.Engine
	injectionMgr *injection.Manager

	// Autocomplete
	suggestions     []slashCommand
	selectedSuggest int
	showSuggestions bool

	// @ file completion cycling (persists after Tab replaces input with abs path)
	fileCandidates []slashCommand // full candidate list for Tab cycling
	fileCycleIdx   int            // next candidate index in fileCandidates
	filePrefix     string         // text before the '@' trigger (restored on confirm)

	// Cursor
	cursorBlink bool

	// Streaming state
	streamCh         chan streamEvent
	streamThinking   string
	streamResponse   string
	mdBuffer         MarkdownBuffer
	streamToolGroups []toolGroup
	streamStep       int
	responseCursor   int // scroll offset in response view
	scrollOffset     int // viewport scroll offset in lines (0 = anchored to bottom)

	// Collapsible sections
	collapsibles  []collapsible
	focusIndex    int // -1 = none
	turnStartTime time.Time

	// Skills
	skillCatalog *skill.Catalog
	activeSkill  *activeSkillInfo // currently executing skill (nil = none)

	// Model picker
	showModelPicker bool
	pickerSearch    string
	pickerSelected  int
	pickerFilter    string // active provider filter ("" = ALL)
	pickerModels    []pickerEntry
	pickerEffort    string // "low", "high", "max"

	// Cycle 2: Interactive permission prompts
	prompter        *permission.Prompter
	showApproval    bool
	pendingApproval *permission.ApprovalRequest

	// Cycle 5: Mid-turn interaction
	cancelCh     chan struct{}
	steeringTool *tools.SteeringTool

	// TUI state
	showSessionPicker bool
	sessionPickerList []*session.SerializedSession
	sessionPickerSel  int

	// External editor mode
	editorMode bool

	// Editor temp file (used for /editor command)
	editorTempFile string

	// Input history
	inputHistory *InputHistory
	savedInput   string // saved input when navigating history

	// Bash mode output
	bashOutput string

	// OAuth login cancellation
	oauthCancel context.CancelFunc

	// Drawer (right-side panel)
	showDrawer     bool
	drawerWidthPct int // percentage of width for drawer (default 35)
	drawerToolLog  []drawerToolEntry
	drawerSkills   []drawerSkillEntry
}

// pickerEntry is a single model entry in the model picker.
type pickerEntry struct {
	alias       string // config key, e.g. "kimi-code/k3-256k"
	model       string // wire model name, e.g. "k3-256k"
	provider    string // provider name, e.g. "managed:kimi-code"
	displayName string // human name, e.g. "K3 256k"
	efforts     []string
}

func newTUIModel(app *App, sess *session.Session) tuiModel {
	cwd, _ := os.Getwd()
	modelName := app.Config.DefaultModel
	if modelName == "" {
		modelName = app.Config.DefaultProvider
	}
	if modelName == "" {
		modelName = "kimi"
	}
	version := BuildVersion()

	// Create LLM provider
	var provider kosong.ChatProvider
	if providers.IsConfigured(app.Config) {
		p, err := providers.NewFromConfig(app.Config)
		if err == nil {
			provider = p
		}
	}

	// Discover skills from project and user skill directories.
	var skillCat *skill.Catalog
	if cat, err := skill.Discover(cwd); err == nil {
		skillCat = cat
	}

	// Set up tool registry
	toolReg := tools.NewRegistry()
	tools.RegisterDefaultTools(toolReg)
	bgMgr := background.NewManager()
	tools.RegisterBackgroundTools(toolReg, bgMgr)

	// Plan mode tools (EnterPlanMode / ExitPlanMode)
	planInjector := injection.NewPlanModeInjector()
	planCtrl := tools.NewPlanModeController(planInjector)
	toolReg.Register(tools.NewEnterPlanModeTool(planCtrl))
	toolReg.Register(tools.NewExitPlanModeTool(planCtrl))

	// SelectTools (progressive tool disclosure)
	selectTools := tools.NewSelectToolsTool(toolReg)
	toolReg.Register(selectTools)

	// SkillTool (model-invoked skill activation)
	if skillCat != nil {
		toolReg.Register(tools.NewSkillTool(skillCat, nil))
	}

	// Cron management tools
	home, _ := os.UserHomeDir()
	cronDir := ""
	if home != "" {
		cronDir = filepath.Join(home, ".kimi-code")
	}
	cronMgr := cron.NewCronManager(cron.NewStore(cronDir), nil)
	tools.RegisterCronTools(toolReg, cronMgr)

	// Swarm roster and individual Agent tool
	roster := swarm.NewRoster(nil)
	tools.RegisterAgentTool(toolReg, roster)

	// Resolve the model's context window size from config so the status bar
	// displays the correct total (e.g. "ctx: 0 / 128K tokens") instead of
	// the hardcoded default.
	var maxCtx int
	if _, mc := app.Config.ResolveModel(); mc != nil {
		maxCtx = mc.MaxContextSize
	}

	// Session service: single source of truth for all session data
	svc := NewSessionService(sess, app, SessionServiceConfig{
		MaxCtx:              maxCtx,
		TriggerRatio:        app.Config.LoopControl.CompactionTriggerRatio,
		ReservedContextSize: app.Config.LoopControl.ReservedContextSize,
	})

	// Plan tracker for drawer progress section (from session service)
	planTrk := svc.PlanTracker()
	toolReg.Register(&tools.UpdatePlanTool{Tracker: planTrk})

	// Goal management tools (from session service)
	goalTrk := svc.GoalTracker()
	tools.RegisterGoalTools(toolReg, goalTrk)

	// Injection manager for system reminders (goal, plan mode)
	injectionMgr := injection.NewManager(
		injection.NewGoalInjector(goalTrk),
		planInjector,
	)

	// Steering tool for mid-turn user input (not registered in tool registry;
	// the streaming loop invokes it directly at step boundaries).
	steering := tools.NewSteeringTool()

	// Register GoGraph tools and hooks when available (opt-out via experimental.gograph=false)
	if app.Config.Experimental["gograph"] != false && tools.IsGoGraphAvailable() {
		runner := tools.NewGoGraphRunner()
		tools.RegisterGoGraphTools(toolReg, runner)
		toolReg.RegisterHook("Grep", tools.NewGoGraphHook(runner))
	}

	// Create hook engine from config
	var hookEngine *hooks.Engine
	if len(app.Config.Hooks) > 0 {
		hookEngine = hooks.NewEngine(app.Config.Hooks)
	}

	permChain := permission.DefaultChain()

	// Initialize input history
	var inputHist *InputHistory
	if home != "" {
		inputHist = NewInputHistory(filepath.Join(home, ".kimi-code"))
		_ = inputHist.Load()
	}

	return tuiModel{
		svc:            svc,
		version:        version,
		cwd:            cwd,
		model:          modelName,
		branch:         getGitBranch(skill.FindProjectRoot(cwd)),
		app:            app,
		provider:       provider,
		skillCatalog:   skillCat,
		toolRegistry:   toolReg,
		bgManager:      bgMgr,
		permChain:      permChain,
		cronManager:    cronMgr,
		swarmRoster:    roster,
		hookEngine:     hookEngine,
		injectionMgr:   injectionMgr,
		focusIndex:     -1,
		prompter:       permission.NewPrompter(),
		inputHistory:   inputHist,
		steeringTool:   steering,
		drawerWidthPct: 35,
	}
}

// recreateProvider rebuilds the LLM provider from the current config.
// This must be called after the model or provider is changed (e.g. via /model)
// so that subsequent API requests go to the correct endpoint with the right
// credentials.
func (m *tuiModel) recreateProvider() {
	if providers.IsConfigured(m.app.Config) {
		p, err := providers.NewFromConfig(m.app.Config)
		if err == nil {
			m.provider = p
		}
	}
	// Sync the context manager's max tokens with the current model config
	// so the status bar reflects the correct context window size.
	if _, mc := m.app.Config.ResolveModel(); mc != nil {
		m.svc.ContextMgr().SetMaxTokens(mc.MaxContextSize)
	}
}

// runOAuthLogin runs the OAuth device code flow asynchronously via tea.Cmd.
// It does NOT mutate m.app.Config directly (that would be a data race).
// Instead, it returns the fetched models via oauthLoginMsg for the Update handler to apply.
func (m *tuiModel) runOAuthLogin() tea.Cmd {
	return func() tea.Msg {
		manager, err := oauth.NewDefaultManager()
		if err != nil {
			return oauthLoginMsg{messages: []chatMessage{{"system", fmt.Sprintf("OAuth init failed: %s", err)}}}
		}

		// Use a cancellable context so the user can abort the login flow.
		ctx, cancel := context.WithCancel(context.Background())
		// Store cancel func on the model so it can be called from Update on Ctrl+C.
		// Note: writing to m from a tea.Cmd goroutine is safe here because
		// oauthCancel is only read when the user presses Ctrl+C (Update handler),
		// and the tea.Cmd has already returned by then.
		m.oauthCancel = cancel
		defer func() {
			cancel()
			m.oauthCancel = nil
		}()
		var msgs []chatMessage

		token, err := manager.Login(ctx, oauth.LoginOptions{
			OnDeviceCode: func(auth *oauth.DeviceAuthorization) error {
				url := auth.VerificationURIComplete
				if url == "" {
					url = auth.VerificationURI
				}
				msg := fmt.Sprintf("Visit: %s\nEnter code: %s", url, auth.UserCode)
				if auth.ExpiresIn > 0 {
					msg += fmt.Sprintf("\nExpires in %ds", auth.ExpiresIn)
				}
				msgs = append(msgs, chatMessage{"system", msg})
				oauth.OpenURL(url)
				return nil
			},
		})
		if err != nil {
			msgs = append(msgs, chatMessage{"system", fmt.Sprintf("Login failed: %s", err)})
			return oauthLoginMsg{messages: msgs}
		}

		// Fetch models (read-only operation, safe from goroutine)
		baseURL := oauth.ResolveBaseURL()
		models, err := oauth.FetchManagedModels(ctx, token.AccessToken, baseURL, nil)
		if err != nil {
			msgs = append(msgs, chatMessage{"system", fmt.Sprintf("Fetch models failed: %s", err)})
			return oauthLoginMsg{messages: msgs}
		}

		// Return models for the Update handler to apply config changes on the main thread
		msgs = append(msgs, chatMessage{"system", fmt.Sprintf("✓ Logged in to Kimi Code (OAuth) with %d models", len(models))})
		return oauthLoginMsg{messages: msgs, success: true, models: models, baseURL: baseURL}
	}
}

// replayFromAudit reconstructs TUI state from the BadgerDB audit trail.
// Returns true if successful, false if no audit data was found.
func (m *tuiModel) replayFromAudit() bool {
	data, err := m.app.AuditFacade.LoadSession(m.svc.ID())
	if err != nil || data == nil {
		return false
	}

	for _, tr := range data.Turns {
		// User message
		m.messages = append(m.messages, chatMessage{role: "user", content: tr.Prompt})
		m.svc.IncrementTurn()
		m.svc.AppendMessages(kosong.CreateUserMessage(tr.Prompt))

		// Assistant turn data (for collapsibles)
		td := turnData{
			thinking: tr.Thinking,
			text:     tr.Response,
		}
		for _, tc := range tr.Tools {
			td.toolGroups = append(td.toolGroups, toolGroup{
				name:      tc.Name,
				args:      tc.Arguments,
				result:    tc.Result,
				isError:   tc.IsError,
				collapsed: true,
				duration:  tc.Duration,
			})
		}
		m.svc.AppendTurn(td)
		m.messages = append(m.messages, chatMessage{role: "assistant", content: tr.Response})

		// LLM conversation history
		assistantMsg := kosong.Message{
			Role:    kosong.RoleAssistant,
			Content: []kosong.ContentPart{{Type: "text", Text: tr.Response}},
		}
		for _, tc := range tr.Tools {
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, kosong.ToolCall{
				Type:      "function",
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: &tc.Arguments,
			})
		}
		m.svc.AppendMessages(assistantMsg)
		for _, tc := range tr.Tools {
			m.svc.AppendMessages(kosong.CreateToolMessage(tc.ID, tc.Result))
		}

		// Accumulate usage
		if tr.Usage != nil {
			su := m.svc.SessionUsage()
			su.InputOther += tr.Usage.InputOther
			su.Output += tr.Usage.Output
			su.InputCacheRead += tr.Usage.InputCacheRead
			su.InputCacheCreation += tr.Usage.InputCacheCreation
			m.svc.SetSessionUsage(su)
		}
	}

	// Apply cache token correction (cache tokens are included in InputOther
	// from the API but tracked separately; subtract to avoid double-counting).
	su := m.svc.SessionUsage()
	su.InputOther -= su.InputCacheRead
	su.InputOther -= su.InputCacheCreation
	if su.InputOther < 0 {
		su.InputOther = 0
	}
	m.svc.SetSessionUsage(su)

	// Seed context manager with accumulated usage
	su = m.svc.SessionUsage()
	totalTokens := su.InputTotal() + su.Output
	if totalTokens > 0 {
		m.svc.ContextMgr().Reset()
		m.svc.ContextMgr().AddTurnUsage(totalTokens)
	}

	m.rebuildCollapsibles()
	return true
}

// replayHistory loads and replays session history into the TUI display.
func (m *tuiModel) replayHistory() {
	// Try audit-based replay first (BadgerDB)
	if m.app.AuditFacade != nil {
		if m.replayFromAudit() {
			return
		}
	}
	// Fallback to FileStore-based replay
	if m.app.SessionStore == nil {
		return
	}
	ctx := context.Background()
	if err := m.app.SessionStore.History().Load(ctx, m.svc.ID()); err != nil {
		return
	}
	msgs := m.app.SessionStore.History().Messages()
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			m.messages = append(m.messages, chatMessage{role: "user", content: msg.Content})
			m.svc.IncrementTurn()
			// Add user message to conversation history
			m.svc.AppendMessages(kosong.CreateUserMessage(msg.Content))
		case "assistant":
			td := turnData{
				thinking: msg.Thinking,
				text:     msg.Content,
			}
			for _, tc := range msg.ToolCalls {
				td.toolGroups = append(td.toolGroups, toolGroup{
					name:      tc.Name,
					args:      tc.Arguments,
					result:    tc.Result,
					isError:   tc.IsError,
					collapsed: true,
					duration:  tc.Duration,
				})
			}
			m.svc.AppendTurn(td)
			m.messages = append(m.messages, chatMessage{role: "assistant", content: msg.Content})
			// Add assistant response to conversation history
			assistantMsg := kosong.Message{
				Role:    kosong.RoleAssistant,
				Content: []kosong.ContentPart{{Type: "text", Text: msg.Content}},
			}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, kosong.ToolCall{
						Type:      "function",
						ID:        tc.ID,
						Name:      tc.Name,
						Arguments: &tc.Arguments,
					})
				}
			}
			m.svc.AppendMessages(assistantMsg)
			// Add tool result messages so the LLM has complete
			// tool call / result pairs when resuming a session.
			for _, tc := range msg.ToolCalls {
				m.svc.AppendMessages(kosong.CreateToolMessage(tc.ID, tc.Result))
			}
		}
	}
	// Restore cumulative session usage from persisted metadata.
	// Use the real API token counts (persisted as tokens_in/tokens_out)
	// for the context manager instead of text-based estimates, which
	// drastically undercount the actual context window usage.
	if m.svc.Session().Metadata != nil {
		var tokensIn, tokensOut int
		su := m.svc.SessionUsage()
		if v, ok := metaInt(m.svc.Session().Metadata, "tokens_in"); ok {
			tokensIn = v
			su.InputOther = v
		}
		if v, ok := metaInt(m.svc.Session().Metadata, "tokens_out"); ok {
			tokensOut = v
			su.Output = v
		}
		if v, ok := metaInt(m.svc.Session().Metadata, "cache_read"); ok {
			su.InputCacheRead = v
			// Move cache tokens from InputOther to InputCacheRead
			// so InputTotal() stays correct.
			su.InputOther -= v
		}
		if v, ok := metaInt(m.svc.Session().Metadata, "cache_creation"); ok {
			su.InputCacheCreation = v
			su.InputOther -= v
		}
		// Guard against negative InputOther from corrupted metadata.
		if su.InputOther < 0 {
			su.InputOther = 0
		}
		m.svc.SetSessionUsage(su)
		// Use persisted real token counts for context window display.
		// tokens_in is the cumulative API input across all turns;
		// this matches what the live session showed before exit.
		if tokensIn > 0 {
			m.svc.ContextMgr().Reset()
			m.svc.ContextMgr().AddTurnUsage(tokensIn + tokensOut)
		} else if m.svc.CompletedTurnsLen() > 0 {
			// Fallback for sessions persisted before real token tracking:
			// use text-based estimates so the context bar isn't empty.
			m.svc.ContextMgr().Reset()
			for _, td := range m.svc.CompletedTurns() {
				m.svc.ContextMgr().AddTurnUsage(agentctx.TurnEstimate(td.text))
			}
		}
	}
	m.rebuildCollapsibles()
}

// ── Streaming events for bubbletea ──

type streamEvent struct {
	kind         string // "think", "text", "tool_start", "tool_result", "step_done", "done", "error", "usage", "finish"
	text         string
	toolName     string
	toolArgs     string
	toolOut      string
	toolErr      bool
	toolDur      time.Duration
	step         int
	usage        *kosong.TokenUsage
	finishReason *string // raw finish_reason from upstream API (on "finish" events)
}

// listenStream waits for the next streaming event from the channel.
func listenStream(ch <-chan streamEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamEvent{kind: "done"}
		}
		return ev
	}
}

// toolGroup represents a single tool invocation with expand/collapse state.
type toolGroup struct {
	name      string
	args      string
	result    string
	isError   bool
	collapsed bool
	duration  time.Duration // wall-clock execution time for this tool
}

// collapsible is a focusable UI section that can be expanded/collapsed.
type collapsible struct {
	kind      string // "thinking", "tools", "response"
	expanded  bool
	turnIndex int // which turn this belongs to
}

// turnData stores the full output of a completed LLM turn.
type turnData struct {
	thinking   string
	text       string
	toolGroups []toolGroup
}

// drawerToolEntry records a single tool call for the drawer's Tools section.
type drawerToolEntry struct {
	name     string
	args     string // short summary of arguments
	isError  bool
	duration time.Duration
	at       time.Time
	tokens   int // estimated token count
}

// drawerSkillEntry records a skill invocation for the drawer's Skills section.
type drawerSkillEntry struct {
	name string
	at   time.Time
}

// truncateRune truncates by rune count (not bytes) to avoid corrupting
// multi-byte UTF-8 characters. Appends "..." when truncation occurs.
func truncateRune(s string, maxRunes int) string {
	if maxRunes < 4 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-1]) + "…"
}

// toolArgSummary extracts a short one-line summary from JSON tool arguments.
func toolArgSummary(args string) string {
	if args == "" || args == "{}" || args == "null" {
		return ""
	}
	// Try to parse as JSON and extract key fields
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		// Not valid JSON, truncate raw (by runes, not bytes)
		return truncateRune(args, 40)
	}
	// Prioritize key fields for display: file paths and commands first.
	var parts []string
	priorityKeys := []string{"file_path", "path", "command", "query", "pattern"}
	seen := map[string]bool{}
	for _, k := range priorityKeys {
		if v, ok := m[k]; ok {
			s := fmt.Sprintf("%v", v)
			s = truncateRune(s, 40)
			parts = append(parts, s)
			seen[k] = true
		}
	}
	var extraKeys []string
	for k := range m {
		if !seen[k] {
			extraKeys = append(extraKeys, k)
		}
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		v := m[k]
		s := fmt.Sprintf("%v", v)
		s = truncateRune(s, 30)
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	result := strings.Join(parts, ", ")
	result = truncateRune(result, 60)
	return result
}

// toolVerb maps a tool name to a human-friendly action verb.
func toolVerb(name string) string {
	verbs := map[string]string{
		"write_file":      "Write",
		"edit_file":       "Edit",
		"search_replace":  "Edit",
		"read_file":       "Read",
		"list_dir":        "List",
		"bash":            "Bash",
		"execute":         "Bash",
		"grep":            "Search",
		"glob":            "Search",
		"search":          "Search",
		"search_codebase": "Search",
		"lsp":             "LSP",
		"fetch":           "Fetch",
		"web_fetch":       "Fetch",
		"delete_file":     "Delete",
	}
	lower := strings.ToLower(name)
	if v, ok := verbs[lower]; ok {
		return v
	}
	// Title-case fallback
	if name == "" {
		return "Tool"
	}
	r := []rune(name)
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

// diff-producing tool names whose results contain unified diff output.
var diffToolNames = map[string]bool{
	"search_replace": true,
	"edit_file":      true,
	"write_file":     true,
}

// diffStats extracts a "+N/-M" badge from tool result text by counting
// lines starting with '+' or '-' in the first 500 characters.
// Only returns stats for tools known to produce diff output.
func diffStats(toolName, result string) string {
	if result == "" {
		return ""
	}
	if !diffToolNames[strings.ToLower(toolName)] {
		return ""
	}
	scan := result
	if len(scan) > 500 {
		scan = scan[:500]
		// Back up to last valid rune boundary to avoid splitting multi-byte chars.
		for len(scan) > 0 && !utf8.ValidString(scan) {
			scan = scan[:len(scan)-1]
		}
	}
	var added, removed int
	for _, line := range strings.Split(scan, "\n") {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			added++
		case '-':
			removed++
		}
	}
	if added == 0 && removed == 0 {
		return ""
	}
	return fmt.Sprintf("+%d/-%d", added, removed)
}

// isContextOverflow returns true if the error text indicates a context
// window overflow (HTTP 413, context length exceeded, etc.).
func isContextOverflow(errText string) bool {
	lower := strings.ToLower(errText)
	return strings.Contains(lower, "context length") ||
		strings.Contains(lower, "too many tokens") ||
		strings.Contains(lower, "maximum context") ||
		strings.Contains(lower, "context limit") ||
		strings.Contains(lower, "status 413") ||
		strings.Contains(lower, "http 413") ||
		strings.Contains(lower, "413 request entity") ||
		strings.Contains(lower, "token limit")
}

// runLLMStream is a bubbletea Cmd that streams the LLM response with tool calling.
// It creates a channel of streamEvents and returns a listenStream command.
// If isOverflowRetry is true, the user message is NOT appended to history
// (it's already present from the failed attempt, preserved by rewriteContext).
func (m *tuiModel) runLLMStream(prompt string, isOverflowRetry bool) tea.Cmd {
	ch := make(chan streamEvent, 64)
	m.streamCh = ch
	m.turnStartTime = time.Now()
	m.svc.SetLastPrompt(prompt) // save for overflow retry

	// Snapshot fields read by the goroutine to avoid data races.
	// The goroutine runs concurrently with the bubbletea Update loop,
	// which may replace m.provider (via /model) or m.activeSkill.
	provider := m.provider
	model := m.model
	cwd := m.cwd
	branch := m.branch
	skillCat := m.skillCatalog
	activeSkill := m.activeSkill
	permChain := m.permChain
	toolReg := m.toolRegistry
	hookEng := m.hookEngine
	injectionMgr := m.injectionMgr
	homeDir, _ := os.UserHomeDir()

	go func() {
		defer close(ch)

		if provider == nil {
			ch <- streamEvent{kind: "error", text: "no provider configured. Set API key in ~/.kimi-code/config.toml"}
			return
		}

		// Add user message to history (skip on overflow retry — already present)
		if !isOverflowRetry {
			m.svc.AppendMessages(kosong.CreateUserMessage(prompt))
		}

		// Record audit: LLM request
		if m.svc.AuditWriter() != nil {
			m.svc.AuditWriter().Record(audit.AuditEvent{
				SessionID: m.svc.ID(),
				Type:      audit.EvtLLMRequest,
				Data:      map[string]any{"prompt": prompt, "model": model},
			})
		}

		ctx := context.Background()
		agentsMd, _ := promptpkg.LoadAgentsMd(cwd, homeDir)
		systemReminders := ""
		if injectionMgr != nil {
			systemReminders = injectionMgr.InjectAll()
		}
		systemPrompt := buildSystemPrompt(cwd, branch, skillCat, activeSkill, agentsMd, systemReminders)

		// Convert tool definitions
		var kosongTools []kosong.Tool
		for _, def := range toolReg.Definitions() {
			kosongTools = append(kosongTools, kosong.Tool{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			})
		}

		maxSteps := 25
		for step := 0; step < maxSteps; step++ {
			// Reset finish reason per step via channel so the model's lastFinishReason
			// doesn't carry over from a previous step if this step errors.
			if step > 0 {
				ch <- streamEvent{kind: "finish", finishReason: nil}
			}
			// Build GenerateOptions with raw payload capture for audit
			var genOpts *kosong.GenerateOptions
			if m.svc.AuditWriter() != nil {
				genOpts = &kosong.GenerateOptions{
					OnRawRequest: func(body []byte) {
						m.svc.AuditWriter().Record(audit.AuditEvent{
							SessionID: m.svc.ID(),
							Type:      audit.EvtLLMRawRequest,
							Data:      map[string]any{"step": step, "body": string(body)},
						})
					},
					OnRawResponse: func(filePath string) {
						defer os.Remove(filePath)
						data, err := os.ReadFile(filePath)
						if err != nil {
							return
						}
						m.svc.AuditWriter().Record(audit.AuditEvent{
							SessionID: m.svc.ID(),
							Type:      audit.EvtLLMRawResponse,
							Data:      map[string]any{"step": step, "raw": string(data)},
						})
					},
				}
			}
			stream, err := provider.Generate(ctx, systemPrompt, kosongTools, m.svc.History(), genOpts)
			if err != nil {
				ch <- streamEvent{kind: "error", text: err.Error()}
				return
			}

			// Consume stream incrementally, batching text/think events for UI smoothness.
			// Sending one streamEvent per SSE token floods Bubble Tea's event loop
			// (Update+View per token), starving keypress handling. Instead, accumulate
			// text/think deltas and flush on a ~50ms timer (~20fps render rate).
			var content []kosong.ContentPart
			var toolCalls []kosong.ToolCall
			var pending *kosong.StreamedMessagePart
			var stepFinishReason *string // raw finish_reason from upstream API

			const streamBatchInterval = 50 * time.Millisecond
			var batchThink, batchText string
			batchFlush := func() {
				if batchThink != "" {
					ch <- streamEvent{kind: "think", text: batchThink}
					if m.svc.AuditWriter() != nil {
						m.svc.AuditWriter().Record(audit.AuditEvent{
							SessionID: m.svc.ID(),
							Type:      audit.EvtLLMDeltaThink,
							Data:      map[string]any{"text": batchThink},
						})
					}
					batchThink = ""
				}
				if batchText != "" {
					ch <- streamEvent{kind: "text", text: batchText}
					if m.svc.AuditWriter() != nil {
						m.svc.AuditWriter().Record(audit.AuditEvent{
							SessionID: m.svc.ID(),
							Type:      audit.EvtLLMDeltaText,
							Data:      map[string]any{"text": batchText},
						})
					}
					batchText = ""
				}
			}

			ticker := time.NewTicker(streamBatchInterval)

			for {
				select {
				case part, ok := <-stream.Parts:
					if !ok {
						// Stream closed, flush any remaining batched content
						batchFlush()
						goto streamDone
					}
					select {
					case <-ctx.Done():
						ticker.Stop()
						batchFlush()
						ch <- streamEvent{kind: "error", text: ctx.Err().Error()}
						return
					default:
					}

					// Batch text/think; send usage and finish immediately (rare)
					switch part.Type {
					case "think":
						batchThink += part.Think
					case "text":
						batchText += part.Text
					case "usage":
						batchFlush() // flush pending text before usage event
						ch <- streamEvent{kind: "usage", usage: part.Usage}
						continue // don't treat as content part
					case "finish":
						batchFlush() // flush pending text before finish event
						stepFinishReason = part.FinishReason
						ch <- streamEvent{kind: "finish", finishReason: part.FinishReason}
						continue // don't treat as content part
					}

					// Merge parts to build final message (same logic as kosong.Generate)
					if pending != nil {
						if kosong.MergeInPlace(pending, &part) {
							continue
						}
						switch pending.Type {
						case "text":
							content = append(content, kosong.ContentPart{Type: "text", Text: pending.Text})
						case "think":
							content = append(content, kosong.ContentPart{Type: "think", Think: pending.Think, Encrypted: pending.Encrypted})
						case "function":
							toolCalls = append(toolCalls, kosong.ToolCall{
								Type: "function", ID: pending.ID, Name: pending.Name,
								Arguments: pending.Arguments, Extras: pending.Extras,
							})
						}
					}
					pending = &part

				case <-ticker.C:
					batchFlush()
				}
			}
		streamDone:
			ticker.Stop()

			// Flush final pending part
			if pending != nil {
				switch pending.Type {
				case "text":
					content = append(content, kosong.ContentPart{Type: "text", Text: pending.Text})
				case "think":
					content = append(content, kosong.ContentPart{Type: "think", Think: pending.Think, Encrypted: pending.Encrypted})
				case "function":
					toolCalls = append(toolCalls, kosong.ToolCall{
						Type: "function", ID: pending.ID, Name: pending.Name,
						Arguments: pending.Arguments, Extras: pending.Extras,
					})
				}
			}

			msg := &kosong.Message{
				Role:      kosong.RoleAssistant,
				Content:   content,
				ToolCalls: toolCalls,
			}

			ch <- streamEvent{kind: "step_done", step: step}
			if m.svc.AuditWriter() != nil {
				stepData := map[string]any{"step": step, "tool_calls": len(msg.ToolCalls)}
				if stepFinishReason != nil {
					stepData["finish_reason"] = *stepFinishReason
				}
				m.svc.AuditWriter().Record(audit.AuditEvent{
					SessionID: m.svc.ID(),
					Type:      audit.EvtLLMStepDone,
					Data:      stepData,
				})
			}

			// If no tool calls, we're done
			if len(msg.ToolCalls) == 0 {
				m.svc.AppendMessages(*msg)
				ch <- streamEvent{kind: "done"}
				return
			}

			m.svc.AppendMessages(*msg)

			// Execute each tool call
			for _, tc := range msg.ToolCalls {
				var argsStr string
				if tc.Arguments != nil {
					argsStr = *tc.Arguments
				}
				ch <- streamEvent{kind: "tool_start", toolName: tc.Name, toolArgs: argsStr}
				if m.svc.AuditWriter() != nil {
					m.svc.AuditWriter().Record(audit.AuditEvent{
						SessionID: m.svc.ID(),
						Type:      audit.EvtLLMToolCall,
						Data:      map[string]any{"id": tc.ID, "name": tc.Name, "arguments": argsStr},
					})
				}

				tool, ok := toolReg.Get(tc.Name)
				if !ok {
					ch <- streamEvent{kind: "tool_result", toolName: tc.Name, toolOut: fmt.Sprintf("tool %q not found", tc.Name), toolErr: true}
					m.svc.AppendMessages(kosong.CreateToolMessage(tc.ID, fmt.Sprintf("tool %q not found", tc.Name)))
					continue
				}

				var input json.RawMessage
				if tc.Arguments != nil {
					input = json.RawMessage(*tc.Arguments)
				} else {
					input = json.RawMessage("{}")
				}

				// Permission check
				permResult := permChain.Evaluate(tc.Name, input)
				if permResult.Decision == permission.DecisionDeny {
					denyMsg := fmt.Sprintf("[Denied] %s", permResult.Reason)
					ch <- streamEvent{kind: "tool_result", toolName: tc.Name, toolOut: denyMsg, toolErr: true}
					m.svc.AppendMessages(kosong.CreateToolMessage(tc.ID, fmt.Sprintf("Permission denied: %s", permResult.Reason)))
					continue
				}

				// PreToolUse hook (blocking)
				if hookEng != nil {
					hookInput := hooks.HookInput{
						Tool:    &hooks.HookToolInput{Name: tc.Name, Input: string(input)},
						Session: &hooks.HookSession{ID: m.svc.ID(), WorkDir: cwd},
					}
					decision := hookEng.TriggerBlock(ctx, hooks.PreToolUse, hookInput)
					if decision.Blocked {
						blockMsg := fmt.Sprintf("[Blocked by hook] %s", decision.Reason)
						ch <- streamEvent{kind: "tool_result", toolName: tc.Name, toolOut: blockMsg, toolErr: true}
						m.svc.AppendMessages(kosong.CreateToolMessage(tc.ID, blockMsg))
						continue
					}
				}

				toolStart := time.Now()
				result, err := tool.Execute(ctx, input, tools.ExecContext{WorkDir: cwd})
				toolDur := time.Since(toolStart)
				if err != nil {
					result = &tools.Result{Output: err.Error(), IsError: true}
				}

				// PostToolUse hook (fire-and-forget)
				if hookEng != nil {
					postEvent := hooks.PostToolUse
					if result.IsError {
						postEvent = hooks.PostToolUseFailure
					}
					hookInput := hooks.HookInput{
						Tool:    &hooks.HookToolInput{Name: tc.Name, Input: string(input), Output: result.Output},
						Session: &hooks.HookSession{ID: m.svc.ID(), WorkDir: cwd},
					}
					hookEng.FireAndForget(ctx, postEvent, hookInput)
				}

				ch <- streamEvent{kind: "tool_result", toolName: tc.Name, toolOut: truncateOutput(result.Output), toolErr: result.IsError, toolDur: toolDur}
				if m.svc.AuditWriter() != nil {
					m.svc.AuditWriter().Record(audit.AuditEvent{
						SessionID: m.svc.ID(),
						Type:      audit.EvtLLMToolResult,
						Data: map[string]any{
							"name":     tc.Name,
							"result":   truncateOutput(result.Output),
							"isError":  result.IsError,
							"duration": toolDur.String(),
						},
					})
				}
				m.svc.AppendMessages(kosong.CreateToolMessage(tc.ID, result.Output))
			}

			// Check for steering messages between steps.
			// DrainAll is atomic (single lock acquisition) and returns nil if empty.
			if m.steeringTool != nil {
				if m.steeringTool.ConsumeSignal() || m.steeringTool.HasMessages() {
					msgs := m.steeringTool.DrainAll()
					if len(msgs) > 0 {
						result := m.steeringTool.FormatMessages(msgs)
						ch <- streamEvent{kind: "tool_start", toolName: "Steering", toolArgs: ""}
						ch <- streamEvent{kind: "tool_result", toolName: "Steering", toolOut: result}
						// Inject as user message to satisfy LLM provider API contracts
						// (tool messages must correspond to assistant-initiated tool calls)
						m.svc.AppendMessages(kosong.CreateUserMessage("[Steering] " + result))
					}
				}
			}
		}

		ch <- streamEvent{kind: "done"}
		if m.svc.AuditWriter() != nil {
			m.svc.AuditWriter().Record(audit.AuditEvent{
				SessionID: m.svc.ID(),
				Type:      audit.EvtLLMDone,
			})
		}
	}()

	return listenStream(ch)
}

// ── Init ──

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestWindowSize,
		m.tickCursor(),
	)
}

// cursorTickMsg is sent periodically to toggle cursor visibility.
type cursorTickMsg struct{}

// workingTickMsg is sent periodically to update the working indicator duration.
type workingTickMsg struct{}

// oauthLoginMsg carries the result of an async OAuth login flow.
type oauthLoginMsg struct {
	messages []chatMessage
	success  bool
	models   []oauth.ModelInfo // fetched models for config provisioning
	baseURL  string            // resolved base URL
}

// modelsResultMsg carries the result of a /models discovery request.
type modelsResultMsg struct {
	text string
}

func (m tuiModel) tickCursor() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg {
		return cursorTickMsg{}
	})
}

func (m tuiModel) tickWorking() tea.Cmd {
	return tea.Tick(1*time.Second, func(_ time.Time) tea.Msg {
		return workingTickMsg{}
	})
}

// ── Update ──

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Trace all incoming messages for event loop diagnosis
	if trace.Enabled() {
		switch v := msg.(type) {
		case tea.KeyPressMsg:
			trace.Log("input", "keypress", map[string]any{"code": string(rune(v.Code)), "mod": int(v.Mod)})
		case streamEvent:
			trace.Log("stream", "event", map[string]any{"kind": v.kind, "textLen": len(v.text)})
		case tea.WindowSizeMsg:
			trace.Log("tui", "resize", map[string]any{"w": v.Width, "h": v.Height})
		case cursorTickMsg:
			// skip - too noisy
		default:
			trace.Logf("tui", "msg", "%T", msg)
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case cursorTickMsg:
		m.cursorBlink = !m.cursorBlink
		return m, m.tickCursor()

	case workingTickMsg:
		// Re-render to update the working indicator duration
		if m.streaming {
			return m, m.tickWorking()
		}
		return m, nil

	case editorResultMsg:
		if msg.err != nil {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Editor error: %s", msg.err)})
			return m, nil
		}
		if msg.content != "" {
			m.input = msg.content
			m.cursor = utf8.RuneCountInString(m.input)
			m.messages = append(m.messages, chatMessage{"system", "Editor content loaded. Press Enter to submit."})
		}
		return m, nil

	case modelsResultMsg:
		m.messages = append(m.messages, chatMessage{"system", msg.text})
		return m, nil

	case oauthLoginMsg:
		m.messages = append(m.messages, msg.messages...)
		if msg.success && len(msg.models) > 0 {
			// Apply config mutations on the main thread to avoid data race
			oauthHost := oauth.GetOAuthHost()
			if err := oauth.ProvisionConfig(m.app.Config, msg.models, msg.baseURL, oauthHost); err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Config provision failed: %s", err)})
			} else if err := m.app.Config.SaveToFile(m.app.ConfigPath); err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Save config failed: %s", err)})
			} else {
				m.recreateProvider()
			}
		}
		return m, nil

	case streamEvent:
		switch msg.kind {
		case "think":
			m.streamThinking += msg.text
			m.rebuildCollapsibles()
			return m, listenStream(m.streamCh)
		case "text":
			if safe, ok := m.mdBuffer.Push(msg.text); ok {
				m.streamResponse += safe
			}
			return m, listenStream(m.streamCh)
		case "tool_start":
			m.streamToolGroups = append(m.streamToolGroups, toolGroup{
				name: msg.toolName, args: msg.toolArgs, collapsed: true, duration: msg.toolDur,
			})
			// Track for drawer (capped to avoid unbounded memory growth)
			m.drawerToolLog = append(m.drawerToolLog, drawerToolEntry{
				name: msg.toolName, args: toolArgSummary(msg.toolArgs), at: time.Now(),
				tokens: agentctx.TokenEstimate(msg.toolArgs),
			})
			if len(m.drawerToolLog) > 500 {
				m.drawerToolLog = m.drawerToolLog[len(m.drawerToolLog)-500:]
			}
			m.rebuildCollapsibles()
			return m, listenStream(m.streamCh)
		case "tool_result":
			if len(m.streamToolGroups) > 0 {
				last := &m.streamToolGroups[len(m.streamToolGroups)-1]
				last.result = msg.toolOut
				last.isError = msg.toolErr
				if msg.toolDur > 0 {
					last.duration = msg.toolDur
				}
			}
			// Update drawer entry — match by name to handle potential reordering
			if len(m.drawerToolLog) > 0 {
				dlast := &m.drawerToolLog[len(m.drawerToolLog)-1]
				if dlast.name == msg.toolName {
					dlast.isError = msg.toolErr
					dlast.duration = msg.toolDur
				}
			}
			// Auto-sync plan tracker: update task status based on tool result
			if m.svc.PlanTracker() != nil {
				keyword := msg.toolName
				// Use tool name as keyword for matching task titles
				if msg.toolErr {
					m.svc.PlanTracker().UpdateTaskByKeyword(keyword, plan.StatusFailed)
				} else {
					// Only auto-mark done if task is currently active (not already done/pending)
					// This prevents overwriting explicit LLM updates
					m.svc.PlanTracker().UpdateTaskByKeyword(keyword, plan.StatusDone)
				}
			}
			return m, listenStream(m.streamCh)
		case "step_done":
			m.streamStep = msg.step
			return m, listenStream(m.streamCh)
		case "error":
			m.streaming = false
			// Record audit: LLM error
			if m.svc.AuditWriter() != nil {
				m.svc.AuditWriter().Record(audit.AuditEvent{
					SessionID: m.svc.ID(),
					Type:      audit.EvtLLMError,
					Data:      map[string]any{"error": msg.text},
				})
			}
			// Overflow error recovery: compact and retry if possible
			if isContextOverflow(msg.text) && m.svc.CompactStrategy() != nil &&
				m.svc.OverflowRetries() < m.svc.CompactStrategy().MaxOverflowAttempts() &&
				m.svc.CompactStrategy().CanCompact() && m.svc.LastPrompt() != "" {
				m.svc.IncrementOverflow()
				if m.performCompaction(true) {
					// Also strip media from history to further reduce payload on 413
					if stripped := m.stripMediaFromHistory(); stripped > 0 {
						m.messages = append(m.messages, chatMessage{"system",
							fmt.Sprintf("Stripped %d media parts from history to reduce payload.", stripped)})
					}
					m.messages = append(m.messages, chatMessage{"system",
						fmt.Sprintf("Context overflow detected, compacted (attempt %d/%d). Retrying...",
							m.svc.OverflowRetries(), m.svc.CompactStrategy().MaxOverflowAttempts())})
					m.streaming = true
					m.streamToolGroups = nil
					m.streamStep = 0
					m.turnStartTime = time.Now()
					m.cancelCh = make(chan struct{})
					m.rebuildCollapsibles()
					return m, tea.Batch(m.runLLMStream(m.svc.LastPrompt(), true), m.tickWorking())
				} // else: compaction failed, fall through to normal error handling
			}
			// /btw: reset side-query state so next prompt isn't affected
			if m.svc.BtwMode() {
				m.svc.TruncateHistory(m.svc.BtwHistoryLen())
				m.svc.SetBtwMode(false)
			}
			// Remove the "Thinking..." placeholder
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "system" && m.messages[len(m.messages)-1].content == "Thinking..." {
				m.messages = m.messages[:len(m.messages)-1]
			}
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Error: %s", msg.text)})
			return m, nil
		case "usage":
			if msg.usage != nil {
				m.svc.AddTurnUsage(*msg.usage)
				// Update pending estimate for live context bar display
				tu := m.svc.TurnUsage()
				m.svc.ContextMgr().SetPendingEstimate(tu.InputTotal() + tu.Output)
				// Record audit: usage event
				if m.svc.AuditWriter() != nil {
					usageData := map[string]any{
						"input":          msg.usage.InputTotal(),
						"output":         msg.usage.Output,
						"cache_read":     msg.usage.InputCacheRead,
						"cache_creation": msg.usage.InputCacheCreation,
					}
					if msg.usage.ReasoningTokens > 0 {
						usageData["reasoning_tokens"] = msg.usage.ReasoningTokens
					}
					m.svc.AuditWriter().Record(audit.AuditEvent{
						SessionID: m.svc.ID(),
						Type:      audit.EvtLLMUsage,
						Data:      usageData,
					})
				}
			}
			return m, listenStream(m.streamCh)
		case "finish":
			m.svc.SetFinishReason(msg.finishReason)
			return m, listenStream(m.streamCh)
		case "done":
			m.streaming = false
			// Flush any remaining buffered markdown
			m.streamResponse += m.mdBuffer.Flush()
			// Remove the "Thinking..." placeholder
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "system" && m.messages[len(m.messages)-1].content == "Thinking..." {
				m.messages = m.messages[:len(m.messages)-1]
			}
			// Save completed turn data
			td := turnData{
				thinking:   m.streamThinking,
				text:       m.streamResponse,
				toolGroups: m.streamToolGroups,
			}
			m.svc.AppendTurn(td)

			// Append assistant response to messages so it stays visible after streaming
			if m.streamResponse == "" && m.streamThinking != "" {
				// Model produced thinking but no visible response.
				// Build a diagnostic message including the finish reason if available.
				diag := "⚠ The model produced extended reasoning but no visible response."
				if reason, ok := m.svc.LastFinishReason(); ok {
					diag += fmt.Sprintf(" (finish_reason: %s)", reason)
				}
				tu := m.svc.TurnUsage()
				if tu.ReasoningTokens > 0 {
					diag += fmt.Sprintf(" [reasoning tokens: %d/%d output]", tu.ReasoningTokens, tu.Output)
				}
				diag += " The thinking budget may have been exhausted. Try rephrasing or reducing thinking effort."
				m.messages = append(m.messages, chatMessage{"assistant", diag})
			} else if m.streamResponse == "" && m.streamThinking == "" {
				// Completely empty response — likely preceded by an error, but
				// provide a diagnostic just in case.
				diag := "⚠ The model produced no response."
				if reason, ok := m.svc.LastFinishReason(); ok {
					diag += fmt.Sprintf(" (finish_reason: %s)", reason)
				}
				m.messages = append(m.messages, chatMessage{"assistant", diag})
			} else {
				m.messages = append(m.messages, chatMessage{"assistant", m.streamResponse})
			}

			// Track token usage: prefer real API usage, fall back to estimation
			savedTurnUsage := m.svc.TurnUsage()
			if savedTurnUsage.GrandTotal() > 0 {
				m.svc.ContextMgr().AddTurnUsage(savedTurnUsage.InputTotal() + savedTurnUsage.Output)
				m.svc.AddSessionUsage(savedTurnUsage)
			} else {
				turnTokens := agentctx.TokenEstimate(m.streamResponse) + agentctx.TokenEstimate(m.streamThinking)
				m.svc.ContextMgr().AddTurnUsage(turnTokens)
				m.svc.AddSessionUsage(kosong.TokenUsage{InputOther: turnTokens})
			}
			m.svc.ResetTurnUsage()   // reset for next turn
			m.svc.ResetOverflow()     // reset overflow retries for next turn

			// Auto-compaction check: trigger if context is nearly full
			if m.svc.ContextMgr().NeedsCompaction() && m.svc.CompactStrategy().ShouldCompact(m.svc.ContextMgr().CurrentUsage()) {
				m.performCompaction(true)
			}

			// Cycle 1: Persist session history (skip for /btw side queries)
			if !m.svc.BtwMode() && m.app.SessionStore != nil {
				var toolCalls []session.ToolCall
				for _, tg := range m.streamToolGroups {
					toolCalls = append(toolCalls, session.ToolCall{
						Name:      tg.name,
						Arguments: tg.args,
						Result:    tg.result,
						IsError:   tg.isError,
						Duration:  tg.duration,
					})
				}
				// Find the last user message to use as the prompt for persistence.
				// m.messages ends with the assistant response just appended above;
				// the user prompt is immediately before it.
				var lastUserContent string
				for i := len(m.messages) - 1; i >= 0; i-- {
					if m.messages[i].role == "user" {
						lastUserContent = m.messages[i].content
						break
					}
				}
				_ = m.app.SessionStore.History().AddTurn(context.Background(), m.svc.ID(),
					lastUserContent,
					m.streamResponse, m.streamThinking, toolCalls)
				m.svc.Session().SetStatus(session.StatusIdle)
				// Persist session summary metadata for /sessions listing
				sess := m.svc.Session()
				su := m.svc.SessionUsage()
				if sess.Metadata == nil {
					sess.Metadata = make(map[string]any)
				}
				sess.Metadata["turns"] = m.svc.TurnCount()
				sess.Metadata["tokens_in"] = su.InputTotal()
				sess.Metadata["tokens_out"] = su.Output
				if su.InputCacheRead > 0 {
					sess.Metadata["cache_read"] = su.InputCacheRead
				}
				if su.InputCacheCreation > 0 {
					sess.Metadata["cache_creation"] = su.InputCacheCreation
				}
				_ = m.app.SessionStore.Save(context.Background(), sess)
			}

			// Record audit: turn completed
			if m.svc.AuditWriter() != nil {
				var lastUserContent string
				for i := len(m.messages) - 1; i >= 0; i-- {
					if m.messages[i].role == "user" {
						lastUserContent = m.messages[i].content
						break
					}
				}
				var toolRecs []audit.ToolCallRecord
				for _, tg := range m.streamToolGroups {
					toolRecs = append(toolRecs, audit.ToolCallRecord{
						Name:      tg.name,
						Arguments: tg.args,
						Result:    tg.result,
						IsError:   tg.isError,
						Duration:  tg.duration,
					})
				}
				var usageRec *audit.UsageRecord
				if savedTurnUsage.GrandTotal() > 0 {
					usageRec = &audit.UsageRecord{
						InputOther:         savedTurnUsage.InputOther,
						Output:             savedTurnUsage.Output,
						InputCacheRead:     savedTurnUsage.InputCacheRead,
						InputCacheCreation: savedTurnUsage.InputCacheCreation,
						ReasoningTokens:    savedTurnUsage.ReasoningTokens,
					}
				}
				var finishReason string
				if reason, ok := m.svc.LastFinishReason(); ok {
					finishReason = reason
				}
				sess := m.svc.Session()
				m.svc.AuditWriter().Record(audit.AuditEvent{
					SessionID: m.svc.ID(),
					Type:      audit.EvtTurnCompleted,
					Data: audit.TurnRecord{
						Prompt:       lastUserContent,
						Response:     m.streamResponse,
						Thinking:     m.streamThinking,
						Tools:        toolRecs,
						Usage:        usageRec,
						FinishReason: finishReason,
					},
				})
				// Also persist session metadata to audit store
				if err := m.svc.AuditWriter().SaveSession(audit.SessionRecord{
					ID:        m.svc.ID(),
					Title:     sess.Title,
					Status:    string(sess.Status),
					CreatedAt: sess.CreatedAt,
					UpdatedAt: time.Now(),
					Metadata:  sess.Metadata,
				}); err != nil {
					slog.Debug("audit save session", "error", err)
				}
			}

			// Clear streaming state
			m.streamThinking = ""
			m.streamResponse = ""
			m.mdBuffer = MarkdownBuffer{}
			m.streamToolGroups = nil
			m.streamStep = 0
			m.streamCh = nil
			m.cancelCh = nil
			m.svc.SetFinishReason(nil) // reset for next turn
			m.scrollOffset = 0       // auto-scroll to bottom on new content
			m.rebuildCollapsibles()

			// /btw mode: discard all messages added to history during streaming
			// so the side query doesn't affect the main conversation context.
			if m.svc.BtwMode() {
				m.svc.TruncateHistory(m.svc.BtwHistoryLen())
				m.svc.SetBtwMode(false)
			}

			// Drain queued steering messages for auto-pickup (single atomic drain)
			if m.steeringTool != nil {
				msgs := m.steeringTool.DrainAll()
				if len(msgs) > 0 {
					parts := make([]string, len(msgs))
					for i, sm := range msgs {
						parts[i] = sm.Content
					}
					nextPrompt := strings.Join(parts, "\n")
					m.messages = append(m.messages, chatMessage{"user", nextPrompt})
					m.svc.IncrementTurn()
					m.streaming = true
					m.streamThinking = ""
					m.streamResponse = ""
					m.mdBuffer = MarkdownBuffer{}
					m.streamToolGroups = nil
					m.streamStep = 0
					m.turnStartTime = time.Now()
					m.messages = append(m.messages, chatMessage{"system", "Thinking..."})
					m.cancelCh = make(chan struct{})
					m.rebuildCollapsibles()
					return m, tea.Batch(m.runLLMStream(nextPrompt, false), m.tickWorking())
				}
			}

			return m, nil
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.quitting {
			return m, nil
		}
		if m.showSessionPicker {
			return m.handleSessionPickerKey(msg)
		}
		if m.showModelPicker {
			return m.handlePickerKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m tuiModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Reset cursor blink on any key
	m.cursorBlink = false
	// Clamp cursor to valid range — guards against stale cursor after
	// input is cleared by commands, model switch, etc.
	m.clampCursor()

	ctrl := msg.Mod&tea.ModCtrl != 0
	switch {
	// ── Quit (Ctrl+C double-press / Ctrl+D) ──
	case msg.Code == 'c' && ctrl:
		// If OAuth login is in progress, cancel it first
		if m.oauthCancel != nil {
			m.oauthCancel()
			m.messages = append(m.messages, chatMessage{"system", "OAuth login cancelled."})
			return m, nil
		}
		if m.ctrlCPending {
			m.quitting = true
			return m, tea.Quit
		}
		m.ctrlCPending = true
		m.messages = append(m.messages, chatMessage{"system", "Press Ctrl+C again to exit."})
		return m, nil
	case msg.Code == 'd' && ctrl:
		m.quitting = true
		return m, tea.Quit

	// ── Toggle drawer (Ctrl+T) ──
	case msg.Code == 't' && ctrl:
		m.ctrlCPending = false
		m.showDrawer = !m.showDrawer
		// Reset scroll position when toggling drawer to avoid jumpy viewport
		m.scrollOffset = 0
		return m, nil

	// ── Newline (Alt+Enter / Shift+Enter / Ctrl+J) ──
	case msg.Code == tea.KeyEnter && (msg.Mod&tea.ModAlt != 0 || msg.Mod&tea.ModShift != 0),
		msg.Code == 'j' && ctrl:
		m.ctrlCPending = false
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		runes := []rune(m.input)
		runes = append(runes[:m.cursor], append([]rune{'\n'}, runes[m.cursor:]...)...)
		m.input = string(runes)
		m.cursor++
		m.showSuggestions = false
		return m, nil

	// ── Submit ──
	case msg.Code == tea.KeyEnter:
		m.ctrlCPending = false
		if len(m.fileCandidates) > 0 && m.fileCycleIdx > 0 {
			// Confirm current file candidate without submitting.
			// Input already holds the absolute path (set by Tab);
			// close the suggestion panel so the user can continue
			// editing or press Enter again to submit.
			m.fileCandidates = nil
			m.fileCycleIdx = 0
			m.filePrefix = ""
			m.showSuggestions = false
			return m, nil
		}
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		return m.handleSubmit()

	// ── Open external editor (Ctrl+G) ──
	case msg.Code == 'g' && ctrl:
		m.ctrlCPending = false
		return m, m.launchEditor()

	// ── Readline: Ctrl+A (start of line) ──
	case msg.Code == 'a' && ctrl:
		m.ctrlCPending = false
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		m.cursor = 0
		return m, nil

	// ── Readline: Ctrl+E (end of line) ──
	case msg.Code == 'e' && ctrl:
		m.ctrlCPending = false
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		m.cursor = utf8.RuneCountInString(m.input)
		return m, nil

	// ── Readline: Ctrl+K (kill to end) ──
	case msg.Code == 'k' && ctrl:
		m.ctrlCPending = false
		runes := []rune(m.input)
		m.input = string(runes[:m.cursor])
		m.updateSuggestions()
		return m, nil

	// ── Readline: Ctrl+U (kill to start) ──
	case msg.Code == 'u' && ctrl:
		m.ctrlCPending = false
		runes := []rune(m.input)
		m.input = string(runes[m.cursor:])
		m.cursor = 0
		m.updateSuggestions()
		return m, nil

	// ── Readline: Ctrl+W (delete word backward) ──
	case msg.Code == 'w' && ctrl:
		m.ctrlCPending = false
		m.deleteWordBackward()
		m.updateSuggestions()
		return m, nil

	// ── Readline: Ctrl+B / Alt+B (word back) ──
	case msg.Code == 'b' && ctrl, msg.Mod&tea.ModAlt != 0 && msg.Text == "b":
		m.ctrlCPending = false
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		m.moveWordBackward()
		return m, nil

	// ── Readline: Ctrl+F / Alt+F (word forward) ──
	case msg.Code == 'f' && ctrl, msg.Mod&tea.ModAlt != 0 && msg.Text == "f":
		m.ctrlCPending = false
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		m.moveWordForward()
		return m, nil

	// ── Backspace (delete at cursor) ──
	case msg.Code == tea.KeyBackspace:
		if m.cursor > 0 {
			runes := []rune(m.input)
			runes = append(runes[:m.cursor-1], runes[m.cursor:]...)
			m.input = string(runes)
			m.cursor--
			m.updateSuggestions()
		}
		return m, nil

	// ── Delete (forward) ──
	case msg.Code == tea.KeyDelete:
		runes := []rune(m.input)
		if m.cursor < len(runes) {
			runes = append(runes[:m.cursor], runes[m.cursor+1:]...)
			m.input = string(runes)
			m.updateSuggestions()
		}
		return m, nil

	// ── Left arrow ──
	case msg.Code == tea.KeyLeft:
		if m.cursor > 0 {
			m.cursor--
		}
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		return m, nil

	// ── Right arrow ──
	case msg.Code == tea.KeyRight:
		runeCount := utf8.RuneCountInString(m.input)
		if m.cursor < runeCount {
			m.cursor++
		}
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		return m, nil

	// ── Up arrow ──
	case msg.Code == tea.KeyUp:
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggest--
			if m.selectedSuggest < 0 {
				m.selectedSuggest = len(m.suggestions) - 1
			}
		} else if strings.ContainsRune(m.input, '\n') {
			// Navigate between lines in multi-line input
			m.moveCursorUp()
		} else if m.input == "" {
			// Empty input: scroll content viewport up
			m.scrollOffset++
		} else if m.inputHistory != nil {
			// Navigate input history
			if m.inputHistory.index == -1 {
				m.savedInput = m.input
			}
			if prev, ok := m.inputHistory.Prev(); ok {
				m.input = prev
				m.cursor = utf8.RuneCountInString(m.input)
				m.updateSuggestions()
			}
		}
		return m, nil

	// ── Down arrow ──
	case msg.Code == tea.KeyDown:
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggest++
			if m.selectedSuggest >= len(m.suggestions) {
				m.selectedSuggest = 0
			}
		} else if strings.ContainsRune(m.input, '\n') {
			// Navigate between lines in multi-line input
			m.moveCursorDown()
		} else if m.input == "" {
			// Empty input: scroll content viewport down
			m.scrollOffset--
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		} else if m.inputHistory != nil && m.inputHistory.index >= 0 {
			// Navigate input history (newer)
			if next, ok := m.inputHistory.Next(); ok {
				m.input = next
				m.cursor = utf8.RuneCountInString(m.input)
			} else {
				// Past the end — restore saved input
				m.input = m.savedInput
				m.savedInput = ""
				m.cursor = utf8.RuneCountInString(m.input)
			}
			m.updateSuggestions()
		}
		return m, nil

	// ── Scroll (PgUp / PgDown) ──
	case msg.Code == tea.KeyPgUp:
		visibleH := m.height - 4
		if visibleH < 1 {
			visibleH = 1
		}
		m.scrollOffset += visibleH / 2
		return m, nil
	case msg.Code == tea.KeyPgDown:
		visibleH := m.height - 4
		if visibleH < 1 {
			visibleH = 1
		}
		m.scrollOffset -= visibleH / 2
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		return m, nil

	// ── Tab ──
	case msg.Code == tea.KeyTab:
		if len(m.fileCandidates) > 0 {
			// @ file completion: Tab cycles through candidates.
			// Each press replaces the @… token with the candidate's
			// absolute path and advances to the next candidate.
			idx := m.fileCycleIdx % len(m.fileCandidates)
			candidate := m.fileCandidates[idx]
			m.input = m.filePrefix + candidate.absPath
			m.cursor = utf8.RuneCountInString(m.input)
			m.selectedSuggest = idx
			m.suggestions = m.fileCandidates
			m.showSuggestions = true
			m.fileCycleIdx = idx + 1
		} else if m.showSuggestions && len(m.suggestions) > 0 {
			// autocomplete — preserve text before the '$' trigger (if any)
			// and the '$' / '/' prefix itself.
			selected := m.suggestions[m.selectedSuggest].name
			if idx := findSkillTrigger(m.input); idx >= 0 {
				prefix := m.input[:idx]
				m.input = prefix + "$" + selected
			} else {
				m.input = "/" + selected
			}
			m.cursor = utf8.RuneCountInString(m.input)
			m.showSuggestions = false
		} else {
			// Toggle collapse of focused section
			m.toggleFocusedCollapse()
		}
		return m, nil

	// ── Steering signal (Ctrl+S) ──
	case msg.Code == 's' && ctrl:
		m.ctrlCPending = false
		if m.streaming && m.steeringTool != nil && m.steeringTool.Len() > 0 {
			m.steeringTool.Signal()
			m.messages = append(m.messages, chatMessage{"system",
				"⚡ Steering signal sent — agent will pick up at next breakpoint"})
		}
		return m, nil

	// ── Escape ──
	case msg.Code == tea.KeyEscape:
		if m.streaming {
			// Cancel the current stream
			if m.cancelCh != nil {
				select {
				case <-m.cancelCh:
				default:
					close(m.cancelCh)
				}
			}
			// Clear steering queue on cancel — user explicitly stopped
			if m.steeringTool != nil {
				discarded := m.steeringTool.DrainAll()
				if len(discarded) > 0 {
					m.messages = append(m.messages, chatMessage{"system",
						fmt.Sprintf("🗑️ %d queued steering message(s) discarded", len(discarded))})
				}
			}
			if m.svc.AuditWriter() != nil {
				m.svc.AuditWriter().Record(audit.AuditEvent{SessionID: m.svc.ID(), Type: audit.EvtUserCancel})
			}
			return m, nil
		}
		m.showSuggestions = false
		m.fileCandidates = nil
		m.fileCycleIdx = 0
		m.filePrefix = ""
		return m, nil

	// ── Space ──
	case msg.Code == tea.KeySpace:
		if len(m.fileCandidates) > 0 && m.fileCycleIdx > 0 {
			// Confirm current file completion candidate (only after Tab
			// has cycled at least once). Input already holds the absolute
			// path set by Tab; append a trailing space so the user can
			// continue typing.
			m.fileCandidates = nil
			m.fileCycleIdx = 0
			m.filePrefix = ""
			m.showSuggestions = false
			m.input += " "
			m.cursor = utf8.RuneCountInString(m.input)
			return m, nil
		}
		runes := []rune(m.input)
		runes = append(runes[:m.cursor], append([]rune{' '}, runes[m.cursor:]...)...)
		m.input = string(runes)
		m.cursor++
		m.showSuggestions = false
		return m, nil

	// ── Printable text ──
	case msg.Text != "":
		isPaste := utf8.RuneCountInString(msg.Text) > 5 // paste detection
		for _, r := range msg.Text {
			runes := []rune(m.input)
			runes = append(runes[:m.cursor], append([]rune{r}, runes[m.cursor:]...)...)
			m.input = string(runes)
			m.cursor++
		}
		if !isPaste {
			m.updateSuggestions()
		} else {
			m.fileCandidates = nil
			m.fileCycleIdx = 0
			m.filePrefix = ""
		}
		// Reset history navigation when typing
		if m.inputHistory != nil {
			m.inputHistory.ResetNavigation()
		}
		return m, nil
	}

	return m, nil
}

// ── Readline helpers ──

// clampCursor ensures the cursor position is within [0, len(runes)].
func (m *tuiModel) clampCursor() {
	n := utf8.RuneCountInString(m.input)
	if m.cursor > n {
		m.cursor = n
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// resetDrawerState clears the drawer's accumulated tool log, skill log, and
// plan tracker. Called on /new, /init, /clear, and session resume so the
// drawer never shows stale data from a previous context.
func (m *tuiModel) resetDrawerState() {
	m.drawerToolLog = nil
	m.drawerSkills = nil
	if m.svc.PlanTracker() != nil {
		m.svc.PlanTracker().Clear()
	}
}

// performCompaction runs LLM-based compaction on the conversation when a
// provider is available, falling back to naive truncation otherwise.
// When auto is true, the compaction was triggered automatically by the strategy;
// when false, it was triggered manually via /compact.
// Returns true if compaction succeeded, false otherwise.
func (m *tuiModel) performCompaction(auto bool) bool {
	return m.performCompactionWithInstruction(auto, "")
}

// performCompactionWithInstruction is like performCompaction but accepts an
// optional custom instruction to append to the compaction prompt.
// Returns true if compaction succeeded.
func (m *tuiModel) performCompactionWithInstruction(auto bool, customInstruction string) bool {
	if m.streaming {
		m.messages = append(m.messages, chatMessage{"system", "Cannot compact while streaming is active."})
		return false
	}
	if m.svc.CompletedTurnsLen() <= 2 {
		m.messages = append(m.messages, chatMessage{"system", "Not enough turns to compact (need > 2)."})
		return false
	}

	// Fire PreCompact hook
	m.fireCompactionHooks(true)

	// Build message list for compactor
	var compactMsgs []agentctx.CompactMessage
	for _, msg := range m.messages {
		if msg.role == "user" || msg.role == "assistant" {
			compactMsgs = append(compactMsgs, agentctx.CompactMessage{Role: msg.role, Content: msg.content})
		}
	}

	prefix := ""
	if auto {
		prefix = "[auto] "
	}

	// Try LLM-based compaction if provider is available
	if m.provider != nil {
		compactor := &agentctx.Compactor{}
		generate := m.makeGenerateFunc()
		result, err := compactor.Run(compactMsgs, generate, agentctx.CompactOptions{
			KeepRecentTurns:   agentctx.DefaultKeepRecentTurns,
			CustomInstruction: customInstruction,
		})
		if err == nil {
			// Rewrite context with compacted messages
			m.rewriteContext(result.RewrittenMessages)
			m.messages = append(m.messages, chatMessage{"system",
				fmt.Sprintf("%sCompacted %d turns into LLM summary (%d → %d tokens).\nKept %d recent turns.",
					prefix, result.RemovedTurns, result.OriginalTokens, result.CompactTokens, result.KeptTurns)})
			if m.svc.CompactStrategy() != nil {
				m.svc.CompactStrategy().RecordCompaction(result.CompactTokens)
			}
			// Fire PostCompact hook
			m.fireCompactionHooks(false)
			return true
		}
		// LLM compaction failed, fall through to naive
		m.messages = append(m.messages, chatMessage{"system",
			fmt.Sprintf("LLM compaction failed (%v), falling back to naive truncation.", err)})
	}

	// Naive compaction fallback
	result, err := agentctx.CompactMessages(compactMsgs, agentctx.DefaultKeepRecentTurns)
	if err != nil {
		m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Compaction failed: %s", err)})
		return false
	}

	// Build rewritten messages from naive result: summary + recent turns verbatim
	var rewritten []agentctx.CompactMessage
	rewritten = append(rewritten, agentctx.CompactMessage{Role: "user", Content: result.Summary})
	keepN := result.KeptTurns
	type naiveTurn struct{ user, assistant string }
	var naiveTurns []naiveTurn
	var cur *naiveTurn
	for _, msg := range compactMsgs {
		switch msg.Role {
		case "user":
			if cur != nil {
				naiveTurns = append(naiveTurns, *cur)
			}
			cur = &naiveTurn{user: msg.Content}
		case "assistant":
			if cur != nil {
				cur.assistant = msg.Content
				naiveTurns = append(naiveTurns, *cur)
				cur = nil
			}
		}
	}
	if cur != nil {
		naiveTurns = append(naiveTurns, *cur)
	}
	if keepN > len(naiveTurns) {
		keepN = len(naiveTurns)
	}
	for _, t := range naiveTurns[len(naiveTurns)-keepN:] {
		if t.user != "" {
			rewritten = append(rewritten, agentctx.CompactMessage{Role: "user", Content: t.user})
		}
		if t.assistant != "" {
			rewritten = append(rewritten, agentctx.CompactMessage{Role: "assistant", Content: t.assistant})
		}
	}

	m.rewriteContext(rewritten)
	m.messages = append(m.messages, chatMessage{"system",
		fmt.Sprintf("%sCompacted %d turns into summary (%d → %d tokens).\nKept %d recent turns.\n\n%s",
			prefix, result.RemovedTurns, result.OriginalTokens, result.CompactTokens, result.KeptTurns, result.Summary)})
	if m.svc.CompactStrategy() != nil {
		m.svc.CompactStrategy().RecordCompaction(m.svc.ContextMgr().CurrentUsage())
	}
	// Fire PostCompact hook
	m.fireCompactionHooks(false)
	return true
}

// makeGenerateFunc creates a GenerateFunc that calls the LLM provider
// synchronously for compaction summarization.
func (m *tuiModel) makeGenerateFunc() agentctx.GenerateFunc {
	return func(systemPrompt string, messages []agentctx.CompactMessage) (string, error) {
		// Convert CompactMessages to kosong.Messages
		var history []kosong.Message
		for _, msg := range messages {
			switch msg.Role {
			case "user":
				history = append(history, kosong.CreateUserMessage(msg.Content))
			case "assistant":
				history = append(history, kosong.CreateAssistantMessage(
					[]kosong.ContentPart{kosong.NewTextPart(msg.Content)}, nil))
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		streamed, err := m.provider.Generate(ctx, systemPrompt, nil, history, nil)
		if err != nil {
			return "", fmt.Errorf("provider.Generate: %w", err)
		}

		// Collect streamed parts synchronously
		var textBuf strings.Builder
		for part := range streamed.Parts {
			if part.Type == "text" {
				textBuf.WriteString(part.Text)
			}
		}
		return textBuf.String(), nil
	}
}

// rewriteContext replaces the conversation messages with the compacted set,
// rewrites the LLM history, and resets the context manager.
func (m *tuiModel) rewriteContext(compactedMsgs []agentctx.CompactMessage) {
	// Replace display messages
	m.messages = nil
	for _, msg := range compactedMsgs {
		m.messages = append(m.messages, chatMessage{role: msg.Role, content: msg.Content})
	}

	// Rewrite LLM conversation history to match compacted messages.
	// This is critical: without this, the next LLM call still sends the
	// full uncompacted history, defeating the purpose of compaction.
	var kosongMsgs []kosong.Message
	for _, msg := range compactedMsgs {
		switch msg.Role {
		case "user":
			kosongMsgs = append(kosongMsgs, kosong.CreateUserMessage(msg.Content))
		case "assistant":
			kosongMsgs = append(kosongMsgs, kosong.CreateAssistantMessage(
				[]kosong.ContentPart{kosong.NewTextPart(msg.Content)}, nil))
		}
	}
	m.svc.RewriteHistory(kosongMsgs)

	// Reset context manager and re-record usage from compacted messages
	m.svc.ContextMgr().Reset()
	compactTokens := 0
	for _, msg := range compactedMsgs {
		compactTokens += agentctx.TokenEstimate(msg.Content) + agentctx.TokenEstimate(msg.Role)
	}
	if compactTokens > 0 {
		m.svc.ContextMgr().AddTurnUsage(compactTokens)
	}

	// Clear completed turns (they've been compacted)
	m.svc.ClearTurns()
	m.rebuildCollapsibles()
}

// stripMediaFromHistory removes image_url, audio_url, and video_url content
// parts from all messages in the LLM history. This is used as a last resort
// when 413 overflow persists after compaction (media payloads can be very large).
// Returns the number of media parts stripped.
func (m *tuiModel) stripMediaFromHistory() int {
	history := m.svc.History()
	stripped := 0
	for i, msg := range history {
		if len(msg.Content) == 0 {
			continue
		}
		var filtered []kosong.ContentPart
		for _, part := range msg.Content {
			if part.Type == "image_url" || part.Type == "audio_url" || part.Type == "video_url" {
				stripped++
				continue
			}
			filtered = append(filtered, part)
		}
		if stripped > 0 && len(filtered) != len(msg.Content) {
			history[i].Content = filtered
		}
	}
	if stripped > 0 {
		m.svc.RewriteHistory(history)
	}
	return stripped
}

// fireCompactionHooks fires PreCompact or PostCompact hooks.
func (m *tuiModel) fireCompactionHooks(pre bool) {
	if m.hookEngine == nil {
		return
	}
	event := hooks.PostCompact
	if pre {
		event = hooks.PreCompact
	}
	hookInput := hooks.HookInput{
		Session: &hooks.HookSession{ID: m.svc.ID(), WorkDir: m.cwd},
	}
	m.hookEngine.FireAndForget(context.Background(), event, hookInput)
}

// turnHistoryCount returns the number of kosong.Message entries a completed
// turn occupies in the LLM history (user + assistant + tool results).
func turnHistoryCount(td turnData) int {
	// user message + assistant message + one tool message per tool group
	return 2 + len(td.toolGroups)
}

func (m *tuiModel) deleteWordBackward() {
	runes := []rune(m.input)
	if m.cursor > len(runes) {
		m.cursor = len(runes)
	}
	if m.cursor <= 0 {
		return
	}
	// Skip trailing spaces
	i := m.cursor - 1
	for i >= 0 && runes[i] == ' ' {
		i--
	}
	// Skip word characters
	for i >= 0 && runes[i] != ' ' {
		i--
	}
	deleteEnd := m.cursor
	deleteStart := i + 1
	runes = append(runes[:deleteStart], runes[deleteEnd:]...)
	m.input = string(runes)
	m.cursor = deleteStart
}

func (m *tuiModel) moveWordBackward() {
	runes := []rune(m.input)
	if m.cursor > len(runes) {
		m.cursor = len(runes)
	}
	if m.cursor <= 0 {
		return
	}
	i := m.cursor - 1
	for i >= 0 && runes[i] == ' ' {
		i--
	}
	for i >= 0 && runes[i] != ' ' {
		i--
	}
	m.cursor = i + 1
}

func (m *tuiModel) moveWordForward() {
	runes := []rune(m.input)
	n := len(runes)
	if m.cursor >= n {
		return
	}
	i := m.cursor
	for i < n && runes[i] != ' ' {
		i++
	}
	for i < n && runes[i] == ' ' {
		i++
	}
	m.cursor = i
}

// ── Multi-line cursor helpers ──

// cursorLineCol returns the (line, column) position within the input
// corresponding to the current cursor index.
func (m tuiModel) cursorLineCol() (line, col int) {
	runes := []rune(m.input)
	pos := m.cursor
	if pos > len(runes) {
		pos = len(runes)
	}
	if pos < 0 {
		pos = 0
	}
	line, col = 0, 0
	for i := 0; i < pos; i++ {
		if runes[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return
}

// cursorFromLineCol converts a (line, column) position back to a cursor index.
func (m tuiModel) cursorFromLineCol(targetLine, targetCol int) int {
	runes := []rune(m.input)
	line, col := 0, 0
	for i := range runes {
		if line == targetLine && col == targetCol {
			return i
		}
		if line > targetLine {
			return i
		}
		if runes[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return len(runes)
}

// inputLines returns the input text split into lines.
func (m tuiModel) inputLines() []string {
	if m.input == "" {
		return []string{""}
	}
	return strings.Split(m.input, "\n")
}

// moveCursorUp moves cursor up one line, preserving column as much as possible.
func (m *tuiModel) moveCursorUp() {
	line, col := m.cursorLineCol()
	if line <= 0 {
		return
	}
	targetCol := col
	m.cursor = m.cursorFromLineCol(line-1, targetCol)
}

// moveCursorDown moves cursor down one line, preserving column as much as possible.
func (m *tuiModel) moveCursorDown() {
	line, col := m.cursorLineCol()
	lines := m.inputLines()
	if line >= len(lines)-1 {
		return
	}
	targetCol := col
	m.cursor = m.cursorFromLineCol(line+1, targetCol)
}

// rebuildCollapsibles rebuilds the collapsible sections list from completed turns
// and active streaming state. Preserves expanded states where possible.
func (m *tuiModel) rebuildCollapsibles() {
	// Save old expanded states keyed by (turnIndex, kind)
	oldExpanded := map[string]bool{}
	for _, c := range m.collapsibles {
		key := fmt.Sprintf("%d:%s", c.turnIndex, c.kind)
		oldExpanded[key] = c.expanded
	}

	m.collapsibles = nil

	// Completed turns
	completedTurns := m.svc.CompletedTurns()
	for ti := range completedTurns {
		td := &completedTurns[ti]
		if td.thinking != "" {
			key := fmt.Sprintf("%d:thinking", ti)
			expanded := oldExpanded[key] // default false
			m.collapsibles = append(m.collapsibles, collapsible{kind: "thinking", turnIndex: ti, expanded: expanded})
		}
		if len(td.toolGroups) > 0 {
			key := fmt.Sprintf("%d:tools", ti)
			expanded := oldExpanded[key] // default false
			m.collapsibles = append(m.collapsibles, collapsible{kind: "tools", turnIndex: ti, expanded: expanded})
		}
	}

	// Active streaming turn (turnIndex = len(completedTurns))
	if m.streaming || m.streamThinking != "" || m.streamResponse != "" || len(m.streamToolGroups) > 0 {
		ti := len(completedTurns)
		if m.streamThinking != "" {
			key := fmt.Sprintf("%d:thinking", ti)
			expanded := true // default expanded during streaming
			if old, ok := oldExpanded[key]; ok {
				expanded = old
			}
			m.collapsibles = append(m.collapsibles, collapsible{kind: "thinking", turnIndex: ti, expanded: expanded})
		}
		if len(m.streamToolGroups) > 0 {
			key := fmt.Sprintf("%d:tools", ti)
			expanded := false // default collapsed
			if old, ok := oldExpanded[key]; ok {
				expanded = old
			}
			m.collapsibles = append(m.collapsibles, collapsible{kind: "tools", turnIndex: ti, expanded: expanded})
		}
	}

	// Clamp focus index
	if m.focusIndex >= len(m.collapsibles) {
		m.focusIndex = len(m.collapsibles) - 1
	}
	if m.focusIndex < 0 && len(m.collapsibles) > 0 {
		m.focusIndex = 0
	}
}

// isSectionExpanded returns the expanded state for a given section.
func (m tuiModel) isSectionExpanded(turnIndex int, kind string) bool {
	for _, c := range m.collapsibles {
		if c.turnIndex == turnIndex && c.kind == kind {
			return c.expanded
		}
	}
	return false
}

// isSectionFocused returns true if the given section is currently focused.
func (m tuiModel) isSectionFocused(turnIndex int, kind string) bool {
	if m.focusIndex < 0 || m.focusIndex >= len(m.collapsibles) {
		return false
	}
	c := m.collapsibles[m.focusIndex]
	return c.turnIndex == turnIndex && c.kind == kind
}

// toggleFocusedCollapse toggles the expand/collapse state of the currently focused section.
func (m *tuiModel) toggleFocusedCollapse() {
	if m.focusIndex < 0 || m.focusIndex >= len(m.collapsibles) {
		return
	}
	c := &m.collapsibles[m.focusIndex]
	c.expanded = !c.expanded

	ti := c.turnIndex
	completedTurns := m.svc.CompletedTurns()
	isStreaming := ti >= len(completedTurns)

	switch c.kind {
	case "tools":
		if isStreaming {
			// Toggle streaming tool groups
			for i := range m.streamToolGroups {
				m.streamToolGroups[i].collapsed = !c.expanded
			}
		} else if ti < len(completedTurns) {
			for i := range completedTurns[ti].toolGroups {
				completedTurns[ti].toolGroups[i].collapsed = !c.expanded
			}
			m.svc.RewriteTurns(completedTurns)
		}
	}
}

func (m *tuiModel) updateSuggestions() {
	if idx := findSkillTrigger(m.input); idx >= 0 {
		// $ trigger (at start or after whitespace): skill-only lookup.
		// Filter on the text following the '$'.
		m.fileCandidates = nil // clear any stale file completion state
		m.fileCycleIdx = 0
		m.filePrefix = ""
		filter := strings.ToLower(m.input[idx+1:])
		m.suggestions = nil
		if m.skillCatalog != nil {
			for _, s := range m.skillCatalog.List() {
				if !s.IsUserActivatable() {
					continue
				}
				name := s.Name
				if strings.HasPrefix(strings.ToLower(name), filter) {
					m.suggestions = append(m.suggestions, slashCommand{
						name:    name,
						desc:    truncateDesc(s.Description, 60),
						isSkill: true,
					})
				}
			}
		}
		m.showSuggestions = len(m.suggestions) > 0
		m.selectedSuggest = 0
	} else if idx := findFileTrigger(m.input); idx >= 0 {
		// @ trigger (at start or after whitespace): file/directory lookup.
		query := m.input[idx+1:]
		candidates := listFileCandidates(query, m.cwd)
		m.fileCandidates = candidates
		m.fileCycleIdx = 0
		m.filePrefix = m.input[:idx]
		m.suggestions = candidates
		m.showSuggestions = len(candidates) > 0
		m.selectedSuggest = 0
	} else if strings.HasPrefix(m.input, "/") && func() bool {
		// Don't trigger command completion when the leading '/' is part
		// of a filesystem path (e.g. after @ file completion produces
		// "/usr/local/bin"). A single '/' prefix without further path
		// separators in the first word is treated as a command.
		firstWord := strings.Fields(m.input)[0]
		return strings.Count(firstWord, "/") <= 1
	}() {
		m.fileCandidates = nil // clear stale file completion state
		m.fileCycleIdx = 0
		m.filePrefix = ""
		filter := strings.ToLower(m.input[1:])
		m.suggestions = nil
		// Built-in commands always shown
		for _, cmd := range slashCommands {
			if strings.HasPrefix(cmd.name, filter) {
				m.suggestions = append(m.suggestions, cmd)
			}
		}
		// Skills only shown when filter has at least 2 chars,
		// so "/" alone doesn't flood with 40+ skill entries.
		if len(filter) >= 2 && m.skillCatalog != nil {
			for _, s := range m.skillCatalog.List() {
				if !s.IsUserActivatable() {
					continue
				}
				name := s.SlashName()
				if strings.HasPrefix(strings.ToLower(name), filter) {
					m.suggestions = append(m.suggestions, slashCommand{
						name:    name,
						desc:    truncateDesc(s.Description, 60),
						isSkill: true,
					})
				}
			}
		}
		m.showSuggestions = len(m.suggestions) > 0
		m.selectedSuggest = 0
	} else {
		m.fileCandidates = nil // clear stale file completion state
		m.fileCycleIdx = 0
		m.filePrefix = ""
		m.showSuggestions = false
	}
}

// truncateDesc shortens a skill description for the suggestion dropdown.
func truncateDesc(desc string, maxLen int) string {
	if maxLen < 4 {
		return desc
	}
	if runes := []rune(desc); len(runes) <= maxLen {
		return desc
	} else {
		return string(runes[:maxLen-3]) + "..."
	}
}

// metaInt extracts an integer from a map[string]any value that may be
// stored as float64 (JSON), int, or int64. Returns (value, true) on
// success, (0, false) when the key is missing or has an unexpected type.
func metaInt(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	default:
		return 0, false
	}
}

// formatSessionList renders a human-friendly session list for the /sessions command.
// Each entry shows the session ID (so the user can copy it for `kimi -S <id>`),
// a truncated title, and metadata (turns, tokens, relative time).
func formatSessionList(sessions []*session.SerializedSession) string {
	const maxDisplay = 20
	const maxTitleLen = 50

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d sessions:\n", len(sessions)))
	for i, s := range sessions {
		if i >= maxDisplay {
			sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(sessions)-maxDisplay))
			break
		}

		// Truncate title for readability
		title := s.Title
		if title == "" || title == "Interactive Session" {
			// Fallback: use first_prompt from metadata if available
			if fp, ok := s.Metadata["first_prompt"].(string); ok && fp != "" {
				title = fp
			} else if title == "" {
				title = "(untitled)"
			}
		}
		if runes := []rune(title); len(runes) > maxTitleLen {
			title = string(runes[:maxTitleLen-3]) + "..."
		}

		// Extract metadata (handles float64 from JSON, int/int64 from Go)
		var turns, tokensIn, tokensOut int
		if v, ok := metaInt(s.Metadata, "turns"); ok {
			turns = v
		}
		if v, ok := metaInt(s.Metadata, "tokens_in"); ok {
			tokensIn = v
		}
		if v, ok := metaInt(s.Metadata, "tokens_out"); ok {
			tokensOut = v
		}

		// Sanitize title: strip newlines/tabs that would break display layout
		title = strings.ReplaceAll(title, "\n", " ")
		title = strings.ReplaceAll(title, "\t", " ")

		// Relative time
		var relTime string
		if s.UpdatedAt.IsZero() {
			relTime = "unknown"
		} else {
			ago := time.Since(s.UpdatedAt)
			switch {
			case ago < time.Minute:
				relTime = "just now"
			case ago < time.Hour:
				relTime = fmt.Sprintf("%dm ago", int(ago.Minutes()))
			case ago < 24*time.Hour:
				relTime = fmt.Sprintf("%dh ago", int(ago.Hours()))
			case ago < 7*24*time.Hour:
				relTime = fmt.Sprintf("%dd ago", int(ago.Hours()/24))
			default:
				relTime = s.UpdatedAt.Format("Jan 02")
			}
		}

		// Format: 1. "title" [id] — 5 turns, 1.2K in / 800 out — 2h ago
		if turns > 0 {
			sb.WriteString(fmt.Sprintf("  %d. \"%s\" [%s] \u2014 %d turns, %s in / %s out \u2014 %s\n",
				i+1, title, s.ID, turns,
				agentctx.FormatTokenCount(tokensIn),
				agentctx.FormatTokenCount(tokensOut),
				relTime))
		} else {
			sb.WriteString(fmt.Sprintf("  %d. \"%s\" [%s] \u2014 %s\n",
				i+1, title, s.ID, relTime))
		}
	}
	sb.WriteString("\nUse: kimi -S <session-id> to resume")
	return sb.String()
}

// firstUserPrompt loads the first user message from a session's history.
// Returns empty string if no user message is found or on error.
func firstUserPrompt(store *session.SessionStore, sessionID string) string {
	if store == nil {
		return ""
	}
	ctx := context.Background()
	if err := store.History().Load(ctx, sessionID); err != nil {
		return ""
	}
	for _, msg := range store.History().Messages() {
		if msg.Role == "user" && msg.Content != "" {
			title := msg.Content
			runes := []rune(title)
			if len(runes) > 50 {
				title = string(runes[:47]) + "..."
			}
			return title
		}
	}
	return ""
}

// findSkillTrigger returns the byte index of a '$' skill trigger in input,
// where '$' is valid at the start of the input or immediately after a whitespace
// character. Returns -1 if no valid trigger is found. When multiple triggers
// exist, the leftmost one wins.
func findSkillTrigger(input string) int {
	for i := 0; i < len(input); i++ {
		if input[i] == '$' && (i == 0 || input[i-1] == ' ' || input[i-1] == '\t' || input[i-1] == '\n' || input[i-1] == '\r') {
			return i
		}
	}
	return -1
}

// parseSkillCommand extracts the skill name and arguments from a /skill: or $ invocation.
// The '$' trigger may appear at the start of the input or after whitespace, allowing
// natural embedding such as "please run $dev-cycle fix bugs".
// Input: "/skill:interview-me how to improve" → name="interview-me", args="how to improve"
// Input: "$dev-cycle fix bugs" → name="dev-cycle", args="fix bugs"
// Input: "try $dev-cycle fix bugs" → name="dev-cycle", args="fix bugs"
func parseSkillCommand(input string) (name, args string) {
	var raw string
	if strings.HasPrefix(input, "/skill:") {
		raw = strings.TrimPrefix(input, "/skill:")
	} else if idx := findSkillTrigger(input); idx >= 0 {
		raw = input[idx+1:]
	} else {
		raw = input
	}
	parts := strings.SplitN(raw, " ", 2)
	name = parts[0]
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return name, args
}

// findFileTrigger returns the byte index of an '@' file-completion trigger in
// input, where '@' is valid at the start of the input or immediately after a
// whitespace character. Returns -1 if no valid trigger is found. When multiple
// triggers exist, the leftmost one wins.
func findFileTrigger(input string) int {
	for i := 0; i < len(input); i++ {
		if input[i] == '@' && (i == 0 || input[i-1] == ' ' || input[i-1] == '\t' || input[i-1] == '\n' || input[i-1] == '\r') {
			return i
		}
	}
	return -1
}

// parseFileQuery splits the text after an '@' trigger into the parent directory
// to scan and the filename prefix to filter on.
//
//	query=""        → dir=cwd,           filter=""
//	query="src"     → dir=cwd,           filter="src"
//	query="src/"    → dir=cwd/src,       filter=""
//	query="src/ma"  → dir=cwd/src,       filter="ma"
//	query="/"       → dir="/",          filter=""
//	query="/usr/l"  → dir="/usr",        filter="l"
func parseFileQuery(query, cwd string) (dir, filter string) {
	if strings.HasPrefix(query, "/") {
		// Absolute path lookup.
		if query == "/" {
			return "/", ""
		}
		dir = filepath.Dir(query)
		filter = filepath.Base(query)
		// filepath.Base("/foo/") returns "foo"; we want "" when trailing slash.
		if strings.HasSuffix(query, "/") {
			dir = filepath.Clean(query)
			filter = ""
		}
		return dir, filter
	}
	// Relative path lookup.
	if query == "" {
		return cwd, ""
	}
	if strings.HasSuffix(query, "/") {
		dir = filepath.Join(cwd, filepath.Clean(query))
		return dir, ""
	}
	dir = filepath.Join(cwd, filepath.Dir(query))
	filter = filepath.Base(query)
	return dir, filter
}

// maxFileCandidates caps the number of filesystem entries returned for a
// single completion query to avoid flooding the UI on huge directories.
const maxFileCandidates = 50

// listFileCandidates scans the filesystem for entries matching the given query
// and returns them as []slashCommand suitable for the suggestion dropdown.
// Directories are listed first, followed by files; each group is sorted by
// name. Hidden entries (starting with '.') are included only when filter
// itself starts with '.'.
func listFileCandidates(query, cwd string) []slashCommand {
	dir, filter := parseFileQuery(query, cwd)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []slashCommand{{name: "[permission denied]", isFile: true, absPath: dir}}
	}
	lowerFilter := strings.ToLower(filter)
	includeHidden := strings.HasPrefix(filter, ".")

	// First pass: collect matching directories and files separately.
	// os.ReadDir has already read all entries into memory, so iterating
	// is fast regardless of directory size. The final cap at
	// maxFileCandidates (applied after sorting) limits the result set.
	var dirs, files []slashCommand
	for _, e := range entries {
		name := e.Name()
		if !includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if lowerFilter != "" && !strings.HasPrefix(strings.ToLower(name), lowerFilter) {
			continue
		}
		absPath := filepath.Join(dir, name)
		isDir := e.IsDir()
		// Resolve symlinks: DirEntry.IsDir uses lstat, so symlinks to
		// directories appear as non-dirs. Stat follows the symlink.
		if !isDir {
			if info, statErr := os.Stat(absPath); statErr == nil {
				isDir = info.IsDir()
			}
		}
		entry := slashCommand{
			name:    name,
			absPath: absPath,
			isFile:  true,
			isDir:   isDir,
		}
		if isDir {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	// Sort each group by name (case-insensitive).
	sortByName := func(s []slashCommand) {
		sort.Slice(s, func(i, j int) bool {
			return strings.ToLower(s[i].name) < strings.ToLower(s[j].name)
		})
	}
	sortByName(dirs)
	sortByName(files)
	// Merge: directories first, then files, capped.
	result := append(dirs, files...)
	if len(result) > maxFileCandidates {
		result = result[:maxFileCandidates]
	}
	return result
}

// executeSkill activates a skill and starts streaming its prompt.
func (m tuiModel) executeSkill(s *skill.Skill, skillArgs, displayInput string) (tea.Model, tea.Cmd) {
	m.activeSkill = &activeSkillInfo{name: s.Name, args: skillArgs}
	// Track for drawer (capped to avoid unbounded memory growth)
	m.drawerSkills = append(m.drawerSkills, drawerSkillEntry{name: s.Name, at: time.Now()})
	if len(m.drawerSkills) > 500 {
		m.drawerSkills = m.drawerSkills[len(m.drawerSkills)-500:]
	}
	m.messages = append(m.messages, chatMessage{"user", displayInput})
	m.messages = append(m.messages, chatMessage{"system",
		fmt.Sprintf("Skill loaded: %s", s.Name)})
	m.input = ""
	m.cursor = 0
	m.showSuggestions = false
	// Build prompt: skill body + user args
	prompt := s.Body
	if skillArgs != "" {
		prompt = s.Body + "\n\n---\n\n" + skillArgs
	}
	m.svc.IncrementTurn()
	m.cancelCh = make(chan struct{})
	m.streaming = true
	m.streamThinking = ""
	m.streamResponse = ""
	m.mdBuffer = MarkdownBuffer{}
	m.streamToolGroups = nil
	m.streamStep = 0
	m.turnStartTime = time.Now()
	m.messages = append(m.messages, chatMessage{"system", "Thinking..."})
	m.rebuildCollapsibles()
	return m, tea.Batch(m.runLLMStream(prompt, false), m.tickWorking())
}

func (m tuiModel) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input)
	if input == "" {
		return m, nil
	}
	m.scrollOffset = 0 // reset scroll to bottom on new input

	// Cycle 5: Bash mode — !command runs shell command directly
	if strings.HasPrefix(input, "!") && !strings.HasPrefix(input, "!!") {
		cmdStr := strings.TrimPrefix(input, "!")
		m.messages = append(m.messages, chatMessage{"user", "$ " + cmdStr})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		// Execute shell command
		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Dir = m.cwd
		out, err := cmd.CombinedOutput()
		if err != nil {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Error: %s\n%s", err, string(out))})
		} else {
			m.messages = append(m.messages, chatMessage{"system", truncateOutput(string(out))})
		}
		return m, nil
	}

	// Audit: record slash command
	if strings.HasPrefix(input, "/") {
		if m.svc.AuditWriter() != nil {
			m.svc.AuditWriter().Record(audit.AuditEvent{SessionID: m.svc.ID(), Type: audit.EvtUserCommand, Data: map[string]any{"command": input}})
		}
	}

	// Handle slash commands
	switch {
	case input == "exit" || input == "quit" || input == "/exit":
		m.quitting = true
		return m, tea.Quit

	case input == "/help":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"assistant", commandReg.renderHelp()})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/new":
		newSess, err := m.app.SessionManager.Create(nil, fmt.Sprintf("session_%d", os.Getpid()), "Interactive Session")
		if err == nil {
			// Audit: record session switch and switch BadgerDB
			if m.svc.AuditWriter() != nil {
				m.svc.AuditWriter().Record(audit.AuditEvent{SessionID: m.svc.ID(), Type: audit.EvtUserSessionSwitch, Data: map[string]any{"from": m.svc.ID(), "to": newSess.ID, "reason": "/new"}})
			}
			home, _ := os.UserHomeDir()
			if home != "" {
				m.app.switchAuditStore(newSess.ID, home)
				m.svc.SetAuditWriter(m.app.AuditWriter)
			}
			m.svc.SetSession(newSess)
			m.svc.Reset()
			m.messages = nil
			m.activeSkill = nil
			m.resetDrawerState()
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("New session created: %s", newSess.ID)})
		} else {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Error: %s", err)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/clear":
		m.messages = nil
		m.activeSkill = nil
		m.resetDrawerState()
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/init":
		m.messages = nil
		m.svc.Reset()
		m.activeSkill = nil
		m.resetDrawerState()
		m.rebuildCollapsibles()
		m.messages = append(m.messages, chatMessage{"system", "Session reset to clean state."})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/models":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", "Fetching models from provider..."})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		cfg := m.app.Config
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			models, err := providers.ListProviderModels(ctx, cfg)
			if err != nil {
				return modelsResultMsg{text: fmt.Sprintf("Failed to list models: %s", err)}
			}
			if len(models) == 0 {
				return modelsResultMsg{text: "No models discovered from provider."}
			}
			var lines []string
			for _, m := range models {
				line := m.ID
				if m.OwnedBy != "" {
					line += fmt.Sprintf(" (by %s)", m.OwnedBy)
				}
				lines = append(lines, "  "+line)
			}
			return modelsResultMsg{text: fmt.Sprintf("Discovered %d models:\n%s", len(models), strings.Join(lines, "\n"))}
		}

	case input == "/yolo" || input == "/auto":
		m.yoloMode = !m.yoloMode
		if m.yoloMode {
			m.permChain = permission.YoloChain()
			m.messages = append(m.messages, chatMessage{"system", "YOLO mode enabled. Tool actions will be auto-approved."})
		} else {
			m.permChain = permission.DefaultChain()
			m.messages = append(m.messages, chatMessage{"system", "YOLO mode disabled."})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/plan":
		m.planMode = !m.planMode
		if m.planMode {
			m.messages = append(m.messages, chatMessage{"system", "Plan mode enabled."})
		} else {
			m.messages = append(m.messages, chatMessage{"system", "Plan mode disabled."})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/model":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.openModelPicker()
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/secondary_model"):
		m.messages = append(m.messages, chatMessage{"user", input})
		args := strings.TrimSpace(strings.TrimPrefix(input, "/secondary_model"))
		if args == "" {
			// Show current secondary model
			if m.app.Config.SecondaryModel != nil && m.app.Config.SecondaryModel.Model != "" {
				m.messages = append(m.messages, chatMessage{"system",
					fmt.Sprintf("Secondary model: %s (effort: %s)", m.app.Config.SecondaryModel.Model, m.app.Config.SecondaryModel.DefaultEffort)})
			} else {
				m.messages = append(m.messages, chatMessage{"system", "No secondary model configured. Use /secondary_model <model> to set one."})
			}
		} else {
			// Set secondary model
			if m.app.Config.SecondaryModel == nil {
				m.app.Config.SecondaryModel = &config.SecondaryModelConfig{}
			}
			m.app.Config.SecondaryModel.Model = args
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Secondary model set to: %s", args)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/permission":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system",
			"Permission modes:\n" +
				"  1. yolo    — auto-approve all tool actions\n" +
				"  2. manual  — ask before each tool action (default)\n" +
				"Use /yolo to toggle, or set permission rules in config.toml."})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/settings":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system",
			"Settings are managed via ~/.kimi-code/config.toml (runtime)\n" +
				"and ~/.kimi-code/tui.toml (UI preferences).\n" +
				"Edit and restart to apply changes."})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	// Cycle 1: Session commands
	case input == "/sessions":
		m.messages = append(m.messages, chatMessage{"user", input})
		if m.app.SessionStore == nil {
			m.messages = append(m.messages, chatMessage{"system", "Session store not available."})
		} else {
			m.openSessionPicker()
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/fork":
		m.messages = append(m.messages, chatMessage{"user", input})
		if m.app.SessionStore != nil {
			forked, err := m.app.SessionStore.Fork(context.Background(), m.svc.ID(), "", m.app.SessionManager)
			if err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Fork failed: %s", err)})
			} else {
				m.svc.SetSession(forked)
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Forked session: %s", forked.ID)})
			}
		} else {
			m.messages = append(m.messages, chatMessage{"system", "Session store not available."})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/title"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/title"))
		if args == "" {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Current title: %s", m.svc.Session().Title)})
		} else {
			m.svc.Session().SetTitle(args)
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Title set to: %s", args)})
			// Persist
			if m.app.SessionStore != nil {
				_ = m.app.SessionStore.Save(context.Background(), m.svc.Session())
			}
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/undo"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/undo"))
		n := 1
		if args != "" {
			if parsed, err := strconv.Atoi(args); err == nil && parsed > 0 {
				n = parsed
			}
		}
		if m.app.SessionStore != nil {
			if err := m.app.SessionStore.History().RemoveLastTurns(context.Background(), m.svc.ID(), n); err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Undo failed: %s", err)})
			} else {
				// Rebuild display
				m.messages = nil
				m.svc.ClearTurns()
				m.svc.ClearHistory()
				m.svc.ContextMgr().Reset()
				m.replayHistory()
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Removed last %d turn(s).", n)})
			}
		} else {
			// In-memory undo: remove turns, history messages, display, and context
			turns := m.svc.CompletedTurns()
			removeN := n
			if removeN > len(turns) {
				removeN = len(turns)
			}
			// Count history messages corresponding to the removed turns
			msgCount := 0
			for i := len(turns) - removeN; i < len(turns); i++ {
				msgCount += turnHistoryCount(turns[i])
			}
			if removeN >= len(turns) {
				m.svc.ClearTurns()
			} else {
				m.svc.RewriteTurns(turns[:len(turns)-removeN])
			}
			m.svc.RemoveLastMessages(msgCount)
			m.svc.ContextMgr().RemoveLastNTurns(removeN)
			// Remove display messages for the undone turns.
			// Each turn is a (user, assistant) pair; walk backwards to find boundaries.
			turnBoundaryCount := 0
			cutIdx := 0
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].role == "user" {
					turnBoundaryCount++
					if turnBoundaryCount >= removeN {
						cutIdx = i
						break
					}
				}
			}
			if turnBoundaryCount >= removeN {
				m.messages = m.messages[:cutIdx]
			} else {
				m.messages = nil
			}
			m.rebuildCollapsibles()
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Removed last %d turn(s).", removeN)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/export-md":
		m.messages = append(m.messages, chatMessage{"user", input})
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", m.svc.Session().Title))
		for _, msg := range m.messages {
			switch msg.role {
			case "user":
				sb.WriteString(fmt.Sprintf("## User\n\n%s\n\n", msg.content))
			case "assistant":
				sb.WriteString(fmt.Sprintf("## Assistant\n\n%s\n\n", msg.content))
			}
		}
		m.messages = append(m.messages, chatMessage{"system", "Markdown export:\n" + sb.String()})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	// Cycle 3: Auth commands
	case input == "/login":
		m.messages = append(m.messages, chatMessage{"user", input})
		// Check if already logged in with OAuth
		if _, ok := m.app.Config.Providers["managed:kimi-code"]; ok {
			m.messages = append(m.messages, chatMessage{"system",
				"Already logged in to Kimi Code (OAuth).\n" +
					"To re-authenticate, run /logout first, then /login again."})
		} else {
			m.messages = append(m.messages, chatMessage{"system", "Starting OAuth login..."})
			cmd := m.runOAuthLogin()
			m.input = ""
			m.cursor = 0
			m.showSuggestions = false
			return m, cmd
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/logout":
		m.messages = append(m.messages, chatMessage{"user", input})
		if err := logoutProvider(m.app.Config, m.app.ConfigPath, ""); err != nil {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Logout failed: %s", err)})
		} else {
			m.recreateProvider()
			m.messages = append(m.messages, chatMessage{"system", "Logged out. Authentication removed."})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	// Cycle 4: Info commands
	case input == "/status":
		m.messages = append(m.messages, chatMessage{"user", input})
		provName := m.app.Config.ResolveProviderName()
		status := "not configured"
		if hasAnyAuth(m.app.Config) {
			status = "configured"
		}
		info := fmt.Sprintf("Session:   %s\nModel:     %s\nProvider:  %s (%s)\nTurns:     %d\nContext:   %s\nYOLO:      %v\nPlan:      %v\nSwarm:     %v",
			m.svc.ID(), m.model, provName, status, m.svc.TurnCount(), m.svc.ContextMgr().UsageDisplay(), m.yoloMode, m.planMode, m.swarmEnabled)
		if m.swarmEnabled {
			active := m.swarmRoster.ActiveCount()
			total := m.swarmRoster.Count()
			info += fmt.Sprintf(" (%d/%d agents)", active, total)
		}
		if m.svc.GoalTracker().IsActive() {
			if snap := m.svc.GoalTracker().Current(); snap != nil {
				info += "\nGoal:      " + snap.Objective
			}
		}
		m.messages = append(m.messages, chatMessage{"system", info})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/usage":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Context usage: %s\nTurns: %d", m.svc.ContextMgr().UsageDisplay(), m.svc.TurnCount())})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/version":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", "kimi-code " + m.version})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/feedback":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", "Feedback: https://github.com/visdomtech/kimi-code/issues"})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/mcp":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", "No MCP servers connected."})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/plugins":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", "No plugins loaded."})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/tasks":
		m.messages = append(m.messages, chatMessage{"user", input})
		tasks := m.bgManager.List(false, 20)
		if len(tasks) == 0 {
			m.messages = append(m.messages, chatMessage{"system", "No background tasks running."})
		} else {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("%d background task(s):\n", len(tasks)))
			for _, t := range tasks {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", t.TaskID, t.Status))
			}
			m.messages = append(m.messages, chatMessage{"system", sb.String()})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	// Cycle 4: Config commands
	case input == "/provider" || strings.HasPrefix(input, "/provider "):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/provider"))
		m.messages = append(m.messages, chatMessage{"user", input})
		if args == "" {
			provName := m.app.Config.ResolveProviderName()
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Current provider: %s", provName)})
		} else {
			if _, ok := m.app.Config.Providers[args]; ok {
				m.app.Config.DefaultProvider = args
				m.recreateProvider()
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Switched to provider: %s", args)})
			} else {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Provider %s not found in config.", args)})
			}
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/experiments":
		m.messages = append(m.messages, chatMessage{"user", input})
		if len(m.app.Config.Experimental) == 0 {
			m.messages = append(m.messages, chatMessage{"system", "No experimental flags enabled."})
		} else {
			var sb strings.Builder
			sb.WriteString("Experimental flags:\n")
			for k, v := range m.app.Config.Experimental {
				sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
			}
			m.messages = append(m.messages, chatMessage{"system", sb.String()})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/reload":
		m.messages = append(m.messages, chatMessage{"user", input})
		if loaded, err := config.LoadFromFile(m.app.ConfigPath); err == nil {
			m.app.Config = loaded
			m.recreateProvider()
			m.messages = append(m.messages, chatMessage{"system", "Config reloaded from " + m.app.ConfigPath})
		} else {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Reload failed: %s", err)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/editor":
		// Open external editor for composing a prompt
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, m.launchEditor()

	case strings.HasPrefix(input, "/effort"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/effort"))
		if args == "" {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Current effort: %s. Options: low, medium, high", m.pickerEffort)})
		} else {
			switch strings.ToLower(args) {
			case "low", "medium", "high", "max":
				m.pickerEffort = strings.ToLower(args)
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Thinking effort set to: %s", args)})
			default:
				m.messages = append(m.messages, chatMessage{"system", "Invalid effort. Use: low, medium, high, max"})
			}
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/theme"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/theme"))
		if args == "" {
			m.messages = append(m.messages, chatMessage{"system", "Available themes: dark, light\nCurrent: auto-detected based on terminal background."})
		} else {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Theme %s — restart to apply.", args)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	// Cycle 6: Context management
	case input == "/compact" || strings.HasPrefix(input, "/compact "):
		m.messages = append(m.messages, chatMessage{"user", input})
		customInstruction := strings.TrimSpace(strings.TrimPrefix(input, "/compact"))
		m.performCompactionWithInstruction(false, customInstruction)
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	// Cycle 8: Agent commands
	case strings.HasPrefix(input, "/goal"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/goal"))
		text, clear := goal.ParseGoalCommand(args)
		m.messages = append(m.messages, chatMessage{"user", input})
		if clear {
			if _, _, err := m.svc.GoalTracker().CancelGoal("user"); err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Error clearing goal: %s", err)})
			} else {
				m.messages = append(m.messages, chatMessage{"system", "Goal cleared."})
			}
		} else {
			if _, _, err := m.svc.GoalTracker().CreateGoal(text, "", goal.BudgetLimits{}, "user"); err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Error setting goal: %s", err)})
			} else {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Goal set: %s", text)})
			}
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/swarm":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.swarmEnabled = !m.swarmEnabled
		active := m.swarmRoster.ActiveCount()
		total := m.swarmRoster.Count()
		if m.swarmEnabled {
			msg := fmt.Sprintf("Swarm mode ENABLED.\nActive sub-agents: %d / %d total\nUse /swarm again to disable, or use the AgentSwarm tool to spawn sub-agents.", active, total)
			if total > 0 {
				msg += "\n\nRoster:"
				for _, r := range m.swarmRoster.List() {
					msg += fmt.Sprintf("\n  [%s] %s — %s", r.Status, r.SubagentID, r.Output)
					if r.Error != "" {
						msg += fmt.Sprintf(" (error: %s)", r.Error)
					}
				}
			}
			m.messages = append(m.messages, chatMessage{"system", msg})
		} else {
			if active > 0 {
				m.swarmRoster.AbortAll()
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Swarm mode DISABLED. Aborted %d active sub-agent(s).", active)})
			} else {
				m.messages = append(m.messages, chatMessage{"system", "Swarm mode DISABLED."})
			}
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/btw"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/btw"))
		m.messages = append(m.messages, chatMessage{"user", input})
		if args == "" {
			m.messages = append(m.messages, chatMessage{"system", "Usage: /btw <prompt> \u2014 side query without affecting main context."})
		} else {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Side query: %s\n(BTW mode: query sent without adding to main conversation history.)", args)})
			// Route to LLM without adding to history — save current
			// history length so we can truncate after streaming completes.
			if m.provider != nil {
				m.svc.SetBtwMode(true)
				m.streaming = true
				m.streamThinking = ""
				m.streamResponse = ""
				m.mdBuffer = MarkdownBuffer{}
				m.streamToolGroups = nil
				m.streamStep = 0
				m.turnStartTime = time.Now()
				m.messages = append(m.messages, chatMessage{"system", "Thinking..."})
				m.rebuildCollapsibles()
				return m, tea.Batch(m.runLLMStream(args, false), m.tickWorking())
			}
			m.messages = append(m.messages, chatMessage{"system", "No provider configured."})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/doctor":
		m.messages = append(m.messages, chatMessage{"user", input})
		results := RunDoctor(m.app.Config)
		m.messages = append(m.messages, chatMessage{"system", FormatDoctorResults(results)})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/copy":
		m.messages = append(m.messages, chatMessage{"user", input})
		// Find last assistant message
		var lastResponse string
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" {
				lastResponse = m.messages[i].content
				break
			}
		}
		if lastResponse == "" {
			m.messages = append(m.messages, chatMessage{"system", "No response to copy."})
		} else {
			m.messages = append(m.messages, chatMessage{"system", "Last response copied to clipboard. (requires clipboard tool)"})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/web"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/web"))
		m.messages = append(m.messages, chatMessage{"user", input})
		if args == "" {
			m.messages = append(m.messages, chatMessage{"system", "Usage: /web <query>"})
		} else {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Web search: %s\n(Web search requires provider integration.)", args)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/add-dir"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/add-dir"))
		m.messages = append(m.messages, chatMessage{"user", input})
		if args == "" {
			m.messages = append(m.messages, chatMessage{"system", "Usage: /add-dir <path>"})
		} else {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Added directory: %s", args)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case !strings.HasPrefix(input, "/") && findSkillTrigger(input) >= 0:
		skillName, skillArgs := parseSkillCommand(input)
		if skillName == "" {
			m.messages = append(m.messages, chatMessage{"user", input})
			m.messages = append(m.messages, chatMessage{"system",
				"Usage: $skill-name [args]. Type $ to browse skills."})
			m.input = ""
			m.cursor = 0
			m.showSuggestions = false
			return m, nil
		}
		if m.skillCatalog != nil {
			if s := m.skillCatalog.Get(skillName); s != nil {
				return m.executeSkill(s, skillArgs, input)
			}
		}
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system",
			fmt.Sprintf("Unknown skill: %s. Type $ to see available skills.", skillName)})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/skill:"):
		skillName, skillArgs := parseSkillCommand(input)
		if m.skillCatalog != nil {
			if s := m.skillCatalog.Get(skillName); s != nil {
				return m.executeSkill(s, skillArgs, input)
			}
		}
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system",
			fmt.Sprintf("Unknown skill: %s. Type /skill: to see available skills.", skillName)})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/"):
		rawAfterSlash := strings.TrimPrefix(input, "/")
		// Extract the command name (first word) for sub-skill lookup
		subName := strings.SplitN(rawAfterSlash, " ", 2)[0]
		if m.skillCatalog != nil {
			if s := m.skillCatalog.Get(subName); s != nil && s.IsSubSkill {
				// Extract args after the sub-skill name
				var subArgs string
				if parts := strings.SplitN(rawAfterSlash, " ", 2); len(parts) > 1 {
					subArgs = strings.TrimSpace(parts[1])
				}
				return m.executeSkill(s, subArgs, input)
			}
		}
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Unknown command: %s. Type /help for available commands.", input)})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	default:
		// If streaming, queue the message instead of starting a new turn
		if m.streaming {
			if m.steeringTool != nil {
				m.steeringTool.Enqueue(input)
			}
			m.messages = append(m.messages, chatMessage{"user", input})
			qLen := 0
			if m.steeringTool != nil {
				qLen = m.steeringTool.Len()
			}
			m.messages = append(m.messages, chatMessage{"system",
				fmt.Sprintf("📨 Queued (%d pending, Ctrl+S to steer agent)", qLen)})
			m.input = ""
			m.cursor = 0
			m.showSuggestions = false
			return m, nil
		}

		// Regular prompt — route through LLM provider
		m.activeSkill = nil // clear active skill on regular user input
		// Reset per-turn compaction counter for the new turn
		if m.svc.CompactStrategy() != nil {
			m.svc.CompactStrategy().ResetForTurn()
		}
		// Save to input history
		if m.inputHistory != nil {
			m.inputHistory.Add(input)
			_ = m.inputHistory.Save()
		}
		m.messages = append(m.messages, chatMessage{"user", input})
		m.svc.IncrementTurn()
		// Auto-title session from first user prompt
		if m.svc.Session().Title == "Interactive Session" {
			title := input
			runes := []rune(title)
			if len(runes) > 50 {
				title = string(runes[:47]) + "..."
			}
			m.svc.Session().SetTitle(title)
			// Persist first prompt for session list display
			sess := m.svc.Session()
			if sess.Metadata == nil {
				sess.Metadata = make(map[string]any)
			}
			sess.Metadata["first_prompt"] = title
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false

		// Set up cancel channel for mid-turn interaction
		m.cancelCh = make(chan struct{})

		// Audit: record user input
		if m.svc.AuditWriter() != nil {
			m.svc.AuditWriter().Record(audit.AuditEvent{SessionID: m.svc.ID(), Type: audit.EvtUserInput, Data: map[string]any{"prompt": input, "model": m.model}})
		}

		if m.provider != nil {
			m.streaming = true
			m.streamThinking = ""
			m.streamResponse = ""
			m.mdBuffer = MarkdownBuffer{}
			m.streamToolGroups = nil
			m.streamStep = 0
			m.turnStartTime = time.Now()
			m.messages = append(m.messages, chatMessage{"system", "Thinking..."})
			m.rebuildCollapsibles()
			return m, tea.Batch(m.runLLMStream(input, false), m.tickWorking())
		}

		// Fallback: no provider configured
		m.messages = append(m.messages, chatMessage{"system",
			"No provider configured. Run /login or add to ~/.kimi-code/config.toml:\n" +
				"  [providers.kimi]\n  type = \"kimi\"\n  api_key = \"YOUR_KEY\""})
		return m, nil
	}
}

// ── View ──

func (m tuiModel) View() tea.View {
	if trace.Enabled() {
		t0 := time.Now()
		defer func() {
			trace.Log("render", "view", map[string]any{
				"duration_ms": time.Since(t0).Milliseconds(),
				"streaming":   m.streaming,
				"messages":    len(m.messages),
			})
		}()
	}
	if m.quitting {
		v := tea.NewView("Goodbye!\n")
		v.AltScreen = true
		return v
	}
	if m.showSessionPicker {
		v := tea.NewView(m.renderSessionPicker())
		v.AltScreen = true
		return v
	}
	if m.showModelPicker {
		v := tea.NewView(m.renderModelPicker())
		v.AltScreen = true
		return v
	}
	if m.width == 0 {
		m.width = 80
		m.height = 24
	}

	var b strings.Builder

	// ── Welcome panel (only if no messages) ──
	if len(m.messages) == 0 && !m.streaming {
		b.WriteString(m.renderWelcome())
		b.WriteString("\n")
		b.WriteString(m.renderTip())
		b.WriteString("\n\n")
	}

	// ── Messages (including completed turn data) ──
	b.WriteString(m.renderMessages())

	// ── Active streaming content ──
	if m.streaming {
		b.WriteString(m.renderStreaming())
		// Working indicator with duration and interrupt hint
		if indicator := m.renderWorkingIndicator(); indicator != "" {
			b.WriteString(indicator)
			b.WriteString("\n")
		}
	} else if m.streamThinking != "" || m.streamResponse != "" {
		// Leftover streaming content before finalization
		b.WriteString(m.renderStreaming())
	}

	// ── Autocomplete suggestions (above input) ──
	if m.showSuggestions {
		s := m.renderSuggestions()
		b.WriteString(s)
	}

	// ── Input box ──
	inputRendered := m.renderInput()
	statusBarRendered := m.renderStatusBar()

	// Calculate visible viewport for content.
	// Trim trailing newlines from content to ensure exact line counting.
	// renderMessages() and renderStreaming() append "\n\n" which creates
	// phantom blank lines that push the output taller than m.height.
	contentStr := strings.TrimRight(b.String(), "\n")
	inputH := strings.Count(inputRendered, "\n") + 1
	statusH := 1
	bottomH := inputH + statusH + 1 // +1 for content→input separator
	visibleH := m.height - bottomH
	if visibleH < 1 {
		visibleH = 1
	}

	contentLines := strings.Split(contentStr, "\n")
	totalLines := len(contentLines)

	// Clamp scroll offset
	maxScroll := totalLines - visibleH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	var result strings.Builder
	if m.scrollOffset > 0 && totalLines > visibleH {
		// Scrolled up: show a window of lines
		end := totalLines - m.scrollOffset
		start := end - visibleH
		if start < 0 {
			start = 0
		}
		if end > totalLines {
			end = totalLines
		}
		for i := start; i < end; i++ {
			result.WriteString(contentLines[i])
			result.WriteString("\n")
		}
		// Scroll indicator
		pct := 100
		if maxScroll > 0 {
			pct = (m.scrollOffset * 100) / maxScroll
		}
		result.WriteString(dimStyle.Render(fmt.Sprintf(" \u2191 scroll %d%% (PgUp/PgDn)", pct)))
		result.WriteString("\n")
	} else if totalLines > visibleH {
		// At bottom with overflow: show last visibleH lines (auto-scroll)
		start := totalLines - visibleH
		for i := start; i < totalLines; i++ {
			result.WriteString(contentLines[i])
			result.WriteString("\n")
		}
	} else {
		// At bottom with room: show all content + padding
		for i := 0; i < totalLines; i++ {
			result.WriteString(contentLines[i])
			result.WriteString("\n")
		}
		padLines := visibleH - totalLines
		for j := 0; j < padLines; j++ {
			result.WriteString("\n")
		}
	}

	// ── Drawer split-pane composition ──
	if m.showDrawer && m.width >= 42 {
		contentLinesSlice := strings.Split(strings.TrimRight(result.String(), "\n"), "\n")

		drawerW := m.width * m.drawerWidthPct / 100
		if drawerW < 20 {
			drawerW = 20
		}
		chatW := m.width - drawerW - 1 // -1 for separator
		if chatW < 20 {
			drawerW = m.width - 21
			chatW = 20
		}

		drawerStr := m.renderDrawer(drawerW)
		drawerLines := strings.Split(strings.TrimRight(drawerStr, "\n"), "\n")

		// Zip content and drawer lines; ensure at least visibleH lines for viewport padding
		var splitResult strings.Builder
		maxLines := len(contentLinesSlice)
		if len(drawerLines) > maxLines {
			maxLines = len(drawerLines)
		}
		if maxLines < visibleH {
			maxLines = visibleH
		}

		for i := 0; i < maxLines; i++ {
			var chatLine, drawerLine string
			if i < len(contentLinesSlice) {
				chatLine = contentLinesSlice[i]
			}
			if i < len(drawerLines) {
				drawerLine = drawerLines[i]
			}

			// Truncate or pad chat line to fit
			chatVisible := lipgloss.Width(chatLine)
			if chatVisible > chatW {
				chatLine = truncateToWidth(chatLine, chatW)
			} else if chatVisible < chatW {
				chatLine = chatLine + strings.Repeat(" ", chatW-chatVisible)
			}

			// Truncate drawer line to fit
			drawerVisible := lipgloss.Width(drawerLine)
			if drawerVisible > drawerW {
				drawerLine = truncateToWidth(drawerLine, drawerW)
			} else if drawerVisible < drawerW {
				drawerLine = drawerLine + strings.Repeat(" ", drawerW-drawerVisible)
			}

			splitResult.WriteString(chatLine)
			splitResult.WriteString(dimStyle.Render("│"))
			splitResult.WriteString(drawerLine)
			splitResult.WriteString("\n")
		}

		result.Reset()
		result.WriteString(splitResult.String())
	}

	result.WriteString(inputRendered)
	result.WriteString("\n")
	result.WriteString(statusBarRendered)

	v := tea.NewView(result.String())
	v.AltScreen = true
	return v
}

func (m tuiModel) renderWelcome() string {
	w := m.width
	if w < 24 {
		// Minimal version
		title := boldPrimary.Render("Welcome to Kimi Code!")
		sub := dimStyle.Render("Send /help for help information.")
		return fmt.Sprintf("\n%s\n%s\n  Model: %s\n", title, sub, m.model)
	}

	// Logo + text side by side
	logoW := 0
	for _, l := range logoLines {
		if len(l) > logoW {
			logoW = len(l)
		}
	}
	gap := "  "
	logo0 := primaryStyle.Render(logoLines[0] + strings.Repeat(" ", logoW-len(logoLines[0])))
	logo1 := primaryStyle.Render(logoLines[1] + strings.Repeat(" ", logoW-len(logoLines[1])))

	rightLine0 := boldPrimary.Render("Welcome to Kimi Code!")
	rightLine1 := dimStyle.Render("Send /help for help information.")

	headerLine0 := logo0 + gap + rightLine0
	headerLine1 := logo1 + gap + rightLine1

	labelStyle := dimStyle.Copy()

	// Model line with provider status
	modelLine := m.model
	if m.provider != nil {
		modelLine = m.model + " " + successStyle.Render("✓ connected")
	} else {
		modelLine = m.model + " " + warningStyle.Render("⚠ no API key")
	}

	infoLines := []string{
		labelStyle.Render("Directory: ") + textStyle.Render(m.cwd),
		labelStyle.Render("Session:   ") + textStyle.Render(m.svc.ID()),
		labelStyle.Render("Model:     ") + textStyle.Render(modelLine),
		labelStyle.Render("Version:   ") + textStyle.Render(m.version),
	}

	// Build content lines
	content := []string{
		headerLine0,
		headerLine1,
		"",
	}
	content = append(content, infoLines...)

	// Render with border
	return borderStyle.Width(w - 4).Render(strings.Join(content, "\n"))
}

func (m tuiModel) renderTip() string {
	tip := boldPrimary.Render("✦ ") + textStyle.Render("Use Kimi K3 with High thinking effort – for the best balance between token spend and capability")
	hint := dimStyle.Render("  Run /model to switch to K3 and set thinking effort to High")
	return tip + "\n" + hint
}

func (m tuiModel) renderMessages() string {
	if len(m.messages) == 0 {
		return ""
	}

	var b strings.Builder
	turnIdx := 0
	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString(userStyle.Render("❯ ") + textStyle.Render(msg.content) + "\n\n")
		case "assistant":
			// Find the corresponding completed turn
			var td *turnData
			completedTurns := m.svc.CompletedTurns()
			if turnIdx < len(completedTurns) {
				td = &completedTurns[turnIdx]
			}
			turnIdx++
			if td != nil {
				// Render thinking block
				if td.thinking != "" {
					b.WriteString(m.renderThinkingBlock(td.thinking, false, turnIdx-1))
					b.WriteString("\n")
				}
				// Render tool groups (compact mode for completed turns)
				if len(td.toolGroups) > 0 {
					b.WriteString(m.renderToolGroupsBlock(td.toolGroups, turnIdx-1, true))
					b.WriteString("\n")
				}
			}
			// Render response text with markdown
			if msg.content != "" {
				b.WriteString(renderMarkdown(msg.content, m.width-4) + "\n\n")
			}
		case "system":
			b.WriteString(warningStyle.Render("⚠ ") + dimStyle.Render(msg.content) + "\n\n")
		}
	}
	return b.String()
}

func (m tuiModel) renderSuggestions() string {
	var b strings.Builder
	n := len(m.suggestions)
	if n == 0 {
		return ""
	}

	// Clamp selection to valid range (defensive: list may shrink between events)
	sel := m.selectedSuggest
	if sel < 0 {
		sel = 0
	}
	if sel >= n {
		sel = n - 1
	}

	// Calculate max display width (for alignment) across all suggestions,
	// accounting for the optional [Skill], [Dir], or [File] label prefixes.
	const skillLabelW = 8 // len("[Skill] ")
	const fileLabelW = 7  // len("[Dir]  ") or len("[File] ")
	maxNameW := 0
	for _, s := range m.suggestions {
		w := len(s.name)
		if s.isSkill {
			w += skillLabelW
		} else if s.isFile {
			w += fileLabelW
		}
		if w > maxNameW {
			maxNameW = w
		}
	}

	// Pagination: limit visible items to avoid flooding the screen
	const maxVisible = 10
	start := 0
	end := n
	if n > maxVisible {
		end = maxVisible
		// Scroll window to keep selected item visible
		if sel >= maxVisible {
			start = sel - maxVisible + 1
			end = start + maxVisible
			if end > n {
				end = n
				start = end - maxVisible
			}
		}
	}

	for i := start; i < end; i++ {
		s := m.suggestions[i]
		name := s.name
		// Show label prefix for skill / file entries.
		label := ""
		if s.isSkill {
			label = "[Skill] "
		} else if s.isFile {
			if s.isDir {
				label = "[Dir]  "
			} else {
				label = "[File] "
			}
		}
		// For file entries, show the absolute path in the description column.
		desc := s.desc
		if s.isFile {
			desc = s.absPath
		}
		displayW := len(name) + len(label)
		pad := strings.Repeat(" ", maxNameW-displayW+2)
		if i == sel {
			b.WriteString(fmt.Sprintf("  → %s%s%s%s\n",
				dimStyle.Render(label), strongStyle.Render(name), pad, primaryStyle.Render(desc)))
		} else {
			b.WriteString(fmt.Sprintf("    %s%s%s%s\n",
				dimStyle.Render(label), textStyle.Render(name), pad, dimStyle.Render(desc)))
		}
	}

	// Pagination indicator: (selected+1/total)
	if n > maxVisible {
		b.WriteString(fmt.Sprintf("  (%d/%d)\n", sel+1, n))
	}

	return b.String()
}

func (m tuiModel) renderInput() string {
	prompt := primaryStyle.Bold(true).Render("> ")

	// Calculate available width for input box
	boxW := m.width - 2
	if boxW < 20 {
		boxW = 20
	}

	// Render input with cursor — handle multi-line correctly
	// by placing cursor within the correct line, not across newlines.
	runes := []rune(m.input)
	cursorPos := m.cursor
	if cursorPos > len(runes) {
		cursorPos = len(runes)
	}
	if cursorPos < 0 {
		cursorPos = 0
	}

	// Bar cursor: bright when visible, very dim when blinking off.
	var cursorChar string
	if m.cursorBlink || m.streaming {
		cursorChar = mutedStyle.Render("▏")
	} else {
		cursorChar = primaryStyle.Render("▏")
	}

	// Split input into lines and insert cursor at the right position
	// within the correct line, then style the whole thing at once.
	lines := strings.Split(string(runes), "\n")
	pos := 0
	for i, line := range lines {
		lineRunes := []rune(line)
		lineLen := len(lineRunes)
		if cursorPos >= pos && cursorPos <= pos+lineLen {
			col := cursorPos - pos
			before := string(lineRunes[:col])
			after := ""
			if col < lineLen {
				after = string(lineRunes[col:])
			}
			lines[i] = before + cursorChar + after
			break
		}
		pos += lineLen + 1 // +1 for the newline
	}
	inputWithCursor := textStyle.Render(strings.Join(lines, "\n"))

	style := inputFocusedStyle
	if m.showSuggestions {
		style = inputBorderStyle
	}

	// Queue indicator when streaming with pending messages
	var queueHint string
	if m.streaming && m.steeringTool != nil && m.steeringTool.Len() > 0 {
		queueHint = mutedStyle.Render(fmt.Sprintf(" [%d queued, Ctrl+S to steer] ", m.steeringTool.Len()))
	}

	return style.Width(boxW).Render(queueHint + prompt + inputWithCursor)
}

// ── Streaming render functions ──

// renderStreaming renders the active streaming content: thinking, tools, and response text.
func (m tuiModel) renderStreaming() string {
	var b strings.Builder
	streamTurnIdx := m.svc.CompletedTurnsLen()

	// Thinking block (during streaming, show expanded by default)
	if m.streamThinking != "" {
		b.WriteString(m.renderThinkingBlock(m.streamThinking, true, streamTurnIdx))
		b.WriteString("\n")
	} else if m.streaming {
		// Show spinner while waiting for thinking
		elapsed := time.Since(m.turnStartTime)
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ▸ Thinking… (%.0fs)", elapsed.Seconds())))
		b.WriteString("\n")
	}

	// Tool groups (full display during streaming)
	if len(m.streamToolGroups) > 0 {
		b.WriteString(m.renderToolGroupsBlock(m.streamToolGroups, streamTurnIdx, false))
		b.WriteString("\n")
	}

	// Response text (streaming) — render with markdown
	if m.streamResponse != "" {
		b.WriteString(renderMarkdown(m.streamResponse, m.width-4))
		if m.streaming {
			b.WriteString(dimStyle.Render("▌")) // streaming cursor
		}
		b.WriteString("\n\n")
	}

	return b.String()
}

// renderThinkingBlock renders a collapsible thinking/reasoning block.
func (m tuiModel) renderThinkingBlock(thinking string, streaming bool, turnIndex int) string {
	if thinking == "" {
		return ""
	}

	focused := m.isSectionFocused(turnIndex, "thinking")
	focusPrefix := "  "
	if focused {
		focusPrefix = primaryStyle.Render("❯ ")
	}

	// Available width for body text (accounting for 4-space indent).
	bodyWidth := m.width - 4
	if bodyWidth < 20 {
		bodyWidth = 20
	}

	if streaming {
		expanded := m.isSectionExpanded(turnIndex, "thinking")
		if expanded {
			header := focusPrefix + dimStyle.Render("▾ Thinking…")
			display := wrapText(thinking, bodyWidth)
			// Height-based cap: min(2000 chars, m.height/3 lines)
			maxLines := m.height / 3
			if maxLines < 5 {
				maxLines = 5
			}
			lines := strings.Split(display, "\n")
			if len(lines) > maxLines {
				display = "…\n" + strings.Join(lines[len(lines)-maxLines:], "\n")
			} else if len(display) > 2000 {
				display = "…" + display[len(display)-2000:]
			}
			body := dimStyle.Render(indentText(display, "    "))
			return header + "\n" + body
		}
		return focusPrefix + dimStyle.Render("▸ Thinking…") + mutedStyle.Render(" (Tab to expand)")
	}

	// After completion
	expanded := m.isSectionExpanded(turnIndex, "thinking")
	if expanded {
		header := focusPrefix + dimStyle.Render("▾ Thought")
		body := dimStyle.Render(indentText(wrapText(thinking, bodyWidth), "    "))
		return header + "\n" + body
	}

	// Collapsed
	return focusPrefix + dimStyle.Render("▸ Thought  ") + mutedStyle.Render("(Tab to expand)")
}

// renderToolGroupsBlock renders collapsible tool invocation blocks.
// When compact=true and not expanded, renders a single summary line instead of listing tools.
func (m tuiModel) renderToolGroupsBlock(groups []toolGroup, turnIndex int, compact bool) string {
	if len(groups) == 0 {
		return ""
	}

	focused := m.isSectionFocused(turnIndex, "tools")
	expanded := m.isSectionExpanded(turnIndex, "tools")
	focusPrefix := "  "
	if focused {
		focusPrefix = primaryStyle.Render("❯ ")
	}

	// Compact mode: single summary line for completed turns
	if compact && !expanded {
		return focusPrefix + dimStyle.Render(fmt.Sprintf("▸ %d tool calls", len(groups))) + mutedStyle.Render(" (Tab to expand)")
	}

	var b strings.Builder

	// When collapsed and many tool groups, only show the last 5
	startIdx := 0
	if !expanded && len(groups) > 5 {
		startIdx = len(groups) - 5
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render(fmt.Sprintf("… %d earlier tool calls", startIdx))))
	}

	for i := startIdx; i < len(groups); i++ {
		tg := groups[i]
		if i > startIdx {
			b.WriteString("\n")
		}

		summary := toolArgSummary(tg.args)
		nameStyled := warningStyle.Render(fmt.Sprintf("[%s]", tg.name))
		verb := primaryStyle.Render(toolVerb(tg.name))
		diff := diffStats(tg.name, tg.result)
		metaRaw := formatToolMetaRaw(tg)
		// Combine diff and meta into a single dim suffix to avoid redundant ANSI sequences
		suffix := ""
		if diff != "" || metaRaw != "" {
			suffix = dimStyle.Render(" " + diff + metaRaw)
		}

		if !expanded {
			// Collapsed: single line
			prefix := focusPrefix
			if i > startIdx {
				prefix = "  " // only first visible line gets focus prefix
			}
			line := fmt.Sprintf("%s▸ %s %s%s", prefix, verb, nameStyled, dimStyle.Render(" "+summary))
			if tg.result == "" && !tg.isError {
				line += dimStyle.Render(" ⋯") // running
			}
			line += suffix
			b.WriteString(line)
		} else {
			// Expanded: header + args + result
			prefix := focusPrefix
			if i > startIdx {
				prefix = "  "
			}
			b.WriteString(fmt.Sprintf("%s▾ %s %s%s%s\n", prefix, verb, nameStyled, dimStyle.Render(" "+summary), suffix))
			if tg.args != "" && tg.args != "{}" {
				b.WriteString(dimStyle.Render(indentText(tg.args, "      ")) + "\n")
			}
			if tg.result != "" {
				resultText := tg.result
				if len(resultText) > 300 {
					resultText = resultText[:297] + "..."
				}
				if tg.isError {
					b.WriteString(warningStyle.Render(indentText(resultText, "      ")) + "\n")
				} else {
					b.WriteString(dimStyle.Render(indentText(resultText, "      ")) + "\n")
				}
			}
		}
	}
	if !expanded {
		b.WriteString(mutedStyle.Render(" (Tab to expand)"))
	}
	return b.String()
}

// renderWorkingIndicator renders a status line showing elapsed time and interrupt hint.
func (m tuiModel) renderWorkingIndicator() string {
	if !m.streaming {
		return ""
	}
	elapsed := time.Since(m.turnStartTime)
	duration := formatWorkingDuration(elapsed)
	return dimStyle.Render(fmt.Sprintf("  Working (%s • esc to interrupt)", duration))
}

// wrapText wraps text to fit within maxWidth runes per line.
// Existing newlines are preserved, and long lines are broken at word
// boundaries. Words longer than maxWidth are broken at the boundary.
// Uses rune count rather than byte length for correct CJK handling.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var result []string
	for _, line := range strings.Split(text, "\n") {
		if utf8.RuneCountInString(line) <= maxWidth {
			result = append(result, line)
			continue
		}
		// Break long lines at word boundaries.
		var wrapped []string
		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			if utf8.RuneCountInString(current)+1+utf8.RuneCountInString(word) <= maxWidth {
				current += " " + word
			} else {
				wrapped = append(wrapped, current)
				current = word
			}
		}
		wrapped = append(wrapped, current)
		// Handle words that are themselves longer than maxWidth.
		var finalLines []string
		for _, wl := range wrapped {
			runes := []rune(wl)
			for len(runes) > maxWidth {
				finalLines = append(finalLines, string(runes[:maxWidth]))
				runes = runes[maxWidth:]
			}
			finalLines = append(finalLines, string(runes))
		}
		result = append(result, finalLines...)
	}
	return strings.Join(result, "\n")
}

// indentText indents each line of text by the given prefix.
func indentText(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// formatRelativeTime returns a human-readable relative time string like "2m ago", "30s ago", "1h ago".
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// formatWorkingDuration formats a duration for the working indicator: "14s", "1m 14s", "1h 23m".
func formatWorkingDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	default:
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
}

// formatToolMetaRaw returns a plain-text suffix showing execution duration and
// estimated tokens for a tool call, e.g. " · 1.2s, 456 tokens". The caller is
// responsible for applying styling. Returns "" when there is nothing to display.
func formatToolMetaRaw(tg toolGroup) string {
	if tg.duration == 0 && tg.result == "" && tg.args == "" {
		return ""
	}
	var parts []string
	if tg.duration > 0 {
		secs := tg.duration.Seconds()
		if secs >= 1 {
			parts = append(parts, fmt.Sprintf("%.1fs", secs))
		} else {
			parts = append(parts, fmt.Sprintf("%.0fms", float64(tg.duration.Milliseconds())))
		}
	}
	// Estimate tokens for this tool call (args + result).
	estTokens := agentctx.TokenEstimate(tg.args) + agentctx.TokenEstimate(tg.result)
	if estTokens > 0 {
		parts = append(parts, fmt.Sprintf("%s tokens", agentctx.FormatTokenCount(estTokens)))
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, ", ")
}

// renderDrawer renders the right-side drawer with Progress, Tools, and Skills sections.
func (m tuiModel) renderDrawer(width int) string {
	if m.svc.PlanTracker() == nil {
		return ""
	}
	if width < 20 {
		width = 20
	}
	var b strings.Builder

	// ── Progress section ──
	b.WriteString(primaryStyle.Render("── Progress "))
	b.WriteString(dimStyle.Render(strings.Repeat("─", max(0, width-14))))
	b.WriteString("\n")

	tasks := m.svc.PlanTracker().Tasks()
	if len(tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks"))
		b.WriteString("\n")
	} else {
		for _, task := range tasks {
			var icon string
			switch task.Status {
			case plan.StatusDone:
				icon = primaryStyle.Render("✓")
			case plan.StatusActive:
				icon = boldPrimary.Render("◉")
			case plan.StatusFailed:
				icon = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗")
			default:
				icon = dimStyle.Render("●")
			}
			title := truncateToWidth(task.Title, width-8)
			b.WriteString(fmt.Sprintf("  %s %s\n", icon, textStyle.Render(title)))
		}
		pending, active, done, failed := m.svc.PlanTracker().Counts()
		total := pending + active + done + failed
		summary := fmt.Sprintf("  %d/%d done", done, total)
		if failed > 0 {
			summary += fmt.Sprintf(" (%d failed)", failed)
		}
		b.WriteString(dimStyle.Render(summary))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// ── Tools section ──
	b.WriteString(primaryStyle.Render("── Tools "))
	b.WriteString(dimStyle.Render(strings.Repeat("─", max(0, width-11))))
	b.WriteString("\n")

	if len(m.drawerToolLog) == 0 {
		b.WriteString(dimStyle.Render("  No tool calls"))
		b.WriteString("\n")
	} else {
		// Show last N tool calls that fit in the drawer (compact format)
		maxShow := 10
		start := 0
		if len(m.drawerToolLog) > maxShow {
			start = len(m.drawerToolLog) - maxShow
		}
		for _, entry := range m.drawerToolLog[start:] {
			icon := primaryStyle.Render("✓")
			if entry.isError {
				icon = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗")
			}
			// Compact format: [relative time] [tool name] [duration] [tokens]
			relTime := formatRelativeTime(entry.at)
			line := fmt.Sprintf("  %s %s %s", icon, dimStyle.Render(relTime), textStyle.Render(entry.name))
			if entry.duration > 0 {
				line += " " + dimStyle.Render(fmt.Sprintf("%.1fs", entry.duration.Seconds()))
			}
			if entry.tokens > 0 {
				line += " " + dimStyle.Render(fmt.Sprintf("~%d tok", entry.tokens))
			}
			b.WriteString(line + "\n")
		}

		// Summary at bottom
		counts := make(map[string]int)
		for _, entry := range m.drawerToolLog {
			counts[entry.name]++
		}
		var parts []string
		for name, count := range counts {
			parts = append(parts, fmt.Sprintf("%d %s", count, name))
		}
		sort.Strings(parts)
		b.WriteString(dimStyle.Render("  " + strings.Join(parts, " · ")))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// ── Skills section ──
	b.WriteString(primaryStyle.Render("── Skills "))
	b.WriteString(dimStyle.Render(strings.Repeat("─", max(0, width-12))))
	b.WriteString("\n")

	if len(m.drawerSkills) == 0 {
		b.WriteString(dimStyle.Render("  No skills used"))
		b.WriteString("\n")
	} else {
		// Cap display to last 10 entries to avoid pushing other sections off-screen
		skills := m.drawerSkills
		maxShow := 10
		if len(skills) > maxShow {
			skills = skills[len(skills)-maxShow:]
		}
		for _, s := range skills {
			b.WriteString(fmt.Sprintf("  %s %s\n", dimStyle.Render(s.at.Format("15:04")), textStyle.Render(s.name)))
		}
	}

	return b.String()
}

// truncateToWidth truncates a string to fit within the given display width,
// accounting for wide characters and ANSI escape sequences.
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w <= maxWidth {
		return s
	}
	// Walk runes and track visible width
	var b strings.Builder
	curW := 0
	inEsc := false
	hadEsc := false
	ellipsisW := lipgloss.Width("…")
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			hadEsc = true
			b.WriteRune(r)
			continue
		}
		if inEsc {
			b.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		rW := lipgloss.Width(string(r))
		if curW+rW > maxWidth-ellipsisW {
			b.WriteString("…")
			break
		}
		b.WriteRune(r)
		curW += rW
	}
	// Close any open ANSI escape sequences to prevent color bleed
	if hadEsc {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func (m tuiModel) renderStatusBar() string {
	w := m.width
	if w < 20 {
		w = 20
	}
	// Account for horizontal padding (1 cell each side)
	innerW := w - 2
	if innerW < 1 {
		innerW = 1
	}

	// Left side: model + effort + mode flags
	var left []string
	left = append(left, strongStyle.Render(m.model))
	effort := "high"
	if m.pickerEffort != "" {
		effort = m.pickerEffort
	}
	left = append(left, dimStyle.Render(effort))
	if m.yoloMode {
		left = append(left, warningStyle.Render("yolo"))
	}
	if m.planMode {
		left = append(left, primaryStyle.Render("plan"))
	}
	if m.svc.GoalTracker().IsActive() {
		left = append(left, successStyle.Render("goal"))
	}
	leftStr := strings.Join(left, " ")

	// Right side: context + turns + token breakdown
	right := m.renderTokenStatus()

	// Simple left-right split; right side has priority, left gets truncated if needed
	leftW := lipgloss.Width(leftStr)
	rightW := lipgloss.Width(right)
	available := innerW - rightW

	if available < 2 {
		// Right side already fills the bar — show only right side
		return statusBarStyle.Width(w).Render(right)
	}

	if leftW > available-2 {
		// Left side doesn't fit — truncate with ellipsis
		maxLeft := available - 2
		if maxLeft > 3 {
			runes := []rune(leftStr)
			leftStr = string(runes[:maxLeft-1]) + "…"
		} else if maxLeft > 0 {
			runes := []rune(leftStr)
			leftStr = string(runes[:maxLeft])
		} else {
			leftStr = ""
		}
	}

	// Build final content with gap
	content := leftStr
	if leftStr != "" {
		content += "  "
	}
	content += right

	return statusBarStyle.Width(w).Render(content)
}

// renderTokenStatus returns the right side of the status bar with token breakdown.
func (m tuiModel) renderTokenStatus() string {
	dot := mutedStyle.Render(" · ")
	tu := m.svc.TurnUsage()
	isLive := m.streaming && tu.GrandTotal() > 0

	// Pending estimate is already tracked via SetPendingEstimate in CurrentUsage(),
	// so UsageDisplay() without extra args shows the correct live total.
	ctxStr := m.svc.ContextMgr().UsageDisplay()

	// During streaming, show live current-turn usage
	if isLive {
		u := tu
		parts := []string{"ctx: " + ctxStr}
		parts = append(parts, fmt.Sprintf("in: %s", agentctx.FormatTokenCount(u.InputTotal())))
		parts = append(parts, fmt.Sprintf("out: %s", agentctx.FormatTokenCount(u.Output)))
		if u.InputCacheRead > 0 {
			parts = append(parts, fmt.Sprintf("cache: %s", agentctx.FormatTokenCount(u.InputCacheRead)))
		}
		return dimStyle.Render(strings.Join(parts, dot))
	}

	// Idle: show cumulative session usage
	su := m.svc.SessionUsage()
	if su.GrandTotal() > 0 {
		u := su
		parts := []string{"ctx: " + ctxStr}
		parts = append(parts, fmt.Sprintf("turns: %d", m.svc.TurnCount()))
		parts = append(parts, fmt.Sprintf("in: %s", agentctx.FormatTokenCount(u.InputTotal())))
		parts = append(parts, fmt.Sprintf("out: %s", agentctx.FormatTokenCount(u.Output)))
		if u.InputCacheRead > 0 {
			parts = append(parts, fmt.Sprintf("cache: %s", agentctx.FormatTokenCount(u.InputCacheRead)))
		}
		return dimStyle.Render(strings.Join(parts, dot))
	}

	// Fallback: just context usage
	return dimStyle.Render("ctx: " + ctxStr)
}

// ── Helpers ──

func getGitBranch(cwd string) string {
	headPath := filepath.Join(cwd, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "main"
	}
	line := strings.TrimSpace(string(data))
	if strings.HasPrefix(line, "ref: refs/heads/") {
		return strings.TrimPrefix(line, "ref: refs/heads/")
	}
	if len(line) > 7 {
		return line[:7]
	}
	return line
}

// buildSystemPrompt creates a context-aware system prompt for the LLM.
// It includes environment info, tool usage guidelines, and available skills.
//
// Cache-friendly layout: stable content (role, tool usage, guidelines, skills)
// comes first so the API prefix cache hits across turns and sessions.
// Dynamic env info (cwd, branch, OS) is appended at the tail so that changes
// to those values don't invalidate the cacheable prefix.
func buildSystemPrompt(cwd, branch string, skillCat *skill.Catalog, active *activeSkillInfo, agentsMd string, systemReminders string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// ── Stable prefix (identical across all sessions) ──
	var sb strings.Builder
	sb.WriteString(`You are a helpful AI coding assistant with access to tools for file operations, code search, and shell commands.

## Tool Usage
- Use Read to examine files before editing
- Use Edit for precise string replacements in files
- Use Write to create or overwrite files
- Use Grep to search for patterns in code
- Use Glob to find files matching patterns
- Use Bash for shell commands, running scripts, and system operations

## Guidelines
- Think step by step before making changes
- Read existing code to understand context before editing
- Use specific, targeted edits rather than rewriting entire files
- Explain what you're doing and why
- If a task requires multiple steps, plan them out first
- Use the update_plan tool to track your plan task list. Call it whenever you start, complete, or add tasks during your work.`)

	// ── Skills (stable per session, appended after core) ──
	if skillCat != nil {
		var skillLines []string
		for _, s := range skillCat.List() {
			if !s.IsUserActivatable() {
				continue
			}
			line := fmt.Sprintf("- **%s**: %s", s.Name, s.Description)
			if s.WhenToUse != "" {
				line += fmt.Sprintf(" (use when: %s)", s.WhenToUse)
			}
			skillLines = append(skillLines, line)
		}
		if len(skillLines) > 0 {
			sb.WriteString("\n\n## Available Skills\nThe following skills can be invoked via $skill-name or /skill:skill-name:\n")
			sb.WriteString(strings.Join(skillLines, "\n"))
		}
	}

	// ── Dynamic env tail (changes per session; placed last to preserve cache prefix) ──
	sb.WriteString(fmt.Sprintf("\n\n## Environment\n- Working directory: %s\n- OS: %s/%s\n- Git branch: %s", cwd, osName, arch, branch))

	// ── Active skill guardrail ──
	if active != nil {
		sb.WriteString("\n\n## Active Skill: ")
		sb.WriteString(active.name)
		sb.WriteString("\nYou are currently executing this skill. Follow its workflow phases in order.\n")
		sb.WriteString("Do NOT perform actions outside this skill's scope ")
		sb.WriteString("(e.g., creating branches, pushing, opening PRs) unless the skill explicitly instructs it.\n")
		if active.args != "" {
			sb.WriteString("Arguments: ")
			sb.WriteString(active.args)
			sb.WriteString("\n")
		}
	}

	// ── AGENTS.md prompt injection ──
	if agentsMd != "" {
		sb.WriteString("\n\n## Project Instructions (AGENTS.md)\n")
		sb.WriteString(agentsMd)
	}

	// ── System reminders (dynamic injections: goal, plan mode, etc.) ──
	if systemReminders != "" {
		sb.WriteString("\n\n")
		sb.WriteString(systemReminders)
	}

	return sb.String()
}

// truncateOutput truncates tool output using a head/tail pattern.
// For outputs exceeding 20 lines, it shows the first 10 and last 10 lines
// with a summary of hidden lines in between (matching goose's pattern).
func truncateOutput(output string) string {
	lines := strings.Split(output, "\n")
	const maxLines = 20
	if len(lines) <= maxLines {
		return output
	}
	head := strings.Join(lines[:10], "\n")
	tail := strings.Join(lines[len(lines)-10:], "\n")
	return fmt.Sprintf("%s\n... (%d lines hidden, %d bytes total)\n%s", head, len(lines)-maxLines, len(output), tail)
}

// ── Model Picker ──

func (m *tuiModel) openModelPicker() {
	cfg := m.app.Config
	m.pickerModels = nil
	m.pickerSearch = ""
	m.pickerSelected = 0
	m.pickerFilter = ""

	// Build picker entries from [models] section
	for alias, mc := range cfg.Models {
		name := mc.DisplayName
		if name == "" {
			name = mc.Model
		}
		m.pickerModels = append(m.pickerModels, pickerEntry{
			alias:       alias,
			model:       mc.Model,
			provider:    mc.Provider,
			displayName: name,
			efforts:     mc.Capabilities,
		})
	}

	// Also add providers that have no models entries but have a default_model
	seen := map[string]bool{}
	for _, e := range m.pickerModels {
		seen[e.alias] = true
	}
	for provName, prov := range cfg.Providers {
		if prov.DefaultModel != "" {
			alias := provName + "/" + prov.DefaultModel
			if !seen[alias] {
				m.pickerModels = append(m.pickerModels, pickerEntry{
					alias:       alias,
					model:       prov.DefaultModel,
					provider:    provName,
					displayName: prov.DefaultModel,
				})
			}
		}
	}

	// Set effort from config
	m.pickerEffort = cfg.Thinking.Effort
	if m.pickerEffort == "" {
		m.pickerEffort = "high"
	}

	m.showModelPicker = true
}

// openSessionPicker populates the session list and shows the interactive picker.
func (m *tuiModel) openSessionPicker() {
	// Use FileStore for session listing (audit store is per-session
	// and cannot list across sessions due to per-DB isolation).
	if m.app.SessionStore == nil {
		return
	}
	sessions, err := m.app.SessionStore.ListAll(context.Background())
	if err != nil || len(sessions) == 0 {
		m.messages = append(m.messages, chatMessage{"system", "No saved sessions found."})
		return
	}
	// Backfill titles from history for sessions with generic title
	for _, s := range sessions {
		if s.Title == "Interactive Session" || s.Title == "" {
			if fp := firstUserPrompt(m.app.SessionStore, s.ID); fp != "" {
				if s.Metadata == nil {
					s.Metadata = make(map[string]any)
				}
				s.Metadata["first_prompt"] = fp
				s.Title = fp
			}
		}
	}
	m.sessionPickerList = sessions
	m.sessionPickerSel = 0
	m.showSessionPicker = true
}

// handleSessionPickerKey handles keyboard input for the session picker.
func (m tuiModel) handleSessionPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Code {
	case 'c':
		if msg.Mod&tea.ModCtrl != 0 {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyEscape:
		m.showSessionPicker = false
		return m, nil

	case tea.KeyEnter:
		if len(m.sessionPickerList) > 0 && m.sessionPickerSel < len(m.sessionPickerList) {
			selected := m.sessionPickerList[m.sessionPickerSel]
			m.showSessionPicker = false
			m.resumeSession(selected.ID)
		}
		return m, nil

	case tea.KeyUp:
		if m.sessionPickerSel > 0 {
			m.sessionPickerSel--
		}
		return m, nil

	case tea.KeyDown:
		if m.sessionPickerSel < len(m.sessionPickerList)-1 {
			m.sessionPickerSel++
		}
		return m, nil
	}
	return m, nil
}

// resumeSession loads a session by ID and replaces the current TUI state.
func (m *tuiModel) resumeSession(id string) {
	sess, err := m.app.SessionManager.Resume(id)
	if err != nil {
		m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Failed to resume session: %s", err)})
		return
	}
	// Audit: record session switch BEFORE updating session identity
	// so the event goes to the old session's audit DB.
	oldSessionID := m.svc.ID()
	if m.svc.AuditWriter() != nil {
		m.svc.AuditWriter().Record(audit.AuditEvent{
			SessionID: oldSessionID,
			Type:      audit.EvtUserSessionSwitch,
			Data:      map[string]any{"from": oldSessionID, "to": sess.ID, "reason": "resume"},
		})
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		m.app.switchAuditStore(sess.ID, home)
		m.svc.SetAuditWriter(m.app.AuditWriter)
	}

	// Swap to the resumed session
	m.svc.SetSession(sess)
	m.svc.Reset()
	sess.SetStatus(session.StatusIdle)

	// Reset display state
	m.messages = nil
	m.collapsibles = nil
	m.streamToolGroups = nil
	m.streamResponse = ""
	m.streamThinking = ""
	m.streaming = false
	m.scrollOffset = 0
	m.focusIndex = -1
	m.input = ""
	m.cursor = 0
	m.cancelCh = nil

	m.activeSkill = nil
	m.resetDrawerState()

	// Replay history into the clean state
	m.replayHistory()
	m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Resumed session: %s", sess.Title)})
}

// renderSessionPicker renders the interactive session picker UI.
func (m tuiModel) renderSessionPicker() string {
	w := m.width
	if w < 40 {
		w = 40
	}

	var b strings.Builder

	// Header
	title := boldPrimary.Render("Resume a session")
	subtitle := dimStyle.Render(fmt.Sprintf(" (%d sessions)", len(m.sessionPickerList)))
	b.WriteString(title + subtitle + "\n")
	b.WriteString(dimStyle.Render("\xe2\x86\x91\xe2\x86\x93 navigate \xc2\xb7 Enter resume \xc2\xb7 Esc cancel"))
	b.WriteString("\n\n")

	// Session list with viewport clamping
	sessions := m.sessionPickerList
	maxVisible := 10
	if m.height > 0 {
		maxVisible = m.height - 10
		if maxVisible < 3 {
			maxVisible = 3
		}
	}

	// Clamp selection to local variable (value receiver cannot mutate m)
	sel := m.sessionPickerSel
	if sel >= len(sessions) {
		sel = len(sessions) - 1
	}
	if sel < 0 {
		sel = 0
	}

	start := 0
	if sel >= maxVisible {
		start = sel - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(sessions) {
		end = len(sessions)
	}

	for i := start; i < end; i++ {
		s := sessions[i]
		// Build display title
		displayTitle := s.Title
		if displayTitle == "" || displayTitle == "Interactive Session" {
			if fp, ok := s.Metadata["first_prompt"].(string); ok && fp != "" {
				displayTitle = fp
			} else if displayTitle == "" {
				displayTitle = "(untitled)"
			}
		}
		if runes := []rune(displayTitle); len(runes) > 60 {
			displayTitle = string(runes[:57]) + "..."
		}
		displayTitle = strings.ReplaceAll(displayTitle, "\n", " ")

		// Relative time
		var relTime string
		if s.UpdatedAt.IsZero() {
			relTime = "unknown"
		} else {
			ago := time.Since(s.UpdatedAt)
			switch {
			case ago < time.Minute:
				relTime = "just now"
			case ago < time.Hour:
				relTime = fmt.Sprintf("%dm ago", int(ago.Minutes()))
			case ago < 24*time.Hour:
				relTime = fmt.Sprintf("%dh ago", int(ago.Hours()))
			case ago < 7*24*time.Hour:
				relTime = fmt.Sprintf("%dd ago", int(ago.Hours()/24))
			default:
				relTime = s.UpdatedAt.Format("Jan 02")
			}
		}

		// Turn count from metadata
		var meta string
		if v, ok := metaInt(s.Metadata, "turns"); ok && v > 0 {
			meta = fmt.Sprintf("%d turns", v)
		}
		metaStr := dimStyle.Render(relTime)
		if meta != "" {
			metaStr = dimStyle.Render(meta+" \xc2\xb7 ") + metaStr
		}

		if i == sel {
			arrow := primaryStyle.Render("\xe2\x9d\xaf")
			name := primaryStyle.Copy().Bold(true).Render(displayTitle)
			b.WriteString(fmt.Sprintf("  %s %s  %s\n", arrow, name, metaStr))
		} else {
			b.WriteString(fmt.Sprintf("    %s  %s\n", textStyle.Render(displayTitle), metaStr))
		}
	}

	remaining := len(sessions) - end
	if remaining > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  \xe2\x96\xbc %d more", remaining)))
		b.WriteString("\n")
	} else if len(sessions) == 0 {
		b.WriteString(dimStyle.Render("  No sessions found"))
		b.WriteString("\n")
	}

	return b.String()
}

// filteredPickerModels returns models matching the current search + filter.
func (m tuiModel) filteredPickerModels() []pickerEntry {
	search := strings.ToLower(m.pickerSearch)
	var out []pickerEntry
	for _, e := range m.pickerModels {
		if m.pickerFilter != "" && e.provider != m.pickerFilter {
			continue
		}
		if search != "" {
			name := strings.ToLower(e.displayName)
			model := strings.ToLower(e.model)
			alias := strings.ToLower(e.alias)
			if !strings.Contains(name, search) && !strings.Contains(model, search) && !strings.Contains(alias, search) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// providerNames returns unique provider names for filter tabs.
func (m tuiModel) providerNames() []string {
	seen := map[string]bool{}
	var names []string
	for _, e := range m.pickerModels {
		if !seen[e.provider] {
			seen[e.provider] = true
			names = append(names, e.provider)
		}
	}
	return names
}

func (m tuiModel) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredPickerModels()

	switch msg.Code {
	case 'c':
		if msg.Mod&tea.ModCtrl != 0 {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyEscape:
		m.showModelPicker = false
		return m, nil

	case tea.KeyEnter:
		if len(filtered) > 0 && m.pickerSelected < len(filtered) {
			selected := filtered[m.pickerSelected]
			m.model = selected.displayName
			// Update the default_model in the config for this session
			m.app.Config.DefaultModel = selected.alias
			// Recreate the provider so requests go to the correct endpoint
			// with the right credentials for the selected model's provider.
			m.recreateProvider()
			m.messages = append(m.messages, chatMessage{"system",
				fmt.Sprintf("Switched to model: %s (%s)", selected.displayName, selected.provider)})
		}
		m.showModelPicker = false
		return m, nil

	case tea.KeyTab:
		// Cycle through provider filters: ALL -> prov1 -> prov2 -> ...
		providers := m.providerNames()
		if m.pickerFilter == "" {
			if len(providers) > 0 {
				m.pickerFilter = providers[0]
			}
		} else {
			found := false
			for i, p := range providers {
				if p == m.pickerFilter {
					if i+1 < len(providers) {
						m.pickerFilter = providers[i+1]
					} else {
						m.pickerFilter = ""
					}
					found = true
					break
				}
			}
			if !found {
				m.pickerFilter = ""
			}
		}
		m.pickerSelected = 0
		return m, nil

	case tea.KeyUp:
		if m.pickerSelected > 0 {
			m.pickerSelected--
		}
		return m, nil

	case tea.KeyDown:
		if m.pickerSelected < len(filtered)-1 {
			m.pickerSelected++
		}
		return m, nil

	case tea.KeyLeft:
		// Cycle thinking effort left
		m.cycleEffort(-1)
		return m, nil

	case tea.KeyRight:
		// Cycle thinking effort right
		m.cycleEffort(1)
		return m, nil

	case tea.KeyBackspace:
		if len(m.pickerSearch) > 0 {
			m.pickerSearch = m.pickerSearch[:len(m.pickerSearch)-1]
			m.pickerSelected = 0
		}
		return m, nil

	case tea.KeySpace:
		m.pickerSearch += " "
		m.pickerSelected = 0
		return m, nil

	default:
		// Printable text for picker search
		if msg.Text != "" {
			m.pickerSearch += msg.Text
			m.pickerSelected = 0
		}
		return m, nil
	}
}

func (m *tuiModel) cycleEffort(dir int) {
	efforts := []string{"low", "high", "max"}
	for i, e := range efforts {
		if e == m.pickerEffort {
			j := i + dir
			if j < 0 {
				j = len(efforts) - 1
			} else if j >= len(efforts) {
				j = 0
			}
			m.pickerEffort = efforts[j]
			return
		}
	}
	m.pickerEffort = "high"
}

func (m tuiModel) renderModelPicker() string {
	w := m.width
	if w < 40 {
		w = 40
	}

	var b strings.Builder

	// Header
	title := boldPrimary.Render("Select a model")
	subtitle := dimStyle.Render(" (type to search)")
	header := title + subtitle
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Tab toggle provider \xc2\xb7 \xe2\x86\x91\xe2\x86\x93 navigate \xc2\xb7 Enter select \xc2\xb7 Alt+S session-only \xc2\xb7 Esc cancel"))
	b.WriteString("\n\n")

	// Provider filter tabs
	providers := m.providerNames()
	tabs := []string{}
	allTab := " All "
	if m.pickerFilter == "" {
		allTab = primaryStyle.Copy().Bold(true).Render("[All]")
	} else {
		allTab = dimStyle.Render("All")
	}
	tabs = append(tabs, allTab)
	for _, p := range providers {
		if p == m.pickerFilter {
			tabs = append(tabs, primaryStyle.Copy().Bold(true).Render("["+p+"]"))
		} else {
			tabs = append(tabs, dimStyle.Render(p))
		}
	}
	b.WriteString(strings.Join(tabs, "  "))
	b.WriteString("\n\n")

	// Model list
	filtered := m.filteredPickerModels()
	maxVisible := 10
	if m.height > 0 {
		maxVisible = m.height - 12
		if maxVisible < 3 {
			maxVisible = 3
		}
	}

	// Clamp selection
	if m.pickerSelected >= len(filtered) {
		m.pickerSelected = len(filtered) - 1
	}
	if m.pickerSelected < 0 {
		m.pickerSelected = 0
	}

	start := 0
	if m.pickerSelected >= maxVisible {
		start = m.pickerSelected - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := start; i < end; i++ {
		e := filtered[i]
		label := e.displayName
		if label == "" {
			label = e.model
		}
		provLabel := dimStyle.Render(e.provider)
		if i == m.pickerSelected {
			arrow := primaryStyle.Render("\xe2\x9d\xaf")
			name := primaryStyle.Copy().Bold(true).Render(label)
			b.WriteString(fmt.Sprintf("  %s %s  %s\n", arrow, name, provLabel))
		} else {
			b.WriteString(fmt.Sprintf("    %s  %s\n", textStyle.Render(label), provLabel))
		}
	}

	remaining := len(filtered) - end
	if remaining > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  \xe2\x96\xbc %d more", remaining)))
		b.WriteString("\n")
	} else if len(filtered) == 0 {
		b.WriteString(dimStyle.Render("  No matching models"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Thinking effort selector
	b.WriteString(dimStyle.Render("Thinking (") + dimStyle.Render("\xe2\x86\x90\xe2\x86\x92") + dimStyle.Render(" to switch)"))
	b.WriteString("\n")
	efforts := []string{"Low", "High", "Max"}
	for i, e := range efforts {
		if strings.ToLower(e) == m.pickerEffort {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(primaryStyle.Copy().Bold(true).Render("[ " + e + " ]"))
		} else {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(dimStyle.Render(e))
		}
	}
	b.WriteString("\n")

	return b.String()
}

// ── Simple TUI fallback (for non-interactive stdin) ──

// editorResultMsg is sent when the external editor exits.
type editorResultMsg struct {
	content string
	err     error
}

// launchEditor opens an external text editor for composing a prompt.
// It uses tea.ExecProcess to suspend the TUI while the editor runs.
func (m *tuiModel) launchEditor() tea.Cmd {
	editorCmd := resolveEditorCommand()

	// Build recent context
	var recentCtx string
	if len(m.messages) > 0 {
		var lines []string
		start := len(m.messages) - 6
		if start < 0 {
			start = 0
		}
		for _, msg := range m.messages[start:] {
			content := msg.content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			lines = append(lines, fmt.Sprintf("%s: %s", msg.role, content))
		}
		recentCtx = strings.Join(lines, "\n")
	}

	// Create temp file with template
	f, err := os.CreateTemp("", "kimi-code-prompt-*.md")
	if err != nil {
		return func() tea.Msg {
			return editorResultMsg{err: err}
		}
	}
	tmpPath := f.Name()

	var tmpl strings.Builder
	tmpl.WriteString("# Write your prompt below (lines starting with # are comments):\n\n")
	if m.input != "" {
		tmpl.WriteString(m.input + "\n")
	}
	if recentCtx != "" {
		tmpl.WriteString("\n---\n# Recent conversation:\n")
		for _, line := range strings.Split(recentCtx, "\n") {
			tmpl.WriteString("# " + line + "\n")
		}
	}
	f.WriteString(tmpl.String())
	f.Close()

	// Store path for reading after editor exits
	m.editorTempFile = tmpPath

	// Use tea.ExecProcess to suspend TUI and run editor.
	args := strings.Fields(editorCmd)
	cmd := exec.Command(args[0], append(args[1:], tmpPath)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return editorResultMsg{err: err}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return editorResultMsg{err: readErr}
		}
		// Extract user content: remove comment lines
		var result strings.Builder
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			if trimmed == "---" {
				break
			}
			result.WriteString(line)
			result.WriteString("\n")
		}
		return editorResultMsg{content: strings.TrimSpace(result.String())}
	})
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (a *App) runSimpleTUI(sess *session.Session) error {
	cwd, _ := os.Getwd()
	version := BuildVersion()

	providerStatus := "⚠ no API key"
	if providers.IsConfigured(a.Config) {
		providerStatus = "✓ connected"
	}

	fmt.Println("╭─────────────────────────────────────────────╮")
	fmt.Printf("│  Welcome to Kimi Code! v%-20s│\n", version)
	fmt.Println("│  Send /help for help information.           │")
	fmt.Println("╰─────────────────────────────────────────────╯")
	fmt.Printf("  Directory: %s\n", cwd)
	fmt.Printf("  Session:   %s\n", sess.ID)
	modelName := a.Config.DefaultModel
	if modelName == "" {
		modelName = a.Config.DefaultProvider
	}
	if modelName == "" {
		modelName = "kimi"
	}
	fmt.Printf("  Model:     %s %s\n", modelName, providerStatus)
	fmt.Printf("  Version:   %s\n", version)
	fmt.Println()

	// Create provider for simple mode too
	var provider kosong.ChatProvider
	if providers.IsConfigured(a.Config) {
		p, err := providers.NewFromConfig(a.Config)
		if err == nil {
			provider = p
		}
	}

	// Set up tool registry for simple mode
	toolReg := tools.NewRegistry()
	tools.RegisterDefaultTools(toolReg)
	bgMgr := background.NewManager()
	tools.RegisterBackgroundTools(toolReg, bgMgr)

	// Plan mode tools
	planInjector := injection.NewPlanModeInjector()
	planCtrl := tools.NewPlanModeController(planInjector)
	toolReg.Register(tools.NewEnterPlanModeTool(planCtrl))
	toolReg.Register(tools.NewExitPlanModeTool(planCtrl))

	// SelectTools
	selectTools := tools.NewSelectToolsTool(toolReg)
	toolReg.Register(selectTools)

	// Cron tools
	simpleHome, _ := os.UserHomeDir()
	simpleCronDir := ""
	if simpleHome != "" {
		simpleCronDir = filepath.Join(simpleHome, ".kimi-code")
	}
	cronMgr := cron.NewCronManager(cron.NewStore(simpleCronDir), nil)
	tools.RegisterCronTools(toolReg, cronMgr)

	// Swarm roster and Agent tool
	roster := swarm.NewRoster(nil)
	tools.RegisterAgentTool(toolReg, roster)

	// Goal tools
	goalTrk := goal.NewTracker()
	tools.RegisterGoalTools(toolReg, goalTrk)

	// Register GoGraph tools and hooks when available (opt-out via experimental.gograph=false)
	if a.Config.Experimental["gograph"] != false && tools.IsGoGraphAvailable() {
		runner := tools.NewGoGraphRunner()
		tools.RegisterGoGraphTools(toolReg, runner)
		toolReg.RegisterHook("Grep", tools.NewGoGraphHook(runner))
	}

	permChain := permission.DefaultChain()

	// Create hook engine from config
	var simpleHookEng *hooks.Engine
	if len(a.Config.Hooks) > 0 {
		simpleHookEng = hooks.NewEngine(a.Config.Hooks)
	}

	branch := getGitBranch(skill.FindProjectRoot(cwd))
	// Discover skills for simple mode
	var skillCat *skill.Catalog
	if cat, err := skill.Discover(cwd); err == nil {
		skillCat = cat
	}
	// Register SkillTool after skill discovery
	if skillCat != nil {
		toolReg.Register(tools.NewSkillTool(skillCat, nil))
	}
	systemPrompt := buildSystemPrompt(cwd, branch, skillCat, nil, func() string {
		if simpleHome == "" {
			return ""
		}
		content, _ := promptpkg.LoadAgentsMd(cwd, simpleHome)
		return content
	}(), "")

	scanner := bufio.NewScanner(os.Stdin)
	var history []kosong.Message
	var yoloMode bool

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		switch input {
		case "exit", "quit", "/exit":
			fmt.Println("Goodbye!")
			return nil
		case "/help":
			fmt.Println("Commands: /help, /yolo, /model, /permission, /settings, /new, /clear, /exit")
		case "/model":
			provName, prov := a.Config.ResolveProvider()
			if prov != nil {
				modelName, mc := a.Config.ResolveModel()
				model := "(unknown)"
				if mc != nil {
					model = mc.Model
				}
				fmt.Printf("Current model: %s (provider: %s, type: %s)\n", model, provName, prov.Type)
				if modelName != "" {
					fmt.Printf("  Alias: %s\n", modelName)
				}
			} else {
				fmt.Println("No provider configured.")
			}
		case "/permission":
			fmt.Println("Permission modes: yolo (auto-approve), manual (ask first)")
			fmt.Println("Use /yolo to toggle, or set rules in config.toml.")
		case "/settings":
			fmt.Println("Settings: ~/.kimi-code/config.toml (runtime), ~/.kimi-code/tui.toml (UI)")
		case "/clear":
			history = nil
			fmt.Println("History cleared.")
		case "/new":
			history = nil
			goalTrk.CancelGoal("user")
			fmt.Println("New session started.")
		case "/yolo":
			yoloMode = !yoloMode
			if yoloMode {
				permChain = permission.YoloChain()
				fmt.Println("YOLO mode enabled.")
			} else {
				permChain = permission.DefaultChain()
				fmt.Println("YOLO mode disabled.")
			}
		default:
			if provider != nil {
				history = append(history, kosong.CreateUserMessage(input))
				ctx := context.Background()

				// Convert tool definitions
				var kosongTools []kosong.Tool
				for _, def := range toolReg.Definitions() {
					kosongTools = append(kosongTools, kosong.Tool{
						Name:        def.Name,
						Description: def.Description,
						Parameters:  def.Parameters,
					})
				}

				// Agent loop: keep going until no more tool calls
				for step := 0; step < 25; step++ {
					stream, err := provider.Generate(ctx, systemPrompt, kosongTools, history, nil)
					if err != nil {
						fmt.Printf("⚠ Error: %s\n", err)
						break
					}

					msg, err := kosong.Generate(ctx, stream)
					if err != nil {
						fmt.Printf("⚠ Error: %s\n", err)
						break
					}

					// Print text output
					for _, part := range msg.Content {
						if part.Type == "text" {
							fmt.Print(part.Text)
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
							fmt.Printf("⚠ Tool %q not found\n", tc.Name)
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
							fmt.Printf("[%s] Denied: %s\n", tc.Name, permResult.Reason)
							history = append(history, kosong.CreateToolMessage(tc.ID, fmt.Sprintf("Permission denied: %s", permResult.Reason)))
							continue
						}
						
						// PreToolUse hook (blocking)
						if simpleHookEng != nil {
							hookInput := hooks.HookInput{
								Tool:    &hooks.HookToolInput{Name: tc.Name, Input: string(toolInput)},
								Session: &hooks.HookSession{WorkDir: cwd},
							}
							decision := simpleHookEng.TriggerBlock(ctx, hooks.PreToolUse, hookInput)
							if decision.Blocked {
								fmt.Printf("[%s] Blocked by hook: %s\n", tc.Name, decision.Reason)
								history = append(history, kosong.CreateToolMessage(tc.ID, fmt.Sprintf("[Blocked by hook] %s", decision.Reason)))
								continue
							}
						}
						
						result, err := tool.Execute(ctx, toolInput, tools.ExecContext{WorkDir: cwd})
						if err != nil {
							result = &tools.Result{Output: err.Error(), IsError: true}
						}
						
						// PostToolUse hook (fire-and-forget)
						if simpleHookEng != nil {
							postEvent := hooks.PostToolUse
							if result.IsError {
								postEvent = hooks.PostToolUseFailure
							}
							hookInput := hooks.HookInput{
								Tool:    &hooks.HookToolInput{Name: tc.Name, Input: string(toolInput), Output: result.Output},
								Session: &hooks.HookSession{WorkDir: cwd},
							}
							simpleHookEng.FireAndForget(ctx, postEvent, hookInput)
						}

						fmt.Printf("[%s] %s\n", tc.Name, truncateOutput(result.Output))
						history = append(history, kosong.CreateToolMessage(tc.ID, result.Output))
					}
				}

				fmt.Println()
			} else {
				fmt.Println("⚠ No provider configured. Add to ~/.kimi-code/config.toml")
			}
		}
	}
	fmt.Println("\nGoodbye!")
	return nil
}
