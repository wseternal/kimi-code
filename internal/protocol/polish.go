// Package protocol provides additional wire types and infrastructure for
// nice-to-have features: notification XML (#77), LLM request logging (#78),
// session hooks (#79), compaction strategies (#80), system reminder (#81),
// tool call dedup (#82), stream decode stats (#83), generate callbacks (#84),
// text decode error modes (#86), file line-ending analysis (#87),
// ULID request ID (#90).
package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── Gap #77: Notification XML ──

// NotificationXML represents a structured notification in XML format.
type NotificationXML struct {
	Type    string `json:"type"`    // "info", "warning", "error", "success"
	Title   string `json:"title"`
	Body    string `json:"body"`
	Action  string `json:"action,omitempty"`
}

// ToXML renders the notification as XML.
func (n *NotificationXML) ToXML() string {
	return fmt.Sprintf(`<notification type="%s"><title>%s</title><body>%s</body></notification>`,
		n.Type, escapeXML(n.Title), escapeXML(n.Body))
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// ── Gap #78: LLM Request Logging ──

// LLMRequestLog records an LLM API request/response for debugging.
type LLMRequestLog struct {
	ID         string         `json:"id"`
	Timestamp  time.Time      `json:"timestamp"`
	Provider   string         `json:"provider"`
	Model      string         `json:"model"`
	Messages   int            `json:"message_count"`
	TokensIn   int            `json:"tokens_in"`
	TokensOut  int            `json:"tokens_out"`
	Duration   time.Duration  `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	FinishReason string       `json:"finish_reason,omitempty"`
}

// LLMRequestLogger accumulates LLM request logs.
type LLMRequestLogger struct {
	mu   sync.Mutex
	logs []LLMRequestLog
	max  int
}

// NewLLMRequestLogger creates a request logger.
func NewLLMRequestLogger(maxEntries int) *LLMRequestLogger {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	return &LLMRequestLogger{max: maxEntries}
}

// Record adds a log entry.
func (l *LLMRequestLogger) Record(entry LLMRequestLog) {
	entry.Timestamp = time.Now()
	l.mu.Lock()
	if len(l.logs) >= l.max {
		l.logs = l.logs[1:]
	}
	l.logs = append(l.logs, entry)
	l.mu.Unlock()
}

// Recent returns the most recent N entries.
func (l *LLMRequestLogger) Recent(n int) []LLMRequestLog {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.logs) {
		n = len(l.logs)
	}
	start := len(l.logs) - n
	result := make([]LLMRequestLog, n)
	copy(result, l.logs[start:])
	return result
}

// ── Gap #79: Session Hooks ──

// HookEvent represents a session lifecycle event for hooks.
type HookEvent string

const (
	HookSessionCreated    HookEvent = "session.created"
	HookSessionStarted    HookEvent = "session.started"
	HookTurnStarted       HookEvent = "turn.started"
	HookTurnCompleted     HookEvent = "turn.completed"
	HookToolCallStarted   HookEvent = "tool_call.started"
	HookToolCallCompleted HookEvent = "tool_call.completed"
	HookCompactionStarted HookEvent = "compaction.started"
	HookSessionEnded      HookEvent = "session.ended"
)

// SessionHook is a callback triggered on session lifecycle events.
type SessionHook struct {
	Event   HookEvent                `json:"event"`
	Command string                   `json:"command,omitempty"`
	Handler func(ctx map[string]any) `json:"-"`
}

// HookEngine manages session hooks.
type HookEngine struct {
	mu    sync.RWMutex
	hooks map[HookEvent][]SessionHook
}

// NewHookEngine creates a hook engine.
func NewHookEngine() *HookEngine {
	return &HookEngine{hooks: make(map[HookEvent][]SessionHook)}
}

// Register adds a hook.
func (e *HookEngine) Register(hook SessionHook) {
	e.mu.Lock()
	e.hooks[hook.Event] = append(e.hooks[hook.Event], hook)
	e.mu.Unlock()
}

// Fire triggers all hooks for an event.
func (e *HookEngine) Fire(event HookEvent, ctx map[string]any) {
	e.mu.RLock()
	hooks := e.hooks[event]
	e.mu.RUnlock()
	for _, h := range hooks {
		if h.Handler != nil {
			h.Handler(ctx)
		}
	}
}

// ── Gap #80: Compaction Strategies ──

// CompactionStrategy defines how context compaction is performed.
type CompactionStrategy string

const (
	CompactionFull    CompactionStrategy = "full"     // summarize entire context
	CompactionMicro   CompactionStrategy = "micro"    // drop old messages, keep recent
	CompactionHandoff CompactionStrategy = "handoff"  // summarize and start fresh
)

// CompactionConfig configures a compaction strategy.
type CompactionConfig struct {
	Strategy        CompactionStrategy `json:"strategy"`
	KeepLastN       int                `json:"keep_last_n,omitempty"`
	TokenThreshold  int                `json:"token_threshold,omitempty"`
	SummaryPrompt   string             `json:"summary_prompt,omitempty"`
}

// DefaultCompactionConfig returns the default compaction configuration.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Strategy:       CompactionFull,
		KeepLastN:      10,
		TokenThreshold: 100000,
		SummaryPrompt:  "Summarize the conversation so far, preserving key decisions and context.",
	}
}

// ── Gap #81: System Reminder Injection ──

// SystemReminder is a system prompt reminder injected at specific points.
type SystemReminder struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	When    string `json:"when"` // "every_turn", "on_compaction", "on_tool_call"
	Priority int   `json:"priority"`
}

// ReminderInjector manages system reminders.
type ReminderInjector struct {
	mu        sync.RWMutex
	reminders []SystemReminder
}

// NewReminderInjector creates a reminder injector.
func NewReminderInjector() *ReminderInjector {
	return &ReminderInjector{}
}

// Add adds a reminder.
func (r *ReminderInjector) Add(reminder SystemReminder) {
	r.mu.Lock()
	r.reminders = append(r.reminders, reminder)
	r.mu.Unlock()
}

// GetForEvent returns reminders that should fire for a given event.
func (r *ReminderInjector) GetForEvent(when string) []SystemReminder {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []SystemReminder
	for _, rem := range r.reminders {
		if rem.When == when {
			result = append(result, rem)
		}
	}
	return result
}

// ── Gap #82: Tool Call Deduplication ──

// ToolCallSignature generates a dedup key for a tool call.
type ToolCallSignature struct {
	ToolName string `json:"tool_name"`
	Input    string `json:"input"` // hash of input
}

// ToolCallDedup tracks recent tool calls to prevent duplicates.
type ToolCallDedup struct {
	mu     sync.Mutex
	recent map[string]time.Time
	window time.Duration
}

// NewToolCallDedup creates a dedup tracker.
func NewToolCallDedup(window time.Duration) *ToolCallDedup {
	if window <= 0 {
		window = 5 * time.Second
	}
	return &ToolCallDedup{
		recent: make(map[string]time.Time),
		window: window,
	}
}

// IsDuplicate checks if a tool call is a recent duplicate.
func (d *ToolCallDedup) IsDuplicate(sig ToolCallSignature) bool {
	key := sig.ToolName + ":" + sig.Input
	d.mu.Lock()
	defer d.mu.Unlock()

	// Clean old entries
	now := time.Now()
	for k, t := range d.recent {
		if now.Sub(t) > d.window {
			delete(d.recent, k)
		}
	}

	if lastSeen, ok := d.recent[key]; ok {
		if now.Sub(lastSeen) <= d.window {
			return true
		}
	}
	d.recent[key] = now
	return false
}

// ── Gap #83: Stream Decode Stats ──

// StreamDecodeStats tracks streaming decode performance.
type StreamDecodeStats struct {
	TotalChunks    int           `json:"total_chunks"`
	DecodedChunks  int           `json:"decoded_chunks"`
	FailedChunks   int           `json:"failed_chunks"`
	TotalBytes     int64         `json:"total_bytes"`
	TotalDuration  time.Duration `json:"total_duration"`
	AvgChunkSize   float64       `json:"avg_chunk_size"`
}

// RecordChunk records a decoded chunk.
func (s *StreamDecodeStats) RecordChunk(bytes int, duration time.Duration, failed bool) {
	s.TotalChunks++
	s.TotalBytes += int64(bytes)
	s.TotalDuration += duration
	if failed {
		s.FailedChunks++
	} else {
		s.DecodedChunks++
	}
	if s.DecodedChunks > 0 {
		s.AvgChunkSize = float64(s.TotalBytes) / float64(s.DecodedChunks)
	}
}

// ── Gap #84: Generate Callbacks ──

// MessagePartCallback is called when a message part is received.
type MessagePartCallback func(partType string, data []byte)

// ToolCallCallback is called when a tool call is initiated.
type ToolCallCallback func(toolName string, input []byte)

// GenerateCallbacks holds callbacks for the generate flow.
type GenerateCallbacks struct {
	OnMessagePart MessagePartCallback `json:"-"`
	OnToolCall    ToolCallCallback    `json:"-"`
}

// ── Gap #86: Text Decode Error Modes ──

// TextDecodeError represents a text decoding error.
type TextDecodeError struct {
	Encoding string `json:"encoding"`
	Position int    `json:"position"`
	Byte     byte   `json:"byte"`
	Reason   string `json:"reason"`
}

// SafeDecode attempts to decode text with fallback.
func SafeDecode(data []byte, encoding string) (string, *TextDecodeError) {
	// Try UTF-8 first (default for Go)
	text := string(data)
	// Check for invalid UTF-8 sequences
	for i := 0; i < len(data); {
		r, size := decodeRune(data[i:])
		if r == 0xFFFD && size == 1 {
			return text, &TextDecodeError{
				Encoding: encoding,
				Position: i,
				Byte:     data[i],
				Reason:   "invalid UTF-8 sequence",
			}
		}
		i += size
	}
	return text, nil
}

func decodeRune(data []byte) (rune, int) {
	if len(data) == 0 {
		return 0, 0
	}
	b := data[0]
	if b < 0x80 {
		return rune(b), 1
	}
	// Multi-byte UTF-8
	var size int
	switch {
	case b < 0xC0:
		return 0xFFFD, 1
	case b < 0xE0:
		size = 2
	case b < 0xF0:
		size = 3
	default:
		size = 4
	}
	if len(data) < size {
		return 0xFFFD, 1
	}
	r := rune(b & (0x7F >> uint(size)))
	for i := 1; i < size; i++ {
		if data[i]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
		r = r<<6 | rune(data[i]&0x3F)
	}
	return r, size
}

// ── Gap #87: File Line-Ending Analysis ──

// LineEnding is the detected line ending style.
type LineEnding string

const (
	LineEndingLF   LineEnding = "lf"
	LineEndingCRLF LineEnding = "crlf"
	LineEndingCR   LineEnding = "cr"
	LineEndingMixed LineEnding = "mixed"
)

// DetectLineEnding analyzes a file's line endings.
func DetectLineEnding(data []byte) LineEnding {
	lf := 0
	crlf := 0
	cr := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' {
			if i+1 < len(data) && data[i+1] == '\n' {
				crlf++
				i++ // skip \n
			} else {
				cr++
			}
		} else if data[i] == '\n' {
			lf++
		}
	}

	if crlf > 0 && lf > 0 {
		return LineEndingMixed
	}
	if crlf > 0 {
		return LineEndingCRLF
	}
	if cr > 0 {
		return LineEndingCR
	}
	return LineEndingLF
}

// ── Gap #90: ULID Request ID ──

// GenerateRequestID generates a ULID-like request ID.
// Format: timestamp (10 chars) + random (16 chars) = 26 chars total.
func GenerateRequestID() string {
	now := time.Now()
	ts := fmt.Sprintf("%010d", now.UnixMilli())
	b := make([]byte, 8)
	rand.Read(b)
	return "req_" + ts + hex.EncodeToString(b)
}
