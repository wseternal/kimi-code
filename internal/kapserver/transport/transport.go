package transport

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/visdomtech/kimi-code/internal/protocol/ws"
)

const (
	// heartbeatInterval is how often the server sends pings.
	heartbeatInterval = 30 * time.Second
	// pongTimeout is how long to wait for a pong before closing.
	pongTimeout = heartbeatInterval + 10*time.Second
	// writeTimeout is the max time to write a frame.
	writeTimeout = 10 * time.Second
)

// S9: atomic counter for connection IDs to avoid time.Now() collisions.
var connIDCounter atomic.Int64

// Connection represents a WebSocket connection.
type Connection struct {
	ID            string
	ClientID      string
	ws            *wsConn // real WebSocket connection (set after upgrade)
	send          chan []byte
	done          chan struct{}
	logger        *slog.Logger
	mu            sync.Mutex
	closed        bool
	subscriptions map[string]bool // session_id -> subscribed
	cursors       ws.CursorsBySession
	agentFilter   ws.AgentFilter
	connectedAt   time.Time
	lastPong      time.Time
}

// NewConnection creates a connection with defaults.
func NewConnection(id, clientID string, logger *slog.Logger) *Connection {
	if logger == nil {
		logger = slog.Default()
	}
	return &Connection{
		ID:            id,
		ClientID:      clientID,
		send:          make(chan []byte, 256),
		done:          make(chan struct{}),
		logger:        logger,
		subscriptions: make(map[string]bool),
		cursors:       make(ws.CursorsBySession),
		agentFilter:   make(ws.AgentFilter),
		connectedAt:   time.Now(),
		lastPong:      time.Now(),
	}
}

// Subscribe adds session subscriptions.
func (c *Connection) Subscribe(sessionIDs []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var accepted []string
	for _, id := range sessionIDs {
		c.subscriptions[id] = true
		accepted = append(accepted, id)
	}
	return accepted
}

// Unsubscribe removes session subscriptions.
func (c *Connection) Unsubscribe(sessionIDs []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var removed []string
	for _, id := range sessionIDs {
		if c.subscriptions[id] {
			delete(c.subscriptions, id)
			removed = append(removed, id)
		}
	}
	return removed
}

// IsSubscribed reports whether the connection subscribes to a session.
func (c *Connection) IsSubscribed(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.subscriptions[sessionID]
}

// Subscriptions returns all subscribed session IDs.
func (c *Connection) Subscriptions() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, 0, len(c.subscriptions))
	for id := range c.subscriptions {
		result = append(result, id)
	}
	return result
}

// SetCursors updates the connection's session cursors.
func (c *Connection) SetCursors(cursors ws.CursorsBySession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range cursors {
		c.cursors[k] = v
	}
}

// GetCursor returns the cursor for a session.
func (c *Connection) GetCursor(sessionID string) (ws.SessionCursor, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.cursors[sessionID]
	return cur, ok
}

// UpdatePong updates the last pong time.
func (c *Connection) UpdatePong() {
	c.mu.Lock()
	c.lastPong = time.Now()
	c.mu.Unlock()
}

// IsAlive checks if the connection responded to the last ping within the timeout.
func (c *Connection) IsAlive(timeout time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Since(c.lastPong) < timeout
}

// Registry manages active WebSocket connections.
type Registry struct {
	connections map[string]*Connection
	byClient    map[string][]string // client_id -> []conn_id
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
		byClient:    make(map[string][]string),
		logger:      logger,
	}
}

// Add adds a connection to the registry.
func (r *Registry) Add(conn *Connection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections[conn.ID] = conn
	r.byClient[conn.ClientID] = append(r.byClient[conn.ClientID], conn.ID)
}

// Remove removes a connection from the registry.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	conn, ok := r.connections[id]
	if !ok {
		return
	}
	delete(r.connections, id)
	// Remove from byClient index
	clientConns := r.byClient[conn.ClientID]
	for i, cid := range clientConns {
		if cid == id {
			r.byClient[conn.ClientID] = append(clientConns[:i], clientConns[i+1:]...)
			break
		}
	}
	if len(r.byClient[conn.ClientID]) == 0 {
		delete(r.byClient, conn.ClientID)
	}
}

// Get returns a connection by ID.
func (r *Registry) Get(id string) (*Connection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, ok := r.connections[id]
	return conn, ok
}

