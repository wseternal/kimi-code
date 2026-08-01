package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// RunHook executes a single hook command with the given input payload.
// The input is serialized as JSON and passed to the command on stdin.
// Exit code 2 or stdout containing `"action": "block"` signals a block decision.
func RunHook(ctx context.Context, def HookDef, input HookInput) HookResult {
	timeout := def.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeoutDur := time.Duration(timeout) * time.Second

	hookCtx, cancel := context.WithTimeout(ctx, timeoutDur)
	defer cancel()

	stdin, err := json.Marshal(input)
	if err != nil {
		return HookResult{Err: fmt.Errorf("marshal hook input: %w", err)}
	}

	shell, flag := shellArgs()
	cmd := exec.CommandContext(hookCtx, shell, flag, def.Command)
	cmd.Stdin = bytes.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	result := HookResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if hookCtx.Err() == context.DeadlineExceeded {
		result.Err = fmt.Errorf("hook timed out after %ds", timeout)
		result.ExitCode = -1
		return result
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Err = runErr
			result.ExitCode = -1
		}
	}

	// Exit code 2 = block
	if result.ExitCode == 2 {
		result.Blocked = true
		result.Reason = strings.TrimSpace(result.Stderr)
		if result.Reason == "" {
			result.Reason = "hook blocked tool execution (exit code 2)"
		}
		return result
	}

	// Check stdout for JSON block action
	if strings.Contains(result.Stdout, `"action"`) && strings.Contains(result.Stdout, `"block"`) {
		var out struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &out); err == nil && out.Action == "block" {
			result.Blocked = true
			result.Reason = out.Reason
			if result.Reason == "" {
				result.Reason = "hook blocked via stdout action"
			}
		}
	}

	return result
}

// shellArgs returns the shell and flag for executing a command string.
func shellArgs() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/c"
	}
	return "sh", "-c"
}
