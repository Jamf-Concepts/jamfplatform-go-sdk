// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
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
			name:     "devices",
			tenantID: "e77c1408-10c8-4007-b177-abc9157fbcaa",
			namespace: "devices", version: "v1",
			want: "/api/devices/v1/tenant/e77c1408-10c8-4007-b177-abc9157fbcaa",
		},
		{
			name:     "device groups",
			tenantID: "t-123",
			namespace: "device-groups", version: "v1",
			want: "/api/device-groups/v1/tenant/t-123",
		},
		{
			name:     "device actions",
			tenantID: "t-123",
			namespace: "device-actions", version: "v1",
			want: "/api/device-actions/v1/tenant/t-123",
		},
		{
			name:     "blueprints",
			tenantID: "t-abc",
			namespace: "blueprints", version: "v1",
			want: "/api/blueprints/v1/tenant/t-abc",
		},
		{
			name:     "compliance benchmarks",
			tenantID: "t-abc",
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

