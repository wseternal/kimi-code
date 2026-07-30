# Gap Catalog: TS → Go Rewrite

Complete inventory of 91 functional gaps between the TypeScript kimi-code project and the Go rewrite.
Each gap includes the TS source reference, description, Go status, severity, and assigned cycle.

---

## Agent Core Gaps

### Gap #1: Streaming Integration in Agent Loop
- **Cycle**: 2 (Core Agent Loop)
- **Severity**: Critical
- **TS Source**: `packages/agent-core/src/agent/index.ts` — incremental token events via event bus
- **Description**: The agent loop emits incremental streaming events (token-by-token) via the event bus. The TUI subscribes to these events for real-time display.
- **Go Status**: `loop.Service` sends full conversation each turn (goose pattern), no incremental streaming output. The TUI's `runLLMStream` works with a channel-based stream but the loop doesn't produce incremental events.
- **Files Affected**: `internal/agentcore/agent/loop/service.go`, event bus wiring

### Gap #2: ContextMemory — Full Message Management
- **Cycle**: 2
- **Severity**: Critical
- **TS Source**: `packages/agent-core/src/agent/context/` — append, undo, import, clear, token counting, projection
- **Description**: Full conversation memory with message-level CRUD operations: append new messages, undo last N, import external messages, clear history, per-message token counting.
- **Go Status**: `context/manager.go` has basic token tracking only — no message-level CRUD operations.
- **Files Affected**: `internal/agentcore/agent/context/manager.go`

### Gap #3: Message Projector
- **Cycle**: 1 (Foundation)
- **Severity**: Critical
- **TS Source**: `packages/agent-core/src/agent/context/projector.ts` — synthesize missing tool results, drop orphans, merge consecutive assistants
- **Description**: Before sending to provider, repairs the message array: synthesizes placeholder tool results for orphaned tool_calls, drops orphan tool_results, merges consecutive same-role messages.
- **Go Status**: **IMPLEMENTED** in Cycle 1. Full projector with adjacency repair, orphan drop, dedup, consecutive merge, leading non-user drop.
- **Files Affected**: `internal/agentcore/agent/context/projector.go` (NEW), `projector_test.go` (NEW)

### Gap #4: Compaction Auto-Trigger + Overflow Recovery
- **Cycle**: 2
- **Severity**: Critical
- **TS Source**: `packages/agent-core/src/agent/compaction/` — auto-compact when context exceeds threshold, handle `APIContextOverflowError` with shrink-and-retry
- **Description**: Automatically triggers compaction when context token count exceeds a configurable threshold. When the provider returns `APIContextOverflowError`, shrinks the context and retries.
- **Go Status**: Has `/compact` command only, no auto-trigger or overflow recovery.
- **Files Affected**: `internal/agentcore/agent/context/manager.go`, loop service

### Gap #5: Injection Manager (6 Injectors)
- **Cycle**: 2
- **Severity**: Critical
- **TS Source**: `packages/agent-core/src/agent/injection/` — goal, plan-mode, permission-mode, todo-list, tools-diff, plugin-session-start injectors
- **Description**: Manages system-prompt injections from 6 independent injectors. Each injector provides context-aware content (goal status, plan mode, permission mode, todo reminders, tool set changes, plugin session start).
- **Go Status**: Not present — model loses context after compaction or mode changes.
- **Files Affected**: `internal/agentcore/agent/injection/` (NEW package)

### Gap #6: Full Goal Lifecycle
- **Cycle**: 2
- **Severity**: Critical
- **TS Source**: `packages/agent-core/src/agent/goal/` + `tools/builtin/goal/` — active/paused/blocked/complete states, budget tracking, persistence, completion criterion
- **Description**: Complete goal system with 4 lifecycle states, per-goal budget tracking (tokens, turns, wall-clock), persistence across sessions, completion criterion field.
- **Go Status**: `goal.go` has basic status tracking only (active/complete).
- **Files Affected**: `internal/agentcore/agent/goal/goal.go`, goal tools

### Gap #7: Goal Continuation Driver
- **Cycle**: 2
- **Severity**: Critical
- **TS Source**: `TurnFlow.driveGoal()` — automatic multi-turn continuation with retry/backoff
- **Description**: When a goal is active and the agent finishes a turn, the continuation driver automatically starts the next turn with retry logic and exponential backoff for transient errors.
- **Go Status**: Not present — requires manual re-prompting.
- **Files Affected**: Loop service, goal package

