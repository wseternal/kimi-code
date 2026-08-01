package hooks

import (
	"context"
	"log"
	"regexp"
	"sync"
)

// Engine manages hook definitions and executes them at lifecycle events.
type Engine struct {
	mu    sync.RWMutex
	hooks []HookDef
}

// NewEngine creates a hook engine from the given hook definitions.
func NewEngine(defs []HookDef) *Engine {
	return &Engine{hooks: defs}
}

// Trigger fires all matching hooks for the given event and returns all results.
// This is non-blocking from the caller's perspective — hooks run synchronously
// but errors are logged rather than propagated.
func (e *Engine) Trigger(ctx context.Context, event EventType, input HookInput) []HookResult {
	matched := e.matchHooks(event, input)
	if len(matched) == 0 {
		return nil
	}

	var results []HookResult
	for _, def := range matched {
		input.Matcher = def.Matcher
		result := RunHook(ctx, def, input)
		if result.Err != nil {
			log.Printf("hooks: %s hook %q failed: %v", event, def.Command, result.Err)
		}
		results = append(results, result)
	}
	return results
}

// TriggerBlock fires all matching hooks for a blocking event (e.g. PreToolUse).
// If any hook returns blocked=true, the block decision is returned immediately.
func (e *Engine) TriggerBlock(ctx context.Context, event EventType, input HookInput) BlockDecision {
	matched := e.matchHooks(event, input)
	if len(matched) == 0 {
		return BlockDecision{}
	}

	var results []HookResult
	for _, def := range matched {
		input.Matcher = def.Matcher
		result := RunHook(ctx, def, input)
		results = append(results, result)

		if result.Blocked {
			return BlockDecision{
				Blocked: true,
				Reason:  result.Reason,
				Results: results,
			}
		}
		if result.Err != nil {
			log.Printf("hooks: %s hook %q failed: %v", event, def.Command, result.Err)
		}
	}
	return BlockDecision{Results: results}
}

// FireAndForget triggers hooks in a background goroutine.
// Useful for PostToolUse, PostCompact, and other non-blocking events.
func (e *Engine) FireAndForget(ctx context.Context, event EventType, input HookInput) {
	matched := e.matchHooks(event, input)
	if len(matched) == 0 {
		return
	}
	go func() {
		for _, def := range matched {
			input.Matcher = def.Matcher
			result := RunHook(ctx, def, input)
			if result.Err != nil {
				log.Printf("hooks: %s hook %q failed: %v", event, def.Command, result.Err)
			}
		}
	}()
}

// Hooks returns a copy of all registered hook definitions.
func (e *Engine) Hooks() []HookDef {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]HookDef, len(e.hooks))
	copy(out, e.hooks)
	return out
}

// matchHooks returns hooks whose event type matches and whose matcher
// (if set) matches the input's matcher value (tool name, subject, etc.).
func (e *Engine) matchHooks(event EventType, input HookInput) []HookDef {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matched []HookDef
	for _, h := range e.hooks {
		if h.Event != event {
			continue
		}
		if h.Matcher != "" && !matchPattern(h.Matcher, input) {
			continue
		}
		matched = append(matched, h)
	}
	return matched
}

// matchPattern checks if the hook matcher matches the input.
// Supports exact match on tool name or regex match.
func matchPattern(pattern string, input HookInput) bool {
	// Try tool name match first (most common for PreToolUse/PostToolUse)
	if input.Tool != nil {
		if input.Tool.Name == pattern {
			return true
		}
	}

	// Try regex match on the matcher field
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Fall back to exact match on the input matcher value
		return input.Matcher == pattern
	}
	if input.Tool != nil && re.MatchString(input.Tool.Name) {
		return true
	}
	return re.MatchString(input.Matcher)
}
