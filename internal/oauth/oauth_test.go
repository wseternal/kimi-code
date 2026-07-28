package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTokenStorage_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	token := &TokenInfo{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Unix() + 3600,
		Scope:        "read write",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}

	// Save
	if err := storage.Save(ctx, "test-provider", token); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := storage.Load(ctx, "test-provider")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if loaded.AccessToken != token.AccessToken {
		t.Errorf("AccessToken mismatch: got %q, want %q", loaded.AccessToken, token.AccessToken)
	}
	if loaded.RefreshToken != token.RefreshToken {
		t.Errorf("RefreshToken mismatch: got %q, want %q", loaded.RefreshToken, token.RefreshToken)
	}
	if loaded.ExpiresIn != token.ExpiresIn {
		t.Errorf("ExpiresIn mismatch: got %d, want %d", loaded.ExpiresIn, token.ExpiresIn)
	}
}

func TestFileTokenStorage_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	token, err := storage.Load(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if token != nil {
		t.Error("Load should return nil for missing token")
	}
}

func TestFileTokenStorage_LoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	// Write corrupt file
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	token, err := storage.Load(ctx, "corrupt")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if token != nil {
		t.Error("Load should return nil for corrupt token")
	}
}

func TestFileTokenStorage_Remove(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	token := &TokenInfo{AccessToken: "test"}
	if err := storage.Save(ctx, "test", token); err != nil {
		t.Fatal(err)
	}

	if err := storage.Remove(ctx, "test"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	loaded, _ := storage.Load(ctx, "test")
	if loaded != nil {
		t.Error("Token should be removed")
	}
}

func TestFileTokenStorage_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	// Should reject path traversal attempts
	_, err := storage.Load(ctx, "../etc/passwd")
	if err == nil {
		t.Error("Should reject path traversal")
	}

	_, err = storage.Load(ctx, ".hidden")
	if err == nil {
		t.Error("Should reject hidden files")
	}
}

func TestFileTokenStorage_List(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	// Save multiple tokens
	storage.Save(ctx, "provider1", &TokenInfo{AccessToken: "t1"})
	storage.Save(ctx, "provider2", &TokenInfo{AccessToken: "t2"})

	names, err := storage.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("Expected 2 names, got %d", len(names))
	}
}

func TestRequestDeviceAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/oauth/device_authorization" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_code":                 "ABCD-1234",
			"device_code":               "device-123",
			"verification_uri":          "https://auth.kimi.com/verify",
			"verification_uri_complete": "https://auth.kimi.com/verify?code=ABCD-1234",
			"expires_in":                900,
			"interval":                  5,
		})
	}))
	defer server.Close()

	config := FlowConfig{
		Name:      "test",
		OAuthHost: server.URL,
		ClientID:  "test-client",
	}

	auth, err := RequestDeviceAuthorization(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("RequestDeviceAuthorization failed: %v", err)
	}

	if auth.UserCode != "ABCD-1234" {
		t.Errorf("UserCode mismatch: got %q", auth.UserCode)
	}
	if auth.DeviceCode != "device-123" {
		t.Errorf("DeviceCode mismatch: got %q", auth.DeviceCode)
	}
	if auth.Interval != 5 {
		t.Errorf("Interval mismatch: got %d", auth.Interval)
	}
}

func TestPollDeviceToken_Pending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "authorization_pending",
		})
	}))
	defer server.Close()

	config := FlowConfig{OAuthHost: server.URL, ClientID: "test"}
	result, err := PollDeviceToken(context.Background(), config, "device-123", nil)
	if err != nil {
		t.Fatalf("PollDeviceToken failed: %v", err)
	}
	if result.Kind != PollPending {
		t.Errorf("Expected PollPending, got %v", result.Kind)
	}
}

func TestPollDeviceToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-123",
			"refresh_token": "refresh-123",
			"expires_in":    3600,
			"token_type":    "Bearer",
		})
	}))
	defer server.Close()

	config := FlowConfig{OAuthHost: server.URL, ClientID: "test"}
	result, err := PollDeviceToken(context.Background(), config, "device-123", nil)
	if err != nil {
		t.Fatalf("PollDeviceToken failed: %v", err)
	}
	if result.Kind != PollSuccess {
		t.Errorf("Expected PollSuccess, got %v", result.Kind)
	}
	if result.Token.AccessToken != "access-123" {
		t.Errorf("AccessToken mismatch: got %q", result.Token.AccessToken)
	}
}

func TestPollDeviceToken_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "expired_token",
		})
	}))
	defer server.Close()

	config := FlowConfig{OAuthHost: server.URL, ClientID: "test"}
	result, err := PollDeviceToken(context.Background(), config, "device-123", nil)
	if err != nil {
		t.Fatalf("PollDeviceToken failed: %v", err)
	}
	if result.Kind != PollExpired {
		t.Errorf("Expected PollExpired, got %v", result.Kind)
	}
}

func TestPollDeviceToken_ServerError_ReturnsPollPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error_description": "service temporarily unavailable",
		})
	}))
	defer server.Close()

	config := FlowConfig{OAuthHost: server.URL, ClientID: "test"}
	result, err := PollDeviceToken(context.Background(), config, "device-123", nil)
	if err != nil {
		t.Fatalf("PollDeviceToken should not return error on 5xx, got: %v", err)
	}
	if result.Kind != PollPending {
		t.Errorf("Expected PollPending on 5xx, got %v", result.Kind)
	}
	if result.ErrorCode != "server_error" {
		t.Errorf("Expected ErrorCode 'server_error', got %q", result.ErrorCode)
	}
}

