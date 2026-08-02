# Plan — Iteration 2

## Focus
Close the Server Routes Wired gate by adding all missing TS route handlers.

## Tasks

### Task 1: Add approval & question routes
- **Files**: `internal/kapserver/server.go`, `internal/kapserver/routes.go`
- **Acceptance**: GET/POST approvals and questions endpoints registered and responding

### Task 2: Add session sub-resource routes
- **Files**: `internal/kapserver/server.go`, `internal/kapserver/routes.go`
- **Acceptance**: tasks, tools, terminals, skills, transcript, fs endpoints registered

### Task 3: Add global routes
- **Files**: `internal/kapserver/server.go`, `internal/kapserver/routes.go`
- **Acceptance**: model-catalog, oauth, connections, workspaces endpoints registered

### Task 4: Clean up stub doc.go packages
- **Files**: middleware/doc.go, routes/doc.go, transport/doc.go
- **Acceptance**: No "stub" language remaining

### Task 5: Fix redundant lookup
- **Files**: `internal/kapserver/routes.go`
- **Acceptance**: handleCompactSession calls sessionManager.Get only once
