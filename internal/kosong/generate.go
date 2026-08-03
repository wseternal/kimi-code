package kosong

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Errors for the kosong package.
var (
	ErrProviderAborted = errors.New("kosong: provider aborted")
	ErrStreamTruncated = errors.New("kosong: stream truncated")
	ErrToolCallInvalid = errors.New("kosong: invalid tool call")
)

// GenerateResult is the output of a single GenerateCall.
type GenerateResult struct {
	// ID is the provider-assigned response identifier, or nil if unavailable.
	ID *string
	// Message is the fully-assembled assistant message.
	Message *Message
	// Usage is token usage for this generation, or nil if not reported.
	Usage *TokenUsage
	// FinishReason is the normalized finish reason, or nil if not emitted.
	FinishReason *FinishReason
	// RawFinishReason is the raw provider-specific finish reason string.
	RawFinishReason *string
	// TraceID is the provider trace identifier from x-trace-id header, or nil.
	TraceID *string
}

// GenerateCallbacks provides optional streaming callbacks.
type GenerateCallbacks struct {
	// OnMessagePart fires for each streamed part (before merging).
	OnMessagePart func(part StreamedMessagePart)
	// OnToolCall fires for each fully-assembled tool call after the stream drains.
	OnToolCall func(toolCall ToolCall)
}

// GenerateCall is the high-level generation function. It calls the provider's
// Generate method, assembles the stream into a Message, validates the response
// is non-empty, fires callbacks, and returns a GenerateResult.
func GenerateCall(
	ctx context.Context,
	provider ChatProvider,
	systemPrompt string,
	tools []Tool,
	history []Message,
	callbacks *GenerateCallbacks,
	opts *GenerateOptions,
) (*GenerateResult, error) {
	// Strip deferred tools from the wire request
	wireTools := filterDeferredTools(tools)

	// Note: OnRequestStart is fired by individual provider Generate() methods
	// (right before the HTTP request), not here, to avoid double invocation.

	stream, err := provider.Generate(ctx, systemPrompt, wireTools, history, opts)
	if err != nil {
		return nil, err
	}

	// Early trace ID capture
	if stream.TraceID != nil && opts != nil && opts.OnTraceID != nil {
		opts.OnTraceID(stream.TraceID)
	}

	// Assemble the stream
	msg, err := generateWithCallbacks(ctx, stream, callbacks)
	if err != nil {
		return nil, err
	}

	// Validate non-empty response
	if len(msg.Content) == 0 && len(msg.ToolCalls) == 0 {
		return nil, NewAPIEmptyResponseError(
			fmt.Sprintf("The API returned an empty response (no content, no tool calls).%s Provider: %s, model: %s",
				formatFinishReasonHint(stream), provider.Name(), provider.ModelName()),
			stream.FinishReason,
			stream.RawFinishReason,
		)
	}

	// Validate non-think-only response
	hasThink := false
	hasText := false
	for _, p := range msg.Content {
		if p.Type == "think" {
			hasThink = true
		}
		if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
			hasText = true
		}
	}
	hasToolCalls := len(msg.ToolCalls) > 0

	if hasThink && !hasText && !hasToolCalls {
		return nil, NewAPIEmptyResponseError(
			fmt.Sprintf("The API returned a response containing only thinking content "+
				"without any text or tool calls. This usually indicates the "+
				"stream was interrupted or the output token budget was exhausted "+
				"during reasoning.%s Provider: %s, model: %s",
				formatFinishReasonHint(stream), provider.Name(), provider.ModelName()),
			stream.FinishReason,
			stream.RawFinishReason,
		)
	}

	// Fire deferred onToolCall callbacks
	if callbacks != nil && callbacks.OnToolCall != nil {
		for _, tc := range msg.ToolCalls {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			callbacks.OnToolCall(tc)
		}
	}

	result := &GenerateResult{
		ID:              stream.ID,
		Message:         msg,
		Usage:           stream.Usage,
		FinishReason:    stream.FinishReason,
		RawFinishReason: stream.RawFinishReason,
	}
	if stream.TraceID != nil {
		result.TraceID = stream.TraceID
	}
	return result, nil
}

