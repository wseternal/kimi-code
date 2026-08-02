# Gate: Shell Mode & File Autocomplete

## Condition
TUI supports shell mode (`!` prefix on empty input runs terminal commands inline) and file autocomplete (`@` triggers path completion).

## Evidence Required
- [ ] Shell mode: `!` prefix detection in TUI input handler
- [ ] Shell mode: command execution via kaos with output fed back to conversation
- [ ] File autocomplete: `@` trigger detection in TUI input
- [ ] File autocomplete: path completion suggestions using kaos glob

## Verification Method
- Code review: TUI input handling extended
- Build passes
- No regression in existing TUI functionality

## Owner
Senior Software Engineer
