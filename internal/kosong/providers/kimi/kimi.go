// Package kimi implements the Kimi (Moonshot) provider adapter.
// It extends the generic OpenAI-compatible chat completions format with
// Kimi-specific features: reasoning key dialect detection, tool schema
// normalization, and thinking config.
package kimi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/trace"
)

// ── Reasoning key dialect (self-contained) ──

var knownReasoningKeys = []string{"reasoning_content", "reasoning_details", "reasoning"}
const defaultReasoningKey = "reasoning_content"

type reasoningKeyDialect struct {
	mu          sync.RWMutex
	explicitKey string
	detected    string
}

func newReasoningKeyDialect(explicitKey string) *reasoningKeyDialect {
	return &reasoningKeyDialect{explicitKey: explicitKey}
}

func (d *reasoningKeyDialect) observe(source map[string]interface{}) (string, bool) {
	for _, key := range knownReasoningKeys {
		if v, ok := source[key]; ok {
			if s, isStr := v.(string); isStr && s != "" {
				d.mu.Lock()
				d.detected = key
				d.mu.Unlock()
				return s, true
			}
		}
	}
	return "", false
}

func (d *reasoningKeyDialect) outboundKey() string {
	if d.explicitKey != "" {
		return d.explicitKey
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.detected != "" {
		return d.detected
	}
	return defaultReasoningKey
}

// ── Schema normalization (self-contained) ──

func normalizeKimiSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return schema
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return schema
	}

	defs := extractDefs(result)
	if defs != nil {
		if resolved, ok := resolveRefs(result, defs, 0).(map[string]interface{}); ok {
			result = resolved
		}
	}
	if completed, ok := completeTypes(result).(map[string]interface{}); ok {
		result = completed
	}
	if stripped, ok := stripMetaFields(result).(map[string]interface{}); ok {
		result = stripped
	}
	return result
}

