// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The SDK has no default response-header timeout. One that fires regardless of
// the caller's context overrides it, which cut a smart-group update at 60s under
// a Terraform `update = "10m"`. The context is the request deadline.
func TestTunedTransport_HasNoDefaultResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	if got := newTunedTransport().ResponseHeaderTimeout; got != 0 {
		t.Errorf("ResponseHeaderTimeout = %v, want 0 — a default bound overrides the caller's context deadline", got)
	}

	c := NewTransport("https://example.invalid", "id", "secret")
	if got := unwrapTuned(t, c.baseClient.Transport).ResponseHeaderTimeout; got != 0 {
		t.Errorf("client built with no options has ResponseHeaderTimeout = %v, want 0", got)
	}
}

func TestTunedTransport_ConfiguresHTTP2HealthCheck(t *testing.T) {
	t.Parallel()

	h2 := newTunedTransport().HTTP2
	if h2 == nil {
		t.Fatal("HTTP2 config is nil — a connection that dies after a request is sent is never noticed")
	}
	if h2.SendPingTimeout != http2PingAfter || h2.PingTimeout != http2PingTimeout {
		t.Errorf("HTTP2 pings = %v/%v, want %v/%v", h2.SendPingTimeout, h2.PingTimeout, http2PingAfter, http2PingTimeout)
	}
}

func TestWithResponseHeaderTimeout_BoundsASlowResponse(t *testing.T) {
	srv, mux := newTestServer(t)
	c := NewTransportWithUserAgent(srv.URL, "id", "secret", "ua/1",
		WithResponseHeaderTimeout(100*time.Millisecond),
		WithMinRequestInterval(0),
	)

	var calls atomic.Int32
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(500 * time.Millisecond)
	})

	start := time.Now()
	err := c.Do(context.Background(), http.MethodPut, "/api/slow", nil, nil)
	elapsed := time.Since(start)
	if err == nil || !isTimeout(err) {
		t.Fatalf("err = %v, want a header timeout", err)
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("returned after %v, want about the 100ms bound", elapsed)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("server saw %d requests, want 1 — the bound the caller chose was retried", got)
	}
}

// With nothing configured a slow response simply succeeds; the bound is opt-in.
func TestResponseHeaderTimeout_NotSetByDefault(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	if err := c.Do(context.Background(), http.MethodGet, "/api/slow", nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestWithResponseHeaderTimeout_NonPositiveMeansNone(t *testing.T) {
	t.Parallel()

	for _, d := range []time.Duration{0, -time.Second} {
		c := NewTransportWithUserAgent("https://example.invalid", "id", "secret", "ua/1", WithResponseHeaderTimeout(d))
		if got := unwrapTuned(t, c.baseClient.Transport).ResponseHeaderTimeout; got != 0 {
			t.Errorf("WithResponseHeaderTimeout(%v) left ResponseHeaderTimeout = %v, want 0", d, got)
		}
	}
}

// Multipart uploads ride the HTTP/1.1 twin, so the bound has to reach it too or
// an upload would wait on response headers forever while everything else is capped.
func TestWithResponseHeaderTimeout_CoversTheHTTP1Transport(t *testing.T) {
	t.Parallel()

	c := NewTransportWithUserAgent("https://example.invalid", "id", "secret", "ua/1", WithResponseHeaderTimeout(7*time.Second))
	if len(c.tuned) != 2 {
		t.Fatalf("tuned transports = %d, want 2", len(c.tuned))
	}
	for i, tr := range c.tuned {
		if tr.ResponseHeaderTimeout != 7*time.Second {
			t.Errorf("tuned[%d].ResponseHeaderTimeout = %v, want 7s", i, tr.ResponseHeaderTimeout)
		}
	}
}

// SetUserAgent builds a new transport; the opt-in bound must come with it, or a
// caller who set one silently loses it the moment they change the user agent.
func TestWithResponseHeaderTimeout_SurvivesSetUserAgent(t *testing.T) {
	t.Parallel()

	c := NewTransportWithUserAgent("https://example.invalid", "id", "secret", "ua/1", WithResponseHeaderTimeout(7*time.Second))
	c.SetUserAgent("other/1")
	if got := unwrapTuned(t, c.baseClient.Transport).ResponseHeaderTimeout; got != 7*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v after SetUserAgent, want 7s", got)
	}
}

// The option cannot reach a caller-supplied client's transport, and must not try:
// mutating it would change a transport the caller still owns. It says so in the
// log instead of failing, in whichever order the options arrive.
func TestWithResponseHeaderTimeout_IgnoredWithCallerSuppliedClient(t *testing.T) {
	cases := map[string][]Option{
		"option first": {WithResponseHeaderTimeout(time.Second), WithHTTPClient(&http.Client{Transport: &http.Transport{}})},
		"option last":  {WithHTTPClient(&http.Client{Transport: &http.Transport{}}), WithResponseHeaderTimeout(time.Second)},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })

			c := NewTransportWithUserAgent("https://example.invalid", "id", "secret", "ua/1", opts...)
			if c.tuned != nil {
				t.Error("tuned transport still referenced after WithHTTPClient replaced it")
			}
			if !strings.Contains(buf.String(), "WithResponseHeaderTimeout: ignored") {
				t.Errorf("no log line explaining the option was ignored; log = %q", buf.String())
			}
		})
	}
}

