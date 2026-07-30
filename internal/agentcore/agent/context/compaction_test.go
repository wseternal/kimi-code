package context

import (
	"fmt"
	"strings"
	"testing"
)

func TestCompactorRun(t *testing.T) {
	longQ := strings.Repeat("What is Go and how does it compare to other languages like Rust and Python? ", 5)
	longA := strings.Repeat("Go is a programming language created by Google focusing on simplicity and concurrency. ", 5)
	longQ2 := strings.Repeat("What about Rust and its focus on memory safety and performance? ", 5)
	longA2 := strings.Repeat("Rust is a systems programming language focused on safety and performance. ", 5)

	messages := []CompactMessage{
		{Role: "user", Content: longQ},
		{Role: "assistant", Content: longA},
		{Role: "user", Content: longQ2},
		{Role: "assistant", Content: longA2},
		{Role: "user", Content: "Compare them."},
		{Role: "assistant", Content: "Go focuses on simplicity, Rust on safety."},
	}

	mockGenerate := func(systemPrompt string, msgs []CompactMessage) (string, error) {
		// Verify the prompt contains instructions
		if !strings.Contains(systemPrompt, "handoff note") {
			t.Error("system prompt should contain compaction instructions")
		}
		return "The user asked about Go, Rust, and their comparison. Go emphasizes simplicity while Rust emphasizes safety.", nil
	}

	c := &Compactor{}
	result, err := c.Run(messages, mockGenerate, CompactOptions{
		KeepRecentTurns: 1,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.RemovedTurns != 2 {
		t.Errorf("RemovedTurns = %d, want 2", result.RemovedTurns)
	}
	if result.KeptTurns != 1 {
		t.Errorf("KeptTurns = %d, want 1", result.KeptTurns)
	}
	if result.Summary == "" {
		t.Error("Summary should not be empty")
	}
	if result.CompactTokens >= result.OriginalTokens {
		t.Errorf("CompactTokens (%d) should be less than OriginalTokens (%d)", result.CompactTokens, result.OriginalTokens)
	}

	// Rewritten messages should include the recent turn + summary
	if len(result.RewrittenMessages) < 3 {
		t.Errorf("RewrittenMessages should have at least 3 entries (user+assistant+summary), got %d", len(result.RewrittenMessages))
	}

	// Last message should be the summary with prefix
	lastMsg := result.RewrittenMessages[len(result.RewrittenMessages)-1]
	if lastMsg.Role != "user" {
		t.Errorf("last message should be user (summary), got %s", lastMsg.Role)
	}
	if !strings.Contains(lastMsg.Content, "compacted") {
		t.Error("summary should contain the compaction prefix")
	}
	if !strings.Contains(lastMsg.Content, "Go") {
		t.Error("summary should contain the LLM-generated content")
	}
}

func TestCompactorRunFallbackOnLLMError(t *testing.T) {
	messages := []CompactMessage{
		{Role: "user", Content: "What is Go?"},
		{Role: "assistant", Content: "Go is a programming language."},
		{Role: "user", Content: "What about Rust?"},
		{Role: "assistant", Content: "Rust is a systems language."},
		{Role: "user", Content: "Compare them."},
		{Role: "assistant", Content: "Go: simplicity, Rust: safety."},
	}

	mockGenerate := func(systemPrompt string, msgs []CompactMessage) (string, error) {
		return "", fmt.Errorf("API error: rate limited")
	}

	c := &Compactor{}
	result, err := c.Run(messages, mockGenerate, CompactOptions{
		KeepRecentTurns: 1,
	})
	if err != nil {
		t.Fatalf("Run should fall back to naive, not error: %v", err)
	}
	if result.Summary == "" {
		t.Error("fallback summary should not be empty")
	}
	if result.KeptTurns != 1 {
		t.Errorf("KeptTurns = %d, want 1", result.KeptTurns)
	}
}

func TestCompactorRunRetryOnEmpty(t *testing.T) {
	attempts := 0
	mockGenerate := func(systemPrompt string, msgs []CompactMessage) (string, error) {
		attempts++
		if attempts < 3 {
			return "", nil // empty response
		}
		return "Valid summary after retries.", nil
	}

	messages := []CompactMessage{
		{Role: "user", Content: "A"},
		{Role: "assistant", Content: "B"},
		{Role: "user", Content: "C"},
		{Role: "assistant", Content: "D"},
		{Role: "user", Content: "E"},
		{Role: "assistant", Content: "F"},
	}

	c := &Compactor{}
	result, err := c.Run(messages, mockGenerate, CompactOptions{KeepRecentTurns: 1})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(result.Summary, "Valid summary") {
		t.Errorf("expected retry summary, got: %s", result.Summary)
	}
	if attempts < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts)
	}
}

