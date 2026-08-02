# Plan — Iteration 1

## Gap Analysis (Already Done vs Remaining)

**Already implemented:**
- Shell mode (`!` prefix) — tui.go line 3048
- File autocomplete (`@` trigger) — tui.go fileCandidates system
- Ctrl+S steering — tui.go line 2046
- GetGoal tool — goal_tools.go line 76
- SetGoalBudget tool — goal_tools.go line 170
- WebSearch tool code — web.go line 246 (but NOT registered)

**Remaining gaps to close:**
1. MCP HTTP/SSE transport — stub, needs real implementation
2. WebSearchTool registration — not in RegisterDefaultTools, needs a default provider
3. Goal queue (`/goal next`, `/goal next manage`) — not in goal tracker or commands
4. Upgrade system (`kimi upgrade`) — not implemented

## Task Breakdown

### Task 1: MCP HTTP/SSE Transport (Gate: MCP HTTP/SSE)
**Files:** `internal/agentcore/agent/mcp/http_client.go` (new)
- Implement `HTTPClient` that satisfies the `Client` interface
- Use JSON-RPC 2.0 over HTTP POST for the `http` transport
- Use SSE (Server-Sent Events) stream for the `sse` transport
- Implement Initialize, ListTools, CallTool, Ping, Close
- Update `createClient` in manager.go to create HTTPClient for "http"/"sse"

### Task 2: WebSearchTool Registration (Gate: WebSearch Tool)
**Files:** `internal/agentcore/agent/tools/web.go`, `internal/agentcore/agent/tools/builtin.go`
- Add `GenericSearchProvider` that uses a configurable search API endpoint
- Register WebSearchTool in `RegisterDefaultTools` with a nil-safe fallback

### Task 3: Goal Queue (Gate: Goal Tools Complete)
**Files:** `internal/agentcore/agent/goal/goal.go`, `internal/cli/commands.go`
- Add queue data structure to Tracker (pending goals)
- Add `QueueGoal`, `NextGoal`, `ListQueue` methods
- Add `/goal next <objective>` and `/goal next manage` slash commands

### Task 4: Upgrade System (Gate: Upgrade System)
**Files:** `internal/cli/upgrade.go` (new), `internal/cli/root.go`
- Implement `kimi upgrade` command
- Check GitHub releases API for latest version
- Compare semantic versions
- Download and replace binary (or instruct user)

## Priority Order
1. Task 1 (MCP HTTP/SSE) — largest gap, most code
2. Task 2 (WebSearch registration) — small change
3. Task 3 (Goal queue) — moderate
4. Task 4 (Upgrade system) — moderate
