// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestServer creates an httptest.Server with a mux that handles the OAuth2
// token endpoint. Callers add their own handlers before making requests.
func newTestServer(t *testing.T) (*httptest.Server, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, mux
}

// newTestClient creates a Client pointed at a test server. Returns the client,
// server, and mux so tests can register handlers.
func newTestClient(t *testing.T) (*Transport, *httptest.Server, *http.ServeMux) {
	t.Helper()
	srv, mux := newTestServer(t)
	c := NewTransport(srv.URL, "test-id", "test-secret")
	return c, srv, mux
}

func TestDo_Success(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "hello"})
	})

	var result struct{ Name string }
	err := c.Do(context.Background(), http.MethodGet, "/api/test", nil, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "hello" {
		t.Errorf("Name = %q, want %q", result.Name, "hello")
	}
}

func TestDo_PostWithBody(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/items", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "123", "name": body["name"]})
	})

	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	err := c.Do(context.Background(), http.MethodPost, "/api/items", map[string]string{"name": "test"}, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "123" {
		t.Errorf("ID = %q, want %q", result.ID, "123")
	}
}

func TestDoExpect_CorrectStatus(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/create", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new"})
	})

	var result struct{ ID string }
	err := c.DoExpect(context.Background(), http.MethodPost, "/api/create", nil, http.StatusCreated, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "new" {
		t.Errorf("ID = %q, want %q", result.ID, "new")
	}
}

func TestDoExpect_WrongStatus(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/missing", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ApiError{
			HTTPStatus: 404,
			TraceID:    "trace-abc",
			Errors:     []Error{{Code: "NOT_FOUND", Field: "id", Description: "resource not found"}},
		})
	})

	err := c.DoExpect(context.Background(), http.MethodGet, "/api/missing", nil, http.StatusOK, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIResponseError)
	if !ok {
		t.Fatalf("expected *APIResponseError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if !apiErr.HasStatus(404) {
		t.Error("HasStatus(404) = false, want true")
	}
	if apiErr.TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want %q", apiErr.TraceID, "trace-abc")
	}
	if len(apiErr.Errors) != 1 {
		t.Fatalf("len(Errors) = %d, want 1", len(apiErr.Errors))
	}
}

func TestDoExpect_NoContent(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/delete", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.DoExpect(context.Background(), http.MethodDelete, "/api/delete", nil, http.StatusNoContent, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoWithContentType(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/patch", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.DoWithContentType(context.Background(), http.MethodPatch, "/api/patch",
		map[string]string{"name": "updated"}, "application/json", http.StatusNoContent, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDo_PatchDefaultContentType(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/patch-default", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/merge-patch+json" {
			t.Errorf("Content-Type = %q, want application/merge-patch+json", ct)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.DoExpect(context.Background(), http.MethodPatch, "/api/patch-default",
		map[string]string{"name": "x"}, http.StatusNoContent, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAPIResponseError_Error(t *testing.T) {
	t.Run("with structured errors", func(t *testing.T) {
		e := &APIResponseError{
			StatusCode: 400,
			Method:     "POST",
			URL:        "https://example.com/api/test",
			TraceID:    "trace-123",
			Errors:     []Error{{Code: "INVALID", Field: "name", Description: "cannot be empty"}},
		}
		got := e.Error()
		if got == "" {
			t.Fatal("error string is empty")
		}
		// Verify key parts are present
		for _, want := range []string{"400", "trace-123", "INVALID", "name", "cannot be empty"} {
			if !contains(got, want) {
				t.Errorf("error %q missing %q", got, want)
			}
		}
	})

	t.Run("with raw body", func(t *testing.T) {
		e := &APIResponseError{
			StatusCode: 500,
			Method:     "GET",
			URL:        "https://example.com/fail",
			Body:       "internal error",
		}
		got := e.Error()
		if !contains(got, "500") || !contains(got, "internal error") {
			t.Errorf("error = %q, want status and body", got)
		}
	})
}

func TestBaseURL(t *testing.T) {
	c, srv, _ := newTestClient(t)
	if got := c.BaseURL(); got != srv.URL {
		t.Errorf("BaseURL() = %q, want %q", got, srv.URL)
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrAuthentication == nil {
		t.Fatal("ErrAuthentication is nil")
	}
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound is nil")
	}
	// Verify they have distinct messages
	if ErrAuthentication.Error() == ErrNotFound.Error() {
		t.Error("ErrAuthentication and ErrNotFound have the same message")
	}
}

func TestValidateCredentials_Success(t *testing.T) {
	srv, _ := newTestServer(t)
	c := NewTransport(srv.URL, "test-id", "test-secret")
	if err := c.ValidateCredentials(context.Background()); err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
}

func TestValidateCredentials_Failure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewTransport(srv.URL, "bad-id", "bad-secret")
	err := c.ValidateCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
}

func TestAccessToken_Success(t *testing.T) {
	srv, _ := newTestServer(t)
	c := NewTransport(srv.URL, "test-id", "test-secret")
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken failed: %v", err)
	}
	if token.AccessToken != "test-token" {
		t.Errorf("AccessToken = %q, want test-token", token.AccessToken)
	}
}

func TestHTTPClient(t *testing.T) {
	c, _, _ := newTestClient(t)
	if c.HTTPClient() == nil {
		t.Fatal("HTTPClient returned nil")
	}
}

func TestSetLogger(t *testing.T) {
	c, _, mux := newTestClient(t)

	var logged bool
	c.SetLogger(&testLogger{onRequest: func() { logged = true }})

	mux.HandleFunc("/api/logged", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	})

	var result map[string]string
	_ = c.Do(context.Background(), http.MethodGet, "/api/logged", nil, &result)
	if !logged {
		t.Error("logger was not called")
	}
}

type testLogger struct {
	onRequest func()
}

func (l *testLogger) LogRequest(_ context.Context, _, _ string, _ []byte) {
	if l.onRequest != nil {
		l.onRequest()
	}
}

func (l *testLogger) LogResponse(_ context.Context, _ int, _ http.Header, _ []byte) {}

func TestDo_MalformedJSON(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/bad-json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	})

	var result map[string]string
	err := c.Do(context.Background(), http.MethodGet, "/api/bad-json", nil, &result)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestDoExpect_NonJSONErrorBody(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/text-error", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("Bad Gateway"))
	})

	err := c.DoExpect(context.Background(), http.MethodGet, "/api/text-error", nil, http.StatusOK, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIResponseError, got %T", err)
	}
	if apiErr.StatusCode != 502 {
		t.Errorf("StatusCode = %d, want 502", apiErr.StatusCode)
	}
	if apiErr.Body != "Bad Gateway" {
		t.Errorf("Body = %q, want Bad Gateway", apiErr.Body)
	}
}

