package kosong

import (
	"regexp"
	"strings"
)

// CatalogModelEntry represents a model in the models.dev-style catalog.
// It captures metadata about a specific model: limits, capabilities, modalities.
type CatalogModelEntry struct {
	ID                     string                    `json:"id,omitempty"`
	Name                   string                    `json:"name,omitempty"`
	Family                 string                    `json:"family,omitempty"`
	LimitContext           int                       `json:"limitContext,omitempty"`
	LimitInput             int                       `json:"limitInput,omitempty"`
	LimitOutput            int                       `json:"limitOutput,omitempty"`
	ToolCall               *bool                     `json:"toolCall,omitempty"`
	Reasoning              bool                      `json:"reasoning,omitempty"`
	ReasoningOptions       []CatalogReasoningOption  `json:"reasoningOptions,omitempty"`
	Status                 string                    `json:"status,omitempty"`
	ModalitiesInput        []string                  `json:"modalitiesInput,omitempty"`
	ModalitiesOutput       []string                  `json:"modalitiesOutput,omitempty"`
	DynamicallyLoadedTools bool                      `json:"dynamicallyLoadedTools,omitempty"`
	InterleavedField       string                    `json:"interleavedField,omitempty"`
}

// CatalogReasoningOption represents a reasoning option from models.dev.
type CatalogReasoningOption struct {
	Type   string   `json:"type,omitempty"`
	Values []string `json:"values,omitempty"`
}

// CatalogProviderEntry represents a provider in the models.dev-style catalog.
type CatalogProviderEntry struct {
	ID     string                        `json:"id,omitempty"`
	Name   string                        `json:"name,omitempty"`
	API    string                        `json:"api,omitempty"`
	Env    []string                      `json:"env,omitempty"`
	NPM    string                        `json:"npm,omitempty"`
	Type   string                        `json:"type,omitempty"`
	Models map[string]CatalogModelEntry  `json:"models,omitempty"`
}

// CatalogModel is a normalized catalog model with resolved capabilities.
type CatalogModel struct {
	ID                string
	Name              string
	MaxOutputSize     int
	ReasoningKey      string
	SupportEfforts    []string
	OffEffort         string
	AlwaysThinking    bool
	Capability        ModelCapability
}

// Catalog is a map of provider ID to provider entry.
type Catalog map[string]CatalogProviderEntry

// embeddingPattern matches model names that are embeddings (not chat models).
var embeddingPattern = regexp.MustCompile(`(?i)embed`)

// isUsableChatModel returns true if the model is usable for chat (not embedding, not deprecated).
func isUsableChatModel(m CatalogModelEntry) bool {
	// Check output modalities
	if len(m.ModalitiesOutput) > 0 {
		hasText := false
		for _, o := range m.ModalitiesOutput {
			if o == "text" {
				hasText = true
				break
			}
		}
		if !hasText {
			return false
		}
	}
	// Skip deprecated/alpha models
	if m.Status == "deprecated" || m.Status == "alpha" {
		return false
	}
	// Skip embedding models
	if embeddingPattern.MatchString(m.Family) ||
		embeddingPattern.MatchString(m.ID) ||
		embeddingPattern.MatchString(m.Name) {
		return false
	}
	return true
}

// CatalogModelToCapability normalizes a catalog model entry into a CatalogModel.
func CatalogModelToCapability(m CatalogModelEntry) *CatalogModel {
	if m.ID == "" {
		return nil
	}
	if m.LimitContext <= 0 {
		return nil
	}
	if !isUsableChatModel(m) {
		return nil
	}

	// Build capability
	cap := ModelCapability{
		ImageIn:                containsStr(m.ModalitiesInput, "image"),
		VideoIn:                containsStr(m.ModalitiesInput, "video"),
		AudioIn:                containsStr(m.ModalitiesInput, "audio"),
		Thinking:               m.Reasoning,
		ToolUse:                true,
		MaxContextTokens:       m.LimitContext,
		MaxInputTokens:         0,
		DynamicallyLoadedTools: m.DynamicallyLoadedTools,
	}
	if m.ToolCall != nil {
		cap.ToolUse = *m.ToolCall
	}
	if m.LimitInput > 0 && m.LimitInput < m.LimitContext {
		cap.MaxInputTokens = m.LimitInput
	}

	// Thinking support can also be inferred from reasoning options
	thinking := catalogThinkingOptions(m.ReasoningOptions)
	if thinking.efforts != nil || thinking.hasToggle {
		cap.Thinking = true
	}

	model := &CatalogModel{
		ID:             m.ID,
		Name:           m.Name,
		MaxOutputSize:  m.LimitOutput,
		SupportEfforts: thinking.efforts,
		OffEffort:      thinking.offEffort,
		AlwaysThinking: thinking.alwaysThinking,
		Capability:     cap,
	}

	// Reasoning key from interleaved field
	if m.InterleavedField != "" {
		model.ReasoningKey = m.InterleavedField
	}

	return model
}

