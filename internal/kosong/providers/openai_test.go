package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

// ── Message conversion tests ──

func TestConvertUserMessage(t *testing.T) {
	msg := kosong.CreateUserMessage("Hello world")
	result := convertMessage(msg)

	if result.Role != "user" {
		t.Errorf("expected role 'user', got %q", result.Role)
	}
	if result.Content != "Hello world" {
		t.Errorf("expected content 'Hello world', got %v", result.Content)
	}
}

func TestConvertAssistantMessageWithToolCalls(t *testing.T) {
	args := `{"path":"/tmp/file"}`
	msg := kosong.CreateAssistantMessage(
		[]kosong.ContentPart{kosong.NewTextPart("Let me read that file")},
		[]kosong.ToolCall{{Type: "function", ID: "call_1", Name: "read", Arguments: &args}},
	)
	result := convertMessage(msg)

	if result.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", result.Role)
	}
	if result.Content != "Let me read that file" {
		t.Errorf("expected text content, got %v", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("expected tool call ID 'call_1', got %q", tc.ID)
	}
	if tc.Function.Name != "read" {
		t.Errorf("expected function name 'read', got %q", tc.Function.Name)
	}
	if tc.Function.Arguments == nil || *tc.Function.Arguments != args {
		t.Errorf("expected arguments %q, got %v", args, tc.Function.Arguments)
	}
}

func TestConvertToolMessage(t *testing.T) {
	msg := kosong.CreateToolMessage("call_1", "file contents here")
	result := convertMessage(msg)

	if result.Role != "tool" {
		t.Errorf("expected role 'tool', got %q", result.Role)
	}
	if result.ToolCallID == nil || *result.ToolCallID != "call_1" {
		t.Errorf("expected tool_call_id 'call_1', got %v", result.ToolCallID)
	}
	if result.Content != "file contents here" {
		t.Errorf("expected content 'file contents here', got %v", result.Content)
	}
}

func TestConvertMultimodalContent(t *testing.T) {
	msg := kosong.Message{
		Role: kosong.RoleUser,
		Content: []kosong.ContentPart{
			kosong.NewTextPart("Describe this image"),
			kosong.NewImageURLPart("https://example.com/img.png", nil),
		},
	}
	result := convertMessage(msg)

	parts, ok := result.Content.([]openAIContentPart)
	if !ok {
		t.Fatalf("expected []openAIContentPart, got %T", result.Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "Describe this image" {
		t.Errorf("first part should be text")
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil || parts[1].ImageURL.URL != "https://example.com/img.png" {
		t.Errorf("second part should be image_url")
	}
}

func TestConvertThinkPartSkipped(t *testing.T) {
	msg := kosong.Message{
		Role: kosong.RoleUser,
		Content: []kosong.ContentPart{
			kosong.NewTextPart("hello"),
			kosong.NewThinkPart("internal reasoning"),
		},
	}
	result := convertMessage(msg)

	// When only text+think parts exist, it concatenates text and returns a string
	// Think parts are not sent in the request content
	text, ok := result.Content.(string)
	if !ok {
		t.Fatalf("expected string content (text-only), got %T", result.Content)
	}
	if text != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}
}

// ── Tool conversion tests ──

func TestConvertTool(t *testing.T) {
	tool := kosong.Tool{
		Name:        "bash",
		Description: "Execute a shell command",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The command to execute",
				},
			},
			"required": []string{"command"},
		},
	}
	result := convertTool(tool)

	if result.Type != "function" {
		t.Errorf("expected type 'function', got %q", result.Type)
	}
	if result.Function.Name != "bash" {
		t.Errorf("expected name 'bash', got %q", result.Function.Name)
	}
	if result.Function.Description != "Execute a shell command" {
		t.Errorf("expected description match")
	}
	props, ok := result.Function.Parameters["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties in parameters")
	}
	if _, ok := props["command"]; !ok {
		t.Error("expected 'command' property")
	}
}

