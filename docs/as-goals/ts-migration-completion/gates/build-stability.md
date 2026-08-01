# Gate: Build Stability

## Condition
The entire Go project builds successfully, all tests pass, and linting passes with no new errors introduced by the migration work.

## Evidence Required
- [ ] `task go:build` exits 0
- [ ] `task go:test` exits 0 (all tests pass, no race conditions)
- [ ] `task go:lint` exits 0 (no new lint violations)
- [ ] No regressions in existing tests

## Verification Method
1. Run `task go:build` — capture output
2. Run `task go:test` — capture output
3. Run `task go:lint` — capture output
4. All must exit 0

## Owner
Engineer (produces), Test Engineer (verifies)
