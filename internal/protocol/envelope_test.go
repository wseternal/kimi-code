package protocol

import (
	"encoding/json"
	"testing"
)

func TestOkEnvelope(t *testing.T) {
	env := OkEnvelope("hello", "req-123")

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	// Verify wire shape
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["code"].(float64) != 0 {
		t.Errorf("expected code 0, got %v", raw["code"])
	}
	if raw["msg"] != "success" {
		t.Errorf("expected msg 'success', got %v", raw["msg"])
	}
	if raw["data"] != "hello" {
		t.Errorf("expected data 'hello', got %v", raw["data"])
	}
	if raw["request_id"] != "req-123" {
		t.Errorf("expected request_id 'req-123', got %v", raw["request_id"])
	}
	// stack and details must be absent when zero
	if _, ok := raw["stack"]; ok {
		t.Error("stack should be omitted when empty")
	}
	if _, ok := raw["details"]; ok {
		t.Error("details should be omitted when nil")
	}
}

func TestOkEnvelopeStruct(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	env := OkEnvelope(payload{Name: "alice", Age: 30}, "req-456")

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}

	data := raw["data"].(map[string]any)
	if data["name"] != "alice" {
		t.Errorf("expected name 'alice', got %v", data["name"])
	}
	if data["age"].(float64) != 30 {
		t.Errorf("expected age 30, got %v", data["age"])
	}
}

func TestErrEnvelope(t *testing.T) {
	env := ErrEnvelope(ErrorCodeSessionNotFound, "session not found", "req-789")

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["code"].(float64) != float64(ErrorCodeSessionNotFound) {
		t.Errorf("expected code %d, got %v", ErrorCodeSessionNotFound, raw["code"])
	}
	if raw["data"] != nil {
		t.Errorf("expected data null, got %v", raw["data"])
	}
	// stack must be absent
	if _, ok := raw["stack"]; ok {
		t.Error("stack should be omitted when empty")
	}
}

func TestErrEnvelopeWithStack(t *testing.T) {
	env := ErrEnvelopeWithStack(ErrorCodeInternalError, "boom", "req-000", "at line 42")

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["stack"] != "at line 42" {
		t.Errorf("expected stack 'at line 42', got %v", raw["stack"])
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}
	original := OkEnvelope([]item{{ID: "a"}, {ID: "b"}}, "rt-1")

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Envelope[[]item]
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Code != 0 {
		t.Errorf("expected code 0, got %d", decoded.Code)
	}
	if decoded.Data == nil || len(*decoded.Data) != 2 {
		t.Fatal("expected 2 items")
	}
	if (*decoded.Data)[0].ID != "a" {
		t.Errorf("expected first item ID 'a', got %s", (*decoded.Data)[0].ID)
	}
}

func TestErrorCodeReason(t *testing.T) {
	if r := ErrorCodeReason(ErrorCodeSuccess); r != "success" {
		t.Errorf("expected 'success', got %q", r)
	}
	if r := ErrorCodeReason(ErrorCodeSessionNotFound); r != "session.not_found" {
		t.Errorf("expected 'session.not_found', got %q", r)
	}
	if r := ErrorCodeReason(99999); r != "unknown" {
		t.Errorf("expected 'unknown', got %q", r)
	}
}

func TestErrorCodeClassification(t *testing.T) {
	if !IsClientError(ErrorCodeSessionNotFound) {
		t.Error("expected SessionNotFound to be a client error")
	}
	if !IsServerError(ErrorCodeInternalError) {
		t.Error("expected InternalError to be a server error")
	}
	if !IsToolError(ErrorCodeToolExecutionFailed) {
		t.Error("expected ToolExecutionFailed to be a tool error")
	}
	if IsClientError(ErrorCodeInternalError) {
		t.Error("InternalError should not be a client error")
	}
}

func TestAPIError(t *testing.T) {
	err := NewAPIError(ErrorCodeSessionNotFound, "abc-123 does not exist")
	if err.Code != ErrorCodeSessionNotFound {
		t.Errorf("expected code %d, got %d", ErrorCodeSessionNotFound, err.Code)
	}
	expected := "[40401 session.not_found] abc-123 does not exist"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestCursorQueryValidate(t *testing.T) {
	q := CursorQuery{BeforeID: "a", AfterID: "b"}
	if err := q.Validate(); err == nil {
		t.Error("expected error for mutually exclusive cursors")
	}

	q = CursorQuery{PageSize: 200}
	if err := q.Validate(); err == nil {
		t.Error("expected error for page_size > 100")
	}

	q = CursorQuery{AfterID: "x", PageSize: 50}
	if err := q.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPageResponse(t *testing.T) {
	resp := PageResponse[string]{Items: []string{"a", "b"}, HasMore: true}
	b, _ := json.Marshal(resp)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	items := raw["items"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if raw["has_more"] != true {
		t.Error("expected has_more true")
	}
}
