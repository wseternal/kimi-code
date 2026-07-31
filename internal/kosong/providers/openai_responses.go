// Package providers implements LLM provider adapters.
// openai_responses.go implements the OpenAI Responses API adapter (/responses endpoint).
// This is distinct from Chat Completions and uses a different data model:
// items instead of messages, function_call items, developer role, etc.
package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/trace"
)

// ── Responses API wire types ──

type responsesRequest struct {
	Model        string            `json:"model"`
	Input        interface{}       `json:"input"` // string | []responsesInputItem
	Instructions string            `json:"instructions,omitempty"`
	Tools        []responsesTool   `json:"tools,omitempty"`
	Stream       bool              `json:"stream"`
	MaxTokens    *int              `json:"max_output_tokens,omitempty"`
	Temperature  *float64          `json:"temperature,omitempty"`
}

type responsesInputItem struct {
	Type    string      `json:"type"` // "message", "function_call", "function_call_output"
	Role    string      `json:"role,omitempty"`
	Content interface{} `json:"content,omitempty"` // string | []responsesContentPart
	// Function call item
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// Function call output item
	Output string `json:"output,omitempty"`
}

type responsesContentPart struct {
	Type string `json:"type"` // "input_text", "output_text"
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string                 `json:"type"` // "function"
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type responsesResponse struct {
	ID          string              `json:"id"`
	Object      string              `json:"object"`
	Status      string              `json:"status"` // "completed", "in_progress", "incomplete", "failed"
	Output      []responsesOutput   `json:"output"`
	Usage       *responsesUsage     `json:"usage,omitempty"`
	Error       *responsesErrorBody `json:"error,omitempty"`
	Model       string              `json:"model"`
}

type responsesOutput struct {
	Type    string `json:"type"` // "message", "function_call", "reasoning"
	Role    string `json:"role,omitempty"`
	Content []responsesOutputContent `json:"content,omitempty"`
	// Function call fields
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// Reasoning fields
	Summary []responsesReasoningSummary `json:"summary,omitempty"`
}

type responsesOutputContent struct {
	Type string `json:"type"` // "output_text", "refusal"
	Text string `json:"text,omitempty"`
}

type responsesReasoningSummary struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
}

