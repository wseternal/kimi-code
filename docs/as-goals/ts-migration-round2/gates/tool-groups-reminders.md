# Gate: Tool Groups & System Reminders

## Condition
SelectTools registers meaningful tool groups for progressive disclosure. System reminders are injected into the conversation at appropriate points (e.g., available skills, rules).

## Evidence Required
- [ ] Tool groups registered → `internal/cli/tui.go` (SelectTools integration)
- [ ] System reminder injection → `internal/agentcore/agent/loop/` or context management
- [ ] Tests for group listing → existing `_test.go` files

## Verification Method
Test engineer verifies SelectTools returns groups and system reminders appear in conversation.

## Owner
Engineer
