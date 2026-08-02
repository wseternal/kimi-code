# Gate: Build & Test Clean

## Condition
The Go CLI builds without errors and all existing tests pass. No lint errors.

## Evidence Required
- [ ] `task go:build` succeeds (binary produced at `build/kimi`)
- [ ] `task go:test` passes (all tests green with race detector)
- [ ] `task go:lint` passes (no golangci-lint errors)

## Verification Method
- Run all three commands and verify clean output
- Any pre-existing test failures should not regress

## Owner
Senior Software Engineer
