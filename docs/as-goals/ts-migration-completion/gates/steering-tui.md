# Gate: Steering TUI Integration

## Condition
The steering system is fully integrated into the TUI: message queuing during streaming, Ctrl+S steering key binding, auto-pickup on stream completion, and the SteeringTool drains queued input at step boundaries.

## Evidence Required
- [ ] Steering queue UI indicator in TUI → `internal/cli/tui.go`
- [ ] Ctrl+S key binding for steering → `internal/tui/keymap/`
- [ ] Auto-pickup logic when streaming completes → `internal/cli/tui.go`
- [ ] Message queue during agent streaming → `internal/cli/tui.go`
- [ ] SteeringTool step-boundary drain integration verified
- [ ] Build and tests pass

## Verification Method
1. Run `task go:build` — exits 0
2. Run `task go:test` — exits 0
3. Code review of TUI integration points

## Owner
Engineer
