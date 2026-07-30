package kosong

import (
	"errors"
	"fmt"
	"testing"
)

func TestChatProviderError(t *testing.T) {
	err := NewChatProviderError("test error")
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got %q", err.Error())
	}

	var base *ChatProviderError
	if !errors.As(err, &base) {
		t.Error("errors.As should match ChatProviderError")
	}
}

func TestAPIStatusError(t *testing.T) {
	reqID := "req-123"
	traceID := "trace-456"
	retryMs := int64(5000)
	err := NewAPIStatusError(500, "internal error", &reqID, &retryMs, &traceID)

	if err.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", err.StatusCode)
	}
	if *err.RequestID != "req-123" {
		t.Errorf("expected requestID 'req-123', got %q", *err.RequestID)
	}
	if *err.RetryAfterMs != 5000 {
		t.Errorf("expected retryAfterMs 5000, got %d", *err.RetryAfterMs)
	}
	if *err.TraceID != "trace-456" {
		t.Errorf("expected traceID 'trace-456', got %q", *err.TraceID)
	}

	// errors.As should match the base type
	var base *ChatProviderError
	if !errors.As(err, &base) {
		t.Error("errors.As should match ChatProviderError via APIStatusError")
	}

	var status *APIStatusError
	if !errors.As(err, &status) {
		t.Error("errors.As should match APIStatusError")
	}
}

func TestAPIStatusErrorInheritance(t *testing.T) {
	reqID := "req-1"

	// APIContextOverflowError embeds APIStatusError
	overflow := NewAPIContextOverflowError(400, "context length exceeded", &reqID, nil, nil)
	var statusErr *APIStatusError
	if !errors.As(overflow, &statusErr) {
		t.Error("APIContextOverflowError should match APIStatusError")
	}
	if statusErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", statusErr.StatusCode)
	}

	// APIProviderRateLimitError is status 429
	rateLimit := NewAPIProviderRateLimitError("rate limited", &reqID, nil, nil)
	if rateLimit.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", rateLimit.StatusCode)
	}
	if !errors.As(rateLimit, &statusErr) {
		t.Error("APIProviderRateLimitError should match APIStatusError")
	}

	// APIProviderQuotaExhaustedError is status 429 but NOT a rate limit
	quota := NewAPIProviderQuotaExhaustedError("insufficient_quota", &reqID, nil, nil)
	if quota.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", quota.StatusCode)
	}

	// APIRequestTooLargeError
	tooLarge := NewAPIRequestTooLargeError(413, "request entity too large", &reqID, nil, nil)
	if tooLarge.StatusCode != 413 {
		t.Errorf("expected status 413, got %d", tooLarge.StatusCode)
	}
}

func TestIsRetryableGenerateError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"connection error", NewAPIConnectionError("connection refused"), true},
		{"timeout error", NewAPITimeoutError("deadline exceeded"), true},
		{"empty response", NewAPIEmptyResponseError("empty", nil, nil), true},
		{"quota exhausted (429, not retryable)", NewAPIProviderQuotaExhaustedError("insufficient_quota", nil, nil, nil), false},
		{"rate limit (429, retryable)", NewAPIProviderRateLimitError("too many requests", nil, nil, nil), true},
		{"500 server error", NewAPIStatusError(500, "internal error", nil, nil, nil), true},
		{"401 unauthorized (not retryable)", NewAPIStatusError(401, "unauthorized", nil, nil, nil), false},
		{"403 forbidden (not retryable)", NewAPIStatusError(403, "forbidden", nil, nil, nil), false},
		{"408 timeout (retryable)", NewAPIStatusError(408, "request timeout", nil, nil, nil), true},
		{"529 overloaded (retryable)", NewAPIStatusError(529, "overloaded", nil, nil, nil), true},
		{"base ChatProviderError (retryable)", NewChatProviderError("unknown"), true},
		{"context overflow (retryable via status)", NewAPIContextOverflowError(400, "context length exceeded", nil, nil, nil), false},
		{"wrapped error", fmt.Errorf("wrapped: %w", NewAPITimeoutError("timeout")), true},
		{"non-provider error", fmt.Errorf("random error"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsRetryableGenerateError(tc.err)
			if got != tc.expected {
				t.Errorf("IsRetryableGenerateError(%v) = %v, want %v", tc.err, got, tc.expected)
			}
		})
	}
}

