# Review — Iteration 1

## Summary
Iteration 1 implemented the foundational pieces for all 5 gates: MCP client (stdio transport), agent profile loader, WebSocket route registration, and server route wiring improvements.

## Code Quality Findings

### Critical: 0
### Warning: 2
- Server compact/undo routes still return 501 (not wired to agent loop)
- Server messages listing still returns empty (transcript not integrated)

### Suggestion: 3
- MCP HTTP/SSE transports not yet implemented (only stdio)
- OAuth login route still returns 501
- Agent profile tool allow/deny lists are parsed but not enforced during tool registration

## Architecture Review
- MCP package follows existing patterns (interface-based, clean separation of transport/adapter/manager)
- Agent profile follows the TOML/Markdown dual-format approach matching TS
- WebSocket transport leverages the already-implemented `transport.HandleWebSocket` 
- Config provider pattern cleanly decouples server from config implementation

## Correctness Review
- MCP JSON-RPC implementation correctly handles request/response ID matching
- Tool name qualification correctly sanitizes names
- Profile frontmatter parsing handles edge cases
- WebSocket route properly delegates to existing transport code

## Security Review
- MCP stdio client inherits parent process env (expected)
- Agent profiles loaded from user-controlled paths (acceptable for CLI tool)
- WebSocket uses existing security middleware (bearer auth, CORS)

## Commits Reviewed
- `a41c152`: feat: implement MCP client, agent profiles, WebSocket route, and server wiring
