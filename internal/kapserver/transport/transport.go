package transport

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/visdomtech/kimi-code/internal/protocol/ws"
)

// Connection represents a WebSocket connection.
type Connection struct {
	ID        string
	conn      http.ResponseWriter
	req       *http.Request
	send      chan []byte
	done      chan struct{}
	logger    *slog.Logger
	mu        sync.Mutex
	closed    bool
}

// Registry manages active WebSocket connections.
type Registry struct {
	connections map[string]*Connection
	mu          sync.RWMutex
	logger      *slog.Logger
}

// NewRegistry creates a new connection registry.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		connections: make(map[string]*Connection),
		logger:      logger,
	}
}

// Add adds a connection to the registry.
func (r *Registry) Add(conn *Connection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections[conn.ID] = conn
}

// Remove removes a connection from the registry.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, id)
}

// Get returns a connection by ID.
func (r *Registry) Get(id string) (*Connection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, ok := r.connections[id]
	return conn, ok
}

// Count returns the number of active connections.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connections)
}

// Broadcast sends a message to all connections.
func (r *Registry) Broadcast(msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, conn := range r.connections {
		conn.Send(msg)
	}
}

// Send sends a message to the connection's send channel.
func (c *Connection) Send(msg []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.send <- msg:
	default:
		// Drop if buffer full (backpressure)
		c.logger.Warn("connection send buffer full, dropping message", "conn_id", c.ID)
	}
}

// Close closes the connection.
func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.done)
}

// Broadcaster manages per-session event broadcasting.
type Broadcaster struct {
	registry    *Registry
	sequence    atomic.Int64
	epoch       string
	ringBuffer  []BroadcasterEvent
	ringSize    int
	mu          sync.RWMutex
	logger      *slog.Logger
}

// BroadcasterEvent is an event in the broadcaster ring buffer.
type BroadcasterEvent struct {
	Seq  int64           `json:"seq"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
	Time time.Time       `json:"time"`
}

// NewBroadcaster creates a new broadcaster.
func NewBroadcaster(registry *Registry, ringSize int, logger *slog.Logger) *Broadcaster {
	if ringSize <= 0 {
		ringSize = 1000
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Broadcaster{
		registry:   registry,
		epoch:      time.Now().Format(time.RFC3339Nano),
		ringBuffer: make([]BroadcasterEvent, 0, ringSize),
		ringSize:   ringSize,
		logger:     logger,
	}
}

// Publish publishes an event to all subscribers.
func (b *Broadcaster) Publish(eventType string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		b.logger.Error("failed to marshal event", "error", err)
		return
	}

	seq := b.sequence.Add(1)
	event := BroadcasterEvent{
		Seq:  seq,
		Type: eventType,
		Data: raw,
		Time: time.Now(),
	}

	b.mu.Lock()
	if len(b.ringBuffer) >= b.ringSize {
		// Remove oldest
		b.ringBuffer = b.ringBuffer[1:]
	}
	b.ringBuffer = append(b.ringBuffer, event)
	b.mu.Unlock()

	// Broadcast to all connections
	msg, _ := json.Marshal(map[string]any{
		"type": eventType,
		"seq":  seq,
		"data": data,
	})
	b.registry.Broadcast(msg)
}

// EventsAfter returns events after a given sequence number.
func (b *Broadcaster) EventsAfter(seq int64) []BroadcasterEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []BroadcasterEvent
	for _, e := range b.ringBuffer {
		if e.Seq > seq {
			result = append(result, e)
		}
	}
	return result
}

// Seq returns the current sequence number.
func (b *Broadcaster) Seq() int64 {
	return b.sequence.Load()
}

// Epoch returns the broadcaster epoch.
func (b *Broadcaster) Epoch() string {
	return b.epoch
}

// HandleWebSocket handles a WebSocket upgrade request.
// Note: This is a stub. Real implementation would use nhooyr.io/websocket or gorilla/websocket.
func HandleWebSocket(registry *Registry, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For now, return a hello message
		hello := ws.ServerHelloMessage{
			Type:      "server_hello",
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Payload: ws.ServerHelloPayload{
				ProtocolVersion:    2,
				MaxEventBufferSize: 1000,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hello)
	}
}