func TestCompactorRunNotEnoughTurns(t *testing.T) {
	messages := []CompactMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}

	c := &Compactor{}
	_, err := c.Run(messages, nil, CompactOptions{KeepRecentTurns: 2})
	if err == nil {
		t.Error("expected error for not enough turns")
	}
}

func TestGroupIntoTurns(t *testing.T) {
	messages := []CompactMessage{
		{Role: "user", Content: "A"},
		{Role: "assistant", Content: "B"},
		{Role: "user", Content: "C"},
		{Role: "assistant", Content: "D"},
		{Role: "user", Content: "E"}, // trailing user message
	}

	turns := groupIntoTurns(messages)
	if len(turns) != 3 {
		t.Errorf("expected 3 turns, got %d", len(turns))
	}
	if turns[0].user != "A" || turns[0].assistant != "B" {
		t.Errorf("turn 0: got %+v", turns[0])
	}
	if turns[2].assistant != "" {
		t.Errorf("turn 2 should have empty assistant, got %q", turns[2].assistant)
	}
}

func TestBuildRewrittenContext(t *testing.T) {
	recentTurns := []turn{
		{user: "Compare them", assistant: "Go: simplicity, Rust: safety"},
	}
	summary := "Previous work covered Go and Rust basics."

	rewritten := buildRewrittenContext(recentTurns, summary, CompactOptions{
		UserMessageMaxTokens: CompactUserMessageMaxTokens,
		HeadTokens:          CompactUserMessageHeadTokens,
	})

	// Should have: user, assistant, summary = 3 messages
	if len(rewritten) != 3 {
		t.Errorf("expected 3 messages, got %d", len(rewritten))
	}

	// Last should be the summary
	last := rewritten[len(rewritten)-1]
	if last.Role != "user" {
		t.Errorf("last should be user (summary), got %s", last.Role)
	}
	if !strings.Contains(last.Content, "compacted") {
		t.Error("summary should contain prefix")
	}
	if !strings.Contains(last.Content, summary) {
		t.Error("summary should contain LLM output")
	}
}

func TestHeadTailSplit(t *testing.T) {
	// Create messages where user content exceeds the budget
	longText := strings.Repeat("word ", 1000) // ~1250 tokens per message
	messages := []CompactMessage{
		{Role: "user", Content: longText},
		{Role: "assistant", Content: "response1"},
		{Role: "user", Content: longText},
		{Role: "assistant", Content: "response2"},
		{Role: "user", Content: "short question"},
		{Role: "assistant", Content: "response3"},
	}

	opts := CompactOptions{
		UserMessageMaxTokens: 500,  // very small budget
		HeadTokens:           100,
	}

	result := applyHeadTailSplit(messages, opts)

	// Should have elision marker since user messages exceed budget
	hasElision := false
	for _, msg := range result {
		if msg.Role == "system" && strings.Contains(msg.Content, "omitted") {
			hasElision = true
		}
	}
	if !hasElision {
		t.Error("expected elision marker when user messages exceed budget")
	}

	// Should have fewer user messages than original (some dropped)
	userCount := 0
	for _, msg := range result {
		if msg.Role == "user" {
			userCount++
		}
	}
	if userCount >= 3 {
		t.Error("some user messages should have been dropped")
	}
}

func TestHeadTailSplitWithinBudget(t *testing.T) {
	messages := []CompactMessage{
		{Role: "user", Content: "short"},
		{Role: "assistant", Content: "response"},
	}

	opts := CompactOptions{
		UserMessageMaxTokens: 20000,
		HeadTokens:           2000,
	}

	result := applyHeadTailSplit(messages, opts)
	if len(result) != len(messages) {
		t.Error("messages within budget should not be modified")
	}
}

func TestCompactionConstants(t *testing.T) {
	if CompactUserMessageMaxTokens != 20000 {
		t.Errorf("CompactUserMessageMaxTokens = %d, want 20000", CompactUserMessageMaxTokens)
	}
	if CompactUserMessageHeadTokens != 2000 {
		t.Errorf("CompactUserMessageHeadTokens = %d, want 2000", CompactUserMessageHeadTokens)
	}
	if DefaultKeepRecentTurns != 2 {
		t.Errorf("DefaultKeepRecentTurns = %d, want 2", DefaultKeepRecentTurns)
	}
}
