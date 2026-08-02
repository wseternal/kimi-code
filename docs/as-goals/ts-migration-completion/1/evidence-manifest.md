# Evidence Manifest — Iteration 1

## Gate Status

| Gate | Status | Evidence | Owner |
|------|--------|----------|-------|
| CLI Option Parity | ✅ Pass | `internal/cli/root.go` — CLIOptions struct with Yolo, Auto, Model, OutputFormat, AddDirs, Plan fields; flag parsing for --yolo, --auto, --model, --output-format, --add-dir, --plan | Engineer |
| Headless Mode Wired | ✅ Pass | `internal/cli/root.go` runHeadless() — full agent loop with provider creation, tool registry, permission chain, skill discovery, system prompt, multi-step generate loop, text/stream-json output | Engineer |
| Server Routes Wired | ⚠️ Partial | `internal/kapserver/server.go` + `routes.go` — 19 routes registered; 13 TS route modules still missing (approvals, questions, modelCatalog, oauth, tasks, etc.) | Engineer |
| Build Test Lint Clean | ✅ Pass | `go build ./...` ✓, `go vet ./...` ✓, `go test -race ./...` all pass (25 packages ok) | Engineer |

## Return Shipments (Failed Gates)

### Gate: Server Routes Wired (Partial)
**Defect:** 13 TS route modules have no Go equivalent: approvals, questions, modelCatalog, oauth, tasks, files, fs, workspaceFs, workspaces, terminals, connections, guiStore, skills, transcript, webAssets, action-suffix
**Root Cause:** These routes serve the web UI / IDE clients (VSCode extension, kimi-web). The Go CLI doesn't consume them. They were intentionally lower-priority during initial migration.
**Routed To:** Architect (for prioritization decision)
**Priority:** Warning (not Critical — the CLI itself works without these routes)

## Code Quality Findings
- Critical: 0
- Warning: 3
  1. WebSocket transport (`transport.go:414`) is a stub — no real WebSocket upgrade
  2. Swarm mode in TUI shows placeholder message
  3. 3 stub doc.go packages (middleware, routes, transport)
- Suggestion: 1
  1. `routes.go:208` calls `sessionManager.Get(id)` twice in handleCompactSession (redundant)

## Commits Reviewed
- `766cc4f`: feat: wire integration gaps for TS-to-Go migration completion
