package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
)

// maxAgentsMdBytes is the soft budget for AGENTS.md content.
// Content exceeding this triggers a warning.
const maxAgentsMdBytes = 32 * 1024 // 32 KB

// agentsMdFiles lists the file names to search for, in priority order.
var agentsMdFiles = []string{
	"AGENTS.md",
	"agents.md",
}

// LoadAgentsMd discovers and loads AGENTS.md content from standard locations.
// Discovery order:
//  1. Brand-level: ~/.gkimi-code/AGENTS.md
//  2. Generic user-level: ~/.agents/AGENTS.md or ~/.agents/agents.md
//  3. Project-level: project root (found via .git) walking to workDir
//
// Each loaded file is annotated with <!-- From: {path} -->.
// Returns the combined content, a warning string (if over budget), and any error.
func LoadAgentsMd(workDir, homeDir string) (content string, warning string) {
	var parts []string
	totalSize := 0

	// 1. Brand-level: ~/.gkimi-code/AGENTS.md
	if homeDir != "" {
		brandPath := filepath.Join(homeDir, config.DataDirName, "AGENTS.md")
		if data, err := os.ReadFile(brandPath); err == nil {
			text := string(data)
			parts = append(parts, annotateSource(brandPath, text))
			totalSize += len(data)
		}
	}

	// 2. Generic user-level: ~/.agents/AGENTS.md or ~/.agents/agents.md
	if homeDir != "" {
		agentsDir := filepath.Join(homeDir, ".agents")
		for _, name := range agentsMdFiles {
			p := filepath.Join(agentsDir, name)
			if data, err := os.ReadFile(p); err == nil {
				text := string(data)
				parts = append(parts, annotateSource(p, text))
				totalSize += len(data)
				break // only take first match
			}
		}
	}

	// 3. Project-level: walk from project root to workDir
	projectRoot := findProjectRoot(workDir)
	if projectRoot != "" {
		// Collect directories from project root to workDir
		dirs := collectDirs(projectRoot, workDir)
		for _, dir := range dirs {
			for _, name := range agentsMdFiles {
				p := filepath.Join(dir, name)
				if data, err := os.ReadFile(p); err == nil {
					text := string(data)
					parts = append(parts, annotateSource(p, text))
					totalSize += len(data)
					break // only take first match per directory
				}
			}
			// Also check .kimi-code/AGENTS.md in each directory
			kimiPath := filepath.Join(dir, ".kimi-code", "AGENTS.md")
			if data, err := os.ReadFile(kimiPath); err == nil {
				text := string(data)
				parts = append(parts, annotateSource(kimiPath, text))
				totalSize += len(data)
			}
		}
	}

	if totalSize > maxAgentsMdBytes {
		warning = fmt.Sprintf("AGENTS.md content is %d bytes, exceeding the %d KB soft budget. Consider trimming.", totalSize, maxAgentsMdBytes/1024)
	}

	return strings.Join(parts, "\n\n"), warning
}

// annotateSource prepends a source annotation comment to the content.
func annotateSource(path, content string) string {
	return fmt.Sprintf("<!-- From: %s -->\n%s", path, strings.TrimSpace(content))
}

// findProjectRoot walks up from workDir looking for a .git directory.
func findProjectRoot(workDir string) string {
	dir := workDir
	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return dir
		}
		// Also check for .git file (git worktrees)
		if _, err := os.Stat(gitPath); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// collectDirs returns directories from root to target (inclusive).
func collectDirs(root, target string) []string {
	var dirs []string
	// Walk from target up to root
	dir := target
	for {
		dirs = append(dirs, dir)
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Reverse to get root-first order
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}
