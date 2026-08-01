# Gate: Cron Tools

## Condition
LLM-callable cron management tools (CronCreate, CronDelete, CronList) are implemented and registered, allowing the agent to manage scheduled tasks.

## Evidence Required
- [ ] CronCreate tool implementation → `internal/agentcore/agent/tools/cron_create.go`
- [ ] CronDelete tool implementation → `internal/agentcore/agent/tools/cron_delete.go`
- [ ] CronList tool implementation → `internal/agentcore/agent/tools/cron_list.go`
- [ ] All 3 tools registered in agent tool setup
- [ ] Tools integrate with existing `cron.Manager`
- [ ] Tests for each tool → `_test.go` files
- [ ] Build and tests pass

## Verification Method
1. Run `task go:build` — exits 0
2. Run `task go:test` — exits 0
3. Verify tool structs implement the Tool interface
4. Verify registration in tool setup code

## Owner
Engineer