// generateWithCallbacks assembles a stream into a Message, firing OnMessagePart
// for each part as it arrives. Supports parallel tool call routing via
// index-based argument delta routing (Gap #13).
func generateWithCallbacks(ctx context.Context, stream *StreamedMessage, callbacks *GenerateCallbacks) (*Message, error) {
	var content []ContentPart
	var toolCalls []ToolCall
	var pending *StreamedMessagePart

	// Index-based routing for parallel tool call argument deltas.
	// Maps provider streaming index (number or string) to position in toolCalls slice.
	toolCallIndexMap := make(map[interface{}]int)

	for part := range stream.Parts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if part.Type == "finish" {
			if part.FinishReason != nil {
				rawFinish := *part.FinishReason
				stream.RawFinishReason = &rawFinish
			}
			continue
		}
		if part.Type == "usage" && part.Usage != nil {
			stream.Usage = part.Usage
			continue
		}

		// Fire callback
		if callbacks != nil && callbacks.OnMessagePart != nil {
			callbacks.OnMessagePart(part)
		}

		// Index-based routing for parallel tool call argument deltas.
		// When a tool_call_part arrives with an index that maps to an existing
		// tool call, route the argument delta directly to that call, bypassing
		// the sequential pending/merge flow. This prevents argument
		// cross-contamination when the provider interleaves deltas from
		// multiple concurrent tool calls.
		if part.Type == "tool_call_part" && part.Index != nil && pending != nil && pending.Type == "function" {
			if !isPendingToolCallAtIndex(pending, part.Index) {
				if arrayIdx, ok := toolCallIndexMap[part.Index]; ok && arrayIdx < len(toolCalls) {
					if part.ArgumentsPart != nil {
						if toolCalls[arrayIdx].Arguments == nil {
							toolCalls[arrayIdx].Arguments = part.ArgumentsPart
						} else {
							combined := *toolCalls[arrayIdx].Arguments + *part.ArgumentsPart
							toolCalls[arrayIdx].Arguments = &combined
						}
					}
					continue
				}
				// Unknown index — register it and fall through to sequential logic.
				// The pending function header will be flushed, then the argument
				// delta will be associated correctly.
			}
		}

		// Try to merge with pending part
		if pending != nil {
			if MergeInPlace(pending, &part) {
				continue
			}
			flushPart(pending, &content, &toolCalls, toolCallIndexMap)
		}

		// Set new pending
		cp := part
		pending = &cp
	}

	// Flush final pending
	if pending != nil {
		flushPart(pending, &content, &toolCalls, toolCallIndexMap)
	}

	msg := &Message{
		Role:      RoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
	}
	return msg, nil
}

// filterDeferredTools strips deferred tools from the list sent to the provider.
func filterDeferredTools(tools []Tool) []Tool {
	hasDeferred := false
	for _, t := range tools {
		if t.Deferred {
			hasDeferred = true
			break
		}
	}
	if !hasDeferred {
		return tools
	}
	var result []Tool
	for _, t := range tools {
		if !t.Deferred {
			result = append(result, t)
		}
	}
	return result
}

// formatFinishReasonHint produces a diagnostic string for empty response errors.
func formatFinishReasonHint(stream *StreamedMessage) string {
	if stream.FinishReason == nil && stream.RawFinishReason == nil {
		return ""
	}
	raw := ""
	if stream.RawFinishReason != nil {
		raw = ", rawFinishReason=" + *stream.RawFinishReason
	}
	fr := "unknown"
	if stream.FinishReason != nil {
		fr = string(*stream.FinishReason)
	}
	filteredHint := ""
	if stream.FinishReason != nil && *stream.FinishReason == FinishFiltered {
		filteredHint = " The provider filtered the response before visible output was emitted."
	}
	return fmt.Sprintf(" Provider stop details: finishReason=%s%s.%s", fr, raw, filteredHint)
}

// Generate consumes a StreamedMessage and assembles the final Message.
// It merges compatible consecutive parts and collects tool calls.
// Deprecated: Use GenerateCall for the high-level wrapper with validation.
func Generate(ctx context.Context, stream *StreamedMessage) (*Message, error) {
	return generateWithCallbacks(ctx, stream, nil)
}

// flushPart converts a StreamedMessagePart to ContentPart or ToolCall and appends.
// For function-type parts, registers the index mapping for parallel tool call routing.
func flushPart(part *StreamedMessagePart, content *[]ContentPart, toolCalls *[]ToolCall, toolCallIndexMap map[interface{}]int) {
	switch part.Type {
	case "text":
		*content = append(*content, ContentPart{Type: "text", Text: part.Text})
	case "think":
		*content = append(*content, ContentPart{Type: "think", Think: part.Think, Encrypted: part.Encrypted})
	case "image_url":
		*content = append(*content, ContentPart{Type: "image_url", ImageURL: part.ImageURL})
	case "audio_url":
		*content = append(*content, ContentPart{Type: "audio_url", AudioURL: part.AudioURL})
	case "video_url":
		*content = append(*content, ContentPart{Type: "video_url", VideoURL: part.VideoURL})
	case "function":
		idx := len(*toolCalls)
		*toolCalls = append(*toolCalls, ToolCall{
			Type:      "function",
			ID:        part.ID,
			Name:      part.Name,
			Arguments: part.Arguments,
			Extras:    part.Extras,
		})
		// Register the index mapping for parallel tool call routing.
		if part.Index != nil {
			toolCallIndexMap[part.Index] = idx
		}
	}
}

// isPendingToolCallAtIndex checks whether the pending part is a tool call at
// the given index. Used for parallel tool call routing to determine if an
// argument delta belongs to the currently pending tool call or a different one.
func isPendingToolCallAtIndex(pending *StreamedMessagePart, index interface{}) bool {
	if pending == nil || pending.Type != "function" {
		return false
	}
	return pending.Index == index
}

// CollectParts drains a stream and returns all parts as a slice.
// Useful for testing and debugging.
func CollectParts(ctx context.Context, stream *StreamedMessage) ([]StreamedMessagePart, error) {
	var parts []StreamedMessagePart
	for part := range stream.Parts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			parts = append(parts, part)
		}
	}
	return parts, nil
}
