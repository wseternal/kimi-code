package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	githubReleaseURL = "https://api.github.com/repos/visdomtech/kimi-code/releases/latest"
	upgradeTimeout   = 15 * time.Second
)

// githubRelease is the minimal fields from the GitHub releases API.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// runUpgrade checks for a newer version and reports upgrade instructions.
func (a *App) runUpgrade() error {
	current := BuildVersion()
	fmt.Printf("Current version: %s\n", current)
	fmt.Println("Checking for updates...")

	client := &http.Client{Timeout: upgradeTimeout}
	req, err := http.NewRequest("GET", githubReleaseURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("parse release: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	if latest == "" {
		return fmt.Errorf("could not determine latest version")
	}

	fmt.Printf("Latest version:  %s\n", latest)

	if !isNewerVersion(current, latest) {
		fmt.Println("You are already on the latest version.")
		return nil
	}

	fmt.Printf("\nA new version is available: %s → %s\n", current, latest)
	fmt.Printf("Release: %s\n", release.Name)
	fmt.Printf("URL: %s\n\n", release.HTMLURL)

	// Find matching asset for current OS/arch
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	assetName := fmt.Sprintf("gkimi-%s-%s", goos, goarch)
	var downloadURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, goos) && strings.Contains(asset.Name, goarch) {
			downloadURL = asset.BrowserDownloadURL
			assetName = asset.Name
			break
		}
	}

	if downloadURL != "" {
		fmt.Printf("Download: %s (%s)\n", assetName, downloadURL)
		fmt.Println("\nTo upgrade, download the binary above and replace your current gkimi binary.")
		fmt.Println("Or run:")
		fmt.Printf("  curl -L -o gkimi %s && chmod +x gkimi && sudo mv gkimi $(which gkimi)\n", downloadURL)
	} else {
		fmt.Println("No pre-built binary found for your platform.")
		fmt.Printf("Visit %s for manual download.\n", release.HTMLURL)
	}

	return nil
}

// isNewerVersion compares two semver strings (e.g. "0.1.0" vs "0.2.0").
// Returns true if latest > current.
func isNewerVersion(current, latest string) bool {
	curParts := parseSemver(current)
	latParts := parseSemver(latest)

	for i := 0; i < 3; i++ {
		if latParts[i] > curParts[i] {
			return true
		}
		if latParts[i] < curParts[i] {
			return false
		}
	}
	return false
}

// parseSemver parses a version string like "1.2.3" into [1, 2, 3].
func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 4)
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		// Strip pre-release suffix
		s := parts[i]
		if idx := strings.IndexAny(s, "-+"); idx >= 0 {
			s = s[:idx]
		}
		n := 0
		for _, c := range s {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		result[i] = n
	}
	return result
}
