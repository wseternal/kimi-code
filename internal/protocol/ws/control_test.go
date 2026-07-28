package ws

import (
	"encoding/json"
	"testing"
)

func TestServerHelloRoundTrip(t *testing.T) {
	msg := ServerHelloMessage{
		Type:      "server_hello",
		Timestamp: "2026-07-25T12:00:00.000Z",
		Payload: ServerHelloPayload{
			WSConnectionID:     "conn-1",
			ProtocolVersion:    ProtocolVersion,
			MaxEventBufferSize: 1024,
			Capabilities:       ServerHelloCapabilities{EventBatching: true, Compression: false},
		},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ServerHelloMessage
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Payload.ProtocolVersion != 2 {
		t.Errorf("expected protocol version 2, got %d", decoded.Payload.ProtocolVersion)
	}
	if decoded.Payload.WSConnectionID != "conn-1" {
		t.Errorf("expected conn-1, got %s", decoded.Payload.WSConnectionID)
	}
	if !decoded.Payload.Capabilities.EventBatching {
		t.Error("expected event_batching true")
	}
}

func TestClientHelloRoundTrip(t *testing.T) {
	msg := ClientHelloMessage{
		Type: "client_hello",
		ID:   "msg-1",
		Payload: ClientHelloPayload{
			ClientID: "client-abc",
			Cursors: CursorsBySession{
				"sess-1": {Seq: 42, Epoch: "e1"},
			},
		},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	json.Unmarshal(b, &raw)
	payload := raw["payload"].(map[string]any)
	if payload["client_id"] != "client-abc" {
		t.Errorf("expected client_id client-abc, got %v", payload["client_id"])
	}
	cursors := payload["cursors"].(map[string]any)
	sess1 := cursors["sess-1"].(map[string]any)
	if sess1["seq"].(float64) != 42 {
		t.Errorf("expected seq 42, got %v", sess1["seq"])
	}
}

func TestAckEnvelope(t *testing.T) {
	ack := AckEnvelope{
		Type: "ack",
		ID:   "msg-1",
		Code: 0,
		Msg:  "success",
	}
	b, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	json.Unmarshal(b, &raw)
	if raw["type"] != "ack" {
		t.Errorf("expected type ack, got %v", raw["type"])
	}
	if raw["code"].(float64) != 0 {
		t.Errorf("expected code 0, got %v", raw["code"])
	}
}

func TestEventEnvelope(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"text": "hello"})
	env := EventEnvelope{
		Type:      "session_event",
		Seq:       10,
		Epoch:     "e1",
		SessionID: "sess-1",
		Timestamp: "2026-07-25T12:00:00.000Z",
		Payload:   payload,
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	var decoded EventEnvelope
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Seq != 10 {
		t.Errorf("expected seq 10, got %d", decoded.Seq)
	}
	if decoded.Epoch != "e1" {
		t.Errorf("expected epoch e1, got %s", decoded.Epoch)
	}
}

func TestSubscribeMessage(t *testing.T) {
	msg := SubscribeMessage{
		Type: "subscribe",
		ID:   "sub-1",
		Payload: SubscribePayload{
			SessionIDs: []string{"s1", "s2"},
			Cursors: CursorsBySession{
				"s1": {Seq: 5},
			},
			AgentFilter: AgentFilter{
				"s1": {"agent-main"},
			},
		},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	json.Unmarshal(b, &raw)
	payload := raw["payload"].(map[string]any)
	ids := payload["session_ids"].([]any)
	if len(ids) != 2 {
		t.Errorf("expected 2 session_ids, got %d", len(ids))
	}
}

func TestTerminalMessages(t *testing.T) {
	// Attach
	attach := TerminalAttachMessage{
		Type: "terminal_attach",
		ID:   "t-1",
		Payload: TerminalAttachPayload{
			SessionID:  "s-1",
			TerminalID: "term-1",
			SinceSeq:   0,
		},
	}
	b, _ := json.Marshal(attach)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	if raw["type"] != "terminal_attach" {
		t.Errorf("expected type terminal_attach, got %v", raw["type"])
	}

	// Output
	output := TerminalOutputMessage{
		Type:       "terminal_output",
		Seq:        5,
		SessionID:  "s-1",
		TerminalID: "term-1",
		Timestamp:  "2026-07-25T12:00:00.000Z",
		Payload:    TerminalOutputPayload{Data: "$ ls\n"},
	}
	b, _ = json.Marshal(output)
	json.Unmarshal(b, &raw)
	if raw["seq"].(float64) != 5 {
		t.Errorf("expected seq 5, got %v", raw["seq"])
	}

	// Exit with null exit_code
	exit := TerminalExitMessage{
		Type:       "terminal_exit",
		SessionID:  "s-1",
		TerminalID: "term-1",
		Timestamp:  "2026-07-25T12:00:00.000Z",
		Payload:    TerminalExitPayload{ExitCode: nil},
	}
	b, _ = json.Marshal(exit)
	json.Unmarshal(b, &raw)
	payload := raw["payload"].(map[string]any)
	if payload["exit_code"] != nil {
		t.Errorf("expected exit_code null, got %v", payload["exit_code"])
	}
}

func TestResyncRequired(t *testing.T) {
	msg := ResyncRequiredMessage{
		Type:      "resync_required",
		Timestamp: "2026-07-25T12:00:00.000Z",
		Payload: ResyncRequiredPayload{
			SessionID:  "s-1",
			Reason:     ResyncEpochChanged,
			CurrentSeq: 100,
			Epoch:      "e2",
		},
	}
	b, _ := json.Marshal(msg)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	payload := raw["payload"].(map[string]any)
	if payload["reason"] != "epoch_changed" {
		t.Errorf("expected reason epoch_changed, got %v", payload["reason"])
	}
}

func TestWsError(t *testing.T) {
	msg := WsErrorMessage{
		Type:      "error",
		Timestamp: "2026-07-25T12:00:00.000Z",
		Payload: WsErrorPayload{
			Code:  50001,
			Msg:   "internal error",
			Fatal: true,
		},
	}
	b, _ := json.Marshal(msg)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	payload := raw["payload"].(map[string]any)
	if payload["fatal"] != true {
		t.Error("expected fatal true")
	}
}