func extractDefs(schema map[string]interface{}) map[string]interface{} {
	if d, ok := schema["$defs"]; ok {
		if m, ok := d.(map[string]interface{}); ok {
			return m
		}
	}
	if d, ok := schema["definitions"]; ok {
		if m, ok := d.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

func resolveRefs(node interface{}, defs map[string]interface{}, depth int) interface{} {
	if depth > 20 {
		return node
	}
	switch v := node.(type) {
	case map[string]interface{}:
		if ref, ok := v["$ref"]; ok {
			if refStr, ok := ref.(string); ok {
				resolved := resolveSingleRef(refStr, defs)
				if resolved != nil {
					merged := copyMap(resolved)
					for k, val := range v {
						if k == "$ref" {
							continue
						}
						merged[k] = resolveRefs(val, defs, depth+1)
					}
					return merged
				}
			}
		}
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = resolveRefs(val, defs, depth+1)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = resolveRefs(item, defs, depth+1)
		}
		return result
	default:
		return node
	}
}

func resolveSingleRef(ref string, defs map[string]interface{}) map[string]interface{} {
	parts := strings.Split(ref, "/")
	if len(parts) < 3 {
		return nil
	}
	name := parts[len(parts)-1]
	if def, ok := defs[name]; ok {
		if m, ok := def.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

func completeTypes(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = completeTypes(val)
		}
		if _, hasType := result["type"]; !hasType {
			if _, hasProps := result["properties"]; hasProps {
				result["type"] = "object"
			} else if _, hasItems := result["items"]; hasItems {
				result["type"] = "array"
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = completeTypes(item)
		}
		return result
	default:
		return node
	}
}

func stripMetaFields(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			if k == "$schema" || k == "$id" {
				continue
			}
			result[k] = stripMetaFields(val)
		}
		if req, ok := result["required"]; ok {
			if arr, ok := req.([]interface{}); ok && len(arr) == 0 {
				delete(result, "required")
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = stripMetaFields(item)
		}
		return result
	default:
		return node
	}
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result
}

// ── Wire types (OpenAI-compatible chat completions) ──

type wireRequest struct {
	Model         string          `json:"model"`
	Messages      []wireMessage   `json:"messages"`
	Tools         []wireTool      `json:"tools,omitempty"`
	Stream        bool            `json:"stream"`
	Temperature   *float64        `json:"temperature,omitempty"`
	MaxTokens     *int            `json:"max_tokens,omitempty"`
	StreamOptions *wireStreamOpts `json:"stream_options,omitempty"`
}

type wireStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    interface{}    `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID *string        `json:"tool_call_id,omitempty"`
	Name       *string        `json:"name,omitempty"`
}

type wireContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *wireMediaURL `json:"image_url,omitempty"`
}

type wireMediaURL struct {
	URL string `json:"url"`
}

type wireToolCall struct {
	Index    *int             `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireFunctionCall struct {
	Name      string  `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type wireResponseChunk struct {
	ID      string           `json:"id"`
	Choices []wireChoiceChunk `json:"choices"`
	Usage   *wireUsage       `json:"usage,omitempty"`
}

type wireChoiceChunk struct {
	Index        int              `json:"index"`
	Delta        *wireChoiceDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

type wireChoiceDelta struct {
	Role             string         `json:"role"`
	Content          *string        `json:"content"`
	ReasoningContent string         `json:"reasoning_content"`
	ToolCalls        []wireToolCall `json:"tool_calls"`
}

type wireUsage struct {
	PromptTokens      int                  `json:"prompt_tokens"`
	CompletionTokens  int                  `json:"completion_tokens"`
	PromptDetails     *wireUsageDetail     `json:"prompt_tokens_details,omitempty"`
	CompletionDetails *wireCompletionDetail `json:"completion_tokens_details,omitempty"`
}

type wireUsageDetail struct {
	CachedTokens int `json:"cached_tokens"`
}

type wireCompletionDetail struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ── Provider ──

// ThinkingConfig controls Kimi's thinking/reasoning behavior.
type ThinkingConfig struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// ProviderConfig holds Kimi-specific configuration.
type ProviderConfig struct {
	APIKey         string
	BaseURL        string
	Model          string
	ThinkingEffort kosong.ThinkingEffort
	MaxTokens      int
	Temperature    *float64
	Thinking       *ThinkingConfig
	PromptCacheKey string
	DefaultHeaders map[string]string
	HTTPClient     *http.Client
}

// Provider is a Kimi-specific ChatProvider.
type Provider struct {
	apiKey         string
	baseURL        string
	model          string
	thinkingEffort kosong.ThinkingEffort
	maxTokens      int
	temperature    *float64
	thinking       *ThinkingConfig
	promptCacheKey string
	defaultHeaders map[string]string
	httpClient     *http.Client
	reasoning      *reasoningKeyDialect
}

// NewProvider creates a Kimi provider.
func NewProvider(cfg ProviderConfig) *Provider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.moonshot.cn/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !hasVersionPath(baseURL) {
		baseURL += "/v1"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Provider{
		apiKey:         cfg.APIKey,
		baseURL:        baseURL,
		model:          cfg.Model,
		thinkingEffort: cfg.ThinkingEffort,
		maxTokens:      cfg.MaxTokens,
		temperature:    cfg.Temperature,
		thinking:       cfg.Thinking,
		promptCacheKey: cfg.PromptCacheKey,
		defaultHeaders: cfg.DefaultHeaders,
		httpClient:     httpClient,
		reasoning:      newReasoningKeyDialect(""),
	}
}

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

func (p *Provider) Name() string                         { return "kimi" }
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
	return nil, fmt.Errorf("video upload not supported by kimi provider")
}

// Generate sends a chat completion request with Kimi-specific enhancements.
func (p *Provider) Generate(
	ctx context.Context,
	systemPrompt string,
	tools []kosong.Tool,
	history []kosong.Message,
	opts *kosong.GenerateOptions,
) (*kosong.StreamedMessage, error) {
	var wireMessages []wireMessage
	if systemPrompt != "" {
		wireMessages = append(wireMessages, wireMessage{Role: "system", Content: systemPrompt})
	}
	for _, msg := range history {
		wireMessages = append(wireMessages, convertMessage(msg))
	}

	var wireTools []wireTool
	for _, t := range tools {
		if t.Deferred {
			continue
		}
		params := t.Parameters
		if params != nil {
			params = normalizeKimiSchema(params)
		}
		wireTools = append(wireTools, wireTool{
			Type:     "function",
			Function: wireFunction{Name: t.Name, Description: t.Description, Parameters: params},
		})
	}

	reqBody := wireRequest{
		Model:         p.model,
		Messages:      wireMessages,
		Tools:         wireTools,
		Stream:        true,
		Temperature:   p.temperature,
		StreamOptions: &wireStreamOpts{IncludeUsage: true},
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

	reqURL := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
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
		trace.Log("http", "request", map[string]any{"url": reqURL, "model": p.model})
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
	stream := &kosong.StreamedMessage{Parts: partsCh, TraceID: traceID}

	go p.consumeSSEStream(ctx, resp.Body, partsCh, opts, stream)
	return stream, nil
}

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

	var finishEmitted bool

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
			if opts != nil && opts.OnStreamEnd != nil {
				opts.OnStreamEnd(&kosong.StreamDecodeStats{})
			}
			return
		}

		var chunk wireResponseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

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

		for _, choice := range chunk.Choices {
			delta := choice.Delta
			if delta == nil {
				continue
			}

			if delta.Content != nil && *delta.Content != "" {
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "text", Text: *delta.Content}:
				case <-ctx.Done():
					return
				}
			}

			if delta.ReasoningContent != "" {
				select {
				case partsCh <- kosong.StreamedMessagePart{Type: "think", Think: delta.ReasoningContent}:
				case <-ctx.Done():
					return
				}
			}

			for _, tc := range delta.ToolCalls {
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

		if !finishEmitted {
			for _, choice := range chunk.Choices {
				if choice.FinishReason != nil {
					finishEmitted = true
					if stream != nil {
						stream.RawFinishReason = choice.FinishReason
						reason := mapFinishReason(choice.FinishReason)
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
					break
				}
			}
		}
	}
}

// ── Message conversion ──

func convertMessage(msg kosong.Message) wireMessage {
	result := wireMessage{Role: string(msg.Role), Name: msg.Name}
	switch msg.Role {
	case kosong.RoleUser, kosong.RoleSystem:
		result.Content = convertContent(msg.Content)
	case kosong.RoleAssistant:
		result.Content = convertContent(msg.Content)
		for i, tc := range msg.ToolCalls {
			idx := i
			result.ToolCalls = append(result.ToolCalls, wireToolCall{
				Index: &idx, ID: tc.ID, Type: "function",
				Function: wireFunctionCall{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
	case kosong.RoleTool:
		result.Content = convertContent(msg.Content)
		result.ToolCallID = msg.ToolCallID
	}
	return result
}

func convertContent(parts []kosong.ContentPart) interface{} {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 && parts[0].Type == "text" {
		return parts[0].Text
	}
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
	var result []wireContentPart
	for _, p := range parts {
		switch p.Type {
		case "text":
			result = append(result, wireContentPart{Type: "text", Text: p.Text})
		case "image_url":
			if p.ImageURL != nil {
				result = append(result, wireContentPart{Type: "image_url", ImageURL: &wireMediaURL{URL: p.ImageURL.URL}})
			}
		}
	}
	return result
}

func mapFinishReason(raw *string) kosong.FinishReason {
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
