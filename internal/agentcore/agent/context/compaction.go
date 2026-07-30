package context

import (
	"fmt"
	"strings"
)

// Constants for LLM-based compaction, adapted from the TS project.
const (
	// CompactUserMessageMaxTokens is the total token budget for kept user
	// messages in the compacted context.
	CompactUserMessageMaxTokens = 20000

	// CompactUserMessageHeadTokens is the head slice reserved for the
	// oldest messages (typically the original task statement).
	CompactUserMessageHeadTokens = 2000

	// DefaultKeepRecentTurns is the number of recent turns to keep
	// verbatim during compaction.
	DefaultKeepRecentTurns = 2

	// MaxCompactRetries is the max attempts when the LLM returns empty.
	MaxCompactRetries = 5
)

// compactionInstruction is the system prompt for the LLM summarizer.
// Adapted from TS compaction-instruction.md.
const compactionInstruction = `You are compacting a conversation to free up context window space.
Write a first-person handoff note to yourself so you can continue this task
after the earlier conversation is cleared.

Focus on:
- The latest request intent and what the user is asking for
- Instructions or constraints currently in force
- What has been done so far (exact commands, file paths, code changes, test results)
- What is still unknown or unverified (gaps in knowledge)
- The forward plan (exact next steps, decisions already made, obstacles foreseen)

Include specific details: file paths, command outputs, error messages, line numbers,
function names. Vague summaries are worse than detailed ones.

Respond with text only. Do not call any tools.`

// compactionSummaryPrefix is prepended to the LLM-generated summary when
// inserted into the rewritten context.
const compactionSummaryPrefix = `The conversation so far has been compacted to free up context. What follows is
your own working summary of this task — use it to continue your train of thought
rather than starting over. Treat it as notes, not proof: where it says a step was
done, tests passed, or a fix worked, verify that yourself before relying on it.`

// elisionMarkerTemplate is inserted between head and tail user messages when
// some messages are omitted due to the token budget.
const elisionMarkerTemplate = `Some of this conversation's user messages were omitted here during compaction:
the messages above this note are the oldest user input, the messages below
are the most recent, and roughly %d tokens in between were dropped. The omitted
content is covered by the compaction summary at the end of the conversation.`

// GenerateFunc is the LLM call interface injected by the TUI.
// It receives a system prompt and conversation messages, returning the
// LLM's text response.
type GenerateFunc func(systemPrompt string, messages []CompactMessage) (string, error)

// Compactor performs LLM-based compaction on conversation history.
type Compactor struct{}

// CompactOptions configures a compaction run.
type CompactOptions struct {
	KeepRecentTurns      int    // turns to keep verbatim (default 2)
	UserMessageMaxTokens int    // token budget for kept user messages (default 20000)
	HeadTokens           int    // head slice for oldest messages (default 2000)
	CustomInstruction    string // optional user instruction appended to prompt
}

// LLMCompactResult holds the output of an LLM-based compaction.
type LLMCompactResult struct {
	Summary          string          // the LLM-generated handoff summary
	RewrittenMessages []CompactMessage // the new context (recent msgs + summary)
	RemovedTurns     int
	KeptTurns        int
	OriginalTokens   int
	CompactTokens    int
}