func TestClassifyBaseAPIError(t *testing.T) {
	tests := []struct {
		msg    string
		check  func(error) bool
	}{
		{"connection refused", func(err error) bool { var e *APIConnectionError; return errors.As(err, &e) }},
		{"operation timed out", func(err error) bool { var e *APITimeoutError; return errors.As(err, &e) }},
		{"deadline exceeded", func(err error) bool { var e *APITimeoutError; return errors.As(err, &e) }},
		{"network unreachable", func(err error) bool { var e *APIConnectionError; return errors.As(err, &e) }},
		{"stream terminated", func(err error) bool { var e *APIConnectionError; return errors.As(err, &e) }},
		{"unknown error", func(err error) bool { var e *ChatProviderError; return errors.As(err, &e) }},
	}

	for _, tc := range tests {
		t.Run(tc.msg, func(t *testing.T) {
			err := ClassifyBaseAPIError(tc.msg)
			if !tc.check(err) {
				t.Errorf("ClassifyBaseAPIError(%q) returned %T, unexpected type", tc.msg, err)
			}
		})
	}
}

func TestIsContextOverflowStatusError(t *testing.T) {
	tests := []struct {
		code    int
		msg     string
		matched bool
	}{
		{400, "context_length_exceeded", true},
		{400, "context window exceeded", true},
		{413, "prompt is too long for maximum", true},
		{422, "maximum context length exceeded", true},
		{400, "invalid parameter", false},
		{500, "context length exceeded", false}, // wrong status
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d_%s", tc.code, tc.msg), func(t *testing.T) {
			got := IsContextOverflowStatusError(tc.code, tc.msg)
			if got != tc.matched {
				t.Errorf("IsContextOverflowStatusError(%d, %q) = %v, want %v", tc.code, tc.msg, got, tc.matched)
			}
		})
	}
}

func TestIsQuotaExhaustedMessage(t *testing.T) {
	tests := []struct {
		msg     string
		matched bool
	}{
		{"exceeded_current_quota_error", true},
		{"insufficient_quota", true},
		{"Your account is suspended due to insufficient balance", true},
		{"too many requests", false},
		{"rate limited", false},
	}

	for _, tc := range tests {
		t.Run(tc.msg, func(t *testing.T) {
			got := IsQuotaExhaustedMessage(tc.msg)
			if got != tc.matched {
				t.Errorf("IsQuotaExhaustedMessage(%q) = %v, want %v", tc.msg, got, tc.matched)
			}
		})
	}
}

func TestNormalizeAPIStatusError(t *testing.T) {
	// 429 with quota exhaustion message → APIProviderQuotaExhaustedError
	err := NormalizeAPIStatusError(429, "exceeded_current_quota_error", nil, nil, nil)
	var quota *APIProviderQuotaExhaustedError
	if !errors.As(err, &quota) {
		t.Errorf("expected APIProviderQuotaExhaustedError, got %T", err)
	}

	// 429 with rate limit → APIProviderRateLimitError
	err = NormalizeAPIStatusError(429, "too many requests", nil, nil, nil)
	var rateLimit *APIProviderRateLimitError
	if !errors.As(err, &rateLimit) {
		t.Errorf("expected APIProviderRateLimitError, got %T", err)
	}

	// 400 with context overflow → APIContextOverflowError
	err = NormalizeAPIStatusError(400, "context_length_exceeded", nil, nil, nil)
	var overflow *APIContextOverflowError
	if !errors.As(err, &overflow) {
		t.Errorf("expected APIContextOverflowError, got %T", err)
	}

	// 413 with request too large → APIRequestTooLargeError
	err = NormalizeAPIStatusError(413, "request entity too large", nil, nil, nil)
	var tooLarge *APIRequestTooLargeError
	if !errors.As(err, &tooLarge) {
		t.Errorf("expected APIRequestTooLargeError, got %T", err)
	}

	// 401 → plain APIStatusError
	err = NormalizeAPIStatusError(401, "unauthorized", nil, nil, nil)
	var statusErr *APIStatusError
	if !errors.As(err, &statusErr) {
		t.Errorf("expected APIStatusError, got %T", err)
	}
	// Should NOT be any subclass
	var overflowErr *APIContextOverflowError
	if errors.As(err, &overflowErr) {
		t.Error("401 should not be APIContextOverflowError")
	}
}

