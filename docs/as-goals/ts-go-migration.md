# TS-to-Go Migration Parity

## Goal
All functionalities of the old TypeScript kimi CLI are migrated, redesigned, or contained by the new Go kimi CLI — the Go implementation has feature parity or a deliberate redesign decision for every TS feature.

## Context
The old TypeScript kimi CLI at `/Users/jiangzhaohua/codes/kimi-code` is a full-featured terminal AI coding assistant with 36+ feature areas. The new Go implementation at `/Users/jiangzhaohua/codes/wseternal/kimi-code` already implements ~95% of features. Remaining gaps include:

1. **MCP HTTP/SSE transport** — stdio works, HTTP/SSE is stub ("not yet implemented")
2. **SSHKaos** — struct defined but Connect() has no real SSH library
3. **WebSearch tool** — present in TS, missing from Go built-in tools
4. **ACP (Agent Client Protocol) command** — `kimi acp` subcommand missing (protocol types exist)
5. **Plugin marketplace** — `/plugins install/marketplace/info/enable/disable/remove/reload` CLI commands and browsing
6. **Upgrade/update system** — `kimi upgrade` / `kimi update` auto-update flow
7. **GetGoal / SetGoalBudget tools** — only CreateGoal and UpdateGoal exist in Go
8. **Shell mode (!)** — inline terminal execution from empty input
9. **File autocomplete (@)** — path completion triggered by @ in input
10. **Clipboard/media paste** — Ctrl-V image/video paste into input
11. **Steer during streaming** — Ctrl-S injection without waiting for turn end
12. **Goal queue** — `/goal next`, `/goal next manage` upcoming goal queue
13. **Node SDK / Klient** — programmatic integration API completeness
14. **Secondary model** — experimental subagent model selection

## Success Criteria
- Go CLI builds and passes all existing tests
- All gap items either have a working implementation or a documented redesign decision
- MCP HTTP/SSE transport connects to a remote MCP server
- WebSearch tool is available as a built-in tool
- Shell mode (!) works from TUI input
- File autocomplete (@) triggers path completion in TUI input
- GetGoal tool returns current goal status
- Goal queue supports next/manage operations
- Upgrade command checks for and applies updates

## Constraints
- Must not break existing passing tests
- Must follow Go conventions (gofmt, go vet, golangci-lint)
- Must use constructor injection (no global state)
- Keep changes focused per iteration

## Out of Scope
- VS Code extension migration (separate effort)
- kimi-web web UI app migration (separate effort)
- vis session visualizer app (separate effort)
- Agent engine v2 experimental features (Go IS the v2)
- Kimi Datasource plugin (third-party data)

## Created
2026-08-02
