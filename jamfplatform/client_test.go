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
	if got := c.transport.TenantID(); got != "tenant-uuid" {
		t.Errorf("tenantID = %q, want tenant-uuid", got)
	}
}

