# TS Migration Completion

## Goal
All CLI-facing functionality from the old TypeScript kimi CLI is available in the new Go kimi CLI — tools are wired and callable by the LLM, goal/cron/agent tools are implemented, and the transcript system provides event-sourced session recording.

## Context
The Go kimi CLI has most infrastructure in place but several tools are implemented yet not registered/wired into the agent, some LLM-callable tools are missing entirely, and the transcript system is a stub. The TypeScript codebase at `/Users/jiangzhaohua/codes/kimi-code` is the reference implementation.

## Success Criteria
- WebSearch tool is registered and callable by the LLM agent
- EnterPlanMode and ExitPlanMode tools are registered and functional
- SkillTool is registered so the model can invoke skills
- SelectTools is registered for progressive tool disclosure
- Goal tools (CreateGoal, GetGoal, UpdateGoal, SetGoalBudget) are implemented and registered
- Cron tools (CronCreate, CronDelete, CronList) are implemented and registered
- Individual Agent tool for sub-agent lifecycle is implemented and registered
- `secondary_model` slash command is implemented
- Steering TUI integration is complete (queue UI, Ctrl+S key, auto-pickup)
- Transcript system provides event-sourced session recording with operations, frames, turns, and pagination
- `task go:build` succeeds and `task go:test` passes

## Constraints
- Must not break existing functionality
- Must follow Go conventions and existing code patterns
- Must pass `task go:lint`
- MCP integration is explicitly out of scope

## Out of Scope
- MCP client integration (deferred to later phase)
- VS Code extension, Web UI, Inspector, Visualizer (separate products)
- MiniDB full stack (intentionally using FileStore)
- ACP adapter full server (wire types already done)
- Plugin marketplace

## Created
2026-08-02
