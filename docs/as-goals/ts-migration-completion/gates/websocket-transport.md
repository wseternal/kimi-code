# Gate: WebSocket Transport

## Condition
WebSocket clients can connect to the server at `/api/v1/ws`, complete the hello handshake, subscribe to session events, and receive real-time event streaming with seq/epoch-based replay on reconnect.

## Evidence Required
- [ ] WebSocket upgrade handler in server → `internal/kapserver/transport/`
- [ ] Client hello / server hello handshake → `internal/protocol/ws/`
- [ ] Session event subscription and broadcasting
- [ ] Seq/epoch journaling for replay on reconnect
- [ ] Ping/pong heartbeat
- [ ] Unit tests for WS handshake, event delivery, and replay
- [ ] Build/test/lint passes

## Verification Method
1. Start server, connect WebSocket client, verify handshake completes
2. Create a session, subscribe to events, submit a prompt, verify events arrive
3. Disconnect and reconnect with cursor, verify replay works
4. Run existing test suite — no regressions

## Owner
Engineer