func TestDeferredToolSkipped(t *testing.T) {
	tools := []kosong.Tool{
		{Name: "active", Description: "active tool", Parameters: map[string]interface{}{}},
		{Name: "deferred", Description: "deferred tool", Parameters: map[string]interface{}{}, Deferred: true},
	}

	var wireTools []openAITool
	for _, t := range tools {
		if t.Deferred {
			continue
		}
		wireTools = append(wireTools, convertTool(t))
	}

	if len(wireTools) != 1 {
		t.Fatalf("expected 1 tool (deferred skipped), got %d", len(wireTools))
	}
	if wireTools[0].Function.Name != "active" {
		t.Errorf("expected active tool, got %q", wireTools[0].Function.Name)
	}
}

// ── Finish reason mapping tests ──

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		raw      *string
		expected kosong.FinishReason
	}{
		{nil, kosong.FinishOther},
		{strPtr("stop"), kosong.FinishCompleted},
		{strPtr("tool_calls"), kosong.FinishToolCalls},
		{strPtr("function_call"), kosong.FinishToolCalls},
		{strPtr("length"), kosong.FinishTruncated},
		{strPtr("content_filter"), kosong.FinishFiltered},
		{strPtr("unknown"), kosong.FinishOther},
	}

	for _, tt := range tests {
		result := MapFinishReason(tt.raw)
		if result != tt.expected {
			raw := "<nil>"
			if tt.raw != nil {
				raw = *tt.raw
			}
			t.Errorf("MapFinishReason(%q) = %q, want %q", raw, result, tt.expected)
		}
	}
}

// ── SSE stream parsing tests ──

