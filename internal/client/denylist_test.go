// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net/http"
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
				if !errors.Is(err, ErrPathNotSupported) {
					t.Fatalf("expected ErrPathNotSupported, got %v", err)
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
	if !errors.Is(err, ErrPathNotSupported) {
		t.Fatalf("expected ErrPathNotSupported, got %v", err)
	}
	_ = srv
}
