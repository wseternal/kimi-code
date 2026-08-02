# Review — Iteration 1

## Summary
Iteration 1 focused on wiring integration gaps: CLI option parity, headless mode agent loop, server route handlers, and klient harness. All 91 cataloged gaps from the migration plan are implemented. The build is clean (build, vet, test all pass).

## What Was Done
1. **CLI Option Parity**: Added `CLIOptions` struct with all TS CLI flags (--yolo, --auto, --model, --output-format, --add-dir, --plan). Flag parsing properly creates options and passes them to TUI/headless modes.
2. **Headless Mode**: Full agent loop implementation with provider creation via `providers.NewFromConfig`, tool registry setup, permission chain selection, skill discovery, system prompt building, and multi-step generate→tool-execute loop. Supports both text and stream-json output formats.
3. **Server Routes**: Replaced TODO stubs with real handlers. Added `ServerOption` pattern (WithPromptSubmit, WithCancelFunc). All 19 registered routes have functional handlers.
4. **Klient Harness**: Added PromptHandler callback for delegating prompt submission.
5. **Build Fix**: Changed `session.StatusBusy` → `session.StatusRunning` across kapserver and klient.

## Gate Analysis
- **CLI Option Parity**: PASS — all flags from TS CLI are present and wired
- **Headless Mode Wired**: PASS — full agent loop from provider creation through tool execution
- **Server Routes Wired**: PARTIAL — 19 routes work, but 13 TS route modules (approvals, questions, modelCatalog, oauth, tasks, files, fs, workspaceFs, workspaces, terminals, connections, guiStore, skills, transcript) are not implemented. These primarily serve web UI / IDE clients.
- **Build Test Lint Clean**: PASS — `go build`, `go vet`, `go test -race` all clean

## Remaining Gaps (Beyond Original 91)
1. **WebSocket transport stub** — No real WebSocket upgrade (uses plain HTTP). TS uses `ws` library with full upgrade, heartbeat, message framing.
2. **13 missing server route modules** — Primarily for web UI / IDE integration (approvals, questions, modelCatalog, oauth, tasks, files, fs, workspaceFs, workspaces, terminals, connections, guiStore, skills, transcript, webAssets).
3. **Swarm mode placeholder** — TUI shows "placeholder — full implementation pending" for swarm mode.
4. **Windows file lock stub** — `lock_windows.go` has TODO for LockFileEx.

## Architecture Assessment
The Go codebase (234 .go files, ~38K LOC) achieves functional parity with the TS CLI app (518 files, ~80K LOC) for core CLI functionality. The file count difference is explained by:
- TS monorepo includes 5 apps (CLI + web + vscode + vis + inspect) and 16 packages (including legacy agent-core v1)
- Go is more concise per feature
- TS has higher test-to-source ratio (1:1 in CLI vs 18% in Go)

## Verdict
3/4 gates pass. Gate "Server Routes Wired" is partial — the 19 core routes work but 13 TS-specific route modules for web UI / IDE clients are missing. These are lower priority for a CLI-focused migration.