### Gap #29: Permission Mode System
- **Cycle**: 4 (Permission & Safety)
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/agent/permission/` — runtime mode switching (manual/yolo/auto), parent delegation
- **Description**: Three permission modes: manual (ask for everything), yolo (approve everything), auto (approve safe operations, ask for risky). Supports parent delegation for nested agents.
- **Go Status**: Basic permission chain exists, no mode switching.
- **Files Affected**: `internal/agentcore/agent/permission/`

### Gap #30: Comprehensive Policy Set (14 Policies)
- **Cycle**: 4
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/agent/permission/policies/` — 14 policy implementations
- **Description**: 14 permission policies: file-access-ask, yolo-mode-approve, auto-mode-approve, user-configured-rules, plan-mode-guard-deny, exit-plan-mode-review-ask, goal-start-review-ask, default-tool-approve, deny-all, git-cwd-write-approve, auto-mode-ask-user-question-deny, agent-swarm-exclusive-deny, fallback-ask.
- **Go Status**: Has 8 policies. Missing: file-access-ask, yolo-mode, plan-mode-guard, user-configured-rules, exit-plan-mode-review, goal-start-review, auto-mode-ask-user-question-deny, agent-swarm-exclusive-deny.
- **Files Affected**: `internal/agentcore/agent/permission/`

### Gap #33: AgentSwarm Tool + Subagent Infrastructure
- **Cycle**: 7 (Advanced Agent)
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/tools/builtin/collaboration/agent-swarm.ts`, `agent.ts` + `session/subagent-*.ts`
- **Description**: Spawn and manage sub-agents: AgentSwarm tool for batch spawning, Agent tool for individual sub-agent lifecycle, subagent-host for lifecycle management, subagent-binding for workspace sharing.
- **Go Status**: Not present. Has basic swarm flag in CLI but no implementation.
- **Files Affected**: `internal/agentcore/agent/swarm/`, new tools

### Gap #34: Background Task Persistence + Reconciliation
- **Cycle**: 7
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/agent/background/persist.ts`
- **Description**: Persist background task state to disk. On startup, reconcile persisted tasks with running processes to detect lost tasks.
- **Go Status**: Background manager is in-memory only. Lost on restart.
- **Files Affected**: `internal/agentcore/agent/background/manager.go`

### Gap #35: Cron/Scheduling System
- **Cycle**: 7
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/tools/cron/` (12 files) — CronManager, scheduler, expression parser, persistence
- **Description**: Full cron scheduling: parse cron expressions, manage scheduled tasks, persist to disk, fire at correct times, create/delete/list tools.
- **Go Status**: Not present.
- **Files Affected**: New `internal/agentcore/agent/cron/` package, new tools

### Gap #36: SkillTool (Model-Invoked)
- **Cycle**: 7
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/tools/builtin/collaboration/skill-tool.ts`
- **Description**: A tool the model can call to invoke skills. The model decides when a skill is needed and calls the SkillTool with the skill name.
- **Go Status**: Not present — skills are CLI commands only.
- **Files Affected**: New tool in `internal/agentcore/agent/tools/`

### Gap #37: Skill Manager Activation Flow
- **Cycle**: 7
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/skill/registry.ts`, `scanner.ts`
- **Description**: Runtime skill activation: scan directories, parse SKILL.md frontmatter, register skills, emit activation events/telemetry.
- **Go Status**: Basic skill discovery exists but no formal activation flow.
- **Files Affected**: `internal/agentcore/agent/skill/`

---

## LLM Provider (Kosong) Gaps

### Gap #8: Anthropic Provider
- **Cycle**: 3 (Provider Adapters)
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/providers/anthropic.ts` (1335 lines)
- **Description**: Full Anthropic SDK adapter: native content blocks, thinking config (budget/adaptive modes, effort params), tool_use/tool_result format, `pause_turn` finish reason, beta features.
- **Go Status**: Only stub `doc.go` exists.
- **Files Affected**: `internal/kosong/providers/anthropic/`

