# Gate: Build-Test-Lint Clean

## Condition
The Go codebase compiles cleanly, all tests pass with race detector, and golangci-lint reports zero issues. This gate must pass every iteration (regression protection).

## Evidence Required
- [ ] `task go:build` exits 0
- [ ] `task go:test` exits 0 (all packages)
- [ ] `task go:lint` exits 0 (or only pre-existing warnings)

## Verification Method
Run `task go:build && task go:test && task go:lint` and capture exit codes.

## Owner
Engineer