func TestPollDeviceToken_BadGateway_ReturnsPollPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html><body>Bad Gateway</body></html>"))
	}))
	defer server.Close()

	config := FlowConfig{OAuthHost: server.URL, ClientID: "test"}
	// 502 with non-JSON body should return error from JSON parse, not PollPending
	_, err := PollDeviceToken(context.Background(), config, "device-123", nil)
	if err == nil {
		t.Fatal("Expected error for non-JSON 5xx response, got nil")
	}
}

func TestRefreshAccessToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	config := FlowConfig{OAuthHost: server.URL, ClientID: "test"}
	token, err := RefreshAccessToken(context.Background(), config, "old-refresh", nil, DefaultRefreshOptions())
	if err != nil {
		t.Fatalf("RefreshAccessToken failed: %v", err)
	}
	if token.AccessToken != "new-access" {
		t.Errorf("AccessToken mismatch: got %q", token.AccessToken)
	}
}

func TestRefreshAccessToken_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid_grant",
		})
	}))
	defer server.Close()

	config := FlowConfig{OAuthHost: server.URL, ClientID: "test"}
	_, err := RefreshAccessToken(context.Background(), config, "bad-refresh", nil, DefaultRefreshOptions())
	if err == nil {
		t.Fatal("Expected error")
	}
	// Should be unauthorized error
	if !containsError(err, ErrUnauthorized) {
		t.Errorf("Expected ErrUnauthorized, got: %v", err)
	}
}

func TestManager_Login(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/device_authorization":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user_code":                 "TEST-1234",
				"device_code":               "device-123",
				"verification_uri_complete": "https://example.com/verify",
				"interval":                  1,
			})
		case "/api/oauth/token":
			pollCount++
			if pollCount < 2 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "authorization_pending",
				})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token":  "final-access",
					"refresh_token": "final-refresh",
					"expires_in":    3600,
				})
			}
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)

	manager := NewManager(ManagerOptions{
		Config: FlowConfig{
			Name:      "test-provider",
			OAuthHost: server.URL,
			ClientID:  "test-client",
		},
		Storage:   storage,
		ConfigDir: dir,
		Sleep:     func(d time.Duration) {}, // No actual sleep in tests
	})

	var deviceCodeShown bool
	token, err := manager.Login(context.Background(), LoginOptions{
		OnDeviceCode: func(auth *DeviceAuthorization) error {
			deviceCodeShown = true
			if auth.UserCode != "TEST-1234" {
				t.Errorf("UserCode mismatch: got %q", auth.UserCode)
			}
			return nil
		},
	})

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !deviceCodeShown {
		t.Error("OnDeviceCode callback not invoked")
	}
	if token.AccessToken != "final-access" {
		t.Errorf("AccessToken mismatch: got %q", token.AccessToken)
	}

	// Verify token was saved
	saved, _ := storage.Load(context.Background(), "test-provider")
	if saved == nil {
		t.Error("Token should be saved")
	}
}

func TestManager_EnsureFresh_NoRefreshNeeded(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	// Save a token that expires in 1 hour
	futureTime := time.Now().Unix() + 3600
	storage.Save(ctx, "test", &TokenInfo{
		AccessToken:  "valid-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    futureTime,
		ExpiresIn:    3600,
	})

	manager := NewManager(ManagerOptions{
		Config:    FlowConfig{Name: "test"},
		Storage:   storage,
		ConfigDir: dir,
	})

	token, err := manager.EnsureFresh(ctx, false)
	if err != nil {
		t.Fatalf("EnsureFresh failed: %v", err)
	}
	if token != "valid-token" {
		t.Errorf("Token mismatch: got %q", token)
	}
}

func TestManager_HasToken(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	manager := NewManager(ManagerOptions{
		Config:    FlowConfig{Name: "test"},
		Storage:   storage,
		ConfigDir: dir,
	})

	if manager.HasToken(ctx) {
		t.Error("Should not have token initially")
	}

	storage.Save(ctx, "test", &TokenInfo{AccessToken: "token"})

	if !manager.HasToken(ctx) {
		t.Error("Should have token after save")
	}
}

func TestManager_Logout(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileTokenStorage(dir)
	ctx := context.Background()

	storage.Save(ctx, "test", &TokenInfo{AccessToken: "token"})

	manager := NewManager(ManagerOptions{
		Config:    FlowConfig{Name: "test"},
		Storage:   storage,
		ConfigDir: dir,
	})

	if err := manager.Logout(ctx); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	if manager.HasToken(ctx) {
		t.Error("Should not have token after logout")
	}
}

func TestRefreshThreshold(t *testing.T) {
	// Less than 300s should return 300
	if got := refreshThreshold(100); got != 300 {
		t.Errorf("Expected 300, got %d", got)
	}

	// More than 600s should return 50%
	if got := refreshThreshold(1000); got != 500 {
		t.Errorf("Expected 500, got %d", got)
	}

	// Zero should return 300
	if got := refreshThreshold(0); got != 300 {
		t.Errorf("Expected 300, got %d", got)
	}
}

func TestTokenWire(t *testing.T) {
	token := &TokenInfo{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    1234567890,
		Scope:        "read",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}

	wire := token.toWire()
	if wire.AccessToken != "access" {
		t.Errorf("Wire AccessToken mismatch")
	}

	restored := fromWire(wire)
	if restored.AccessToken != token.AccessToken {
		t.Errorf("Restored AccessToken mismatch")
	}
	if restored.ExpiresAt != token.ExpiresAt {
		t.Errorf("Restored ExpiresAt mismatch")
	}
}

func containsError(err, target error) bool {
	for e := err; e != nil; {
		if e == target {
			return true
		}
		if unwrapper, ok := e.(interface{ Unwrap() error }); ok {
			e = unwrapper.Unwrap()
		} else {
			break
		}
	}
	return false
}
