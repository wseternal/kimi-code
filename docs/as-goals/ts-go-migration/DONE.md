# Goal Achieved — TS-to-Go Migration Parity

## Iterations: 1/10

## Gates Passed
- [x] MCP HTTP/SSE Transport — New HTTP/SSE client implementation
- [x] WebSearch Tool — Registered in default tools
- [x] Shell Mode & Autocomplete — Already implemented (no changes needed)
- [x] Goal Tools Complete — GetGoal, SetGoalBudget (pre-existing), queue operations added
- [x] Upgrade System — `kimi upgrade`/`kimi update` command
- [x] Build & Test Clean — All builds and tests pass

## Commits
- `1af029d`: feat: implement MCP HTTP/SSE transport, WebSearch registration, goal queue, and upgrade system
- `2ab3c0c`: fix: return QueuedGoal by value to avoid unsafe pointer to internal slice

## Working Tree
- Status: clean (artifacts committed)
- Branch: main

## Summary of Changes

### MCP HTTP/SSE Transport
- New file: `internal/agentcore/agent/mcp/http_client.go` (458 lines)
- Implements full Client interface for both Streamable HTTP and legacy SSE transports
- SSE endpoint auto-discovery, JSON-RPC 2.0 over HTTP POST, SSE event parsing
- Bearer token auth via environment variable
- Updated `manager.go` to create HTTPClient for http/sse transport types

### WebSearch Tool
- Registered `NewWebSearchTool(nil)` in `RegisterDefaultTools`
- Nil-safe: returns "not configured" error when invoked without a provider
- Matches TS CLI behavior when no search service is set up

### Goal Queue & Slash Commands
- Added `QueuedGoal` type and 5 queue operations to `goal.Tracker`
- Extended `/goal` slash command with: status, pause, resume, cancel, next, next manage
- Updated help text in commands.go

### Upgrade System
- New file: `internal/cli/upgrade.go` (138 lines)
- `kimi upgrade`/`kimi update` checks GitHub releases API
- Semver comparison, platform-specific download URL detection
- Added to root.go subcommand dispatch and help text

### Already Implemented (No Changes)
- Shell mode (`!` prefix) — tui.go line 3048
- File autocomplete (`@` trigger) — fileCandidates system
- Ctrl+S steering — tui.go line 2046
- GetGoal tool — goal_tools.go line 76
- SetGoalBudget tool — goal_tools.go line 170
- WebSearch tool code — web.go line 246
