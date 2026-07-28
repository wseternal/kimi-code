# Repository-level Agent Guide

Reply in the same language as the user.

This is a Go project (`github.com/visdomtech/kimi-code`) — a CLI/TUI coding assistant. Keep the root `AGENTS.md` limited to hot-path rules: the project map, hard constraints, and workflow requirements.

## Working Principles

- Think from first principles. Start from real requirements, code facts, and verification results; if the goal is unclear, discuss it with the user first.
- Treat code, not documentation, as the source of truth. Unless the user explicitly says otherwise, do not read ordinary Markdown just to understand the implementation.
- Before making code changes, read the relevant code and the most recent constraints.
- Keep changes focused. Do not slip in unrelated refactors along the way.
- When committing, do not add any co-author attribution, and do not reveal the identity of the agent in commit messages, PR descriptions, or any explanatory text.

## Project Map

- `cmd/kimi`: CLI entry point.
- `internal/cli`: Command parsing, TUI (bubbletea), headless mode, slash commands.
- `internal/agentcore/di`: Dependency injection scope tree (App/Session/Agent).
- `internal/agentcore/config`: Configuration loading and management.
- `internal/agentcore/event`: Typed event bus (generics).
- `internal/agentcore/session`: Session lifecycle and management.
- `internal/agentcore/agent/loop`: Turn/step queue, LLM agent loop.
- `internal/agentcore/agent/tools`: Tool interface and built-in tools.
- `internal/agentcore/agent/permission`: Permission system and approval prompts.
- `internal/agentcore/agent/context`: Context window management and compaction.
- `internal/agentcore/agent/goal`: Goal tracking with system prompt injection.
- `internal/agentcore/agent/skill`: Skill discovery and catalog.
- `internal/agentcore/agent/background`: Background task management.
- `internal/kosong`: LLM provider abstraction layer.
- `internal/kaos`: OS abstraction (local filesystem, process execution).
- `internal/persistence`: Key-value store and query layer.
- `internal/protocol`: Wire types, events, WebSocket control.
- `internal/kapserver`: HTTP server, REST routes, WebSocket transport.
- `internal/klient`: Client SDK harness.
- `pkg`: Public Go packages.

## Environment Requirements

- **Go**: `>=1.24` (from `go.mod`).
- **Task**: [Taskfile](https://taskfile.dev) for build orchestration (`Taskfile.yml`).
- **golangci-lint**: for linting.

## Build & Test Commands

```sh
task go:build       # build the CLI binary to build/kimi
task go:test        # run tests with race detector
task go:lint        # golangci-lint
task go:fmt         # go fmt
task go:tidy        # go mod tidy
task go:release     # cross-compile for all platforms
task go:clean       # clean build artifacts
```

## General Coding Rules

- Follow standard Go conventions: `gofmt`, `go vet`, `golangci-lint`.
- Use constructor injection for dependencies (no global state).
- Do not add too many new test files. Prefer adding tests to existing `_test.go` files in the same package.
- When a test fails because of a user modification, default to fixing the test first; do not change the implementation to satisfy an old test unless the implementation truly has a bug.
- Do not commit throwaway scratch or exploratory files. Never stage:
  - Agent working notes or handoff/summary documents (e.g. `HANDOVER-*.md`, `HANDOFF-*.md`).
  - Throwaway prototypes or design mockups (e.g. `*-designs.html`, `*-mockup.html`).
  Before committing, run `git status` and remove anything matching these patterns. Put scratch work under `.tmp/` (gitignored).

## Workflow Requirements

- When creating a PR, the PR title must follow Conventional Commit style, e.g. `feat(cli): add session persistence`.
- When an AI agent opens or updates a PR, fill in `.github/pull_request_template.md` — link the related issue or explain the problem, then describe what changed. Do not leave placeholder text or submit a generic summary.
- Do not submit vague AI-generated PR text. The human author must understand the change well enough to explain the code, edge cases, and why the approach fits this repository.
- In public text and test data, replace real internal identifiers with neutral placeholders.
