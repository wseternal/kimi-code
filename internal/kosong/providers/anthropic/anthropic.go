// Package anthropic implements the Anthropic Messages API provider adapter.
// It handles native content blocks, thinking configuration, tool_use/tool_result
// format, and SSE streaming for Claude models.
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/trace"
)

// ── Anthropic wire types ──

type anthropicRequest struct {
	Model     string              `json:"model"`
	System    interface{}         `json:"system,omitempty"` // string | []anthropicSystemBlock
	Messages  []anthropicMessage  `json:"messages"`
	Tools     []anthropicTool     `json:"tools,omitempty"`
	MaxTokens int                 `json:"max_tokens"`
	Stream    bool                `json:"stream"`
	Thinking  *anthropicThinking  `json:"thinking,omitempty"`
	Metadata  *anthropicMetadata  `json:"metadata,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
}

type anthropicSystemBlock struct {
	Type      string `json:"type"` // "text"
	Text      string `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicThinking struct {
	Type         string `json:"type"`                    // "enabled" or "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // for "enabled"
}

type anthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"` // "user" or "assistant"
	Content interface{} `json:"content"` // string | []anthropicContentBlock
}

type anthropicContentBlock struct {
	Type         string              `json:"type"` // "text", "image", "tool_use", "tool_result", "thinking"
	Text         string              `json:"text,omitempty"`
	Thinking     string              `json:"thinking,omitempty"`
	Source       *anthropicImageSource `json:"source,omitempty"`
	ID           string              `json:"id,omitempty"`
	Name         string              `json:"name,omitempty"`
	Input        interface{}         `json:"input,omitempty"`
	ToolUseID    string              `json:"tool_use_id,omitempty"`
	Content      interface{}         `json:"content,omitempty"` // for tool_result
	IsError      *bool               `json:"is_error,omitempty"`
	Signature    string              `json:"signature,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"` // "base64" or "url"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []anthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   *string                 `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        *anthropicUsage         `json:"usage"`
	Error        *anthropicErrorBody     `json:"error,omitempty"`
}

type anthropicUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadTokens    int `json:"cache_read_input_tokens,omitempty"`
}

type anthropicErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// SSE streaming event types
type anthropicStreamEvent struct {
	Type         string              `json:"type"`
	Message      *anthropicResponse  `json:"message,omitempty"`
	Index        *int                `json:"index,omitempty"`
	ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`
	Delta        *anthropicDelta     `json:"delta,omitempty"`
	Usage        *anthropicUsage     `json:"usage,omitempty"`
}

type anthropicDelta struct {
	Type         string  `json:"type"` // "text_delta", "thinking_delta", "input_json_delta", "signature_delta"
	Text         string  `json:"text,omitempty"`
	Thinking     string  `json:"thinking,omitempty"`
	PartialJSON  string  `json:"partial_json,omitempty"`
	Signature    string  `json:"signature,omitempty"`
	StopReason   *string `json:"stop_reason,omitempty"`
	Usage        *anthropicUsage `json:"usage,omitempty"`
}

// ── Provider ──

// Config holds Anthropic provider configuration.
type Config struct {
	APIKey         string
	BaseURL        string // default: https://api.anthropic.com
	Model          string
	ThinkingEffort kosong.ThinkingEffort
	MaxTokens      int
	Temperature    *float64
	DefaultHeaders map[string]string
	HTTPClient     *http.Client
	// ThinkingBudget is the token budget for extended thinking.
	// If 0, thinking is not explicitly configured.
	ThinkingBudget int
}

// Provider implements kosong.ChatProvider for Anthropic's Messages API.
type Provider struct {
	apiKey         string
	baseURL        string
	model          string
	thinkingEffort kosong.ThinkingEffort
	maxTokens      int
	temperature    *float64
	defaultHeaders map[string]string
	httpClient     *http.Client
	thinkingBudget int
}

// NewProvider creates a new Anthropic provider.
func NewProvider(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 8192
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Provider{
		apiKey:         cfg.APIKey,
		baseURL:        cfg.BaseURL,
		model:          cfg.Model,
		thinkingEffort: cfg.ThinkingEffort,
		maxTokens:      cfg.MaxTokens,
		temperature:    cfg.Temperature,
		defaultHeaders: cfg.DefaultHeaders,
		httpClient:     httpClient,
		thinkingBudget: cfg.ThinkingBudget,
	}
}

func (p *Provider) Name() string                         { return "anthropic" }
func (p *Provider) ModelName() string                     { return p.model }
func (p *Provider) ThinkingEffort() kosong.ThinkingEffort { return p.thinkingEffort }
func (p *Provider) MaxCompletionTokens() int              { return p.maxTokens }

func (p *Provider) WithThinking(effort kosong.ThinkingEffort) kosong.ChatProvider {
	cp := *p
	cp.thinkingEffort = effort
	return &cp
}

func (p *Provider) WithMaxCompletionTokens(maxTokens int, _ *kosong.MaxCompletionTokensOptions) kosong.ChatProvider {
	cp := *p
	cp.maxTokens = maxTokens
	return &cp
}

func (p *Provider) UploadVideo(_ context.Context, _ interface{}, _ *kosong.GenerateOptions) (*kosong.VideoURLPart, error) {
	return nil, fmt.Errorf("video upload not supported by anthropic provider")
}

// Generate sends a Messages API request to Anthropic and returns a streamed response.
func (p *Provider) Generate(
	ctx context.Context,
	systemPrompt string,
	tools []kosong.Tool,
	history []kosong.Message,
	opts *kosong.GenerateOptions,
) (*kosong.StreamedMessage, error) {
	// Build wire messages
	var messages []anthropicMessage
	for _, msg := range history {
		messages = append(messages, convertMessage(msg))
	}

	// Build wire tools
	var wireTools []anthropicTool
	for _, t := range tools {
		if t.Deferred {
			continue
		}
		wireTools = append(wireTools, convertTool(t))
	}

	// Build request
	reqBody := anthropicRequest{
		Model:     p.model,
		Messages:  messages,
		Tools:     wireTools,
		MaxTokens: p.maxTokens,
		Stream:    true,
		Temperature: p.temperature,
	}

	// System prompt
	if systemPrompt != "" {
		reqBody.System = systemPrompt
	}

	// Thinking configuration
	if p.thinkingBudget > 0 {
		reqBody.Thinking = &anthropicThinking{
			Type:         "enabled",
			BudgetTokens: p.thinkingBudget,
		}
		// When thinking is enabled, temperature must be 1
		reqBody.Temperature = nil
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if opts != nil && opts.OnRawRequest != nil {
		opts.OnRawRequest(bodyBytes)
	}
	if opts != nil && opts.OnRequestStart != nil {
		opts.OnRequestStart()
	}

	url := p.baseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("anthropic-version", "2023-06-01")
	if p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}
	for k, v := range p.defaultHeaders {
		req.Header.Set(k, v)
	}

	// Request-scoped auth override
	if opts != nil && opts.Auth != nil {
		if opts.Auth.APIKey != nil {
			req.Header.Set("x-api-key", *opts.Auth.APIKey)
		}
		for k, v := range opts.Auth.Headers {
			req.Header.Set(k, v)
		}
	}

	// Beta header for extended thinking
	if p.thinkingBudget > 0 {
		req.Header.Set("anthropic-beta", "output-128k-2025-02-19")
	}

	if opts != nil && opts.OnRequestSent != nil {
		opts.OnRequestSent()
	}

	if trace.Enabled() {
		trace.Log("http", "request", map[string]any{"url": url, "model": p.model})
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, kosong.ClassifyBaseAPIError(fmt.Sprintf("request failed: %s", err))
	}

	if trace.Enabled() {
		trace.Log("http", "response", map[string]any{"status": resp.StatusCode, "model": p.model})
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var requestID *string
		if rid := resp.Header.Get("X-Request-Id"); rid != "" {
			requestID = &rid
		}
		return nil, kosong.NormalizeAPIStatusError(resp.StatusCode, string(body), requestID, nil, nil)
	}

	var traceID *string
	if tid := resp.Header.Get("X-Request-Id"); tid != "" {
		traceID = &tid
		if opts != nil && opts.OnTraceID != nil {
			opts.OnTraceID(traceID)
		}
	}

	partsCh := make(chan kosong.StreamedMessagePart, 64)
	stream := &kosong.StreamedMessage{
		Parts:   partsCh,
		TraceID: traceID,
	}

	go p.consumeSSEStream(ctx, resp.Body, partsCh, opts, stream)
	return stream, nil
}

// consumeSSEStream processes Anthropic's SSE event stream.
func (p *Provider) consumeSSEStream(
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

	var currentToolCallIndex int
	var argsBuffer string
	var finishEmitted bool

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// Anthropic SSE: "event: <type>" followed by "data: <json>"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil && event.Message.Usage != nil {
				usage := &kosong.TokenUsage{
					InputOther:         event.Message.Usage.InputTokens,
					Output:             event.Message.Usage.OutputTokens,
					InputCacheCreation: event.Message.Usage.CacheCreationTokens,
					InputCacheRead:     event.Message.Usage.CacheReadTokens,
				}
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "usage", Usage: usage}:
				case <-ctx.Done():
					return
				}
			}

		case "content_block_start":
			if event.ContentBlock == nil {
				continue
			}
			cb := event.ContentBlock
			switch cb.Type {
			case "tool_use":
				currentToolCallIndex = safeInt(event.Index)
				argsBuffer = ""
				select {
				case partsCh <- kosong.StreamedMessagePart{
					Type:  "function",
					ID:    cb.ID,
					Name:  cb.Name,
					Index: currentToolCallIndex,
				}:
				case <-ctx.Done():
					return
				}
			case "thinking":
				// Thinking block started; deltas will follow
			}

		case "content_block_delta":
			if event.Delta == nil {
				continue
			}
			d := event.Delta
			switch d.Type {
			case "text_delta":
				if d.Text != "" {
					select {
					case partsCh <- kosong.StreamedMessagePart{Type: "text", Text: d.Text}:
					case <-ctx.Done():
						return
					}
				}
			case "thinking_delta":
				if d.Thinking != "" {
					select {
					case partsCh <- kosong.StreamedMessagePart{Type: "think", Think: d.Thinking}:
					case <-ctx.Done():
						return
					}
				}
			case "signature_delta":
				// Encrypted reasoning signature — emit as think part with encrypted field
				if d.Signature != "" {
					sig := d.Signature
					select {
					case partsCh <- kosong.StreamedMessagePart{
						Type:      "think",
						Encrypted: &sig,
					}:
					case <-ctx.Done():
						return
					}
				}
			case "input_json_delta":
				if d.PartialJSON != "" {
					argsBuffer += d.PartialJSON
					args := d.PartialJSON
					select {
					case partsCh <- kosong.StreamedMessagePart{
						Type:          "tool_call_part",
						ArgumentsPart: &args,
						Index:         currentToolCallIndex,
					}:
					case <-ctx.Done():
						return
					}
				}
			}

		case "content_block_stop":
			// Nothing specific to do here; tool call args were streamed as deltas

		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != nil && !finishEmitted {
				finishEmitted = true
				reason := mapAnthropicFinishReason(event.Delta.StopReason)
				if stream != nil {
					stream.RawFinishReason = event.Delta.StopReason
					stream.FinishReason = &reason
				}
				select {
				case partsCh <- kosong.StreamedMessagePart{
					Type:         "finish",
					FinishReason: event.Delta.StopReason,
				}:
				case <-ctx.Done():
					return
				}
			}
			if event.Usage != nil {
				usage := &kosong.TokenUsage{
					Output: event.Usage.OutputTokens,
				}
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "usage", Usage: usage}:
				case <-ctx.Done():
					return
				}
			}

		case "message_stop":
			if opts != nil && opts.OnStreamEnd != nil {
				opts.OnStreamEnd(&kosong.StreamDecodeStats{})
			}
			return
		}
	}
}

// ── Message conversion ──

func convertMessage(msg kosong.Message) anthropicMessage {
	result := anthropicMessage{
		Role: string(msg.Role),
	}

	// Anthropic only supports "user" and "assistant" roles
	// Tool messages become "user" messages with tool_result content blocks
	switch msg.Role {
	case kosong.RoleUser, kosong.RoleSystem:
		result.Role = "user"
		result.Content = convertUserContent(msg.Content)

	case kosong.RoleAssistant:
		result.Role = "assistant"
		result.Content = convertAssistantContent(msg)

	case kosong.RoleTool:
		result.Role = "user"
		result.Content = convertToolResult(msg)
	}

	return result
}

func convertUserContent(parts []kosong.ContentPart) interface{} {
	if len(parts) == 0 {
		return ""
	}
	// Fast path: single text part
	if len(parts) == 1 && parts[0].Type == "text" {
		return parts[0].Text
	}

	var blocks []anthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "text":
			if p.Text != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: p.Text})
			}
		case "image_url":
			if p.ImageURL != nil {
				blocks = append(blocks, anthropicContentBlock{
					Type: "image",
					Source: &anthropicImageSource{
						Type: "url",
						URL:  p.ImageURL.URL,
					},
				})
			}
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return blocks
}

func convertAssistantContent(msg kosong.Message) interface{} {
	var blocks []anthropicContentBlock

	// Add thinking parts first
	for _, p := range msg.Content {
		if p.Type == "think" && p.Think != "" {
			block := anthropicContentBlock{Type: "thinking", Thinking: p.Think}
			if p.Encrypted != nil {
				block.Signature = *p.Encrypted
			}
			blocks = append(blocks, block)
		}
	}

	// Add text parts
	for _, p := range msg.Content {
		if p.Type == "text" && p.Text != "" {
			blocks = append(blocks, anthropicContentBlock{Type: "text", Text: p.Text})
		}
	}

	// Add tool_use blocks
	for _, tc := range msg.ToolCalls {
		var input interface{}
		if tc.Arguments != nil {
			// Parse JSON arguments
			var parsed interface{}
			if err := json.Unmarshal([]byte(*tc.Arguments), &parsed); err == nil {
				input = parsed
			} else {
				input = map[string]interface{}{}
			}
		} else {
			input = map[string]interface{}{}
		}
		blocks = append(blocks, anthropicContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}

	if len(blocks) == 0 {
		return ""
	}
	return blocks
}

func convertToolResult(msg kosong.Message) interface{} {
	toolUseID := ""
	if msg.ToolCallID != nil {
		toolUseID = *msg.ToolCallID
	}

	// Extract text content
	var content interface{}
	if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
		content = msg.Content[0].Text
	} else if len(msg.Content) > 0 {
		var blocks []anthropicContentBlock
		for _, p := range msg.Content {
			if p.Type == "text" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: p.Text})
			} else if p.Type == "image_url" && p.ImageURL != nil {
				blocks = append(blocks, anthropicContentBlock{
					Type: "image",
					Source: &anthropicImageSource{Type: "url", URL: p.ImageURL.URL},
				})
			}
		}
		if len(blocks) > 0 {
			content = blocks
		}
	}

	return []anthropicContentBlock{
		{
			Type:      "tool_result",
			ToolUseID: toolUseID,
			Content:   content,
		},
	}
}

func convertTool(t kosong.Tool) anthropicTool {
	schema := t.Parameters
	if schema == nil {
		schema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	return anthropicTool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: schema,
	}
}

// mapAnthropicFinishReason maps Anthropic stop reasons to kosong finish reasons.
func mapAnthropicFinishReason(raw *string) kosong.FinishReason {
	if raw == nil {
		return kosong.FinishOther
	}
	switch *raw {
	case "end_turn", "stop_sequence":
		return kosong.FinishCompleted
	case "tool_use":
		return kosong.FinishToolCalls
	case "max_tokens":
		return kosong.FinishTruncated
	case "pause_turn":
		return kosong.FinishPaused
	default:
		return kosong.FinishOther
	}
}

func safeInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
