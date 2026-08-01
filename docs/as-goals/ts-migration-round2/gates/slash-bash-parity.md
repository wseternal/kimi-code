# Gate: Slash Command Parity & Bash Background

## Condition
Stub slash commands have real implementations (not just print statements). Bash tool supports `run_in_background` parameter for long-running commands.

## Evidence Required
- [ ] Stub commands replaced with real implementations → `internal/cli/tui.go` or `internal/cli/commands.go`
- [ ] Bash `run_in_background` parameter → `internal/agentcore/agent/tools/bash.go` or equivalent
- [ ] Tests for new behavior → existing `_test.go` files

## Verification Method
Test engineer verifies each previously-stub command has real behavior, and Bash tool handles background execution.

## Owner
Engineer
