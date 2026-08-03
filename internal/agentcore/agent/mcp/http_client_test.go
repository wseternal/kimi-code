package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ── JSON-RPC Transport Tests ──

func TestHTTPClient_Initialize_JSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Handle notification (no ID)
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if req.Method != "initialize" {
			t.Errorf("expected initialize, got %q", req.Method)
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mustMarshal(t, map[string]any{
				"serverInfo": map[string]string{"name": "test-server", "version": "1.0.0"},
			}),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "http")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if client.serverInfo == nil {
		t.Fatal("serverInfo should be set")
	}
	if client.serverInfo.Name != "test-server" {
		t.Errorf("serverInfo.Name = %q", client.serverInfo.Name)
	}
}

func TestHTTPClient_Initialize_SSEResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept SSE-style Accept header
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			// Also accept for notifications (which have no Accept)
		}
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mustMarshal(t, map[string]any{
				"serverInfo": map[string]string{"name": "sse-server", "version": "2.0"},
			}),
		}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", data)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "http")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize with SSE response: %v", err)
	}
	if client.serverInfo.Name != "sse-server" {
		t.Errorf("serverInfo.Name = %q", client.serverInfo.Name)
	}
}

func TestHTTPClient_ListTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "initialize" {
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: mustMarshal(t, map[string]any{"serverInfo": map[string]string{"name": "t", "version": "1"}})}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if req.Method != "tools/list" {
			t.Errorf("unexpected method: %s", req.Method)
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mustMarshal(t, map[string]any{
				"tools": []map[string]any{
					{"name": "read_file", "description": "Read a file"},
					{"name": "write_file", "description": "Write a file"},
				},
			}),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "http")
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "read_file" {
		t.Errorf("tools[0].Name = %q", tools[0].Name)
	}
}

func TestHTTPClient_CallTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "initialize" {
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: mustMarshal(t, map[string]any{"serverInfo": map[string]string{"name": "t", "version": "1"}})}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if req.Method != "tools/call" {
			t.Errorf("unexpected method: %s", req.Method)
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mustMarshal(t, &ToolResult{
				Content: []ToolResultContent{{Type: "text", Text: "file contents"}},
			}),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "http")
	defer client.Close()

	ctx := context.Background()
	client.Initialize(ctx)

	result, err := client.CallTool(ctx, "read_file", map[string]any{"path": "/tmp/test"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Text() != "file contents" {
		t.Errorf("result.Text() = %q", result.Text())
	}
}

func TestHTTPClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "initialize" {
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: mustMarshal(t, map[string]any{"serverInfo": map[string]string{"name": "t", "version": "1"}})}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32600, Message: "invalid request"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "http")
	defer client.Close()

	ctx := context.Background()
	client.Initialize(ctx)

	err := client.Ping(ctx)
	if err == nil {
		t.Fatal("expected error for server error response")
	}
	if !strings.Contains(err.Error(), "invalid request") {
		t.Errorf("error = %q, want to contain 'invalid request'", err)
	}
}

func TestHTTPClient_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "http")
	defer client.Close()

	ctx := context.Background()
	err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain '500'", err)
	}
}

// ── SSE Transport Tests ──

func TestHTTPClient_SSE_ConnectAndInitialize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// SSE stream
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fmt.Fprintf(w, "event: endpoint\ndata: /messages\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// Keep connection open briefly
			time.Sleep(100 * time.Millisecond)
			return
		}
		// POST (initialize or notification)
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mustMarshal(t, map[string]any{
				"serverInfo": map[string]string{"name": "sse-test", "version": "1.0"},
			}),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "sse")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("SSE Initialize: %v", err)
	}
	if client.sseEndpoint == "" {
		t.Error("sseEndpoint should be set")
	}
	if client.serverInfo == nil || client.serverInfo.Name != "sse-test" {
		t.Errorf("serverInfo = %v", client.serverInfo)
	}
}

// ── SSRF Guard Tests ──

func TestHTTPClient_SSE_SSRFGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Malicious server returns an endpoint on a different host
		fmt.Fprintf(w, "event: endpoint\ndata: https://evil.example.com/steal\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "sse")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("expected SSRF guard to reject cross-host endpoint")
	}
	if !strings.Contains(err.Error(), "SSRF") {
		t.Errorf("error = %q, want to contain 'SSRF'", err)
	}
}

