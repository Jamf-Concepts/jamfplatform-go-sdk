// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// Per-phase HTTP timeouts. We deliberately do NOT set http.Client.Timeout —
// that's a whole-request deadline that caps body transfer, which kills any
// realistic package upload and silently overrides caller-supplied context
// deadlines (e.g. Terraform's `timeouts { create = "45m" }` block). Body
// transfer time is bounded solely by the caller's ctx; these phase
// timeouts exist to fail fast on dead networks, not healthy long transfers.
const (
	// dialTimeout bounds TCP connection establishment. 10s is generous
	// for well-connected cloud endpoints but catches blackholed hosts.
	dialTimeout = 10 * time.Second
	// tlsHandshakeTimeout bounds the TLS handshake after dial.
	tlsHandshakeTimeout = 10 * time.Second
	// responseHeaderTimeout bounds server time-to-first-byte after the
	// full request body has been written. Uploads that legitimately take
	// hours are unaffected; only servers that accept the body then hang
	// are killed.
	responseHeaderTimeout = 60 * time.Second
	// idleConnTimeout bounds how long a pooled idle connection lives.
	idleConnTimeout = 90 * time.Second
	// maxIdleConnsPerHost governs how many pooled HTTP/1.1 connections
	// per host the SDK may keep warm. Go's default is 2, which
	// serializes terraform-style parallel operations
	// (`-parallelism=10`) at the pool. 10 matches default TF
	// parallelism. HTTP/2 (which apigw.jamf.com supports) multiplexes
	// on a single connection, so this ceiling only binds on the
	// HTTP/1.1 fallback path.
	maxIdleConnsPerHost = 10
)

// newTunedTransport returns a fresh *http.Transport tuned for the SDK's
// typical workload: large uploads (multi-GB packages), bursts of small
// reads (Terraform refresh), HTTP/2 multiplexing where the gateway
// supports it. Not shared across Transport instances — every SDK client
// gets its own pool so multi-tenant usage is isolated.
func newTunedTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		// Larger socket buffers reduce syscalls on big bodies. Package
		// upload is the driver — 1 MiB write buffer pairs with the 1 MiB
		// io.CopyBuffer in multipart.go.
		WriteBufferSize: 1 << 20,
		ReadBufferSize:  1 << 16,
	}
}

// userAgentTransport wraps an http.RoundTripper to add a User-Agent header to all requests.
type userAgentTransport struct {
	base      http.RoundTripper
	userAgent string
}

// RoundTrip adds the User-Agent header and delegates to the base transport.
func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req2)
}

// newCookieJar returns a default in-memory cookie jar. Jamf Cloud uses
// sticky-session cookies to pin a client to a single app node so that
// writes are visible on subsequent reads; without a jar the cookies are
// silently dropped and reads can race against the cluster.
// See https://developer.jamf.com/jamf-pro/docs/sticky-sessions-for-jamf-cloud.
func newCookieJar() *cookiejar.Jar {
	jar, _ := cookiejar.New(nil) // cookiejar.New only errors on invalid options; nil is valid.
	return jar
}

// newOAuth2Client creates an HTTP client with automatic OAuth2 token management.
// The base and OAuth2-wrapped clients share a cookie jar so cookies set during
// token fetch (e.g. load-balancer session cookies) also apply to API calls.
// No Client.Timeout is set — callers bound request lifetime via ctx; the
// underlying Transport bounds each network phase individually.
func newOAuth2Client(config *clientcredentials.Config, userAgent string) (oauthClient *http.Client, baseClient *http.Client) {
	jar := newCookieJar()
	base := &http.Client{Jar: jar}

	var rt http.RoundTripper = newTunedTransport()
	if userAgent != "" {
		rt = &userAgentTransport{base: rt, userAgent: userAgent}
	}
	base.Transport = rt

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	outer := config.Client(ctx)
	outer.Jar = jar
	return outer, base
}

// wrapWithOAuth2 wraps a base HTTP client with OAuth2 token management,
// preserving the base client's cookie jar on the outer client.
func wrapWithOAuth2(config *clientcredentials.Config, base *http.Client) *http.Client {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	outer := config.Client(ctx)
	outer.Jar = base.Jar
	return outer
}

// newCachingOAuth2Client creates an HTTP client whose OAuth2 token source is wrapped with a TokenCache.
func newCachingOAuth2Client(config *clientcredentials.Config, base *http.Client, cache TokenCache, cacheKey string) *http.Client {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	ts := &cachingTokenSource{
		source:   config.TokenSource(ctx),
		cache:    cache,
		cacheKey: cacheKey,
	}
	return &http.Client{
		Transport: &oauth2.Transport{
			Source: oauth2.ReuseTokenSource(nil, ts),
			Base:   base.Transport,
		},
		Timeout: base.Timeout,
		Jar:     base.Jar,
	}
}

// validateCredentials tests authentication by requesting an OAuth token.
func validateCredentials(ctx context.Context, config *clientcredentials.Config, baseClient *http.Client) error {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, baseClient)
	ts := config.TokenSource(ctx)
	_, err := ts.Token()
	return err
}

// AccessToken returns a valid OAuth2 token from the client's credentials configuration.
func (c *Transport) AccessToken(ctx context.Context) (*oauth2.Token, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.baseClient)
	ts := c.tokenSource(ctx)
	return ts.Token()
}

// tokenSource returns the appropriate TokenSource, wrapping with caching if configured.
func (c *Transport) tokenSource(ctx context.Context) oauth2.TokenSource {
	ts := c.oauthConfig.TokenSource(ctx)
	if c.tokenCache == nil {
		return ts
	}
	return &cachingTokenSource{
		source:   ts,
		cache:    c.tokenCache,
		cacheKey: c.cacheKey,
	}
}

// cachingTokenSource wraps an oauth2.TokenSource with a TokenCache for disk persistence.
type cachingTokenSource struct {
	source   oauth2.TokenSource
	cache    TokenCache
	cacheKey string
}

// Token returns a cached token if valid, otherwise fetches a new one and caches it.
func (s *cachingTokenSource) Token() (*oauth2.Token, error) {
	if accessToken, expiresAt, ok := s.cache.Load(s.cacheKey); ok && accessToken != "" && expiresAt.After(time.Now()) {
		return &oauth2.Token{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			Expiry:      expiresAt,
		}, nil
	}

	token, err := s.source.Token()
	if err != nil {
		return nil, err
	}

	_ = s.cache.Store(s.cacheKey, token.AccessToken, token.Expiry)
	return token, nil
}
