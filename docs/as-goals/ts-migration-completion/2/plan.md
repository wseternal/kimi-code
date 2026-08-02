# Plan — Iteration 2

## Focus
Wire remaining server route stubs (compact, undo, messages, OAuth login, transcript) using callback-based architecture.

## Tasks

### Task 1: Add callback types and server options
- **Files:** `internal/kapserver/server.go`
- **What:** Add CompactFunc, UndoFunc, MessageListFunc, OAuthLoginFunc, TranscriptListFunc callback types and With* options
- **Acceptance:** All callback types defined, ServerOption functions created

### Task 2: Wire route handlers to use callbacks
- **Files:** `internal/kapserver/routes.go`, `internal/kapserver/server.go`
- **What:** Update handleCompactSession, handleUndoSession, handleListMessages, handleOAuthLogin, handleListTranscript to invoke callbacks when configured
- **Acceptance:** All 5 routes invoke callbacks when wired, gracefully return 501 when not
