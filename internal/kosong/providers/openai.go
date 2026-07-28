// Package providers implements LLM provider adapters.
//
// The OpenAI-compatible provider handles the chat completions API format
// used by OpenAI, Kimi (Moonshot), DeepSeek, and other compatible APIs.
// It converts kosong messages/tools to the wire format, sends HTTP requests
// with SSE streaming, and parses response chunks into StreamedMessagePart.
package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/trace"
)

// ── OpenAI wire types ──

type openAIRequest struct {
	Model          string             `json:"model"`
	Messages       []openAIMessage    `json:"messages"`
	Tools          []openAITool       `json:"tools,omitempty"`
	Stream         bool               `json:"stream"`
	Temperature    *float64           `json:"temperature,omitempty"`
	MaxTokens      *int               `json:"max_tokens,omitempty"`
	ResponseFormat *openAIRespFormat  `json:"response_format,omitempty"`
	StreamOptions  *openAIStreamOpts  `json:"stream_options,omitempty"`
}

type openAIStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIRespFormat struct {
	Type       string           `json:"type"`
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
}

type openAIJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict *bool           `json:"strict,omitempty"`
}

type openAIMessage struct {
	Role       string            `json:"role"`
	Content    interface{}       `json:"content"` // string | []openAIContentPart
	ToolCalls  []openAIToolCall  `json:"tool_calls,omitempty"`
	ToolCallID *string           `json:"tool_call_id,omitempty"`
	Name       *string           `json:"name,omitempty"`
}

type openAIContentPart struct {
	Type     string           `json:"type"`
	Text     string           `json:"text,omitempty"`
	ImageURL *openAIMediaURL  `json:"image_url,omitempty"`
	AudioURL *openAIMediaURL  `json:"audio_url,omitempty"`
	VideoURL *openAIMediaURL  `json:"video_url,omitempty"`
}

type openAIMediaURL struct {
	URL string  `json:"url"`
	ID  *string `json:"id,omitempty"`
}

type openAIToolCall struct {
	Index    *int               `json:"index,omitempty"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string  `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openAIResponse struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Model   string            `json:"model"`
	Choices []openAIChoice    `json:"choices"`
	Usage   *openAIUsage      `json:"usage,omitempty"`
	Error   *openAIErrorBody  `json:"error,omitempty"`
}

