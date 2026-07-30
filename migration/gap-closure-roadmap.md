# Go Rewrite Gap Closure — Master Roadmap

Systematic plan to close all 91 functional gaps between the TypeScript kimi-code and the Go rewrite.
Organized into 8 prioritized cycles based on dependency order and impact.

## Status

| Cycle | Name | Gaps Covered | Status |
|-------|------|-------------|--------|
| 1 | Foundation Hardening | #11, #12, #3, #14, #49 | **Done** |
| 2 | Core Agent Loop | #1, #2, #4, #5, #6, #7 | **Done** |
| 3 | Provider Adapters | #8, #9, #10, #42, #44, #45, #46 | **Done** |
| 4 | Permission & Safety | #29, #30, #31, #32, #40, #41 | **Done** |
| 5 | Protocol & Events | #23-#27, #71-#73 | Pending |
| 6 | Server & Klient | #19-#22, #59-#61 | Pending |
| 7 | Advanced Agent | #33-#39 | Pending |
| 8 | TUI & Polish | #15-#18, #50-#58, #70, #77-#91 | Pending |

## Dependency Graph

```
Cycle 1 (Foundation) ──→ Cycle 2 (Agent Loop) ──→ Cycle 3 (Providers)
                     ──→ Cycle 4 (Permissions)
                     ──→ Cycle 5 (Protocol) ──→ Cycle 6 (Server/Klient)
                                               ──→ Cycle 7 (Advanced Agent)
                                               ──→ Cycle 8 (TUI Polish)
```

- Cycles 1-2 are prerequisites for everything else
- Cycles 3-4 can partially parallelize after Cycle 2
- Cycles 5-8 are independent of each other but depend on Cycle 5 (Protocol)

## Cycle 1: Foundation Hardening (P0)

**Goal**: Lay the foundation that everything else depends on.
**Gaps**: #11, #12, #3, #14, #49
**Files**: ~10
**Status**: Done

| Gap | Description | Status |
|-----|-------------|--------|
| #11 | Typed Error Hierarchy for Kosong | Done |
| #12 | Model Capability System | Done |
| #3 | Message Projector | Done |
| #14 | Consecutive User Message Merging | Done (part of #3) |
| #49 | Generate() High-Level Wrapper | Done |

## Cycle 2: Core Agent Loop (P0)

**Goal**: Make the agent loop production-ready with proper message management.
**Gaps**: #1, #2, #4, #5, #6, #7
**Files**: ~15
**Status**: Done

| Gap | Description | Status |
|-----|-------------|--------|
| #1 | Streaming integration in agent loop (incremental token events via event bus) | Done |
| #2 | ContextMemory — full message management (append, undo, import, clear, token counting) | Done |
| #4 | Compaction auto-trigger + overflow recovery | Done |
| #5 | Injection Manager (7 injectors: goal, plan-mode, permission-mode, todo-list, tools-diff, plugin-session-start, background-tasks) | Done |
| #6 | Full Goal lifecycle (active/paused/blocked/complete, budget tracking, persistence) | Done |
| #7 | Goal continuation driver (automatic multi-turn continuation) | Done |

## Cycle 3: Provider Adapters (P1)

**Goal**: Enable multi-provider support beyond OpenAI-compatible.
**Gaps**: #8, #9, #10, #42, #44, #45, #46
**Files**: ~12

| Gap | Description | Priority |
|-----|-------------|----------|
| #8 | Anthropic provider (native content blocks, thinking config, tool_use format) | Critical |
| #9 | Google GenAI provider (Gemini/Vertex adapter) | Critical |
| #10 | Kimi-specific provider (reasoning_content/details, thinking config, prompt_cache_key) | Critical |
| #42 | OpenAI Responses API provider (/responses endpoint) | Important |
| #44 | Reasoning key dialect detection (auto-detect reasoning_content vs reasoning_details) | Important |
| #45 | Tool call ID normalization (per-provider ID sanitization) | Important |
| #46 | Kimi schema normalization (JSON Schema $ref dereferencing) | Important |

## Cycle 4: Permission & Safety (P1)

**Goal**: Full permission system with mode switching and additional tools.
**Gaps**: #29, #30, #31, #32, #40, #41
**Files**: ~10

| Gap | Description | Priority |
|-----|-------------|----------|
| #29 | Permission mode system (runtime mode switching: manual/yolo/auto) | Important |
| #30 | Comprehensive policy set (14 policies: file-access-ask, yolo-mode, auto-mode, etc.) | Important |
| #31 | Plan mode tools (EnterPlanMode, ExitPlanMode) | Important |
| #32 | AskUser tool (structured user questions) | Important |
| #40 | Tool result budgeting (truncate oversized tool results) | Important |
| #41 | ReadMedia tool (read image/video/audio files) | Important |

## Cycle 5: Protocol & Events (P2)

**Goal**: Rich typed event system and complete REST contract.
**Gaps**: #23-#27, #71-#73
**Files**: ~20

