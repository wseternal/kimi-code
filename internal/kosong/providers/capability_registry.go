package providers

import (
	"regexp"
	"strings"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

// CapabilityMatcher returns true if the normalized (lowercased) model name
// matches a specific capability group.
type CapabilityMatcher func(normalizedModelName string) bool

// CapabilityCatalogEntry pairs a matcher with a capability descriptor.
type CapabilityCatalogEntry struct {
	Matches    CapabilityMatcher
	Capability kosong.ModelCapability
}

// ── Well-known capability sets ──

var (
	// OpenAI reasoning models (o1, o3, o4-mini, etc.) — thinking + tool use, no vision.
	openAIReasoningCapability = kosong.ModelCapability{
		Thinking: true, ToolUse: true,
	}

	// OpenAI vision models (gpt-4o, gpt-4-turbo, gpt-4.1, gpt-4.5) — image + tool use.
	openAIVisionToolCapability = kosong.ModelCapability{
		ImageIn: true, ToolUse: true,
	}

	// OpenAI text-only models (gpt-3.5-turbo) — tool use only.
	openAITextToolCapability = kosong.ModelCapability{
		ToolUse: true,
	}

	// Anthropic vision + tool use, no thinking (Claude 3, 3.5, 3.7).
	anthropicVisionToolCapability = kosong.ModelCapability{
		ImageIn: true, ToolUse: true,
	}

	// Anthropic thinking + vision + tool use (Opus 4, Sonnet 4, Haiku 4, Fable).
	anthropicThinkingVisionToolCapability = kosong.ModelCapability{
		ImageIn: true, Thinking: true, ToolUse: true,
	}

	// Gemini multimodal (image + video + audio) + tool use, no thinking.
	geminiMultimodalToolCapability = kosong.ModelCapability{
		ImageIn: true, VideoIn: true, AudioIn: true, ToolUse: true,
	}

	// Gemini multimodal + thinking (2.5+, models with "thinking" in name).
	geminiThinkingMultimodalToolCapability = kosong.ModelCapability{
		ImageIn: true, VideoIn: true, AudioIn: true, Thinking: true, ToolUse: true,
	}
)

// ── Prefix sets ──

var openAIVisionToolPrefixes = []string{
	"gpt-4o",
	"gpt-4-turbo",
	"gpt-4.1",
	"gpt-4.5",
}

var claudeVisionToolPrefixes = []string{
	"claude-3-",
	"claude-3.5-",
	"claude-3.7-",
}

var claudeThinkingVisionToolPrefixes = []string{
	"claude-opus-4",
	"claude-sonnet-4",
	"claude-haiku-4",
	"claude-fable",
}

var geminiCataloguedPrefixes = []string{
	"gemini-1.5-pro",
	"gemini-1.5-flash",
	"gemini-2.0-flash",
	"gemini-2.0-pro",
	"gemini-2.5-pro",
	"gemini-2.5-flash",
}

// openAIResponsesDeveloperRoleModels are models that use the "developer" role
// in the OpenAI Responses API instead of "system".
var openAIResponsesDeveloperRoleModels = map[string]bool{
	"gpt-4.1":      true,
	"gpt-4.1-mini": true,
	"gpt-4.1-nano": true,
	"gpt-5-codex":  true,
	"o1":           true,
	"o1-mini":      true,
	"o1-pro":       true,
	"o3":           true,
	"o3-mini":      true,
	"o3-pro":       true,
	"o4-mini":      true,
}

// openAIReasoningPattern matches model names like o1, o1-mini, o3, o4-mini.
var openAIReasoningPattern = regexp.MustCompile(`^o\d`)

// ── Catalogs ──

var openAILegacyCatalog = []CapabilityCatalogEntry{
	{Matches: isOpenAIReasoningModel, Capability: openAIReasoningCapability},
	{Matches: hasPrefixMatcher(openAIVisionToolPrefixes), Capability: openAIVisionToolCapability},
	{Matches: func(name string) bool { return strings.HasPrefix(name, "gpt-3.5-turbo") }, Capability: openAITextToolCapability},
}

var openAIResponsesCatalog = []CapabilityCatalogEntry{
	{Matches: isOpenAIReasoningModel, Capability: openAIReasoningCapability},
	{Matches: hasPrefixMatcher(openAIVisionToolPrefixes), Capability: openAIVisionToolCapability},
}

var anthropicCatalog = []CapabilityCatalogEntry{
	{Matches: hasPrefixMatcher(claudeVisionToolPrefixes), Capability: anthropicVisionToolCapability},
	{Matches: hasPrefixMatcher(claudeThinkingVisionToolPrefixes), Capability: anthropicThinkingVisionToolCapability},
}

// ── Helpers ──

func hasPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func hasPrefixMatcher(prefixes []string) CapabilityMatcher {
	return func(name string) bool { return hasPrefix(name, prefixes) }
}

func isOpenAIReasoningModel(name string) bool {
	return openAIReasoningPattern.MatchString(name)
}

func capabilityFromCatalog(modelName string, catalog []CapabilityCatalogEntry) kosong.ModelCapability {
	normalized := strings.ToLower(modelName)
	for _, entry := range catalog {
		if entry.Matches(normalized) {
			return entry.Capability
		}
	}
	return kosong.UnknownCapability
}

// ── Public API ──

// GetOpenAILegacyModelCapability returns the capability for a model using
// the legacy OpenAI chat completions API.
func GetOpenAILegacyModelCapability(modelName string) kosong.ModelCapability {
	return capabilityFromCatalog(modelName, openAILegacyCatalog)
}

// GetOpenAIResponsesModelCapability returns the capability for a model using
// the OpenAI Responses API.
func GetOpenAIResponsesModelCapability(modelName string) kosong.ModelCapability {
	return capabilityFromCatalog(modelName, openAIResponsesCatalog)
}

// GetAnthropicModelCapability returns the capability for an Anthropic model.
func GetAnthropicModelCapability(modelName string) kosong.ModelCapability {
	return capabilityFromCatalog(modelName, anthropicCatalog)
}

// GetGoogleGenAIModelCapability returns the capability for a Gemini model.
func GetGoogleGenAIModelCapability(modelName string) kosong.ModelCapability {
	normalized := strings.ToLower(modelName)
	if !strings.HasPrefix(normalized, "gemini-") {
		return kosong.UnknownCapability
	}
	if !hasPrefix(normalized, geminiCataloguedPrefixes) {
		return kosong.UnknownCapability
	}
	if strings.HasPrefix(normalized, "gemini-2.5-") || strings.Contains(normalized, "thinking") {
		return geminiThinkingMultimodalToolCapability
	}
	return geminiMultimodalToolCapability
}

// UsesOpenAIResponsesDeveloperRole returns true if the model uses the
// "developer" role instead of "system" in the OpenAI Responses API.
func UsesOpenAIResponsesDeveloperRole(modelName string) bool {
	normalized := strings.ToLower(modelName)
	if openAIResponsesDeveloperRoleModels[normalized] {
		return true
	}
	for model := range openAIResponsesDeveloperRoleModels {
		if strings.HasPrefix(normalized, model+"-") {
			return true
		}
	}
	return false
}
