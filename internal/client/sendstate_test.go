// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// dropAfterReading is a handler that models the failure send tracking exists
// for: the request arrives complete, the server may have acted on it, and the
// connection is reset before any response. The body is drained first so the
// client's write has finished, and the short sleep lets its WroteRequest hook
// run before the reset reaches the read side — without both the test would
// exercise a failed write, which is deliberately retryable.
func dropAfterReading(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	time.Sleep(50 * time.Millisecond)
	if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
		_ = conn.Close()
	}
}

func TestRetryableTransportError(t *testing.T) {
	t.Parallel()

	reset := errors.New("read: connection reset by peer")
	timeout := &net.DNSError{IsTimeout: true}

	sent := func(method string) *sendState {
		st := &sendState{method: method}
		st.sent.Store(true)
		return st
	}
	cases := []struct {
		name string
		st   *sendState
		err  error
		want bool
	}{
		{"untracked request keeps the old behaviour", nil, reset, true},
		{"never fully written, POST", &sendState{method: http.MethodPost}, reset, true},
		{"never fully written, timeout", &sendState{method: http.MethodPost}, timeout, true},
		{"sent, GET reset", sent(http.MethodGet), reset, true},
		{"sent, PUT reset", sent(http.MethodPut), reset, true},
		{"sent, DELETE reset", sent(http.MethodDelete), reset, true},
		{"sent, POST reset", sent(http.MethodPost), reset, false},
		{"sent, PATCH reset", sent(http.MethodPatch), reset, false},
		{"sent, GET timeout", sent(http.MethodGet), timeout, false},
		{"sent, PUT timeout", sent(http.MethodPut), timeout, false},
		{"sent, timeout wrapped deep in the chain", sent(http.MethodGet), errors.Join(errors.New("outer"), timeout), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := retryableTransportError(tc.st, tc.err); got != tc.want {
				t.Errorf("retryableTransportError = %v, want %v", got, tc.want)
			}
		})
	}
}

// failingTransport returns err without ever invoking the request trace, as a
// dial failure would.
type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

// The flag is per attempt. If a retry that fails before sending were judged by
// the attempt before it, one send would permanently disable retries for the
// request.
func TestSendTrackingTransport_ClearsFlagForEachAttempt(t *testing.T) {
	t.Parallel()

	ctx, st := withSendState(context.Background(), http.MethodPost)
	st.sent.Store(true) // as left behind by an earlier attempt
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://example.invalid/", nil)

	tr := &sendTrackingTransport{base: failingTransport{err: errors.New("dial tcp: refused")}}
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("expected the stub's error")
	}
	if st.sent.Load() {
		t.Error("sent still set after an attempt that never wrote — a retry would be judged by the previous attempt")
	}
}

func TestSendTrackingTransport_SetsFlagOnceRequestIsWritten(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	t.Cleanup(srv.Close)

	ctx, st := withSendState(context.Background(), http.MethodPost)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader("payload"))
	resp, err := (&sendTrackingTransport{base: newTunedTransport()}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !st.sent.Load() {
		t.Error("sent not set after a request that was written in full")
	}
}

// A request with no sendState — a token exchange, a raw request on
// Transport.HTTPClient — must pass straight through.
func TestSendTrackingTransport_PassesThroughUntrackedRequests(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/", nil)
	want := errors.New("boom")
	if _, err := (&sendTrackingTransport{base: failingTransport{err: want}}).RoundTrip(req); !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

// The behaviour the whole change is for: a write whose request was fully sent
// and then lost its connection must not be sent again. Before send tracking,
// retryablehttp's default policy re-sent a POST here on every attempt.
func TestRetryAfterSend_NonIdempotentMethodsAreNotReplayed(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			c, _, mux := newTestClient(t)
			c.throttle.setInterval(0)
			shrinkRetryWaits(c, time.Millisecond, 5*time.Millisecond)
			var calls atomic.Int32
			mux.HandleFunc("/api/write", func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				dropAfterReading(w, r)
			})

			// A PATCH body has no default Content-Type and is refused before
			// sending, which would pass the count check below for the wrong
			// reason; state the type so the request is actually written.
			err := c.DoWithContentType(context.Background(), method, "/api/write",
				map[string]string{"k": "v"}, "application/json", http.StatusOK, nil)
			if err == nil {
				t.Fatal("expected a transport error")
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("server saw %d requests, want 1 — a %s that was sent in full was replayed", got, method)
			}
		})
	}
}

