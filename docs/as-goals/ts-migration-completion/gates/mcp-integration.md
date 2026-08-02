# Gate: MCP Integration

## Condition
MCP servers can be configured in config.toml, started via stdio/http/sse transport, and their tools become available to the agent during a session. The agent can invoke MCP tools and receive results.

## Evidence Required
- [ ] MCP client implementation with stdio transport → `internal/agentcore/agent/mcp/`
- [ ] MCP config parsing (servers section in config.toml) → `internal/agentcore/config/`
- [ ] MCP tools registered in agent tool registry → `internal/agentcore/agent/tools/`
- [ ] Unit tests for MCP client lifecycle and tool invocation
- [ ] Build/test/lint passes

## Verification Method
1. Verify MCP client can start a stdio server process and exchange JSON-RPC messages
2. Verify tools from MCP servers appear in the tool registry
3. Verify tool call dispatching works (invoke MCP tool, get result back)
4. Run existing test suite — no regressions

## Owner
Engineer
