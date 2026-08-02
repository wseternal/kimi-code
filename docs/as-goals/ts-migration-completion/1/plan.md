# Plan — Iteration 1

## Focus
Implement the foundational pieces for all 5 gates, prioritizing quick wins first.

## Tasks (ordered)

### Task 1: Wire WebSocket upgrade route into server
- **Files:** `internal/kapserver/server.go`
- **What:** Register `HandleWebSocket` on `/api/v1/ws` path in `setupRoutes()`
- **Acceptance:** WebSocket clients can connect and complete handshake

### Task 2: MCP config parsing
- **Files:** `internal/agentcore/config/config.go`
- **What:** Add `McpServers` field to Config struct, parse from TOML
- **Acceptance:** Config loads MCP server definitions from TOML

### Task 3: MCP stdio client implementation
- **Files:** `internal/agentcore/agent/mcp/` (new files)
- **What:** Implement MCPClient interface, stdio transport (spawn process, JSON-RPC), tool discovery
- **Acceptance:** Can start an MCP server process, list tools, call tools

### Task 4: MCP tool adapter
- **Files:** `internal/agentcore/agent/mcp/`, `internal/agentcore/agent/tools/`
- **What:** Adapter wrapping MCP tools as `tools.Tool`, registers into registry
- **Acceptance:** MCP tools appear in agent tool list

### Task 5: Agent profile loader
- **Files:** `internal/agentcore/agent/profile/` (new files)
- **What:** Define AgentProfile struct, load from TOML/Markdown, apply to session
- **Acceptance:** Profiles load and affect system prompt

### Task 6: Wire --agent and --agent-file CLI flags
- **Files:** `internal/cli/root.go`
- **What:** Add flags, pass profile to session creation
- **Acceptance:** CLI flags work

### Task 7: Wire server route stubs
- **Files:** `internal/kapserver/routes/`
- **What:** Wire compact, undo, messages, model-catalog, OAuth login to real implementations
- **Acceptance:** Endpoints return real data

### Task 8: Tests
- Add tests for MCP client, agent profile loader, server route wiring
