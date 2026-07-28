package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	agentctx "github.com/visdomtech/kimi-code/internal/agentcore/agent/context"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/background"
	"github.com/visdomtech/kimi-code/internal/agentcore/config"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/goal"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/permission"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/skill"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/tools"
	"github.com/visdomtech/kimi-code/internal/agentcore/session"
	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/kosong/providers"
)

// ── Theme (auto-detects light/dark terminal background) ──

type themeColors struct {
	primary    string
	accent     string
	text       string
	textStrong string
	textDim    string
	textMuted  string
	border     string
	success    string
	warning    string
	error      string
	roleUser   string
	statusBarBg string
}

var darkTheme = themeColors{
	primary:    "#4FA8FF",
	accent:     "#5BC0BE",
	text:       "#E0E0E0",
	textStrong: "#F5F5F5",
	textDim:    "#888888",
	textMuted:  "#6B6B6B",
	border:     "#5A5A5A",
	success:    "#4EC87E",
	warning:    "#E8A838",
	error:      "#E85454",
	roleUser:   "#FFCB6B",
	statusBarBg: "#1A1A2E",
}

var lightTheme = themeColors{
	primary:    "#1A5FB4",
	accent:     "#26A269",
	text:       "#3D3846",
	textStrong: "#1E1E2E",
	textDim:    "#77767B",
	textMuted:  "#9A9A9A",
	border:     "#C0BFBC",
	success:    "#26A269",
	warning:    "#E5A50A",
	error:      "#C01C28",
	roleUser:   "#A2734C",
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
	primaryStyle    lipgloss.Style
	boldPrimary     lipgloss.Style
	dimStyle        lipgloss.Style
	textStyle       lipgloss.Style
	strongStyle     lipgloss.Style
	successStyle    lipgloss.Style
	warningStyle    lipgloss.Style
	userStyle       lipgloss.Style
	mutedStyle      lipgloss.Style
	borderStyle     lipgloss.Style
	inputBorderStyle lipgloss.Style
	inputFocusedStyle lipgloss.Style
	statusBarStyle   lipgloss.Style
)

func init() {
	initTheme()
}

