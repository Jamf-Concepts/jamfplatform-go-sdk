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

// TestScopeOptionsAreMutuallyExclusive pins the precedence the option godoc
// promises. A client carries exactly one scope, so setting both WithTenantID
// and WithEnvironmentID is a configuration mistake rather than a combination —
// environment wins, and it wins regardless of the order the options are passed,
// because NewClient applies them in a fixed order rather than the caller's.
//
// Worth pinning because the mechanism is invisible at the call site: a reader
// would reasonably expect last-one-wins, and swapping the two blocks in
// NewClient would silently invert the behaviour with no test failing.
func TestScopeOptionsAreMutuallyExclusive(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
		want string
	}{
		{"tenant only", []Option{WithTenantID("T-1")}, "T-1"},
		{"environment only", []Option{WithEnvironmentID("E-1")}, ""},
		{"tenant then environment", []Option{WithTenantID("T-1"), WithEnvironmentID("E-1")}, ""},
		{"environment then tenant", []Option{WithEnvironmentID("E-1"), WithTenantID("T-1")}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient("https://example.invalid", "id", "secret", tc.opts...)
			// TenantID() reports a value only when the scope really is a tenant,
			// so an empty result means the client ended up environment-scoped.
			if got := c.transport.TenantID(); got != tc.want {
				t.Errorf("TenantID() = %q, want %q — scope precedence changed", got, tc.want)
			}
		})
	}
}

// TestScopeIsReadableByConsumers is written the way a consumer must write it:
// naming jamfplatform.ScopeKind, switching over the exported constants and
// calling ScopeHeader, with no import of internal/client. That was impossible
// before — the kind was exported from an internal package, so no consumer could
// name the type — which forced downstream code to pass the scope in alongside
// the client rather than read it back off one.
//
// It also pins that all three states are distinguishable. TenantID alone cannot
// do that: it returns "" for both environment and organization scope, so a
// consumer reading only that accessor cannot tell an environment-scoped client
// from an unscoped one.
func TestScopeIsReadableByConsumers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		opts       []Option
		wantKind   ScopeKind
		wantHeader string
		wantID     string
	}{
		{"tenant", []Option{WithTenantID("T-1")}, ScopeTenant, "X-Tenant-Id", "T-1"},
		{"environment", []Option{WithEnvironmentID("E-1")}, ScopeEnvironment, "X-Environment-Id", "E-1"},
		// No scope option: the organization-scoped case, where the gateway
		// resolves context from the token and the client sends no scope header.
		{"organization", nil, 0, "", ""},
		// An ID-less kind is not a scope; Scope must not report a header the
		// request will not carry.
		{"tenant with empty ID", []Option{WithTenantID("")}, 0, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient("https://example.invalid", "id", "secret", tc.opts...)
			kind, id := c.Scope()
			if kind != tc.wantKind {
				t.Errorf("kind = %v, want %v", kind, tc.wantKind)
			}
			if got := kind.ScopeHeader(); got != tc.wantHeader {
				t.Errorf("ScopeHeader() = %q, want %q", got, tc.wantHeader)
			}
			if id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
			// The three states must not collapse into each other.
			if kind == ScopeTenant && c.transport.TenantID() != id {
				t.Errorf("TenantID() = %q disagrees with Scope() id %q", c.transport.TenantID(), id)
			}
		})
	}
}
