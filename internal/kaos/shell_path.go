// Package kaos provides login shell PATH enrichment (Gap #64).
package kaos

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ShellPathConfig holds configuration for login shell PATH enrichment.
type ShellPathConfig struct {
	Shell   string        `json:"shell"`
	Timeout time.Duration `json:"timeout"`
}

// DefaultShellPathConfig returns the default configuration.
func DefaultShellPathConfig() ShellPathConfig {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
		} else {
			shell = "/bin/sh"
		}
	}
	return ShellPathConfig{
		Shell:   shell,
		Timeout: 5 * time.Second,
	}
}

// LoginShellPath extracts PATH entries from the user's login shell that
// are not present in the current process PATH. This is needed because
// daemon processes may not inherit the full login shell PATH.
func LoginShellPath(ctx context.Context, cfg ShellPathConfig) ([]string, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch {
	case strings.HasSuffix(cfg.Shell, "fish"):
		cmd = exec.CommandContext(ctx, cfg.Shell, "-l", "-c", "echo $PATH")
	case strings.HasSuffix(cfg.Shell, "csh") || strings.HasSuffix(cfg.Shell, "tcsh"):
		cmd = exec.CommandContext(ctx, cfg.Shell, "-l", "-c", "echo $PATH")
	case runtime.GOOS == "windows":
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", "echo %PATH%")
	default:
		// bash, zsh, sh — use login shell
		cmd = exec.CommandContext(ctx, cfg.Shell, "-l", "-c", "echo $PATH")
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("login shell PATH extraction failed: %w", err)
	}

	shellPath := strings.TrimSpace(string(output))
	if shellPath == "" {
		return nil, nil
	}

	currentPath := os.Getenv("PATH")
	currentSet := make(map[string]bool)
	sep := string(os.PathListSeparator)
	for _, p := range strings.Split(currentPath, sep) {
		currentSet[p] = true
	}

	var missing []string
	for _, p := range strings.Split(shellPath, sep) {
		p = strings.TrimSpace(p)
		if p != "" && !currentSet[p] {
			missing = append(missing, p)
		}
	}
	return missing, nil
}

// EnrichPATH appends missing login shell PATH entries to the current PATH.
func EnrichPATH(ctx context.Context, cfg ShellPathConfig) (string, error) {
	missing, err := LoginShellPath(ctx, cfg)
	if err != nil {
		return os.Getenv("PATH"), err
	}
	if len(missing) == 0 {
		return os.Getenv("PATH"), nil
	}

	current := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	enriched := current + sep + strings.Join(missing, sep)
	return enriched, nil
}

// ParseShellPath parses a PATH string into individual entries.
func ParseShellPath(path string) []string {
	sep := string(os.PathListSeparator)
	parts := strings.Split(path, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ReadEnvironmentFromFile reads environment variables from a file (e.g., .env).
func ReadEnvironmentFromFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			// Strip surrounding quotes
			val = strings.Trim(val, `"'`)
			env[key] = val
		}
	}
	return env, scanner.Err()
}
