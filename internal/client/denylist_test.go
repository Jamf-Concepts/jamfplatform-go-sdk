// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCheckDeniedPath(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		url     string
		wantErr bool
	}{
		// Header-scoped URLs — the shape the SDK builds now. These are the
		// cases that regressed to "allowed" when scoping left the path, so they
		// come first: a fixture carrying /tenant/ can no longer prove anything
		// about a real request.
		{"denied auth token, header-scoped URL", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/auth/token", true},
		{"denied auth keep-alive, header-scoped URL", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/auth/keep-alive", true},
		{"denied auth invalidate, header-scoped URL", http.MethodPost, "https://x.apigw.jamf.com/api/pro/auth/invalidate-token", true},
		{"denied auth current, header-scoped URL", http.MethodGet, "https://x.apigw.jamf.com/api/pro/auth/current", true},
		{"denied oauth token, header-scoped URL", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/oauth/token", true},
		{"denied with query string, header-scoped URL", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/auth/token?foo=1", true},
		{"allowed resource, header-scoped URL", http.MethodGet, "https://x.apigw.jamf.com/api/pro/v1/computers", false},
		{"method mismatch is allowed, header-scoped URL", http.MethodGet, "https://x.apigw.jamf.com/api/pro/v1/auth/token", false},
		// A path ending in a longer segment must not match: the leading slash on
		// each denied suffix is what prevents it.
		{"similar suffix is not denied", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/xauth/token", false},
		// Legacy path-scoped URLs. The gateway still accepts them during the
		// transition, so the guard must still catch them.
		{"denied pro auth token", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/token", true},
		{"denied pro auth keep-alive", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/keep-alive", true},
		{"denied pro auth invalidate", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/invalidate-token", true},
		{"denied pro auth current", http.MethodGet, "https://x.apigw.jamf.com/api/pro/tenant/abc123/auth/current", true},
		{"denied pro oauth token", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/oauth/token", true},
		{"denied with query string", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/token?foo=1", true},
		{"allowed pro resource", http.MethodGet, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/computers", false},
		{"allowed platform resource", http.MethodGet, "https://x.apigw.jamf.com/api/devices/v1/tenant/abc123/devices", false},
		{"method mismatch is allowed", http.MethodGet, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/token", false},
		// Previously asserted as allowed on the grounds that it had no /tenant/
		// segment. That reasoning described every URL the SDK now builds, so the
		// expectation is inverted: a denied suffix is denied regardless of shape.
		{"no tenant segment is still denied", http.MethodPost, "https://x.apigw.jamf.com/api/auth/token", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkDeniedPath(tc.method, tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "path not supported") {
					t.Fatalf("expected path-not-supported rejection, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestDoRefusesDeniedPath(t *testing.T) {
	c, srv, _ := newTestClient(t)
	path := "/api/pro/v1/tenant/abc123/auth/token"
	err := c.Do(context.Background(), http.MethodPost, path, nil, nil)
	if err == nil {
		t.Fatalf("expected error calling denied path, got nil")
	}
	if !strings.Contains(err.Error(), "path not supported") {
		t.Fatalf("expected path-not-supported rejection, got %v", err)
	}
	_ = srv
}

func TestTransportAPIPrefix(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		version   string
		want      string
	}{
		{"devices", "devices", "v1", "/devices/v1"},
		{"device groups", "device-groups", "v1", "/device-groups/v1"},
		{"device actions", "device-actions", "v1", "/device-actions/v1"},
		{"blueprints", "blueprints", "v1", "/blueprints/v1"},
		{"compliance benchmarks", "compliance-benchmarks", "v1", "/compliance-benchmarks/v1"},
		{"slashed namespace", "ddm/report", "v1", "/ddm/report/v1"},
		{"version-less pro", "pro", "", "/pro"},
		{"proclassic has no version", "proclassic", "", "/proclassic"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The scope is not in the path, so the prefix is independent of
			// which tenant the client is configured for. That independence is
			// the property this asserts: a tenant ID must never reach the URL.
			tr := &Transport{}
			WithTenantID("t-should-not-appear")(tr)
			got := tr.APIPrefix(tc.namespace, tc.version)
			if got != tc.want {
				t.Errorf("APIPrefix(%q, %q) = %q, want %q", tc.namespace, tc.version, got, tc.want)
			}
			if strings.Contains(got, "t-should-not-appear") {
				t.Error("the tenant ID leaked into the URL path; it belongs in the X-Tenant-Id header")
			}
		})
	}
}

// TestScopeKindValues pins the numeric value of every ScopeKind. The constants
// are exported and the block used to run off iota, which counts a ConstSpec's
// position rather than the constants before it: adding ScopeOrganization as a
// named zero value ahead of an `iota + 1` run renumbered ScopeTenant to 2 and
// ScopeEnvironment to 3 with nothing to notice it, because every comparison in
// the SDK is symbolic. The block now writes each value out, and this fails if
// another constant is ever inserted at the top.
func TestScopeKindValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind ScopeKind
		want int
	}{
		{"ScopeOrganization", ScopeOrganization, 0},
		{"ScopeTenant", ScopeTenant, 1},
		{"ScopeEnvironment", ScopeEnvironment, 2},
	} {
		if int(tc.kind) != tc.want {
			t.Errorf("%s = %d, want %d — the constants renumbered; write the values out rather than reaching for iota", tc.name, int(tc.kind), tc.want)
		}
	}
}

// TestScopeHeader pins the header each scope kind travels in. Organization
// scope is deliberately absent: the gateway resolves it from the access token
// alone, so there is no header for a client to send.
func TestScopeHeader(t *testing.T) {
	for _, tc := range []struct {
		kind ScopeKind
		want string
	}{
		{ScopeTenant, "X-Tenant-Id"},
		{ScopeEnvironment, "X-Environment-Id"},
		{ScopeKind(0), ""},
		{ScopeKind(99), ""},
	} {
		if got := tc.kind.ScopeHeader(); got != tc.want {
			t.Errorf("ScopeKind(%d).ScopeHeader() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// TestSetScopeHeader covers what execute() stamps on an outbound request.
func TestSetScopeHeader(t *testing.T) {
	tests := []struct {
		name string
		tr   *Transport
		want map[string]string
	}{
		{"tenant scope", &Transport{scopeKind: ScopeTenant, scopeID: "t-123"}, map[string]string{"X-Tenant-Id": "t-123"}},
		{"environment scope", &Transport{scopeKind: ScopeEnvironment, scopeID: "e-456"}, map[string]string{"X-Environment-Id": "e-456"}},
		// No scope configured is not an error: the gateway falls back to the
		// access token, which is how organization-scoped products work.
		{"no scope configured", &Transport{}, nil},
		{"kind set but ID empty", &Transport{scopeKind: ScopeTenant}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://example.invalid/api/pro/v1/buildings", nil)
			if err != nil {
				t.Fatal(err)
			}
			tc.tr.setScopeHeader(req)
			for _, h := range []string{"X-Tenant-Id", "X-Environment-Id"} {
				if got := req.Header.Get(h); got != tc.want[h] {
					t.Errorf("%s = %q, want %q", h, got, tc.want[h])
				}
			}
		})
	}
}

// TestWithEnvironmentID checks the option records an environment scope, and
// that a client carries exactly one scope — the options are alternatives, not
// additive, because the gateway refuses a header that disagrees with the
// credential (403 OWNERSHIP_FORBIDDEN).
func TestWithEnvironmentID(t *testing.T) {
	tr := &Transport{}
	WithEnvironmentID("e-123")(tr)
	if tr.scopeKind != ScopeEnvironment {
		t.Errorf("scopeKind = %v, want ScopeEnvironment", tr.scopeKind)
	}
	if tr.scopeID != "e-123" {
		t.Errorf("scopeID = %q, want %q", tr.scopeID, "e-123")
	}
	if got := tr.TenantID(); got != "" {
		t.Errorf("TenantID() on an environment-scoped client = %q, want empty", got)
	}

	// Last option wins; the client never sends both headers.
	WithTenantID("t-456")(tr)
	if tr.scopeKind != ScopeTenant || tr.scopeID != "t-456" {
		t.Errorf("after WithTenantID: kind=%v id=%q, want ScopeTenant/t-456", tr.scopeKind, tr.scopeID)
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/api/pro/v1/buildings", nil)
	if err != nil {
		t.Fatal(err)
	}
	tr.setScopeHeader(req)
	if got := req.Header.Get("X-Environment-Id"); got != "" {
		t.Errorf("X-Environment-Id = %q after switching to tenant scope, want empty", got)
	}
	if got := req.Header.Get("X-Tenant-Id"); got != "t-456" {
		t.Errorf("X-Tenant-Id = %q, want t-456", got)
	}
}

// TestWithTenantID checks the option records a tenant scope, and that TenantID
// only reports a value back when the scope really is a tenant.
func TestWithTenantID(t *testing.T) {
	tr := &Transport{}
	WithTenantID("t-abc")(tr)
	if tr.scopeKind != ScopeTenant {
		t.Errorf("scopeKind = %v, want ScopeTenant", tr.scopeKind)
	}
	if got := tr.TenantID(); got != "t-abc" {
		t.Errorf("TenantID() = %q, want %q", got, "t-abc")
	}
	env := &Transport{scopeKind: ScopeEnvironment, scopeID: "e-1"}
	if got := env.TenantID(); got != "" {
		t.Errorf("TenantID() on an environment-scoped client = %q, want empty", got)
	}
}
