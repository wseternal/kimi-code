# Gate: Goal Tools Complete

## Condition
GetGoal and SetGoalBudget tools are implemented alongside existing CreateGoal/UpdateGoal, and goal queue operations (`/goal next`, `/goal next manage`) are supported.

## Evidence Required
- [ ] GetGoal tool in `internal/agentcore/agent/tools/goal_tools.go`
- [ ] SetGoalBudget tool in `internal/agentcore/agent/tools/goal_tools.go`
- [ ] Goal queue data structure in `internal/agentcore/agent/goal/`
- [ ] `/goal next` and `/goal next manage` slash commands in `internal/cli/commands.go`

## Verification Method
- Code review: tools follow existing patterns
- Build passes
- Tests pass

## Owner
Senior Software Engineer
