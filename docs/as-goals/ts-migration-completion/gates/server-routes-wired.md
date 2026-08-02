# Gate: Server Routes Wired

## Condition
Server routes that trigger agent actions are connected to the loop service or session manager:
- POST /sessions/:id/prompts — submits a prompt to the agent loop
- POST /sessions/:id/compact — triggers compaction
- POST /sessions/:id/undo — triggers context undo
- GET /sessions/:id/messages — returns actual message history

## Evidence Required
- [ ] `internal/kapserver/routes.go`: prompt submit route wired to loop.Service or session
- [ ] Message history route reads from session store
- [ ] Compact/undo routes call appropriate services
- [ ] No remaining "TODO: wire to agent loop" comments in server routes
- [ ] `go build ./cmd/kimi` and `go test ./...` pass

## Verification Method
1. Grep for "TODO.*wire" in `internal/kapserver/` — must return zero matches
2. Verify prompt submit handler creates/queues a turn
3. Verify messages handler reads from actual store

## Owner
Engineer
