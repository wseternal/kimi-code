package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/visdomtech/kimi-code/internal/agentcore/config"
)

// DoctorResult holds the result of a diagnostic check.
type DoctorResult struct {
	Name   string
	Status string // "ok", "warn", "error"
	Detail string
}

// RunDoctor performs system diagnostics.
func RunDoctor(cfg *config.Config) []DoctorResult {
	var results []DoctorResult

	// Check Go version
	results = append(results, checkGoVersion())

	// Check config file
	results = append(results, checkConfig(cfg))

	// Check API keys
	results = append(results, checkAPIKeys(cfg))

	// Check network
	results = append(results, checkNetwork())

	// Check disk space
	results = append(results, checkDiskSpace())

	// Check git
	results = append(results, checkGit())

	return results
}

func checkGoVersion() DoctorResult {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return DoctorResult{Name: "Go Version", Status: "warn", Detail: "go not found in PATH"}
	}
	return DoctorResult{Name: "Go Version", Status: "ok", Detail: strings.TrimSpace(string(out))}
}

func checkConfig(cfg *config.Config) DoctorResult {
	home, _ := os.UserHomeDir()
	path := config.ConfigPath(home)
	if _, err := os.Stat(path); err != nil {
		return DoctorResult{Name: "Config File", Status: "warn", Detail: fmt.Sprintf("not found at %s", path)}
	}
	return DoctorResult{Name: "Config File", Status: "ok", Detail: path}
}

func checkAPIKeys(cfg *config.Config) DoctorResult {
	var configured []string
	for name, prov := range cfg.Providers {
		if prov.APIKey != "" || prov.OAuth != nil {
			configured = append(configured, name)
		}
	}
	if len(configured) == 0 {
		return DoctorResult{Name: "API Keys", Status: "error", Detail: "no API keys configured. Run /login to set up."}
	}
	return DoctorResult{Name: "API Keys", Status: "ok", Detail: fmt.Sprintf("configured: %s", strings.Join(configured, ", "))}
}

func checkNetwork() DoctorResult {
	// Simple check: can we resolve a common hostname?
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "api.moonshot.cn")
	if err := cmd.Run(); err != nil {
		// Try alternative
		cmd2 := exec.Command("ping", "-n", "1", "-w", "2000", "api.moonshot.cn")
		if runtime.GOOS == "windows" {
			if err := cmd2.Run(); err != nil {
				return DoctorResult{Name: "Network", Status: "warn", Detail: "cannot reach api.moonshot.cn"}
			}
		} else {
			return DoctorResult{Name: "Network", Status: "warn", Detail: "cannot reach api.moonshot.cn"}
		}
	}
	return DoctorResult{Name: "Network", Status: "ok", Detail: "api.moonshot.cn reachable"}
}

func checkDiskSpace() DoctorResult {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, config.DataDirName)
	info, err := os.Stat(dataDir)
	if err != nil {
		return DoctorResult{Name: "Data Directory", Status: "ok", Detail: fmt.Sprintf("%s (will be created on first use)", dataDir)}
	}
	return DoctorResult{Name: "Data Directory", Status: "ok", Detail: fmt.Sprintf("%s (exists)", info.Name())}
}

func checkGit() DoctorResult {
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return DoctorResult{Name: "Git", Status: "warn", Detail: "git not found in PATH"}
	}
	return DoctorResult{Name: "Git", Status: "ok", Detail: strings.TrimSpace(string(out))}
}

// FormatDoctorResults formats doctor results for display.
func FormatDoctorResults(results []DoctorResult) string {
	var b strings.Builder
	b.WriteString("🩺 Doctor Report\n")
	b.WriteString(strings.Repeat("─", 40) + "\n")
	for _, r := range results {
		icon := "✓"
		switch r.Status {
		case "warn":
			icon = "⚠"
		case "error":
			icon = "✗"
		}
		b.WriteString(fmt.Sprintf("  %s %-15s %s\n", icon, r.Name, r.Detail))
	}
	return b.String()
}