### Gap #9: Google GenAI Provider
- **Cycle**: 3
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/providers/google-genai.ts` (34.5KB)
- **Description**: Full Gemini/Vertex adapter: native `Content` format, function calling, multimodal (image/video/audio), thinking support.
- **Go Status**: Only stub `doc.go` exists.
- **Files Affected**: `internal/kosong/providers/google/`

### Gap #10: Kimi-Specific Provider
- **Cycle**: 3
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/providers/kimi.ts` (683 lines)
- **Description**: Kimi-specific features: `reasoning_content`/`reasoning_details` handling, `generationKwargs` (temperature, top_p, penalties, stop, cache key), `thinking` extra_body config, Kimi schema normalization, `prompt_cache_key`.
- **Go Status**: Only stub `doc.go`. Generic OpenAI adapter used instead, missing all Kimi-specific features.
- **Files Affected**: `internal/kosong/providers/kimi/`

### Gap #11: Typed Error Hierarchy
- **Cycle**: 1 (Foundation)
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/errors.ts` (620 lines)
- **Description**: Rich error class hierarchy: `ChatProviderError` → `APIConnectionError`, `APITimeoutError`, `APIStatusError`, `APIContextOverflowError`, `APIRequestTooLargeError`, `APIProviderRateLimitError`, `APIProviderQuotaExhaustedError`, `APIEmptyResponseError`. Each carries statusCode, requestId, retryAfterMs, traceId. Includes `IsRetryableGenerateError()` classification.
- **Go Status**: Has 3 sentinel errors + raw `fmt.Errorf`. **IMPLEMENTED** in Cycle 1.
- **Files Affected**: `internal/kosong/errors.go` (NEW), `openai.go` (MODIFIED)

### Gap #12: Model Capability Registry
- **Cycle**: 1 (Foundation)
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/capability.ts` + `providers/capability-registry.ts` (211 lines)
- **Description**: Per-provider model capability catalogs: maps model name patterns to capabilities (image_in, video_in, audio_in, thinking, tool_use, max_context_tokens). Covers OpenAI, Anthropic, Google, Kimi models.
- **Go Status**: Not present. **IMPLEMENTED** in Cycle 1.
- **Files Affected**: `internal/kosong/capability.go` (NEW), `providers/capability_registry.go` (NEW)

### Gap #13: Parallel Tool Call Routing
- **Cycle**: 3
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/generate.ts` (lines 98-184)
- **Description**: Index-based routing map (`toolCallIndexMap`) routes interleaved argument deltas from parallel tool calls to the correct `ToolCall` by `_streamIndex`.
- **Go Status**: Does sequential merging only.
- **Files Affected**: `internal/kosong/generate.go`

### Gap #14: Consecutive User Message Merging
- **Cycle**: 1 (Foundation)
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/providers/merge-user-messages.ts` (65 lines)
- **Description**: Merges consecutive same-role user turns for strict providers (Anthropic, Gemini) that reject alternating-role violations.
- **Go Status**: Not present. **IMPLEMENTED** as part of Message Projector in Cycle 1.
- **Files Affected**: Part of `projector.go`

### Gap #42: OpenAI Responses API Provider
- **Cycle**: 3
- **Severity**: Important
- **TS Source**: `packages/kosong/src/providers/openai-responses.ts` (1229 lines)
- **Description**: Dedicated provider for OpenAI's newer Responses API (`/responses` endpoint), different from Chat Completions. Handles `function_call` items, `incomplete_details`, developer role messages.
- **Go Status**: Not present.
- **Files Affected**: New provider in `internal/kosong/providers/`

### Gap #43: Model Catalog System
- **Cycle**: 3
- **Severity**: Important
- **TS Source**: `packages/kosong/src/catalog.ts` (510 lines)
- **Description**: models.dev-style catalog: `CatalogModelEntry` with limits, tool_call, reasoning, reasoning_options, modalities, interleaved; `CatalogProviderEntry` with api, env, npm, type; `CatalogModel` normalized with `supportEfforts`, `offEffort`, `alwaysThinking`, `protocol` overrides.
- **Go Status**: Not present.
- **Files Affected**: `internal/kosong/catalog.go` (NEW)

