# Kimi Code CLI (Go)

A terminal-native AI coding agent, redesigned from the ground up in Go.

[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)](https://go.dev)
[![GitHub](https://img.shields.io/badge/github-wseternal%2Fkimi-code-181717?logo=github)](https://github.com/wseternal/kimi-code)

> This is the Go rewrite of [Kimi Code CLI](https://github.com/wseternal/kimi-code) — a single-binary, terminal-native AI coding agent. The original TypeScript implementation has been fully replaced by a self-contained Go codebase with zero Node.js dependencies.

## What is Kimi Code CLI

Kimi Code CLI is an AI coding agent that runs in your terminal. It reads and edits code, runs shell commands, searches files, fetches web pages, and autonomously decides the next step based on feedback. It works out of the box with Kimi models and can also be configured to use any OpenAI-compatible provider.

## Install

### From source

Requirements: Go ≥ 1.24, [Taskfile](https://taskfile.dev) (optional).

```sh
git clone https://github.com/wseternal/kimi-code.git
cd kimi-code
go build -o build/gkimi ./cmd/kimi
```

Or with Taskfile:

```sh
task go:build          # builds to build/gkimi
```

### Pre-built binaries

Pre-built binaries for macOS, Linux, and Windows are available on the [releases page](https://github.com/wseternal/kimi-code/releases).

## Quick Start

```sh
cd your-project
gkimi
```

On first launch, run `/login` inside the TUI and choose your preferred authentication method. Then try:

```
Take a look at this project and explain its main directories.
```

## Features

- **Single binary.** No Node.js, no runtime dependencies. One executable, ready in milliseconds.
- **Interactive TUI.** Built with [bubbletea](https://github.com/charmbracelet/bubbletea) — streaming output, collapsible thinking/tool blocks, readline keybindings, syntax-aware rendering.
- **Session persistence.** Sessions are saved to disk automatically. Resume with `gkimi -S <id>`, continue the last session with `gkimi -c`, or pick from a list with `/sessions`.
- **34+ slash commands.** Session management (`/fork`, `/title`, `/undo`, `/compact`, `/export-md`), agent control (`/goal`, `/swarm`), configuration (`/model`, `/provider`, `/auto`), and more.
- **Built-in tools.** Bash execution, file read/write/edit, glob pattern matching, ripgrep-backed search, web fetch (with SSRF guard), web search, background tasks, and todo list management.
- **Permission system.** 8-policy approval chain with interactive TUI prompts, session-scoped approval, auto mode, and sensitive file detection.
- **Context management.** Token tracking, automatic compaction, undo support, and real-time usage display.
- **Skill discovery.** Custom skills from `.agents/skills/*/SKILL.md` are auto-discovered and exposed as `/skill:name` commands.
- **Headless mode.** Run `gkimi -p "your prompt"` for non-interactive scripting and CI pipelines.
- **OpenAI-compatible providers.** Works with Kimi, OpenAI, Anthropic, Google, or any compatible endpoint via TOML config.

## Configuration

Kimi Code CLI reads configuration from `~/.gkimi-code/config.toml`. Key sections:

```toml
default_model = "kimi-latest"

[providers.kimi]
api_key = "your-api-key"
base_url = "https://api.moonshot.ai/v1"
```

## Development

```sh
task go:build          # build the CLI binary to build/gkimi
task go:test           # run tests with race detector
task go:lint           # golangci-lint
task go:fmt            # go fmt
task go:run            # run the CLI in dev mode
task go:release        # cross-compile for all platforms
```

### Project Structure

```
cmd/kimi/              CLI entry point
internal/cli/          TUI (bubbletea), commands, headless mode, auth
internal/agentcore/    Agent engine
  ├── di/              Dependency injection (App/Session/Agent scopes)
  ├── config/          Configuration loading
  ├── session/         Session lifecycle and persistence
  └── agent/
      ├── loop/        Turn/step queue, LLM agent loop
      ├── tools/       Built-in tools (Bash, Read, Write, Grep, etc.)
      ├── permission/  Permission system and approval prompts
      ├── context/     Context window management and compaction
      ├── goal/        Goal tracking with system prompt injection
      ├── skill/       Skill discovery and catalog
      └── background/  Background task management
internal/kosong/       LLM provider abstraction
internal/kaos/         OS abstraction (filesystem, processes)
internal/persistence/  Key-value store
internal/protocol/     Wire types, events, WebSocket
internal/kapserver/    HTTP server, REST, WebSocket transport
pkg/                   Public Go packages
```

## License

Released under the [MIT License](LICENSE).
