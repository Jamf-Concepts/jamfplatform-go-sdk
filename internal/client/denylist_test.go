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
		{"denied pro auth token", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/token", true},
		{"denied pro auth keep-alive", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/keep-alive", true},
		{"denied pro auth invalidate", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/invalidate-token", true},
		{"denied pro auth current", http.MethodGet, "https://x.apigw.jamf.com/api/pro/tenant/abc123/auth/current", true},
		{"denied pro oauth token", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/oauth/token", true},
		{"denied with query string", http.MethodPost, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/token?foo=1", true},
		{"allowed pro resource", http.MethodGet, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/computers", false},
		{"allowed platform resource", http.MethodGet, "https://x.apigw.jamf.com/api/devices/v1/tenant/abc123/devices", false},
		{"method mismatch is allowed", http.MethodGet, "https://x.apigw.jamf.com/api/pro/v1/tenant/abc123/auth/token", false},
		{"no tenant segment", http.MethodPost, "https://x.apigw.jamf.com/api/auth/token", false},
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

func TestTransportTenantPrefix(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		namespace string
		version   string
		want      string
	}{
		{"devices", "e77c1408-10c8-4007-b177-abc9157fbcaa", "devices", "v1", "/api/devices/v1/tenant/e77c1408-10c8-4007-b177-abc9157fbcaa"},
		{"device groups", "t-123", "device-groups", "v1", "/api/device-groups/v1/tenant/t-123"},
		{"device actions", "t-123", "device-actions", "v1", "/api/device-actions/v1/tenant/t-123"},
		{"blueprints", "t-abc", "blueprints", "v1", "/api/blueprints/v1/tenant/t-abc"},
		{"compliance benchmarks", "t-abc", "compliance-benchmarks", "v1", "/api/compliance-benchmarks/v1/tenant/t-abc"},
		{"version-less pro", "t-abc", "pro", "", "/api/pro/tenant/t-abc"},
		{"proclassic has no version", "t-abc", "proclassic", "", "/api/proclassic/tenant/t-abc"},
		// Security Cloud is the one namespace whose version follows the tenant;
		// see tenantFirstNamespaces for why. Its sub-namespaces inherit the
		// ordering, and a versionless call is unaffected because there is no
		// version segment to order.
		{"securitycloud is tenant-first", "t-jsc", "securitycloud", "v1", "/api/securitycloud/tenant/t-jsc/v1"},
		{"securitycloud v2 is tenant-first too", "t-jsc", "securitycloud", "v2", "/api/securitycloud/tenant/t-jsc/v2"},
		{"a securitycloud sub-namespace inherits the ordering", "t-jsc", "securitycloud/uem-connect", "v1", "/api/securitycloud/uem-connect/tenant/t-jsc/v1"},
		{"versionless securitycloud is unchanged", "t-jsc", "securitycloud", "", "/api/securitycloud/tenant/t-jsc"},
		// A namespace that merely starts with the same letters must not match.
		{"securitycloud-devices is not securitycloud", "t-abc", "securitycloud-devices", "v1", "/api/securitycloud-devices/v1/tenant/t-abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &Transport{tenantID: tc.tenantID}
			if got := tr.TenantPrefix(tc.namespace, tc.version); got != tc.want {
				t.Errorf("TenantPrefix() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTransportTenantPrefixNamespaceOverride covers the case a customer holding
// both Jamf Pro and Jamf Security Cloud hits: two products, two tenant IDs, one
// Client. A regression here sends Security Cloud calls to the Jamf Pro tenant,
// which answers 403 OWNERSHIP_FORBIDDEN rather than anything that reads like a
// wrong-tenant bug.
func TestTransportTenantPrefixNamespaceOverride(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		namespace string
		version   string
		want      string
	}{
		{
			name:      "override applies to its own namespace",
			overrides: map[string]string{"securitycloud": "jsc-tenant"},
			namespace: "securitycloud",
			version:   "v1",
			want:      "/api/securitycloud/tenant/jsc-tenant/v1",
		},
		{
			name:      "other namespaces keep the client-wide tenant",
			overrides: map[string]string{"securitycloud": "jsc-tenant"},
			namespace: "pro",
			version:   "v1",
			want:      "/api/pro/v1/tenant/pro-tenant",
		},
		{
			name:      "override reaches a slashed sub-namespace via its first segment",
			overrides: map[string]string{"securitycloud": "jsc-tenant"},
			namespace: "securitycloud/uem-connect",
			version:   "",
			want:      "/api/securitycloud/uem-connect/tenant/jsc-tenant",
		},
		{
			name:      "an exact slashed key wins over its first segment",
			overrides: map[string]string{"ddm": "wrong", "ddm/report": "ddm-tenant"},
			namespace: "ddm/report",
			version:   "v1",
			want:      "/api/ddm/report/v1/tenant/ddm-tenant",
		},
		{
			name:      "no overrides configured",
			namespace: "securitycloud",
			version:   "v1",
			want:      "/api/securitycloud/tenant/pro-tenant/v1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &Transport{tenantID: "pro-tenant", nsTenantIDs: tc.overrides}
			if got := tr.TenantPrefix(tc.namespace, tc.version); got != tc.want {
				t.Errorf("TenantPrefix(%q, %q) = %q, want %q", tc.namespace, tc.version, got, tc.want)
			}
		})
	}
}

// TestWithNamespaceTenantID checks the option ignores empty inputs rather than
// registering an override that would build /tenant/ with no ID.
func TestWithNamespaceTenantID(t *testing.T) {
	tr := &Transport{tenantID: "pro-tenant"}
	WithNamespaceTenantID("securitycloud", "")(tr)
	WithNamespaceTenantID("", "jsc-tenant")(tr)
	if len(tr.nsTenantIDs) != 0 {
		t.Errorf("empty namespace or tenant ID registered an override: %v", tr.nsTenantIDs)
	}
	WithNamespaceTenantID("securitycloud", "jsc-tenant")(tr)
	if got := tr.TenantPrefix("securitycloud", "v1"); got != "/api/securitycloud/tenant/jsc-tenant/v1" {
		t.Errorf("after WithNamespaceTenantID, TenantPrefix() = %q", got)
	}
}
