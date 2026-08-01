# Gate: Session Fork & Undo

## Condition
Session fork creates a branch of the current session (preserving history up to the fork point). Session undo withdraws the last user prompt and its associated agent responses.

## Evidence Required
- [ ] Fork logic → `internal/agentcore/session/` or `internal/cli/session_service.go`
- [ ] Undo logic → same location
- [ ] Slash commands `/fork` and `/undo` → `internal/cli/`
- [ ] Tests for fork/undo behavior → existing `_test.go` files

## Verification Method
Test engineer verifies fork creates independent session branch, undo removes last prompt+response pair.

## Owner
Engineer
