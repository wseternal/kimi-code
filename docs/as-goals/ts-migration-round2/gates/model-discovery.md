# Gate: Model Discovery

## Condition
Dynamic model discovery lists available models from configured providers. The model catalog can be refreshed and supports filtering.

## Evidence Required
- [ ] Model catalog type → `internal/protocol/model_catalog.go` or equivalent
- [ ] Provider model listing → `internal/kosong/` or provider implementations
- [ ] Slash command or tool to list models → `internal/cli/`

## Verification Method
Test engineer verifies model listing returns provider models and catalog can be refreshed.

## Owner
Engineer
