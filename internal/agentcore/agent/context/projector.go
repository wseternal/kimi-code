package context

import (
	"strings"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

// SyntheticToolResultText is the placeholder text used when a tool call has
// no matching result and one must be synthesized for wire validity.
const SyntheticToolResultText = "Tool result is not available in the current context. Do not assume the tool completed successfully."

// ProjectionAnomalyKind identifies the type of repair the projector applied.
type ProjectionAnomalyKind string

const (
	AnomalyToolResultReordered     ProjectionAnomalyKind = "tool_result_reordered"
	AnomalyToolResultSynthesized   ProjectionAnomalyKind = "tool_result_synthesized"
	AnomalyOrphanToolResultDropped ProjectionAnomalyKind = "orphan_tool_result_dropped"
	AnomalyDuplicateToolCallDropped ProjectionAnomalyKind = "duplicate_tool_call_dropped"
	AnomalyDuplicateToolResultDropped ProjectionAnomalyKind = "duplicate_tool_result_dropped"
	AnomalyLeadingNonUserDropped   ProjectionAnomalyKind = "leading_non_user_dropped"
	AnomalyConsecutiveAssistantsMerged ProjectionAnomalyKind = "consecutive_assistants_merged"
	AnomalyWhitespaceTextDropped   ProjectionAnomalyKind = "whitespace_text_dropped"
	AnomalyVacuousMessageDropped   ProjectionAnomalyKind = "vacuous_message_dropped"
)

// ProjectionAnomaly describes a repair the projector applied to keep the
// outgoing wire valid for strict providers.
type ProjectionAnomaly struct {
	Kind       ProjectionAnomalyKind
	ToolCallID string
	Role       string
	Trailing   bool
}

// ProjectOptions controls which projection passes to run.
type ProjectOptions struct {
	// SynthesizeMissing: when true, synthesize a tool_result for every
	// assistant tool_use whose result is absent — including trailing calls.
	SynthesizeMissing bool

	// DropOrphanResults: when true, drop tool_results with no matching
	// assistant tool_use anywhere in the messages.
	DropOrphanResults bool

	// DropLeadingNonUser: when true, drop leading messages until the
	// first one is a user turn.
	DropLeadingNonUser bool

	// MergeConsecutiveAssistants: when true, merge back-to-back assistant
	// messages into one.
	MergeConsecutiveAssistants bool

	// DedupeDuplicateToolCalls: when true, drop assistant tool calls whose
	// id already appeared earlier, and drop duplicate tool results.
	DedupeDuplicateToolCalls bool

	// OnAnomaly is called for every repair the projector applies.
	OnAnomaly func(ProjectionAnomaly)
}

// Project repairs a message array for strict LLM providers. It merges
// adjacent user messages, repairs tool exchange adjacency, and optionally
// runs strict-resend passes (dedup, orphan drop, leading non-user drop,
// consecutive assistant merge).
func Project(messages []kosong.Message, opts *ProjectOptions) []kosong.Message {
	if opts == nil {
		opts = &ProjectOptions{}
	}

	result := mergeAdjacentUserMessages(messages, opts.OnAnomaly)

	if opts.DedupeDuplicateToolCalls {
		result = dedupeDuplicateToolCalls(result, opts.OnAnomaly)
	}

	result = repairToolExchangeAdjacency(result, opts)

	if opts.MergeConsecutiveAssistants {
		result = mergeConsecutiveAssistantMessages(result, opts.OnAnomaly)
	}

	if opts.DropOrphanResults {
		result = dropOrphanToolResults(result, opts.OnAnomaly)
	}

	if opts.DropLeadingNonUser {
		result = dropLeadingNonUserMessages(result, opts.OnAnomaly)
	}

	return result
}

// TrimTrailingOpenToolExchange removes the trailing assistant+tool messages
// if the last non-tool message is an assistant with unclosed tool calls.
func TrimTrailingOpenToolExchange(messages []kosong.Message) []kosong.Message {
	lastNonTool := len(messages) - 1
	for lastNonTool >= 0 && messages[lastNonTool].Role == kosong.RoleTool {
		lastNonTool--
	}

	if lastNonTool < 0 {
		return nil
	}

	assistant := messages[lastNonTool]
	if assistant.Role != kosong.RoleAssistant || len(assistant.ToolCalls) == 0 {
		result := make([]kosong.Message, len(messages))
		copy(result, messages)
		return result
	}

	// Collect tool result IDs after the assistant
	trailingIDs := make(map[string]bool)
	for _, m := range messages[lastNonTool+1:] {
		if m.ToolCallID != nil {
			trailingIDs[*m.ToolCallID] = true
		}
	}

	// Check if all tool calls are answered
	allClosed := true
	for _, tc := range assistant.ToolCalls {
		if !trailingIDs[tc.ID] {
			allClosed = false
			break
		}
	}

	if allClosed {
		result := make([]kosong.Message, len(messages))
		copy(result, messages)
		return result
	}

	result := make([]kosong.Message, lastNonTool)
	copy(result, messages[:lastNonTool])
	return result
}

// ── Internal projection passes ──

func mergeAdjacentUserMessages(messages []kosong.Message, onAnomaly func(ProjectionAnomaly)) []kosong.Message {
	var out []kosong.Message
	for _, msg := range messages {
		prepared := prepareMessageForProjection(msg, onAnomaly)
		if prepared == nil {
			continue
		}

		if len(out) > 0 && canMergeUserMessage(*prepared) && canMergeUserMessage(out[len(out)-1]) {
			out[len(out)-1] = mergeTwoUserMessages(out[len(out)-1], *prepared)
			continue
		}
		out = append(out, *prepared)
	}
	return out
}

func prepareMessageForProjection(msg kosong.Message, onAnomaly func(ProjectionAnomaly)) *kosong.Message {
	// Skip partial messages
	if msg.Partial != nil && *msg.Partial {
		return nil
	}

	// Filter whitespace-only / empty text parts
	var filtered []kosong.ContentPart
	changed := false
	for i, part := range msg.Content {
		if part.Type == "text" && strings.TrimSpace(part.Text) == "" {
			if !changed {
				filtered = make([]kosong.ContentPart, i)
				copy(filtered, msg.Content[:i])
				changed = true
			}
			// Report non-empty whitespace-only blocks
			if len(part.Text) > 0 && onAnomaly != nil {
				onAnomaly(ProjectionAnomaly{Kind: AnomalyWhitespaceTextDropped, Role: string(msg.Role)})
			}
			continue
		}
		if changed {
			filtered = append(filtered, part)
		}
	}

	var next kosong.Message
	if changed {
		next = msg
		next.Content = filtered
	} else {
		next = msg
	}

	// Tool results must have content
	if next.Role == kosong.RoleTool && len(next.Content) == 0 {
		// Cannot empty a tool result — keep it with a placeholder
		next.Content = []kosong.ContentPart{kosong.NewTextPart("(empty result)")}
	}

	// Messages with tool declarations are intentionally content-free
	if len(next.Tools) > 0 {
		return &next
	}
	if len(next.ToolCalls) > 0 {
		return &next
	}
	if len(next.Content) == 0 {
		return nil
	}

	// Drop vacuous messages (all parts serialize to nothing)
	if allVacuous(next.Content) {
		if onAnomaly != nil {
			onAnomaly(ProjectionAnomaly{Kind: AnomalyVacuousMessageDropped, Role: string(next.Role)})
		}
		return nil
	}

	return &next
}

func allVacuous(parts []kosong.ContentPart) bool {
	for _, p := range parts {
		if !isVacuousContentPart(p) {
			return false
		}
	}
	return true
}

func isVacuousContentPart(part kosong.ContentPart) bool {
	switch part.Type {
	case "text":
		return strings.TrimSpace(part.Text) == ""
	case "think":
		return part.Encrypted == nil && strings.TrimSpace(part.Think) == ""
	}
	return false
}

func canMergeUserMessage(msg kosong.Message) bool {
	return msg.Role == kosong.RoleUser
}

func mergeTwoUserMessages(a, b kosong.Message) kosong.Message {
	aText := extractTextParts(a)
	bText := extractTextParts(b)

	// Skip empty text when one side has no text
	var mergedText string
	if aText == "" {
		mergedText = bText
	} else if bText == "" {
		mergedText = aText
	} else {
		mergedText = aText + "\n\n" + bText
	}
	merged := kosong.NewTextPart(mergedText)

	var nonText []kosong.ContentPart
	for _, p := range a.Content {
		if p.Type != "text" {
			nonText = append(nonText, p)
		}
	}
	for _, p := range b.Content {
		if p.Type != "text" {
			nonText = append(nonText, p)
		}
	}

	content := []kosong.ContentPart{merged}
	content = append(content, nonText...)

	// Preserve tool declarations from either message
	var tools []kosong.Tool
	tools = append(tools, a.Tools...)
	tools = append(tools, b.Tools...)

	return kosong.Message{
		Role:      kosong.RoleUser,
		Content:   content,
		ToolCalls: []kosong.ToolCall{},
		Tools:     tools,
	}
}

func extractTextParts(msg kosong.Message) string {
	var sb strings.Builder
	for _, p := range msg.Content {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func repairToolExchangeAdjacency(messages []kosong.Message, opts *ProjectOptions) []kosong.Message {
	// Find last non-tool message index
	lastNonTool := len(messages) - 1
	for lastNonTool >= 0 && messages[lastNonTool].Role == kosong.RoleTool {
		lastNonTool--
	}

	out := make([]kosong.Message, 0, len(messages))
	consumed := make(map[int]bool)

	for i := 0; i < len(messages); i++ {
		if consumed[i] {
			continue
		}
		msg := messages[i]
		if msg.Role != kosong.RoleAssistant || len(msg.ToolCalls) == 0 {
			out = append(out, msg)
			continue
		}

		out = append(out, msg)
		pending := make(map[string]bool)
		for _, tc := range msg.ToolCalls {
			pending[tc.ID] = true
		}

		foreignBetween := false
		for j := i + 1; j < len(messages) && len(pending) > 0; j++ {
			if consumed[j] {
				continue
			}
			next := messages[j]
			if next.Role == kosong.RoleTool && next.ToolCallID != nil && pending[*next.ToolCallID] {
				out = append(out, next)
				consumed[j] = true
				delete(pending, *next.ToolCallID)
				if foreignBetween && opts.OnAnomaly != nil {
					opts.OnAnomaly(ProjectionAnomaly{Kind: AnomalyToolResultReordered, ToolCallID: *next.ToolCallID})
				}
			} else {
				foreignBetween = true
			}
		}

		// Close any tool call whose result is absent
		isMidHistory := i < lastNonTool
		if opts.SynthesizeMissing || isMidHistory {
			for missingID := range pending {
				out = append(out, makeSyntheticToolResult(missingID))
				if opts.OnAnomaly != nil {
					opts.OnAnomaly(ProjectionAnomaly{
						Kind:       AnomalyToolResultSynthesized,
						ToolCallID: missingID,
						Trailing:   !isMidHistory,
					})
				}
			}
		}
	}
	return out
}

func dedupeDuplicateToolCalls(messages []kosong.Message, onAnomaly func(ProjectionAnomaly)) []kosong.Message {
	seenCallIDs := make(map[string]bool)
	seenResultIDs := make(map[string]bool)
	var out []kosong.Message

	for _, msg := range messages {
		if msg.Role == kosong.RoleAssistant && len(msg.ToolCalls) > 0 {
			var kept []kosong.ToolCall
			for _, tc := range msg.ToolCalls {
				if seenCallIDs[tc.ID] {
					if onAnomaly != nil {
						onAnomaly(ProjectionAnomaly{Kind: AnomalyDuplicateToolCallDropped, ToolCallID: tc.ID})
					}
					continue
				}
				seenCallIDs[tc.ID] = true
				kept = append(kept, tc)
			}

			if len(kept) == len(msg.ToolCalls) {
				out = append(out, msg)
			} else if len(kept) > 0 || !allVacuous(msg.Content) {
				dup := msg
				dup.ToolCalls = kept
				if dup.ToolCalls == nil {
					dup.ToolCalls = []kosong.ToolCall{}
				}
				out = append(out, dup)
			} else if len(msg.Content) > 0 {
				if onAnomaly != nil {
					onAnomaly(ProjectionAnomaly{Kind: AnomalyVacuousMessageDropped, Role: string(msg.Role)})
				}
			}
			continue
		}

		if msg.Role == kosong.RoleTool && msg.ToolCallID != nil {
			if seenResultIDs[*msg.ToolCallID] {
				if onAnomaly != nil {
					onAnomaly(ProjectionAnomaly{Kind: AnomalyDuplicateToolResultDropped, ToolCallID: *msg.ToolCallID})
				}
				continue
			}
			seenResultIDs[*msg.ToolCallID] = true
		}
		out = append(out, msg)
	}
	return out
}

func dropOrphanToolResults(messages []kosong.Message, onAnomaly func(ProjectionAnomaly)) []kosong.Message {
	toolUseIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == kosong.RoleAssistant {
			for _, tc := range msg.ToolCalls {
				toolUseIDs[tc.ID] = true
			}
		}
	}

	var out []kosong.Message
	for _, msg := range messages {
		if msg.Role != kosong.RoleTool || msg.ToolCallID == nil {
			out = append(out, msg)
			continue
		}
		if toolUseIDs[*msg.ToolCallID] {
			out = append(out, msg)
			continue
		}
		if onAnomaly != nil {
			onAnomaly(ProjectionAnomaly{Kind: AnomalyOrphanToolResultDropped, ToolCallID: *msg.ToolCallID})
		}
	}
	return out
}

func mergeConsecutiveAssistantMessages(messages []kosong.Message, onAnomaly func(ProjectionAnomaly)) []kosong.Message {
	var out []kosong.Message
	for _, msg := range messages {
		if len(out) > 0 && out[len(out)-1].Role == kosong.RoleAssistant && msg.Role == kosong.RoleAssistant {
			prev := &out[len(out)-1]
			prev.Content = append(prev.Content, msg.Content...)
			prev.ToolCalls = append(prev.ToolCalls, msg.ToolCalls...)
			if onAnomaly != nil {
				onAnomaly(ProjectionAnomaly{Kind: AnomalyConsecutiveAssistantsMerged})
			}
			continue
		}
		out = append(out, msg)
	}
	return out
}

func dropLeadingNonUserMessages(messages []kosong.Message, onAnomaly func(ProjectionAnomaly)) []kosong.Message {
	start := 0
	for start < len(messages) && messages[start].Role != kosong.RoleUser {
		if onAnomaly != nil {
			onAnomaly(ProjectionAnomaly{Kind: AnomalyLeadingNonUserDropped, Role: string(messages[start].Role)})
		}
		start++
	}
	if start == 0 {
		result := make([]kosong.Message, len(messages))
		copy(result, messages)
		return result
	}
	result := make([]kosong.Message, len(messages)-start)
	copy(result, messages[start:])
	return result
}

func makeSyntheticToolResult(toolCallID string) kosong.Message {
	id := toolCallID
	return kosong.Message{
		Role:       kosong.RoleTool,
		Content:    []kosong.ContentPart{kosong.NewTextPart(SyntheticToolResultText)},
		ToolCalls:  []kosong.ToolCall{},
		ToolCallID: &id,
	}
}