type responsesUsage struct {
	InputTokens        int                   `json:"input_tokens"`
	OutputTokens       int                   `json:"output_tokens"`
	InputTokensDetails *responsesInputDetail `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *responsesOutputDetail `json:"output_tokens_details,omitempty"`
}

type responsesInputDetail struct {
	CachedTokens int `json:"cached_tokens"`
}

type responsesOutputDetail struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type responsesErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SSE streaming event types for Responses API
// NOTE: In the real OpenAI Responses API streaming format, the "delta" field
// is a plain string (e.g., {"type":"response.output_text.delta","delta":"Hello"}),
// NOT a nested object. This differs from Chat Completions where delta is an object.
type responsesStreamEvent struct {
	Type         string                  `json:"type"`
	Response     *responsesResponse      `json:"response,omitempty"`
	Item         *responsesOutput        `json:"item,omitempty"`
	Delta        string                  `json:"delta,omitempty"`
	Part         *responsesOutputContent `json:"part,omitempty"`
	OutputIndex  *int                    `json:"output_index,omitempty"`
	ContentIndex *int                    `json:"content_index,omitempty"`
}

// ── OpenAIResponsesProvider ──

// OpenAIResponsesConfig holds configuration for the Responses API provider.
type OpenAIResponsesConfig struct {
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

// OpenAIResponsesProvider implements kosong.ChatProvider for OpenAI's /responses endpoint.
type OpenAIResponsesProvider struct {
	name           string
	apiKey         string
	baseURL        string
	model          string
	thinkingEffort kosong.ThinkingEffort
	maxTokens      int
	temperature    *float64
	defaultHeaders map[string]string
	httpClient     *http.Client
	toolCallPolicy ToolCallIDPolicy
}

// NewOpenAIResponsesProvider creates a new Responses API provider.
func NewOpenAIResponsesProvider(cfg OpenAIResponsesConfig) *OpenAIResponsesProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Name == "" {
		cfg.Name = "openai_responses"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAIResponsesProvider{
		name:           cfg.Name,
		apiKey:         cfg.APIKey,
		baseURL:        cfg.BaseURL,
		model:          cfg.Model,
		thinkingEffort: cfg.ThinkingEffort,
		maxTokens:      cfg.MaxTokens,
		temperature:    cfg.Temperature,
		defaultHeaders: cfg.DefaultHeaders,
		httpClient:     httpClient,
		toolCallPolicy: DefaultOpenAIResponsesPolicy(),
	}
}

func (p *OpenAIResponsesProvider) Name() string                         { return p.name }
func (p *OpenAIResponsesProvider) ModelName() string                     { return p.model }
func (p *OpenAIResponsesProvider) ThinkingEffort() kosong.ThinkingEffort { return p.thinkingEffort }
func (p *OpenAIResponsesProvider) MaxCompletionTokens() int              { return p.maxTokens }

func (p *OpenAIResponsesProvider) WithThinking(effort kosong.ThinkingEffort) kosong.ChatProvider {
	cp := *p
	cp.thinkingEffort = effort
	return &cp
}

func (p *OpenAIResponsesProvider) WithMaxCompletionTokens(maxTokens int, _ *kosong.MaxCompletionTokensOptions) kosong.ChatProvider {
	cp := *p
	cp.maxTokens = maxTokens
	return &cp
}

func (p *OpenAIResponsesProvider) UploadVideo(_ context.Context, _ interface{}, _ *kosong.GenerateOptions) (*kosong.VideoURLPart, error) {
	return nil, fmt.Errorf("video upload not supported by %s provider", p.name)
}

// Generate sends a request to the Responses API and returns a streamed response.
func (p *OpenAIResponsesProvider) Generate(
	ctx context.Context,
	systemPrompt string,
	tools []kosong.Tool,
	history []kosong.Message,
	opts *kosong.GenerateOptions,
) (*kosong.StreamedMessage, error) {
	// Normalize tool call IDs before sending
	normalizedHistory := NormalizeToolCallIdsForProvider(history, p.toolCallPolicy)

	// Convert messages to input items
	input := p.buildInput(systemPrompt, normalizedHistory)

	// Convert tools
	var wireTools []responsesTool
	for _, t := range tools {
		if t.Deferred {
			continue
		}
		wireTools = append(wireTools, responsesTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}

	reqBody := responsesRequest{
		Model:  p.model,
		Input:  input,
		Tools:  wireTools,
		Stream: true,
		Temperature: p.temperature,
	}
	if systemPrompt != "" {
		reqBody.Instructions = systemPrompt
	}
	if p.maxTokens > 0 {
		reqBody.MaxTokens = &p.maxTokens
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

	url := p.baseURL + "/responses"
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

	if trace.Enabled() {
		trace.Log("http", "request", map[string]any{"url": url, "model": p.model})
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if opts != nil && opts.OnRawResponse != nil {
			if f, ferr := os.CreateTemp("", "kimi-raw-resp-*.jsonl"); ferr == nil {
				fmt.Fprintf(f, "network_error: %s\n", err.Error())
				f.Close()
				opts.OnRawResponse(f.Name())
			}
		}
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

// buildInput converts system prompt + messages into Responses API input format.
func (p *OpenAIResponsesProvider) buildInput(systemPrompt string, history []kosong.Message) interface{} {
	if len(history) == 0 && systemPrompt == "" {
		return ""
	}

	var items []responsesInputItem

	for _, msg := range history {
		switch msg.Role {
		case kosong.RoleUser, kosong.RoleSystem:
			items = append(items, responsesInputItem{
				Type:    "message",
				Role:    string(msg.Role),
				Content: buildContentParts(msg.Content),
			})

		case kosong.RoleAssistant:
			// Emit message content if any
			if len(msg.Content) > 0 {
				items = append(items, responsesInputItem{
					Type:    "message",
					Role:    "assistant",
					Content: buildContentParts(msg.Content),
				})
			}
			// Emit function_call items for tool calls
			for _, tc := range msg.ToolCalls {
				args := ""
				if tc.Arguments != nil {
					args = *tc.Arguments
				}
				items = append(items, responsesInputItem{
					Type:      "function_call",
					CallID:    tc.ID,
					Name:      tc.Name,
					Arguments: args,
				})
			}

		case kosong.RoleTool:
			output := ""
			if len(msg.Content) > 0 {
				output = kosong.ExtractText(&msg, "\n")
			}
			callID := ""
			if msg.ToolCallID != nil {
				callID = *msg.ToolCallID
			}
			items = append(items, responsesInputItem{
				Type:   "function_call_output",
				CallID: callID,
				Output: output,
			})
		}
	}

	if len(items) == 0 {
		return ""
	}
	return items
}

func buildContentParts(parts []kosong.ContentPart) []responsesContentPart {
	var result []responsesContentPart
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			result = append(result, responsesContentPart{Type: "input_text", Text: p.Text})
		}
	}
	return result
}

// consumeSSEStream processes the Responses API SSE event stream.
func (p *OpenAIResponsesProvider) consumeSSEStream(
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

	var finishEmitted bool
	var currentFunctionName string
	var currentFunctionID string
	var currentFunctionIndex int

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return
		}

		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "response.created":
			// Response started

		case "response.completed":
			if event.Response != nil && event.Response.Usage != nil {
				u := event.Response.Usage
				usage := &kosong.TokenUsage{
					InputOther: u.InputTokens,
					Output:     u.OutputTokens,
				}
				if u.InputTokensDetails != nil {
					usage.InputCacheRead = u.InputTokensDetails.CachedTokens
					usage.InputOther -= usage.InputCacheRead
				}
				if u.OutputTokensDetails != nil {
					usage.ReasoningTokens = u.OutputTokensDetails.ReasoningTokens
				}
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "usage", Usage: usage}:
				case <-ctx.Done():
					return
				}
			}

		case "response.output_item.added":
			if event.Item != nil {
				switch event.Item.Type {
				case "function_call":
					currentFunctionName = event.Item.Name
					currentFunctionID = event.Item.CallID
					if event.OutputIndex != nil {
						currentFunctionIndex = *event.OutputIndex
					}
					select {
					case partsCh <- kosong.StreamedMessagePart{
						Type:  "function",
						ID:    currentFunctionID,
						Name:  currentFunctionName,
						Index: currentFunctionIndex,
					}:
					case <-ctx.Done():
						return
					}
				case "reasoning":
					// Reasoning item started
				}
			}

		case "response.output_text.delta":
			if event.Delta != "" {
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "text", Text: event.Delta}:
				case <-ctx.Done():
					return
				}
			}

		case "response.function_call_arguments.delta":
			if event.Delta != "" {
				args := event.Delta
				select {
				case partsCh <- kosong.StreamedMessagePart{
					Type:          "tool_call_part",
					ArgumentsPart: &args,
					Index:         currentFunctionIndex,
				}:
				case <-ctx.Done():
					return
				}
			}

		case "response.reasoning_summary_text.delta":
			if event.Delta != "" {
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "think", Think: event.Delta}:
				case <-ctx.Done():
					return
				}
			}

		case "response.output_item.done":
			if event.Item != nil && !finishEmitted {
				// Check if response is complete
				if event.Item.Type == "message" {
					// Message item completed - might indicate end
				}
			}

		case "response.failed", "response.incomplete":
			if !finishEmitted {
				finishEmitted = true
				reason := "failed"
				if stream != nil {
					stream.RawFinishReason = &reason
				}
			}
		}
	}

	// Emit a finish if we haven't already
	if !finishEmitted {
		finishEmitted = true
		stop := "stop"
		reason := MapFinishReason(&stop)
		if stream != nil {
			stream.RawFinishReason = &stop
			stream.FinishReason = &reason
		}
		select {
		case partsCh <- kosong.StreamedMessagePart{
			Type:         "finish",
			FinishReason: &stop,
		}:
		case <-ctx.Done():
			return
		}
	}

	if opts != nil && opts.OnStreamEnd != nil {
		opts.OnStreamEnd(&kosong.StreamDecodeStats{})
	}
}
