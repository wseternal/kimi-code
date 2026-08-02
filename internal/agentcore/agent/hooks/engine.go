package hooks

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
)

// Engine manages hook definitions and executes them at lifecycle events.
type Engine struct {
	mu     sync.RWMutex
	hooks  []HookDef
	logger *slog.Logger
}

// NewEngine creates a hook engine from the given hook definitions.
func NewEngine(defs []HookDef, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{hooks: defs, logger: logger}
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
			e.logger.Error("hook failed", "event", event, "command", def.Command, "error", result.Err)
		} else if result.Blocked {
			e.logger.Info("hook blocked", "event", event, "command", def.Command, "reason", result.Reason)
		} else {
			e.logger.Debug("hook executed", "event", event, "command", def.Command, "exit_code", result.ExitCode, "duration_ms", result.Duration.Milliseconds())
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
			e.logger.Info("hook blocked", "event", event, "command", def.Command, "reason", result.Reason)
			return BlockDecision{
				Blocked: true,
				Reason:  result.Reason,
				Results: results,
			}
		}
		if result.Err != nil {
			e.logger.Error("hook failed", "event", event, "command", def.Command, "error", result.Err)
		} else {
			e.logger.Debug("hook executed", "event", event, "command", def.Command, "exit_code", result.ExitCode, "duration_ms", result.Duration.Milliseconds())
		}
	}
	return BlockDecision{Results: results}
}

// FireAndForget triggers hooks in a background goroutine.
// Useful for PostToolUse, PostCompact, and other non-blocking events.
// Pointer fields in HookInput are defensively copied to prevent
// concurrent mutation by the caller.
func (e *Engine) FireAndForget(ctx context.Context, event EventType, input HookInput) {
	matched := e.matchHooks(event, input)
	if len(matched) == 0 {
		return
	}
	// Defensive copy of pointer fields to isolate from caller mutations.
	if input.Tool != nil {
		tool := *input.Tool
		input.Tool = &tool
	}
	if input.Session != nil {
		sess := *input.Session
		input.Session = &sess
	}
	go func() {
		for _, def := range matched {
			input.Matcher = def.Matcher
			result := RunHook(ctx, def, input)
			if result.Err != nil {
				e.logger.Error("hook failed (async)", "event", event, "command", def.Command, "error", result.Err)
			} else {
				e.logger.Debug("hook executed (async)", "event", event, "command", def.Command, "exit_code", result.ExitCode, "duration_ms", result.Duration.Milliseconds())
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