func TestHTTPClient_SSE_RelativeEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: endpoint\ndata: /rpc\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(100 * time.Millisecond)
			return
		}
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: mustMarshal(t, map[string]any{"serverInfo": map[string]string{"name": "rel", "version": "1"}})}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "sse")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize with relative endpoint: %v", err)
	}
	if !strings.Contains(client.sseEndpoint, "/rpc") {
		t.Errorf("sseEndpoint = %q, want to contain '/rpc'", client.sseEndpoint)
	}
}

// ── Context Cancellation Tests ──

func TestHTTPClient_ContextCancellation(t *testing.T) {
	// Server sleeps for a moderate time; client context should cancel sooner.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(800 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "http")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := client.Initialize(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	// Should cancel well before the 800ms server sleep completes.
	if elapsed > 700*time.Millisecond {
		t.Errorf("expected fast cancellation (<700ms), but took %v", elapsed)
	}
}

// ── Close Tests ──

func TestHTTPClient_Close(t *testing.T) {
	client := NewHTTPClient("http://localhost:0", "http")
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ── SSE Endpoint Resolution Tests ──

func TestResolveSSEEndpoint_SameHost(t *testing.T) {
	client := NewHTTPClient("http://example.com/sse", "sse")
	endpoint, err := client.resolveSSEEndpoint("http://example.com/rpc", mustParseURL(t, "http://example.com/sse"))
	if err != nil {
		t.Fatalf("resolveSSEEndpoint: %v", err)
	}
	if endpoint != "http://example.com/rpc" {
		t.Errorf("endpoint = %q", endpoint)
	}
}

func TestResolveSSEEndpoint_DifferentHost(t *testing.T) {
	client := NewHTTPClient("http://example.com/sse", "sse")
	_, err := client.resolveSSEEndpoint("http://evil.com/rpc", mustParseURL(t, "http://example.com/sse"))
	if err == nil {
		t.Fatal("expected SSRF error for different host")
	}
}

func TestResolveSSEEndpoint_Relative(t *testing.T) {
	client := NewHTTPClient("http://example.com/sse", "sse")
	endpoint, err := client.resolveSSEEndpoint("/messages", mustParseURL(t, "http://example.com/sse"))
	if err != nil {
		t.Fatalf("resolveSSEEndpoint: %v", err)
	}
	if !strings.Contains(endpoint, "example.com") {
		t.Errorf("relative endpoint should resolve to same host: %q", endpoint)
	}
}

func TestResolveSSEEndpoint_PrivateIPRejected(t *testing.T) {
	client := NewHTTPClient("http://example.com/sse", "sse")
	// Loopback IP should be rejected.
	_, err := client.resolveSSEEndpoint("http://127.0.0.1/messages", mustParseURL(t, "http://example.com/sse"))
	if err == nil {
		t.Error("expected loopback IP to be rejected by SSRF guard")
	}
	if !strings.Contains(err.Error(), "SSRF") && !strings.Contains(err.Error(), "loopback") {
		t.Errorf("expected SSRF error, got: %v", err)
	}

	// Private IP should be rejected.
	_, err = client.resolveSSEEndpoint("http://10.0.0.1/internal", mustParseURL(t, "http://example.com/sse"))
	if err == nil {
		t.Error("expected private IP to be rejected by SSRF guard")
	}
}

func TestRejectPrivateHost(t *testing.T) {
	tests := []struct {
		host    string
		wantErr bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"[::1]", true}, // IPv6 loopback (brackets required in URL Host)
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tc := range tests {
		u := &url.URL{Scheme: "http", Host: tc.host}
		err := rejectPrivateHost(u)
		if tc.wantErr && err == nil {
			t.Errorf("rejectPrivateHost(%q) should have returned error", tc.host)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("rejectPrivateHost(%q) unexpected error: %v", tc.host, err)
		}
	}
}

// ── Helpers ──

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return json.RawMessage(data)
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("mustParseURL: %v", err)
	}
	return u
}
