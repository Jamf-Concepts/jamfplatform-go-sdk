// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_DefaultUserAgent(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if got := c.BaseURL(); got != "https://example.com" {
		t.Errorf("BaseURL() = %q, want %q", got, "https://example.com")
	}
}

func TestNewClient_WithUserAgent(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret", WithUserAgent("custom/1.0"))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestNewClient_EmptyUserAgent(t *testing.T) {
	// Empty user agent should keep the default
	c := NewClient("https://example.com", "id", "secret", WithUserAgent(""))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret", WithHTTPClient(nil))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestNewClient_WithLogger(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret", WithLogger(nil))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestValidateCredentials_Success(t *testing.T) {
	c, _ := testServer(t)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
}

func TestValidateCredentials_Failure(t *testing.T) {
	// Point at a server that returns an error for the token endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "bad-id", "bad-secret")
	err := c.ValidateCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
}

func TestAccessToken_Success(t *testing.T) {
	c, _ := testServer(t)
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken failed: %v", err)
	}
	if token.AccessToken != "test-token" {
		t.Errorf("AccessToken = %q, want test-token", token.AccessToken)
	}
}

func TestWithTenantID(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret", WithTenantID("tenant-uuid"))
	if c.tenantID != "tenant-uuid" {
		t.Errorf("tenantID = %q, want tenant-uuid", c.tenantID)
	}
}

func TestTenantPrefix(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		namespace string
		version   string
		want      string
	}{
		{
			name:      "devices",
			tenantID:  "e77c1408-10c8-4007-b177-abc9157fbcaa",
			namespace: "devices", version: "v1",
			want: "/api/devices/v1/tenant/e77c1408-10c8-4007-b177-abc9157fbcaa",
		},
		{
			name:      "device groups",
			tenantID:  "t-123",
			namespace: "device-groups", version: "v1",
			want: "/api/device-groups/v1/tenant/t-123",
		},
		{
			name:      "device actions",
			tenantID:  "t-123",
			namespace: "device-actions", version: "v1",
			want: "/api/device-actions/v1/tenant/t-123",
		},
		{
			name:      "blueprints",
			tenantID:  "t-abc",
			namespace: "blueprints", version: "v1",
			want: "/api/blueprints/v1/tenant/t-abc",
		},
		{
			name:      "compliance benchmarks",
			tenantID:  "t-abc",
			namespace: "compliance-benchmarks", version: "v1",
			want: "/api/compliance-benchmarks/v1/tenant/t-abc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{tenantID: tt.tenantID}
			got := c.tenantPrefix(tt.namespace, tt.version)
			if got != tt.want {
				t.Errorf("tenantPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Multi-page pagination tests — exercises each generated pagination style
// ---------------------------------------------------------------------------

func TestListDevices_MultiPage(t *testing.T) {
	// hasNext style: server returns hasNext=true/false to signal more pages.
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	callCount := 0
	mux.HandleFunc("/api/devices/v1/tenant/t-test/devices", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results": []map[string]any{{"id": "d-1"}, {"id": "d-2"}},
				"hasNext": true,
			})
		case "1":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results": []map[string]any{{"id": "d-3"}},
				"hasNext": false,
			})
		default:
			t.Errorf("unexpected page %s", page)
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	devices, err := c.ListDevices(context.Background(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 3 {
		t.Fatalf("got %d devices, want 3", len(devices))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}

func TestListBlueprints_MultiPage(t *testing.T) {
	// sizeCheck style: hasMore = len(results) >= pageSize && len(results) > 0.
	// ListAllPages uses pageSize=100, so we return 100 items on page 0
	// and fewer on page 1 to signal end.
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	callCount := 0
	mux.HandleFunc("/api/blueprints/v1/tenant/t-test/blueprints", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("bp-%d", i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 150,
			})
		case "1":
			items := make([]map[string]any, 50)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("bp-%d", 100+i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 150,
			})
		default:
			t.Errorf("unexpected page %s", page)
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	blueprints, err := c.ListBlueprints(context.Background(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(blueprints) != 150 {
		t.Fatalf("got %d blueprints, want 150", len(blueprints))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}

func TestListDeclarationReportClients_MultiPage(t *testing.T) {
	// totalCount style: hasNext = (page+1)*pageSize < totalCount.
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	callCount := 0
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/declarations/decl-1", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"deviceId": fmt.Sprintf("dev-%d", i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 120,
			})
		case "1":
			items := make([]map[string]any, 20)
			for i := range items {
				items[i] = map[string]any{"deviceId": fmt.Sprintf("dev-%d", 100+i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 120,
			})
		default:
			t.Errorf("unexpected page %s", page)
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	clients, err := c.ListDeclarationReportClients(context.Background(), "decl-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 120 {
		t.Fatalf("got %d clients, want 120", len(clients))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}
