// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
func newTestClient(t *testing.T) (*Client, *httptest.Server, *http.ServeMux) {
	t.Helper()
	srv, mux := newTestServer(t)
	c := NewClient(srv.URL, "test-id", "test-secret")
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
