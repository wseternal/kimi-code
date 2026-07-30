package kosong

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ChatProviderError is the base error for all chat provider errors.
type ChatProviderError struct {
	Msg string
}

func (e *ChatProviderError) Error() string { return e.Msg }

// NewChatProviderError creates a base provider error.
func NewChatProviderError(msg string) *ChatProviderError {
	return &ChatProviderError{Msg: msg}
}

// APIConnectionError is a network-level connection failure.
type APIConnectionError struct {
	ChatProviderError
}

func (e *APIConnectionError) Unwrap() error { return &e.ChatProviderError }

// NewAPIConnectionError creates a connection error.
func NewAPIConnectionError(msg string) *APIConnectionError {
	return &APIConnectionError{ChatProviderError{Msg: msg}}
}

// APITimeoutError means the request timed out.
type APITimeoutError struct {
	ChatProviderError
}

func (e *APITimeoutError) Unwrap() error { return &e.ChatProviderError }

// NewAPITimeoutError creates a timeout error.
func NewAPITimeoutError(msg string) *APITimeoutError {
	return &APITimeoutError{ChatProviderError{Msg: msg}}
}

// APIStatusError is an HTTP status error from the API.
type APIStatusError struct {
	ChatProviderError
	StatusCode    int
	RequestID     *string
	RetryAfterMs  *int64 // server-requested backoff in milliseconds
	TraceID       *string
}

func (e *APIStatusError) Unwrap() error { return &e.ChatProviderError }

// NewAPIStatusError creates a status error with full metadata.
func NewAPIStatusError(statusCode int, msg string, requestID *string, retryAfterMs *int64, traceID *string) *APIStatusError {
	return &APIStatusError{
		ChatProviderError: ChatProviderError{Msg: msg},
		StatusCode:        statusCode,
		RequestID:         requestID,
		RetryAfterMs:      retryAfterMs,
		TraceID:           traceID,
	}
}

// APIContextOverflowError means the request exceeded the model context window.
type APIContextOverflowError struct {
	APIStatusError
}

func (e *APIContextOverflowError) Unwrap() error { return &e.APIStatusError }

// NewAPIContextOverflowError creates a context overflow error.
func NewAPIContextOverflowError(statusCode int, msg string, requestID *string, retryAfterMs *int64, traceID *string) *APIContextOverflowError {
	return &APIContextOverflowError{
		APIStatusError: APIStatusError{
			ChatProviderError: ChatProviderError{Msg: msg},
			StatusCode:        statusCode,
			RequestID:         requestID,
			RetryAfterMs:      retryAfterMs,
			TraceID:           traceID,
		},
	}
}

// APIRequestTooLargeError means the serialized request body exceeded the
// provider's byte ceiling (e.g. accumulated base64 images). Distinct from
// token-count overflow which is handled by compaction.
type APIRequestTooLargeError struct {
	APIStatusError
}

func (e *APIRequestTooLargeError) Unwrap() error { return &e.APIStatusError }

// NewAPIRequestTooLargeError creates a request-too-large error.
func NewAPIRequestTooLargeError(statusCode int, msg string, requestID *string, retryAfterMs *int64, traceID *string) *APIRequestTooLargeError {
	return &APIRequestTooLargeError{
		APIStatusError: APIStatusError{
			ChatProviderError: ChatProviderError{Msg: msg},
			StatusCode:        statusCode,
			RequestID:         requestID,
			RetryAfterMs:      retryAfterMs,
			TraceID:           traceID,
		},
	}
}

// APIProviderRateLimitError means the provider rate-limited the request.
type APIProviderRateLimitError struct {
	APIStatusError
}

func (e *APIProviderRateLimitError) Unwrap() error { return &e.APIStatusError }

// NewAPIProviderRateLimitError creates a rate limit error (status 429).
func NewAPIProviderRateLimitError(msg string, requestID *string, retryAfterMs *int64, traceID *string) *APIProviderRateLimitError {
	return &APIProviderRateLimitError{
		APIStatusError: APIStatusError{
			ChatProviderError: ChatProviderError{Msg: msg},
			StatusCode:        429,
			RequestID:         requestID,
			RetryAfterMs:      retryAfterMs,
			TraceID:           traceID,
		},
	}
}