| Gap | Description | Priority |
|-----|-------------|----------|
| #23 | Typed event structs (~50 strongly typed event interfaces) | Critical |
| #24 | PromptOrigin discriminated union (13 variants) | Critical |
| #25 | AgentPhase state machine (8 phases) | Critical |
| #26 | Approval/Question wire types | Critical |
| #27 | REST endpoint contract (21 definitions) | Critical |
| #71 | KimiErrorCode string enum (~75 codes) | Important |
| #72 | KimiErrorPayload (code, message, retryable, cause) | Important |
| #73 | ToolInputDisplay/ToolResultDisplay (12 variants each) | Important |

## Cycle 6: Server & Klient (P2)

**Goal**: Full daemon server with auth, WebSocket, and client SDK.
**Gaps**: #19-#22, #59-#61
**Files**: ~25

| Gap | Description | Priority |
|-----|-------------|----------|
| #19 | Auth system (token store, credential validation, bearer middleware) | Critical |
| #20 | WebSocket v1 protocol (connection lifecycle, event broadcasting) | Critical |
| #21 | Klient SDK (channel SPI, contract system, facades, transports) | Critical |
| #22 | Most REST routes missing (only 2 of 21 exist) | Critical |
| #59 | Search service (snippet extraction, wire extract) | Important |
| #60 | Snapshot service (point-in-time session snapshot) | Important |
| #61 | Security middleware (host check, origin/CORS, rate limiting) | Important |

## Cycle 7: Advanced Agent (P3)

**Goal**: Sub-agent, swarm, cron, and advanced tool features.
**Gaps**: #33-#39
**Files**: ~18

| Gap | Description | Priority |
|-----|-------------|----------|
| #33 | AgentSwarm tool + subagent infrastructure | Important |
| #34 | Background task persistence + reconciliation | Important |
| #35 | Cron/Scheduling system (CronManager, scheduler, persistence, tools) | Important |
| #36 | SkillTool (model-invoked skill invocation) | Important |
| #37 | Skill Manager activation flow | Important |
| #38 | select_tools (progressive tool disclosure) | Important |
| #39 | Dynamic tools support (progressive tool loading) | Important |

## Cycle 8: TUI & Polish (P3)

**Goal**: Feature-complete TUI with remaining nice-to-have gaps.
**Gaps**: #15-#18, #50-#58, #70, #77-#91
**Files**: ~20

| Gap | Description | Priority |
|-----|-------------|----------|
| #15 | Session picker / session list dialog | Important |
| #16 | Image attachment support | Important |
| #17 | MCP server integration UI | Important |
| #18 | Plugin system | Important |
| #50 | Configurable keybindings | Important |
| #51 | Background task status display | Important |
| #52 | Goal queue display | Important |
| #53 | Terminal notification system | Important |
| #54 | Message replay | Important |
| #55 | Paging system | Important |
| #56 | Render cache | Important |
| #57 | Tmux keyboard handling | Important |
| #58 | Foreground task management | Important |
| #70 | Telemetry client | Important |
| #77 | Notification XML | Nice-to-have |
| #78 | LLM request logging/recording | Nice-to-have |
| #79 | Session hooks engine | Nice-to-have |
| #80 | Compaction strategies (full, micro, handoff) | Nice-to-have |
| #81 | System reminder injection | Nice-to-have |
| #82 | Tool call deduplication | Nice-to-have |
| #83 | Stream decode stats | Nice-to-have |
| #84 | Generate callbacks (onMessagePart, onToolCall) | Nice-to-have |
| #85 | ACP adapter (IDE integration) | Nice-to-have |
| #86 | Text decode error modes | Nice-to-have |
| #87 | File line-ending analysis | Nice-to-have |
| #88 | OAuth token state machine | Nice-to-have |
| #89 | OAuth managed usage/feedback | Nice-to-have |
| #90 | ULID request ID | Nice-to-have |
| #91 | AsyncAPI generator | Nice-to-have |

## Intentionally Not Ported

| TS Package | Reason |
|-----------|--------|
| `node-sdk/` (20 files) | Node.js-specific — Go consumers use native HTTP/WS |
| `tree-sitter-bash/` (7 files) | Go has native tree-sitter bindings available if needed |
| `migration-legacy/` (27 files) | One-time migration from legacy storage |
| `minidb/` full stack | Go uses simple FileStore — may adopt BadgerDB or similar later |
| Apps: `kimi-inspect/`, `kimi-web/`, `vis/`, `vscode/` | Separate products, not part of core CLI |

## Estimated Effort

| Cycle | Effort | Items | Priority |
|-------|--------|-------|----------|
| **1**: Foundation | ~10 files | 5 | **P0** |
| **2**: Agent Loop | ~15 files | 6 | **P0** |
| **3**: Providers | ~12 files | 7 | **P1** |
| **4**: Permissions | ~10 files | 6 | **P1** |
| **5**: Protocol | ~20 files | 8 | **P2** |
| **6**: Server/Klient | ~25 files | 7 | **P2** |
| **7**: Advanced Agent | ~18 files | 7 | **P3** |
| **8**: TUI Polish | ~20 files | 29 | **P3** |

**Total**: 91 gaps across 8 cycles. ~130 files estimated.
