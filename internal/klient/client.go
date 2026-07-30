package klient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/visdomtech/kimi-code/internal/protocol"
	"github.com/visdomtech/kimi-code/internal/protocol/rest"
)

// Client is the HTTP API client for the kimi-code daemon.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// ClientConfig configures the client.
type ClientConfig struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// NewClient creates a new API client.
func NewClient(cfg ClientConfig) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:9876"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{
		baseURL:    cfg.BaseURL,
		httpClient: &http.Client{Timeout: cfg.Timeout},
		token:      cfg.Token,
	}
}

// ── Session Operations ──

// ListSessions returns all sessions.
func (c *Client) ListSessions(ctx context.Context) ([]protocol.Session, error) {
	var result []protocol.Session
	if err := c.get(ctx, "/api/v1/sessions", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateSession creates a new session.
func (c *Client) CreateSession(ctx context.Context, title string) (*protocol.Session, error) {
	var result protocol.Session
	body := map[string]string{"title": title}
	if err := c.post(ctx, "/api/v1/sessions", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetSession returns a session by ID.
func (c *Client) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	var result protocol.Session
	if err := c.get(ctx, "/api/v1/sessions/"+id, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteSession deletes a session.
func (c *Client) DeleteSession(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/sessions/"+id)
}

// GetSessionStatus returns the status of a session.
func (c *Client) GetSessionStatus(ctx context.Context, id string) (*rest.SessionStatusResponse, error) {
	var result rest.SessionStatusResponse
	if err := c.get(ctx, "/api/v1/sessions/"+id+"/status", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SubmitPrompt submits a prompt to a session.
func (c *Client) SubmitPrompt(ctx context.Context, sessionID, text string) (*rest.SubmitPromptResponse, error) {
	var result rest.SubmitPromptResponse
	body := rest.SubmitPromptRequest{Text: text}
	if err := c.post(ctx, "/api/v1/sessions/"+sessionID+"/prompts", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AbortSession aborts a running session.
func (c *Client) AbortSession(ctx context.Context, id string) error {
	return c.post(ctx, "/api/v1/sessions/"+id+"/abort", nil, nil)
}

// CompactSession triggers compaction.
func (c *Client) CompactSession(ctx context.Context, id, instruction string) error {
	body := rest.CompactSessionRequest{Instruction: instruction}
	return c.post(ctx, "/api/v1/sessions/"+id+"/compact", body, nil)
}

// SnapshotSession captures a session snapshot.
func (c *Client) SnapshotSession(ctx context.Context, id string) (*rest.SessionSnapshot, error) {
	var result rest.SessionSnapshot
	if err := c.get(ctx, "/api/v1/sessions/"+id+"/snapshot", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ExportSession exports a session.
func (c *Client) ExportSession(ctx context.Context, id string) (*rest.ExportSessionResponse, error) {
	var result rest.ExportSessionResponse
	if err := c.get(ctx, "/api/v1/sessions/"+id+"/export", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SearchSession performs a code search within a session.
func (c *Client) SearchSession(ctx context.Context, id string, query rest.SearchRequest) (*rest.SearchResponse, error) {
	var result rest.SearchResponse
	if err := c.post(ctx, "/api/v1/sessions/"+id+"/search", query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ── Config Operations ──

// GetConfig returns the current configuration.
func (c *Client) GetConfig(ctx context.Context) (*rest.ConfigResponse, error) {
	var result rest.ConfigResponse
	if err := c.get(ctx, "/api/v1/config", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ── Meta Operations ──

// GetMeta returns server metadata.
func (c *Client) GetMeta(ctx context.Context) (*rest.MetaResponse, error) {
	var result rest.MetaResponse
	if err := c.get(ctx, "/api/v1/meta", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Health checks server health.
func (c *Client) Health(ctx context.Context) error {
	return c.get(ctx, "/api/v1/health", nil)
}

// ── Internal HTTP helpers ──

func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) post(ctx context.Context, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, result)
}

func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) do(req *http.Request, result interface{}) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var env protocol.Envelope[struct{}]
		json.NewDecoder(resp.Body).Decode(&env)
		if env.Msg != "" {
			return fmt.Errorf("API error %d: %s", env.Code, env.Msg)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if result != nil {
		var env protocol.Envelope[json.RawMessage]
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if env.Data != nil {
			return json.Unmarshal(*env.Data, result)
		}
	}
	return nil
}