### Gap #44: Reasoning Key Dialect Detection
- **Cycle**: 3
- **Severity**: Important
- **TS Source**: `packages/kosong/src/providers/reasoning-key.ts` (90 lines)
- **Description**: Auto-detects which reasoning field (`reasoning_content`, `reasoning_details`, `reasoning`) the endpoint uses and echoes it back on outbound messages.
- **Go Status**: Hardcodes `reasoning_content` only.
- **Files Affected**: OpenAI/Kimi providers

### Gap #45: Tool Call ID Normalization
- **Cycle**: 3
- **Severity**: Important
- **TS Source**: `packages/kosong/src/providers/tool-call-id.ts` (133 lines)
- **Description**: Per-provider ID policies: sanitizes IDs to safe chars, truncates to max length, deduplicates with suffixes; different policies for Kimi (64 chars) vs Anthropic vs OpenAI Responses.
- **Go Status**: Tool call IDs passed through as-is.
- **Files Affected**: Provider adapters

### Gap #46: Kimi Schema Normalization
- **Cycle**: 3
- **Severity**: Important
- **TS Source**: `packages/kosong/src/providers/kimi-schema.ts` (472 lines)
- **Description**: JSON Schema `$ref` dereferencing, type completion, Moonshot-specific normalizations for tool parameter schemas.
- **Go Status**: Not present.
- **Files Affected**: Kimi provider

### Gap #47: Kimi File Upload (Video)
- **Cycle**: 3
- **Severity**: Important
- **TS Source**: `packages/kosong/src/providers/kimi-files.ts` (194 lines)
- **Description**: Uploads videos to Moonshot's file service, returns `ms://` URL references; handles MIME type detection, file validation.
- **Go Status**: `UploadVideo` returns "not supported" error.
- **Files Affected**: Kimi provider

### Gap #48: Empty/Think-Only Response Detection
- **Cycle**: 1
- **Severity**: Important
- **TS Source**: `packages/kosong/src/generate.ts` (lines 215-245)
- **Description**: Throws `APIEmptyResponseError` with finish reason context when response has no content/tool calls, or only thinking without text.
- **Go Status**: Returns message without validation. **IMPLEMENTED** as part of GenerateCall wrapper in Cycle 1.
- **Files Affected**: Part of `generate.go` modifications

### Gap #49: Generate() High-Level Wrapper
- **Cycle**: 1 (Foundation)
- **Severity**: Critical
- **TS Source**: `packages/kosong/src/generate.ts`
- **Description**: High-level function: takes `provider + systemPrompt + tools + history + callbacks + options`, calls `provider.generate()`, streams, merges, validates, returns `GenerateResult` with {id, message, usage, finishReason, rawFinishReason, traceId}.
- **Go Status**: `Generate()` only assembles a StreamedMessage into a Message. **IMPLEMENTED** in Cycle 1.
- **Files Affected**: `internal/kosong/generate.go`

### Gap #83: Stream Decode Stats
- **Cycle**: 8 (Polish)
- **Severity**: Nice-to-have
- **TS Source**: `packages/kosong/src/generate.ts` + `provider.ts`
- **Description**: `StreamDecodeStats`: tracks `serverDecodeMs` vs `clientConsumeMs` for performance attribution; `onStreamEnd` callback fires with stats.
- **Go Status**: `OnStreamEnd` callback exists but called with empty stats.

### Gap #84: Generate Callbacks (onMessagePart, onToolCall)
- **Cycle**: 8
- **Severity**: Nice-to-have
- **TS Source**: `packages/kosong/src/generate.ts`
- **Description**: Streaming callbacks fire per-part and per-completed-tool-call; tool calls deferred until stream end.
- **Go Status**: Not present.

---

## TUI/CLI Gaps

### Gap #15: Session Picker / Session List Dialog
- **Cycle**: 8 (TUI & Polish)
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/kimi-tui.ts`
- **Description**: Interactive session picker with search, resume, fork, delete operations.
- **Go Status**: CLI flags only (`-S`, `-c`).
- **Files Affected**: `internal/cli/tui.go`

### Gap #16: Image Attachment Support
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/image-attachment-store.ts`
- **Description**: Attach images to prompts via paste/drag, base64 encoding, preview in input area.
- **Go Status**: Not present.

