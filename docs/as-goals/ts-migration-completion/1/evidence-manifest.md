# Evidence Manifest — Iteration 1

## Gate Status

| Gate | Status | Evidence | Owner |
|------|--------|----------|-------|
| Build-Test-Lint Clean | ✅ Pass | `task go:build` exits 0, `task go:test` all 20+ packages pass | Engineer |
| MCP Integration | ✅ Pass | `internal/agentcore/agent/mcp/client.go`, `stdio.go`, `adapter.go`, `manager.go`, `mcp_test.go`; config.go McpServerConfig; tools registered via adapter | Engineer |
| WebSocket Transport | ✅ Pass | `internal/kapserver/server.go` registers `GET /api/v1/ws` via `transport.HandleWebSocket`; `internal/kapserver/transport/transport.go` has full handshake, subscribe, heartbeat, journal | Engineer |
| Server Routes Wired | ❌ Fail | Model-catalog and config now return real data; connections wired. BUT compact, undo, messages, OAuth login still stubbed | Engineer |
| Agent Profiles | ✅ Pass | `internal/agentcore/agent/profile/profile.go` with TOML+Markdown loading; `--agent`/`--agent-file` flags in root.go; profile_test.go passes | Engineer |

## Return Shipments (Failed Gates)

### Gate: Server Routes Wired
**Defect:** compact, undo, messages listing, and OAuth login endpoints still return 501/empty
**Root Cause:** These require wiring to the agent loop's context manager (compact), transcript store (messages/undo), and OAuth manager (login) — subsystems that exist but aren't connected to the server
**Routed To:** Engineer
**Priority:** Warning

## Code Quality Findings
- Critical: 0
- Warning: 2
- Suggestion: 3

## Commits Reviewed
- `a41c152`: feat: implement MCP client, agent profiles, WebSocket route, and server wiring
