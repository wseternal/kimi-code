# Spec: Steering Input Separation

## Objective

Decouple the TUI input subsystem from the streaming content renderer so the
user always has a fully functional input editor — even while the agent is
streaming. Submitted messages go to a queue and are delivered to the agent as a
core steering tool, which injects queued messages into the conversation context
at natural breakpoints (between LLM steps).

## Tech Stack

- **Language:** Go >= 1.24
- **TUI framework:** Bubble Tea v2 (charmbracelet/bubbletea)
- **Build:** Taskfile (`task go:build`, `task go:test`, `task go:lint`)

## Commands

```
Build:  task go:build
Test:   task go:test
Lint:   task go:lint
Fmt:    task go:fmt
Tidy:   task go:tidy
```

## Project Structure

```
internal/cli/
  tui.go                        → TUI model, key handling, streaming, rendering
  tui_test.go                   → TUI unit tests
internal/agentcore/agent/tools/
  steering.go                   → NEW: SteeringTool implementation (system tool, NOT registered in tool registry)
  builtin.go                    → Unchanged (steering is invoked directly by streaming loop)
  registry.go                   → Tool interface and Registry (unchanged)
internal/agentcore/agent/loop/
  service.go                    → Agent loop (unchanged — steering is TUI-level)
```

## Code Style

```go
// SteeringTool is a system-injected tool that delivers queued user messages
// to the agent at step boundaries. The LLM does not call this tool directly;
// the streaming loop invokes it between steps and injects the result as a
// tool message in the conversation.
type SteeringTool struct {
    mu       sync.Mutex
    queue    []SteeringMessage
    signaled atomic.Bool // true when user presses steering key
}

type SteeringMessage struct {
    Content string `json:"content"`
}
```

- Follow existing tool patterns: `Definition() Definition` + `Execute(ctx, input, exec) (*Result, error)`
- Thread-safe queue access via `sync.Mutex`
- Atomic bool for the steering signal (hot path: single writer, many readers)

## Testing Strategy

- **Framework:** `testing` package with `-race` flag
- **Locations:** Add tests to existing `tui_test.go`; new `steering_test.go` for SteeringTool
- **Coverage:**
  - SteeringTool: queue/drain/signal operations, thread safety
  - TUI key handling: verify all editing keys work when `m.streaming == true`
- **No new test files** unless the existing ones don't cover the package

## Boundaries

- **Always do:**
  - Keep `handleKey()` as the single key-handling path regardless of streaming state
  - Make all queue operations thread-safe
  - Preserve ESC as the hard cancel
  - Run `task go:test` and `task go:lint` before committing

- **Ask first:**
  - Changing the streaming goroutine's step loop structure
  - Adding new Bubble Tea message types that affect the update loop
  - Modifying the `streamEvent` channel protocol

- **Never do:**
  - Remove or degrade any existing input editing key
  - Force-interrupt the agent (ESC is the only hard cancel)
  - Change the agent loop service (`loop/service.go`)

## Design

### 1. Remove the `if m.streaming` Key Hijack

**Current (lines 1577–1629 in tui.go):**
```go
if m.streaming {
    switch {
    case msg.Code == 'c' && msg.Mod&tea.ModCtrl != 0: // quit
    case msg.Code == 't' && msg.Mod&tea.ModCtrl != 0: // drawer toggle
    case msg.Code == tea.KeyEscape:                    // cancel stream
    case msg.Code == tea.KeyTab:                       // collapse toggle
    case msg.Code == tea.KeyEnter:                     // collapse toggle
    case msg.Code == tea.KeyUp:                        // focus nav
    case msg.Code == tea.KeyDown:                      // focus nav
    default:                                           // raw text queue only
    }
}
```

**After:** Remove the entire `if m.streaming` block. All keypresses flow
through `handleKey()`. Streaming-specific keys (ESC = cancel stream, drawer
toggle) are handled as conditional branches inside `handleKey()`.

### 2. Message Queue in handleSubmit

**Current:** `handleSubmit()` clears input and calls `runLLMStream()` directly.

**After:**
```go
func (m tuiModel) handleSubmit() (tea.Model, tea.Cmd) {
    input := strings.TrimSpace(m.input)
    if input == "" { return m, nil }

    // Slash commands and !bash still execute immediately
    // ...existing slash/bash handling...

    if m.streaming {
        // Queue the message — don't start a new stream
        m.steeringTool.Enqueue(input)
        m.messages = append(m.messages, chatMessage{"user", input})
        m.messages = append(m.messages, chatMessage{"system",
            fmt.Sprintf("📨 Queued (press %s to steer agent)", steeringKeyHint)})
        m.input = ""
        m.cursor = 0
        return m, nil
    }

    // Normal submit: start streaming
    // ...existing submit logic...
}
```