### Gap #17: MCP Server Integration UI
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/mcp-server-status.ts`, `mcp-oauth.ts`, `mcp-tool-name.ts`
- **Description**: MCP server status display, MCP OAuth flow, MCP tool naming conventions.
- **Go Status**: Not present.

### Gap #18: Plugin System
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/plugin-source-label.ts`
- **Description**: Plugin discovery, labeling, session-start hooks.
- **Go Status**: Not present.

### Gap #50: Configurable Keybindings
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/pi-tui/src/`
- **Description**: User-configurable key maps via config file.
- **Go Status**: Not present.

### Gap #51: Background Task Status Display
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/background-task-status.ts`, `background-agent-status.ts`
- **Description**: Display running/completed/failed background task status in TUI.
- **Go Status**: Not present.

### Gap #52: Goal Queue Display
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/goal-queue-store.ts`
- **Description**: Visual display of queued goals in the TUI.
- **Go Status**: Not present.

### Gap #53: Terminal Notification System
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/terminal-notification.ts`
- **Description**: Terminal bell/notification system for agent completion and important events.
- **Go Status**: Not present.

### Gap #54: Message Replay
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/message-replay.ts`
- **Description**: Replay message history with proper formatting.
- **Go Status**: Not present.

### Gap #55: Paging System
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/paging.ts`
- **Description**: Scrollable content paging for long outputs.
- **Go Status**: Not present.

### Gap #56: Render Cache
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/render-cache.ts`
- **Description**: Optimized rendering with caching to avoid re-rendering unchanged content.
- **Go Status**: Not present.

### Gap #57: Tmux Keyboard Handling
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/tmux-keyboard.ts`
- **Description**: Proper keyboard handling when running inside tmux (paste bracket mode, etc.).
- **Go Status**: Not present.

### Gap #58: Foreground Task Management
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `apps/kimi-code/src/tui/utils/foreground-task.ts`
- **Description**: Manage foreground task lifecycle in TUI (detach to background, etc.).
- **Go Status**: Not present.

---

## Server & Klient Gaps

### Gap #19: Auth System
- **Cycle**: 6 (Server & Klient)
- **Severity**: Critical
- **TS Source**: `packages/kap-server/src/services/auth/` — token store, credentials, password, persistent tokens, bearer middleware
- **Description**: Complete auth system: token store with persistent tokens, credential validation, password hashing, bearer token middleware for all routes.
- **Go Status**: Not present.

### Gap #20: WebSocket v1 Protocol
- **Cycle**: 6
- **Severity**: Critical
- **TS Source**: `packages/kap-server/src/transport/ws/v1/` — connection lifecycle, event broadcasting, session journal, in-flight turn tracking
- **Description**: Real WebSocket protocol: connection management with bearer auth, event broadcasting, session event journal, in-flight turn tracking, subagent roster tracking, fs-watch bridge.
- **Go Status**: Stub WS transport only.

### Gap #21: Klient SDK
- **Cycle**: 6
- **Severity**: Critical
- **TS Source**: `packages/klient/src/` (45 files)
- **Description**: Full client SDK: channel SPI, contract system (agent/global/session contracts), facades (agent/global/session), event hub, IPC/memory transports, validation.
- **Go Status**: 1-file harness stub.

### Gap #22: Most REST Routes Missing
- **Cycle**: 6
- **Severity**: Critical
- **TS Source**: `packages/kap-server/src/routes/` — 21 route files
- **Description**: Complete REST API: sessions, messages, approvals, auth, config, connections, files, fs, guiStore, meta, modelCatalog, oauth, prompts, questions, search, sessionExport, skills, snapshot, shutdown, tasks, terminals, tools, transcript, webAssets, workspaceFs, workspaces.
- **Go Status**: Only `messages` and `sessions` routes.

### Gap #59: Search Service
- **Cycle**: 6
- **Severity**: Important
- **TS Source**: `packages/kap-server/src/search/`
- **Description**: Code search with snippet extraction and wire-format extraction for search results.
- **Go Status**: Not present.

### Gap #60: Snapshot Service
- **Cycle**: 6
- **Severity**: Important
- **TS Source**: `packages/kap-server/src/services/snapshot/`
- **Description**: Point-in-time session snapshot for WebSocket reconnection — captures full session state.
- **Go Status**: Not present.

