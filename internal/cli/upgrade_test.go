package cli

import (
	"testing"
)

func TestParseSemver_Basic(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.1.0", [3]int{0, 1, 0}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"0.0.0", [3]int{0, 0, 0}},
	}
	for _, tt := range tests {
		got := parseSemver(tt.input)
		if got != tt.want {
			t.Errorf("parseSemver(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseSemver_WithPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"v1.2.3", [3]int{1, 2, 3}},
		{"v0.1.0", [3]int{0, 1, 0}},
	}
	for _, tt := range tests {
		got := parseSemver(tt.input)
		if got != tt.want {
			t.Errorf("parseSemver(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseSemver_WithPreRelease(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3-beta.1", [3]int{1, 2, 3}},
		{"0.1.0-rc.1", [3]int{0, 1, 0}},
		{"2.0.0+build.123", [3]int{2, 0, 0}},
		{"1.0.0-alpha+001", [3]int{1, 0, 0}},
	}
	for _, tt := range tests {
		got := parseSemver(tt.input)
		if got != tt.want {
			t.Errorf("parseSemver(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseSemver_Partial(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2", [3]int{1, 2, 0}},
		{"1", [3]int{1, 0, 0}},
		{"", [3]int{0, 0, 0}},
	}
	for _, tt := range tests {
		got := parseSemver(tt.input)
		if got != tt.want {
			t.Errorf("parseSemver(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseSemver_Invalid(t *testing.T) {
	// Invalid strings should not panic; they return zeroed values.
	tests := []struct {
		input string
		want  [3]int
	}{
		{"abc", [3]int{0, 0, 0}},
		{"abc.def.ghi", [3]int{0, 0, 0}},
		{"v", [3]int{0, 0, 0}},
	}
	for _, tt := range tests {
		got := parseSemver(tt.input)
		if got != tt.want {
			t.Errorf("parseSemver(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "0.1.0", false},
		{"0.2.0", "0.1.0", false},
		{"1.0.0", "2.0.0", true},
		{"1.0.0", "1.0.1", true},
		{"1.0.1", "1.0.0", false},
		{"0.1.0", "1.0.0", true},
		{"v0.1.0", "v0.2.0", true},
		{"0.3.0", "0.3.0", false},
	}
	for _, tt := range tests {
		got := isNewerVersion(tt.current, tt.latest)
		if got != tt.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}
