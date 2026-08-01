# Gate: AGENTS.md Prompt Injection

## Condition
AGENTS.md files found in project root directories are read and their content is injected into the agent's system prompt, following the same conventions as the TS implementation.

## Evidence Required
- [ ] AGENTS.md file discovery logic → `internal/agentcore/agent/prompt/` or `internal/cli/`
- [ ] System prompt builder includes AGENTS.md content → system prompt output
- [ ] Tests for AGENTS.md loading → existing `_test.go` files

## Verification Method
Test engineer verifies AGENTS.md content appears in the constructed system prompt when the file exists in the project root.

## Owner
Engineer