### 3. Steering Key (Ctrl+S)

Added to `handleKey()`:
```go
case msg.Code == 's' && ctrl:
    if m.streaming && m.steeringTool != nil && m.steeringTool.Len() > 0 {
        m.steeringTool.Signal()
        m.messages = append(m.messages, chatMessage{"system",
            "⚡ Steering signal sent — agent will pick up at next breakpoint"})
    }
    return m, nil
```

### 4. SteeringTool (`internal/agentcore/agent/tools/steering.go`)

```go
type SteeringTool struct {
    mu       sync.Mutex
    queue    []SteeringMessage
    signaled atomic.Bool
}

// Enqueue adds a message to the steering queue.
func (t *SteeringTool) Enqueue(content string) { ... }

// HasMessages reports whether there are queued messages.
func (t *SteeringTool) HasMessages() bool { ... }

// DrainAll returns and clears all queued messages.
func (t *SteeringTool) DrainAll() []SteeringMessage { ... }

// Signal sets the steering priority flag.
func (t *SteeringTool) Signal() { t.signaled.Store(true) }

// ConsumeSignal checks and clears the priority signal.
func (t *SteeringTool) ConsumeSignal() bool {
    return t.signaled.Swap(false)
}

// Definition returns the tool definition for the LLM.
func (t *SteeringTool) Definition() Definition {
    return Definition{
        Name:        "Steering",
        Description: "System tool: delivers user steering messages. " +
            "These are mid-conversation instructions from the user that " +
            "should be considered before proceeding with further actions.",
        Parameters: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{},
        },
    }
}

// Execute returns queued messages as formatted context.
func (t *SteeringTool) Execute(ctx context.Context, input json.RawMessage,
    exec ExecContext) (*Result, error) {
    msgs := t.DrainAll()
    if len(msgs) == 0 {
        return &Result{Output: "No steering messages."}, nil
    }
    // Format messages for the LLM
    var sb strings.Builder
    sb.WriteString("The user has sent the following steering messages:\n")
    for _, m := range msgs {
        sb.WriteString(fmt.Sprintf("\n- %s", m.Content))
    }
    sb.WriteString("\n\nPlease consider these instructions before proceeding.")
    return &Result{Output: sb.String()}, nil
}
```

### 5. Injection Point in runLLMStream

In the TUI's streaming goroutine, after each LLM step completes and before
the next step begins, check for queued messages:

```go
// After processing tool calls for this step:
if steering.ConsumeSignal() || steering.HasMessages() {
    msgs := steering.DrainAll()
    if len(msgs) > 0 {
        output := steering.FormatMessages(msgs)
        // Inject as user message to satisfy LLM provider API contracts
        messages = append(messages, kosong.CreateUserMessage("[Steering] "+output))
    }
}
```

This is the only change to the streaming loop — a 5-line injection at the
step boundary.

### 6. Auto-Pickup on Stream Completion

When the stream ends (`streamEvent{kind: "done"}`), check for queued
messages and auto-start a new turn:

```go
case "done":
    m.streaming = false
    // ...existing done handling...

    // Auto-pickup queued steering messages
    if m.steeringTool != nil {
        msgs := m.steeringTool.DrainAll()
        if len(msgs) > 0 {
            parts := make([]string, len(msgs))
            for i, sm := range msgs {
                parts[i] = sm.Content
            }
            nextPrompt := strings.Join(parts, "\n")
        m.streaming = true
        m.cancelCh = make(chan struct{})
        // ...reset streaming state...
        return m, m.runLLMStream(nextPrompt)
    }
```

### 7. UI Indicators

- **Queue badge:** Show `"[N queued]"` next to the input prompt when messages are queued
- **Steering hint:** Show `Ctrl+S to steer` when queued + streaming
- **Visual separator:** Queued messages shown in the chat with a distinct style (e.g., dimmed or with a `📨` prefix)

## Success Criteria

1. During streaming, pressing any editing key (arrows, Ctrl+A/E/K/W,
   backspace, delete, word nav, history up/down) works identically to
   when idle. No degraded key map.
2. Pressing Enter during streaming queues the message and shows a visual
   indicator. The current stream is not interrupted.
3. Pressing Ctrl+S during streaming signals the agent to prioritize the
   queue at its next step boundary.
4. Queued messages are injected into the LLM conversation as a steering
   tool result between steps.
5. When the stream finishes, any remaining queued messages are
   automatically sent as the next prompt.
6. ESC still cancels the current stream immediately (unchanged).
7. All existing tests pass with `-race`. New tests cover SteeringTool
   queue operations and thread safety.

## Open Questions

_None — all design decisions confirmed with user._
