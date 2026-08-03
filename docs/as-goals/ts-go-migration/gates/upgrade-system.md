# Gate: Upgrade System

## Condition
A `kimi upgrade` (alias `kimi update`) command checks for newer versions and can install updates when available.

## Evidence Required
- [ ] Upgrade command registered in `internal/cli/root.go` or new file
- [ ] Version check logic (compare current vs latest from GitHub releases or registry)
- [ ] Download and replace binary (or delegate to package manager)
- [ ] User-friendly output showing current/latest version

## Verification Method
- Code review: upgrade flow is safe (no arbitrary code execution)
- Build passes
- Command appears in help

## Owner
Senior Software Engineer
