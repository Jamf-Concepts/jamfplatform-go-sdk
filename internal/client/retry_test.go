// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// --- no-retry-on-4xx regression tests ---
//
// The transport must NOT retry unexpected 4xx responses. Eventual-consistency
// handling is a caller concern; the only mechanical retry that survives is the
// server-instructed 429/Retry-After path (covered below). These guards assert a
// single dispatch and an immediately-surfaced *APIResponseError.

func TestNoRetryOn4xx_DeleteSingleCall(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0) // isolate retry behavior from request pacing
	var calls atomic.Int32
	mux.HandleFunc("/api/eventual", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	})

	start := time.Now()
	err := c.DoExpect(context.Background(), http.MethodDelete, "/api/eventual", nil, http.StatusNoContent, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusBadRequest) {
		t.Fatalf("expected APIResponseError(400), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call (no 4xx retry), got %d", calls.Load())
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected immediate surface, got %v (looks like a backoff loop)", elapsed)
	}
}

func TestNoRetryOn4xx_GetSingleCall(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	var calls atomic.Int32
	mux.HandleFunc("/api/missing", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})

	start := time.Now()
	err := c.Do(context.Background(), http.MethodGet, "/api/missing", nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call (no 4xx retry), got %d", calls.Load())
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected immediate surface, got %v", elapsed)
	}
}

// TestNoRetryOn4xx_Delete404IsError pins the deliberate behavior change: a 404
// on DELETE now surfaces as *APIResponseError(404) rather than being swallowed
// as idempotent-delete success. The "404-on-DELETE == success" judgement is a
// lifecycle semantic the consumer owns.
func TestNoRetryOn4xx_Delete404IsError(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	var calls atomic.Int32
	mux.HandleFunc("/api/gone", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})

	err := c.DoExpect(context.Background(), http.MethodDelete, "/api/gone", nil, http.StatusNoContent, nil)
	if err == nil {
		t.Fatal("expected APIResponseError(404), got nil (404-on-DELETE must no longer be success)")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call, got %d", calls.Load())
	}
}

// --- 429/Retry-After tests (the only surviving mechanical retry) ---

func TestRetryAfter_IntegerSeconds(t *testing.T) {
	c, srv, mux := newTestClient(t)
	var calls atomic.Int32
	mux.HandleFunc("/api/rate", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	start := time.Now()
	err := c.Do(context.Background(), http.MethodGet, "/api/rate", nil, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls (initial + retry), got %d", calls.Load())
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected ~1s wait before retry, got %v", elapsed)
	}
	_ = srv
}

func TestRetryAfter_MissingReturns429(t *testing.T) {
	c, srv, mux := newTestClient(t)
	var calls atomic.Int32
	mux.HandleFunc("/api/rate", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/rate", nil, nil)
	if err == nil {
		t.Fatalf("expected error on 429 without Retry-After, got nil")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusTooManyRequests) {
		t.Fatalf("expected APIResponseError(429), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry without Retry-After), got %d", calls.Load())
	}
	_ = srv
}

func TestRetryAfter_OverCapReturns429(t *testing.T) {
	c, srv, mux := newTestClient(t)
	var calls atomic.Int32
	mux.HandleFunc("/api/rate", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120") // above 60s cap
		w.WriteHeader(http.StatusTooManyRequests)
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/rate", nil, nil)
	if err == nil {
		t.Fatalf("expected error when Retry-After exceeds cap, got nil")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusTooManyRequests) {
		t.Fatalf("expected APIResponseError(429), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry over cap), got %d", calls.Load())
	}
	_ = srv
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Now()
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"5", 5 * time.Second},
		{"-3", 0},
		{"not-a-number", 0},
	}
	for _, tc := range cases {
		if got := parseRetryAfter(tc.in, now); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	// HTTP-date in the future
	future := now.Add(7 * time.Second).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(future, now)
	if got < 6*time.Second || got > 8*time.Second {
		t.Errorf("parseRetryAfter(future date) = %v, want ~7s", got)
	}
	// HTTP-date in the past — zero
	past := now.Add(-1 * time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past, now); got != 0 {
		t.Errorf("parseRetryAfter(past date) = %v, want 0", got)
	}
}