func TestSSEStreamParsing(t *testing.T) {
	// Create a mock server that returns SSE stream
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected Bearer auth, got %q", auth)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "trace-123")
		w.WriteHeader(http.StatusOK)

		// Send SSE chunks
		chunks := []string{
			`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			w.(http.Flusher).Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIProviderConfig{
		Name:    "test",
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	stream, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("Hi")}, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Collect parts
	parts, err := kosong.CollectParts(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectParts failed: %v", err)
	}

	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "Hello" {
		t.Errorf("first part: expected text 'Hello', got %+v", parts[0])
	}
	if parts[1].Type != "text" || parts[1].Text != " world" {
		t.Errorf("second part: expected text ' world', got %+v", parts[1])
	}
	if parts[2].Type != "finish" || parts[2].FinishReason == nil || *parts[2].FinishReason != "stop" {
		t.Errorf("third part: expected finish with reason 'stop', got %+v", parts[2])
	}

	// Check trace ID
	if stream.TraceID == nil || *stream.TraceID != "trace-123" {
		t.Errorf("expected trace ID 'trace-123', got %v", stream.TraceID)
	}
}

func TestSSEStreamWithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":""}}]},"finish_reason":null}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"/tmp/file\"}"}}]},"finish_reason":null}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}
		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			w.(http.Flusher).Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIProviderConfig{
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	stream, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("read a file")}, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	parts, err := kosong.CollectParts(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectParts failed: %v", err)
	}

	// Should have: function header + 2 tool_call_part deltas + finish
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d: %+v", len(parts), parts)
	}

	// First part: function header
	if parts[0].Type != "function" || parts[0].Name != "read" || parts[0].ID != "call_1" {
		t.Errorf("first part should be function header, got %+v", parts[0])
	}
	// Last part: finish with reason "tool_calls"
	if parts[3].Type != "finish" || parts[3].FinishReason == nil || *parts[3].FinishReason != "tool_calls" {
		t.Errorf("fourth part: expected finish with reason 'tool_calls', got %+v", parts[3])
	}
}

func TestSSEStreamWithReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think..."},"finish_reason":null}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"content":"The answer is 42"},"finish_reason":null}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			w.(http.Flusher).Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIProviderConfig{
		Name:    "kimi",
		BaseURL: server.URL,
		Model:   "kimi-k3",
	})

	stream, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("What is the answer?")}, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	parts, err := kosong.CollectParts(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectParts failed: %v", err)
	}

	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %+v", len(parts), parts)
	}
	if parts[0].Type != "think" || parts[0].Think != "Let me think..." {
		t.Errorf("first part should be think, got %+v", parts[0])
	}
	if parts[1].Type != "text" || parts[1].Text != "The answer is 42" {
		t.Errorf("second part should be text, got %+v", parts[1])
	}
	if parts[2].Type != "finish" || parts[2].FinishReason == nil || *parts[2].FinishReason != "stop" {
		t.Errorf("third part: expected finish with reason 'stop', got %+v", parts[2])
	}
}

func TestAPIErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "Invalid API key",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIProviderConfig{
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	_, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("Hi")}, nil)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	var statusErr *kosong.APIStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected APIStatusError, got %T: %v", err, err)
	}
	if statusErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", statusErr.StatusCode)
	}
}

func TestProviderWithThinking(t *testing.T) {
	provider := NewOpenAIProvider(OpenAIProviderConfig{
		Name:  "test",
		Model: "k3",
	})

	if provider.ThinkingEffort() != "" {
		t.Errorf("expected empty thinking effort, got %q", provider.ThinkingEffort())
	}

	p2 := provider.WithThinking(kosong.ThinkingEffort("high"))
	if p2.ThinkingEffort() != "high" {
		t.Errorf("expected 'high' effort, got %q", p2.ThinkingEffort())
	}
	// Original should be unchanged
	if provider.ThinkingEffort() != "" {
		t.Errorf("original should be unchanged, got %q", provider.ThinkingEffort())
	}
}

func TestProviderWithMaxCompletionTokens(t *testing.T) {
	provider := NewOpenAIProvider(OpenAIProviderConfig{
		Name:  "test",
		Model: "gpt-4",
	})

	p2 := provider.WithMaxCompletionTokens(8192, nil)
	if p2.MaxCompletionTokens() != 8192 {
		t.Errorf("expected 8192, got %d", p2.MaxCompletionTokens())
	}
	if provider.MaxCompletionTokens() != 0 {
		t.Errorf("original should be 0, got %d", provider.MaxCompletionTokens())
	}
}

// ── Generate → Message assembly integration test ──

func TestGenerateAssemblesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello "},"finish_reason":null}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"content":"world!"},"finish_reason":null}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			w.(http.Flusher).Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIProviderConfig{
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	stream, err := provider.Generate(context.Background(), "You are helpful", nil,
		[]kosong.Message{kosong.CreateUserMessage("Hi")}, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	msg, err := kosong.Generate(context.Background(), stream)
	if err != nil {
		t.Fatalf("Generate assembly failed: %v", err)
	}

	if msg.Role != kosong.RoleAssistant {
		t.Errorf("expected assistant role, got %q", msg.Role)
	}
	text := kosong.ExtractText(msg, "")
	if text != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", text)
	}
}

func strPtr(s string) *string {
	return &s
}

// ── Base URL normalization tests ──

func TestHasVersionPath(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://api.deepseek.com", false},
		{"https://api.deepseek.com/", false},
		{"https://api.deepseek.com/v1", true},
		{"https://api.deepseek.com/v1/", true},
		{"https://api.openai.com/v1", true},
		{"https://api.openai.com/v2", true},
		{"https://generativelanguage.googleapis.com/v1beta", true},
		{"https://example.com/custom/path", false},
		{"https://example.com", false},
		{"http://localhost:8080", false},
		{"http://localhost:8080/v1", true},
	}
	for _, tc := range cases {
		got := hasVersionPath(tc.url)
		if got != tc.want {
			t.Errorf("hasVersionPath(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestNewOpenAIProvider_NormalizesBaseURL(t *testing.T) {
	// A URL without /v1 should get /v1 appended.
	p := NewOpenAIProvider(OpenAIProviderConfig{
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-v4-pro",
	})
	if !strings.HasSuffix(p.baseURL, "/v1") {
		t.Errorf("expected baseURL to end with /v1, got %q", p.baseURL)
	}
	expected := "https://api.deepseek.com/v1"
	if p.baseURL != expected {
		t.Errorf("expected baseURL %q, got %q", expected, p.baseURL)
	}

	// A URL already ending with /v1 should not be double-appended.
	p2 := NewOpenAIProvider(OpenAIProviderConfig{
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-v4-pro",
	})
	if p2.baseURL != "https://api.deepseek.com/v1" {
		t.Errorf("expected baseURL to remain unchanged, got %q", p2.baseURL)
	}

	// Trailing slash should be trimmed before normalization.
	p3 := NewOpenAIProvider(OpenAIProviderConfig{
		BaseURL: "https://api.deepseek.com/",
		Model:   "deepseek-v4-pro",
	})
	if p3.baseURL != "https://api.deepseek.com/v1" {
		t.Errorf("expected /v1 appended after trimming slash, got %q", p3.baseURL)
	}
}