func initTheme() {
	t := darkTheme
	if !lipgloss.HasDarkBackground() {
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
		Foreground(lipgloss.Color(t.textDim))
}

// ── Slash commands ──

type slashCommand struct {
	name string
	desc string
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

type tuiModel struct {
	// State
	sessionID string
	version   string
	cwd       string
	model     string
	branch    string
	messages  []chatMessage
	input     string
	cursor    int
	width     int
	height    int
	turnCount int
	yoloMode  bool
	planMode  bool
	quitting  bool
	streaming bool
	sess      *session.Session
	app       *App

	// LLM provider
	provider kosong.ChatProvider
	history  []kosong.Message // conversation history for multi-turn

	// Agent tools
	toolRegistry *tools.Registry
	bgManager    *background.Manager
	permChain    *permission.Chain

	// Autocomplete
	suggestions      []slashCommand
	selectedSuggest  int
	showSuggestions  bool

	// Cursor
	cursorBlink bool

	// Streaming state
	streamCh           chan streamEvent
	streamThinking     string
	streamResponse     string
	mdBuffer           MarkdownBuffer
	streamToolGroups   []toolGroup
	streamStep         int
	responseCursor     int // scroll offset in response view

	// Collapsible sections
	collapsibles   []collapsible
	focusIndex     int // -1 = none
	completedTurns []turnData
	turnStartTime  time.Time

	// Skills
	skillCatalog *skill.Catalog

	// Model picker
	showModelPicker  bool
	pickerSearch     string
	pickerSelected   int
	pickerFilter     string // active provider filter ("" = ALL)
	pickerModels     []pickerEntry
	pickerEffort     string // "low", "high", "max"

	// Cycle 2: Interactive permission prompts
	prompter    *permission.Prompter
	showApproval bool
	pendingApproval *permission.ApprovalRequest

	// Cycle 5: Mid-turn interaction
	cancelCh        chan struct{}
	queuedMessages  []string

	// Cycle 6: Context management
	contextMgr   *agentctx.ContextManager
	turnUsage    kosong.TokenUsage // real API usage for current turn (live during streaming)
	sessionUsage kosong.TokenUsage // cumulative session token usage

	// Cycle 8: Goal tracker
	goalTracker *goal.Tracker

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
	modelName := app.Config.DefaultProvider
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

	permChain := permission.DefaultChain()

	// Initialize input history
	var inputHist *InputHistory
	home, _ := os.UserHomeDir()
	if home != "" {
		inputHist = NewInputHistory(filepath.Join(home, ".kimi-code"))
		_ = inputHist.Load()
	}

	return tuiModel{
		sessionID:    sess.ID,
		version:      version,
		cwd:          cwd,
		model:        modelName,
		branch:       getGitBranch(skill.FindProjectRoot(cwd)),
		sess:         sess,
		app:          app,
		provider:     provider,
		history:      []kosong.Message{},
		skillCatalog: skillCat,
		toolRegistry: toolReg,
		bgManager:    bgMgr,
		permChain:    permChain,
		focusIndex:   -1,
		prompter:     permission.NewPrompter(),
		contextMgr:   agentctx.NewContextManager(0),
		goalTracker:  goal.NewTracker(),
		inputHistory: inputHist,
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
}

// replayHistory loads and replays session history into the TUI display.
func (m *tuiModel) replayHistory() {
	if m.app.SessionStore == nil {
		return
	}
	ctx := context.Background()
	if err := m.app.SessionStore.History().Load(ctx, m.sessionID); err != nil {
		return
	}
	msgs := m.app.SessionStore.History().Messages()
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			m.messages = append(m.messages, chatMessage{role: "user", content: msg.Content})
			m.turnCount++
		case "assistant":
			td := turnData{
				thinking: msg.Thinking,
				text:     msg.Content,
			}
			for _, tc := range msg.ToolCalls {
				td.toolGroups = append(td.toolGroups, toolGroup{
					name:    tc.Name,
					args:    tc.Arguments,
					result:  tc.Result,
					isError: tc.IsError,
					collapsed: true,
				})
			}
			m.completedTurns = append(m.completedTurns, td)
			m.messages = append(m.messages, chatMessage{role: "assistant", content: msg.Content})
			// Rebuild conversation history for multi-turn
			m.history = append(m.history, kosong.CreateUserMessage(msg.Content))
			m.history = append(m.history, kosong.Message{
				Role:    kosong.RoleAssistant,
				Content: []kosong.ContentPart{{Type: "text", Text: msg.Content}},
			})
			// Track token usage
			m.contextMgr.AddTurnUsage(agentctx.TokenEstimate(msg.Content) * 2)
		}
	}
	m.rebuildCollapsibles()
}

// ── Streaming events for bubbletea ──

type streamEvent struct {
	kind     string // "think", "text", "tool_start", "tool_result", "step_done", "done", "error", "usage"
	text     string
	toolName string
	toolArgs string
	toolOut  string
	toolErr  bool
	step     int
	usage    *kosong.TokenUsage
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
}

// collapsible is a focusable UI section that can be expanded/collapsed.
type collapsible struct {
	kind      string // "thinking", "tools", "response"
	expanded  bool
	turnIndex int    // which turn this belongs to
}

// turnData stores the full output of a completed LLM turn.
type turnData struct {
	thinking   string
	text       string
	toolGroups []toolGroup
}

// toolArgSummary extracts a short one-line summary from JSON tool arguments.
func toolArgSummary(args string) string {
	if args == "" || args == "{}" || args == "null" {
		return ""
	}
	// Try to parse as JSON and extract key fields
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		// Not valid JSON, truncate raw
		if len(args) > 40 {
			return args[:37] + "..."
		}
		return args
	}
	var parts []string
	for k, v := range m {
		s := fmt.Sprintf("%v", v)
		if len(s) > 30 {
			s = s[:27] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	result := strings.Join(parts, ", ")
	if len(result) > 60 {
		result = result[:57] + "..."
	}
	return result
}

// runLLMStream is a bubbletea Cmd that streams the LLM response with tool calling.
// It creates a channel of streamEvents and returns a listenStream command.
func (m *tuiModel) runLLMStream(prompt string) tea.Cmd {
	ch := make(chan streamEvent, 64)
	m.streamCh = ch
	m.turnStartTime = time.Now()

	go func() {
		defer close(ch)

		if m.provider == nil {
			ch <- streamEvent{kind: "error", text: "no provider configured. Set API key in ~/.kimi-code/config.toml"}
			return
		}

		// Add user message to history
		m.history = append(m.history, kosong.CreateUserMessage(prompt))

		ctx := context.Background()
		systemPrompt := buildSystemPrompt(m.cwd, m.branch, m.skillCatalog)

		// Convert tool definitions
		var kosongTools []kosong.Tool
		for _, def := range m.toolRegistry.Definitions() {
			kosongTools = append(kosongTools, kosong.Tool{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			})
		}

		maxSteps := 25
		for step := 0; step < maxSteps; step++ {
			stream, err := m.provider.Generate(ctx, systemPrompt, kosongTools, m.history, nil)
			if err != nil {
				ch <- streamEvent{kind: "error", text: err.Error()}
				return
			}

			// Consume stream incrementally, sending events for each part
			var content []kosong.ContentPart
			var toolCalls []kosong.ToolCall
			var pending *kosong.StreamedMessagePart

			for part := range stream.Parts {
				select {
				case <-ctx.Done():
					ch <- streamEvent{kind: "error", text: ctx.Err().Error()}
					return
				default:
				}

				// Send streaming events for incremental UI updates
				switch part.Type {
				case "think":
					ch <- streamEvent{kind: "think", text: part.Think}
				case "text":
					ch <- streamEvent{kind: "text", text: part.Text}
				case "usage":
					ch <- streamEvent{kind: "usage", usage: part.Usage}
				}

				// Merge parts to build final message (same logic as kosong.Generate)
				if pending != nil {
					if kosong.MergeInPlace(pending, &part) {
						continue
					}
					// Flush pending
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
			}

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

			// If no tool calls, we're done
			if len(msg.ToolCalls) == 0 {
				m.history = append(m.history, *msg)
				ch <- streamEvent{kind: "done"}
				return
			}

			m.history = append(m.history, *msg)

			// Execute each tool call
			for _, tc := range msg.ToolCalls {
				var argsStr string
				if tc.Arguments != nil {
					argsStr = *tc.Arguments
				}
				ch <- streamEvent{kind: "tool_start", toolName: tc.Name, toolArgs: argsStr}

				tool, ok := m.toolRegistry.Get(tc.Name)
				if !ok {
					ch <- streamEvent{kind: "tool_result", toolName: tc.Name, toolOut: fmt.Sprintf("tool %q not found", tc.Name), toolErr: true}
					m.history = append(m.history, kosong.CreateToolMessage(tc.ID, fmt.Sprintf("tool %q not found", tc.Name)))
					continue
				}

				var input json.RawMessage
				if tc.Arguments != nil {
					input = json.RawMessage(*tc.Arguments)
				} else {
					input = json.RawMessage("{}")
				}

				// Permission check
				permResult := m.permChain.Evaluate(tc.Name, input)
				if permResult.Decision == permission.DecisionDeny {
					denyMsg := fmt.Sprintf("[Denied] %s", permResult.Reason)
					ch <- streamEvent{kind: "tool_result", toolName: tc.Name, toolOut: denyMsg, toolErr: true}
					m.history = append(m.history, kosong.CreateToolMessage(tc.ID, fmt.Sprintf("Permission denied: %s", permResult.Reason)))
					continue
				}

				result, err := tool.Execute(ctx, input, tools.ExecContext{WorkDir: m.cwd})
				if err != nil {
					result = &tools.Result{Output: err.Error(), IsError: true}
				}

				ch <- streamEvent{kind: "tool_result", toolName: tc.Name, toolOut: truncateOutput(result.Output), toolErr: result.IsError}
				m.history = append(m.history, kosong.CreateToolMessage(tc.ID, result.Output))
			}
		}

		ch <- streamEvent{kind: "done"}
	}()

	return listenStream(ch)
}

// ── Init ──

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(tea.WindowSize(), m.tickCursor())
}

// cursorTickMsg is sent periodically to toggle cursor visibility.
type cursorTickMsg struct{}

func (m tuiModel) tickCursor() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg {
		return cursorTickMsg{}
	})
}

