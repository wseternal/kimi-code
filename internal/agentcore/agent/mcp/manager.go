package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/tools"
	"github.com/visdomtech/kimi-code/internal/agentcore/config"
)

// lookupEnv resolves an environment variable name to its value.
func lookupEnv(name string) string {
	return os.Getenv(name)
}

// ServerStatus represents the connection state of an MCP server.
type ServerStatus string

const (
	StatusDisconnected ServerStatus = "disconnected"
	StatusConnecting   ServerStatus = "connecting"
	StatusConnected    ServerStatus = "connected"
	StatusError        ServerStatus = "error"
)

// ServerState tracks a single MCP server's connection.
type ServerState struct {
	Name   string
	Config config.McpServerConfig
	Client Client
	Status ServerStatus
	Tools  []ToolDefinition
	Error  string
}

// Manager manages the lifecycle of MCP server connections.
type Manager struct {
	mu      sync.RWMutex
	servers map[string]*ServerState
	logger  *slog.Logger
}

// NewManager creates a new MCP connection manager.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		servers: make(map[string]*ServerState),
		logger:  logger,
	}
}

// ConnectAll initializes all MCP servers from config and registers their
// tools into the provided registry. Returns total tools registered.
func (m *Manager) ConnectAll(ctx context.Context, servers map[string]config.McpServerConfig, registry *tools.Registry) (int, error) {
	total := 0
	for name, cfg := range servers {
		n, err := m.Connect(ctx, name, cfg, registry)
		if err != nil {
			m.logger.Error("mcp: failed to connect", "server", name, "error", err)
			// Continue with other servers
			continue
		}
		total += n
	}
	return total, nil
}

// Connect initializes a single MCP server and registers its tools.
func (m *Manager) Connect(ctx context.Context, name string, cfg config.McpServerConfig, registry *tools.Registry) (int, error) {
	m.mu.Lock()
	state := &ServerState{
		Name:   name,
		Config: cfg,
		Status: StatusConnecting,
	}
	m.servers[name] = state
	m.mu.Unlock()

	// Create client based on transport
	client, err := createClient(name, cfg, m.logger)
	if err != nil {
		m.setError(name, err)
		return 0, err
	}

	// Initialize the connection
	if err := client.Initialize(ctx); err != nil {
		m.setError(name, err)
		return 0, fmt.Errorf("mcp %s: initialize: %w", name, err)
	}

	// Discover and register tools
	defs, count, err := RegisterServerTools(ctx, client, name, registry, cfg.EnabledTools)
	if err != nil {
		m.setError(name, err)
		return 0, err
	}

	m.mu.Lock()
	state.Client = client
	state.Status = StatusConnected
	state.Tools = defs
	state.Error = ""
	m.mu.Unlock()

	m.logger.Info("mcp: connected", "server", name, "tools", count)
	return count, nil
}

// Disconnect shuts down a single MCP server.
func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	state, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("mcp: server %q not found", name)
	}
	delete(m.servers, name)
	m.mu.Unlock()

	if state.Client != nil {
		return state.Client.Close()
	}
	return nil
}

// DisconnectAll shuts down all MCP servers.
func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	servers := make(map[string]*ServerState, len(m.servers))
	for k, v := range m.servers {
		servers[k] = v
	}
	m.servers = make(map[string]*ServerState)
	m.mu.Unlock()

	for name, state := range servers {
		if state.Client != nil {
			if err := state.Client.Close(); err != nil {
				m.logger.Warn("mcp: close error", "server", name, "error", err)
			}
		}
	}
}

// Statuses returns the current status of all MCP servers.
func (m *Manager) Statuses() map[string]ServerState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ServerState, len(m.servers))
	for k, v := range m.servers {
		result[k] = *v
	}
	return result
}

// setError marks a server as errored.
func (m *Manager) setError(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.servers[name]; ok {
		state.Status = StatusError
		state.Error = err.Error()
	}
}

// createClient creates an MCP client based on the transport config.
func createClient(name string, cfg config.McpServerConfig, logger *slog.Logger) (Client, error) {
	switch cfg.Transport {
	case "stdio":
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio transport requires a command")
		}
		var opts []StdioOption
		opts = append(opts, WithStdioLogger(logger.With("mcp_server", name)))
		if len(cfg.Env) > 0 {
			opts = append(opts, WithStdioEnv(cfg.Env))
		}
		if cfg.StartupTimeoutMs > 0 {
			opts = append(opts, WithStartupTimeout(time.Duration(cfg.StartupTimeoutMs)*time.Millisecond))
		}
		if cfg.ToolTimeoutMs > 0 {
			opts = append(opts, WithToolTimeout(time.Duration(cfg.ToolTimeoutMs)*time.Millisecond))
		}
		return NewStdioClient(cfg.Command, cfg.Args, opts...), nil

	case "http", "sse":
		if cfg.URL == "" {
			return nil, fmt.Errorf("%s transport requires a url", cfg.Transport)
		}
		var opts []HTTPOption
		opts = append(opts, WithHTTPLogger(logger.With("mcp_server", name)))
		// Resolve bearer token: prefer explicit BearerTokenEnv field,
		// fall back to legacy "BEARER_TOKEN_ENV" key in Env map.
		bearerEnvName := cfg.BearerTokenEnv
		if bearerEnvName == "" {
			if v, ok := cfg.Env["BEARER_TOKEN_ENV"]; ok {
				bearerEnvName = v
			}
		}
		if bearerEnvName != "" {
			if tok := lookupEnv(bearerEnvName); tok != "" {
				opts = append(opts, WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + tok}))
			}
		}
		if cfg.StartupTimeoutMs > 0 {
			opts = append(opts, WithHTTPStartupTimeout(time.Duration(cfg.StartupTimeoutMs)*time.Millisecond))
		}
		if cfg.ToolTimeoutMs > 0 {
			opts = append(opts, WithHTTPToolTimeout(time.Duration(cfg.ToolTimeoutMs)*time.Millisecond))
		}
		return NewHTTPClient(cfg.URL, cfg.Transport, opts...), nil

	default:
		return nil, fmt.Errorf("unknown transport %q (supported: stdio, http, sse)", cfg.Transport)
	}
}
