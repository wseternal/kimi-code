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
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxResponseBodySize is the cap for fallback response reads (10 MB).
const maxResponseBodySize = 10 * 1024 * 1024

// DefaultHTTPClientVersion is the version sent in MCP clientInfo.
const DefaultHTTPClientVersion = "0.3.0"

// HTTPClient connects to an MCP server via HTTP/SSE transport.
// Supports both the "http" (Streamable HTTP) and "sse" (legacy SSE) transports
// as defined in the MCP spec (2024-11-05).
type HTTPClient struct {
	url           string
	transportType string // "http" or "sse"
	logger        *slog.Logger
	httpClient    *http.Client
	headers       map[string]string
	clientVersion string

	mu          sync.Mutex
	nextID      atomic.Int64
	initialized bool
	serverInfo  *ServerInfo

	// SSE transport state
	sseEndpoint string // endpoint URL for POST requests (discovered via SSE)
	sseCancel   context.CancelFunc
	sseBody     io.Closer // held so connectSSE can clean up

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

// WithHTTPClientVersion sets the client version sent in MCP clientInfo.
func WithHTTPClientVersion(version string) HTTPOption {
	return func(c *HTTPClient) { c.clientVersion = version }
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
		clientVersion:  DefaultHTTPClientVersion,
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
			"version": c.clientVersion,
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

	// Send initialized notification (best-effort; log but don't block)
	if err := c.sendNotification(initCtx, "notifications/initialized", nil); err != nil {
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
	if c.sseBody != nil {
		c.sseBody.Close()
		c.sseBody = nil
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

	// Fallback: try JSON first, then SSE (bounded read to avoid OOM)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
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
func (c *HTTPClient) sendNotification(ctx context.Context, method string, params interface{}) error {
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(data))
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

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return fmt.Errorf("mcp sse: unexpected status %d", resp.StatusCode)
	}

	c.sseCancel = cancel
	c.sseBody = resp.Body

	// Derive the base URL host for SSRF validation of the discovered endpoint.
	baseURL, err := url.Parse(c.url)
	if err != nil {
		resp.Body.Close()
		cancel()
		return fmt.Errorf("mcp sse: invalid base url: %w", err)
	}

	// Read the initial "endpoint" event to discover the POST URL.
	// Use a bounded scanner to avoid reading forever if the server misbehaves.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

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
				endpoint, err := c.resolveSSEEndpoint(eventData, baseURL)
				if err != nil {
					resp.Body.Close()
					cancel()
					return err
				}
				c.sseEndpoint = endpoint
				c.logger.Debug("mcp sse: discovered endpoint", "endpoint", endpoint)
				// Keep the SSE connection open in background for server notifications.
				// drainSSE will close resp.Body when done.
				go c.drainSSE(scanner, resp.Body)
				return nil
			}
			eventType = ""
			eventData = ""
		}
	}

	if err := scanner.Err(); err != nil {
		resp.Body.Close()
		cancel()
		return fmt.Errorf("mcp sse: read events: %w", err)
	}

	resp.Body.Close()
	cancel()
	return fmt.Errorf("mcp sse: no endpoint event received")
}

// resolveSSEEndpoint resolves a (possibly relative) endpoint URL and validates
// it stays on the same host to prevent SSRF via a malicious MCP server.
func (c *HTTPClient) resolveSSEEndpoint(endpoint string, baseURL *url.URL) (string, error) {
	if !strings.HasPrefix(endpoint, "http") {
		// Resolve relative URL against the base.
		ref, err := url.Parse(endpoint)
		if err != nil {
			return "", fmt.Errorf("mcp sse: invalid endpoint %q: %w", endpoint, err)
		}
		resolved := baseURL.ResolveReference(ref)
		return resolved.String(), nil
	}
	// Absolute URL — must match the same host.
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("mcp sse: invalid endpoint URL %q: %w", endpoint, err)
	}
	if parsed.Host != baseURL.Host {
		return "", fmt.Errorf("mcp sse: endpoint host %q does not match base host %q (SSRF guard)", parsed.Host, baseURL.Host)
	}
	return endpoint, nil
}

// drainSSE reads remaining SSE events (server notifications) in background.
// It closes body when the stream ends to prevent resource leaks.
func (c *HTTPClient) drainSSE(scanner *bufio.Scanner, body io.Closer) {
	defer body.Close()
	for scanner.Scan() {
		// Server notifications — log and discard for now.
		// A full implementation would dispatch these to the agent.
		line := scanner.Text()
		if line != "" {
			c.logger.Debug("mcp sse: notification", "line", line)
		}
	}
	if err := scanner.Err(); err != nil {
		c.logger.Debug("mcp sse: drain ended", "error", err)
	}
}

// parseJSONResponse parses a JSON-RPC response from a reader.
func (c *HTTPClient) parseJSONResponse(r io.Reader, expectedID int64) (json.RawMessage, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxResponseBodySize))
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
// Respects context cancellation to avoid hanging on a stalled server stream.
func (c *HTTPClient) parseSSEResponse(ctx context.Context, r io.Reader, expectedID int64) (json.RawMessage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventData string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("mcp: sse stream cancelled: %w", ctx.Err())
		default:
		}

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
