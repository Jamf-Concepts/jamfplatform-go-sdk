// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

// TestCookieJar_StickySession verifies Set-Cookie on a first response is
// echoed back on subsequent requests — the behavior Jamf Cloud depends on
// for session affinity to the same app node.
func TestCookieJar_StickySession(t *testing.T) {
	c, srv, mux := newTestClient(t)

	const cookieName = "JSESSIONID"
	const cookieValue = "node-a-1234"

	var secondCall atomic.Bool

	mux.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		if secondCall.Load() {
			cookie, err := r.Cookie(cookieName)
			if err != nil {
				t.Errorf("second call missing cookie %q: %v", cookieName, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if cookie.Value != cookieValue {
				t.Errorf("cookie value = %q, want %q", cookie.Value, cookieValue)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: cookieValue, Path: "/"})
		secondCall.Store(true)
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	if err := c.Do(ctx, http.MethodGet, "/api/test", nil, nil); err != nil {
		t.Fatalf("first Do: %v", err)
	}
	if err := c.Do(ctx, http.MethodGet, "/api/test", nil, nil); err != nil {
		t.Fatalf("second Do: %v", err)
	}
	if !secondCall.Load() {
		t.Fatal("second call never observed")
	}
	_ = srv
}
