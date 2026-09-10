// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// DoWithOptions carries the four per-request dimensions together, which is
// the reason it replaced a wrapper per combination. Each is asserted against
// the request the server actually received, not against the options struct.
func TestDoWithOptions_CarriesEveryDimension(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.scopeKind, c.scopeID = ScopeTenant, "t-test"

	var got *http.Request
	mux.HandleFunc("/api/things/1", func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})

	headers := http.Header{}
	headers.Set("If-Match", "7")
	err := c.DoWithOptions(context.Background(), http.MethodPatch, "/api/things/1",
		map[string]string{"name": "x"},
		RequestOptions{ExpectedStatus: http.StatusNoContent, ContentType: "application/json", Headers: headers},
		nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("handler never ran")
	}
	if v := got.Header.Get("If-Match"); v != "7" {
		t.Errorf("If-Match = %q, want %q", v, "7")
	}
	if v := got.Header.Get("Content-Type"); v != "application/json" {
		t.Errorf("Content-Type = %q, want %q — a PATCH would otherwise default to application/merge-patch+json", v, "application/json")
	}
	// The scope header must survive: doRequestFull applies extraHeaders after
	// setScopeHeader, so this is the assertion that a caller-supplied header
	// map has not displaced the scope.
	if v := got.Header.Get("X-Tenant-Id"); v != "t-test" {
		t.Errorf("X-Tenant-Id = %q, want %q — extra headers displaced the scope", v, "t-test")
	}
}

// A zero ExpectedStatus means 200, so the options literal for a plain read
// stays empty rather than restating the default.
func TestDoWithOptions_ZeroExpectedStatusMeans200(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/things", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "hello"})
	})

	var result struct{ Name string }
	if err := c.DoWithOptions(context.Background(), http.MethodGet, "/api/things", nil, RequestOptions{}, &result); err != nil {
		t.Fatal(err)
	}
	if result.Name != "hello" {
		t.Errorf("Name = %q, want %q", result.Name, "hello")
	}
}

// NoRetry has to be asserted against the number of requests that reach the
// server, not against the options struct. It is the one dimension whose effect
// is invisible in the request itself — it swaps the http.Client — so a test
// that only inspects headers and status cannot see it, and deleting the branch
// from DoWithOptions passed the whole package before this existed.
//
// **The method has to be PUT.** isRetryableWriteStatus never retries a 5xx on
// a POST or PATCH, so NoRetry is unobservable on either: the same test written
// against a PATCH passes with the branch deleted and pins nothing. PUT is also
// the real case — the V3 prestage endpoints carry an optimistic-lock field in
// the body that a successful-but-500ing write has already advanced, so a blind
// retry replays a stale precondition and turns a committed write into a 409.
// The mirror test below is what keeps this one honest.
func TestDoWithOptions_NoRetrySendsExactlyOneRequest(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	shrinkRetryWaits(c, 2*time.Millisecond, 5*time.Millisecond)

	var calls atomic.Int32
	mux.HandleFunc("/api/versionlocked", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	headers := http.Header{}
	headers.Set("If-Match", "7")
	err := c.DoWithOptions(context.Background(), http.MethodPut, "/api/versionlocked", nil,
		RequestOptions{ExpectedStatus: http.StatusNoContent, Headers: headers, NoRetry: true}, nil)
	if err == nil {
		t.Fatal("want an error for a persistent 500, got nil")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusInternalServerError) {
		t.Fatalf("want APIResponseError(500), got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("server saw %d requests, want exactly 1 — NoRetry did not reach the transport, so the precondition was replayed", got)
	}
}

// The mirror of the above, and the reason it is not decoration: it proves the
// default path retries this exact method and status, so the single request
// asserted above is a statement about NoRetry rather than about the retry
// policy declining to retry the method at all. Written first against a PATCH,
// this test failed — which is how the PATCH version above was caught being a
// no-op.
func TestDoWithOptions_WithoutNoRetryTheRequestIsRetried(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	shrinkRetryWaits(c, 2*time.Millisecond, 5*time.Millisecond)
	c.retry.RetryMax = 2

	var calls atomic.Int32
	mux.HandleFunc("/api/flaky", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	err := c.DoWithOptions(context.Background(), http.MethodPut, "/api/flaky", nil,
		RequestOptions{ExpectedStatus: http.StatusNoContent}, nil)
	if err == nil {
		t.Fatal("want an error for a persistent 500, got nil")
	}
	if got := calls.Load(); got < 2 {
		t.Errorf("server saw %d requests, want more than 1 — the default path is not retrying, so the NoRetry assertion above proves nothing", got)
	}
}

// An unexpected status is still an *APIResponseError with the body decoded —
// the general entry point must not lose the error surface the named ones have.
func TestDoWithOptions_UnexpectedStatusSurfacesAPIError(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/things/1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"httpStatus": 409,
			"traceId":    "trace-1",
			"errors": []map[string]string{{
				"code": "POLICY_VERSION_CONFLICT", "description": "stale precondition",
			}},
		})
	})

	err := c.DoWithOptions(context.Background(), http.MethodPatch, "/api/things/1", nil,
		RequestOptions{ExpectedStatus: http.StatusNoContent}, nil)
	if err == nil {
		t.Fatal("want an error for 409, got nil")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *APIResponseError: %v", err)
	}
	if !apiErr.HasStatus(http.StatusConflict) {
		t.Errorf("status = %d, want 409", apiErr.StatusCode)
	}
	if codes := apiErr.Details(); len(codes) == 0 || codes[0].Code != "POLICY_VERSION_CONFLICT" {
		t.Errorf("Details() = %+v, want the POLICY_VERSION_CONFLICT code", codes)
	}
}