// The other half: idempotent methods still recover from a dropped connection.
// Go's own transport may add a transparent retry on a reused connection, so
// this asserts "more than one" rather than a count.
func TestRetryAfterSend_IdempotentMethodsStillRetried(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			c, _, mux := newTestClient(t)
			c.throttle.setInterval(0)
			shrinkRetryWaits(c, time.Millisecond, 5*time.Millisecond)
			var calls atomic.Int32
			mux.HandleFunc("/api/idem", func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) < 3 {
					dropAfterReading(w, r)
					return
				}
				w.WriteHeader(http.StatusOK)
			})

			if err := c.Do(context.Background(), method, "/api/idem", nil, nil); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if got := calls.Load(); got < 3 {
				t.Errorf("server saw %d requests, want at least 3 (two drops, then success)", got)
			}
		})
	}
}

// A timeout after the send is the caller's bound, so it is surfaced rather than
// multiplied by the attempt count — for an idempotent method too.
func TestRetryAfterSend_TimeoutIsNotRetriedForAnyMethod(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			c, _, mux := newTestClient(t)
			c.throttle.setInterval(0)
			shrinkRetryWaits(c, time.Millisecond, 5*time.Millisecond)
			unwrapTuned(t, c.baseClient.Transport).ResponseHeaderTimeout = 100 * time.Millisecond

			var calls atomic.Int32
			mux.HandleFunc("/api/slow", func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				time.Sleep(400 * time.Millisecond)
			})

			err := c.Do(context.Background(), method, "/api/slow", nil, nil)
			if err == nil {
				t.Fatal("expected a header timeout")
			}
			if !isTimeout(err) {
				t.Errorf("err = %v, want a timeout", err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("server saw %d requests, want 1 — a post-send timeout was retried", got)
			}
		})
	}
}

// Failing before anything is written stays retryable on every method: nothing
// reached the server, so a POST cannot have been applied. Counted through the
// logger, which sees one line per attempt, because there is no server to count.
func TestRetryBeforeSend_PostIsStillRetried(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens here now: every dial is refused

	c := NewTransport("http://"+addr, "id", "secret")
	c.throttle.setInterval(0)
	shrinkRetryWaits(c, time.Millisecond, 5*time.Millisecond)
	var attempts atomic.Int32
	c.SetLogger(&testLogger{onRequest: func() { attempts.Add(1) }})

	if err := c.Do(context.Background(), http.MethodPost, "/api/create", nil, nil); err == nil {
		t.Fatal("expected a connection error")
	}
	if got, want := int(attempts.Load()), retryMax+1; got != want {
		t.Errorf("POST attempted %d times against a refused connection, want %d", got, want)
	}
}

// A caller-supplied client must get the same protection: it is the path
// consumers that bring their own transport are on.
func TestRetryAfterSend_AppliesToCallerSuppliedClient(t *testing.T) {
	srv, mux := newTestServer(t)
	c := NewTransportWithUserAgent(srv.URL, "id", "secret", "ua/1",
		WithHTTPClient(&http.Client{Transport: &http.Transport{}}),
		WithMinRequestInterval(0),
	)
	shrinkRetryWaits(c, time.Millisecond, 5*time.Millisecond)
	var calls atomic.Int32
	mux.HandleFunc("/api/write", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		dropAfterReading(w, r)
	})

	if err := c.Do(context.Background(), http.MethodPost, "/api/write", nil, nil); err == nil {
		t.Fatal("expected a transport error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("server saw %d requests, want 1 — the caller's transport was not tracked", got)
	}
}

func TestDoMultipart_NoReplayAfterSendForPost(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	var calls atomic.Int32
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		dropAfterReading(w, r)
	})

	err := c.DoMultipart(context.Background(), http.MethodPost, "/api/upload", []MultipartField{
		{Name: "file", Filename: "pkg.bin", Content: bytes.NewReader([]byte(strings.Repeat("D", 2048)))},
	}, http.StatusCreated, nil)
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("server saw %d uploads, want 1 — a POST sent in full was re-streamed", got)
	}
}

// PUT is idempotent, so the same drop is recovered by re-streaming from a
// rewound source. This waits out one real backoff because DoMultipart's loop
// uses the package constants rather than the client's retry settings.
func TestDoMultipart_ReplaysAfterSendForPut(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	var calls atomic.Int32
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			dropAfterReading(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusCreated)
	})

	err := c.DoMultipart(context.Background(), http.MethodPut, "/api/upload", []MultipartField{
		{Name: "file", Filename: "pkg.bin", Content: bytes.NewReader([]byte(strings.Repeat("D", 2048)))},
	}, http.StatusCreated, nil)
	if err != nil {
		t.Fatalf("DoMultipart: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("server saw %d uploads, want 2 (one dropped, one retried)", got)
	}
}
