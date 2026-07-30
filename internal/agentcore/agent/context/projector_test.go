package context

import (
	"testing"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

func userMsg(text string) kosong.Message {
	return kosong.Message{
		Role:      kosong.RoleUser,
		Content:   []kosong.ContentPart{kosong.NewTextPart(text)},
		ToolCalls: []kosong.ToolCall{},
	}
}

func assistantMsg(text string, toolCalls ...kosong.ToolCall) kosong.Message {
	if toolCalls == nil {
		toolCalls = []kosong.ToolCall{}
	}
	return kosong.Message{
		Role:      kosong.RoleAssistant,
		Content:   []kosong.ContentPart{kosong.NewTextPart(text)},
		ToolCalls: toolCalls,
	}
}

func toolMsg(toolCallID, text string) kosong.Message {
	id := toolCallID
	return kosong.Message{
		Role:       kosong.RoleTool,
		Content:    []kosong.ContentPart{kosong.NewTextPart(text)},
		ToolCalls:  []kosong.ToolCall{},
		ToolCallID: &id,
	}
}

func TestMergeAdjacentUserMessages(t *testing.T) {
	msgs := []kosong.Message{
		userMsg("hello"),
		userMsg("world"),
		assistantMsg("hi"),
	}

	result := Project(msgs, nil)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	text := kosong.ExtractText(&result[0], "")
	if text != "hello\n\nworld" {
		t.Errorf("expected merged text 'hello\\n\\nworld', got %q", text)
	}
	if result[0].Role != kosong.RoleUser {
		t.Errorf("expected user role, got %s", result[0].Role)
	}
}

func TestRepairToolExchangeAdjacency(t *testing.T) {
	tc := kosong.ToolCall{Type: "function", ID: "call_1", Name: "read"}
	msgs := []kosong.Message{
		assistantMsg("", tc),
		userMsg("interleaved user message"),
		toolMsg("call_1", "file contents"),
	}

	result := Project(msgs, nil)

	// Expected: assistant, tool result (pulled up), user message (pushed down)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[0].Role != kosong.RoleAssistant {
		t.Errorf("msg[0] should be assistant, got %s", result[0].Role)
	}
	if result[1].Role != kosong.RoleTool {
		t.Errorf("msg[1] should be tool (reordered), got %s", result[1].Role)
	}
	if result[1].ToolCallID == nil || *result[1].ToolCallID != "call_1" {
		t.Error("tool result should have toolCallId call_1")
	}
	if result[2].Role != kosong.RoleUser {
		t.Errorf("msg[2] should be user (pushed down), got %s", result[2].Role)
	}
}

func TestSynthesizeMissingToolResult(t *testing.T) {
	tc := kosong.ToolCall{Type: "function", ID: "call_orphan", Name: "search"}
	msgs := []kosong.Message{
		assistantMsg("", tc),
		userMsg("next turn"),
	}

	var anomalies []ProjectionAnomaly
	result := Project(msgs, &ProjectOptions{
		OnAnomaly: func(a ProjectionAnomaly) { anomalies = append(anomalies, a) },
	})

	// Mid-history: should synthesize
	if len(result) != 3 {
		t.Fatalf("expected 3 messages (assistant, synthetic tool, user), got %d", len(result))
	}
	if result[1].Role != kosong.RoleTool {
		t.Errorf("expected synthetic tool result, got %s", result[1].Role)
	}
	text := kosong.ExtractText(&result[1], "")
	if text != SyntheticToolResultText {
		t.Errorf("expected synthetic text, got %q", text)
	}

	foundSynthesized := false
	for _, a := range anomalies {
		if a.Kind == AnomalyToolResultSynthesized && a.ToolCallID == "call_orphan" {
			foundSynthesized = true
			if a.Trailing {
				t.Error("expected Trailing=false for mid-history")
			}
		}
	}
	if !foundSynthesized {
		t.Error("expected tool_result_synthesized anomaly")
	}
}

