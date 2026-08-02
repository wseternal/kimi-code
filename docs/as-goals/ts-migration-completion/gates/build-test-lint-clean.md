# Gate: Build Test Lint Clean

## Condition
The entire Go codebase builds, passes all tests, and passes golangci-lint with zero errors.

## Evidence Required
- [ ] `go build ./cmd/kimi` exits 0
- [ ] `go test ./...` exits 0 with race detector
- [ ] `go vet ./...` exits 0
- [ ] No compilation errors in any package

## Verification Method
1. Run `task go:build` — must exit 0
2. Run `task go:test` — must exit 0
3. Run `go vet ./...` — must exit 0

## Owner
Engineer
