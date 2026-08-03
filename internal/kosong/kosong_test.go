package kosong

import (
	"context"
	"errors"
	"testing"
)

func TestMergeInPlace_TextText(t *testing.T) {
	a := StreamedMessagePart{Type: "text", Text: "hello "}
	b := StreamedMessagePart{Type: "text", Text: "world"}
	if !MergeInPlace(&a, &b) {
		t.Fatal("expected merge to succeed")
	}
	if a.Text != "hello world" {
		t.Fatalf("got %q, want %q", a.Text, "hello world")
	}
}

func TestMergeInPlace_ThinkThink(t *testing.T) {
	a := StreamedMessagePart{Type: "think", Think: "step 1 "}
	b := StreamedMessagePart{Type: "think", Think: "step 2"}
	if !MergeInPlace(&a, &b) {
		t.Fatal("expected merge to succeed")
	}
	if a.Think != "step 1 step 2" {
		t.Fatalf("got %q, want %q", a.Think, "step 1 step 2")
	}
}

func TestMergeInPlace_ThinkEncrypted(t *testing.T) {
	enc := "sig"
	a := StreamedMessagePart{Type: "think", Think: "reasoning", Encrypted: &enc}
	b := StreamedMessagePart{Type: "think", Think: "more"}
	if MergeInPlace(&a, &b) {
		t.Fatal("expected merge to fail when target has encrypted")
	}
}

func TestMergeInPlace_ToolCallDelta(t *testing.T) {
	args := `{"key":`
	a := StreamedMessagePart{Type: "function", ID: "1", Name: "test", Arguments: &args}
	delta := `"value"}`
	b := StreamedMessagePart{Type: "tool_call_part", ArgumentsPart: &delta}
	if !MergeInPlace(&a, &b) {
		t.Fatal("expected merge to succeed")
	}
	if *a.Arguments != `{"key":"value"}` {
		t.Fatalf("got %q, want %q", *a.Arguments, `{"key":"value"}`)
	}
}

func TestMergeInPlace_Incompatible(t *testing.T) {
	a := StreamedMessagePart{Type: "text", Text: "hello"}
	b := StreamedMessagePart{Type: "think", Think: "world"}
	if MergeInPlace(&a, &b) {
		t.Fatal("expected merge to fail for incompatible types")
	}
}

func TestGenerate_MergesStream(t *testing.T) {
	ch := make(chan StreamedMessagePart, 5)
	ch <- StreamedMessagePart{Type: "text", Text: "hello "}
	ch <- StreamedMessagePart{Type: "text", Text: "world"}
	args := `{"x":1}`
	ch <- StreamedMessagePart{Type: "function", ID: "call_1", Name: "mytool", Arguments: &args}
	ch <- StreamedMessagePart{Type: "text", Text: " after tool"}
	close(ch)

	stream := &StreamedMessage{Parts: ch}
	msg, err := Generate(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msg.Content))
	}
	if msg.Content[0].Text != "hello world" {
		t.Fatalf("got text %q", msg.Content[0].Text)
	}
	if msg.Content[1].Text != " after tool" {
		t.Fatalf("got text %q", msg.Content[1].Text)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Name != "mytool" {
		t.Fatalf("got tool call name %q", msg.ToolCalls[0].Name)
	}
}

