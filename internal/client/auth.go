// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// defaultHTTPTimeout is the default timeout for HTTP requests.
const defaultHTTPTimeout = 30 * time.Second

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

// newOAuth2Client creates an HTTP client with automatic OAuth2 token management.
func newOAuth2Client(config *clientcredentials.Config, userAgent string) (oauthClient *http.Client, baseClient *http.Client) {
	base := &http.Client{Timeout: defaultHTTPTimeout}

	transport := http.DefaultTransport
	if userAgent != "" {
		transport = &userAgentTransport{
			base:      http.DefaultTransport,
			userAgent: userAgent,
		}
	}
	base.Transport = transport

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	return config.Client(ctx), base
}

// wrapWithOAuth2 wraps a base HTTP client with OAuth2 token management.
func wrapWithOAuth2(config *clientcredentials.Config, base *http.Client) *http.Client {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	return config.Client(ctx)
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
func (c *Client) AccessToken(ctx context.Context) (*oauth2.Token, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.baseClient)
	ts := c.tokenSource(ctx)
	return ts.Token()
}

// tokenSource returns the appropriate TokenSource, wrapping with caching if configured.
func (c *Client) tokenSource(ctx context.Context) oauth2.TokenSource {
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
