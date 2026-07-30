// Package google implements the Google GenAI (Gemini/Vertex) provider adapter.
// It handles the native Gemini content format, function calling, multimodal
// support, and thinking/reasoning for Gemini models.
package google

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/visdomtech/kimi-code/internal/kosong"
	"github.com/visdomtech/kimi-code/internal/trace"
)

// ── Google GenAI wire types ──

type genaiRequest struct {
	Contents          []genaiContent         `json:"contents"`
	Tools             []genaiTool            `json:"tools,omitempty"`
	SystemInstruction *genaiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *genaiGenerationConfig `json:"generationConfig,omitempty"`
}

type genaiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	TopK            *int     `json:"topK,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type genaiContent struct {
	Role  string     `json:"role"` // "user", "model"
	Parts []genaiPart `json:"parts"`
}

type genaiPart struct {
	Text             string              `json:"text,omitempty"`
	InlineData       *genaiBlob          `json:"inlineData,omitempty"`
	FileData         *genaiFileData      `json:"fileData,omitempty"`
	FunctionCall     *genaiFunctionCall  `json:"functionCall,omitempty"`
	FunctionResponse *genaiFunctionResp  `json:"functionResponse,omitempty"`
	Thought          *bool               `json:"thought,omitempty"` // Gemini thinking
}

type genaiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64
}

type genaiFileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

type genaiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type genaiFunctionResp struct {
	Name     string      `json:"name"`
	Response interface{} `json:"response"`
}

type genaiTool struct {
	FunctionDeclarations []genaiFunctionDecl `json:"functionDeclarations,omitempty"`
}

type genaiFunctionDecl struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type genaiResponse struct {
	Candidates []genaiCandidate `json:"candidates"`
	UsageMetadata *genaiUsage   `json:"usageMetadata,omitempty"`
	Error      *genaiErrorBody  `json:"error,omitempty"`
}

type genaiCandidate struct {
	Content      genaiContent `json:"content"`
	FinishReason *string      `json:"finishReason,omitempty"`
	Index        int          `json:"index"`
}

type genaiUsage struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
}

type genaiErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ── Provider ──

// Config holds Google GenAI provider configuration.
type Config struct {
	APIKey         string
	BaseURL        string // default: https://generativelanguage.googleapis.com/v1beta
	Model          string
	ThinkingEffort kosong.ThinkingEffort
	MaxTokens      int
	Temperature    *float64
	DefaultHeaders map[string]string
	HTTPClient     *http.Client
}

// Provider implements kosong.ChatProvider for Google's GenAI API.
type Provider struct {
	apiKey         string
	baseURL        string
	model          string
	thinkingEffort kosong.ThinkingEffort
	maxTokens      int
	temperature    *float64
	defaultHeaders map[string]string
	httpClient     *http.Client
}

// NewProvider creates a new Google GenAI provider.
func NewProvider(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
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
	}
}

func (p *Provider) Name() string                         { return "google-genai" }
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
	return nil, fmt.Errorf("video upload not supported by google-genai provider")
}

// Generate sends a streaming generateContent request to Google GenAI.
func (p *Provider) Generate(
	ctx context.Context,
	systemPrompt string,
	tools []kosong.Tool,
	history []kosong.Message,
	opts *kosong.GenerateOptions,
) (*kosong.StreamedMessage, error) {
	// Build contents
	var contents []genaiContent
	for _, msg := range history {
		contents = append(contents, convertMessage(msg))
	}

	// Build tools
	var wireTools []genaiTool
	var fnDecls []genaiFunctionDecl
	for _, t := range tools {
		if t.Deferred {
			continue
		}
		fnDecls = append(fnDecls, convertToolDecl(t))
	}
	if len(fnDecls) > 0 {
		wireTools = []genaiTool{{FunctionDeclarations: fnDecls}}
	}

	// Build generation config
	genConfig := &genaiGenerationConfig{}
	if p.temperature != nil {
		genConfig.Temperature = p.temperature
	}
	if p.maxTokens > 0 {
		genConfig.MaxOutputTokens = p.maxTokens
	}

	reqBody := genaiRequest{
		Contents:         contents,
		Tools:            wireTools,
		GenerationConfig: genConfig,
	}

	// System instruction
	if systemPrompt != "" {
		reqBody.SystemInstruction = &genaiContent{
			Role: "system",
			Parts: []genaiPart{{Text: systemPrompt}},
		}
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

	// Use streaming endpoint with SSE.
	// API key is passed via header (x-goog-api-key) to avoid leaking it in URLs/logs.
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", p.baseURL, p.model)

	// Determine the API key to use (request-scoped override or provider default).
	apiKey := p.apiKey
	if opts != nil && opts.Auth != nil && opts.Auth.APIKey != nil {
		apiKey = *opts.Auth.APIKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		req.Header.Set("x-goog-api-key", apiKey)
	}
	for k, v := range p.defaultHeaders {
		req.Header.Set(k, v)
	}
	if opts != nil && opts.Auth != nil {
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
		return nil, kosong.ClassifyBaseAPIError(fmt.Sprintf("request failed: %s", err))
	}

	if trace.Enabled() {
		trace.Log("http", "response", map[string]any{"status": resp.StatusCode, "model": p.model})
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, kosong.NormalizeAPIStatusError(resp.StatusCode, string(body), nil, nil, nil)
	}

	partsCh := make(chan kosong.StreamedMessagePart, 64)
	stream := &kosong.StreamedMessage{Parts: partsCh}

	go p.consumeSSEStream(ctx, resp.Body, partsCh, opts, stream)
	return stream, nil
}

// consumeSSEStream processes Google GenAI SSE events.
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
	var callSeq atomic.Int64

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

		var response genaiResponse
		if err := json.Unmarshal([]byte(data), &response); err != nil {
			continue
		}

		// Process usage
		if response.UsageMetadata != nil {
			u := response.UsageMetadata
			usage := &kosong.TokenUsage{
				InputOther:      u.PromptTokenCount,
				Output:          u.CandidatesTokenCount,
				InputCacheRead:  u.CachedContentTokenCount,
				ReasoningTokens: u.ThoughtsTokenCount,
			}
			select {
			case partsCh <- kosong.StreamedMessagePart{Type: "usage", Usage: usage}:
			case <-ctx.Done():
				return
			}
		}

		// Process candidates
		for _, candidate := range response.Candidates {
			for _, part := range candidate.Content.Parts {
				// Check if this is a thinking part
				if part.Thought != nil && *part.Thought {
					if part.Text != "" {
						select {
						case partsCh <- kosong.StreamedMessagePart{Type: "think", Think: part.Text}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				// Text content
				if part.Text != "" {
					select {
					case partsCh <- kosong.StreamedMessagePart{Type: "text", Text: part.Text}:
					case <-ctx.Done():
						return
					}
				}

				// Function call
				if part.FunctionCall != nil {
					fc := part.FunctionCall
					argsJSON, _ := json.Marshal(fc.Args)
					argsStr := string(argsJSON)
					seq := callSeq.Add(1)
					select {
					case partsCh <- kosong.StreamedMessagePart{
						Type:      "function",
						ID:        fmt.Sprintf("call_%d_%s_%d", seq, fc.Name, candidate.Index),
						Name:      fc.Name,
						Arguments: &argsStr,
					}:
					case <-ctx.Done():
						return
					}
				}
			}

			// Handle finish reason
			if candidate.FinishReason != nil && !finishEmitted {
				finishEmitted = true
				reason := mapGenAIFinishReason(candidate.FinishReason)
				if stream != nil {
					stream.RawFinishReason = candidate.FinishReason
					stream.FinishReason = &reason
				}
				select {
				case partsCh <- kosong.StreamedMessagePart{
					Type:         "finish",
					FinishReason: candidate.FinishReason,
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}

	if opts != nil && opts.OnStreamEnd != nil {
		opts.OnStreamEnd(&kosong.StreamDecodeStats{})
	}
}

// ── Message conversion ──

func convertMessage(msg kosong.Message) genaiContent {
	result := genaiContent{}

	switch msg.Role {
	case kosong.RoleUser, kosong.RoleSystem:
		result.Role = "user"
	case kosong.RoleAssistant:
		result.Role = "model"
	case kosong.RoleTool:
		result.Role = "user" // function response comes as user
		return convertToolResponse(msg)
	}

	for _, p := range msg.Content {
		switch p.Type {
		case "text":
			if p.Text != "" {
				result.Parts = append(result.Parts, genaiPart{Text: p.Text})
			}
		case "think":
			if p.Think != "" {
				thought := true
				result.Parts = append(result.Parts, genaiPart{Text: p.Think, Thought: &thought})
			}
		case "image_url":
			if p.ImageURL != nil {
				result.Parts = append(result.Parts, genaiPart{
					FileData: &genaiFileData{
						MimeType: "image/*",
						FileURI:  p.ImageURL.URL,
					},
				})
			}
		}
	}

	// Add function calls from tool calls
	for _, tc := range msg.ToolCalls {
		var args map[string]interface{}
		if tc.Arguments != nil {
			json.Unmarshal([]byte(*tc.Arguments), &args)
		}
		result.Parts = append(result.Parts, genaiPart{
			FunctionCall: &genaiFunctionCall{
				Name: tc.Name,
				Args: args,
			},
		})
	}

	return result
}

func convertToolResponse(msg kosong.Message) genaiContent {
	var response interface{}
	if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
		response = map[string]interface{}{"result": msg.Content[0].Text}
	} else {
		var texts []string
		for _, p := range msg.Content {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		response = map[string]interface{}{"result": strings.Join(texts, "\n")}
	}

	name := ""
	if msg.ToolCallID != nil {
		name = *msg.ToolCallID
	}

	return genaiContent{
		Role: "user",
		Parts: []genaiPart{
			{
				FunctionResponse: &genaiFunctionResp{
					Name:     name,
					Response: response,
				},
			},
		},
	}
}

func convertToolDecl(t kosong.Tool) genaiFunctionDecl {
	schema := t.Parameters
	if schema != nil {
		// Remove "type": "object" from root as Gemini doesn't need it
		schema = cleanSchemaForGemini(schema)
	}
	return genaiFunctionDecl{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  schema,
	}
}

// cleanSchemaForGemini removes fields that Gemini doesn't support.
func cleanSchemaForGemini(schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		// Gemini doesn't support $schema, $id, additionalProperties in some contexts
		if k == "$schema" || k == "$id" {
			continue
		}
		if subMap, ok := v.(map[string]interface{}); ok {
			result[k] = cleanSchemaForGemini(subMap)
		} else {
			result[k] = v
		}
	}
	return result
}

// mapGenAIFinishReason maps Gemini finish reasons to kosong finish reasons.
func mapGenAIFinishReason(raw *string) kosong.FinishReason {
	if raw == nil {
		return kosong.FinishOther
	}
	switch *raw {
	case "STOP":
		return kosong.FinishCompleted
	case "MAX_TOKENS":
		return kosong.FinishTruncated
	case "SAFETY", "RECITATION":
		return kosong.FinishFiltered
	default:
		return kosong.FinishOther
	}
}
