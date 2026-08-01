# Gate: Goal Tools

## Condition
LLM-callable goal management tools (CreateGoal, GetGoal, UpdateGoal, SetGoalBudget) are implemented and registered, allowing the agent to autonomously manage goals.

## Evidence Required
- [ ] CreateGoal tool implementation → `internal/agentcore/agent/tools/goal_create.go`
- [ ] GetGoal tool implementation → `internal/agentcore/agent/tools/goal_get.go`
- [ ] UpdateGoal tool implementation → `internal/agentcore/agent/tools/goal_update.go`
- [ ] SetGoalBudget tool implementation → `internal/agentcore/agent/tools/goal_budget.go`
- [ ] All 4 tools registered in agent tool setup
- [ ] Tools integrate with existing `goal.Tracker`
- [ ] Tests for each tool → `_test.go` files
- [ ] Build and tests pass

## Verification Method
1. Run `task go:build` — exits 0
2. Run `task go:test` — exits 0
3. Verify tool structs implement the Tool interface
4. Verify registration in tool setup code

## Owner
Engineer
