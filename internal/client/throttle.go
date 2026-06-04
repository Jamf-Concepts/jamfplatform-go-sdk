// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// defaultMinRequestInterval is the minimum wall-clock spacing applied between
// the start of consecutive outbound requests unless overridden via
// WithMinRequestInterval. 100ms paces the parallel request burst Terraform
// fans out (default -parallelism=10) without materially slowing a serial
// workload.
const defaultMinRequestInterval = 100 * time.Millisecond

// requestThrottle enforces a minimum elapsed time between the start of
// consecutive outbound requests across the whole Transport. It is shared by
// every goroutine dispatching through the client, so it must be goroutine-safe;
// it serialises request starts through a single reserved-slot timestamp.
type requestThrottle struct {
	mu       sync.Mutex
	interval time.Duration // <= 0 disables the gate
	last     time.Time     // start time reserved for the most recent caller
}

// newRequestThrottle returns a throttle with the given minimum interval.
func newRequestThrottle(interval time.Duration) *requestThrottle {
	return &requestThrottle{interval: interval}
}

// setInterval updates the minimum interval. Only called during client
// construction (single-threaded), before any request is dispatched, so it
// needs no synchronisation against wait.
func (t *requestThrottle) setInterval(d time.Duration) {
	t.interval = d
}

// wait blocks until the caller's reserved request-start slot is reached, then
// returns. Concurrent callers queue in arrival order: each reserves the next
// slot under the lock, then sleeps (outside the lock) until it elapses. A
// cancelled ctx aborts the wait and returns ctx.Err(); the reserved slot is
// not rolled back, which at worst spaces the next caller slightly further out.
func (t *requestThrottle) wait(ctx context.Context) error {
	if t.interval <= 0 {
		return nil
	}

	t.mu.Lock()
	now := time.Now()
	wait := t.interval - now.Sub(t.last)
	// Reserve this slot regardless of wait so concurrent callers queue in order.
	if wait > 0 {
		t.last = now.Add(wait)
	} else {
		t.last = now
		wait = 0
	}
	t.mu.Unlock()

	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// throttleTransport is an http.RoundTripper that paces every outbound request
// through a shared requestThrottle before delegating to the base transport.
// Installed at the base of the transport chain so it covers API calls,
// multipart uploads, and OAuth2 token fetches alike.
type throttleTransport struct {
	base     http.RoundTripper
	throttle *requestThrottle
}

// RoundTrip waits for the throttle, then delegates. A ctx cancelled during the
// wait surfaces as ctx.Err() without a network round-trip.
func (t *throttleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.throttle.wait(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}
