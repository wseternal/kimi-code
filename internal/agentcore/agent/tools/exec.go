package tools

import (
	"context"
	"os/exec"
)

// newCommand creates an exec.Cmd with context.
func newCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
