package protocol

import "encoding/json"

// Envelope is the standard API response wrapper.
// Wire shape: { "code": int, "msg": string, "data": T|null, "request_id": string }
type Envelope[T any] struct {
	Code      int             `json:"code"`
	Msg       string          `json:"msg"`
	Data      *T              `json:"data"`
	RequestID string          `json:"request_id"`
	Details   json.RawMessage `json:"details,omitempty"`
	Stack     string          `json:"stack,omitempty"`
}

// OkEnvelope builds a success envelope (code 0).
func OkEnvelope[T any](data T, requestID string) Envelope[T] {
	return Envelope[T]{
		Code:      0,
		Msg:       "success",
		Data:      &data,
		RequestID: requestID,
	}
}

// ErrEnvelope builds an error envelope. stack is omitted from JSON when empty.
func ErrEnvelope(code int, msg, requestID string) Envelope[struct{}] {
	return Envelope[struct{}]{
		Code:      code,
		Msg:       msg,
		Data:      nil,
		RequestID: requestID,
	}
}

// ErrEnvelopeWithStack builds an error envelope with a stack trace string.
func ErrEnvelopeWithStack(code int, msg, requestID, stack string) Envelope[struct{}] {
	return Envelope[struct{}]{
		Code:      code,
		Msg:       msg,
		Data:      nil,
		RequestID: requestID,
		Stack:     stack,
	}
}