// ── Update ──

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case cursorTickMsg:
		m.cursorBlink = !m.cursorBlink
		return m, m.tickCursor()

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
				name: msg.toolName, args: msg.toolArgs, collapsed: true,
			})
			m.rebuildCollapsibles()
			return m, listenStream(m.streamCh)
		case "tool_result":
			if len(m.streamToolGroups) > 0 {
				last := &m.streamToolGroups[len(m.streamToolGroups)-1]
				last.result = msg.toolOut
				last.isError = msg.toolErr
			}
			return m, listenStream(m.streamCh)
		case "step_done":
			m.streamStep = msg.step
			return m, listenStream(m.streamCh)
		case "error":
			m.streaming = false
			// Remove the "Thinking..." placeholder
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "system" && m.messages[len(m.messages)-1].content == "Thinking..." {
				m.messages = m.messages[:len(m.messages)-1]
			}
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Error: %s", msg.text)})
			return m, nil
		case "usage":
			if msg.usage != nil {
				m.turnUsage = kosong.AddUsage(m.turnUsage, *msg.usage)
			}
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
			m.completedTurns = append(m.completedTurns, td)

			// Append assistant response to messages so it stays visible after streaming
			m.messages = append(m.messages, chatMessage{"assistant", m.streamResponse})

			// Track token usage: prefer real API usage, fall back to estimation
			if m.turnUsage.GrandTotal() > 0 {
				m.contextMgr.AddTurnUsage(m.turnUsage.InputTotal() + m.turnUsage.Output)
				m.sessionUsage = kosong.AddUsage(m.sessionUsage, m.turnUsage)
			} else {
				turnTokens := agentctx.TokenEstimate(m.streamResponse) + agentctx.TokenEstimate(m.streamThinking)
				m.contextMgr.AddTurnUsage(turnTokens)
			}
			m.turnUsage = kosong.TokenUsage{} // reset for next turn

			// Cycle 1: Persist session history
			if m.app.SessionStore != nil {
				var toolCalls []session.ToolCall
				for _, tg := range m.streamToolGroups {
					toolCalls = append(toolCalls, session.ToolCall{
						Name:      tg.name,
						Arguments: tg.args,
						Result:    tg.result,
						IsError:   tg.isError,
					})
				}
				_ = m.app.SessionStore.History().AddTurn(context.Background(), m.sessionID,
					m.messages[len(m.messages)-1].content, // last user message
					m.streamResponse, m.streamThinking, toolCalls)
				m.sess.SetStatus(session.StatusIdle)
				// Persist session summary metadata for /sessions listing
				if m.sess.Metadata == nil {
					m.sess.Metadata = make(map[string]any)
				}
				m.sess.Metadata["turns"] = m.turnCount
				m.sess.Metadata["tokens_in"] = m.sessionUsage.InputTotal()
				m.sess.Metadata["tokens_out"] = m.sessionUsage.Output
				_ = m.app.SessionStore.Save(context.Background(), m.sess)
			}

			// Clear streaming state
			m.streamThinking = ""
			m.streamResponse = ""
			m.mdBuffer = MarkdownBuffer{}
			m.streamToolGroups = nil
			m.streamStep = 0
			m.streamCh = nil
			m.cancelCh = nil
			m.rebuildCollapsibles()

			// Drain queued messages (Cycle 5: mid-turn interaction)
			if len(m.queuedMessages) > 0 {
				nextPrompt := strings.Join(m.queuedMessages, "\n")
				m.queuedMessages = nil
				m.messages = append(m.messages, chatMessage{"user", nextPrompt})
				m.turnCount++
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
				return m, m.runLLMStream(nextPrompt)
			}

			return m, nil
		}
		return m, nil

	case tea.KeyMsg:
		if m.quitting {
			return m, nil
		}
		if m.streaming {
			m.clampCursor() // guard against stale cursor after input clear
			// Allow collapse navigation during streaming
			switch msg.Type {
			case tea.KeyCtrlC:
				m.quitting = true
				return m, tea.Quit
			case tea.KeyEscape:
				// Cancel the current stream
				if m.cancelCh != nil {
					select {
					case <-m.cancelCh:
					default:
						close(m.cancelCh)
					}
				}
				return m, nil
			case tea.KeyTab:
				m.toggleFocusedCollapse()
				return m, nil
			case tea.KeyEnter:
				m.toggleFocusedCollapse()
				return m, nil
			case tea.KeyUp:
				if m.focusIndex > 0 {
					m.focusIndex--
				}
				return m, nil
			case tea.KeyDown:
				if m.focusIndex < len(m.collapsibles)-1 {
					m.focusIndex++
				}
				return m, nil
			case tea.KeyRunes:
				// Queue message during streaming (Cycle 5: mid-turn interaction)
				if msg.String() != "" {
					// Start capturing input for queued message
					for _, r := range msg.Runes {
						runes := []rune(m.input)
						runes = append(runes[:m.cursor], append([]rune{r}, runes[m.cursor:]...)...)
						m.input = string(runes)
						m.cursor++
					}
				}
				return m, nil
			default:
				// Ignore other input during streaming
				return m, nil
			}
		}
		if m.showModelPicker {
			return m.handlePickerKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Reset cursor blink on any key
	m.cursorBlink = false
	// Clamp cursor to valid range — guards against stale cursor after
	// input is cleared by commands, model switch, etc.
	m.clampCursor()

	switch {
	// ── Quit ──
	case msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlD:
		m.quitting = true
		return m, tea.Quit

	// ── Newline (Alt+Enter / Option+Enter) ──
	case msg.Type == tea.KeyEnter && msg.Alt:
		runes := []rune(m.input)
		runes = append(runes[:m.cursor], append([]rune{'\n'}, runes[m.cursor:]...)...)
		m.input = string(runes)
		m.cursor++
		m.showSuggestions = false
		return m, nil

	// ── Submit ──
	case msg.Type == tea.KeyEnter:
		return m.handleSubmit()

	// ── Readline: Ctrl+A (start of line) ──
	case msg.Type == tea.KeyCtrlA:
		m.cursor = 0
		return m, nil

	// ── Readline: Ctrl+E (end of line) ──
	case msg.Type == tea.KeyCtrlE:
		m.cursor = utf8.RuneCountInString(m.input)
		return m, nil

	// ── Readline: Ctrl+K (kill to end) ──
	case msg.Type == tea.KeyCtrlK:
		runes := []rune(m.input)
		m.input = string(runes[:m.cursor])
		return m, nil

	// ── Readline: Ctrl+U (kill to start) ──
	case msg.Type == tea.KeyCtrlU:
		runes := []rune(m.input)
		m.input = string(runes[m.cursor:])
		m.cursor = 0
		return m, nil

	// ── Readline: Ctrl+W (delete word backward) ──
	case msg.Type == tea.KeyCtrlW:
		m.deleteWordBackward()
		return m, nil

	// ── Readline: Alt+B / Ctrl+B (word back) ──
	case msg.Type == tea.KeyCtrlB || (msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && msg.Runes[0] == 'b'):
		m.moveWordBackward()
		return m, nil

	// ── Readline: Alt+F / Ctrl+F (word forward) ──
	case msg.Type == tea.KeyCtrlF || (msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && msg.Runes[0] == 'f'):
		m.moveWordForward()
		return m, nil

	// ── Backspace (delete at cursor) ──
	case msg.Type == tea.KeyBackspace:
		if m.cursor > 0 {
			runes := []rune(m.input)
			runes = append(runes[:m.cursor-1], runes[m.cursor:]...)
			m.input = string(runes)
			m.cursor--
			m.updateSuggestions()
		}
		return m, nil

	// ── Delete (forward) ──
	case msg.Type == tea.KeyDelete:
		runes := []rune(m.input)
		if m.cursor < len(runes) {
			runes = append(runes[:m.cursor], runes[m.cursor+1:]...)
			m.input = string(runes)
			m.updateSuggestions()
		}
		return m, nil

	// ── Left arrow ──
	case msg.Type == tea.KeyLeft:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	// ── Right arrow ──
	case msg.Type == tea.KeyRight:
		runeCount := utf8.RuneCountInString(m.input)
		if m.cursor < runeCount {
			m.cursor++
		}
		return m, nil

	// ── Up arrow ──
	case msg.Type == tea.KeyUp:
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggest--
			if m.selectedSuggest < 0 {
				m.selectedSuggest = len(m.suggestions) - 1
			}
		} else if strings.ContainsRune(m.input, '\n') {
			// Navigate between lines in multi-line input
			m.moveCursorUp()
		} else if m.inputHistory != nil {
			// Navigate input history
			if m.inputHistory.index == -1 {
				m.savedInput = m.input
			}
			if prev, ok := m.inputHistory.Prev(); ok {
				m.input = prev
				m.cursor = utf8.RuneCountInString(m.input)
			}
		}
		return m, nil

	// ── Down arrow ──
	case msg.Type == tea.KeyDown:
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggest++
			if m.selectedSuggest >= len(m.suggestions) {
				m.selectedSuggest = 0
			}
		} else if strings.ContainsRune(m.input, '\n') {
			// Navigate between lines in multi-line input
			m.moveCursorDown()
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
		}
		return m, nil

	// ── Tab ──
	case msg.Type == tea.KeyTab:
		if m.showSuggestions && len(m.suggestions) > 0 {
			// autocomplete
			m.input = "/" + m.suggestions[m.selectedSuggest].name
			m.cursor = utf8.RuneCountInString(m.input)
			m.showSuggestions = false
		} else {
			// Toggle collapse of focused section
			m.toggleFocusedCollapse()
		}
		return m, nil

	case msg.Type == tea.KeyEscape:
		m.showSuggestions = false
		return m, nil

	case msg.Type == tea.KeyRunes:
		isPaste := len(msg.Runes) > 5 // paste detection: many runes at once
		for _, r := range msg.Runes {
			runes := []rune(m.input)
			runes = append(runes[:m.cursor], append([]rune{r}, runes[m.cursor:]...)...)
			m.input = string(runes)
			m.cursor++
		}
		if !isPaste {
			m.updateSuggestions()
		}
		// Reset history navigation when typing
		if m.inputHistory != nil {
			m.inputHistory.ResetNavigation()
		}
		return m, nil

	case msg.Type == tea.KeySpace:
		runes := []rune(m.input)
		runes = append(runes[:m.cursor], append([]rune{' '}, runes[m.cursor:]...)...)
		m.input = string(runes)
		m.cursor++
		m.showSuggestions = false
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
	for ti := range m.completedTurns {
		td := &m.completedTurns[ti]
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
		ti := len(m.completedTurns)
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
	isStreaming := ti >= len(m.completedTurns)

	switch c.kind {
	case "tools":
		if isStreaming {
			// Toggle streaming tool groups
			for i := range m.streamToolGroups {
				m.streamToolGroups[i].collapsed = !c.expanded
			}
		} else if ti < len(m.completedTurns) {
			for i := range m.completedTurns[ti].toolGroups {
				m.completedTurns[ti].toolGroups[i].collapsed = !c.expanded
			}
		}
	}
}

