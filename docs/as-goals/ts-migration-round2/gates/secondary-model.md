# Gate: Secondary Model Config

## Condition
The config supports specifying a secondary/fallback model. The `secondary_model` slash command allows switching models mid-session. Sub-agents can use the secondary model.

## Evidence Required
- [ ] Config field for secondary model → `internal/agentcore/config/config.go`
- [ ] `/secondary_model` slash command → `internal/cli/tui.go` or `internal/cli/commands.go`
- [ ] Sub-agent model override → `internal/agentcore/agent/swarm/` or tool

## Verification Method
Test engineer verifies config field exists, slash command is functional, and model override works.

## Owner
Engineer