// GetByClient returns all connections for a client ID.
func (r *Registry) GetByClient(clientID string) []*Connection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byClient[clientID]
	result := make([]*Connection, 0, len(ids))
	for _, id := range ids {
		if conn, ok := r.connections[id]; ok {
			result = append(result, conn)
		}
	}
	return result
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

// BroadcastToSession sends a message to all connections subscribed to a session.
// TODO(S8): Build a reverse index (sessionID → []conn) to avoid scanning all
// connections on every broadcast, reducing from O(C) to O(S) where S is subscribers.
func (r *Registry) BroadcastToSession(sessionID string, msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, conn := range r.connections {
		if conn.IsSubscribed(sessionID) {
			conn.Send(msg)
		}
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

// Close closes the connection and its send channel (S7 fix).
func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.done) // signal writePump to send close frame and exit
}

// Broadcaster manages per-session event broadcasting with journal replay.
type Broadcaster struct {
	registry    *Registry
	sequence    atomic.Int64
	epoch       string
	ring        ringBuf[BroadcasterEvent] // fixed-size circular buffer
	journal     *EventJournal
	turnTracker *TurnTracker
	mu          sync.RWMutex
	logger      *slog.Logger
}

// BroadcasterEvent is an event in the broadcaster ring buffer.
type BroadcasterEvent struct {
	Seq       int64           `json:"seq"`
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Data      json.RawMessage `json:"data"`
	Volatile  bool            `json:"volatile,omitempty"`
	Time      time.Time       `json:"time"`
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
		registry:    registry,
		epoch:       time.Now().Format(time.RFC3339Nano),
		ring:        newRingBuf[BroadcasterEvent](ringSize),
		journal:     NewEventJournal(),
		turnTracker: NewTurnTracker(),
		logger:      logger,
	}
}

// Publish publishes an event to all subscribers.
func (b *Broadcaster) Publish(eventType string, sessionID string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		b.logger.Error("failed to marshal event", "error", err)
		return
	}

	seq := b.sequence.Add(1)
	now := time.Now()
	event := BroadcasterEvent{
		Seq:       seq,
		Type:      eventType,
		SessionID: sessionID,
		Data:      raw,
		Time:      now,
	}

	b.mu.Lock()
	b.ring.Push(event)
	b.mu.Unlock()

	// Persist to journal for replay
	if sessionID != "" {
		b.journal.Append(sessionID, event)
	}

	// Build the wire message once and reuse (W13: avoid double marshal).
	msg, _ := json.Marshal(map[string]any{
		"type":       eventType,
		"seq":        seq,
		"epoch":      b.epoch,
		"session_id": sessionID,
		"timestamp":  now.Format(time.RFC3339Nano),
		"data":       json.RawMessage(raw),
	})
	if sessionID != "" {
		b.registry.BroadcastToSession(sessionID, msg)
	} else {
		b.registry.Broadcast(msg)
	}
}

// PublishVolatile publishes a volatile event (not persisted to journal).
func (b *Broadcaster) PublishVolatile(eventType string, sessionID string, data any) {
	seq := b.sequence.Add(1)
	msg, err := json.Marshal(map[string]any{
		"type":       eventType,
		"seq":        seq,
		"epoch":      b.epoch,
		"session_id": sessionID,
		"volatile":   true,
		"timestamp":  time.Now().Format(time.RFC3339Nano),
		"data":       data,
	})
	if err != nil {
		b.logger.Error("failed to marshal volatile event", "error", err)
		return
	}

	if sessionID != "" {
		b.registry.BroadcastToSession(sessionID, msg)
	} else {
		b.registry.Broadcast(msg)
	}
}

// EventsAfter returns events after a given sequence number.
func (b *Broadcaster) EventsAfter(seq int64) []BroadcasterEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var result []BroadcasterEvent
	for _, e := range b.ring.All() {
		if e.Seq > seq {
			result = append(result, e)
		}
	}
	return result
}

// ReplayForSession returns journal events for a session after a cursor.
func (b *Broadcaster) ReplayForSession(sessionID string, afterSeq int64) []BroadcasterEvent {
	return b.journal.EventsAfter(sessionID, afterSeq)
}

// NeedsResync checks if a client cursor is too far behind to replay.
func (b *Broadcaster) NeedsResync(sessionID string, clientEpoch string) bool {
	return clientEpoch != "" && clientEpoch != b.epoch
}