// APIProviderQuotaExhaustedError means the account's quota/balance is
// exhausted. Deliberately NOT a subclass of APIProviderRateLimitError:
// quota exhaustion is deterministic until the account is recharged,
// so it is excluded from retry.
type APIProviderQuotaExhaustedError struct {
	APIStatusError
}

func (e *APIProviderQuotaExhaustedError) Unwrap() error { return &e.APIStatusError }

// NewAPIProviderQuotaExhaustedError creates a quota exhaustion error (status 429).
func NewAPIProviderQuotaExhaustedError(msg string, requestID *string, retryAfterMs *int64, traceID *string) *APIProviderQuotaExhaustedError {
	return &APIProviderQuotaExhaustedError{
		APIStatusError: APIStatusError{
			ChatProviderError: ChatProviderError{Msg: msg},
			StatusCode:        429,
			RequestID:         requestID,
			RetryAfterMs:      retryAfterMs,
			TraceID:           traceID,
		},
	}
}

// APIEmptyResponseError means the API returned an empty response (no content, no tool calls).
type APIEmptyResponseError struct {
	ChatProviderError
	FinishReason    *FinishReason
	RawFinishReason *string
}

func (e *APIEmptyResponseError) Unwrap() error { return &e.ChatProviderError }

// NewAPIEmptyResponseError creates an empty response error.
func NewAPIEmptyResponseError(msg string, finishReason *FinishReason, rawFinishReason *string) *APIEmptyResponseError {
	return &APIEmptyResponseError{
		ChatProviderError: ChatProviderError{Msg: msg},
		FinishReason:      finishReason,
		RawFinishReason:   rawFinishReason,
	}
}

// --- Classification functions ---

// retryableStatuses are transient HTTP statuses worth retrying.
var retryableStatuses = map[int]bool{
	408: true, // request timeout
	409: true, // lock/conflict timeout
	429: true, // rate limit
	500: true, // internal server error
	502: true, // bad gateway
	503: true, // service unavailable
	504: true, // gateway timeout
	529: true, // provider overloaded
}

// IsRetryableGenerateError classifies whether an error is worth retrying.
func IsRetryableGenerateError(err error) bool {
	if err == nil {
		return false
	}

	// Connection and timeout errors are always retryable.
	var connErr *APIConnectionError
	var timeoutErr *APITimeoutError
	if errors.As(err, &connErr) || errors.As(err, &timeoutErr) {
		return true
	}

	// Empty response errors are retryable.
	var emptyErr *APIEmptyResponseError
	if errors.As(err, &emptyErr) {
		return true
	}

	// Quota exhaustion is NOT retryable (deterministic until recharge).
	var quotaErr *APIProviderQuotaExhaustedError
	if errors.As(err, &quotaErr) {
		return false
	}

	// Image format errors are NOT retryable (deterministic per history).
	if IsImageFormatError(err) {
		return false
	}

	// Status errors: check specific status codes.
	var statusErr *APIStatusError
	if errors.As(err, &statusErr) {
		return retryableStatuses[statusErr.StatusCode]
	}

	// Base ChatProviderError (unclassified): retry unless it's an image format error.
	var baseErr *ChatProviderError
	if errors.As(err, &baseErr) {
		return true
	}

	return false
}

// --- Pattern matching ---

var (
	networkRE = regexp.MustCompile(`(?i)network|connection|connect|disconnect|terminated`)
	timeoutRE = regexp.MustCompile(`(?i)timed?\s*out|timeout|deadline`)
)

// ClassifyBaseAPIError classifies a raw error message into the right error type.
func ClassifyBaseAPIError(msg string) error {
	if timeoutRE.MatchString(msg) {
		return NewAPITimeoutError(msg)
	}
	if networkRE.MatchString(msg) {
		return NewAPIConnectionError(msg)
	}
	return NewChatProviderError(fmt.Sprintf("Error: %s", msg))
}

// --- Context overflow patterns ---

var contextOverflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)context[ _\-]?length`),
	regexp.MustCompile(`(?i)(?:context[ _\-]?window.*exceed|exceed.*context[ _\-]?window)`),
	regexp.MustCompile(`(?i)maximum context`),
	regexp.MustCompile(`(?i)exceed(?:ed|s|ing)?\s+(?:the\s+)?max(?:imum)?\s+tokens?`),
	regexp.MustCompile(`(?i)(?:too many tokens.*(?:prompt|input|context)|(?:prompt|input|context).*too many tokens)`),
	regexp.MustCompile(`(?i)prompt is too long.*maximum`),
	regexp.MustCompile(`(?i)input token count.*exceeds?.*maximum number of tokens`),
	regexp.MustCompile(`(?i)request.*exceed(?:ed|s|ing)?.*model token limit`),
}

// IsContextOverflowStatusError checks whether the status+message indicates context overflow.
func IsContextOverflowStatusError(statusCode int, message string) bool {
	if statusCode != 400 && statusCode != 413 && statusCode != 422 {
		return false
	}
	lower := strings.ToLower(message)
	for _, p := range contextOverflowPatterns {
		if p.MatchString(lower) {
			return true
		}
	}
	return false
}

// --- Request too large patterns ---

var requestTooLargePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)request exceeds the maximum size`),
	regexp.MustCompile(`(?i)request entity too large`),
	regexp.MustCompile(`(?i)request_too_large`),
	regexp.MustCompile(`(?i)exceeds? the maximum allowed number of bytes`),
	regexp.MustCompile(`(?i)payload too large`),
	regexp.MustCompile(`(?i)content too large`),
	regexp.MustCompile(`(?i)request (?:body )?too large`),
}

// IsRequestTooLargeStatusError checks whether the status+message indicates request body too large.
func IsRequestTooLargeStatusError(statusCode int, message string) bool {
	if statusCode != 413 {
		return false
	}
	lower := strings.ToLower(message)
	for _, p := range requestTooLargePatterns {
		if p.MatchString(lower) {
			return true
		}
	}
	return false
}

// --- Quota exhaustion patterns ---

var quotaExhaustedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)exceeded_current_quota`),
	regexp.MustCompile(`(?i)insufficient_quota`),
	regexp.MustCompile(`(?i)insufficient balance`),
	regexp.MustCompile(`(?i)quota.*exhaust`),
	regexp.MustCompile(`(?i)account.*suspended`),
}

// IsQuotaExhaustedMessage checks whether the message indicates quota exhaustion (not transient rate limit).
func IsQuotaExhaustedMessage(message string) bool {
	lower := strings.ToLower(message)
	for _, p := range quotaExhaustedPatterns {
		if p.MatchString(lower) {
			return true
		}
	}
	return false
}

// --- Image format error patterns ---

var imageFormatProviderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unsupported media type for base64 image`),
	regexp.MustCompile(`(?i)invalid data url for image`),
}

var imageFormatStatusPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unsupported image (?:url|format|type)`),
	regexp.MustCompile(`(?i)does not represent a valid image`),
	regexp.MustCompile(`(?i)could not (?:process|decode) (?:the |input )?image`),
	regexp.MustCompile(`(?i)unable to process (?:the |input )?image`),
	regexp.MustCompile(`(?i)failed to decode (?:the )?image`),
	regexp.MustCompile(`(?i)invalid image(?: data| type| format)?`),
}

var mediaTypeFieldRE = regexp.MustCompile(`(?i)(?:media|mime)_?type`)

// IsImageFormatError checks whether the error is about an image format/data rejection.
func IsImageFormatError(err error) bool {
	// Check for APIStatusError with status 400
	var statusErr *APIStatusError
	if errors.As(err, &statusErr) {
		// Exclude overflow and request-too-large
		var overflowErr *APIContextOverflowError
		var tooLargeErr *APIRequestTooLargeError
		if errors.As(err, &overflowErr) || errors.As(err, &tooLargeErr) {
			return false
		}
		if statusErr.StatusCode != 400 {
			return false
		}
		lower := strings.ToLower(statusErr.Msg)
		for _, p := range imageFormatStatusPatterns {
			if p.MatchString(lower) {
				return true
			}
		}
		if mediaTypeFieldRE.MatchString(lower) && strings.Contains(lower, "image") {
			return true
		}
		return false
	}

	// Check for base ChatProviderError (pre-request client-side rejections)
	var baseErr *ChatProviderError
	if errors.As(err, &baseErr) {
		lower := strings.ToLower(baseErr.Msg)
		for _, p := range imageFormatProviderPatterns {
			if p.MatchString(lower) {
				return true
			}
		}
	}
	return false
}

// --- Tool exchange adjacency error patterns ---

var toolExchangePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)tool_use[\s\S]*tool_result`),
	regexp.MustCompile(`(?i)tool_result[\s\S]*tool_use`),
	regexp.MustCompile(`(?i)unexpected\s+` + "`?" + `tool_result`),
	regexp.MustCompile(`(?i)tool_call_id[\s\S]*not found`),
	regexp.MustCompile(`(?i)role\s+['"` + "`" + `]?tool['"` + "`" + `]?\s+must be a response to a preceding message`),
	regexp.MustCompile(`(?i)assistant message with\s+['"` + "`" + `]?tool_calls['"` + "`" + `]?\s+must be followed by tool messages`),
	regexp.MustCompile(`(?i)tool_call_ids? did not have response messages`),
	regexp.MustCompile(`(?i)insufficient tool messages following`),
}

