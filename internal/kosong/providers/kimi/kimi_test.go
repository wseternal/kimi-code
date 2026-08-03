package kimi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/visdomtech/kimi-code/internal/kosong"
)

func TestUploadVideo_InMemoryBytes(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Parse multipart form
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("failed to parse form: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Check purpose
		if purpose := r.FormValue("purpose"); purpose != "video" {
			t.Errorf("expected purpose 'video', got %q", purpose)
		}

		// Check file
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("failed to get file: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// No filename provided, so it uses the generated one based on MIME type
		if header.Filename != "upload.mp4" {
			t.Errorf("expected filename 'upload.mp4', got %q", header.Filename)
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "file-12345"})
	}))
	defer server.Close()

	prov := NewProvider(ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "moonshot-v1-8k",
	})

	input := &kosong.VideoUploadInput{
		Data:     []byte("fake video data"),
		MIMEType: "video/mp4",
	}

	result, err := prov.UploadVideo(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "video_url" {
		t.Errorf("expected type 'video_url', got %q", result.Type)
	}
	if result.VideoURL.URL != "ms://file-12345" {
		t.Errorf("expected URL 'ms://file-12345', got %q", result.VideoURL.URL)
	}
	if result.VideoURL.ID == nil || *result.VideoURL.ID != "file-12345" {
		t.Errorf("expected ID 'file-12345', got %v", result.VideoURL.ID)
	}
}

func TestUploadVideo_FilePath(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.mp4")
	if err := os.WriteFile(tmpFile, []byte("fake video"), 0644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("failed to parse form: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("failed to get file: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		defer file.Close()
		if header.Filename != "test.mp4" {
			t.Errorf("expected filename 'test.mp4', got %q", header.Filename)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "file-99999"})
	}))
	defer server.Close()

	prov := NewProvider(ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "moonshot-v1-8k",
	})

	result, err := prov.UploadVideo(context.Background(), tmpFile, nil)
	if err != nil {
		t.Fatal(err)
	}

	if result.VideoURL.URL != "ms://file-99999" {
		t.Errorf("expected URL 'ms://file-99999', got %q", result.VideoURL.URL)
	}
}

func TestUploadVideo_FileNotFound(t *testing.T) {
	prov := NewProvider(ProviderConfig{
		APIKey:  "test-key",
		BaseURL: "http://localhost:1",
		Model:   "moonshot-v1-8k",
	})

	_, err := prov.UploadVideo(context.Background(), "/nonexistent/video.mp4", nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUploadVideo_InvalidMIME(t *testing.T) {
	prov := NewProvider(ProviderConfig{
		APIKey:  "test-key",
		BaseURL: "http://localhost:1",
		Model:   "moonshot-v1-8k",
	})

	input := &kosong.VideoUploadInput{
		Data:     []byte("data"),
		MIMEType: "image/png",
	}

	_, err := prov.UploadVideo(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error for non-video MIME type")
	}
}

func TestUploadVideo_NoAPIKey(t *testing.T) {
	prov := NewProvider(ProviderConfig{
		BaseURL: "http://localhost:1",
		Model:   "moonshot-v1-8k",
	})

	input := &kosong.VideoUploadInput{
		Data:     []byte("data"),
		MIMEType: "video/mp4",
	}

	_, err := prov.UploadVideo(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestUploadVideo_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	prov := NewProvider(ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "moonshot-v1-8k",
	})

	input := &kosong.VideoUploadInput{
		Data:     []byte("data"),
		MIMEType: "video/mp4",
	}

	_, err := prov.UploadVideo(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestUploadVideo_AuthOverride(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "file-auth"})
	}))
	defer server.Close()

	prov := NewProvider(ProviderConfig{
		APIKey:  "base-key",
		BaseURL: server.URL,
		Model:   "moonshot-v1-8k",
	})

	overrideKey := "override-key"
	opts := &kosong.GenerateOptions{
		Auth: &kosong.ProviderRequestAuth{
			APIKey: &overrideKey,
		},
	}

	input := &kosong.VideoUploadInput{
		Data:     []byte("data"),
		MIMEType: "video/mp4",
	}

	_, err := prov.UploadVideo(context.Background(), input, opts)
	if err != nil {
		t.Fatal(err)
	}

	if receivedAuth != "Bearer override-key" {
		t.Errorf("expected 'Bearer override-key', got %q", receivedAuth)
	}
}

func TestGuessMIMEFromExt(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"video.mp4", "video/mp4"},
		{"clip.mov", "video/quicktime"},
		{"movie.avi", "video/x-msvideo"},
		{"test.webm", "video/webm"},
		{"noext", ""},
		{"file.txt", ""},
	}

	for _, tc := range tests {
		got := guessMIMEFromExt(tc.filename)
		if got != tc.expected {
			t.Errorf("guessMIMEFromExt(%q) = %q, want %q", tc.filename, got, tc.expected)
		}
	}
}

func TestGuessFilename(t *testing.T) {
	if got := guessFilename("video/mp4"); got != "upload.mp4" {
		t.Errorf("guessFilename(video/mp4) = %q", got)
	}
	if got := guessFilename("video/unknown"); got != "upload.bin" {
		t.Errorf("guessFilename(video/unknown) = %q", got)
	}
}

func TestUploadVideo_InvalidExtension(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("not a video"), 0644); err != nil {
		t.Fatal(err)
	}

	prov := NewProvider(ProviderConfig{
		APIKey:  "test-key",
		BaseURL: "http://localhost:1",
		Model:   "moonshot-v1-8k",
	})

	_, err := prov.UploadVideo(context.Background(), tmpFile, nil)
	if err == nil {
		t.Fatal("expected error for non-video file extension")
	}
}
