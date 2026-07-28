# Migration Trace Index

Chronological log of all completed migration steps from TypeScript to Go.

| # | Phase | Step | Description | Date | Commit |
|---|-------|------|-------------|------|--------|
| 1 | 0 — Skeleton | [0.1](phase-0-skeleton/0.1-go-module-init.md) | Initialize Go module, directory structure, Makefile, linting | 2026-07-25 | — |
| 2 | 1 — Protocol | [1.1–1.4](phase-1-protocol/1.1-1.4-wire-types.md) | Wire types, error codes, pagination, WS v2 control, events, session/message domain types, REST schemas | 2026-07-25 | — |
| 3 | 2 — Persistence | [2.1–2.2](phase-2-persistence/2.1-2.2-storage-query.md) | Store interface, FileStore backend, Query builder, AppendLog | 2026-07-25 | — |
| 4 | 3 — Kosong | [3.1–3.3](phase-3-kosong/3.1-3.3-provider-message-generate.md) | ChatProvider interface, Message/ContentPart types, stream merge, TokenUsage, Tool | 2026-07-25 | — |
| 5 | 4 — Kaos | [4.1–4.3](phase-4-kaos/4.1-4.3-interface-local-env.md) | Kaos interface, LocalKaos, environment detection, KaosProcess | 2026-07-25 | — |
| 6 | 5 — Agent Core | [5.1–5.8](phase-5-agentcore/5.1-5.8-di-config-event-session-loop-tools.md) | DI scope, config, event bus, wire service, session, agent loop, tools | 2026-07-25 | — |
| 7 | 6 — Server | [6.1–6.4](phase-6-server/6.1-6.4-http-ws-transport.md) | HTTP server, REST routes, WS transport, connection registry | 2026-07-25 | — |
| 8 | 7 — CLI & TUI | [7.1–7.4](phase-7-cli-tui/7.1-7.4-cli-klient-headless.md) | CLI entry, klient SDK, headless mode stub | 2026-07-25 | — |
| 9 | 8 — Integration | [8.1–8.4](phase-8-integration/8.1-8.4-integration-hardening.md) | Binary verification, race-detector tests, stats | 2026-07-25 | — |
| 10 | 9 — Post-Migration | [9.1–9.11](phase-9-post-migration/9.1-9.11-post-migration-improvements.md) | REPL, bubbletea TUI, OpenAI provider, TOML config, tools, streaming, bug fixes | 2026-07-26 | multiple |
| 11 | 10 — Gap Closure | [10.1–10.8](phase-10-gap-closure/10.1-10.8-gap-closure.md) | Session persistence, permissions, auth, commands, context, TUI polish, advanced features | 2026-07-26 | `66c2b752e` |
| 12 | 11 — TS Removal | [11.1–11.8](phase-11-ts-removal/11.1-11.8-ts-removal.md) | Remove ~48 MB / ~3,949 TS files, update configs/docs, create Go CI | 2026-07-26 | `a84ac3388` |
