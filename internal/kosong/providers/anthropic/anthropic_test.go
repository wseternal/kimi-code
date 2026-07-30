package anthropic

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
	if result.Content != "Hello" {
		t.Errorf("expected content 'Hello', got %v", result.Content)
	}
}

func TestConvertAssistantMessageWithToolCalls(t *testing.T) {
	args := `{"path":"/tmp/file"}`
	msg := kosong.CreateAssistantMessage(
		[]kosong.ContentPart{kosong.NewTextPart("reading file")},
		[]kosong.ToolCall{{Type: "function", ID: "toolu_1", Name: "read", Arguments: &args}},
	)
	result := convertMessage(msg)
	if result.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", result.Role)
	}
	blocks, ok := result.Content.([]anthropicContentBlock)
	if !ok {
		t.Fatalf("expected []anthropicContentBlock, got %T", result.Content)
	}
	// Should have text + tool_use
	hasText := false
	hasToolUse := false
	for _, b := range blocks {
		if b.Type == "text" {
			hasText = true
		}
		if b.Type == "tool_use" && b.ID == "toolu_1" && b.Name == "read" {
			hasToolUse = true
		}
	}
	if !hasText {
		t.Error("expected text block")
	}
	if !hasToolUse {
		t.Error("expected tool_use block")
	}
}

func TestConvertToolMessage(t *testing.T) {
	callID := "toolu_1"
	msg := kosong.Message{
		Role:       kosong.RoleTool,
		Content:    []kosong.ContentPart{kosong.NewTextPart("file contents")},
		ToolCallID: &callID,
	}
	result := convertMessage(msg)
	// Tool messages become "user" with tool_result
	if result.Role != "user" {
		t.Errorf("expected role 'user', got %q", result.Role)
	}
	blocks, ok := result.Content.([]anthropicContentBlock)
	if !ok {
		t.Fatalf("expected []anthropicContentBlock, got %T", result.Content)
	}
	if len(blocks) != 1 || blocks[0].Type != "tool_result" {
		t.Errorf("expected single tool_result block, got %+v", blocks)
	}
	if blocks[0].ToolUseID != "toolu_1" {
		t.Errorf("expected tool_use_id 'toolu_1', got %q", blocks[0].ToolUseID)
	}
}

func TestConvertThinkingPart(t *testing.T) {
	sig := "sig_abc"
	msg := kosong.CreateAssistantMessage(
		[]kosong.ContentPart{
			kosong.NewThinkPart("my reasoning"),
			kosong.NewTextPart("answer"),
		},
		nil,
	)
	// Add encrypted to the think part
	msg.Content[0].Encrypted = &sig

	result := convertMessage(msg)
	blocks, ok := result.Content.([]anthropicContentBlock)
	if !ok {
		t.Fatalf("expected []anthropicContentBlock, got %T", result.Content)
	}

	hasThinking := false
	for _, b := range blocks {
		if b.Type == "thinking" && b.Thinking == "my reasoning" && b.Signature == "sig_abc" {
			hasThinking = true
		}
	}
	if !hasThinking {
		t.Error("expected thinking block with signature")
	}
}

func TestSSEStreamParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key 'test-key', got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "trace-abc")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`{"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			`{"type":"message_stop"}`,
		}
		for _, ev := range events {
			w.Write([]byte("data: " + ev + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	provider := NewProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "claude-sonnet-4-20250514",
	})

	stream, err := provider.Generate(context.Background(), "You are helpful", nil,
		[]kosong.Message{kosong.CreateUserMessage("Hi")}, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	parts, err := kosong.CollectParts(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectParts failed: %v", err)
	}

	// Should have: usage + 2 text deltas + finish + output usage
	hasText := false
	hasFinish := false
	for _, p := range parts {
		if p.Type == "text" {
			hasText = true
		}
		if p.Type == "finish" {
			hasFinish = true
			if p.FinishReason == nil || *p.FinishReason != "end_turn" {
				t.Errorf("expected finish_reason 'end_turn', got %v", p.FinishReason)
			}
		}
	}
	if !hasText {
		t.Error("expected text parts")
	}
	if !hasFinish {
		t.Error("expected finish part")
	}
}

func TestSSEStreamWithToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`{"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":20}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"read"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"pa"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"th\":\"/tmp\"}"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
			`{"type":"message_stop"}`,
		}
		for _, ev := range events {
			w.Write([]byte("data: " + ev + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	provider := NewProvider(Config{
		BaseURL: server.URL,
		Model:   "claude-sonnet-4-20250514",
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
	hasToolCallPart := false
	hasFinish := false
	for _, p := range parts {
		if p.Type == "function" && p.ID == "toolu_123" && p.Name == "read" {
			hasFunction = true
		}
		if p.Type == "tool_call_part" && p.ArgumentsPart != nil {
			hasToolCallPart = true
		}
		if p.Type == "finish" && p.FinishReason != nil && *p.FinishReason == "tool_use" {
			hasFinish = true
		}
	}
	if !hasFunction {
		t.Error("expected function header part")
	}
	if !hasToolCallPart {
		t.Error("expected tool_call_part delta")
	}
	if !hasFinish {
		t.Error("expected finish part with tool_use")
	}
}

func TestAPIErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "authentication_error",
				"message": "Invalid API key",
			},
		})
	}))
	defer server.Close()

	provider := NewProvider(Config{
		BaseURL: server.URL,
		Model:   "claude-sonnet-4-20250514",
	})

	_, err := provider.Generate(context.Background(), "", nil,
		[]kosong.Message{kosong.CreateUserMessage("Hi")}, nil)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestMapAnthropicFinishReason(t *testing.T) {
	tests := []struct {
		raw    *string
		expect kosong.FinishReason
	}{
		{nil, kosong.FinishOther},
		{strPtr("end_turn"), kosong.FinishCompleted},
		{strPtr("tool_use"), kosong.FinishToolCalls},
		{strPtr("max_tokens"), kosong.FinishTruncated},
		{strPtr("pause_turn"), kosong.FinishPaused},
	}
	for _, tt := range tests {
		got := mapAnthropicFinishReason(tt.raw)
		if got != tt.expect {
			raw := "<nil>"
			if tt.raw != nil {
				raw = *tt.raw
			}
			t.Errorf("mapAnthropicFinishReason(%q) = %q, want %q", raw, got, tt.expect)
		}
	}
}

func strPtr(s string) *string { return &s }