func TestTrailingToolCallNotSynthesizedByDefault(t *testing.T) {
	tc := kosong.ToolCall{Type: "function", ID: "call_pending", Name: "write"}
	msgs := []kosong.Message{
		userMsg("do it"),
		assistantMsg("", tc),
	}

	result := Project(msgs, nil)

	// Trailing call with no result: should NOT synthesize by default
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (no synthesis for trailing), got %d", len(result))
	}
}

func TestTrailingToolCallSynthesizedWithFlag(t *testing.T) {
	tc := kosong.ToolCall{Type: "function", ID: "call_pending", Name: "write"}
	msgs := []kosong.Message{
		userMsg("do it"),
		assistantMsg("", tc),
	}

	result := Project(msgs, &ProjectOptions{SynthesizeMissing: true})

	if len(result) != 3 {
		t.Fatalf("expected 3 messages (with synthesis for trailing), got %d", len(result))
	}
	if result[2].Role != kosong.RoleTool {
		t.Errorf("expected synthetic tool result at end, got %s", result[2].Role)
	}
}

func TestDropOrphanToolResults(t *testing.T) {
	msgs := []kosong.Message{
		userMsg("hello"),
		toolMsg("call_stray", "orphan result"),
		assistantMsg("response"),
	}

	var anomalies []ProjectionAnomaly
	result := Project(msgs, &ProjectOptions{
		DropOrphanResults: true,
		OnAnomaly:         func(a ProjectionAnomaly) { anomalies = append(anomalies, a) },
	})

	for _, m := range result {
		if m.Role == kosong.RoleTool && m.ToolCallID != nil && *m.ToolCallID == "call_stray" {
			t.Error("orphan tool result should have been dropped")
		}
	}

	found := false
	for _, a := range anomalies {
		if a.Kind == AnomalyOrphanToolResultDropped {
			found = true
		}
	}
	if !found {
		t.Error("expected orphan_tool_result_dropped anomaly")
	}
}

func TestDropLeadingNonUserMessages(t *testing.T) {
	msgs := []kosong.Message{
		assistantMsg("stale response"),
		toolMsg("call_old", "old result"),
		userMsg("fresh prompt"),
		assistantMsg("answer"),
	}

	result := Project(msgs, &ProjectOptions{DropLeadingNonUser: true})

	if len(result) != 2 {
		t.Fatalf("expected 2 messages after dropping leading non-user, got %d", len(result))
	}
	if result[0].Role != kosong.RoleUser {
		t.Errorf("first message should be user, got %s", result[0].Role)
	}
}

func TestMergeConsecutiveAssistants(t *testing.T) {
	msgs := []kosong.Message{
		userMsg("prompt"),
		assistantMsg("first"),
		assistantMsg("second"),
	}

	result := Project(msgs, &ProjectOptions{MergeConsecutiveAssistants: true})

	if len(result) != 2 {
		t.Fatalf("expected 2 messages (merged), got %d", len(result))
	}
	text := kosong.ExtractText(&result[1], "")
	if text != "firstsecond" {
		t.Errorf("expected merged text 'firstsecond', got %q", text)
	}
}

func TestDedupeDuplicateToolCalls(t *testing.T) {
	tc1 := kosong.ToolCall{Type: "function", ID: "call_1", Name: "read"}
	tc1dup := kosong.ToolCall{Type: "function", ID: "call_1", Name: "read"}

	msgs := []kosong.Message{
		assistantMsg("", tc1),
		toolMsg("call_1", "result1"),
		assistantMsg("", tc1dup),
		toolMsg("call_1", "result1_dup"),
	}

	var anomalies []ProjectionAnomaly
	result := Project(msgs, &ProjectOptions{
		DedupeDuplicateToolCalls: true,
		OnAnomaly:               func(a ProjectionAnomaly) { anomalies = append(anomalies, a) },
	})

	// Second assistant call and second result should be dropped
	assistantCount := 0
	toolCount := 0
	for _, m := range result {
		if m.Role == kosong.RoleAssistant {
			assistantCount++
		}
		if m.Role == kosong.RoleTool {
			toolCount++
		}
	}
	if assistantCount != 1 {
		t.Errorf("expected 1 assistant, got %d", assistantCount)
	}
	if toolCount != 1 {
		t.Errorf("expected 1 tool result, got %d", toolCount)
	}
}

