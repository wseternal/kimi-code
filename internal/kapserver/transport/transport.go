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

// Connection represents a WebSocket connection.
type Connection struct {
	ID            string
	ClientID      string
	conn          http.ResponseWriter
	req           *http.Request
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

// Broadcaster manages per-session event broadcasting with journal replay.
type Broadcaster struct {
	registry    *Registry
	sequence    atomic.Int64
	epoch       string
	ring        []BroadcasterEvent // fixed-size circular buffer
	ringHead    int                // index of the oldest element
	ringCount   int                // number of elements currently in the ring
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
		ring:        make([]BroadcasterEvent, ringSize),
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
	event := BroadcasterEvent{
		Seq:       seq,
		Type:      eventType,
		SessionID: sessionID,
		Data:      raw,
		Time:      time.Now(),
	}

	b.mu.Lock()
	// Circular buffer write: overwrite oldest when full.
	idx := (b.ringHead + b.ringCount) % len(b.ring)
	if b.ringCount == len(b.ring) {
		// Buffer full — advance head (overwrite oldest)
		b.ringHead = (b.ringHead + 1) % len(b.ring)
	} else {
		b.ringCount++
	}
	b.ring[idx] = event
	b.mu.Unlock()

	// Persist to journal for replay
	if sessionID != "" {
		b.journal.Append(sessionID, event)
	}

	// Route to session subscribers or broadcast globally
	msg, _ := json.Marshal(map[string]any{
		"type":       eventType,
		"seq":        seq,
		"epoch":      b.epoch,
		"session_id": sessionID,
		"timestamp":  event.Time.Format(time.RFC3339Nano),
		"data":       data,
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
	for i := 0; i < b.ringCount; i++ {
		e := b.ring[(b.ringHead+i)%len(b.ring)]
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

// HandleWebSocket handles a WebSocket upgrade request.
// Note: This is a stub. Real implementation would use nhooyr.io/websocket or gorilla/websocket.
func HandleWebSocket(registry *Registry, broadcaster *Broadcaster, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		connID := fmt.Sprintf("ws_%d", time.Now().UnixNano())
		clientID := r.URL.Query().Get("client_id")
		if clientID == "" {
			clientID = "anonymous"
		}

		conn := NewConnection(connID, clientID, logger)
		registry.Add(conn)
		defer func() {
			registry.Remove(connID)
			conn.Close()
		}()

		logger.Info("websocket connection opened", "conn_id", connID, "client_id", clientID)

		// Send server hello
		hello := ws.ServerHelloMessage{
			Type:      "server_hello",
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Payload: ws.ServerHelloPayload{
				WSConnectionID:     connID,
				ProtocolVersion:    ws.ProtocolVersion,
				HeartbeatMs:        30000,
				MaxEventBufferSize: len(broadcaster.ring),
				Capabilities: ws.ServerHelloCapabilities{
					EventBatching: true,
					Compression:   false,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hello)
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

// ── Event Journal ──

// EventJournal stores per-session event history for replay on reconnect.
type EventJournal struct {
	mu       sync.RWMutex
	sessions map[string][]BroadcasterEvent
	maxPerSession int
}

// NewEventJournal creates a new event journal.
func NewEventJournal() *EventJournal {
	return &EventJournal{
		sessions:      make(map[string][]BroadcasterEvent),
		maxPerSession: 5000,
	}
}

// Append adds an event to a session's journal.
func (j *EventJournal) Append(sessionID string, event BroadcasterEvent) {
	j.mu.Lock()
	defer j.mu.Unlock()
	events := j.sessions[sessionID]
	if len(events) >= j.maxPerSession {
		events = events[1:]
	}
	j.sessions[sessionID] = append(events, event)
}

// EventsAfter returns events for a session after a given sequence.
func (j *EventJournal) EventsAfter(sessionID string, afterSeq int64) []BroadcasterEvent {
	j.mu.RLock()
	defer j.mu.RUnlock()
	events := j.sessions[sessionID]
	var result []BroadcasterEvent
	for _, e := range events {
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
	return len(j.sessions[sessionID])
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
