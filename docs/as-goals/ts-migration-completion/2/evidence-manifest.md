# Evidence Manifest — Iteration 2

## Gate Status

| Gate | Status | Evidence | Owner |
|------|--------|----------|-------|
| CLI Option Parity | ✅ Pass | `internal/cli/root.go` — CLIOptions struct, flag parsing for --yolo/--auto/--model/--output-format/--add-dir/--plan | Engineer |
| Headless Mode Wired | ✅ Pass | `internal/cli/root.go` runHeadless() — full agent loop with provider, tools, permissions, skills, multi-step generate | Engineer |
| Server Routes Wired | ✅ Pass | `internal/kapserver/server.go` — 36 routes registered; `routes.go` — all handler implementations | Engineer |
| Build Test Lint Clean | ✅ Pass | `go build ./...` ✓, `go vet ./...` ✓, `go test -race ./...` all pass | Engineer |

## Routes Added in Iteration 2
- GET /api/v1/sessions/{id}/approvals
- POST /api/v1/sessions/{id}/approvals/{approval_id}
- GET /api/v1/sessions/{id}/questions
- POST /api/v1/sessions/{id}/questions/{question_id}
- GET /api/v1/sessions/{id}/tasks
- GET /api/v1/sessions/{id}/tools
- GET /api/v1/sessions/{id}/terminals
- GET /api/v1/sessions/{id}/skills
- GET /api/v1/sessions/{id}/transcript
- GET /api/v1/sessions/{id}/fs
- GET /api/v1/model-catalog
- GET /api/v1/oauth/status
- POST /api/v1/oauth/login
- GET /api/v1/connections
- GET /api/v1/workspaces
- POST /api/v1/workspaces

## Code Quality Findings
- Critical: 0
- Warning: 1 (WebSocket transport uses simplified implementation)
- Suggestion: 0

## Commits Reviewed
- `a7a0784`: feat: add 15 missing server route handlers for TS parity
