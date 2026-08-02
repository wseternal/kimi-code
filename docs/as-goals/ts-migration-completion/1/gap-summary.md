# Gap Summary — Iteration 1

## Failed Gates
- **Server Routes Wired (Partial)**: 13 TS route modules have no Go equivalent. These serve web UI / IDE clients. Priority routes: approvals, questions, modelCatalog, oauth, tasks.

## Unresolved Findings
- Warning: WebSocket transport is a stub (no real upgrade, no heartbeat, no message framing)
- Warning: Swarm mode in TUI shows placeholder message
- Warning: 3 stub doc.go packages (middleware, routes, transport sub-packages)
- Suggestion: Windows file lock is a stub (LockFileEx not implemented)

## Next Iteration Focus
1. Add missing high-priority server route handlers: approvals, questions, modelCatalog, oauth, tasks
2. Add medium-priority routes: files, fs, workspaceFs, workspaces, skills, transcript
3. Clean up stub doc.go packages (remove or merge into parent)
4. Fix redundant sessionManager.Get in handleCompactSession (DONE)
