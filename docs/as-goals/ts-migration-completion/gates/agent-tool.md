# Gate: Agent Tool

## Condition
An individual Agent tool (distinct from AgentSwarm) is implemented and registered, allowing the LLM to spawn and manage individual sub-agents with custom configuration.

## Evidence Required
- [ ] Agent tool implementation → `internal/agentcore/agent/tools/agent.go`
- [ ] Tool supports: custom prompt, model, work_dir, tools, timeout parameters
- [ ] Sub-agent lifecycle management (start, monitor, get result)
- [ ] Registered in agent tool setup
- [ ] Tests → `_test.go` files
- [ ] Build and tests pass

## Verification Method
1. Run `task go:build` — exits 0
2. Run `task go:test` — exits 0
3. Verify tool struct implements the Tool interface
4. Verify registration in tool setup code

## Owner
Engineer
