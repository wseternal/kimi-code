package tools

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
)

// mockFetcher implements URLFetcher for testing.
type mockFetcher struct {
	content string
	err     error
}

func (m *mockFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return m.content, m.err
}

func TestFetchURLTool_ValidURL(t *testing.T) {
	fetcher := &mockFetcher{content: "Hello World content", err: nil}
	tool := &FetchURLTool{Fetcher: fetcher}
	ctx := context.Background()

	input := json.RawMessage(`{"url": "https://example.com"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Hello World") {
		t.Errorf("expected content in output, got: %s", result.Output)
	}
}

func TestFetchURLTool_EmptyURL(t *testing.T) {
	tool := &FetchURLTool{Fetcher: &mockFetcher{}}
	ctx := context.Background()

	input := json.RawMessage(`{"url": ""}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error for empty URL, got: %s", result.Output)
	}
}

func TestFetchURLTool_FetchError(t *testing.T) {
	fetcher := &mockFetcher{content: "", err: context.DeadlineExceeded}
	tool := &FetchURLTool{Fetcher: fetcher}
	ctx := context.Background()

	input := json.RawMessage(`{"url": "https://example.com"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error result, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Error") {
		t.Errorf("expected 'Error' in output, got: %s", result.Output)
	}
}

func TestFetchURLTool_EmptyContent(t *testing.T) {
	fetcher := &mockFetcher{content: "", err: nil}
	tool := &FetchURLTool{Fetcher: fetcher}
	ctx := context.Background()

	input := json.RawMessage(`{"url": "https://example.com"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty content from fetcher is passed through
	if result.Output != "" {
		t.Errorf("expected empty output, got: %s", result.Output)
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false}, // example.com
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if got := isPrivateIP(ip); got != tt.want {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestExtractTextFromHTML(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "simple paragraph",
			html: "<html><body><p>Hello World</p></body></html>",
			want: "Hello World",
		},
		{
			name: "strips script tags",
			html: "<html><body><script>alert('x')</script><p>Content</p></body></html>",
			want: "Content",
		},
		{
			name: "strips style tags",
			html: "<html><body><style>.foo{}</style><p>Content</p></body></html>",
			want: "Content",
		},
		{
			name: "multiple paragraphs",
			html: "<p>First</p><p>Second</p>",
			want: "First\nSecond",
		},
		{
			name: "strips inline tags",
			html: "<p>Hello <b>bold</b> and <i>italic</i></p>",
			want: "Hello bold and italic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextFromHTML(tt.html)
			if got != tt.want {
				t.Errorf("extractTextFromHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

// mockSearchProvider implements WebSearchProvider for testing.
type mockSearchProvider struct {
	results []WebSearchResult
	err     error
}

func (m *mockSearchProvider) Search(_ context.Context, _ string) ([]WebSearchResult, error) {
	return m.results, m.err
}

func TestWebSearchTool_Results(t *testing.T) {
	provider := &mockSearchProvider{
		results: []WebSearchResult{
			{Title: "Go Language", URL: "https://go.dev", Snippet: "Go is an open source programming language", SiteName: "go.dev"},
			{Title: "Go Tutorial", URL: "https://go.dev/tour", Snippet: "A Tour of Go", Date: "2024-01-15"},
		},
	}
	tool := NewWebSearchTool(provider)
	ctx := context.Background()

	input := json.RawMessage(`{"query": "golang"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Go Language") {
		t.Errorf("expected title in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "https://go.dev") {
		t.Errorf("expected URL in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "---") {
		t.Errorf("expected separator between results, got: %s", result.Output)
	}
}

func TestWebSearchTool_NoResults(t *testing.T) {
	provider := &mockSearchProvider{results: []WebSearchResult{}}
	tool := NewWebSearchTool(provider)
	ctx := context.Background()

	input := json.RawMessage(`{"query": "xyz123nonexistent"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "No search results") {
		t.Errorf("expected 'No search results' message, got: %s", result.Output)
	}
}

func TestWebSearchTool_EmptyQuery(t *testing.T) {
	tool := NewWebSearchTool(&mockSearchProvider{})
	ctx := context.Background()

	input := json.RawMessage(`{"query": ""}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error for empty query, got: %s", result.Output)
	}
}

func TestWebSearchTool_NoProvider(t *testing.T) {
	tool := NewWebSearchTool(nil)
	ctx := context.Background()

	input := json.RawMessage(`{"query": "test"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error for nil provider, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "not configured") {
		t.Errorf("expected 'not configured' message, got: %s", result.Output)
	}
}
