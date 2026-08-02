# Gate: CLI Option Parity

## Condition
All CLI flags from the TS kimi-code CLI are implemented and functional in the Go CLI:
- `--yolo` / `-y`: Start in YOLO permission mode
- `--auto`: Start in fully autonomous auto mode
- `--model <model>` / `-m`: Override model for this invocation
- `--output-format <format>`: Set headless output format (text/stream-json)
- `--add-dir <dir>`: Add additional workspace directory (repeatable)
- `--plan`: Start in plan mode

## Evidence Required
- [ ] `internal/cli/root.go`: Flag parsing for all listed options
- [ ] Flags propagate to TUI model and headless runner
- [ ] `go build ./cmd/kimi` succeeds with new flags
- [ ] Manual test: `build/kimi --help` shows all flags

## Verification Method
1. Run `build/kimi --help` and verify all flags appear
2. Run `build/kimi --model nonexistent 2>&1` to verify flag is parsed
3. Verify `go build ./cmd/kimi` and `go test ./...` pass

## Owner
Engineer
