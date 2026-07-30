package providers

import (
	"testing"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

func TestGetOpenAILegacyModelCapability(t *testing.T) {
	tests := []struct {
		model string
		want  kosong.ModelCapability
	}{
		{"o1", openAIReasoningCapability},
		{"o1-mini", openAIReasoningCapability},
		{"o3", openAIReasoningCapability},
		{"o4-mini", openAIReasoningCapability},
		{"gpt-4o", openAIVisionToolCapability},
		{"gpt-4o-mini", openAIVisionToolCapability},
		{"gpt-4-turbo", openAIVisionToolCapability},
		{"gpt-4.1", openAIVisionToolCapability},
		{"gpt-4.5-preview", openAIVisionToolCapability},
		{"gpt-3.5-turbo", openAITextToolCapability},
		{"gpt-3.5-turbo-16k", openAITextToolCapability},
		{"unknown-model", kosong.UnknownCapability},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := GetOpenAILegacyModelCapability(tt.model)
			if got != tt.want {
				t.Errorf("GetOpenAILegacyModelCapability(%q) = %+v, want %+v", tt.model, got, tt.want)
			}
		})
	}
}

func TestGetOpenAIResponsesModelCapability(t *testing.T) {
	tests := []struct {
		model string
		want  kosong.ModelCapability
	}{
		{"o1", openAIReasoningCapability},
		{"o3-mini", openAIReasoningCapability},
		{"gpt-4o", openAIVisionToolCapability},
		{"gpt-4.1-mini", openAIVisionToolCapability},
		// Responses catalog has no text-only entry
		{"gpt-3.5-turbo", kosong.UnknownCapability},
		{"unknown", kosong.UnknownCapability},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := GetOpenAIResponsesModelCapability(tt.model)
			if got != tt.want {
				t.Errorf("GetOpenAIResponsesModelCapability(%q) = %+v, want %+v", tt.model, got, tt.want)
			}
		})
	}
}

func TestGetAnthropicModelCapability(t *testing.T) {
	tests := []struct {
		model string
		want  kosong.ModelCapability
	}{
		{"claude-3-opus-20240229", anthropicVisionToolCapability},
		{"claude-3.5-sonnet-20241022", anthropicVisionToolCapability},
		{"claude-3.7-sonnet", anthropicVisionToolCapability},
		{"claude-opus-4-20250514", anthropicThinkingVisionToolCapability},
		{"claude-sonnet-4-20250514", anthropicThinkingVisionToolCapability},
		{"claude-haiku-4", anthropicThinkingVisionToolCapability},
		{"claude-fable", anthropicThinkingVisionToolCapability},
		{"claude-2.1", kosong.UnknownCapability},
		{"unknown", kosong.UnknownCapability},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := GetAnthropicModelCapability(tt.model)
			if got != tt.want {
				t.Errorf("GetAnthropicModelCapability(%q) = %+v, want %+v", tt.model, got, tt.want)
			}
		})
	}
}

func TestGetGoogleGenAIModelCapability(t *testing.T) {
	tests := []struct {
		model string
		want  kosong.ModelCapability
	}{
		{"gemini-1.5-pro", geminiMultimodalToolCapability},
		{"gemini-1.5-flash", geminiMultimodalToolCapability},
		{"gemini-2.0-flash", geminiMultimodalToolCapability},
		{"gemini-2.0-pro", geminiMultimodalToolCapability},
		{"gemini-2.5-pro", geminiThinkingMultimodalToolCapability},
		{"gemini-2.5-flash", geminiThinkingMultimodalToolCapability},
		{"gemini-2.0-flash-thinking", geminiThinkingMultimodalToolCapability},
		// Uncatalogued gemini prefix
		{"gemini-3.0-pro", kosong.UnknownCapability},
		// Non-gemini
		{"palm-2", kosong.UnknownCapability},
		{"unknown", kosong.UnknownCapability},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := GetGoogleGenAIModelCapability(tt.model)
			if got != tt.want {
				t.Errorf("GetGoogleGenAIModelCapability(%q) = %+v, want %+v", tt.model, got, tt.want)
			}
		})
	}
}

func TestUsesOpenAIResponsesDeveloperRole(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gpt-4.1", true},
		{"gpt-4.1-mini", true},
		{"gpt-4.1-nano-2025-04-14", true},
		{"o1", true},
		{"o1-mini", true},
		{"o3", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-5-codex", true},
		{"gpt-4o", false},
		{"gpt-3.5-turbo", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := UsesOpenAIResponsesDeveloperRole(tt.model)
			if got != tt.want {
				t.Errorf("UsesOpenAIResponsesDeveloperRole(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestCaseInsensitiveModelLookup(t *testing.T) {
	// All public functions should normalize to lowercase.
	if got := GetOpenAILegacyModelCapability("GPT-4O"); got != openAIVisionToolCapability {
		t.Errorf("expected vision capability for GPT-4O, got %+v", got)
	}
	if got := GetAnthropicModelCapability("Claude-Opus-4-20250514"); got != anthropicThinkingVisionToolCapability {
		t.Errorf("expected thinking capability for Claude-Opus-4, got %+v", got)
	}
	if got := GetGoogleGenAIModelCapability("Gemini-2.5-Pro"); got != geminiThinkingMultimodalToolCapability {
		t.Errorf("expected thinking multimodal for Gemini-2.5-Pro, got %+v", got)
	}
	if got := UsesOpenAIResponsesDeveloperRole("O3-Mini"); !got {
		t.Error("expected true for O3-Mini developer role")
	}
}
