// Package oauth provides managed usage tracking and feedback (Gaps #89).
package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// UsageRecord represents a usage data point.
type UsageRecord struct {
	ProviderID string    `json:"provider_id"`
	ModelID    string    `json:"model_id"`
	TokensIn   int       `json:"tokens_in"`
	TokensOut  int       `json:"tokens_out"`
	Timestamp  time.Time `json:"timestamp"`
	SessionID  string    `json:"session_id,omitempty"`
}

// UsageSummary holds aggregated usage statistics.
type UsageSummary struct {
	TotalTokensIn  int     `json:"total_tokens_in"`
	TotalTokensOut int     `json:"total_tokens_out"`
	TotalCost      float64 `json:"total_cost"`
	RequestCount   int     `json:"request_count"`
}

// UsageTracker tracks usage data and reports to the managed platform.
type UsageTracker struct {
	mu       sync.Mutex
	endpoint string
	token    string
	buffer   []UsageRecord
	client   *http.Client
}

// NewUsageTracker creates a usage tracker.
func NewUsageTracker(endpoint, token string) *UsageTracker {
	return &UsageTracker{
		endpoint: endpoint,
		token:    token,
		buffer:   make([]UsageRecord, 0, 100),
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Record adds a usage record.
func (t *UsageTracker) Record(record UsageRecord) {
	record.Timestamp = time.Now()
	t.mu.Lock()
	t.buffer = append(t.buffer, record)
	t.mu.Unlock()
}

// Flush sends buffered usage data to the managed platform.
func (t *UsageTracker) Flush(ctx context.Context) error {
	t.mu.Lock()
	if len(t.buffer) == 0 {
		t.mu.Unlock()
		return nil
	}
	records := make([]UsageRecord, len(t.buffer))
	copy(records, t.buffer)
	t.buffer = t.buffer[:0]
	t.mu.Unlock()

	if t.endpoint == "" {
		return nil
	}

	payload := struct {
		Records []UsageRecord `json:"records"`
	}{Records: records}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal usage: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.endpoint+"/api/usage", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		// Re-buffer on failure
		t.mu.Lock()
		t.buffer = append(records, t.buffer...)
		t.mu.Unlock()
		return err
	}
	resp.Body.Close()
	return nil
}

// Summary returns aggregated usage summary from buffer.
func (t *UsageTracker) Summary() UsageSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	var s UsageSummary
	s.RequestCount = len(t.buffer)
	for _, r := range t.buffer {
		s.TotalTokensIn += r.TokensIn
		s.TotalTokensOut += r.TokensOut
	}
	return s
}

// BufferSize returns the current buffer size.
func (t *UsageTracker) BufferSize() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.buffer)
}

// ── Feedback (Gap #89) ──

// FeedbackEntry represents user feedback on a response.
type FeedbackEntry struct {
	SessionID string    `json:"session_id"`
	MessageID string    `json:"message_id"`
	Rating    int       `json:"rating"` // -1, 0, 1
	Comment   string    `json:"comment,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// FeedbackClient submits feedback to the managed platform.
type FeedbackClient struct {
	endpoint string
	token    string
	client   *http.Client
}

// NewFeedbackClient creates a feedback client.
func NewFeedbackClient(endpoint, token string) *FeedbackClient {
	return &FeedbackClient{
		endpoint: endpoint,
		token:    token,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Submit sends feedback to the managed platform.
func (c *FeedbackClient) Submit(ctx context.Context, entry FeedbackEntry) error {
	entry.Timestamp = time.Now()

	if c.endpoint == "" {
		return nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal feedback: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/feedback", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("feedback submit failed: %d", resp.StatusCode)
	}
	return nil
}