func (m *tuiModel) updateSuggestions() {
	if strings.HasPrefix(m.input, "/") {
		filter := strings.ToLower(m.input[1:])
		m.suggestions = nil
		// Built-in commands first
		for _, cmd := range slashCommands {
			if strings.HasPrefix(cmd.name, filter) {
				m.suggestions = append(m.suggestions, cmd)
			}
		}
		// Then discovered skills (only user-activatable ones)
		if m.skillCatalog != nil {
			for _, s := range m.skillCatalog.List() {
				if !s.IsUserActivatable() {
					continue
				}
				name := s.SlashName()
				if strings.HasPrefix(strings.ToLower(name), filter) {
					m.suggestions = append(m.suggestions, slashCommand{
						name: name,
						desc: truncateDesc(s.Description, 60),
					})
				}
			}
		}
		m.showSuggestions = len(m.suggestions) > 0
		m.selectedSuggest = 0
	} else {
		m.showSuggestions = false
	}
}

// truncateDesc shortens a skill description for the suggestion dropdown.
func truncateDesc(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen-3] + "..."
}

// parseSkillCommand extracts the skill name and arguments from a /skill: invocation.
// Input: "/skill:interview-me how to improve" → name="interview-me", args="how to improve"
func parseSkillCommand(input string) (name, args string) {
	raw := strings.TrimPrefix(input, "/skill:")
	parts := strings.SplitN(raw, " ", 2)
	name = parts[0]
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return name, args
}