### Gap #61: Security Middleware
- **Cycle**: 6
- **Severity**: Important
- **TS Source**: `packages/kap-server/src/security/bindClassify.ts`
- **Description**: Host classification, origin/CORS validation, rate limiting for non-loopback deployments.
- **Go Status**: Not present.

---

## Infrastructure Gaps

### Gap #23: Typed Event Structs (~50)
- **Cycle**: 5 (Protocol & Events)
- **Severity**: Critical
- **TS Source**: `packages/protocol/src/events.ts`
- **Description**: ~50 strongly typed event interfaces with Zod schemas: turn.started, tool.call.delta, compaction.completed, etc. Each event has a typed payload.
- **Go Status**: 17 string constants + `json.RawMessage` payload.

### Gap #24: PromptOrigin Discriminated Union
- **Cycle**: 5
- **Severity**: Critical
- **TS Source**: `packages/protocol/src/events.ts`
- **Description**: 13 variants tagging each prompt with its origin: user, skill_activation, plugin_command, injection, shell_command, etc.
- **Go Status**: Not present.

### Gap #25: AgentPhase State Machine
- **Cycle**: 5
- **Severity**: Critical
- **TS Source**: `packages/protocol/src/events.ts`
- **Description**: 8 phases: idle, running, streaming, tool_call, retrying, awaiting_approval, interrupted, ended.
- **Go Status**: Not present.

### Gap #26: Approval/Question Wire Types
- **Cycle**: 5
- **Severity**: Critical
- **TS Source**: `packages/protocol/src/approval.ts`, `question.ts`
- **Description**: ApprovalRequest/Response with diff preview, QuestionRequest/Response with options, multi-select, other text.
- **Go Status**: Not present in protocol package.

### Gap #27: REST Endpoint Contract (21 Definitions)
- **Cycle**: 5
- **Severity**: Critical
- **TS Source**: `packages/protocol/src/rest/` — 21 files
- **Description**: Complete REST API contract definitions for all daemon endpoints.
- **Go Status**: Only 2 (`rest/message.go`, `rest/session.go`).

### Gap #63: SSHKaos — Remote Execution
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/kaos/src/ssh.ts` (946 lines)
- **Description**: Full remote execution environment via SSH/SFTP with file ops, process spawning, glob, stat.
- **Go Status**: Only LocalKaos exists.

### Gap #64: Login Shell PATH Enrichment
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/kaos/src/login-shell-path.ts`
- **Description**: Runs user's login shell to extract PATH entries missing from daemon environment.
- **Go Status**: Not present.

### Gap #65: Advanced Glob (Symlink Cycle Detection)
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/kaos/src/local.ts`
- **Description**: Robust recursive glob with symlink cycle detection via (dev,ino) tracking.
- **Go Status**: Uses basic `filepath.Glob` with no recursion or cycle detection.

### Gap #66: OAuth Managed Provider Provisioning
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/oauth/src/managed-kimi-code.ts` (859 lines)
- **Description**: Fetches models from managed platform, parses capabilities, builds provider config.
- **Go Status**: Not present.

### Gap #67: OAuth Custom API Registry
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/oauth/src/custom-registry.ts` (442 lines)
- **Description**: Fetches and parses custom api.json registries, creates provider entries.
- **Go Status**: Not present.

### Gap #68: OAuth Provider Model Refresh
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/oauth/src/refreshProviderModels.ts` (767 lines)
- **Description**: Coordinates refreshing all provider models from managed platform, open platforms, and custom registries.
- **Go Status**: Not present.

### Gap #69: OAuth Toolkit Facade
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/oauth/src/toolkit.ts` (476 lines)
- **Description**: High-level facade tying together identity, storage, manager, provisioning, usage, feedback.
- **Go Status**: Not present.

### Gap #70: Telemetry Client
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/telemetry/src/` (9 files)
- **Description**: Event tracking with buffering, context scoping, HTTP transport, flush lifecycle, crash handlers, system metrics.
- **Go Status**: Stub (`doc.go` only).

