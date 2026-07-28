package kosong

import "context"

// ThinkingEffort is the thinking effort passed to ChatProvider.WithThinking.
// "off" and "on" are reserved; other values are model-declared efforts.
type ThinkingEffort string

const (
	ThinkingOff ThinkingEffort = "off"
	ThinkingOn  ThinkingEffort = "on"
)

// FinishReason is a normalized finish-reason signal.
type FinishReason string

const (
	FinishCompleted FinishReason = "completed"
	FinishToolCalls FinishReason = "tool_calls"
	FinishTruncated FinishReason = "truncated"
	FinishFiltered  FinishReason = "filtered"
	FinishPaused    FinishReason = "paused"
	FinishOther     FinishReason = "other"
)

// ResponseFormat constrains model output format.
type ResponseFormat struct {
	Type       string      `json:"type"` // "json_object" or "json_schema"
	JSONSchema *JSONSchema `json:"jsonSchema,omitempty"`
}

// JSONSchema is the schema for json_schema response format.
type JSONSchema struct {
	Name        string                 `json:"name"`
	Schema      map[string]interface{} `json:"schema"`
	Strict      *bool                  `json:"strict,omitempty"`
	Description *string                `json:"description,omitempty"`
}

// MaxCompletionTokensOptions provides context for clamping completion tokens.
type MaxCompletionTokensOptions struct {
	UsedContextTokens *int `json:"usedContextTokens,omitempty"`
	MaxContextTokens  *int `json:"maxContextTokens,omitempty"`
}

// ProviderRequestAuth is request-scoped provider auth.
type ProviderRequestAuth struct {
	APIKey  *string           `json:"apiKey,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// StreamDecodeStats is decode-phase accounting for a streamed generation.
type StreamDecodeStats struct {
	ServerDecodeMs float64 `json:"serverDecodeMs"`
	ClientConsumeMs float64 `json:"clientConsumeMs"`
}

// VideoUploadInput is in-memory video bytes for providers requiring uploaded file references.
type VideoUploadInput struct {
	Data     []byte  `json:"data"`
	MIMEType string  `json:"mimeType"`
	Filename *string `json:"filename,omitempty"`
}

// GenerateOptions are per-call options for ChatProvider.Generate.
type GenerateOptions struct {
	Auth           *ProviderRequestAuth `json:"auth,omitempty"`
	ResponseFormat *ResponseFormat      `json:"responseFormat,omitempty"`
	OnRequestStart func()               `json:"-"`
	OnRequestSent  func()               `json:"-"`
	OnTraceID      func(traceID *string) `json:"-"`
	OnStreamEnd    func(stats *StreamDecodeStats) `json:"-"`
}

// StreamedMessage is a stream of message parts produced by a single LLM response.
// Consumers receive parts from the channel until it closes.
type StreamedMessage struct {
	Parts           <-chan StreamedMessagePart
	ID              *string
	Usage           *TokenUsage
	FinishReason    *FinishReason
	RawFinishReason *string
	TraceID         *string
}

// ChatProvider is the unified interface for an LLM chat provider.
type ChatProvider interface {
	// Name returns a short identifier for the provider backend.
	Name() string
	// ModelName returns the model name passed to the upstream API.
	ModelName() string
	// ThinkingEffort returns the current thinking effort, or empty if not configured.
	ThinkingEffort() ThinkingEffort
	// MaxCompletionTokens returns the effective completion-token cap, or 0 if unset.
	MaxCompletionTokens() int
	// Generate sends a conversation to the LLM and returns a streamed response.
	Generate(ctx context.Context, systemPrompt string, tools []Tool, history []Message, opts *GenerateOptions) (*StreamedMessage, error)
	// WithThinking returns a shallow copy with the given thinking effort.
	WithThinking(effort ThinkingEffort) ChatProvider
	// WithMaxCompletionTokens returns a shallow copy with the given completion budget.
	WithMaxCompletionTokens(maxTokens int, opts *MaxCompletionTokensOptions) ChatProvider
	// UploadVideo uploads a video and returns a content part. Optional.
	UploadVideo(ctx context.Context, input interface{}, opts *GenerateOptions) (*VideoURLPart, error)
}
