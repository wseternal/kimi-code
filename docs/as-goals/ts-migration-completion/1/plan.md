# Iteration 1 Plan — TS Migration Completion

## Task Ordering

### Task 1: Wire existing tools into agent registry
**Files:** `internal/cli/tui.go`, `internal/agentcore/agent/tools/builtin.go`
- Add `RegisterPlanModeTools(registry, controller)` function
- Add `RegisterSelectTools(registry)` function  
- Add `RegisterSkillTools(registry, catalog, handler)` function
- WebSearch needs a provider — register when available
- Wire in both TUI mode and simple mode tool setup
- **Acceptance:** All 5 tools appear in `registry.Definitions()`

### Task 2: Implement Goal tools
**Files:** `internal/agentcore/agent/tools/goal_tools.go` (new)
- `CreateGoalTool` — calls `Tracker.CreateGoal(objective, criterion, budget, "model")`
- `GetGoalTool` — calls `Tracker.Snapshot()`
- `UpdateGoalTool` — status transitions: active→pause/resume/complete/blocked
- `SetGoalBudgetTool` — sets token/turn/time budgets
- **Acceptance:** All 4 tools implement `Tool` interface, registered with tracker

### Task 3: Implement Cron tools
**Files:** `internal/agentcore/agent/tools/cron_tools.go` (new)
- `CronCreateTool` — calls `CronManager.Create(expression, prompt, model, workDir)`
- `CronDeleteTool` — calls `CronManager.Delete(id)`
- `CronListTool` — calls `CronManager.List()` and formats output
- **Acceptance:** All 3 tools implement `Tool` interface, registered with manager

### Task 4: Implement Agent tool
**Files:** `internal/agentcore/agent/tools/agent_tool.go` (new)
- `AgentTool` — individual sub-agent lifecycle via Roster
- Input: `{prompt, description, model?, work_dir?, tools?, timeout?, run_in_background?}`
- Operations: spawn new, get result, resume
- **Acceptance:** Tool implements `Tool` interface, uses existing `swarm.Roster`

### Task 5: Implement Transcript system models and operations
**Files:** `internal/transcript/models.go`, `internal/transcript/operations.go`, `internal/transcript/store.go`, `internal/transcript/pagination.go`, `internal/transcript/grouping.go` (all new/replacing doc.go)
- Model types: TurnId, StepId, FrameId, Turn, Step, Frame, Interaction, Attachment, Todo, Task, Meta, Prompt
- Operations: reset, turn.upsert, step.upsert, frame.upsert, append, marker.upsert, task.upsert, interaction.upsert, attachment.upsert, todo.upsert, prompt.upsert, meta.merge, items.remove
- Apply function: copy-on-write reducer
- Store: in-memory with persistence hooks
- Pagination: cursor-based page extraction
- Grouping: turn folding, history grouping
- **Acceptance:** All types defined, apply function tested, build passes

### Task 6: Wire everything in tui.go and verify build
- Register Goal tools with `goal.Tracker` from session service
- Register Cron tools with `cron.CronManager`
- Register Agent tool with `swarm.Roster`
- Verify `task go:build`, `task go:test` pass

## Dependencies
- Task 1 is independent
- Tasks 2, 3, 4 are independent of each other
- Task 5 is independent
- Task 6 depends on all above
