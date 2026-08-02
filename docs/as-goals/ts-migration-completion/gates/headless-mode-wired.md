# Gate: Headless Mode Wired

## Condition
`kimi -p "prompt"` runs the full agent loop (provider call, tool execution, multi-step), streams output to stdout, and exits with the final response. Supports both text and stream-json output formats.

## Evidence Required
- [ ] `internal/cli/root.go`: `runHeadless` calls loop.Service or equivalent agent loop
- [ ] Text output mode prints assistant text to stdout
- [ ] stream-json output mode emits newline-delimited JSON events
- [ ] Tool calls are executed and results fed back to the model
- [ ] `go build ./cmd/kimi` and `go test ./...` pass

## Verification Method
1. Inspect `runHeadless` implementation — must not contain "not yet fully wired" TODO
2. Verify the function calls provider.Generate or loop.Service.SubmitTurn
3. Verify tool execution loop exists
4. Verify output formatting (text vs stream-json)

## Owner
Engineer
