package cli

import (
	"strings"
	"testing"
)

func TestBuildVersion_ContainsVersionAndSHA(t *testing.T) {
	// Save and restore globals so tests don't interfere.
	origVersion := Version
	origSHA := GitSHA
	t.Cleanup(func() {
		Version = origVersion
		GitSHA = origSHA
	})

	Version = "0.1.0"
	GitSHA = "abc1234"

	got := BuildVersion()
	if !strings.Contains(got, "0.1.0") {
		t.Errorf("BuildVersion() = %q, should contain version 0.1.0", got)
	}
	if !strings.Contains(got, "abc1234") {
		t.Errorf("BuildVersion() = %q, should contain git SHA abc1234", got)
	}
}

func TestBuildVersion_FallsBackWhenSHAMissing(t *testing.T) {
	origVersion := Version
	origSHA := GitSHA
	t.Cleanup(func() {
		Version = origVersion
		GitSHA = origSHA
	})

	Version = "0.1.0"
	GitSHA = ""

	got := BuildVersion()
	// Should still contain the version even when SHA is empty.
	if !strings.Contains(got, "0.1.0") {
		t.Errorf("BuildVersion() = %q, should contain version 0.1.0", got)
	}
	// Should not contain a dangling empty parenthesis.
	if strings.Contains(got, "()") {
		t.Errorf("BuildVersion() = %q, should not have empty parens when SHA is empty", got)
	}
}

func TestBuildVersion_DefaultValues(t *testing.T) {
	origVersion := Version
	origSHA := GitSHA
	t.Cleanup(func() {
		Version = origVersion
		GitSHA = origSHA
	})

	Version = "dev"
	GitSHA = "unknown"

	got := BuildVersion()
	if !strings.Contains(got, "dev") {
		t.Errorf("BuildVersion() = %q, should contain 'dev'", got)
	}
}
