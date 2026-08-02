# Gate: Server Routes Wired

## Condition
Currently-stubbed server endpoints return real data instead of 501/empty: compact, undo, messages listing, model-catalog, and OAuth login route.

## Evidence Required
- [ ] `POST /sessions/{id}/compact` triggers context compaction → `internal/kapserver/routes/`
- [ ] `POST /sessions/{id}/undo` performs conversation undo
- [ ] `GET /sessions/{id}/messages` returns message history from transcript
- [ ] `GET /model-catalog` returns configured providers and models from config
- [ ] `POST /oauth/login` triggers OAuth device flow
- [ ] Unit or integration tests for at least compact and model-catalog
- [ ] Build/test/lint passes

## Verification Method
1. Start server, create session, verify compact endpoint triggers compaction
2. Verify model-catalog returns providers from config.toml
3. Verify messages endpoint returns transcript data
4. Run existing test suite — no regressions

## Owner
Engineer
