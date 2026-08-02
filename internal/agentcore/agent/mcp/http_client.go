package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HTTPClient connects to an MCP server via HTTP/SSE transport.
// Supports both the "http" (Streamable HTTP) and "sse" (legacy SSE) transports
// as defined in the MCP spec (2024-11-05).
type HTTPClient struct {
	url           string
	transportType string // "http" or "sse"
	logger        *slog.Logger
	httpClient    *http.Client
	headers       map[string]string

	mu          sync.Mutex
	nextID      atomic.Int64
	initialized bool
	serverInfo  *ServerInfo

	// SSE transport state
	sseEndpoint string // endpoint URL for POST requests (discovered via SSE)
	sseCancel   context.CancelFunc

	toolTimeout    time.Duration
	startupTimeout time.Duration
}

// HTTPOption configures an HTTPClient.
type HTTPOption func(*HTTPClient)

// WithHTTPHeaders sets custom HTTP headers (e.g. auth tokens).
func WithHTTPHeaders(headers map[string]string) HTTPOption {
	return func(c *HTTPClient) {
		for k, v := range headers {
			c.headers[k] = v
		}
	}
}

// WithHTTPLogger sets the logger.
func WithHTTPLogger(logger *slog.Logger) HTTPOption {
	return func(c *HTTPClient) { c.logger = logger }
}

// WithHTTPStartupTimeout sets the initialization timeout.
func WithHTTPStartupTimeout(d time.Duration) HTTPOption {
	return func(c *HTTPClient) { c.startupTimeout = d }
}

// WithHTTPToolTimeout sets the per-tool-call timeout.
func WithHTTPToolTimeout(d time.Duration) HTTPOption {
	return func(c *HTTPClient) { c.toolTimeout = d }
}