// blackholeProxy forwards TCP until hole is set, then keeps the connection open
// and swallows every byte in both directions: a path that has silently died.
func blackholeProxy(t *testing.T, backend string, hole *atomic.Bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	pipe := func(dst, src net.Conn) {
		buf := make([]byte, 32<<10)
		for {
			n, err := src.Read(buf)
			if n > 0 && !hole.Load() {
				_, _ = dst.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			b, err := net.Dial("tcp", backend)
			if err != nil {
				_ = c.Close()
				continue
			}
			go pipe(b, c)
			go pipe(c, b)
		}
	}()
	return ln.Addr().String()
}

// What replaces the header timeout: a connection that dies after the request was
// sent is noticed by the h2 health check, and a slow-but-alive response is not
// interrupted. The transport is the SDK's own, with only the two ping durations
// shortened so the test does not wait out 45 seconds; it guards against the
// config being silently ignored (ForceAttemptHTTP2 with a custom dialer is easy
// to get subtly wrong), not against Go's pings working.
func TestTunedTransport_HTTP2HealthCheckEndsADeadConnection(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			time.Sleep(1500 * time.Millisecond)
		}
		_, _ = io.WriteString(w, "ok")
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	backend := srv.Listener.Addr().String()

	newClient := func() *http.Client {
		pool := x509.NewCertPool()
		pool.AddCert(srv.Certificate())
		tr := newTunedTransport()
		tr.Proxy = nil
		tr.TLSClientConfig = &tls.Config{RootCAs: pool}
		tr.HTTP2.SendPingTimeout = 200 * time.Millisecond
		tr.HTTP2.PingTimeout = 200 * time.Millisecond
		t.Cleanup(tr.CloseIdleConnections)
		return &http.Client{Transport: tr}
	}
	get := func(c *http.Client, addr, path string) (time.Duration, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+path, nil)
		start := time.Now()
		resp, err := c.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		return time.Since(start), err
	}

	var hole atomic.Bool
	addr := blackholeProxy(t, backend, &hole)
	c := newClient()
	if _, err := get(c, addr, "/fast"); err != nil {
		t.Fatalf("warm-up: %v", err)
	}
	hole.Store(true)
	elapsed, err := get(c, addr, "/fast")
	if err == nil {
		t.Fatal("request over a blackholed connection succeeded")
	}
	if elapsed > 3*time.Second {
		t.Errorf("dead connection took %v to fail, want a few hundred ms from the health check (err=%v)", elapsed, err)
	}

	var alive atomic.Bool
	addr = blackholeProxy(t, backend, &alive)
	if elapsed, err := get(newClient(), addr, "/slow"); err != nil {
		t.Errorf("slow but live response failed after %v: %v — the health check must not interrupt it", elapsed, err)
	}
}
