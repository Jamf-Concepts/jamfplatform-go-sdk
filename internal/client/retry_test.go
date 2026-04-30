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

// --- 4xx retry tests ---

func TestRetryOn4xx_DefaultOff(t *testing.T) {
	c, _, mux := newTestClient(t)
	var calls atomic.Int32
	mux.HandleFunc("/api/eventual", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	})

	err := c.Do(context.Background(), http.MethodDelete, "/api/eventual", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call (no retry by default), got %d", calls.Load())
	}
}

func TestRetryOn4xx_SuccessFirstAttempt(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.retryOn4xx = true
	var calls atomic.Int32
	mux.HandleFunc("/api/ok", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/ok", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry on success), got %d", calls.Load())
	}
}

func TestRetryOn4xx_RetryThenSucceed(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.retryOn4xx = true
	var calls atomic.Int32
	mux.HandleFunc("/api/eventual", func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	start := time.Now()
	err := c.DoExpect(context.Background(), http.MethodDelete, "/api/eventual", nil, http.StatusNoContent, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (initial + one retry), got %d", calls.Load())
	}
	if elapsed < 1500*time.Millisecond {
		t.Errorf("expected ~2s backoff before retry, got %v", elapsed)
	}
}

func TestRetryOn4xx_401NotRetried(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.retryOn4xx = true
	var calls atomic.Int32
	mux.HandleFunc("/api/auth", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/auth", nil, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusUnauthorized) {
		t.Fatalf("expected APIResponseError(401), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call (401 not retried), got %d", calls.Load())
	}
}

func TestRetryOn4xx_403NotRetried(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.retryOn4xx = true
	var calls atomic.Int32
	mux.HandleFunc("/api/forbidden", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/forbidden", nil, nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusForbidden) {
		t.Fatalf("expected APIResponseError(403), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call (403 not retried), got %d", calls.Load())
	}
}

func TestRetryOn4xx_5xxNotRetried(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.retryOn4xx = true
	var calls atomic.Int32
	mux.HandleFunc("/api/server-error", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/server-error", nil, nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusInternalServerError) {
		t.Fatalf("expected APIResponseError(500), got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call (5xx not retried), got %d", calls.Load())
	}
}

func TestRetryOn4xx_ContextCancelled(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.retryOn4xx = true
	mux.HandleFunc("/api/eventual", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.Do(ctx, http.MethodDelete, "/api/eventual", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIResponseError, got: %v", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

func TestRetryOn4xx_ExponentialBackoff(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.retryOn4xx = true
	var calls atomic.Int32
	// Fail with 400 twice, succeed on the third call.
	mux.HandleFunc("/api/multi", func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	start := time.Now()
	err := c.DoExpect(context.Background(), http.MethodDelete, "/api/multi", nil, http.StatusNoContent, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after 2 retries, got: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
	// First backoff 2s, second backoff 4s → total ≥ 6s.
	if elapsed < 5*time.Second {
		t.Errorf("expected ≥6s total (2s+4s backoffs), got %v", elapsed)
	}
}

func TestRetryOn4xx_WithOption(t *testing.T) {
	srv, _ := newTestServer(t)
	c := NewTransportWithUserAgent(srv.URL, "id", "secret", "test", WithRetryOn4xx(true))
	if !c.retryOn4xx {
		t.Error("retryOn4xx should be true when WithRetryOn4xx(true) is applied")
	}
}

func TestIs4xxRetryable(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{400, true},
		{404, true},
		{409, true},
		{422, true},
		{499, true},
		{401, false},
		{403, false},
		{200, false},
		{301, false},
		{500, false},
		{502, false},
	}
	for _, tc := range cases {
		if got := is4xxRetryable(tc.status); got != tc.want {
			t.Errorf("is4xxRetryable(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

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
