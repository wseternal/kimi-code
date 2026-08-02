# TS Migration Completion

## Goal
All core CLI/agent functionalities from the old TypeScript kimi CLI are implemented, wired, and operational in the new Go kimi CLI — the Go binary can serve as a full replacement for interactive TUI, headless, and server modes.

## Context
The Go rewrite (~53K lines) already covers: CLI parsing, TUI (Bubbletea v2), headless mode, LLM provider abstraction (5 backends), 20+ agent tools, session management, OAuth, REST server (30+ routes), persistence, audit trail, transcript, and client SDK. All tests pass and the build is clean.

Key gaps identified by comparing the TS codebase (`/Users/jiangzhaohua/codes/kimi-code`) to the Go codebase (`/Users/jiangzhaohua/codes/wseternal/kimi-code`):

1. **MCP integration** — directory exists with 1 stub file, not wired to agent tools
2. **WebSocket transport** — protocol types defined, not connected to server or event bus
3. **Server stub endpoints** — compact, undo, messages listing, model-catalog, OAuth login, connections return 501 or empty
4. **SSH remote execution** — interface defined, all methods return "not yet connected"
5. **Plugin system** — not implemented (TS has plugin manager, manifest, store, marketplace)
6. **Agent profiles** — directory exists but only stub
7. **Telemetry** — minimal stub, no cloud/console appenders
8. **Self-update** — not implemented (TS has CDN-based update)

## Success Criteria
- MCP servers can be configured, started, and their tools are available to the agent
- WebSocket clients can connect to the server and receive real-time session events
- Server endpoints for compact, undo, messages, model-catalog, and OAuth return real data
- Agent profiles can be loaded and applied to sessions
- `task go:build` succeeds, `task go:test` passes, `task go:lint` clean
- The Go CLI can run a full interactive TUI session with tool execution

## Constraints
- Go 1.24+, no new heavy dependencies without justification
- Follow existing DI patterns (App/Session/Agent scopes)
- Follow existing coding conventions (gofmt, go vet, constructor injection)
- Must not break existing passing tests

## Out of Scope
- VS Code extension (separate TypeScript app)
- Web UI frontend (kimi-web — separate app)
- Visualization tool (kimi-vis — separate app)
- Inspection tool (kimi-inspect — separate app)
- ACP adapter (niche protocol bridge)
- Legacy `~/.kimi` migration tool (one-time migration utility)
- Native SEA binary builds
- Plugin marketplace CDN infrastructure (but local plugin loading is in scope)

## Created
2026-08-02