### Gap #71: KimiErrorCode String Enum (~75 Codes)
- **Cycle**: 5
- **Severity**: Important
- **TS Source**: `packages/protocol/src/events.ts`
- **Description**: Typed semantic error codes (e.g., `session.fork_active_turn`, `provider.rate_limit`).
- **Go Status**: Integer error codes only.

### Gap #72: KimiErrorPayload
- **Cycle**: 5
- **Severity**: Important
- **TS Source**: `packages/protocol/src/events.ts`
- **Description**: Structured wire error with retryable flag and causal chain.
- **Go Status**: `APIError` with code+message only.

### Gap #73: ToolInputDisplay/ToolResultDisplay (12 Variants Each)
- **Cycle**: 5
- **Severity**: Important
- **TS Source**: `packages/protocol/src/display.ts`
- **Description**: Discriminated unions for rich TUI rendering of tool calls/results.
- **Go Status**: Not present.

### Gap #74: FS Browsing/Search/Git-Status Types
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/protocol/src/fs.ts`
- **Description**: FsEntry, FsSearchHit, FsGrepFileHit, FsGitStatusEntry, FsChangeEvent wire types.
- **Go Status**: Not present.

### Gap #75: Workspace Wire Types
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/protocol/src/workspace.ts`
- **Description**: Workspace, WorkspaceCreate, WorkspaceUpdate, workspace_id regex.
- **Go Status**: Not present.

### Gap #76: Model Catalog Wire Types
- **Cycle**: 8
- **Severity**: Important
- **TS Source**: `packages/protocol/src/modelCatalog.ts`
- **Description**: ModelCatalogItem, ProviderCatalogItem, ProviderRefreshChange/Failure.
- **Go Status**: Not present.

---

## Additional Agent Core Tool Gaps

### Gap #31: Plan Mode Tools
- **Cycle**: 4
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/tools/builtin/planning/`
- **Description**: EnterPlanMode and ExitPlanMode tools for plan-mode workflow.
- **Go Status**: Not present.

### Gap #32: AskUser Tool
- **Cycle**: 4
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/tools/builtin/collaboration/ask-user.ts`
- **Description**: Structured user questions with options, multi-select.
- **Go Status**: Not present.

### Gap #38: select_tools (Progressive Disclosure)
- **Cycle**: 7
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/tools/builtin/select-tools.ts`
- **Description**: Progressive tool loading — model can request additional tools.
- **Go Status**: Not present.

### Gap #39: Dynamic Tools Support
- **Cycle**: 7
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/agent/context/dynamic-tools.ts`
- **Description**: Progressive tool loading infrastructure — tools can be added mid-session.
- **Go Status**: Not present.

