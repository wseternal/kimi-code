package transport

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-5AB5AA520B85"

// WS opcodes (RFC 6455 §5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// wsConn wraps a hijacked TCP connection with WebSocket frame I/O.
type wsConn struct {
	conn   net.Conn
	rw     *bufio.ReadWriter
	closed bool
}

// upgradeWebSocket performs the server-side WebSocket handshake.
func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return nil, errors.New("not a websocket request")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("missing key")
	}

	// Compute accept hash per RFC 6455 §4.2.2.
	h := sha1.New()
	h.Write([]byte(key + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Hijack the connection to get raw TCP access.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
		return nil, errors.New("hijack not supported")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack failed: %w", err)
	}

	// Write the handshake response.
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := bufrw.WriteString(resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake write: %w", err)
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake flush: %w", err)
	}

	return &wsConn{conn: conn, rw: bufrw}, nil
}

// writeFrame writes a WebSocket frame (server frames are NOT masked per RFC 6455 §5.1).
func (wc *wsConn) writeFrame(opcode byte, payload []byte) error {
	if wc.closed {
		return errors.New("connection closed")
	}
	var header []byte
	header = append(header, 0x80|opcode) // FIN=1 + opcode

	length := len(payload)
	switch {
	case length <= 125:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126)
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(length))
		header = append(header, buf...)
	default:
		header = append(header, 127)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(length))
		header = append(header, buf...)
	}

	if _, err := wc.rw.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := wc.rw.Write(payload); err != nil {
			return err
		}
	}
	return wc.rw.Flush()
}

// writeText writes a text frame.
func (wc *wsConn) writeText(msg string) error {
	return wc.writeFrame(opText, []byte(msg))
}

// writePing writes a ping frame with optional payload.
func (wc *wsConn) writePing(payload []byte) error {
	return wc.writeFrame(opPing, payload)
}

// writeClose writes a close frame.
func (wc *wsConn) writeClose(code int, reason string) error {
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload, uint16(code))
	copy(payload[2:], reason)
	return wc.writeFrame(opClose, payload)
}

// readFrame reads a single WebSocket frame. Returns opcode and payload.
func (wc *wsConn) readFrame() (byte, []byte, error) {
	// First byte: FIN + opcode.
	b0, err := wc.rw.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	opcode := b0 & 0x0F

	// Second byte: MASK + payload length.
	b1, err := wc.rw.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	masked := (b1 & 0x80) != 0
	length := uint64(b1 & 0x7F)

	switch length {
	case 126:
		buf := make([]byte, 2)
		if _, err := io.ReadFull(wc.rw, buf); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(buf))
	case 127:
		buf := make([]byte, 8)
		if _, err := io.ReadFull(wc.rw, buf); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(buf)
	}

	// Read mask key if present (client frames are always masked).
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(wc.rw, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	// Read payload.
	payload := make([]byte, length)
	if _, err := io.ReadFull(wc.rw, payload); err != nil {
		return 0, nil, err
	}

	// Unmask if needed.
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return opcode, payload, nil
}

// close closes the underlying TCP connection.
func (wc *wsConn) close() {
	if wc.closed {
		return
	}
	wc.closed = true
	wc.conn.Close()
}

// setReadDeadline sets the read deadline on the underlying connection.
func (wc *wsConn) setReadDeadline(t time.Time) error {
	return wc.conn.SetReadDeadline(t)
}
