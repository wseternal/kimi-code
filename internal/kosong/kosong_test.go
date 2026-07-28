package kosong

import (
	"context"
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