func TestCreateUserMessage(t *testing.T) {
	msg := CreateUserMessage("hello")
	if msg.Role != RoleUser {
		t.Fatalf("expected role user, got %v", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text != "hello" {
		t.Fatalf("unexpected content: %v", msg.Content)
	}
}

func TestExtractText(t *testing.T) {
	msg := &Message{
		Content: []ContentPart{
			{Type: "text", Text: "part1"},
			{Type: "think", Think: "ignored"},
			{Type: "text", Text: "part2"},
		},
	}
	got := ExtractText(msg, " ")
	if got != "part1 part2" {
		t.Fatalf("got %q, want %q", got, "part1 part2")
	}
}

func TestTokenUsage(t *testing.T) {
	a := TokenUsage{InputOther: 10, Output: 5, InputCacheRead: 3, InputCacheCreation: 2}
	b := TokenUsage{InputOther: 1, Output: 2, InputCacheRead: 0, InputCacheCreation: 1}
	sum := AddUsage(a, b)
	if sum.InputTotal() != 10+3+2+1+0+1 {
		t.Fatalf("input total: got %d", sum.InputTotal())
	}
	if sum.GrandTotal() != sum.InputTotal()+5+2 {
		t.Fatalf("grand total: got %d", sum.GrandTotal())
	}
}

func TestIsToolDeclarationOnlyMessage(t *testing.T) {
	msg := &Message{Tools: []Tool{{Name: "x"}}}
	if !IsToolDeclarationOnlyMessage(msg) {
		t.Fatal("expected true")
	}
	msg.Content = []ContentPart{{Type: "text", Text: "hi"}}
	if IsToolDeclarationOnlyMessage(msg) {
		t.Fatal("expected false with content")
	}
}

// ── mock provider for GenerateCall tests ──

type mockProvider struct {
	name      string
	model     string
	stream    *StreamedMessage
	genErr    error
}

func (m *mockProvider) Name() string                                        { return m.name }
func (m *mockProvider) ModelName() string                                   { return m.model }
func (m *mockProvider) ThinkingEffort() ThinkingEffort                      { return "" }
func (m *mockProvider) MaxCompletionTokens() int                            { return 0 }
func (m *mockProvider) WithThinking(ThinkingEffort) ChatProvider            { return m }
func (m *mockProvider) WithMaxCompletionTokens(int, *MaxCompletionTokensOptions) ChatProvider { return m }
func (m *mockProvider) UploadVideo(context.Context, interface{}, *GenerateOptions) (*VideoURLPart, error) {
	return nil, errors.New("not supported")
}
func (m *mockProvider) Generate(ctx context.Context, systemPrompt string, tools []Tool, history []Message, opts *GenerateOptions) (*StreamedMessage, error) {
	if m.genErr != nil {
		return nil, m.genErr
	}
	return m.stream, nil
}

func makeStream(parts ...StreamedMessagePart) *StreamedMessage {
	ch := make(chan StreamedMessagePart, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	return &StreamedMessage{Parts: ch}
}

func TestGenerateCallSuccess(t *testing.T) {
	stream := makeStream(
		StreamedMessagePart{Type: "text", Text: "hello "},
		StreamedMessagePart{Type: "text", Text: "world"},
	)
	prov := &mockProvider{name: "test", model: "test-model", stream: stream}

	result, err := GenerateCall(context.Background(), prov, "sys", nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message == nil {
		t.Fatal("expected message")
	}
	text := ExtractText(result.Message, "")
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
}

func TestGenerateCallEmptyResponse(t *testing.T) {
	stream := makeStream() // no parts
	prov := &mockProvider{name: "test", model: "test-model", stream: stream}

	_, err := GenerateCall(context.Background(), prov, "sys", nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty response")
	}
	var emptyErr *APIEmptyResponseError
	if !errors.As(err, &emptyErr) {
		t.Errorf("expected APIEmptyResponseError, got %T", err)
	}
}

func TestGenerateCallThinkOnlyResponse(t *testing.T) {
	stream := makeStream(
		StreamedMessagePart{Type: "think", Think: "reasoning..."},
	)
	prov := &mockProvider{name: "test", model: "test-model", stream: stream}

	_, err := GenerateCall(context.Background(), prov, "sys", nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for think-only response")
	}
	var emptyErr *APIEmptyResponseError
	if !errors.As(err, &emptyErr) {
		t.Errorf("expected APIEmptyResponseError, got %T", err)
	}
}

func TestGenerateCallWithToolCalls(t *testing.T) {
	args := `{"key":"value"}`
	stream := makeStream(
		StreamedMessagePart{Type: "function", ID: "call_1", Name: "read", Arguments: &args},
	)
	prov := &mockProvider{name: "test", model: "test-model", stream: stream}

	var toolCallsFired int
	result, err := GenerateCall(context.Background(), prov, "sys", nil, nil, &GenerateCallbacks{
		OnToolCall: func(tc ToolCall) { toolCallsFired++ },
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Message.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(result.Message.ToolCalls))
	}
	if toolCallsFired != 1 {
		t.Errorf("expected OnToolCall fired once, got %d", toolCallsFired)
	}
}

func TestGenerateCallProviderError(t *testing.T) {
	prov := &mockProvider{name: "test", model: "m", genErr: errors.New("auth failed")}

	_, err := GenerateCall(context.Background(), prov, "sys", nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "auth failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFilterDeferredTools(t *testing.T) {
	tools := []Tool{
		{Name: "read"},
		{Name: "deferred_tool", Deferred: true},
		{Name: "write"},
	}
	result := filterDeferredTools(tools)
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}
	for _, tool := range result {
		if tool.Deferred {
			t.Errorf("deferred tool should be filtered: %s", tool.Name)
		}
	}
}

func TestFilterDeferredToolsNoneDeferred(t *testing.T) {
	tools := []Tool{{Name: "a"}, {Name: "b"}}
	result := filterDeferredTools(tools)
	if len(result) != 2 {
		t.Fatalf("expected 2 tools unchanged, got %d", len(result))
	}
}

// ── Parallel tool call routing tests ──

func TestGenerate_ParallelToolCallRouting(t *testing.T) {
	// Simulate interleaved deltas from 2 parallel tool calls (index 0 and 1).
	// The provider emits argument deltas for both calls interleaved.
	arg0a := `{"file":"`    // tool call 0, first chunk
	arg0b := `test.go"}`      // tool call 0, second chunk
	arg1a := `{"pattern":"`   // tool call 1, first chunk
	arg1b := `*.go"}`         // tool call 1, second chunk

	ch := make(chan StreamedMessagePart, 10)
	// Tool call 0 header (index=0)
	ch <- StreamedMessagePart{Type: "function", ID: "call_0", Name: "read", Index: 0}
	// Tool call 1 header (index=1)
	ch <- StreamedMessagePart{Type: "function", ID: "call_1", Name: "grep", Index: 1}
	// Interleaved argument deltas
	ch <- StreamedMessagePart{Type: "tool_call_part", ArgumentsPart: &arg0a, Index: 0}
	ch <- StreamedMessagePart{Type: "tool_call_part", ArgumentsPart: &arg1a, Index: 1}
	ch <- StreamedMessagePart{Type: "tool_call_part", ArgumentsPart: &arg0b, Index: 0}
	ch <- StreamedMessagePart{Type: "tool_call_part", ArgumentsPart: &arg1b, Index: 1}
	close(ch)

	stream := &StreamedMessage{Parts: ch}
	msg, err := Generate(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}

	if len(msg.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(msg.ToolCalls))
	}

	// Tool call 0 should have correctly assembled arguments
	if msg.ToolCalls[0].Name != "read" {
		t.Errorf("tool call 0: expected name 'read', got %q", msg.ToolCalls[0].Name)
	}
	if msg.ToolCalls[0].Arguments == nil {
		t.Fatal("tool call 0: expected arguments, got nil")
	}
	if *msg.ToolCalls[0].Arguments != `{"file":"test.go"}` {
		t.Errorf("tool call 0: arguments = %q, want %q", *msg.ToolCalls[0].Arguments, `{"file":"test.go"}`)
	}

	// Tool call 1 should have correctly assembled arguments
	if msg.ToolCalls[1].Name != "grep" {
		t.Errorf("tool call 1: expected name 'grep', got %q", msg.ToolCalls[1].Name)
	}
	if msg.ToolCalls[1].Arguments == nil {
		t.Fatal("tool call 1: expected arguments, got nil")
	}
	if *msg.ToolCalls[1].Arguments != `{"pattern":"*.go"}` {
		t.Errorf("tool call 1: arguments = %q, want %q", *msg.ToolCalls[1].Arguments, `{"pattern":"*.go"}`)
	}
}

func TestGenerate_ParallelToolCallRoutingStringIndex(t *testing.T) {
	// Test with string-based indices (some providers use string IDs).
	arg0 := `{"key":"a"}`
	arg1 := `{"key":"b"}`

	ch := make(chan StreamedMessagePart, 6)
	ch <- StreamedMessagePart{Type: "function", ID: "call_a", Name: "tool_a", Index: "idx_a"}
	ch <- StreamedMessagePart{Type: "tool_call_part", ArgumentsPart: &arg0, Index: "idx_a"}
	ch <- StreamedMessagePart{Type: "function", ID: "call_b", Name: "tool_b", Index: "idx_b"}
	ch <- StreamedMessagePart{Type: "tool_call_part", ArgumentsPart: &arg1, Index: "idx_b"}
	close(ch)

	stream := &StreamedMessage{Parts: ch}
	msg, err := Generate(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}

	if len(msg.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Arguments == nil || *msg.ToolCalls[0].Arguments != `{"key":"a"}` {
		t.Errorf("tool call 0: unexpected arguments %v", msg.ToolCalls[0].Arguments)
	}
	if msg.ToolCalls[1].Arguments == nil || *msg.ToolCalls[1].Arguments != `{"key":"b"}` {
		t.Errorf("tool call 1: unexpected arguments %v", msg.ToolCalls[1].Arguments)
	}
}

func TestIsPendingToolCallAtIndex(t *testing.T) {
	// nil pending
	if isPendingToolCallAtIndex(nil, 0) {
		t.Error("nil pending should return false")
	}
	// non-function pending
	p := &StreamedMessagePart{Type: "text", Text: "hi"}
	if isPendingToolCallAtIndex(p, 0) {
		t.Error("text pending should return false")
	}
	// function pending with matching index
	f := &StreamedMessagePart{Type: "function", ID: "1", Name: "t", Index: 0}
	if !isPendingToolCallAtIndex(f, 0) {
		t.Error("function with index 0 should match")
	}
	if isPendingToolCallAtIndex(f, 1) {
		t.Error("function with index 0 should not match index 1")
	}
	// function pending without index
	ni := &StreamedMessagePart{Type: "function", ID: "1", Name: "t"}
	if isPendingToolCallAtIndex(ni, 0) {
		t.Error("function without index should not match")
	}
}

func TestIsPendingToolCallAtIndex_TypeNormalization(t *testing.T) {
	// int vs float64 — common when JSON deserializes numbers as float64.
	f := &StreamedMessagePart{Type: "function", ID: "1", Name: "t", Index: 0}
	if !isPendingToolCallAtIndex(f, float64(0)) {
		t.Error("int 0 should match float64(0) after normalization")
	}
	if isPendingToolCallAtIndex(f, float64(1)) {
		t.Error("int 0 should not match float64(1)")
	}
	// string index should still work
	fs := &StreamedMessagePart{Type: "function", ID: "1", Name: "t", Index: "idx_0"}
	if !isPendingToolCallAtIndex(fs, "idx_0") {
		t.Error("string index should match")
	}
	if isPendingToolCallAtIndex(fs, "idx_1") {
		t.Error("different string index should not match")
	}
}

// ── Model catalog tests ──

func TestCatalogModelToCapability(t *testing.T) {
	// Basic valid entry
	entry := CatalogModelEntry{
		ID: "test-model", Name: "Test", LimitContext: 128000, LimitOutput: 4096,
		ModalitiesInput: []string{"text", "image"}, ToolCall: boolPtr(true),
	}
	cm := CatalogModelToCapability(entry)
	if cm == nil {
		t.Fatal("expected non-nil CatalogModel")
	}
	if !cm.Capability.ImageIn {
		t.Error("expected ImageIn")
	}
	if cm.Capability.VideoIn {
		t.Error("unexpected VideoIn")
	}
	if !cm.Capability.ToolUse {
		t.Error("expected ToolUse")
	}
	if cm.Capability.MaxContextTokens != 128000 {
		t.Errorf("MaxContextTokens = %d, want 128000", cm.Capability.MaxContextTokens)
	}

	// Empty ID should return nil
	nilCm := CatalogModelToCapability(CatalogModelEntry{})
	if nilCm != nil {
		t.Error("expected nil for empty ID")
	}

	// Zero context should return nil
	nilCm2 := CatalogModelToCapability(CatalogModelEntry{ID: "x", LimitContext: 0})
	if nilCm2 != nil {
		t.Error("expected nil for zero context")
	}
}

func TestCatalogModelToCapability_Reasoning(t *testing.T) {
	entry := CatalogModelEntry{
		ID: "reasoning-model", Name: "R", LimitContext: 200000, LimitOutput: 100000,
		Reasoning: true, ModalitiesInput: []string{"text"}, ToolCall: boolPtr(true),
	}
	cm := CatalogModelToCapability(entry)
	if cm == nil {
		t.Fatal("expected non-nil")
	}
	if !cm.Capability.Thinking {
		t.Error("expected Thinking for reasoning model")
	}
}

func TestCatalogModelToCapability_Deprecated(t *testing.T) {
	entry := CatalogModelEntry{
		ID: "old-model", Name: "Old", LimitContext: 8000,
		Status: "deprecated", ModalitiesInput: []string{"text"},
	}
	cm := CatalogModelToCapability(entry)
	if cm != nil {
		t.Error("expected nil for deprecated model")
	}
}

func TestLookupModelCapability(t *testing.T) {
	// Exact match
	cap := LookupModelCapability("gpt-4o")
	if !cap.ImageIn || !cap.ToolUse {
		t.Errorf("gpt-4o: expected ImageIn+ToolUse, got %+v", cap)
	}

	// Prefix match
	cap2 := LookupModelCapability("gpt-4o-2024-11-20")
	if !cap2.ImageIn {
		t.Errorf("gpt-4o variant: expected ImageIn, got %+v", cap2)
	}

	// Reasoning model
	cap3 := LookupModelCapability("o3-mini")
	if !cap3.Thinking || !cap3.ToolUse {
		t.Errorf("o3-mini: expected Thinking+ToolUse, got %+v", cap3)
	}

	// Case insensitive
	cap4 := LookupModelCapability("GPT-4O")
	if !cap4.ImageIn {
		t.Errorf("GPT-4O: expected ImageIn, got %+v", cap4)
	}

	// Unknown model
	cap5 := LookupModelCapability("unknown-model-xyz")
	if cap5 != UnknownCapability {
		t.Errorf("unknown model: expected UnknownCapability, got %+v", cap5)
	}
}

func TestLookupProviderModels(t *testing.T) {
	models := LookupProviderModels("openai")
	if len(models) == 0 {
		t.Fatal("expected OpenAI models")
	}

	// Check that we got expected models
	found := map[string]bool{}
	for _, m := range models {
		found[m.ID] = true
	}
	if !found["gpt-4o"] {
		t.Error("missing gpt-4o in OpenAI models")
	}

	// Unknown provider
	nilModels := LookupProviderModels("nonexistent")
	if nilModels != nil {
		t.Errorf("expected nil for unknown provider, got %v", nilModels)
	}
}

func TestBuiltinCatalogProviders(t *testing.T) {
	catalog := GetBuiltinCatalog()
	expectedProviders := []string{"openai", "anthropic", "google", "kimi"}
	for _, p := range expectedProviders {
		if _, ok := catalog[p]; !ok {
			t.Errorf("missing provider %q in builtin catalog", p)
		}
	}
}

func TestCatalogThinkingOptions(t *testing.T) {
	// Effort with none option
	opts := catalogThinkingOptions([]CatalogReasoningOption{
		{Type: "effort", Values: []string{"none", "low", "high"}},
	})
	if opts.offEffort != "none" {
		t.Errorf("expected offEffort 'none', got %q", opts.offEffort)
	}
	if len(opts.efforts) != 2 {
		t.Errorf("expected 2 efforts, got %d", len(opts.efforts))
	}

	// Always thinking (efforts but no off)
	opts2 := catalogThinkingOptions([]CatalogReasoningOption{
		{Type: "effort", Values: []string{"low", "medium", "high"}},
	})
	if !opts2.alwaysThinking {
		t.Error("expected alwaysThinking when no off effort")
	}

	// Toggle
	opts3 := catalogThinkingOptions([]CatalogReasoningOption{
		{Type: "toggle"},
	})
	if !opts3.hasToggle {
		t.Error("expected hasToggle")
	}
}

func TestIsUsableChatModel(t *testing.T) {
	// Embedding model
	emb := CatalogModelEntry{ID: "text-embedding-3", Family: "embeddings"}
	if isUsableChatModel(emb) {
		t.Error("embedding model should not be usable")
	}

	// Alpha model
	alpha := CatalogModelEntry{ID: "test", Status: "alpha"}
	if isUsableChatModel(alpha) {
		t.Error("alpha model should not be usable")
	}

	// Non-text output
	imgOnly := CatalogModelEntry{ID: "img", ModalitiesOutput: []string{"image"}}
	if isUsableChatModel(imgOnly) {
		t.Error("image-only output should not be usable")
	}

	// Normal model
	normal := CatalogModelEntry{ID: "ok", ModalitiesInput: []string{"text"}}
	if !isUsableChatModel(normal) {
		t.Error("normal model should be usable")
	}
}