// Seq returns the current sequence number.
func (b *Broadcaster) Seq() int64 {
	return b.sequence.Load()
}

// Epoch returns the broadcaster epoch.
func (b *Broadcaster) Epoch() string {
	return b.epoch
}

// TurnTracker returns the in-flight turn tracker.
func (b *Broadcaster) TurnTracker() *TurnTracker {
	return b.turnTracker
}

// Journal returns the event journal.
func (b *Broadcaster) Journal() *EventJournal {
	return b.journal
}

// HandleWebSocket handles a WebSocket upgrade request using RFC 6455
// handshake via net/http.Hijacker, then runs read/write pumps with heartbeat.
func HandleWebSocket(registry *Registry, broadcaster *Broadcaster, logger *slog.Logger) http.HandlerFunc {
	return HandleWebSocketWithOrigins(registry, broadcaster, logger, nil)
}

// HandleWebSocketWithOrigins handles WebSocket upgrades with optional origin restriction.
func HandleWebSocketWithOrigins(registry *Registry, broadcaster *Broadcaster, logger *slog.Logger, allowedOrigins []string) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Perform WebSocket handshake.
		wsconn, err := upgradeWebSocket(w, r, allowedOrigins)
		if err != nil {
			logger.Error("websocket upgrade failed", "error", err)
			return
		}

		// S9 fix: use atomic counter for connection ID to avoid collisions.
		connID := fmt.Sprintf("ws_%d", connIDCounter.Add(1))
		clientID := r.URL.Query().Get("client_id")
		if clientID == "" {
			clientID = "anonymous"
		}

		// Create a logger scoped to this connection.
		connLogger := logger.With("conn_id", connID, "client_id", clientID)

		// W10 fix: use NewConnection constructor, then assign ws.
		conn := NewConnection(connID, clientID, connLogger)
		conn.ws = wsconn
		registry.Add(conn)
		defer func() {
			registry.Remove(connID)
			conn.Close()
			wsconn.close()
		}()

		connLogger.Info("websocket connection opened")

		// Send server hello over the WebSocket.
		hello := ws.ServerHelloMessage{
			Type:      "server_hello",
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Payload: ws.ServerHelloPayload{
				WSConnectionID:     connID,
				ProtocolVersion:    ws.ProtocolVersion,
				HeartbeatMs:        int(heartbeatInterval.Milliseconds()),
				MaxEventBufferSize: broadcaster.ring.Cap(),
				Capabilities: ws.ServerHelloCapabilities{
					EventBatching: true,
					Compression:   false,
				},
			},
		}
		helloJSON, _ := json.Marshal(hello)
		// W13 fix: use connLogger instead of logger for connection-scoped context.
		if err := conn.ws.writeText(string(helloJSON)); err != nil {
			connLogger.Error("failed to send server hello", "error", err)
			return
		}

		// Run read and write pumps concurrently.
		errCh := make(chan error, 2)
		go func() { errCh <- conn.writePump() }()
		go func() { errCh <- conn.readPump(registry, broadcaster) }()

		// Wait for the first pump to finish, then close the TCP connection
		// to force the other pump to exit, and wait for it.
		<-errCh
		wsconn.close() // close TCP to unblock the other pump
		<-errCh
		connLogger.Info("websocket connection closed")
	}
}

// writePump drains the send channel and writes frames to the WebSocket.
// It also sends periodic ping frames for heartbeat.
func (c *Connection) writePump() error {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case msg := <-c.send:
			c.ws.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.ws.writeText(string(msg)); err != nil {
				c.logger.Error("writePump: write failed", "error", err)
				return err
			}
		case <-ticker.C:
			c.ws.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.ws.writePing(nil); err != nil {
				c.logger.Error("writePump: ping failed", "error", err)
				return err
			}
		case <-c.done:
			c.ws.writeClose(1000, "closing")
			return nil
		}
	}
}

// readPump reads frames from the WebSocket and dispatches them.
func (c *Connection) readPump(registry *Registry, broadcaster *Broadcaster) error {
	c.ws.setReadDeadline(time.Now().Add(pongTimeout))
	for {
		opcode, payload, err := c.ws.readFrame()
		if err != nil {
			c.logger.Error("readPump: read failed", "error", err) // W12b
			return err
		}
		// Reset read deadline after each successful read.
		c.ws.setReadDeadline(time.Now().Add(pongTimeout))

		switch opcode {
		case opText:
			c.handleMessage(payload, registry, broadcaster)
		case opPing:
			// Respond to ping with pong (writePong is thread-safe via writeMu).
			c.ws.writePong(payload)
		case opPong:
			c.UpdatePong()
		case opClose:
			c.logger.Info("readPump: received close frame") // W12b
			return nil
		}
	}
}