// IsToolExchangeAdjacencyError checks whether the error is about tool call/result pairing.
func IsToolExchangeAdjacencyError(err error) bool {
	var statusErr *APIStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	var overflowErr *APIContextOverflowError
	if errors.As(err, &overflowErr) {
		return false
	}
	if statusErr.StatusCode != 400 && statusErr.StatusCode != 422 {
		return false
	}
	lower := strings.ToLower(statusErr.Msg)
	for _, p := range toolExchangePatterns {
		if p.MatchString(lower) {
			return true
		}
	}
	return false
}

// --- Structural request error patterns ---

var structuralRequestPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)text content blocks must be non-empty`),
	regexp.MustCompile(`(?i)text content blocks must contain non-whitespace`),
	regexp.MustCompile(`(?i)first message must use the .*user.* role`),
	regexp.MustCompile(`(?i)roles must alternate`),
	regexp.MustCompile(`(?i)multiple .*(?:user|assistant).* roles in a row`),
	regexp.MustCompile(`(?i)tool_use[\s\S]*ids must be unique`),
	regexp.MustCompile(`(?i)message at position \d+ with role ['"` + "`" + `]?[a-z]+['"` + "`" + `]? must not be empty`),
}

// IsRecoverableRequestStructureError checks whether the error is a structural request rejection
// that can be recovered by re-projecting the message array.
func IsRecoverableRequestStructureError(err error) bool {
	if IsToolExchangeAdjacencyError(err) {
		return true
	}
	var statusErr *APIStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	var overflowErr *APIContextOverflowError
	if errors.As(err, &overflowErr) {
		return false
	}
	if statusErr.StatusCode != 400 && statusErr.StatusCode != 422 {
		return false
	}
	lower := strings.ToLower(statusErr.Msg)
	for _, p := range structuralRequestPatterns {
		if p.MatchString(lower) {
			return true
		}
	}
	return false
}

// --- Normalize API status error ---

// NormalizeAPIStatusError upgrades a raw HTTP status + message into the
// most specific APIStatusError subclass. This is the single entry point
// for providers to construct status errors.
func NormalizeAPIStatusError(statusCode int, message string, requestID *string, retryAfterMs *int64, traceID *string) error {
	// Quota exhaustion (429 but deterministic) must be checked before rate limit.
	if statusCode == 429 && IsQuotaExhaustedMessage(message) {
		return NewAPIProviderQuotaExhaustedError(message, requestID, retryAfterMs, traceID)
	}

	if statusCode == 429 {
		return NewAPIProviderRateLimitError(message, requestID, retryAfterMs, traceID)
	}

	// Context overflow first: some providers return prompt-too-long as 413.
	if IsContextOverflowStatusError(statusCode, message) {
		return NewAPIContextOverflowError(statusCode, message, requestID, retryAfterMs, traceID)
	}

	if IsRequestTooLargeStatusError(statusCode, message) {
		return NewAPIRequestTooLargeError(statusCode, message, requestID, retryAfterMs, traceID)
	}

	return NewAPIStatusError(statusCode, message, requestID, retryAfterMs, traceID)
}

// ParseRetryAfterMs parses a "retry-after" response header value (integer seconds)
// into milliseconds. Returns nil for non-integer or missing values.
func ParseRetryAfterMs(raw string) *int64 {
	if raw == "" {
		return nil
	}
	var seconds int64
	for _, c := range raw {
		if c < '0' || c > '9' {
			return nil // non-integer (HTTP-date or invalid)
		}
		seconds = seconds*10 + int64(c-'0')
	}
	if seconds < 0 {
		return nil
	}
	ms := seconds * 1000
	return &ms
}
