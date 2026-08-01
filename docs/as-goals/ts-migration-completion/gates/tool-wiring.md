# Gate: Tool Wiring

## Condition
All existing but unregistered tools (WebSearch, EnterPlanMode, ExitPlanMode, SkillTool, SelectTools) are registered in the agent's tool registry and callable by the LLM during a session.

## Evidence Required
- [ ] Tool registration code in `internal/cli/tui.go` or tool setup path → grep shows all 5 tools registered
- [ ] WebSearch tool is included in default tool list → `internal/cli/tui.go`
- [ ] EnterPlanMode and ExitPlanMode are registered → `internal/cli/tui.go`
- [ ] SkillTool is registered → `internal/cli/tui.go`
- [ ] SelectTools is registered → `internal/cli/tui.go`
- [ ] Build passes → `task go:build` output
- [ ] Tests pass → `task go:test` output

## Verification Method
1. Run `task go:build` — exits 0
2. Run `task go:test` — exits 0
3. Grep for tool names in registration code — all 5 found

## Owner
Engineer
