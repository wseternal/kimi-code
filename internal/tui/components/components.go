// Package components provides reusable TUI components for the kimi-code interface.
// Covers: session picker (#15), image attachment (#16), MCP server status (#17),
// plugin labels (#18), background task display (#51), goal queue (#52),
// terminal notifications (#53), message replay (#54), paging (#55),
// render cache (#56), tmux keyboard (#57), foreground tasks (#58).
package components

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── Gap #15: Session Picker ──

// SessionItem represents a session in the picker list.
type SessionItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
	Active    bool      `json:"active"`
}

// SessionPicker provides a session selection list.
type SessionPicker struct {
	items    []SessionItem
	selected int
	filter   string
}

// NewSessionPicker creates a session picker.
func NewSessionPicker(items []SessionItem) *SessionPicker {
	return &SessionPicker{items: items}
}

// SetFilter sets the text filter.
func (p *SessionPicker) SetFilter(filter string) {
	p.filter = filter
	p.selected = 0
}

// FilteredItems returns items matching the filter.
func (p *SessionPicker) FilteredItems() []SessionItem {
	if p.filter == "" {
		return p.items
	}
	filter := strings.ToLower(p.filter)
	var result []SessionItem
	for _, item := range p.items {
		if strings.Contains(strings.ToLower(item.Title), filter) ||
			strings.Contains(strings.ToLower(item.ID), filter) {
			result = append(result, item)
		}
	}
	return result
}

// Selected returns the currently selected item.
func (p *SessionPicker) Selected() *SessionItem {
	items := p.FilteredItems()
	if p.selected >= 0 && p.selected < len(items) {
		return &items[p.selected]
	}
	return nil
}

// MoveUp moves the selection up.
func (p *SessionPicker) MoveUp() {
	if p.selected > 0 {
		p.selected--
	}
}

// MoveDown moves the selection down.
func (p *SessionPicker) MoveDown() {
	items := p.FilteredItems()
	if p.selected < len(items)-1 {
		p.selected++
	}
}

// SelectedIndex returns the current selection index.
func (p *SessionPicker) SelectedIndex() int { return p.selected }

// ── Gap #16: Image Attachment ──

// ImageAttachment represents an attached image.
type ImageAttachment struct {
	Path     string `json:"path"`
	MIMEType string `json:"mime_type"`
	Data     []byte `json:"data,omitempty"` // base64 or raw
	Name     string `json:"name"`
	Size     int    `json:"size"`
}

// ImageAttachmentStore manages attached images.
type ImageAttachmentStore struct {
	mu       sync.RWMutex
	images   []ImageAttachment
	maxCount int
	maxSize  int // bytes per image
}

// NewImageAttachmentStore creates an image store.
func NewImageAttachmentStore(maxCount, maxSize int) *ImageAttachmentStore {
	if maxCount <= 0 {
		maxCount = 10
	}
	if maxSize <= 0 {
		maxSize = 20 * 1024 * 1024 // 20MB
	}
	return &ImageAttachmentStore{maxCount: maxCount, maxSize: maxSize}
}

// Add adds an image attachment.
func (s *ImageAttachmentStore) Add(img ImageAttachment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.images) >= s.maxCount {
		return fmt.Errorf("maximum %d attachments", s.maxCount)
	}
	if img.Size > s.maxSize {
		return fmt.Errorf("image too large: %d bytes (max %d)", img.Size, s.maxSize)
	}
	s.images = append(s.images, img)
	return nil
}

// List returns all attachments.
func (s *ImageAttachmentStore) List() []ImageAttachment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ImageAttachment, len(s.images))
	copy(result, s.images)
	return result
}

// Remove removes an attachment by index.
func (s *ImageAttachmentStore) Remove(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.images) {
		return false
	}
	s.images = append(s.images[:index], s.images[index+1:]...)
	return true
}

// Clear removes all attachments.
func (s *ImageAttachmentStore) Clear() {
	s.mu.Lock()
	s.images = nil
	s.mu.Unlock()
}

// Count returns the number of attachments.
func (s *ImageAttachmentStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.images)
}

// ── Gap #17: MCP Server Status ──

// MCPServerInfo describes an MCP server's connection status.
type MCPServerInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // "connected", "disconnected", "error", "starting"
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
	URL       string `json:"url,omitempty"`
}

// MCPServerStatus tracks MCP server connections.
type MCPServerStatus struct {
	mu      sync.RWMutex
	servers map[string]*MCPServerInfo
}

// NewMCPServerStatus creates an MCP server status tracker.
func NewMCPServerStatus() *MCPServerStatus {
	return &MCPServerStatus{servers: make(map[string]*MCPServerInfo)}
}

// Update updates a server's status.
func (m *MCPServerStatus) Update(info MCPServerInfo) {
	m.mu.Lock()
	m.servers[info.Name] = &info
	m.mu.Unlock()
}

// Get returns a server's info.
func (m *MCPServerStatus) Get(name string) *MCPServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.servers[name]
	if !ok {
		return nil
	}
	copy := *s
	return &copy
}