func (m tuiModel) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input)
	if input == "" {
		return m, nil
	}

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
			m.sess = newSess
			m.sessionID = newSess.ID
			m.messages = nil
			m.completedTurns = nil
			m.turnCount = 0
			m.history = nil
			m.contextMgr.Reset()
			m.goalTracker.Clear()
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
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/init":
		m.messages = nil
		m.completedTurns = nil
		m.history = nil
		m.turnCount = 0
		m.contextMgr.Reset()
		m.goalTracker.Clear()
		m.rebuildCollapsibles()
		m.messages = append(m.messages, chatMessage{"system", "Session reset to clean state."})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

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
		if m.app.SessionStore != nil {
			sessions, err := m.app.SessionStore.ListAll(context.Background())
			if err != nil || len(sessions) == 0 {
				m.messages = append(m.messages, chatMessage{"system", "No saved sessions found."})
			} else {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("Found %d sessions:\n", len(sessions)))
				for i, s := range sessions {
					if i >= 20 {
						sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(sessions)-20))
						break
					}
					// Extract metadata
					turns := 0
					tokensIn := 0
					tokensOut := 0
					if tc, ok := s.Metadata["turns"].(float64); ok {
						turns = int(tc)
					}
					if ti, ok := s.Metadata["tokens_in"].(float64); ok {
						tokensIn = int(ti)
					}
					if to, ok := s.Metadata["tokens_out"].(float64); ok {
						tokensOut = int(to)
					}
					// Relative time
					ago := time.Since(s.UpdatedAt)
					var relTime string
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
					// Format display line
					if turns > 0 {
						sb.WriteString(fmt.Sprintf("  %d. %q — %d turns, %s in / %s out — %s\n",
							i+1, s.Title, turns,
							agentctx.FormatTokenCount(tokensIn),
							agentctx.FormatTokenCount(tokensOut),
							relTime))
					} else {
						sb.WriteString(fmt.Sprintf("  %d. %q — %s\n", i+1, s.Title, relTime))
					}
				}
				sb.WriteString("\nUse: kimi -S <session-id> to resume")
				m.messages = append(m.messages, chatMessage{"system", sb.String()})
			}
		} else {
			m.messages = append(m.messages, chatMessage{"system", "Session store not available."})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/fork":
		m.messages = append(m.messages, chatMessage{"user", input})
		if m.app.SessionStore != nil {
			forked, err := m.app.SessionStore.Fork(context.Background(), m.sessionID, "", m.app.SessionManager)
			if err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Fork failed: %s", err)})
			} else {
				m.sess = forked
				m.sessionID = forked.ID
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
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Current title: %s", m.sess.Title)})
		} else {
			m.sess.SetTitle(args)
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Title set to: %s", args)})
			// Persist
			if m.app.SessionStore != nil {
				_ = m.app.SessionStore.Save(context.Background(), m.sess)
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
			if err := m.app.SessionStore.History().RemoveLastTurns(context.Background(), m.sessionID, n); err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Undo failed: %s", err)})
			} else {
				// Rebuild display
				m.messages = nil
				m.completedTurns = nil
				m.history = nil
				m.contextMgr.Reset()
				m.replayHistory()
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Removed last %d turn(s).", n)})
			}
		} else {
			// In-memory undo
			if n >= len(m.completedTurns) {
				m.completedTurns = nil
			} else {
				m.completedTurns = m.completedTurns[:len(m.completedTurns)-n]
			}
			m.contextMgr.RemoveLastNTurns(n)
			m.rebuildCollapsibles()
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Removed last %d turn(s) from display.", n)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/export-md":
		m.messages = append(m.messages, chatMessage{"user", input})
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", m.sess.Title))
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
		m.messages = append(m.messages, chatMessage{"system",
			"To set up an API key, run: kimi login\n" +
				"Or manually edit ~/.kimi-code/config.toml:\n" +
				"  [providers.kimi]\n  type = \"kimi\"\n  api_key = \"YOUR_KEY\""})
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
			m.messages = append(m.messages, chatMessage{"system", "Logged out. Provider API key removed."})
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
		if hasAnyAPIKey(m.app.Config) {
			status = "configured"
		}
		info := fmt.Sprintf("Session:   %s\nModel:     %s\nProvider:  %s (%s)\nTurns:     %d\nContext:   %s\nYOLO:      %v\nPlan:      %v",
			m.sessionID, m.model, provName, status, m.turnCount, m.contextMgr.UsageDisplay(), m.yoloMode, m.planMode)
		if m.goalTracker.IsActive() {
			info += "\nGoal:      " + m.goalTracker.Current().Text
		}
		m.messages = append(m.messages, chatMessage{"system", info})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/usage":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Context usage: %s\nTurns: %d", m.contextMgr.UsageDisplay(), m.turnCount)})
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
	case input == "/compact":
		m.messages = append(m.messages, chatMessage{"user", input})
		if len(m.completedTurns) <= 2 {
			m.messages = append(m.messages, chatMessage{"system", "Not enough turns to compact (need > 2)."})
		} else {
			var compactMsgs []agentctx.CompactMessage
			for _, msg := range m.messages {
				if msg.role == "user" || msg.role == "assistant" {
					compactMsgs = append(compactMsgs, agentctx.CompactMessage{Role: msg.role, Content: msg.content})
				}
			}
			result, err := agentctx.CompactMessages(compactMsgs, 2)
			if err != nil {
				m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Compaction failed: %s", err)})
			} else {
				m.messages = append(m.messages, chatMessage{"system",
					fmt.Sprintf("Compacted %d turns into summary (%d → %d tokens).\nKept %d recent turns.\n\n%s",
						result.RemovedTurns, result.OriginalTokens, result.CompactTokens, result.KeptTurns, result.Summary)})
			}
		}
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
			m.goalTracker.Clear()
			m.messages = append(m.messages, chatMessage{"system", "Goal cleared."})
		} else {
			m.goalTracker.Set(text)
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Goal set: %s", text)})
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case input == "/swarm":
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", "Swarm mode: parallel sub-agent execution.\nUsage: Set a goal with /goal, then enable swarm mode.\n(Currently a placeholder — full implementation pending.)"})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	case strings.HasPrefix(input, "/btw"):
		args := strings.TrimSpace(strings.TrimPrefix(input, "/btw"))
		m.messages = append(m.messages, chatMessage{"user", input})
		if args == "" {
			m.messages = append(m.messages, chatMessage{"system", "Usage: /btw <prompt> — side query without affecting main context."})
		} else {
			m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Side query: %s\n(BTW mode: query sent without adding to main conversation history.)", args)})
			// Route to LLM without adding to history
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
				return m, m.runLLMStream(args)
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

	case strings.HasPrefix(input, "/skill:"):
		skillName, skillArgs := parseSkillCommand(input)
		if m.skillCatalog != nil {
			if s := m.skillCatalog.Get(skillName); s != nil {
				m.messages = append(m.messages, chatMessage{"user", input})
				m.messages = append(m.messages, chatMessage{"system",
					fmt.Sprintf("Skill loaded: %s", s.Name)})
				m.input = ""
				m.showSuggestions = false
				// Build prompt: skill body + user args
				prompt := s.Body
				if skillArgs != "" {
					prompt = s.Body + "\n\n---\n\n" + skillArgs
				}
				m.turnCount++
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
				return m, m.runLLMStream(prompt)
			}
		}
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system",
			fmt.Sprintf("Unknown skill: %s. Type / to see available skills.", skillName)})
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
				m.messages = append(m.messages, chatMessage{"user", input})
				m.messages = append(m.messages, chatMessage{"system",
					fmt.Sprintf("Skill loaded: %s", s.Name)})
				m.input = ""
				m.showSuggestions = false
				// Build prompt: skill body + user args
				prompt := s.Body
				if subArgs != "" {
					prompt = s.Body + "\n\n---\n\n" + subArgs
				}
				m.turnCount++
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
				return m, m.runLLMStream(prompt)
			}
		}
		m.messages = append(m.messages, chatMessage{"user", input})
		m.messages = append(m.messages, chatMessage{"system", fmt.Sprintf("Unknown command: %s. Type /help for available commands.", input)})
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false
		return m, nil

	default:
		// Regular prompt — route through LLM provider
		// Save to input history
		if m.inputHistory != nil {
			m.inputHistory.Add(input)
			_ = m.inputHistory.Save()
		}
		m.messages = append(m.messages, chatMessage{"user", input})
		m.turnCount++
		// Auto-title session from first user prompt
		if m.sess.Title == "Interactive Session" && m.turnCount == 1 {
			title := input
			runes := []rune(title)
			if len(runes) > 50 {
				title = string(runes[:47]) + "..."
			}
			m.sess.SetTitle(title)
		}
		m.input = ""
		m.cursor = 0
		m.showSuggestions = false

		// Set up cancel channel for mid-turn interaction
		m.cancelCh = make(chan struct{})

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
			return m, m.runLLMStream(input)
		}

		// Fallback: no provider configured
		m.messages = append(m.messages, chatMessage{"system",
			"No provider configured. Run /login or add to ~/.kimi-code/config.toml:\n" +
				"  [providers.kimi]\n  type = \"kimi\"\n  api_key = \"YOUR_KEY\""})
		return m, nil
	}
}

