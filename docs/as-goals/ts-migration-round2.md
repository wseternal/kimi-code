# TS Migration Round 2

## Goal
All non-MCP functionality from the old TypeScript kimi CLI is available in the new Go kimi CLI — hook system, AGENTS.md prompt injection, secondary model config, compaction, model discovery, session fork/undo, system reminders, SelectTools groups, goal queue, and slash command parity.

## Context
Round 1 (iteration `d300b5d` + `147e552`) completed tool wiring (PlanMode, SelectTools, SkillTool), Goal/Cron/Agent tools, steering TUI, and transcript system. This round targets the remaining feature gaps identified by deep re-investigation of both codebases.

## Success Criteria
- Hook system supports PreToolUse, PostToolUse, and Session lifecycle events with shell command hooks from config.toml
- AGENTS.md files in project roots are read and injected into the system prompt
- Secondary model config allows specifying a fallback/cheaper model for sub-agents
- Full compaction summarizes conversation history when context window fills up
- Dynamic model discovery lists available models from providers
- Session fork creates a branch of the current session; undo withdraws the last prompt
- System reminders are injected into the conversation at appropriate points
- SelectTools registers meaningful tool groups for progressive disclosure
- Goal queue supports multiple queued goals (not just one active)
- Stub slash commands have real implementations
- Bash tool supports `run_in_background` parameter
- `task go:build` succeeds and `task go:test` passes

## Constraints
- Must not break existing functionality
- Must follow Go conventions and existing code patterns
- MCP integration is explicitly out of scope
- Plugin system is deferred (too large for remaining budget)

## Out of Scope
- MCP client integration (stdio, HTTP, SSE transports)
- Plugin system (GitHub plugins, marketplace)
- VS Code extension, Web UI, Inspector, Visualizer
- WebSearch provider (needs external API key)

## Created
2026-08-02