// Run performs LLM-based compaction on the conversation messages.
// It splits messages into old (to summarize) and recent (to keep),
// calls the LLM to generate a handoff summary, and builds the
// rewritten context.
//
// On failure (LLM error, empty response), it falls back to naive
// CompactMessages truncation.
func (c *Compactor) Run(
	messages []CompactMessage,
	generate GenerateFunc,
	opts CompactOptions,
) (*LLMCompactResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages to compact")
	}

	// Apply defaults
	if opts.KeepRecentTurns <= 0 {
		opts.KeepRecentTurns = DefaultKeepRecentTurns
	}
	if opts.UserMessageMaxTokens <= 0 {
		opts.UserMessageMaxTokens = CompactUserMessageMaxTokens
	}
	if opts.HeadTokens <= 0 {
		opts.HeadTokens = CompactUserMessageHeadTokens
	}

	// Group messages into turns (user+assistant pairs)
	turns := groupIntoTurns(messages)
	if len(turns) <= opts.KeepRecentTurns {
		return nil, fmt.Errorf("not enough turns to compact (%d turns, keeping %d)", len(turns), opts.KeepRecentTurns)
	}

	// Split into old and recent
	recentTurns := turns[len(turns)-opts.KeepRecentTurns:]
	oldTurns := turns[:len(turns)-opts.KeepRecentTurns]

	// Calculate original token count
	originalTokens := 0
	for _, msg := range messages {
		originalTokens += TokenEstimate(msg.Content) + TokenEstimate(msg.Role)
	}

	// Build old conversation text for the summarizer
	var oldConversation strings.Builder
	for i, t := range oldTurns {
		oldConversation.WriteString(fmt.Sprintf("[Turn %d]\nUser: %s\nAssistant: %s\n\n", i+1, t.user, t.assistant))
	}

	// Build the summarizer prompt
	prompt := compactionInstruction
	if opts.CustomInstruction != "" {
		prompt += "\n\nAdditional instruction from user: " + opts.CustomInstruction
	}

	// Prepare messages for the summarizer (old turns as conversation context)
	var summarizerMessages []CompactMessage
	summarizerMessages = append(summarizerMessages, CompactMessage{
		Role:    "user",
		Content: "Here is the conversation to summarize:\n\n" + oldConversation.String(),
	})

	// Call LLM with retry on empty response
	var summary string
	var err error
	for attempt := 0; attempt < MaxCompactRetries; attempt++ {
		summary, err = generate(prompt, summarizerMessages)
		if err != nil {
			// Fall back to naive compaction
			naiveResult, naiveErr := CompactMessages(messages, opts.KeepRecentTurns)
			if naiveErr != nil {
				return nil, fmt.Errorf("LLM compaction failed (%v) and naive fallback failed: %w", err, naiveErr)
			}
			return &LLMCompactResult{
				Summary:           naiveResult.Summary,
				RewrittenMessages: buildNaiveRewritten(messages, naiveResult, opts),
				RemovedTurns:      naiveResult.RemovedTurns,
				KeptTurns:         naiveResult.KeptTurns,
				OriginalTokens:    naiveResult.OriginalTokens,
				CompactTokens:     naiveResult.CompactTokens,
			}, nil
		}
		summary = strings.TrimSpace(summary)
		if summary != "" {
			break
		}
		// Empty response: drop oldest old turn and retry
		if len(oldTurns) > 1 {
			oldTurns = oldTurns[1:]
			oldConversation.Reset()
			for i, t := range oldTurns {
				oldConversation.WriteString(fmt.Sprintf("[Turn %d]\nUser: %s\nAssistant: %s\n\n", i+1, t.user, t.assistant))
			}
			summarizerMessages[0] = CompactMessage{
				Role:    "user",
				Content: "Here is the conversation to summarize:\n\n" + oldConversation.String(),
			}
		}
	}

	if summary == "" {
		// All retries exhausted with empty responses, fall back to naive
		naiveResult, naiveErr := CompactMessages(messages, opts.KeepRecentTurns)
		if naiveErr != nil {
			return nil, fmt.Errorf("compaction produced empty summary and naive fallback failed: %w", naiveErr)
		}
		return &LLMCompactResult{
			Summary:           naiveResult.Summary,
			RewrittenMessages: buildNaiveRewritten(messages, naiveResult, opts),
			RemovedTurns:      naiveResult.RemovedTurns,
			KeptTurns:         naiveResult.KeptTurns,
			OriginalTokens:    naiveResult.OriginalTokens,
			CompactTokens:     naiveResult.CompactTokens,
		}, nil
	}

	// Build the rewritten context:
	// 1. Recent messages (kept verbatim)
	// 2. Compaction summary (as user message with prefix)
	rewritten := buildRewrittenContext(recentTurns, summary, opts)

	compactTokens := 0
	for _, msg := range rewritten {
		compactTokens += TokenEstimate(msg.Content) + TokenEstimate(msg.Role)
	}

	return &LLMCompactResult{
		Summary:           summary,
		RewrittenMessages: rewritten,
		RemovedTurns:      len(oldTurns),
		KeptTurns:         len(recentTurns),
		OriginalTokens:    originalTokens,
		CompactTokens:     compactTokens,
	}, nil
}

// turn is a user+assistant pair for compaction grouping.
type turn struct {
	user      string
	assistant string
}

// groupIntoTurns groups messages into user+assistant pairs.
func groupIntoTurns(messages []CompactMessage) []turn {
	var turns []turn
	var current *turn
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			if current != nil {
				turns = append(turns, *current)
			}
			current = &turn{user: msg.Content}
		case "assistant":
			if current != nil {
				current.assistant = msg.Content
				turns = append(turns, *current)
				current = nil
			}
		}
	}
	if current != nil {
		turns = append(turns, *current)
	}
	return turns
}

