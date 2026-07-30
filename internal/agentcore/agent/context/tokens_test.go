package context

import (
	"testing"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

func TestTokenEstimateCJK(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		// Pure ASCII: ceil(ascii/4)
		{"hello", 2},          // 5 ascii → (5+3)/4 = 2
		{"hello world", 3},    // 11 ascii → (11+3)/4 = 3
		{"abcdefgh", 2},       // 8 ascii → (8+3)/4 = 2
		{"abcdefghijkl", 3},   // 12 ascii → (12+3)/4 = 3
		// Pure CJK: ~1 char/token each (3 bytes each in UTF-8)
		{"你好", 2},           // 2 non-ASCII → 2
		{"你好世界", 4},       // 4 non-ASCII → 4
		// Mixed ASCII + CJK
		{"hello世界", 4},      // 5 ascii + 2 non-ASCII → (5+3)/4 + 2 = 4
		{"hi你好", 3},         // 2 ascii + 2 non-ASCII → (2+3)/4 + 2 = 3
		// Code-like content
		{"func main() {}", 4}, // 16 ascii → (16+3)/4 = 4
	}
	for _, tt := range tests {
		got := TokenEstimate(tt.input)
		if got != tt.want {
			t.Errorf("TokenEstimate(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestEstimateContentPart(t *testing.T) {
	// Text part
	textPart := kosong.NewTextPart("hello world")
	if got := EstimateContentPart(textPart); got != 3 {
		t.Errorf("text part: got %d, want 3", got)
	}

	// Think part
	thinkPart := kosong.NewThinkPart("let me think about this")
	if got := EstimateContentPart(thinkPart); got != 6 {
		t.Errorf("think part: got %d, want 6", got)
	}

	// Image part
	imgPart := kosong.NewImageURLPart("data:image/png;base64,abc123", nil)
	if got := EstimateContentPart(imgPart); got != MediaTokenEstimate {
		t.Errorf("image part: got %d, want %d", got, MediaTokenEstimate)
	}

	// Unknown type
	unknown := kosong.ContentPart{Type: "unknown_type"}
	if got := EstimateContentPart(unknown); got != 0 {
		t.Errorf("unknown part: got %d, want 0", got)
	}
}

func TestEstimateMessage(t *testing.T) {
	msg := kosong.CreateUserMessage("hello world")
	got := EstimateMessage(&msg)
	// "user" = 1 token + "hello world" = 3 tokens = 4
	if got != 4 {
		t.Errorf("EstimateMessage(user 'hello world') = %d, want 4", got)
	}
}

func TestEstimateMessages(t *testing.T) {
	msgs := []kosong.Message{
		kosong.CreateUserMessage("hello"),
		kosong.CreateAssistantMessage([]kosong.ContentPart{kosong.NewTextPart("hi there")}, nil),
	}
	got := EstimateMessages(msgs)
	// user=1 + hello=2 = 3, assistant=3 + "hi there"=2 = 5 → total=8
	if got != 8 {
		t.Errorf("EstimateMessages = %d, want 8", got)
	}
}

func TestEstimateTools(t *testing.T) {
	tools := []kosong.Tool{
		{
			Name:        "read_file",
			Description: "Read the contents of a file",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "File path",
					},
				},
			},
		},
	}
	got := EstimateTools(tools)
	if got <= 0 {
		t.Errorf("EstimateTools = %d, want > 0", got)
	}
	// read_file(~2) + description(~7) + JSON params(~20+) should be > 20
	if got < 20 {
		t.Errorf("EstimateTools = %d, expected at least 20 for a tool with params", got)
	}
}

func TestMediaTokenEstimate(t *testing.T) {
	if MediaTokenEstimate != 2000 {
		t.Errorf("MediaTokenEstimate = %d, want 2000", MediaTokenEstimate)
	}
}