func TestParseRetryAfterMs(t *testing.T) {
	tests := []struct {
		raw      string
		expected *int64
	}{
		{"", nil},
		{"5", ptrInt64(5000)},
		{"0", ptrInt64(0)},
		{"120", ptrInt64(120000)},
		{"Wed, 21 Oct 2025 07:28:00 GMT", nil}, // HTTP-date
		{"abc", nil},
		{"-1", nil}, // negative after parse fails on '-'
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got := ParseRetryAfterMs(tc.raw)
			if tc.expected == nil && got != nil {
				t.Errorf("ParseRetryAfterMs(%q) = %v, want nil", tc.raw, *got)
			}
			if tc.expected != nil && got == nil {
				t.Errorf("ParseRetryAfterMs(%q) = nil, want %d", tc.raw, *tc.expected)
			}
			if tc.expected != nil && got != nil && *got != *tc.expected {
				t.Errorf("ParseRetryAfterMs(%q) = %d, want %d", tc.raw, *got, *tc.expected)
			}
		})
	}
}

func TestIsImageFormatError(t *testing.T) {
	// Status 400 with image format message
	err := NormalizeAPIStatusError(400, "unsupported image format for base64", nil, nil, nil)
	if !IsImageFormatError(err) {
		t.Error("expected IsImageFormatError=true for image format rejection")
	}

	// Status 400 with non-image message
	err = NormalizeAPIStatusError(400, "invalid parameter", nil, nil, nil)
	if IsImageFormatError(err) {
		t.Error("expected IsImageFormatError=false for non-image rejection")
	}

	// Context overflow should NOT be image format
	err = NormalizeAPIStatusError(400, "context_length_exceeded for image", nil, nil, nil)
	if IsImageFormatError(err) {
		t.Error("context overflow should not be image format error")
	}

	// Pre-request client-side image rejection
	baseErr := NewChatProviderError("unsupported media type for base64 image: webp")
	if !IsImageFormatError(baseErr) {
		t.Error("expected IsImageFormatError=true for client-side image rejection")
	}
}

func TestIsToolExchangeAdjacencyError(t *testing.T) {
	err := NormalizeAPIStatusError(400, "tool_use blocks must be followed by tool_result blocks", nil, nil, nil)
	if !IsToolExchangeAdjacencyError(err) {
		t.Error("expected tool exchange adjacency error")
	}

	err = NormalizeAPIStatusError(400, "tool_call_id call_abc is not found", nil, nil, nil)
	if !IsToolExchangeAdjacencyError(err) {
		t.Error("expected tool_call_id not found to be adjacency error")
	}

	// Non-400/422 should not match
	err = NormalizeAPIStatusError(500, "tool_use blocks must be followed by tool_result blocks", nil, nil, nil)
	if IsToolExchangeAdjacencyError(err) {
		t.Error("500 should not be tool exchange adjacency error")
	}
}

func TestIsRecoverableRequestStructureError(t *testing.T) {
	// Tool exchange errors are recoverable structural errors
	err := NormalizeAPIStatusError(400, "tool_use blocks must be followed by tool_result blocks", nil, nil, nil)
	if !IsRecoverableRequestStructureError(err) {
		t.Error("tool exchange error should be recoverable")
	}

	// Other structural errors
	err = NormalizeAPIStatusError(400, "roles must alternate between user and assistant", nil, nil, nil)
	if !IsRecoverableRequestStructureError(err) {
		t.Error("role alternation error should be recoverable")
	}

	err = NormalizeAPIStatusError(400, "text content blocks must be non-empty", nil, nil, nil)
	if !IsRecoverableRequestStructureError(err) {
		t.Error("empty text block error should be recoverable")
	}
}

func TestAPIEmptyResponseError(t *testing.T) {
	finish := FinishTruncated
	raw := "length"
	err := NewAPIEmptyResponseError("empty response", &finish, &raw)

	if err.FinishReason == nil || *err.FinishReason != FinishTruncated {
		t.Error("expected FinishReason=truncated")
	}
	if err.RawFinishReason == nil || *err.RawFinishReason != "length" {
		t.Error("expected RawFinishReason=length")
	}

	// Should match ChatProviderError
	var base *ChatProviderError
	if !errors.As(err, &base) {
		t.Error("APIEmptyResponseError should match ChatProviderError")
	}
}

func ptrInt64(v int64) *int64 { return &v }