// buildRewrittenContext constructs the compacted message list:
// recent messages (with head/tail split for user messages) + summary.
func buildRewrittenContext(recentTurns []turn, summary string, opts CompactOptions) []CompactMessage {
	var result []CompactMessage

	// Add recent turns verbatim
	for _, t := range recentTurns {
		if t.user != "" {
			result = append(result, CompactMessage{Role: "user", Content: t.user})
		}
		if t.assistant != "" {
			result = append(result, CompactMessage{Role: "assistant", Content: t.assistant})
		}
	}

	// Apply head/tail split to user messages if they exceed the budget
	result = applyHeadTailSplit(result, opts)

	// Append the compaction summary as a user message
	summaryText := compactionSummaryPrefix + "\n\n" + summary
	result = append(result, CompactMessage{Role: "user", Content: summaryText})

	return result
}

// applyHeadTailSplit applies the head/tail user message split when the total
// user message tokens exceed the budget. Non-user messages are kept as-is.
func applyHeadTailSplit(messages []CompactMessage, opts CompactOptions) []CompactMessage {
	// Collect user message indices and total tokens
	var userIndices []int
	totalUserTokens := 0
	for i, msg := range messages {
		if msg.Role == "user" {
			userIndices = append(userIndices, i)
			totalUserTokens += TokenEstimate(msg.Content)
		}
	}

	// If within budget, return as-is
	if totalUserTokens <= opts.UserMessageMaxTokens {
		return messages
	}

	// Split budget
	headBudget := opts.HeadTokens
	if headBudget > opts.UserMessageMaxTokens {
		headBudget = opts.UserMessageMaxTokens
	}
	tailBudget := opts.UserMessageMaxTokens - headBudget

	// Select tail messages (newest first)
	var tailIndices []int
	tailRemaining := tailBudget
	for i := len(userIndices) - 1; i >= 0; i-- {
		idx := userIndices[i]
		tokens := TokenEstimate(messages[idx].Content)
		if tokens <= tailRemaining {
			tailIndices = append([]int{idx}, tailIndices...)
			tailRemaining -= tokens
		} else {
			// Partial: keep the end of this message
			tailIndices = append([]int{idx}, tailIndices...)
			break
		}
	}

	// Build result: keep all non-user messages, and selected user messages
	// For simplicity, include all non-user messages and tail user messages
	tailSet := make(map[int]bool)
	for _, idx := range tailIndices {
		tailSet[idx] = true
	}

	var result []CompactMessage
	for i, msg := range messages {
		if msg.Role != "user" {
			result = append(result, msg)
		} else if tailSet[i] {
			result = append(result, msg)
		}
		// Head messages are omitted (covered by summary)
	}

	// Insert elision marker if any messages were dropped
	if len(tailIndices) < len(userIndices) {
		omittedTokens := 0
		for _, idx := range userIndices {
			if !tailSet[idx] {
				omittedTokens += TokenEstimate(messages[idx].Content)
			}
		}
		// Find position: after first non-user message or at start
		elisionMsg := CompactMessage{
			Role:    "system",
			Content: fmt.Sprintf(elisionMarkerTemplate, omittedTokens),
		}
		// Insert elision near the beginning (after first message if any)
		if len(result) > 0 {
			result = append([]CompactMessage{result[0], elisionMsg}, result[1:]...)
		} else {
			result = append([]CompactMessage{elisionMsg}, result...)
		}
	}

	return result
}

// buildNaiveRewritten constructs rewritten messages from naive compaction result.
func buildNaiveRewritten(messages []CompactMessage, result *CompactionResult, opts CompactOptions) []CompactMessage {
	turns := groupIntoTurns(messages)
	keepN := result.KeptTurns
	if keepN > len(turns) {
		keepN = len(turns)
	}
	recentTurns := turns[len(turns)-keepN:]

	var rewritten []CompactMessage
	// Add summary first
	rewritten = append(rewritten, CompactMessage{Role: "user", Content: result.Summary})
	// Add recent turns
	for _, t := range recentTurns {
		if t.user != "" {
			rewritten = append(rewritten, CompactMessage{Role: "user", Content: t.user})
		}
		if t.assistant != "" {
			rewritten = append(rewritten, CompactMessage{Role: "assistant", Content: t.assistant})
		}
	}
	return rewritten
}
