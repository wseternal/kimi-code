# Gate: Compaction

## Condition
Full conversation compaction exists — when the context window fills up, the system can summarize older conversation turns to free space, preserving key facts and decisions.

## Evidence Required
- [ ] Compaction logic → `internal/agentcore/agent/context/` or equivalent
- [ ] Integration with agent loop → triggers when context usage exceeds threshold
- [ ] Summarization produces condensed output preserving key information
- [ ] Tests for compaction behavior → existing `_test.go` files

## Verification Method
Test engineer verifies compaction triggers at threshold, produces a summary, and the agent can continue after compaction.

## Owner
Engineer
