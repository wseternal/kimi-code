# Gate: MCP HTTP/SSE Transport

## Condition
MCP servers configured with `http` or `sse` type can connect and their tools are available in the agent's tool registry, similar to stdio.

## Evidence Required
- [ ] HTTP/SSE transport implementation in `internal/agentcore/agent/mcp/manager.go` (or new file)
- [ ] Connection lifecycle (connect, list tools, call tool) working for HTTP endpoints
- [ ] Existing stdio transport still works (no regression)

## Verification Method
- Code review: HTTP transport follows MCP protocol spec
- Build passes with `task go:build`
- Existing tests pass with `task go:test`

## Owner
Senior Software Engineer