type thinkingOptions struct {
	efforts        []string
	offEffort      string
	hasToggle      bool
	alwaysThinking bool
}

func catalogThinkingOptions(opts []CatalogReasoningOption) thinkingOptions {
	var result thinkingOptions
	for _, opt := range opts {
		if opt.Type == "toggle" {
			result.hasToggle = true
			continue
		}
		if opt.Type != "effort" || len(opt.Values) == 0 {
			continue
		}
		var offEffort string
		var selectable []string
		for _, v := range opt.Values {
			if strings.EqualFold(v, "none") {
				offEffort = v
			} else if v != "" {
				selectable = append(selectable, v)
			}
		}
		if offEffort != "" {
			result.offEffort = offEffort
		}
		if len(selectable) > 0 {
			result.efforts = selectable
		}
	}
	// Always thinking: has effort levels, no off effort, no toggle
	if result.efforts != nil && result.offEffort == "" && !result.hasToggle {
		result.alwaysThinking = true
	}
	return result
}

// CatalogProviderModels extracts normalized models from a provider entry.
func CatalogProviderModels(entry CatalogProviderEntry) []CatalogModel {
	var models []CatalogModel
	for _, raw := range entry.Models {
		cm := CatalogModelToCapability(raw)
		if cm != nil {
			models = append(models, *cm)
		}
	}
	return models
}

// ── Built-in catalog ──

