# Gap Summary — Iteration 1

## Failed Gates
- **Server Routes Wired**: compact, undo, messages listing, and OAuth login endpoints still return 501/empty. Need to wire:
  - `POST /sessions/{id}/compact` → agent context compaction
  - `POST /sessions/{id}/undo` → conversation undo
  - `GET /sessions/{id}/messages` → transcript data
  - `POST /oauth/login` → OAuth device flow
  - `GET /sessions/{id}/transcript` → transcript store

## Unresolved Findings
- Warning: compact/undo not wired to agent loop
- Warning: messages listing not wired to transcript
- Suggestion: MCP HTTP/SSE transports not implemented
- Suggestion: agent profile tool allow/deny not enforced

## Next Iteration Focus
1. Wire server compact route to trigger context compaction via a callback
2. Wire server undo route with a conversation undo callback
3. Wire messages listing to return transcript/audit data
4. Wire OAuth login to trigger device flow
5. Wire transcript listing to return real entries
