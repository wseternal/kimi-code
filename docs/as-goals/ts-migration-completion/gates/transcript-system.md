# Gate: Transcript System

## Condition
An event-sourced transcript system is implemented providing: operation-based recording, frames, turns, interactions, attachments, todos, tasks, pagination, and history grouping — matching the TS reference architecture.

## Evidence Required
- [ ] Transcript model types → `internal/transcript/models.go` (ids, turn, frame, interaction, attachment, todo, item, task, meta, prompt)
- [ ] Operations and apply logic → `internal/transcript/operations.go`
- [ ] Transcript store/recorder → `internal/transcript/store.go`
- [ ] Pagination → `internal/transcript/pagination.go`
- [ ] History grouping / turn folding → `internal/transcript/grouping.go`
- [ ] Integration with session lifecycle → session create/resume uses transcript
- [ ] Tests → `_test.go` files
- [ ] Build and tests pass

## Verification Method
1. Run `task go:build` — exits 0
2. Run `task go:test` — exits 0
3. Verify model types cover TS reference (turn, frame, interaction, attachment, todo, task)
4. Verify operations support append, update, delete
5. Verify pagination and grouping work

## Owner
Engineer