// builtinCatalog contains the built-in model catalog for known providers.
var builtinCatalog = Catalog{
	"openai": {
		ID:   "openai",
		Name: "OpenAI",
		API:  "https://api.openai.com/v1",
		Env:  []string{"OPENAI_API_KEY"},
		Type: "openai",
		Models: map[string]CatalogModelEntry{
			"gpt-4o": {
				ID: "gpt-4o", Name: "GPT-4o", LimitContext: 128000, LimitOutput: 16384,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"gpt-4o-mini": {
				ID: "gpt-4o-mini", Name: "GPT-4o mini", LimitContext: 128000, LimitOutput: 16384,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"gpt-4.1": {
				ID: "gpt-4.1", Name: "GPT-4.1", LimitContext: 1047576, LimitOutput: 32768,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"gpt-4.1-mini": {
				ID: "gpt-4.1-mini", Name: "GPT-4.1 mini", LimitContext: 1047576, LimitOutput: 32768,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"gpt-4.1-nano": {
				ID: "gpt-4.1-nano", Name: "GPT-4.1 nano", LimitContext: 1047576, LimitOutput: 32768,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"o1": {
				ID: "o1", Name: "o1", LimitContext: 200000, LimitOutput: 100000,
				Reasoning: true, ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"o1-mini": {
				ID: "o1-mini", Name: "o1 mini", LimitContext: 128000, LimitOutput: 65536,
				Reasoning: true, ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
			"o3": {
				ID: "o3", Name: "o3", LimitContext: 200000, LimitOutput: 100000,
				Reasoning: true, ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"o3-mini": {
				ID: "o3-mini", Name: "o3 mini", LimitContext: 200000, LimitOutput: 100000,
				Reasoning: true, ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
			"o4-mini": {
				ID: "o4-mini", Name: "o4 mini", LimitContext: 200000, LimitOutput: 100000,
				Reasoning: true, ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"gpt-3.5-turbo": {
				ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", LimitContext: 16385, LimitOutput: 4096,
				ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
		},
	},
	"anthropic": {
		ID:   "anthropic",
		Name: "Anthropic",
		API:  "https://api.anthropic.com",
		Env:  []string{"ANTHROPIC_API_KEY"},
		Type: "anthropic",
		Models: map[string]CatalogModelEntry{
			"claude-sonnet-4-20250514": {
				ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", LimitContext: 200000, LimitOutput: 8192,
				Reasoning: true, ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"claude-opus-4-20250514": {
				ID: "claude-opus-4-20250514", Name: "Claude Opus 4", LimitContext: 200000, LimitOutput: 8192,
				Reasoning: true, ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"claude-3-7-sonnet-20250219": {
				ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", LimitContext: 200000, LimitOutput: 8192,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"claude-3-5-sonnet-20241022": {
				ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", LimitContext: 200000, LimitOutput: 8192,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
			"claude-3-5-haiku-20241022": {
				ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", LimitContext: 200000, LimitOutput: 8192,
				ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
			"claude-3-opus-20240229": {
				ID: "claude-3-opus-20240229", Name: "Claude 3 Opus", LimitContext: 200000, LimitOutput: 4096,
				ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
			},
		},
	},
	"google": {
		ID:   "google",
		Name: "Google",
		API:  "https://generativelanguage.googleapis.com/v1beta",
		Env:  []string{"GOOGLE_API_KEY"},
		Type: "google-genai",
		Models: map[string]CatalogModelEntry{
			"gemini-2.5-pro": {
				ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", LimitContext: 1048576, LimitOutput: 65536,
				Reasoning: true, ModalitiesInput: []string{"text", "image", "video", "audio"}, ToolCall: boolPtr(true),
			},
			"gemini-2.5-flash": {
				ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", LimitContext: 1048576, LimitOutput: 65536,
				Reasoning: true, ModalitiesInput: []string{"text", "image", "video", "audio"}, ToolCall: boolPtr(true),
			},
			"gemini-2.0-flash": {
				ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", LimitContext: 1048576, LimitOutput: 8192,
				ModalitiesInput: []string{"text", "image", "video", "audio"}, ToolCall: boolPtr(true),
			},
			"gemini-2.0-flash-lite": {
				ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash-Lite", LimitContext: 1048576, LimitOutput: 8192,
				ModalitiesInput: []string{"text", "image", "video", "audio"}, ToolCall: boolPtr(true),
			},
			"gemini-1.5-pro": {
				ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", LimitContext: 2097152, LimitOutput: 8192,
				ModalitiesInput: []string{"text", "image", "video", "audio"}, ToolCall: boolPtr(true),
			},
			"gemini-1.5-flash": {
				ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", LimitContext: 1048576, LimitOutput: 8192,
				ModalitiesInput: []string{"text", "image", "video", "audio"}, ToolCall: boolPtr(true),
			},
		},
	},
	"kimi": {
		ID:   "kimi",
		Name: "Kimi (Moonshot)",
		API:  "https://api.moonshot.cn/v1",
		Env:  []string{"MOONSHOT_API_KEY"},
		Type: "kimi",
		Models: map[string]CatalogModelEntry{
			"moonshot-v1-8k": {
				ID: "moonshot-v1-8k", Name: "Moonshot v1 8K", LimitContext: 8192, LimitOutput: 4096,
				ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
			"moonshot-v1-32k": {
				ID: "moonshot-v1-32k", Name: "Moonshot v1 32K", LimitContext: 32768, LimitOutput: 4096,
				ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
			"moonshot-v1-128k": {
				ID: "moonshot-v1-128k", Name: "Moonshot v1 128K", LimitContext: 131072, LimitOutput: 4096,
				ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
			"kimi-latest": {
				ID: "kimi-latest", Name: "Kimi Latest", LimitContext: 131072, LimitOutput: 8192,
				Reasoning: true, ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
			},
		},
	},
}

// GetBuiltinCatalog returns the built-in model catalog.
func GetBuiltinCatalog() Catalog {
	return builtinCatalog
}

// LookupModelCapability looks up a model's capability from the built-in catalog.
// It searches all providers for a matching model ID. Returns UnknownCapability if not found.
func LookupModelCapability(modelName string) ModelCapability {
	normalized := strings.ToLower(modelName)
	for _, provider := range builtinCatalog {
		for _, model := range provider.Models {
			if strings.ToLower(model.ID) == normalized || strings.HasPrefix(normalized, strings.ToLower(model.ID)) {
				cm := CatalogModelToCapability(model)
				if cm != nil {
					return cm.Capability
				}
			}
		}
	}
	return UnknownCapability
}

// LookupProviderModels returns all normalized models for a provider from the built-in catalog.
func LookupProviderModels(providerID string) []CatalogModel {
	provider, ok := builtinCatalog[providerID]
	if !ok {
		return nil
	}
	return CatalogProviderModels(provider)
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func boolPtr(b bool) *bool {
	return &b
}
