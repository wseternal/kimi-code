package kosong

import (
	"context"
	"errors"
)

// Errors for the kosong package.
var (
	ErrProviderAborted = errors.New("kosong: provider aborted")
	ErrStreamTruncated = errors.New("kosong: stream truncated")
	ErrToolCallInvalid = errors.New("kosong: invalid tool call")
)

// Generate consumes a StreamedMessage and assembles the final Message.
// It merges compatible consecutive parts and collects tool calls.
func Generate(ctx context.Context, stream *StreamedMessage) (*Message, error) {
	var content []ContentPart
	var toolCalls []ToolCall
	var pending *StreamedMessagePart

	for part := range stream.Parts {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Capture finish reason and usage metadata from stream parts
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

		// Try to merge with pending part
		if pending != nil {
			if MergeInPlace(pending, &part) {
				continue
			}
			// Flush pending
			flushPart(pending, &content, &toolCalls)
		}

		// Set new pending
		pending = &part
	}

	// Flush final pending
	if pending != nil {
		flushPart(pending, &content, &toolCalls)
	}

	// Build final message
	msg := &Message{
		Role:      RoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
	}

	return msg, nil
}

// flushPart converts a StreamedMessagePart to ContentPart or ToolCall and appends.
func flushPart(part *StreamedMessagePart, content *[]ContentPart, toolCalls *[]ToolCall) {
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
		*toolCalls = append(*toolCalls, ToolCall{
			Type:      "function",
			ID:        part.ID,
			Name:      part.Name,
			Arguments: part.Arguments,
			Extras:    part.Extras,
		})
	}
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