type openAIChoice struct {
	Index        int             `json:"index"`
	Message      *openAIMessage  `json:"message,omitempty"`
	Delta        *openAIMessage  `json:"delta,omitempty"`
	FinishReason *string         `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens        int                      `json:"prompt_tokens"`
	CompletionTokens    int                      `json:"completion_tokens"`
	PromptDetails       *openAIUsageDetail       `json:"prompt_tokens_details,omitempty"`
	CompletionDetails   *openAICompletionDetail  `json:"completion_tokens_details,omitempty"`
}

type openAIUsageDetail struct {
	CachedTokens int `json:"cached_tokens"`
}

type openAICompletionDetail struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type openAIErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Streaming chunk types (for SSE delta parsing)
type openAIChoiceDelta struct {
	Role             string           `json:"role"`
	Content          *string          `json:"content"`
	ReasoningContent string           `json:"reasoning_content"`
	ToolCalls        []openAIToolCall `json:"tool_calls"`
}

type openAIResponseChunk struct {
	ID      string              `json:"id"`
	Choices []openAIChoiceChunk `json:"choices"`
	Usage   *openAIUsage        `json:"usage,omitempty"`
}

type openAIChoiceChunk struct {
	Index        int                `json:"index"`
	Delta        *openAIChoiceDelta `json:"delta"`
	FinishReason *string            `json:"finish_reason"`
}

// ── OpenAIProvider ──

// OpenAIProviderConfig holds configuration for the OpenAI-compatible provider.
type OpenAIProviderConfig struct {
	Name           string
	APIKey         string
	BaseURL        string
	Model          string
	ThinkingEffort kosong.ThinkingEffort
	MaxTokens      int
	Temperature    *float64
	DefaultHeaders map[string]string
	HTTPClient     *http.Client
}

// OpenAIProvider implements kosong.ChatProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	name           string
	apiKey         string
	baseURL        string
	model          string
	thinkingEffort kosong.ThinkingEffort
	maxTokens      int
	temperature    *float64
	defaultHeaders map[string]string
	httpClient     *http.Client
}

// NewOpenAIProvider creates a new OpenAI-compatible provider.
func NewOpenAIProvider(cfg OpenAIProviderConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	// Normalize base URL: OpenAI-compatible APIs (DeepSeek, etc.) expect
	// requests under /v1. If the user provides a URL without a version path
	// (e.g. "https://api.deepseek.com"), append /v1 so the request hits
	// {baseURL}/v1/chat/completions instead of {baseURL}/chat/completions.
	if !hasVersionPath(cfg.BaseURL) {
		cfg.BaseURL += "/v1"
	}
	if cfg.Name == "" {
		cfg.Name = "openai"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAIProvider{
		name:           cfg.Name,
		apiKey:         cfg.APIKey,
		baseURL:        cfg.BaseURL,
		model:          cfg.Model,
		thinkingEffort: cfg.ThinkingEffort,
		maxTokens:      cfg.MaxTokens,
		temperature:    cfg.Temperature,
		defaultHeaders: cfg.DefaultHeaders,
		httpClient:     httpClient,
	}
}

// hasVersionPath reports whether the URL already has a version path segment
// such as /v1, /v2, or /v1beta. This prevents double-appending /v1 when
// users provide a fully-qualified base URL like "https://api.deepseek.com/v1".
func hasVersionPath(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		return false
	}
	last := path[strings.LastIndex(path, "/")+1:]
	return strings.HasPrefix(last, "v")
}

func (p *OpenAIProvider) Name() string                { return p.name }
func (p *OpenAIProvider) ModelName() string            { return p.model }
func (p *OpenAIProvider) ThinkingEffort() kosong.ThinkingEffort { return p.thinkingEffort }
func (p *OpenAIProvider) MaxCompletionTokens() int     { return p.maxTokens }

func (p *OpenAIProvider) WithThinking(effort kosong.ThinkingEffort) kosong.ChatProvider {
	cp := *p
	cp.thinkingEffort = effort
	return &cp
}

func (p *OpenAIProvider) WithMaxCompletionTokens(maxTokens int, _ *kosong.MaxCompletionTokensOptions) kosong.ChatProvider {
	cp := *p
	cp.maxTokens = maxTokens
	return &cp
}

func (p *OpenAIProvider) UploadVideo(_ context.Context, _ interface{}, _ *kosong.GenerateOptions) (*kosong.VideoURLPart, error) {
	return nil, fmt.Errorf("video upload not supported by %s provider", p.name)
}

// Generate sends a chat completion request and returns a streamed response.
func (p *OpenAIProvider) Generate(
	ctx context.Context,
	systemPrompt string,
	tools []kosong.Tool,
	history []kosong.Message,
	opts *kosong.GenerateOptions,
) (*kosong.StreamedMessage, error) {
	// Build wire messages
	var wireMessages []openAIMessage
	if systemPrompt != "" {
		wireMessages = append(wireMessages, openAIMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	for _, msg := range history {
		wireMessages = append(wireMessages, convertMessage(msg))
	}

	// Build wire tools
	var wireTools []openAITool
	for _, t := range tools {
		if t.Deferred {
			continue
		}
		wireTools = append(wireTools, convertTool(t))
	}

	// Build request body
	reqBody := openAIRequest{
		Model:         p.model,
		Messages:      wireMessages,
		Tools:         wireTools,
		Stream:        true,
		Temperature:   p.temperature,
		StreamOptions: &openAIStreamOpts{IncludeUsage: true},
	}
	if p.maxTokens > 0 {
		reqBody.MaxTokens = &p.maxTokens
	}
	if opts != nil && opts.ResponseFormat != nil {
		reqBody.ResponseFormat = convertResponseFormat(opts.ResponseFormat)
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Fire raw request callback for audit/diagnostics
	if opts != nil && opts.OnRawRequest != nil {
		opts.OnRawRequest(bodyBytes)
	}

	// Fire callbacks
	if opts != nil && opts.OnRequestStart != nil {
		opts.OnRequestStart()
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	for k, v := range p.defaultHeaders {
		req.Header.Set(k, v)
	}
	if opts != nil && opts.Auth != nil {
		if opts.Auth.APIKey != nil {
			req.Header.Set("Authorization", "Bearer "+*opts.Auth.APIKey)
		}
		for k, v := range opts.Auth.Headers {
			req.Header.Set(k, v)
		}
	}

	if opts != nil && opts.OnRequestSent != nil {
		opts.OnRequestSent()
	}

	// Trace the HTTP request
	if trace.Enabled() {
		trace.Log("http", "request", map[string]any{
			"url":   url,
			"model": p.model,
		})
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		trace.Log("http", "error", map[string]any{"error": err.Error()})
		// Fire raw response callback with network error for diagnostics
		if opts != nil && opts.OnRawResponse != nil {
			if f, ferr := os.CreateTemp("", "kimi-raw-resp-*.jsonl"); ferr == nil {
				fmt.Fprintf(f, "network_error: %s\n", err.Error())
				f.Close()
				opts.OnRawResponse(f.Name())
			}
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if trace.Enabled() {
		trace.Log("http", "response", map[string]any{
			"status":     resp.StatusCode,
			"model":      p.model,
		})
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// Fire raw response callback with error body for diagnostics
		if opts != nil && opts.OnRawResponse != nil {
			if f, ferr := os.CreateTemp("", "kimi-raw-resp-*.jsonl"); ferr == nil {
				fmt.Fprintf(f, "http_status: %d\n", resp.StatusCode)
				f.Write(body)
				f.Write([]byte("\n"))
				f.Close()
				opts.OnRawResponse(f.Name())
			}
		}
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Extract trace ID from headers
	var traceID *string
	if tid := resp.Header.Get("X-Request-Id"); tid != "" {
		traceID = &tid
		if opts != nil && opts.OnTraceID != nil {
			opts.OnTraceID(traceID)
		}
	}

	// Create streaming channel
	partsCh := make(chan kosong.StreamedMessagePart, 64)

	stream := &kosong.StreamedMessage{
		Parts:    partsCh,
		TraceID:  traceID,
	}

	go p.consumeSSEStream(ctx, resp.Body, partsCh, opts, stream)

	return stream, nil
}

// consumeSSEStream reads SSE events from the response body and sends parts to the channel.
// When raw response capture is enabled (OnRawResponse callback set), each SSE data line
// is streamed directly to a temporary file on disk (zero-buffer auditing) instead of
// accumulating in memory. The file path is passed to the callback when the stream ends.
func (p *OpenAIProvider) consumeSSEStream(
	ctx context.Context,
	body io.ReadCloser,
	partsCh chan<- kosong.StreamedMessagePart,
	opts *kosong.GenerateOptions,
	stream *kosong.StreamedMessage,
) {
	defer close(partsCh)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	tracing := trace.Enabled()
	var chunkCount int

	// Stream raw SSE data lines to a temp file for zero-buffer auditing.
	// Only created when the OnRawResponse callback is set.
	captureRaw := opts != nil && opts.OnRawResponse != nil
	var rawFile *os.File
	if captureRaw {
		var err error
		rawFile, err = os.CreateTemp("", "kimi-raw-resp-*.jsonl")
		if err != nil {
			captureRaw = false // fall back to no capture on file error
		}
	}

	defer func() {
		if tracing {
			trace.Log("http", "stream_end", map[string]any{"chunks": chunkCount})
		}
		if rawFile != nil {
			rawFile.Close()
			if opts.OnRawResponse != nil {
				opts.OnRawResponse(rawFile.Name())
			} else {
				os.Remove(rawFile.Name()) // cleanup if callback gone
			}
		}
	}()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// SSE format: "data: <json>" or "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			if opts != nil && opts.OnStreamEnd != nil {
				opts.OnStreamEnd(&kosong.StreamDecodeStats{})
			}
			return
		}

		// Stream raw data line to temp file for verbatim audit (zero-copy to disk)
		if captureRaw && rawFile != nil {
			if _, err := rawFile.WriteString(data + "\n"); err != nil {
				captureRaw = false // stop writing on first failure to avoid futile retries
			}
		}

		var chunk openAIResponseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}
		chunkCount++

		// Capture usage from chunk (usually in final chunk)
		if chunk.Usage != nil {
			usage := &kosong.TokenUsage{
				InputOther: chunk.Usage.PromptTokens,
				Output:     chunk.Usage.CompletionTokens,
			}
			if chunk.Usage.PromptDetails != nil {
				usage.InputCacheRead = chunk.Usage.PromptDetails.CachedTokens
				usage.InputOther -= usage.InputCacheRead
			}
			if chunk.Usage.CompletionDetails != nil {
				usage.ReasoningTokens = chunk.Usage.CompletionDetails.ReasoningTokens
			}
			select {
			case partsCh <- kosong.StreamedMessagePart{Type: "usage", Usage: usage}:
			case <-ctx.Done():
				return
			}
		}

		// Capture finish_reason from any choice in the chunk.
		// In OpenAI streaming, finish_reason appears in the last chunk's choice.
		// Only emit the first finish_reason to avoid multiple "finish" parts.
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil {
				// Populate stream-level metadata for callers using stream.FinishReason.
				if stream != nil {
					stream.RawFinishReason = choice.FinishReason
					reason := MapFinishReason(choice.FinishReason)
					stream.FinishReason = &reason
				}
				select {
				case partsCh <- kosong.StreamedMessagePart{
					Type:         "finish",
					FinishReason: choice.FinishReason,
				}:
				case <-ctx.Done():
					return
				}
				break // only first finish_reason
			}
		}

		// Process choices
		for _, choice := range chunk.Choices {
			delta := choice.Delta
			if delta == nil {
				continue
			}

			// Handle content (text)
			if delta.Content != nil && *delta.Content != "" {
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "text", Text: *delta.Content}:
				case <-ctx.Done():
					return
				}
			}

			// Handle reasoning_content (thinking) for Kimi/DeepSeek
			if delta.ReasoningContent != "" {
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "think", Think: delta.ReasoningContent}:
				case <-ctx.Done():
					return
				}
			}

			// Handle tool calls
			for _, tc := range delta.ToolCalls {
				// Tool call header (first delta for this index)
				if tc.ID != "" && tc.Function.Name != "" {
					select {
					case partsCh <- kosong.StreamedMessagePart{
						Type:  "function",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Index: tc.Index,
					}:
					case <-ctx.Done():
						return
					}
				}

				// Tool call arguments delta
				if tc.Function.Arguments != nil && *tc.Function.Arguments != "" {
					args := *tc.Function.Arguments
					select {
					case partsCh <- kosong.StreamedMessagePart{
						Type:          "tool_call_part",
						ArgumentsPart: &args,
						Index:         tc.Index,
					}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}
}

// ── Message conversion ──

// convertMessage converts a kosong.Message to the OpenAI wire format.
func convertMessage(msg kosong.Message) openAIMessage {
	result := openAIMessage{
		Role: string(msg.Role),
		Name: msg.Name,
	}

	switch msg.Role {
	case kosong.RoleUser, kosong.RoleSystem:
		result.Content = convertContent(msg.Content)

	case kosong.RoleAssistant:
		result.Content = convertContent(msg.Content)
		if len(msg.ToolCalls) > 0 {
			for i, tc := range msg.ToolCalls {
				idx := i
				result.ToolCalls = append(result.ToolCalls, openAIToolCall{
					Index: &idx,
					ID:    tc.ID,
					Type:  "function",
					Function: openAIFunctionCall{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}

	case kosong.RoleTool:
		result.Content = convertContent(msg.Content)
		result.ToolCallID = msg.ToolCallID
	}

	return result
}

// convertContent converts kosong content parts to OpenAI content format.
// For simple text-only content, returns a plain string. Otherwise returns []openAIContentPart.
func convertContent(parts []kosong.ContentPart) interface{} {
	if len(parts) == 0 {
		return ""
	}

	// Fast path: single text part → string
	if len(parts) == 1 && parts[0].Type == "text" {
		return parts[0].Text
	}

	// Check if all parts are text (concatenate)
	allText := true
	for _, p := range parts {
		if p.Type != "text" && p.Type != "think" {
			allText = false
			break
		}
	}
	if allText {
		var sb strings.Builder
		for i, p := range parts {
			if p.Type == "text" {
				if i > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(p.Text)
			}
		}
		return sb.String()
	}

	// Multi-modal content
	var result []openAIContentPart
	for _, p := range parts {
		switch p.Type {
		case "text":
			result = append(result, openAIContentPart{Type: "text", Text: p.Text})
		case "image_url":
			if p.ImageURL != nil {
				result = append(result, openAIContentPart{
					Type:     "image_url",
					ImageURL: &openAIMediaURL{URL: p.ImageURL.URL, ID: p.ImageURL.ID},
				})
			}
		case "audio_url":
			if p.AudioURL != nil {
				result = append(result, openAIContentPart{
					Type:     "audio_url",
					AudioURL: &openAIMediaURL{URL: p.AudioURL.URL, ID: p.AudioURL.ID},
				})
			}
		case "video_url":
			if p.VideoURL != nil {
				result = append(result, openAIContentPart{
					Type:     "video_url",
					VideoURL: &openAIMediaURL{URL: p.VideoURL.URL, ID: p.VideoURL.ID},
				})
			}
		}
		// Skip "think" parts in request (they're in reasoning_content)
	}
	return result
}

// convertTool converts a kosong.Tool to the OpenAI wire format.
func convertTool(t kosong.Tool) openAITool {
	return openAITool{
		Type: "function",
		Function: openAIFunction{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	}
}

// convertResponseFormat converts a kosong.ResponseFormat.
func convertResponseFormat(rf *kosong.ResponseFormat) *openAIRespFormat {
	if rf == nil {
		return nil
	}
	result := &openAIRespFormat{Type: rf.Type}
	if rf.JSONSchema != nil {
		schemaBytes, _ := json.Marshal(rf.JSONSchema.Schema)
		result.JSONSchema = &openAIJSONSchema{
			Name:   rf.JSONSchema.Name,
			Schema: schemaBytes,
			Strict: rf.JSONSchema.Strict,
		}
	}
	return result
}

// MapFinishReason normalizes an OpenAI finish_reason to kosong.FinishReason.
func MapFinishReason(raw *string) kosong.FinishReason {
	if raw == nil {
		return kosong.FinishOther
	}
	switch *raw {
	case "stop":
		return kosong.FinishCompleted
	case "tool_calls", "function_call":
		return kosong.FinishToolCalls
	case "length":
		return kosong.FinishTruncated
	case "content_filter":
		return kosong.FinishFiltered
	default:
		return kosong.FinishOther
	}
}
