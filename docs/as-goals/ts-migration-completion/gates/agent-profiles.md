# Gate: Agent Profiles

## Condition
Agent profiles can be defined (as Markdown files or config), loaded at startup or session creation, and applied to sessions — affecting system prompt, tool selection, and behavioral configuration.

## Evidence Required
- [ ] Agent profile loader (Markdown/config format) → `internal/agentcore/agent/profile/`
- [ ] Profile application during session creation
- [ ] Profile affects system prompt content
- [ ] CLI flag `--agent <name>` and `--agent-file <path>` wired
- [ ] Unit tests for profile loading and application
- [ ] Build/test/lint passes

## Verification Method
1. Define a profile file, start session with `--agent-file`, verify system prompt includes profile content
2. Use `--agent <name>` with a named profile, verify it loads
3. Run existing test suite — no regressions

## Owner
Engineer
