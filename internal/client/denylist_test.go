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