func TestDropPartialMessages(t *testing.T) {
	partial := boolPtr(true)
	msgs := []kosong.Message{
		userMsg("hello"),
		{Role: kosong.RoleAssistant, Content: []kosong.ContentPart{kosong.NewTextPart("incomplete")}, ToolCalls: []kosong.ToolCall{}, Partial: partial},
		userMsg("next"),
	}

	result := Project(msgs, nil)

	for _, m := range result {
		if m.Partial != nil && *m.Partial {
			t.Error("partial messages should be dropped")
		}
	}
}

func TestDropWhitespaceOnlyContent(t *testing.T) {
	msgs := []kosong.Message{
		userMsg("hello"),
		{Role: kosong.RoleAssistant, Content: []kosong.ContentPart{kosong.NewTextPart("  \n  ")}, ToolCalls: []kosong.ToolCall{}},
	}

	var anomalies []ProjectionAnomaly
	result := Project(msgs, &ProjectOptions{
		OnAnomaly: func(a ProjectionAnomaly) { anomalies = append(anomalies, a) },
	})

	// Vacuous assistant message should be dropped
	for _, m := range result {
		if m.Role == kosong.RoleAssistant {
			t.Error("vacuous assistant should be dropped")
		}
	}
}

func TestTrimTrailingOpenToolExchange(t *testing.T) {
	tc := kosong.ToolCall{Type: "function", ID: "call_1", Name: "read"}

	tests := []struct {
		name     string
		msgs     []kosong.Message
		wantLen  int
	}{
		{
			name: "all closed",
			msgs: []kosong.Message{
				userMsg("prompt"),
				assistantMsg("", tc),
				toolMsg("call_1", "result"),
			},
			wantLen: 3,
		},
		{
			name: "unclosed trailing",
			msgs: []kosong.Message{
				userMsg("prompt"),
				assistantMsg("", tc),
			},
			wantLen: 1,
		},
		{
			name: "no tool calls",
			msgs: []kosong.Message{
				userMsg("prompt"),
				assistantMsg("answer"),
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimTrailingOpenToolExchange(tt.msgs)
			if len(result) != tt.wantLen {
				t.Errorf("expected %d messages, got %d", tt.wantLen, len(result))
			}
		})
	}
}

func TestToolDeclarationOnlyMessageSurvives(t *testing.T) {
	// Tool declaration with no adjacent user message should survive projection
	toolDecl := kosong.Message{
		Role:      kosong.RoleUser,
		Content:   []kosong.ContentPart{},
		ToolCalls: []kosong.ToolCall{},
		Tools:     []kosong.Tool{{Name: "test", Description: "a test tool"}},
	}

	msgs := []kosong.Message{toolDecl}
	result := Project(msgs, nil)

	if len(result) != 1 {
		t.Fatalf("expected tool declaration message to survive alone, got %d messages", len(result))
	}
	if len(result[0].Tools) != 1 {
		t.Error("expected tools to be preserved")
	}

	// When followed by user message, they merge (both are user role)
	msgs2 := []kosong.Message{toolDecl, userMsg("hello")}
	result2 := Project(msgs2, nil)

	if len(result2) != 1 {
		t.Fatalf("expected merged user message, got %d messages", len(result2))
	}
	if len(result2[0].Tools) != 1 {
		t.Error("tools should be preserved after merge")
	}
}

func TestMultipleToolCallsSingleAssistant(t *testing.T) {
	tc1 := kosong.ToolCall{Type: "function", ID: "call_1", Name: "read"}
	tc2 := kosong.ToolCall{Type: "function", ID: "call_2", Name: "write"}

	msgs := []kosong.Message{
		assistantMsg("", tc1, tc2),
		toolMsg("call_2", "write result"),
		toolMsg("call_1", "read result"),
		userMsg("next"),
	}

	result := Project(msgs, nil)

	// Both tool results should be pulled up after the assistant
	if len(result) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(result))
	}
	if result[0].Role != kosong.RoleAssistant {
		t.Error("first should be assistant")
	}
	if result[1].Role != kosong.RoleTool || result[2].Role != kosong.RoleTool {
		t.Error("next two should be tool results (reordered)")
	}
}
