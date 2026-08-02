# TS Migration Completion

## Goal
All user-facing functionality from the TypeScript kimi-code CLI (`/Users/jiangzhaohua/codes/kimi-code`) is reimplemented and wired in the Go kimi-code CLI (`/Users/jiangzhaohua/codes/wseternal/kimi-code`), so the Go binary is a drop-in replacement.

## Context
- The Go codebase has 232 Go source files implementing 91 cataloged functional gaps (all marked "Done" in gap-catalog.md).
- All packages build and tests pass (protocol, agentcore, kosong, kaos, kapserver, oauth, persistence, transcript, audit, cli, etc.).
- **However**, the agent loop (`loop.Service`) is not wired to: the TUI (which has its own inline loop), headless mode, server routes, or klient harness.
- The CLI is missing several flags present in the TS version: `--yolo`, `--auto`, `--model`, `--output-format`, `--skills-dir`, `--agent`, `--agent-file`, `--add-dir`, `--plan`.
- CLI subcommands `upgrade`, `acp`, `web` are missing.
- Headless mode (`kimi -p`) prints "not yet fully wired" instead of executing.

## Success Criteria
- `kimi -p "prompt"` runs headless with streaming output and exits with the agent's response (text + stream-json formats).
- `kimi --model <name>`, `kimi --yolo`, `kimi --auto`, `kimi --plan` work on the command line.
- `kimi server` routes that trigger agent actions (prompts, compact, undo) are wired to the loop service.
- The TUI uses the shared `loop.Service` instead of its inline agent loop (or the inline loop is at least functionally equivalent with streaming, compaction, hooks, injection).
- `go build`, `go test ./...`, and `golangci-lint` all pass clean.

## Constraints
- Must remain a single Go binary (`cmd/kimi/main.go` entry point).
- Must not break existing working functionality (TUI, slash commands, session persistence).
- Must follow Go conventions and existing project patterns (constructor injection, event bus, etc.).
- Each commit must build and pass tests.

## Out of Scope
- Node.js SDK (`packages/node-sdk/`) — Go consumers use native HTTP/WS.
- VS Code extension (`apps/vscode/`) — separate product.
- Web apps (`apps/kimi-web/`, `apps/vis/`, `apps/kimi-inspect/`) — separate products.
- `tree-sitter-bash` — Go has native tree-sitter if needed.
- Migration legacy scripts — one-time migration tooling.
- The `upgrade` subcommand (requires release infrastructure specific to TS build).

## Created
2026-08-02
