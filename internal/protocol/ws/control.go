package ws

import (
	"encoding/json"
)

// WS protocol version. v2: cursor-based multi-device sync with {seq, epoch}.
const ProtocolVersion = 2

// SessionCursor is a per-session sync cursor. seq is the last durable event
// the client has applied (journal offset). epoch identifies the journal
// incarnation; a mismatch triggers resync_required(epoch_changed).
type SessionCursor struct {
	Seq   int    `json:"seq"`
	Epoch string `json:"epoch,omitempty"`
}

// CursorsBySession maps session_id to the client's last known cursor.
type CursorsBySession map[string]SessionCursor

// AgentFilter is a per-session agent allowlist. Keys are session ids, values
// are the non-empty set of agent ids the client wants events for. Sessions
// absent from the map receive every agent (legacy behavior).
type AgentFilter map[string][]string

// --- Event Envelope (server → client) ---

// EventEnvelope wraps a session event with sync metadata.
type EventEnvelope struct {
	Type      string          `json:"type"`
	Seq       int             `json:"seq"`
	Epoch     string          `json:"epoch,omitempty"`
	Volatile  bool            `json:"volatile,omitempty"`
	Offset    int             `json:"offset,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// --- Control Envelope (client ↔ server) ---

// ControlEnvelope is the generic client control message wrapper.
type ControlEnvelope struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

// AckEnvelope is the server acknowledgement for a control message.
type AckEnvelope struct {
	Type    string          `json:"type"` // always "ack"
	ID      string          `json:"id"`
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Payload json.RawMessage `json:"payload"`
}

// --- Server Hello ---

// ServerHelloCapabilities advertises server capabilities.
type ServerHelloCapabilities struct {
	EventBatching bool `json:"event_batching"`
	Compression   bool `json:"compression"`
}

// ServerHelloPayload is the payload of the server_hello message.
type ServerHelloPayload struct {
	WSConnectionID     string                 `json:"ws_connection_id"`
	ProtocolVersion    int                    `json:"protocol_version"`
	HeartbeatMs        int                    `json:"heartbeat_ms,omitempty"`
	MaxEventBufferSize int                    `json:"max_event_buffer_size"`
	Capabilities       ServerHelloCapabilities `json:"capabilities"`
}

// ServerHelloMessage is sent by the server immediately after the socket opens.
type ServerHelloMessage struct {
	Type      string             `json:"type"` // "server_hello"
	Timestamp string             `json:"timestamp"`
	Payload   ServerHelloPayload `json:"payload"`
}

// --- Client Hello ---

// ClientHelloPayload is the handshake payload. Only client_id is required;
// the subscription fields are deprecated in favor of subscribe frames.
type ClientHelloPayload struct {
	ClientID      string             `json:"client_id"`
	Subscriptions []string           `json:"subscriptions,omitempty"` // deprecated
	Cursors       CursorsBySession   `json:"cursors,omitempty"`       // deprecated
	AgentFilter   AgentFilter        `json:"agent_filter,omitempty"`  // deprecated
}

// ClientHelloMessage wraps the client_hello control message.
type ClientHelloMessage struct {
	Type    string             `json:"type"` // "client_hello"
	ID      string             `json:"id"`
	Payload ClientHelloPayload `json:"payload"`
}

// ClientHelloAckPayload is the server's response to client_hello.
type ClientHelloAckPayload struct {
	AcceptedSubscriptions []string           `json:"accepted_subscriptions"`
	ResyncRequired        []string           `json:"resync_required"`
	Cursors               CursorsBySession   `json:"cursors,omitempty"`
}

// --- Subscribe / Unsubscribe ---

// WatchFsConfig configures filesystem watching.
type WatchFsConfig struct {
	Paths     []string `json:"paths"`
	Recursive bool     `json:"recursive,omitempty"`
}

// SubscribePayload requests subscription to session event streams.
type SubscribePayload struct {
	SessionIDs  []string                   `json:"session_ids"`
	Cursors     CursorsBySession           `json:"cursors,omitempty"`
	WatchFs     map[string]WatchFsConfig   `json:"watch_fs,omitempty"`
	AgentFilter AgentFilter                `json:"agent_filter,omitempty"`
}

// SubscribeMessage wraps a subscribe control message.
type SubscribeMessage struct {
	Type    string           `json:"type"` // "subscribe"
	ID      string           `json:"id"`
	Payload SubscribePayload `json:"payload"`
}

// SubscribeAckPayload is the server response to subscribe.
type SubscribeAckPayload struct {
	Accepted       []string           `json:"accepted"`
	NotFound       []string           `json:"not_found"`
	ResyncRequired []string           `json:"resync_required"`
	Cursors        CursorsBySession   `json:"cursors,omitempty"`
}

// UnsubscribePayload requests removal of session subscriptions.
type UnsubscribePayload struct {
	SessionIDs []string `json:"session_ids"`
}

// UnsubscribeMessage wraps an unsubscribe control message.
type UnsubscribeMessage struct {
	Type    string             `json:"type"` // "unsubscribe"
	ID      string             `json:"id"`
	Payload UnsubscribePayload `json:"payload"`
}

// --- Watch FS Add / Remove ---

// WatchFsAddPayload adds filesystem watch paths for a session.
type WatchFsAddPayload struct {
	SessionID string   `json:"session_id"`
	Paths     []string `json:"paths"`
	Recursive bool     `json:"recursive,omitempty"`
}

// WatchFsAddMessage wraps a watch_fs_add control message.
type WatchFsAddMessage struct {
	Type    string            `json:"type"` // "watch_fs_add"
	ID      string            `json:"id"`
	Payload WatchFsAddPayload `json:"payload"`
}

// WatchFsRemovePayload removes filesystem watch paths for a session.
type WatchFsRemovePayload struct {
	SessionID string   `json:"session_id"`
	Paths     []string `json:"paths"`
}

// WatchFsRemoveMessage wraps a watch_fs_remove control message.
type WatchFsRemoveMessage struct {
	Type    string               `json:"type"` // "watch_fs_remove"
	ID      string               `json:"id"`
	Payload WatchFsRemovePayload `json:"payload"`
}

// WatchFsAckPayload is the server response to watch_fs operations.
type WatchFsAckPayload struct {
	WatchedPaths []string `json:"watched_paths,omitempty"`
	CurrentCount int      `json:"current_count,omitempty"`
}

// --- Abort ---

// AbortPayload requests abort of a running prompt.
type AbortPayload struct {
	SessionID string `json:"session_id"`
	PromptID  string `json:"prompt_id"`
}

// AbortMessage wraps an abort control message.
type AbortMessage struct {
	Type    string       `json:"type"` // "abort"
	ID      string       `json:"id"`
	Payload AbortPayload `json:"payload"`
}

// AbortAckPayload is the server response to abort.
type AbortAckPayload struct {
	Aborted bool `json:"aborted,omitempty"`
	AtSeq   int  `json:"at_seq,omitempty"`
}

// --- Terminal Operations ---

// TerminalAttachPayload attaches to a terminal stream.
type TerminalAttachPayload struct {
	SessionID  string `json:"session_id"`
	TerminalID string `json:"terminal_id"`
	SinceSeq   int    `json:"since_seq,omitempty"`
}

// TerminalAttachMessage wraps a terminal_attach control message.
type TerminalAttachMessage struct {
	Type    string                `json:"type"` // "terminal_attach"
	ID      string                `json:"id"`
	Payload TerminalAttachPayload `json:"payload"`
}

// TerminalAttachAckPayload is the server response to terminal_attach.
type TerminalAttachAckPayload struct {
	Attached bool `json:"attached"`
	Replayed int  `json:"replayed"`
}

// TerminalDetachPayload detaches from a terminal stream.
type TerminalDetachPayload struct {
	SessionID  string `json:"session_id"`
	TerminalID string `json:"terminal_id"`
}

// TerminalDetachMessage wraps a terminal_detach control message.
type TerminalDetachMessage struct {
	Type    string                `json:"type"` // "terminal_detach"
	ID      string                `json:"id"`
	Payload TerminalDetachPayload `json:"payload"`
}

// TerminalDetachAckPayload is the server response to terminal_detach.
type TerminalDetachAckPayload struct {
	Detached bool `json:"detached"`
}

// TerminalInputPayload sends raw bytes to a terminal.
type TerminalInputPayload struct {
	SessionID  string `json:"session_id"`
	TerminalID string `json:"terminal_id"`
	Data       string `json:"data"`
}

// TerminalInputMessage wraps a terminal_input control message.
type TerminalInputMessage struct {
	Type    string               `json:"type"` // "terminal_input"
	ID      string               `json:"id"`
	Payload TerminalInputPayload `json:"payload"`
}

// TerminalInputAckPayload is the server response to terminal_input.
type TerminalInputAckPayload struct {
	Accepted bool `json:"accepted"`
}

// TerminalResizePayload resizes a terminal.
type TerminalResizePayload struct {
	SessionID  string `json:"session_id"`
	TerminalID string `json:"terminal_id"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

// TerminalResizeMessage wraps a terminal_resize control message.
type TerminalResizeMessage struct {
	Type    string                `json:"type"` // "terminal_resize"
	ID      string                `json:"id"`
	Payload TerminalResizePayload `json:"payload"`
}

// TerminalResizeAckPayload is the server response to terminal_resize.
type TerminalResizeAckPayload struct {
	Resized bool `json:"resized"`
}

// TerminalClosePayload closes a terminal.
type TerminalClosePayload struct {
	SessionID  string `json:"session_id"`
	TerminalID string `json:"terminal_id"`
}

// TerminalCloseMessage wraps a terminal_close control message.
type TerminalCloseMessage struct {
	Type    string               `json:"type"` // "terminal_close"
	ID      string               `json:"id"`
	Payload TerminalClosePayload `json:"payload"`
}

// TerminalCloseAckPayload is the server response to terminal_close.
type TerminalCloseAckPayload struct {
	Closed bool `json:"closed"`
}

// --- Ping / Pong ---

// PingPayload carries a nonce for heartbeat ping.
type PingPayload struct {
	Nonce string `json:"nonce"`
}

// PingMessage is sent by the server as a heartbeat.
type PingMessage struct {
	Type      string      `json:"type"` // "ping"
	Timestamp string      `json:"timestamp"`
	Payload   PingPayload `json:"payload"`
}

// PongPayload echoes the nonce from a ping.
type PongPayload struct {
	Nonce string `json:"nonce"`
}

// PongMessage is sent by the client in response to a ping.
type PongMessage struct {
	Type    string      `json:"type"` // "pong"
	Payload PongPayload `json:"payload"`
}

// --- Resync Required ---

// ResyncReason enumerates reasons for a resync_required message.
type ResyncReason string

const (
	ResyncBufferOverflow    ResyncReason = "buffer_overflow"
	ResyncSessionRecreated  ResyncReason = "session_recreated"
	ResyncEpochChanged      ResyncReason = "epoch_changed"
)

// ResyncRequiredPayload signals that a client must rebuild local state.
type ResyncRequiredPayload struct {
	SessionID  string       `json:"session_id"`
	Reason     ResyncReason `json:"reason"`
	CurrentSeq int          `json:"current_seq"`
	Epoch      string       `json:"epoch,omitempty"`
}

// ResyncRequiredMessage wraps a resync_required system message.
type ResyncRequiredMessage struct {
	Type      string                `json:"type"` // "resync_required"
	Timestamp string                `json:"timestamp"`
	Payload   ResyncRequiredPayload `json:"payload"`
}

// --- WS Error ---

// WsErrorPayload carries a protocol or runtime error.
type WsErrorPayload struct {
	Code      int             `json:"code"`
	Msg       string          `json:"msg"`
	Fatal     bool            `json:"fatal"`
	RequestID string          `json:"request_id,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
}

// WsErrorMessage wraps an error system message.
type WsErrorMessage struct {
	Type      string           `json:"type"` // "error"
	Timestamp string           `json:"timestamp"`
	Payload   WsErrorPayload   `json:"payload"`
}

// --- Terminal Output / Exit (server → client) ---

// TerminalOutputPayload carries terminal output data.
type TerminalOutputPayload struct {
	Data string `json:"data"`
}

// TerminalOutputMessage wraps a terminal_output event.
type TerminalOutputMessage struct {
	Type      string                `json:"type"` // "terminal_output"
	Seq       int                   `json:"seq"`
	SessionID string                `json:"session_id"`
	TerminalID string               `json:"terminal_id"`
	Timestamp string                `json:"timestamp"`
	Payload   TerminalOutputPayload `json:"payload"`
}

// TerminalExitPayload signals a terminal process exit.
type TerminalExitPayload struct {
	ExitCode *int `json:"exit_code,omitempty"`
}

// TerminalExitMessage wraps a terminal_exit event.
type TerminalExitMessage struct {
	Type       string              `json:"type"` // "terminal_exit"
	SessionID  string              `json:"session_id"`
	TerminalID string              `json:"terminal_id"`
	Timestamp  string              `json:"timestamp"`
	Payload    TerminalExitPayload `json:"payload"`
}

// --- Direction & Kind ---

// Direction indicates whether a message flows client→server or server→client.
type Direction string

const (
	ClientToServer Direction = "client_to_server"
	ServerToClient Direction = "server_to_client"
)

// Kind classifies WS operations.
type Kind string

const (
	KindControl Kind = "control"
	KindSystem  Kind = "system"
	KindEvent   Kind = "event"
)
