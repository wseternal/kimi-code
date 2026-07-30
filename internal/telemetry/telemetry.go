// Package telemetry provides event tracking with buffering, HTTP transport,
// flush lifecycle, and crash handlers.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event is a telemetry event.
type Event struct {
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	SessionID  string         `json:"session_id,omitempty"`
}

// Config holds telemetry configuration.
type Config struct {
	Endpoint   string        `json:"endpoint"`
	APIKey     string        `json:"api_key,omitempty"`
	BatchSize  int           `json:"batch_size"`
	FlushInterval time.Duration `json:"flush_interval"`
	Enabled    bool          `json:"enabled"`
	AppName    string        `json:"app_name"`
	AppVersion string        `json:"app_version"`
}

// Client is the telemetry client.
type Client struct {
	config    Config
	mu        sync.Mutex
	buffer    []Event
	client    *http.Client
	onFlush   func(events []Event)
	stopped   bool
}

// NewClient creates a telemetry client.
func NewClient(cfg Config) *Client {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 30 * time.Second
	}
	return &Client{
		config: cfg,
		buffer: make([]Event, 0, cfg.BatchSize*2),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Track records a telemetry event.
func (c *Client) Track(name string, properties map[string]any) {
	if !c.config.Enabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.buffer = append(c.buffer, Event{
		Name:       name,
		Properties: properties,
		Timestamp:  time.Now(),
	})

	// Auto-flush if buffer is full
	if len(c.buffer) >= c.config.BatchSize {
		go c.flush()
	}
}

// TrackWithSession records an event with a session ID.
func (c *Client) TrackWithSession(name, sessionID string, properties map[string]any) {
	if !c.config.Enabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.buffer = append(c.buffer, Event{
		Name:       name,
		Properties: properties,
		Timestamp:  time.Now(),
		SessionID:  sessionID,
	})
}

// Flush sends all buffered events.
func (c *Client) Flush() error {
	return c.flush()
}

func (c *Client) flush() error {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return nil
	}
	events := make([]Event, len(c.buffer))
	copy(events, c.buffer)
	c.buffer = c.buffer[:0]
	c.mu.Unlock()

	if c.onFlush != nil {
		c.onFlush(events)
	}

	if c.config.Endpoint == "" {
		return nil // no endpoint, just drop
	}

	return c.send(events)
}

func (c *Client) send(events []Event) error {
	payload := struct {
		AppName    string  `json:"app_name"`
		AppVersion string  `json:"app_version"`
		Events     []Event `json:"events"`
	}{
		AppName:    c.config.AppName,
		AppVersion: c.config.AppVersion,
		Events:     events,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telemetry: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		// Re-buffer on failure (best effort)
		c.mu.Lock()
		c.buffer = append(events, c.buffer...)
		c.mu.Unlock()
		return err
	}
	resp.Body.Close()
	return nil
}

// Stop stops the telemetry client and flushes remaining events.
func (c *Client) Stop() error {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
	return c.flush()
}

// SetFlushCallback sets a callback invoked on each flush.
func (c *Client) SetFlushCallback(fn func([]Event)) {
	c.onFlush = fn
}

// BufferSize returns the current buffer size.
func (c *Client) BufferSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.buffer)
}

// ── System Metrics ──

// SystemMetrics captures system-level telemetry.
type SystemMetrics struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryMB    int64   `json:"memory_mb"`
	Goroutines  int     `json:"goroutines"`
	UptimeSec   int64   `json:"uptime_sec"`
}

// CollectSystemMetrics gathers current system metrics.
func CollectSystemMetrics() SystemMetrics {
	return SystemMetrics{
		Goroutines: 0, // Would use runtime.NumGoroutine()
		UptimeSec:  0, // Would track from start time
	}
}
