package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

func TestConvertUserMessage(t *testing.T) {
	msg := kosong.CreateUserMessage("Hello")
	result := convertMessage(msg)
	if result.Role != "user" {
		t.Errorf("expected role 'user', got %q", result.Role)
	}
	if len(result.Parts) != 1 || result.Parts[0].Text != "Hello" {
		t.Errorf("expected text 'Hello', got %+v", result.Parts)
	}
}

func TestConvertAssistantMessage(t *testing.T) {
	args := `{"path":"/tmp/file"}`
	msg := kosong.CreateAssistantMessage(
		[]kosong.ContentPart{kosong.NewTextPart("reading file")},
		[]kosong.ToolCall{{Type: "function", ID: "call_1", Name: "read", Arguments: &args}},
	)
	result := convertMessage(msg)
	if result.Role != "model" {
		t.Errorf("expected role 'model', got %q", result.Role)
	}
	hasText := false
	hasFunctionCall := false
	for _, p := range result.Parts {
		if p.Text == "reading file" {
			hasText = true
		}
		if p.FunctionCall != nil && p.FunctionCall.Name == "read" {
			hasFunctionCall = true
		}
	}
	if !hasText {
		t.Error("expected text part")
	}
	if !hasFunctionCall {
		t.Error("expected function_call part")
	}
}

func TestConvertToolResponse(t *testing.T) {
	callID := "read"
	msg := kosong.Message{
		Role:       kosong.RoleTool,
		Content:    []kosong.ContentPart{kosong.NewTextPart("file contents")},
		ToolCallID: &callID,
	}
	result := convertMessage(msg)
	if result.Role != "user" {
		t.Errorf("expected role 'user' for tool response, got %q", result.Role)
	}
	if len(result.Parts) != 1 || result.Parts[0].FunctionResponse == nil {
		t.Fatal("expected function_response part")
	}
	if result.Parts[0].FunctionResponse.Name != "read" {
		t.Errorf("expected function_response name 'read', got %q", result.Parts[0].FunctionResponse.Name)
	}
}

func TestConvertThinkingPart(t *testing.T) {
	msg := kosong.CreateAssistantMessage(
		[]kosong.ContentPart{
			kosong.NewThinkPart("my reasoning"),
			kosong.NewTextPart("answer"),
		},
		nil,
	)
	result := convertMessage(msg)

	hasThinking := false
	hasText := false
	for _, p := range result.Parts {
		if p.Thought != nil && *p.Thought && p.Text == "my reasoning" {
			hasThinking = true
		}
		if p.Thought == nil && p.Text == "answer" {
			hasText = true
		}
	}
	if !hasThinking {
		t.Error("expected thinking part with thought=true")
	}
	if !hasText {
		t.Error("expected text part without thought flag")
	}
}

func TestSSEStreamParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":null}]}`,
			`{"candidates":[{"content":{"role":"model","parts":[{"text":" world"}]},"finishReason":"STOP"}]}`,
		}
		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	provider := NewProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gemini-2.5-pro",
	})

	stream, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("Hi")}, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	parts, err := kosong.CollectParts(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectParts failed: %v", err)
	}

	hasText := false
	hasFinish := false
	for _, p := range parts {
		if p.Type == "text" {
			hasText = true
		}
		if p.Type == "finish" && p.FinishReason != nil && *p.FinishReason == "STOP" {
			hasFinish = true
		}
	}
	if !hasText {
		t.Error("expected text parts")
	}
	if !hasFinish {
		t.Error("expected finish part")
	}
}

func TestSSEStreamWithFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"/tmp/file"}}}]},"finishReason":"STOP"}]}`
		w.Write([]byte("data: " + chunk + "\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer server.Close()

	provider := NewProvider(Config{
		BaseURL: server.URL,
		Model:   "gemini-2.5-pro",
	})

	stream, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("read file")}, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	parts, err := kosong.CollectParts(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectParts failed: %v", err)
	}

	hasFunction := false
	for _, p := range parts {
		if p.Type == "function" && p.Name == "read" {
			hasFunction = true
		}
	}
	if !hasFunction {
		t.Error("expected function call part")
	}
}

func TestAPIErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    401,
				"message": "API key not valid",
				"status":  "UNAUTHENTICATED",
			},
		})
	}))
	defer server.Close()

	provider := NewProvider(Config{
		BaseURL: server.URL,
		Model:   "gemini-2.5-pro",
	})

	_, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("Hi")}, nil)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestMapGenAIFinishReason(t *testing.T) {
	tests := []struct {
		raw    *string
		expect kosong.FinishReason
	}{
		{nil, kosong.FinishOther},
		{strPtr("STOP"), kosong.FinishCompleted},
		{strPtr("MAX_TOKENS"), kosong.FinishTruncated},
		{strPtr("SAFETY"), kosong.FinishFiltered},
		{strPtr("OTHER"), kosong.FinishOther},
	}
	for _, tt := range tests {
		got := mapGenAIFinishReason(tt.raw)
		if got != tt.expect {
			raw := "<nil>"
			if tt.raw != nil {
				raw = *tt.raw
			}
			t.Errorf("mapGenAIFinishReason(%q) = %q, want %q", raw, got, tt.expect)
		}
	}
}

func strPtr(s string) *string { return &s }
