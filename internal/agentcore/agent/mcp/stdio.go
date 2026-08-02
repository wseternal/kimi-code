package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// StdioClient connects to an MCP server via stdio (child process).
type StdioClient struct {
	command string
	args    []string
	env     []string
	logger  *slog.Logger

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr strings.Builder

	mu     sync.Mutex
	nextID atomic.Int64

	startupTimeout time.Duration
	toolTimeout    time.Duration

	serverInfo *ServerInfo
}

// StdioOption configures a StdioClient.
type StdioOption func(*StdioClient)

// WithStdioEnv sets environment variables for the child process.
func WithStdioEnv(env map[string]string) StdioOption {
	return func(c *StdioClient) {
		for k, v := range env {
			c.env = append(c.env, fmt.Sprintf("%s=%s", k, v))
		}
	}
}

// WithStdioLogger sets the logger.
func WithStdioLogger(logger *slog.Logger) StdioOption {
	return func(c *StdioClient) { c.logger = logger }
}

// WithStartupTimeout sets the initialization timeout.
func WithStartupTimeout(d time.Duration) StdioOption {
	return func(c *StdioClient) { c.startupTimeout = d }
}

// WithToolTimeout sets the per-tool-call timeout.
func WithToolTimeout(d time.Duration) StdioOption {
	return func(c *StdioClient) { c.toolTimeout = d }
}

// NewStdioClient creates a new stdio MCP client.
func NewStdioClient(command string, args []string, opts ...StdioOption) *StdioClient {
	c := &StdioClient{
		command:        command,
		args:           args,
		logger:         slog.Default(),
		startupTimeout: 30 * time.Second,
		toolTimeout:    60 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Initialize spawns the child process and performs the MCP handshake.
func (c *StdioClient) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build environment: inherit current env + overrides
	env := os.Environ()
	env = append(env, c.env...)

	c.cmd = exec.CommandContext(ctx, c.command, c.args...)
	c.cmd.Env = env

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp stdio: stdin pipe: %w", err)
	}

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp stdio: stdout pipe: %w", err)
	}
	c.stdout = bufio.NewReader(stdoutPipe)

	c.cmd.Stderr = &c.stderr

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("mcp stdio: start %s: %w", c.command, err)
	}

	// Perform MCP initialize handshake
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
		c.cleanup()
		return fmt.Errorf("mcp stdio: initialize: %w", err)
	}

	// Parse server info from response
	var initResult struct {
		ServerInfo ServerInfo `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp, &initResult); err != nil {
		c.logger.Warn("mcp: could not parse server info", "error", err)
	} else {
		c.serverInfo = &initResult.ServerInfo
	}

	// Send initialized notification (no response expected)
	if err := c.sendNotification("notifications/initialized", nil); err != nil {
		c.logger.Warn("mcp: failed to send initialized notification", "error", err)
	}

	c.logger.Info("mcp: initialized",
		"server", c.serverInfo.Name,
		"version", c.serverInfo.Version,
		"command", c.command,
	)

	return nil
}

// ListTools returns all tools from the server.
func (c *StdioClient) ListTools(ctx context.Context) ([]ToolDefinition, error) {
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
func (c *StdioClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
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

// Ping sends a ping to the server.
func (c *StdioClient) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.call(ctx, "ping", nil)
	return err
}

// Close terminates the child process.
func (c *StdioClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cleanup()
}

// cleanup terminates the process. Must be called with mu held.
func (c *StdioClient) cleanup() error {
	if c.stdin != nil {
		c.stdin.Close()
		c.stdin = nil
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
		c.cmd = nil
	}
	return nil
}

// ServerInfo returns the server metadata from initialization.
func (c *StdioClient) ServerInfo() *ServerInfo {
	return c.serverInfo
}

// call sends a JSON-RPC request and waits for the response.
func (c *StdioClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callLocked(ctx, method, params)
}

// callLocked sends a JSON-RPC request with the lock already held.
func (c *StdioClient) callLocked(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.sendRequest(req); err != nil {
		return nil, err
	}

	return c.readResponse(ctx, id)
}

// sendRequest writes a JSON-RPC request to stdin.
func (c *StdioClient) sendRequest(req jsonRPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mcp: marshal request: %w", err)
	}
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("mcp: write to stdin: %w", err)
	}
	return nil
}

// sendNotification writes a JSON-RPC notification (no ID, no response).
func (c *StdioClient) sendNotification(method string, params interface{}) error {
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
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("mcp: write notification: %w", err)
	}
	return nil
}

// readResponse reads JSON-RPC responses until we get one matching our ID.
func (c *StdioClient) readResponse(ctx context.Context, expectedID int64) (json.RawMessage, error) {
	for {
		// Check context timeout
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("mcp: timeout: %w", ctx.Err())
		default:
		}

		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("mcp: read from stdout: %w", err)
		}

		line = []byte(strings.TrimSpace(string(line)))
		if len(line) == 0 {
			continue
		}

		// Try to parse as response
		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Might be a log message from the server, skip
			c.logger.Debug("mcp: skipping non-JSON line", "line", string(line))
			continue
		}

		// Skip notifications (no ID)
		if resp.ID == 0 {
			c.logger.Debug("mcp: skipping notification", "line", string(line))
			continue
		}

		// Check if this is our response
		if resp.ID != expectedID {
			c.logger.Debug("mcp: skipping response for different ID", "got", resp.ID, "expected", expectedID)
			continue
		}

		if resp.Error != nil {
			return nil, fmt.Errorf("mcp: server error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		return resp.Result, nil
	}
}