// NewHTTPClient creates a new HTTP/SSE MCP client.
func NewHTTPClient(url string, transportType string, opts ...HTTPOption) *HTTPClient {
	if transportType != "http" && transportType != "sse" {
		transportType = "http"
	}
	c := &HTTPClient{
		url:            url,
		transportType:  transportType,
		logger:         slog.Default(),
		httpClient:     &http.Client{Timeout: 120 * time.Second},
		headers:        make(map[string]string),
		toolTimeout:    60 * time.Second,
		startupTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Initialize performs the MCP handshake with the server.
func (c *HTTPClient) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.transportType == "sse" {
		if err := c.connectSSE(ctx); err != nil {
			return err
		}
	}

	initCtx, cancel := context.WithTimeout(ctx, c.startupTimeout)
	defer cancel()

	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "kimi-code",
			"version": "0.1.0",
		},
	}

	resp, err := c.callLocked(initCtx, "initialize", initParams)
	if err != nil {
		return fmt.Errorf("mcp http: initialize: %w", err)
	}

	var initResult struct {
		ServerInfo ServerInfo `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp, &initResult); err != nil {
		c.logger.Warn("mcp: could not parse server info", "error", err)
	} else {
		c.serverInfo = &initResult.ServerInfo
	}

	// Send initialized notification
	if err := c.sendNotification("notifications/initialized", nil); err != nil {
		c.logger.Warn("mcp: failed to send initialized notification", "error", err)
	}

	c.initialized = true

	if c.serverInfo != nil {
		c.logger.Info("mcp: initialized",
			"server", c.serverInfo.Name,
			"version", c.serverInfo.Version,
			"url", c.url,
			"transport", c.transportType,
		)
	}

	return nil
}

// ListTools returns all tools from the server.
func (c *HTTPClient) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	ctx, cancel := context.WithTimeout(ctx, c.toolTimeout)
	defer cancel()

	resp, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/list: %w", err)
	}

	var result struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/list: %w", err)
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the server.
func (c *HTTPClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.toolTimeout)
	defer cancel()

	params := map[string]any{
		"name":      name,
		"arguments": args,
	}

	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/call %s: %w", name, err)
	}

	var result ToolResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/call result: %w", err)
	}
	return &result, nil
}

// Ping checks if the server is alive.
func (c *HTTPClient) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.call(ctx, "ping", nil)
	return err
}

// Close shuts down the connection.
func (c *HTTPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sseCancel != nil {
		c.sseCancel()
		c.sseCancel = nil
	}
	c.initialized = false
	return nil
}

// call sends a JSON-RPC request and waits for the response.
func (c *HTTPClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callLocked(ctx, method, params)
}

// callLocked sends a JSON-RPC request (lock must be held).
func (c *HTTPClient) callLocked(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}

	targetURL := c.url
	if c.transportType == "sse" && c.sseEndpoint != "" {
		targetURL = c.sseEndpoint
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("mcp: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("mcp: http %d: %s", resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")

	// JSON response
	if strings.Contains(contentType, "application/json") {
		return c.parseJSONResponse(resp.Body, id)
	}

	// SSE response (Streamable HTTP may return SSE)
	if strings.Contains(contentType, "text/event-stream") {
		return c.parseSSEResponse(ctx, resp.Body, id)
	}

	// Fallback: try JSON first, then SSE
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mcp: read response: %w", err)
	}

	var jsonResp jsonRPCResponse
	if err := json.Unmarshal(body, &jsonResp); err == nil && jsonResp.ID == id {
		if jsonResp.Error != nil {
			return nil, fmt.Errorf("mcp: server error %d: %s", jsonResp.Error.Code, jsonResp.Error.Message)
		}
		return jsonResp.Result, nil
	}

	// Try parsing as SSE
	reader := bufio.NewReader(bytes.NewReader(body))
	return c.parseSSEResponse(ctx, reader, id)
}

// sendNotification sends a JSON-RPC notification (no ID, no response expected).
func (c *HTTPClient) sendNotification(method string, params interface{}) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp: marshal notification: %w", err)
	}

	targetURL := c.url
	if c.transportType == "sse" && c.sseEndpoint != "" {
		targetURL = c.sseEndpoint
	}

	httpReq, err := http.NewRequest("POST", targetURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("mcp: create notification request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: notification request: %w", err)
	}
	resp.Body.Close()
	return nil
}

// connectSSE establishes an SSE connection and discovers the POST endpoint.
func (c *HTTPClient) connectSSE(ctx context.Context) error {
	sseCtx, cancel := context.WithCancel(ctx)

	httpReq, err := http.NewRequestWithContext(sseCtx, "GET", c.url, nil)
	if err != nil {
		cancel()
		return fmt.Errorf("mcp sse: create request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		cancel()
		return fmt.Errorf("mcp sse: connect: %w", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		cancel()
		return fmt.Errorf("mcp sse: unexpected status %d", resp.StatusCode)
	}

	c.sseCancel = cancel

	// Read the initial "endpoint" event to discover the POST URL
	scanner := bufio.NewScanner(resp.Body)
	var eventType, eventData string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			eventData = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		} else if line == "" {
			// End of event
			if eventType == "endpoint" && eventData != "" {
				// The endpoint may be relative — resolve against base URL
				endpoint := eventData
				if !strings.HasPrefix(endpoint, "http") {
					// Resolve relative URL
					baseURL := c.url
					if idx := strings.LastIndex(baseURL, "/"); idx >= 0 {
						endpoint = baseURL[:idx+1] + strings.TrimPrefix(endpoint, "/")
					}
				}
				c.sseEndpoint = endpoint
				c.logger.Debug("mcp sse: discovered endpoint", "endpoint", endpoint)
				// Keep the SSE connection open in background for server notifications
				go c.drainSSE(scanner)
				return nil
			}
			eventType = ""
			eventData = ""
		}
	}

	if err := scanner.Err(); err != nil {
		cancel()
		return fmt.Errorf("mcp sse: read events: %w", err)
	}

	cancel()
	return fmt.Errorf("mcp sse: no endpoint event received")
}

// drainSSE reads remaining SSE events (server notifications) in background.
func (c *HTTPClient) drainSSE(scanner *bufio.Scanner) {
	for scanner.Scan() {
		// Server notifications — log and discard for now.
		// A full implementation would dispatch these to the agent.
		line := scanner.Text()
		if line != "" {
			c.logger.Debug("mcp sse: notification", "line", line)
		}
	}
}

// parseJSONResponse parses a JSON-RPC response from a reader.
func (c *HTTPClient) parseJSONResponse(r io.Reader, expectedID int64) (json.RawMessage, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("mcp: read json response: %w", err)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("mcp: parse json response: %w", err)
	}

	if resp.ID != expectedID {
		return nil, fmt.Errorf("mcp: response ID mismatch: got %d, want %d", resp.ID, expectedID)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcp: server error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// parseSSEResponse reads SSE events until we find the response for our request ID.
func (c *HTTPClient) parseSSEResponse(_ context.Context, r io.Reader, expectedID int64) (json.RawMessage, error) {
	scanner := bufio.NewScanner(r)
	var eventData string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data:") {
			eventData = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		} else if line == "" && eventData != "" {
			// End of event — parse the data
			var resp jsonRPCResponse
			if err := json.Unmarshal([]byte(eventData), &resp); err == nil {
				if resp.ID == expectedID {
					if resp.Error != nil {
						return nil, fmt.Errorf("mcp: server error %d: %s", resp.Error.Code, resp.Error.Message)
					}
					return resp.Result, nil
				}
			}
			eventData = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("mcp: sse read error: %w", err)
	}

	return nil, fmt.Errorf("mcp: sse stream ended without response for ID %d", expectedID)
}
