// Package providers implements LLM provider adapters.
// tool_call_id.go provides tool call ID normalization and sanitization.
package providers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

// ToolCallIDPolicy defines how tool call IDs are normalized for a provider.
type ToolCallIDPolicy struct {
	Normalize func(id string) string
	MaxLength int
}

var nonAlphanumericUnderscoreDash = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// SanitizeToolCallId replaces invalid characters with underscores and
// truncates to maxLength. Valid chars: [a-zA-Z0-9_-].
func SanitizeToolCallId(id string, maxLength int) string {
	if id == "" {
		return id
	}
	sanitized := nonAlphanumericUnderscoreDash.ReplaceAllString(id, "_")
	if maxLength > 0 && len(sanitized) > maxLength {
		sanitized = sanitized[:maxLength]
	}
	return sanitized
}

// SanitizeOpenAIResponsesCallId strips trailing `|` suffix before sanitizing.
// The Responses API appends `|` to some IDs.
func SanitizeOpenAIResponsesCallId(id string, maxLength int) string {
	// Strip trailing `|` variants
	id = strings.TrimRight(id, "|")
	// Also strip `|<suffix>` patterns
	if idx := strings.LastIndex(id, "|"); idx >= 0 {
		id = id[:idx]
	}
	return SanitizeToolCallId(id, maxLength)
}

// NormalizeToolCallIdsForProvider walks all messages and normalizes
// tool call IDs and tool_call_id references according to the policy.
// Ensures uniqueness by appending _2, _3, etc. for duplicates.
func NormalizeToolCallIdsForProvider(messages []kosong.Message, policy ToolCallIDPolicy) []kosong.Message {
	if policy.Normalize == nil {
		return messages
	}

	// Build ID mapping: old -> new
	idMap := make(map[string]string)
	usedIDs := make(map[string]bool)

	// First pass: collect all IDs and build mapping
	for _, msg := range messages {
		for _, tc := range msg.ToolCalls {
			oldID := tc.ID
			if _, exists := idMap[oldID]; exists {
				continue
			}
			newID := policy.Normalize(oldID)
			// Ensure uniqueness
			newID = ensureUnique(newID, usedIDs)
			idMap[oldID] = newID
			usedIDs[newID] = true
		}
		if msg.ToolCallID != nil {
			oldID := *msg.ToolCallID
			if _, exists := idMap[oldID]; !exists {
				newID := policy.Normalize(oldID)
				newID = ensureUnique(newID, usedIDs)
				idMap[oldID] = newID
				usedIDs[newID] = true
			}
		}
	}

	// Second pass: apply mapping
	result := make([]kosong.Message, len(messages))
	for i, msg := range messages {
		result[i] = msg

		// Normalize tool call IDs
		if len(msg.ToolCalls) > 0 {
			tcs := make([]kosong.ToolCall, len(msg.ToolCalls))
			copy(tcs, msg.ToolCalls)
			for j := range tcs {
				if newID, ok := idMap[tcs[j].ID]; ok {
					tcs[j].ID = newID
				}
			}
			result[i].ToolCalls = tcs
		}

		// Normalize tool_call_id references
		if msg.ToolCallID != nil {
			if newID, ok := idMap[*msg.ToolCallID]; ok {
				copied := newID
				result[i].ToolCallID = &copied
			}
		}
	}

	return result
}

// ensureUnique appends _2, _3, etc. if the ID is already in use.
func ensureUnique(id string, used map[string]bool) string {
	if !used[id] {
		return id
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", id, suffix)
		if !used[candidate] {
			return candidate
		}
	}
}

// DefaultKimiPolicy returns the tool call ID policy for Kimi/Anthropic providers.
func DefaultKimiPolicy() ToolCallIDPolicy {
	return ToolCallIDPolicy{
		Normalize: func(id string) string { return SanitizeToolCallId(id, 64) },
		MaxLength: 64,
	}
}

// DefaultOpenAIResponsesPolicy returns the tool call ID policy for OpenAI Responses API.
func DefaultOpenAIResponsesPolicy() ToolCallIDPolicy {
	return ToolCallIDPolicy{
		Normalize: func(id string) string { return SanitizeOpenAIResponsesCallId(id, 64) },
		MaxLength: 64,
	}
}
