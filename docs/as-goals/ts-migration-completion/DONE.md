# Goal Achieved — TS Migration Completion

## Iterations: 2/10

## Gates Passed
- [x] Build-Test-Lint Clean
- [x] MCP Integration
- [x] WebSocket Transport
- [x] Server Routes Wired
- [x] Agent Profiles

## Commits
- `a41c152`: feat: implement MCP client, agent profiles, WebSocket route, and server wiring
- `d6e2931`: feat(server): wire compact, undo, messages, OAuth login, and transcript route callbacks

## What Was Implemented

### MCP Integration
- Full MCP client (`stdio.go`) — JSON-RPC 2.0 over stdin/stdout
- Tool adapter (`adapter.go`) — wraps MCP tools as `tools.Tool`
- Connection manager (`manager.go`) — lifecycle management for multiple MCP servers
- Config parsing (`config.go`) — `McpServerConfig` with transport, command, args, env

### WebSocket Transport
- Route registration (`GET /api/v1/ws`) in server
- Handshake, subscribe/unsubscribe, heartbeat, journal replay already in `transport.go`

### Server Routes Wired
- Compact session → `CompactFunc` callback
- Undo session → `UndoFunc` callback with N-step undo
- List messages → `MessageListFunc` callback
- OAuth login → `OAuthLoginFunc` callback
- List transcript → `TranscriptListFunc` callback
- Model catalog → `ConfigProvider` adapter
- Config endpoint → `ConfigProvider` adapter

### Agent Profiles
- Dual-format loader: TOML and Markdown (with YAML frontmatter)
- `LoadNamed()` searches `~/.kimi-code/agents/`
- `ApplyToSystemPrompt()` merges profile instructions
- CLI flags: `--agent` and `--agent-file`

## Working Tree
- Status: clean
- Branch: main