// ── View ──

func (m tuiModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	if m.showModelPicker {
		return m.renderModelPicker()
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
	} else if m.streamThinking != "" || m.streamResponse != "" {
		// Leftover streaming content before finalization
		b.WriteString(m.renderStreaming())
	}

	// ── Autocomplete suggestions (above input) ──
	suggestLines := 0
	if m.showSuggestions {
		s := m.renderSuggestions()
		b.WriteString(s)
		suggestLines = strings.Count(s, "\n")
	}

	// ── Input box ──
	inputRendered := m.renderInput()
	statusBarRendered := m.renderStatusBar()

	// Calculate padding to push input+status to screen bottom
	contentStr := b.String()
	contentLines := strings.Count(contentStr, "\n")
	inputH := strings.Count(inputRendered, "\n") + 1
	statusH := 1
	bottomH := inputH + suggestLines + statusH + 1 // +1 for input\n separator
	padLines := m.height - contentLines - bottomH
	if padLines > 0 {
		b.WriteString(strings.Repeat("\n", padLines))
	}

	b.WriteString(inputRendered)
	b.WriteString("\n")
	b.WriteString(statusBarRendered)

	return b.String()
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
		labelStyle.Render("Session:   ") + textStyle.Render(m.sessionID),
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
			if turnIdx < len(m.completedTurns) {
				td = &m.completedTurns[turnIdx]
			}
			turnIdx++
			if td != nil {
				// Render thinking block
				if td.thinking != "" {
					b.WriteString(m.renderThinkingBlock(td.thinking, false, turnIdx-1))
					b.WriteString("\n")
				}
				// Render tool groups
				if len(td.toolGroups) > 0 {
					b.WriteString(m.renderToolGroupsBlock(td.toolGroups, turnIdx-1))
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
	maxNameW := 0
	for _, s := range m.suggestions {
		if len(s.name) > maxNameW {
			maxNameW = len(s.name)
		}
	}

	for i, s := range m.suggestions {
		name := s.name
		pad := strings.Repeat(" ", maxNameW-len(name)+2)
		if i == m.selectedSuggest {
			b.WriteString(fmt.Sprintf("  → %s%s%s\n",
				strongStyle.Render(name), pad, primaryStyle.Render(s.desc)))
		} else {
			b.WriteString(fmt.Sprintf("    %s%s%s\n",
				textStyle.Render(name), pad, dimStyle.Render(s.desc)))
		}
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

	// Render input with cursor
	var inputWithCursor string
	runes := []rune(m.input)
	cursorPos := m.cursor
	if cursorPos > len(runes) {
		cursorPos = len(runes)
	}
	if cursorPos < 0 {
		cursorPos = 0
	}

	before := string(runes[:cursorPos])
	after := ""
	if cursorPos < len(runes) {
		after = string(runes[cursorPos:])
	}

	var cursorChar string
	if m.cursorBlink || m.streaming {
		cursorChar = dimStyle.Render("▏")
	} else {
		cursorChar = primaryStyle.Render("█")
	}
	inputWithCursor = textStyle.Render(before) + cursorChar + textStyle.Render(after)

	style := inputFocusedStyle
	if m.showSuggestions {
		style = inputBorderStyle
	}

	return style.Width(boxW).Render(prompt + inputWithCursor)
}

// ── Streaming render functions ──

// renderStreaming renders the active streaming content: thinking, tools, and response text.
func (m tuiModel) renderStreaming() string {
	var b strings.Builder
	streamTurnIdx := len(m.completedTurns)

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

	// Tool groups
	if len(m.streamToolGroups) > 0 {
		b.WriteString(m.renderToolGroupsBlock(m.streamToolGroups, streamTurnIdx))
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

	if streaming {
		expanded := m.isSectionExpanded(turnIndex, "thinking")
		if expanded {
			header := focusPrefix + dimStyle.Render("▾ Thinking…")
			display := thinking
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
		body := dimStyle.Render(indentText(thinking, "    "))
		return header + "\n" + body
	}

	// Collapsed
	return focusPrefix + dimStyle.Render("▸ Thought  ") + mutedStyle.Render("(Tab to expand)")
}

// renderToolGroupsBlock renders collapsible tool invocation blocks.
func (m tuiModel) renderToolGroupsBlock(groups []toolGroup, turnIndex int) string {
	if len(groups) == 0 {
		return ""
	}

	focused := m.isSectionFocused(turnIndex, "tools")
	expanded := m.isSectionExpanded(turnIndex, "tools")
	focusPrefix := "  "
	if focused {
		focusPrefix = primaryStyle.Render("❯ ")
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

		if !expanded {
			// Collapsed: single line
			prefix := focusPrefix
			if i > startIdx {
				prefix = "  " // only first visible line gets focus prefix
			}
			line := fmt.Sprintf("%s▸ %s %s", prefix, nameStyled, dimStyle.Render(summary))
			if tg.result == "" && !tg.isError {
				line += dimStyle.Render(" ⋯") // running
			}
			b.WriteString(line)
		} else {
			// Expanded: header + args + result
			prefix := focusPrefix
			if i > startIdx {
				prefix = "  "
			}
			b.WriteString(fmt.Sprintf("%s▾ %s %s\n", prefix, nameStyled, dimStyle.Render(summary)))
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

// indentText indents each line of text by the given prefix.
func indentText(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderStatusBar() string {
	w := m.width
	if w < 20 {
		w = 20
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
	if m.goalTracker.IsActive() {
		left = append(left, successStyle.Render("goal"))
	}
	leftStr := strings.Join(left, " ")

	// Right side: context + turns + token breakdown
	right := m.renderTokenStatus()

	// Simple left-right split
	leftW := lipgloss.Width(leftStr)
	rightW := lipgloss.Width(right)
	gap := w - leftW - rightW - 2
	if gap < 2 {
		gap = 2
	}

	return statusBarStyle.Width(w).Render(leftStr + strings.Repeat(" ", gap) + right)
}

// renderTokenStatus returns the right side of the status bar with token breakdown.
func (m tuiModel) renderTokenStatus() string {
	ctxStr := m.contextMgr.UsageDisplay()
	dot := mutedStyle.Render(" · ")

	// During streaming, show live current-turn usage
	if m.streaming && m.turnUsage.GrandTotal() > 0 {
		u := m.turnUsage
		parts := []string{"ctx: " + ctxStr}
		parts = append(parts, fmt.Sprintf("in: %s", agentctx.FormatTokenCount(u.InputTotal())))
		parts = append(parts, fmt.Sprintf("out: %s", agentctx.FormatTokenCount(u.Output)))
		if u.InputCacheRead > 0 {
			parts = append(parts, fmt.Sprintf("cache: %s", agentctx.FormatTokenCount(u.InputCacheRead)))
		}
		return dimStyle.Render(strings.Join(parts, dot))
	}

	// Idle: show cumulative session usage
	if m.sessionUsage.GrandTotal() > 0 {
		u := m.sessionUsage
		parts := []string{"ctx: " + ctxStr}
		parts = append(parts, fmt.Sprintf("turns: %d", m.turnCount))
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
func buildSystemPrompt(cwd, branch string, skillCat *skill.Catalog) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	prompt := fmt.Sprintf(`You are a helpful AI coding assistant with access to tools for file operations, code search, and shell commands.

## Environment
- Working directory: %s
- OS: %s/%s
- Git branch: %s

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
- If a task requires multiple steps, plan them out first`, cwd, osName, arch, branch)

	// Append available skills section
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
			prompt += "\n\n## Available Skills\nThe following skills can be invoked by the user via slash commands:\n" + strings.Join(skillLines, "\n")
		}
	}

	return prompt
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

func (m tuiModel) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredPickerModels()

	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit

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

	case tea.KeyRunes:
		for _, r := range msg.Runes {
			m.pickerSearch += string(r)
		}
		m.pickerSelected = 0
		return m, nil

	case tea.KeySpace:
		m.pickerSearch += " "
		m.pickerSelected = 0
		return m, nil
	}

	return m, nil
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

	// Use tea.ExecProcess to suspend TUI and run editor
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
	fmt.Printf("  Model:     %s %s\n", a.Config.DefaultProvider, providerStatus)
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

	permChain := permission.DefaultChain()

	branch := getGitBranch(skill.FindProjectRoot(cwd))
	// Discover skills for simple mode
	var skillCat *skill.Catalog
	if cat, err := skill.Discover(cwd); err == nil {
		skillCat = cat
	}
	systemPrompt := buildSystemPrompt(cwd, branch, skillCat)

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

						result, err := tool.Execute(ctx, toolInput, tools.ExecContext{WorkDir: cwd})
						if err != nil {
							result = &tools.Result{Output: err.Error(), IsError: true}
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