// List returns all servers.
func (m *MCPServerStatus) List() []MCPServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MCPServerInfo, 0, len(m.servers))
	for _, s := range m.servers {
		result = append(result, *s)
	}
	return result
}

// ── Gap #18: Plugin Source Label ──

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"` // "project", "user", "system"
	Active  bool   `json:"active"`
}

// FormatPluginLabel formats a plugin source label for display.
func FormatPluginLabel(plugin PluginInfo) string {
	label := plugin.Name
	if plugin.Version != "" {
		label += "@" + plugin.Version
	}
	if plugin.Source != "" {
		label += fmt.Sprintf(" [%s]", plugin.Source)
	}
	return label
}

// ── Gap #51: Background Task Status Display ──

// BackgroundTaskDisplay renders background task status.
type BackgroundTaskDisplay struct {
	maxVisible int
}

// NewBackgroundTaskDisplay creates a task display.
func NewBackgroundTaskDisplay(maxVisible int) *BackgroundTaskDisplay {
	if maxVisible <= 0 {
		maxVisible = 5
	}
	return &BackgroundTaskDisplay{maxVisible: maxVisible}
}

// Render renders a task status line.
func (d *BackgroundTaskDisplay) Render(taskID, command, status string, pid int) string {
	icon := "●"
	switch status {
	case "running":
		icon = "◉"
	case "completed":
		icon = "✓"
	case "failed":
		icon = "✗"
	case "killed":
		icon = "⊘"
	}
	cmd := command
	if len(cmd) > 40 {
		cmd = cmd[:37] + "..."
	}
	return fmt.Sprintf("%s %s [%s] (pid:%d)", icon, cmd, taskID, pid)
}

// ── Gap #52: Goal Queue Display ──

// GoalQueueItem represents a goal in the queue.
type GoalQueueItem struct {
	ID        string `json:"id"`
	Objective string `json:"objective"`
	Status    string `json:"status"` // "active", "paused", "blocked", "complete"
	Progress  string `json:"progress,omitempty"`
}

// FormatGoalQueue renders the goal queue for display.
func FormatGoalQueue(goals []GoalQueueItem) string {
	if len(goals) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Goals:\n")
	for _, g := range goals {
		icon := "○"
		switch g.Status {
		case "active":
			icon = "●"
		case "paused":
			icon = "◐"
		case "blocked":
			icon = "⊘"
		case "complete":
			icon = "✓"
		}
		sb.WriteString(fmt.Sprintf("  %s %s", icon, g.Objective))
		if g.Progress != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", g.Progress))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── Gap #53: Terminal Notification ──

// Notification represents a terminal notification.
type Notification struct {
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Level   string    `json:"level"` // "info", "warning", "error", "success"
	Created time.Time `json:"created"`
}

// NotificationManager manages terminal notifications.
type NotificationManager struct {
	mu            sync.RWMutex
	notifications []Notification
	maxQueue      int
}

// NewNotificationManager creates a notification manager.
func NewNotificationManager(maxQueue int) *NotificationManager {
	if maxQueue <= 0 {
		maxQueue = 50
	}
	return &NotificationManager{maxQueue: maxQueue}
}

// Push adds a notification.
func (m *NotificationManager) Push(n Notification) {
	n.Created = time.Now()
	m.mu.Lock()
	if len(m.notifications) >= m.maxQueue {
		m.notifications = m.notifications[1:]
	}
	m.notifications = append(m.notifications, n)
	m.mu.Unlock()
}

// Latest returns the most recent notification.
func (m *NotificationManager) Latest() *Notification {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.notifications) == 0 {
		return nil
	}
	n := m.notifications[len(m.notifications)-1]
	return &n
}

// Clear removes all notifications.
func (m *NotificationManager) Clear() {
	m.mu.Lock()
	m.notifications = nil
	m.mu.Unlock()
}

// ── Gap #54: Message Replay ──

// ReplayEntry stores a message for replay.
type ReplayEntry struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	ToolName  string    `json:"tool_name,omitempty"`
}

// MessageReplayStore stores messages for replay.
type MessageReplayStore struct {
	mu       sync.RWMutex
	messages []ReplayEntry
	maxSize  int
}

// NewMessageReplayStore creates a replay store.
func NewMessageReplayStore(maxSize int) *MessageReplayStore {
	if maxSize <= 0 {
		maxSize = 500
	}
	return &MessageReplayStore{maxSize: maxSize}
}

// Add adds a message to the replay store.
func (s *MessageReplayStore) Add(entry ReplayEntry) {
	entry.Timestamp = time.Now()
	s.mu.Lock()
	if len(s.messages) >= s.maxSize {
		s.messages = s.messages[1:]
	}
	s.messages = append(s.messages, entry)
	s.mu.Unlock()
}

// Range returns messages in a range.
func (s *MessageReplayStore) Range(start, end int) []ReplayEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if start < 0 {
		start = 0
	}
	if end > len(s.messages) || end <= 0 {
		end = len(s.messages)
	}
	if start >= end {
		return nil
	}
	result := make([]ReplayEntry, end-start)
	copy(result, s.messages[start:end])
	return result
}

// Count returns the number of stored messages.
func (s *MessageReplayStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages)
}

// ── Gap #55: Paging System ──

