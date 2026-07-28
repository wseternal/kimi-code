package kosong

// TokenUsage is a token usage breakdown for a single LLM generation.
// TokenUsage records token consumption for a single LLM generation call.
type TokenUsage struct {
	InputOther         int `json:"inputOther"`
	Output             int `json:"output"`
	InputCacheRead     int `json:"inputCacheRead"`
	InputCacheCreation int `json:"inputCacheCreation"`
	// ReasoningTokens is the number of output tokens spent on reasoning/thinking.
	// This is a subset of Output and is reported separately by some APIs
	// (e.g. OpenAI's completion_tokens_details.reasoning_tokens).
	ReasoningTokens int `json:"reasoningTokens,omitempty"`
}

// InputTotal computes total input tokens.
func (u TokenUsage) InputTotal() int {
	return u.InputOther + u.InputCacheRead + u.InputCacheCreation
}

// GrandTotal computes grand total tokens.
func (u TokenUsage) GrandTotal() int {
	return u.InputTotal() + u.Output
}

// EmptyUsage returns a zero-valued TokenUsage.
func EmptyUsage() TokenUsage {
	return TokenUsage{}
}

// AddUsage sums two TokenUsage values.
func AddUsage(a, b TokenUsage) TokenUsage {
	return TokenUsage{
		InputOther:         a.InputOther + b.InputOther,
		Output:             a.Output + b.Output,
		InputCacheRead:     a.InputCacheRead + b.InputCacheRead,
		InputCacheCreation: a.InputCacheCreation + b.InputCacheCreation,
		ReasoningTokens:    a.ReasoningTokens + b.ReasoningTokens,
	}
}
