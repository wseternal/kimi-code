# Gate: Hook System

## Condition
A hook system exists that supports PreToolUse, PostToolUse, and Session lifecycle events. Shell command hooks can be defined in config.toml and are executed at the appropriate lifecycle points.

## Evidence Required
- [ ] Hook types defined → `internal/agentcore/agent/hooks/` or equivalent
- [ ] Config.toml hook parsing → `internal/agentcore/config/config.go`
- [ ] Hook execution integrated into tool pipeline → `internal/agentcore/agent/tools/` or `internal/agentcore/agent/loop/`
- [ ] Tests for hook execution → existing `_test.go` files

## Verification Method
Test engineer verifies hook types exist, config parsing works, and hooks fire at correct lifecycle points.

## Owner
Engineer
