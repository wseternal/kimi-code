# Intent: Steering Input Separation

Decouple the TUI input subsystem from the streaming content renderer so the
user always has a fully functional input box, and submitted messages are
delivered to the agent as a core steering tool.

## Outcome

Fully independent input and rendering subsystems. The input editor works
identically whether the agent is idle or streaming. Submitted messages are
queued and delivered to the agent as a core tool (like other tools), bringing
steering context into the conversation naturally.

## User

The human operator using the TUI, who should never feel "locked out" of the
input box by the agent's working state.

## Why Now

The current `if m.streaming` block in `internal/cli/tui.go` (around line 1577)
hijacks the entire keyboard during streaming. It strips out cursor movement,
deletion, history navigation, and all editing keys, replacing them with a
degraded key map that only allows collapse navigation and raw character
queuing. This is an unnecessary coupling between the output renderer and the
input editor — a design flaw.

## Success Criteria

- All input editing keys (arrows, Ctrl+A/E/K/W, backspace, delete, word
  navigation, history up/down, etc.) work normally during streaming.
- Submitted prompts go to a queue and are visible in the UI.
- Queued messages are delivered to the agent as a **steering core tool**,
  injecting context the agent sees and acts on through its normal
  decision-making.
- A steering button/action signals the agent to prioritize queued messages
  at natural breakpoints (e.g., before tool invocations).
- ESC remains unchanged as the explicit hard cancel signal.

## Constraint

Steering is cooperative, not forced. The agent loop owns when to respect
steering messages — it checks at natural breakpoints. No forced interrupt.
ESC is the only way to abort the agent's current work immediately.

## Out of Scope

- Modifying the streaming output renderer.
- Redesigning the collapsible/drawer system.
- Restructuring the agent loop's turn/step state machine.
  (The loop already knows how to process tools; steering is just another one.)

## Design Notes

### TUI Changes (`internal/cli/tui.go`)

1. **Remove the `if m.streaming` key hijack.** All keypresses flow through
   `handleKey()` regardless of streaming state.
2. **Message queue.** Submitted prompts (Enter) go to a queue when the agent
   is busy. Visible indicator in the UI (e.g., "1 queued message").
3. **Steering button.** An explicit action (key or UI element) that signals
   the agent "I have a queued message, please process it at your next
   breakpoint."
4. **Auto-pickup.** When the agent finishes its current turn, it
   automatically picks up queued messages without requiring the user to
   press the steering button.

### Steering Core Tool (`internal/agentcore/agent/tools/`)

- Implemented as a core tool alongside existing tools.
- Delivers queued user messages as context to the agent.
- The agent sees steering input the same way it sees tool results — as
  additional information to inform its next action.
- No special-case hooks in the agent loop required.

### Agent Loop (`internal/agentcore/agent/loop/`)

- No structural changes.
- The steering tool integrates through the existing tool mechanism.
- The agent naturally sees steering messages when it next evaluates context.