### Gap #40: Tool Result Budgeting
- **Cycle**: 4
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/agent/`
- **Description**: Truncates oversized tool results to fit within context budget.
- **Go Status**: Not present.

### Gap #41: ReadMedia Tool
- **Cycle**: 4
- **Severity**: Important
- **TS Source**: `packages/agent-core/src/tools/builtin/file/read-media.ts`
- **Description**: Read image/video/audio files as content parts.
- **Go Status**: Not present.

---

## Nice-to-Have Gaps (Cycle 8)

### Gap #77: Notification XML
- **Severity**: Nice-to-have
- **TS Source**: `packages/agent-core/src/agent/context/notification-xml.ts`
- **Description**: Background task completion notifications injected as XML.

### Gap #78: LLM Request Logging/Recording
- **Severity**: Nice-to-have
- **TS Source**: `packages/agent-core/src/agent/llm-request-logger.ts`, `llm-request-recorder.ts`
- **Description**: Diagnostic logging of LLM requests for debugging.

### Gap #79: Session Hooks Engine
- **Severity**: Nice-to-have
- **TS Source**: `packages/agent-core/src/session/hooks/`
- **Description**: Pre/post-turn hook system for extensibility.

### Gap #80: Compaction Strategies (Full, Micro, Handoff)
- **Severity**: Nice-to-have
- **TS Source**: `packages/agent-core/src/agent/compaction/`
- **Description**: Multiple compaction strategies: full (summarize everything), micro (drop old tool results), handoff (summarize for sub-agent).

### Gap #81: System Reminder Injection
- **Severity**: Nice-to-have
- **TS Source**: `packages/agent-core/src/agent/injection/`
- **Description**: `<system-reminder>` wrapper with origin tracking for injected content.

### Gap #82: Tool Call Deduplication
- **Severity**: Nice-to-have
- **TS Source**: `packages/agent-core/src/agent/`
- **Description**: Prevents duplicate tool calls in the same step.

### Gap #85: ACP Adapter (IDE Integration)
- **Severity**: Nice-to-have
- **TS Source**: `packages/acp-adapter/src/` (19 files)
- **Description**: Agent Client Protocol adapter for IDE integrations (Cursor, VS Code, etc.).

### Gap #86: Text Decode Error Modes
- **Severity**: Nice-to-have
- **TS Source**: `packages/kaos/src/internal.ts`
- **Description**: strict/replace/ignore UTF-8 error handling for file reading.

### Gap #87: File Line-Ending Analysis
- **Severity**: Nice-to-have
- **TS Source**: `packages/kaos/src/local.ts`
- **Description**: CRLF/LF/NUL/binary detection for files.

### Gap #88: OAuth Token State Machine
- **Severity**: Nice-to-have
- **TS Source**: `packages/oauth/src/token-state.ts`
- **Description**: Explicit token lifecycle states (absent → pending → active → expired → revoked).

### Gap #89: OAuth Managed Usage/Feedback
- **Severity**: Nice-to-have
- **TS Source**: `packages/oauth/src/managed-usage.ts`, `managed-feedback.ts`
- **Description**: Usage data fetching and feedback submission to managed platform.

### Gap #90: ULID Request ID
- **Severity**: Nice-to-have
- **TS Source**: `packages/protocol/src/request-id.ts`
- **Description**: ULID-based request ID parsing/generation.

### Gap #91: AsyncAPI Generator
- **Severity**: Nice-to-have
- **TS Source**: `packages/protocol/src/asyncapi.ts`
- **Description**: Generates AsyncAPI 3.1.0 spec from WS operation definitions.

---

## Additional Protocol Gaps (Cycle 5)

### Gap #1.5: ToolExchangeAdjacencyError Detection
- **Severity**: Important
- **TS Source**: `packages/kosong/src/errors.ts`
- **Description**: Detects strict provider rejections for malformed tool_use/tool_result pairing.
- **Go Status**: **IMPLEMENTED** in Cycle 1 errors.go.

### Gap #1.6: IsRecoverableRequestStructureError
- **Severity**: Important
- **TS Source**: `packages/kosong/src/errors.ts`
- **Description**: Detects structural request rejections (empty text blocks, role alternation) for re-projection recovery.
- **Go Status**: **IMPLEMENTED** in Cycle 1 errors.go.

### Gap #1.7: Additional Protocol Event Types
- **Severity**: Important
- **TS Source**: `packages/protocol/src/events.ts`
- **Description**: GoalSnapshot/GoalBudget with budget reports, TaskInfo discriminated union, CompactionResult, ToolUpdate, VolatileEventType, Subagent events, Prompt lifecycle events, Shell events, HookResultEvent.
- **Go Status**: Not present.

### Gap #1.8: REST Protocol Types
- **Severity**: Important
- **TS Source**: `packages/protocol/src/rest/`
- **Description**: ConfigResponse, Session snapshot, Prompt submission types, OAuth REST endpoints.
- **Go Status**: Not present.

### Gap #1.9: Display Protocol Types
- **Severity**: Important
- **TS Source**: `packages/protocol/src/display.ts`
- **Description**: ToolInputDisplay (12 variants), ToolResultDisplay (12 variants).
- **Go Status**: Not present.

### Gap #1.10: Additional Wire Types
- **Severity**: Important
- **TS Source**: `packages/protocol/src/`
- **Description**: ToolDescriptor/McpServer types, Task wire type, SkillDescriptor, FileMeta, Workspace types, ModelCatalog types.
- **Go Status**: Not present.

---

## Transcript System Gap

### Gap #T1: Complete Transcript System
- **Cycle**: 5
- **Severity**: Critical
- **TS Source**: `packages/transcript/src/` (23 files)
- **Description**: Full event-sourced transcript recording with operations, frames, turns, interactions, attachments, todos, tasks, prompts, pagination, history grouping, view registry, contract schema, granularity filtering.
- **Go Status**: Stub (`doc.go` only).
