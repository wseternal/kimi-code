package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxFetchSize     = 10 * 1024 * 1024 // 10MB
	fetchTimeout     = 30 * time.Second
	maxRedirects     = 10
	chromeUserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// URLFetcher is the provider interface for URL fetching.
type URLFetcher interface {
	Fetch(ctx context.Context, rawURL string) (string, error)
}

// FetchURLTool fetches content from a URL.
type FetchURLTool struct {
	Fetcher URLFetcher
}

func NewFetchURLTool() *FetchURLTool {
	return &FetchURLTool{Fetcher: &LocalURLFetcher{}}
}

type fetchURLInput struct {
	URL string `json:"url"`
}

func (t *FetchURLTool) Definition() Definition {
	return Definition{
		Name:        "FetchURL",
		Description: "Fetch content from a URL. Returns the text content of the page.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{"type": "string", "description": "The URL to fetch content from (http or https)"},
			},
			"required": []string{"url"},
		},
	}
}

func (t *FetchURLTool) Execute(ctx context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var params fetchURLInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.URL == "" {
		return &Result{Output: "URL is required", IsError: true}, nil
	}

	content, err := t.Fetcher.Fetch(ctx, params.URL)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Error fetching URL: %s", err.Error()), IsError: true}, nil
	}
	return &Result{Output: content}, nil
}

// LocalURLFetcher fetches URLs using net/http with SSRF protection.
type LocalURLFetcher struct{}

func (f *LocalURLFetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("only http and https URLs are supported, got %q", parsed.Scheme)
	}

	// SSRF guard: resolve and check IP
	host := parsed.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("DNS lookup failed for %s: %w", host, err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return "", fmt.Errorf("access to private IP addresses is blocked (SSRF protection)")
		}
	}

	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			// Check redirect target IP
			redirectHost := req.URL.Hostname()
			redirectIPs, err := net.LookupIP(redirectHost)
			if err != nil {
				return fmt.Errorf("DNS lookup failed for redirect target: %w", err)
			}
			for _, ip := range redirectIPs {
				if isPrivateIP(ip) {
					return fmt.Errorf("redirect to private IP blocked")
				}
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", chromeUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,text/markdown,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read with size limit
	limited := io.LimitReader(resp.Body, maxFetchSize)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	text := string(body)

	// Extract text from HTML
	if strings.Contains(contentType, "text/html") {
		text = extractTextFromHTML(text)
	}

	if strings.TrimSpace(text) == "" {
		return "The page returned empty content.", nil
	}

	return text, nil
}

// isPrivateIP checks if an IP is private/reserved (SSRF protection).
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// extractTextFromHTML strips HTML tags and returns readable text.
func extractTextFromHTML(html string) string {
	// Remove script and style blocks
	html = removeBetween(html, "<script", "</script>")
	html = removeBetween(html, "<style", "</style>")
	html = removeBetween(html, "<!--", "-->")

	// Replace block elements with newlines
	for _, tag := range []string{"</p>", "</div>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>", "</li>", "</br>", "<br>", "<br/>"} {
		html = strings.ReplaceAll(html, tag, "\n")
	}

	// Strip remaining tags
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}

	// Clean up whitespace
	lines := strings.Split(result.String(), "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// removeBetween removes content between startTag and endTag (inclusive).
func removeBetween(s, startTag, endTag string) string {
	for {
		start := strings.Index(strings.ToLower(s), strings.ToLower(startTag))
		if start == -1 {
			break
		}
		end := strings.Index(strings.ToLower(s[start:]), strings.ToLower(endTag))
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+len(endTag):]
	}
	return s
}

// WebSearchProvider is the provider interface for web search.
type WebSearchProvider interface {
	Search(ctx context.Context, query string) ([]WebSearchResult, error)
}

// WebSearchResult represents a single search result.
type WebSearchResult struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Snippet  string `json:"snippet"`
	Date     string `json:"date,omitempty"`
	SiteName string `json:"site_name,omitempty"`
}

// WebSearchTool searches the web.
type WebSearchTool struct {
	Provider WebSearchProvider
}

func NewWebSearchTool(provider WebSearchProvider) *WebSearchTool {
	return &WebSearchTool{Provider: provider}
}

type webSearchInput struct {
	Query string `json:"query"`
}

func (t *WebSearchTool) Definition() Definition {
	return Definition{
		Name:        "WebSearch",
		Description: "Search the web for information. Returns a list of results with titles, URLs, and snippets.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "The search query"},
			},
			"required": []string{"query"},
		},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var params webSearchInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.Query == "" {
		return &Result{Output: "Query is required", IsError: true}, nil
	}
	if t.Provider == nil {
		return &Result{Output: "Web search is not configured. Set up a WebSearchProvider.", IsError: true}, nil
	}

	results, err := t.Provider.Search(ctx, params.Query)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Search error: %s", err.Error()), IsError: true}, nil
	}

	if len(results) == 0 {
		return &Result{Output: "No search results found."}, nil
	}

	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		b.WriteString(fmt.Sprintf("Title: %s\n", r.Title))
		if r.SiteName != "" {
			b.WriteString(fmt.Sprintf("Site: %s\n", r.SiteName))
		}
		if r.Date != "" {
			b.WriteString(fmt.Sprintf("Date: %s\n", r.Date))
		}
		b.WriteString(fmt.Sprintf("URL: %s\n", r.URL))
		b.WriteString(fmt.Sprintf("Snippet: %s\n", r.Snippet))
	}
	return &Result{Output: b.String()}, nil
}