// handleMessage dispatches a text message to the appropriate handler.
// W9 fix: route ACK responses via send channel to maintain single-writer guarantee.
func (c *Connection) handleMessage(data []byte, registry *Registry, broadcaster *Broadcaster) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		c.logger.Warn("invalid message envelope", "error", err) // W12b
		return
	}

	switch envelope.Type {
	case "client_hello":
		var msg ws.ClientHelloMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Warn("invalid client_hello", "error", err) // W12b
			return
		}
		ack := HandleClientHello(c, broadcaster, msg)
		resp, _ := json.Marshal(map[string]interface{}{"type": "client_hello_ack", "payload": ack})
		c.Send(resp) // W9: route via send channel

	case "subscribe":
		var msg ws.SubscribeMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Warn("invalid subscribe", "error", err) // W12b
			return
		}
		ack := HandleSubscribe(c, broadcaster, msg)
		resp, _ := json.Marshal(map[string]interface{}{"type": "subscribe_ack", "payload": ack})
		c.Send(resp) // W9: route via send channel

	case "unsubscribe":
		var msg ws.UnsubscribeMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Warn("invalid unsubscribe", "error", err) // W12b
			return
		}
		HandleUnsubscribe(c, msg)
		resp, _ := json.Marshal(map[string]interface{}{"type": "unsubscribe_ack"})
		c.Send(resp) // W9: route via send channel

	default:
		c.logger.Debug("unhandled message type", "type", envelope.Type)
	}
}

// HandleClientHello processes a client_hello message and returns the ack.
func HandleClientHello(conn *Connection, broadcaster *Broadcaster, msg ws.ClientHelloMessage) ws.ClientHelloAckPayload {
	// Apply deprecated subscription fields
	if len(msg.Payload.Subscriptions) > 0 {
		conn.Subscribe(msg.Payload.Subscriptions)
	}
	if msg.Payload.Cursors != nil {
		conn.SetCursors(msg.Payload.Cursors)
	}
	if msg.Payload.AgentFilter != nil {
		conn.mu.Lock()
		conn.agentFilter = msg.Payload.AgentFilter
		conn.mu.Unlock()
	}

	return ws.ClientHelloAckPayload{
		AcceptedSubscriptions: conn.Subscriptions(),
	}
}

// HandleSubscribe processes a subscribe message.
func HandleSubscribe(conn *Connection, broadcaster *Broadcaster, msg ws.SubscribeMessage) ws.SubscribeAckPayload {
	accepted := conn.Subscribe(msg.Payload.SessionIDs)
	if msg.Payload.Cursors != nil {
		conn.SetCursors(msg.Payload.Cursors)
	}
	if msg.Payload.AgentFilter != nil {
		conn.mu.Lock()
		conn.agentFilter = msg.Payload.AgentFilter
		conn.mu.Unlock()
	}

	// Check for resync needs
	var resyncRequired []string
	for _, sid := range accepted {
		if cursor, ok := conn.GetCursor(sid); ok {
			if broadcaster.NeedsResync(sid, cursor.Epoch) {
				resyncRequired = append(resyncRequired, sid)
			}
		}
	}

	return ws.SubscribeAckPayload{
		Accepted:       accepted,
		ResyncRequired: resyncRequired,
	}
}

// HandleUnsubscribe processes an unsubscribe message.
func HandleUnsubscribe(conn *Connection, msg ws.UnsubscribeMessage) {
	conn.Unsubscribe(msg.Payload.SessionIDs)
}

// ── Ring Buffer ──

// ringBuf is a generic fixed-size circular buffer.
type ringBuf[T any] struct {
	data  []T
	head  int // index of the oldest element
	count int // number of elements currently stored
}

// newRingBuf creates a ring buffer with the given capacity.
func newRingBuf[T any](capacity int) ringBuf[T] {
	return ringBuf[T]{data: make([]T, capacity)}
}

