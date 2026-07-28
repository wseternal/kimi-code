package oauth

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ReadDeviceID reads the persistent device ID from the config directory.
func ReadDeviceID(homeDir string) string {
	path := filepath.Join(homeDir, "device_id")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// CreateDeviceID generates or reads a persistent device ID.
func CreateDeviceID(homeDir string) string {
	if id := ReadDeviceID(homeDir); id != "" {
		return id
	}

	// Generate new UUID
	id := generateUUID()

	// Persist best-effort
	if err := os.MkdirAll(homeDir, 0o700); err == nil {
		path := filepath.Join(homeDir, "device_id")
		os.WriteFile(path, []byte(id), 0o600)
	}

	return id
}

// CreateDeviceHeaders creates the X-Msh-* device identity headers.
func CreateDeviceHeaders(version, homeDir string) DeviceHeaders {
	hostname, _ := os.Hostname()
	deviceID := CreateDeviceID(homeDir)

	return DeviceHeaders{
		"X-Msh-Platform":    KimiCodePlatform,
		"X-Msh-Version":     sanitizeASCII(version, "unknown"),
		"X-Msh-Device-Name": sanitizeASCII(hostname, "unknown"),
		"X-Msh-Device-Model": sanitizeASCII(deviceModel(), "unknown"),
		"X-Msh-Os-Version":  sanitizeASCII(runtime.GOOS+" "+runtime.GOARCH, "unknown"),
		"X-Msh-Device-Id":   deviceID,
	}
}

// deviceModel returns a human-readable device model string.
func deviceModel() string {
	switch runtime.GOOS {
	case "darwin":
		// Try to get macOS product version
		out, err := exec.Command("/usr/bin/sw_vers", "-productVersion").Output()
		if err == nil {
			ver := strings.TrimSpace(string(out))
			if ver != "" {
				return fmt.Sprintf("macOS %s %s", ver, runtime.GOARCH)
			}
		}
		return fmt.Sprintf("macOS %s", runtime.GOARCH)
	case "windows":
		return fmt.Sprintf("Windows %s", runtime.GOARCH)
	case "linux":
		return fmt.Sprintf("Linux %s", runtime.GOARCH)
	default:
		return fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH)
	}
}

// sanitizeASCII removes non-ASCII characters and returns a fallback if empty.
func sanitizeASCII(value, fallback string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 0x20 && r <= 0x7E {
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())
	if result == "" {
		return fallback
	}
	return result
}

// generateUUID generates a UUID v4 string.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		// crypto/rand failure is catastrophic; fall back to timestamp-based ID
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	// Set version 4
	uuid[6] = (uuid[6] & 0x0F) | 0x40
	// Set variant bits
	uuid[8] = (uuid[8] & 0x3F) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

// OpenURL opens a URL in the default browser (best-effort).
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// Use rundll32 to avoid cmd.exe shell metacharacter interpretation
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Wait in a goroutine to reap the child process and avoid zombies.
	go func() { _ = cmd.Wait() }()
	return nil
}