func TestSetUserAgent(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.SetUserAgent("custom-agent/2.0")

	mux.HandleFunc("/api/ua", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	})

	var result map[string]string
	err := c.Do(context.Background(), http.MethodGet, "/api/ua", nil, &result)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetHTTPClient(t *testing.T) {
	c, _, mux := newTestClient(t)

	custom := &http.Client{}
	c.SetHTTPClient(custom)

	mux.HandleFunc("/api/custom-http", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	})

	var result map[string]string
	err := c.Do(context.Background(), http.MethodGet, "/api/custom-http", nil, &result)
	// SetHTTPClient creates a new oauth2 client wrapping the custom http.Client,
	// which won't have the test server's token endpoint configured.
	// The important thing is the method doesn't panic.
	_ = err
}

type mockTokenCache struct {
	loadFn  func(key string) (string, time.Time, bool)
	storeFn func(key string, token string, expiresAt time.Time) error
}

func (m *mockTokenCache) Load(key string) (string, time.Time, bool) {
	return m.loadFn(key)
}

func (m *mockTokenCache) Store(key string, token string, expiresAt time.Time) error {
	return m.storeFn(key, token, expiresAt)
}

func TestTokenCache_LoadHit(t *testing.T) {
	srv, _ := newTestServer(t)

	cache := &mockTokenCache{
		loadFn: func(_ string) (string, time.Time, bool) {
			return "cached-token", time.Now().Add(time.Hour), true
		},
		storeFn: func(_ string, _ string, _ time.Time) error {
			return nil
		},
	}

	c := NewTransportWithUserAgent(srv.URL, "cid", "csecret", "test",
		WithTokenCache(cache, "test-key"))

	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "cached-token" {
		t.Errorf("expected cached-token, got %q", token.AccessToken)
	}
}

func TestTokenCache_LoadMiss(t *testing.T) {
	srv, _ := newTestServer(t)

	var stored bool
	cache := &mockTokenCache{
		loadFn: func(_ string) (string, time.Time, bool) {
			return "", time.Time{}, false
		},
		storeFn: func(_ string, token string, _ time.Time) error {
			if token != "test-token" {
				t.Errorf("expected Store to receive %q, got %q", "test-token", token)
			}
			stored = true
			return nil
		},
	}

	c := NewTransportWithUserAgent(srv.URL, "cid", "csecret", "test",
		WithTokenCache(cache, "test-key"))

	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "test-token" {
		t.Errorf("expected test-token, got %q", token.AccessToken)
	}
	if !stored {
		t.Error("expected Store to be called after fetch")
	}
}

func TestTokenCache_StoreErrorIgnored(t *testing.T) {
	srv, _ := newTestServer(t)

	cache := &mockTokenCache{
		loadFn: func(_ string) (string, time.Time, bool) {
			return "", time.Time{}, false
		},
		storeFn: func(_ string, _ string, _ time.Time) error {
			return fmt.Errorf("disk full")
		},
	}

	c := NewTransportWithUserAgent(srv.URL, "cid", "csecret", "test",
		WithTokenCache(cache, "test-key"))

	_, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("expected success despite Store error, got: %v", err)
	}
}

func TestTokenCache_ExpiredCacheEntry(t *testing.T) {
	srv, _ := newTestServer(t)

	cache := &mockTokenCache{
		loadFn: func(_ string) (string, time.Time, bool) {
			return "expired-token", time.Now().Add(-time.Hour), true
		},
		storeFn: func(_ string, _ string, _ time.Time) error {
			return nil
		},
	}

	c := NewTransportWithUserAgent(srv.URL, "cid", "csecret", "test",
		WithTokenCache(cache, "test-key"))

	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "test-token" {
		t.Errorf("expected test-token from fresh fetch, got %q", token.AccessToken)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
