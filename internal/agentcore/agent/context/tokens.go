package context

import (
	"encoding/json"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

// MediaTokenEstimate is the fixed token cost for image/audio/video parts whose
// real size cannot be cheaply derived. Industry standard ~2000 tokens per media item.
const MediaTokenEstimate = 2000

// TokenEstimate estimates token count from text using a character-class heuristic:
//   - ASCII (~4 chars per token)
//   - Non-ASCII / CJK (~1 char per token)
//
// Uses range iteration to correctly count code points (not bytes) for CJK text.
func TokenEstimate(text string) int {
	if text == "" {
		return 0
	}
	var ascii, nonASCII int
	for _, r := range text {
		if r <= 127 {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII
}

// EstimateContentPart estimates tokens for a single kosong.ContentPart.
func EstimateContentPart(part kosong.ContentPart) int {
	switch part.Type {
	case "text":
		return TokenEstimate(part.Text)
	case "think":
		return TokenEstimate(part.Think)
	case "image_url", "audio_url", "video_url":
		return MediaTokenEstimate
	default:
		return 0
	}
}

// EstimateMessage estimates tokens for a single message (content + tool calls + tool defs).
func EstimateMessage(msg *kosong.Message) int {
	total := TokenEstimate(string(msg.Role))
	for _, part := range msg.Content {
		total += EstimateContentPart(part)
	}
	for _, tc := range msg.ToolCalls {
		total += TokenEstimate(tc.Name)
		if tc.Arguments != nil {
			total += TokenEstimate(*tc.Arguments)
		}
	}
	if len(msg.Tools) > 0 {
		total += EstimateTools(msg.Tools)
	}
	return total
}

// EstimateMessages estimates tokens for a slice of messages.
func EstimateMessages(msgs []kosong.Message) int {
	total := 0
	for i := range msgs {
		total += EstimateMessage(&msgs[i])
	}
	return total
}

// EstimateTools estimates tokens for tool definitions (name + description + JSON schema).
func EstimateTools(tools []kosong.Tool) int {
	total := 0
	for _, tool := range tools {
		total += TokenEstimate(tool.Name)
		total += TokenEstimate(tool.Description)
		if tool.Parameters != nil {
			data, err := json.Marshal(tool.Parameters)
			if err == nil {
				total += TokenEstimate(string(data))
			}
		}
	}
	return total
}