// Push writes an item, overwriting the oldest when full.
func (r *ringBuf[T]) Push(item T) {
	idx := (r.head + r.count) % len(r.data)
	if r.count == len(r.data) {
		r.head = (r.head + 1) % len(r.data)
	} else {
		r.count++
	}
	r.data[idx] = item
}

// All returns all items in insertion order (oldest first).
func (r *ringBuf[T]) All() []T {
	out := make([]T, 0, r.count)
	for i := 0; i < r.count; i++ {
		out = append(out, r.data[(r.head+i)%len(r.data)])
	}
	return out
}

// Cap returns the buffer capacity.
func (r *ringBuf[T]) Cap() int { return len(r.data) }

// ── Event Journal ──

// EventJournal stores per-session event history for replay on reconnect.
// W11 fix: uses ring buffer per session to avoid O(n) slice shift at capacity.
type EventJournal struct {
	mu            sync.RWMutex
	sessions      map[string]*ringBuf[BroadcasterEvent]
	maxPerSession int
}

// NewEventJournal creates a new event journal.
func NewEventJournal() *EventJournal {
	return &EventJournal{
		sessions:      make(map[string]*ringBuf[BroadcasterEvent]),
		maxPerSession: 5000,
	}
}

// Append adds an event to a session's journal using ring buffer (W11 fix).
func (j *EventJournal) Append(sessionID string, event BroadcasterEvent) {
	j.mu.Lock()
	defer j.mu.Unlock()
	ring, ok := j.sessions[sessionID]
	if !ok {
		rb := newRingBuf[BroadcasterEvent](j.maxPerSession)
		ring = &rb
		j.sessions[sessionID] = ring
	}
	ring.Push(event)
}

// EventsAfter returns events for a session after a given sequence.
func (j *EventJournal) EventsAfter(sessionID string, afterSeq int64) []BroadcasterEvent {
	j.mu.RLock()
	defer j.mu.RUnlock()
	ring, ok := j.sessions[sessionID]
	if !ok || ring.count == 0 {
		return nil
	}
	var result []BroadcasterEvent
	for _, e := range ring.All() {
		if e.Seq > afterSeq {
			result = append(result, e)
		}
	}
	return result
}

// Clear removes the journal for a session.
func (j *EventJournal) Clear(sessionID string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.sessions, sessionID)
}

// Count returns the number of events for a session.
func (j *EventJournal) Count(sessionID string) int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if ring, ok := j.sessions[sessionID]; ok {
		return ring.count
	}
	return 0
}

// ── Turn Tracker ──

// TurnState tracks the state of an in-flight agent turn.
type TurnState struct {
	SessionID string    `json:"session_id"`
	TurnID    string    `json:"turn_id"`
	PromptID  string    `json:"prompt_id"`
	StartedAt time.Time `json:"started_at"`
	StepCount int       `json:"step_count"`
}

// TurnTracker tracks in-flight turns per session.
type TurnTracker struct {
	mu    sync.RWMutex
	turns map[string]*TurnState // session_id -> current turn
}

// NewTurnTracker creates a new turn tracker.
func NewTurnTracker() *TurnTracker {
	return &TurnTracker{turns: make(map[string]*TurnState)}
}

// StartTurn begins tracking a turn for a session.
func (t *TurnTracker) StartTurn(sessionID, turnID, promptID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.turns[sessionID] = &TurnState{
		SessionID: sessionID,
		TurnID:    turnID,
		PromptID:  promptID,
		StartedAt: time.Now(),
	}
}

// IncrementStep bumps the step counter for a session's turn.
func (t *TurnTracker) IncrementStep(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if turn, ok := t.turns[sessionID]; ok {
		turn.StepCount++
	}
}

// EndTurn removes the turn tracking for a session.
func (t *TurnTracker) EndTurn(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.turns, sessionID)
}

// GetTurn returns the current turn state for a session.
func (t *TurnTracker) GetTurn(sessionID string) (*TurnState, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	turn, ok := t.turns[sessionID]
	return turn, ok
}

// ActiveTurns returns all session IDs with in-flight turns.
func (t *TurnTracker) ActiveTurns() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]string, 0, len(t.turns))
	for sid := range t.turns {
		result = append(result, sid)
	}
	return result
}

// IsSessionBusy reports whether a session has an in-flight turn.
func (t *TurnTracker) IsSessionBusy(sessionID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.turns[sessionID]
	return ok
}