// Pager provides pagination over a list of items.
type Pager struct {
	items    []string
	pageSize int
	current  int
}

// NewPager creates a pager.
func NewPager(items []string, pageSize int) *Pager {
	if pageSize <= 0 {
		pageSize = 20
	}
	return &Pager{items: items, pageSize: pageSize}
}

// CurrentPage returns the current page of items.
func (p *Pager) CurrentPage() []string {
	start := p.current * p.pageSize
	end := start + p.pageSize
	if end > len(p.items) {
		end = len(p.items)
	}
	if start >= len(p.items) {
		return nil
	}
	return p.items[start:end]
}

// TotalPages returns the total number of pages.
func (p *Pager) TotalPages() int {
	n := len(p.items) / p.pageSize
	if len(p.items)%p.pageSize > 0 {
		n++
	}
	return n
}

// NextPage advances to the next page.
func (p *Pager) NextPage() bool {
	if p.current < p.TotalPages()-1 {
		p.current++
		return true
	}
	return false
}

// PrevPage goes to the previous page.
func (p *Pager) PrevPage() bool {
	if p.current > 0 {
		p.current--
		return true
	}
	return false
}

// Page returns the current page number (0-based).
func (p *Pager) Page() int { return p.current }

// ── Gap #56: Render Cache ──

// RenderCache caches rendered output to avoid re-rendering.
type RenderCache struct {
	mu    sync.RWMutex
	cache map[string]string
	maxSize int
}

// NewRenderCache creates a render cache.
func NewRenderCache(maxSize int) *RenderCache {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &RenderCache{cache: make(map[string]string), maxSize: maxSize}
}

// Get returns cached output.
func (c *RenderCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.cache[key]
	return v, ok
}

// Set caches rendered output.
func (c *RenderCache) Set(key, value string) {
	c.mu.Lock()
	if len(c.cache) >= c.maxSize {
		// Evict oldest (simple FIFO)
		for k := range c.cache {
			delete(c.cache, k)
			break
		}
	}
	c.cache[key] = value
	c.mu.Unlock()
}

// Clear clears the cache.
func (c *RenderCache) Clear() {
	c.mu.Lock()
	c.cache = make(map[string]string)
	c.mu.Unlock()
}

// ── Gap #57: Tmux Keyboard Handling ──

// TmuxKeyTranslator translates tmux key sequences to standard key names.
type TmuxKeyTranslator struct {
	enabled bool
	mapping map[string]string
}

// NewTmuxKeyTranslator creates a tmux key translator.
func NewTmuxKeyTranslator() *TmuxKeyTranslator {
	return &TmuxKeyTranslator{
		mapping: map[string]string{
			"\x1b[1;5A": "ctrl+up",
			"\x1b[1;5B": "ctrl+down",
			"\x1b[1;5C": "ctrl+right",
			"\x1b[1;5D": "ctrl+left",
			"\x1b[5~":   "pgup",
			"\x1b[6~":   "pgdown",
			"\x1b[H":    "home",
			"\x1b[F":    "end",
			"\x1b[3~":   "delete",
		},
	}
}

// Translate translates a raw key sequence.
func (t *TmuxKeyTranslator) Translate(raw string) string {
	if !t.enabled {
		return raw
	}
	if mapped, ok := t.mapping[raw]; ok {
		return mapped
	}
	return raw
}

// SetEnabled enables/disables tmux translation.
func (t *TmuxKeyTranslator) SetEnabled(enabled bool) {
	t.enabled = enabled
}

// ── Gap #58: Foreground Task Management ──

// ForegroundTask represents a task running in the foreground (blocking the TUI).
type ForegroundTask struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	Status    string    `json:"status"` // "running", "completed", "failed"
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	ExitCode  int       `json:"exit_code,omitempty"`
}

// ForegroundTaskManager tracks foreground tasks.
type ForegroundTaskManager struct {
	mu      sync.RWMutex
	current *ForegroundTask
	history []ForegroundTask
}

// NewForegroundTaskManager creates a foreground task manager.
func NewForegroundTaskManager() *ForegroundTaskManager {
	return &ForegroundTaskManager{}
}

// Start begins a foreground task.
func (m *ForegroundTaskManager) Start(id, command string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = &ForegroundTask{
		ID:        id,
		Command:   command,
		Status:    "running",
		StartedAt: time.Now(),
	}
}

// Complete marks the current task as done.
func (m *ForegroundTaskManager) Complete(exitCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		m.current.Status = "completed"
		m.current.EndedAt = time.Now()
		m.current.ExitCode = exitCode
		if exitCode != 0 {
			m.current.Status = "failed"
		}
		m.history = append(m.history, *m.current)
		m.current = nil
	}
}

// Current returns the current foreground task.
func (m *ForegroundTaskManager) Current() *ForegroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return nil
	}
	copy := *m.current
	return &copy
}

// History returns past foreground tasks.
func (m *ForegroundTaskManager) History() []ForegroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ForegroundTask, len(m.history))
	copy(result, m.history)
	return result
}

// IsBusy reports whether a foreground task is running.
func (m *ForegroundTaskManager) IsBusy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current != nil
}
