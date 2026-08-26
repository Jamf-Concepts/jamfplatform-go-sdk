// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// proxyStub impersonates a reverse proxy fronting the gateway on a path prefix,
// serving the token endpoint and one API endpoint and recording what each
// arrived with. It is the public-package twin of internal/client's fakeProxy;
// duplicated rather than exported, because the point of these tests is to enter
// through the public surface only.
func proxyStub(t *testing.T) (baseURL string, seen func() map[string]http.Header) {
	t.Helper()

	var mu sync.Mutex
	got := map[string]http.Header{}
	record := func(phase string, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		got[phase] = r.Header.Clone()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxypath/auth/token", func(w http.ResponseWriter, r *http.Request) {
		record("token", r)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-abc123",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}); err != nil {
			t.Errorf("writing token response: %v", err)
		}
	})
	mux.HandleFunc("/proxypath/api/pro/v1/buildings", func(w http.ResponseWriter, r *http.Request) {
		record("api", r)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"totalCount":0,"results":[]}`)); err != nil {
			t.Errorf("writing api response: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + "/proxypath", func() map[string]http.Header {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

// The proxy options have to survive the trip through NewClient's option
// translation, which is the only path a consumer takes. The behaviour itself is
// covered in internal/client; what is asserted here is the wiring — deleting
// either translation block in NewClient leaves those tests green, because they
// construct the transport with the internal options directly.
func TestWithHeaders_ReachesTheWireThroughNewClient(t *testing.T) {
	t.Parallel()

	base, seen := proxyStub(t)
	c := NewClient(base, "cid", "csecret",
		WithMinRequestInterval(0),
		WithTenantID("real-tenant"),
		WithHeaders(http.Header{
			"Authorization": {"Basic c3ZjOnMzY3JldA=="},
			"X-Routing-Tag": {"corp-egress"},
			"X-Tenant-Id":   {"hijacked"},
		}),
		WithAuthorizationHeaderName("X-Jamf-Bearer"),
	)

	var out map[string]any
	if err := c.Transport().Do(context.Background(), http.MethodGet, "/api/pro/v1/buildings", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}

	got := seen()
	tok, ok := got["token"]
	if !ok {
		t.Fatal("token phase never reached the proxy")
	}
	if v := tok.Get("X-Routing-Tag"); v != "corp-egress" {
		t.Errorf("token phase: X-Routing-Tag = %q, want %q — WithHeaders must reach the token exchange", v, "corp-egress")
	}

	api, ok := got["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.Get("Authorization"); v != "Basic c3ZjOnMzY3JldA==" {
		t.Errorf("Authorization = %q, want the caller's proxy credential", v)
	}
	if v := api.Get("X-Jamf-Bearer"); v != "Bearer tok-abc123" {
		t.Errorf("X-Jamf-Bearer = %q, want the relocated bearer — WithAuthorizationHeaderName was not wired through", v)
	}
	if v := api.Get("X-Routing-Tag"); v != "corp-egress" {
		t.Errorf("X-Routing-Tag = %q, want %q", v, "corp-egress")
	}
	if v := api.Get("X-Tenant-Id"); v != "real-tenant" {
		t.Errorf("X-Tenant-Id = %q, want the configured scope %q — the reserved-header guard must apply to the public option too", v, "real-tenant")
	}
}
