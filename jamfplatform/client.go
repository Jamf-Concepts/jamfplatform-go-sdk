// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"log"
	"net/http"
	"path/filepath"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/internal/client"
	"golang.org/x/oauth2"
)

// Client provides typed methods for all Jamf Platform API operations.
type Client struct {
	transport *client.Transport
}

// NewClient creates a new Jamf Platform API client.
func NewClient(baseURL, clientID, clientSecret string, opts ...Option) *Client {
	cfg := &clientConfig{
		userAgent: "jamfplatform-go-sdk/dev",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	var transportOpts []client.Option
	if cfg.httpClient != nil {
		transportOpts = append(transportOpts, client.WithHTTPClient(cfg.httpClient))
	}

	var cache client.TokenCache
	if cfg.tokenCache != nil {
		cache = cfg.tokenCache
	} else if cfg.cacheDir != "" {
		cache = client.NewFileTokenCache(cfg.cacheDir)
	}
	if cache != nil {
		transportOpts = append(transportOpts, client.WithTokenCache(cache, client.CacheKey(baseURL, clientID)))
	}
	if cfg.cookieJarDir != "" {
		jarPath := filepath.Join(cfg.cookieJarDir, "jamfplatform-cookies-"+client.CacheKey(baseURL, clientID))
		if jar, err := client.NewFileCookieJar(jarPath); err == nil {
			transportOpts = append(transportOpts, client.WithCookieJar(jar))
		} else {
			log.Printf("jamfplatform: WithFileCookieJar: failed to open %s: %v — falling back to in-memory jar", jarPath, err)
		}
	}
	if cfg.tenantID != "" {
		transportOpts = append(transportOpts, client.WithTenantID(cfg.tenantID))
	}
	if cfg.retryOn4xx {
		transportOpts = append(transportOpts, client.WithRetryOn4xx(true))
	}

	transport := client.NewTransportWithUserAgent(baseURL, clientID, clientSecret, cfg.userAgent, transportOpts...)
	if cfg.logger != nil {
		transport.SetLogger(cfg.logger)
	}

	return &Client{
		transport: transport,
	}
}

// BaseURL returns the base URL configured for the client.
func (c *Client) BaseURL() string {
	return c.transport.BaseURL()
}

// ValidateCredentials tests authentication by requesting an OAuth token.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	return c.transport.ValidateCredentials(ctx)
}

// AccessToken returns a valid OAuth2 token from the client's credentials configuration.
func (c *Client) AccessToken(ctx context.Context) (*oauth2.Token, error) {
	return c.transport.AccessToken(ctx)
}

// Transport returns the underlying transport used by sub-package clients in
// jamfplatform/. Sub-package constructors (e.g. devices.New) call this to
// share the authenticated HTTP layer.
func (c *Client) Transport() *client.Transport {
	return c.transport
}

// clientConfig holds configuration applied via Option functions.
type clientConfig struct {
	userAgent    string
	httpClient   *http.Client
	logger       Logger
	tenantID     string
	tokenCache   TokenCache
	cacheDir     string
	cookieJarDir string
	retryOn4xx   bool
}

// Option configures a Client.
type Option func(*clientConfig)

// WithUserAgent sets a custom user agent string.
func WithUserAgent(userAgent string) Option {
	return func(cfg *clientConfig) {
		if userAgent != "" {
			cfg.userAgent = userAgent
		}
	}
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(cfg *clientConfig) {
		cfg.httpClient = httpClient
	}
}

// WithLogger sets a logger for HTTP request/response logging.
func WithLogger(logger Logger) Option {
	return func(cfg *clientConfig) {
		cfg.logger = logger
	}
}

// WithTokenCache sets a custom token cache for persisting tokens across process restarts.
func WithTokenCache(cache TokenCache) Option {
	return func(cfg *clientConfig) {
		cfg.tokenCache = cache
	}
}

// WithFileTokenCache enables file-based token caching in the given directory.
func WithFileTokenCache(dir string) Option {
	return func(cfg *clientConfig) {
		cfg.cacheDir = dir
	}
}

// WithFileCookieJar enables file-based cookie jar persistence in the given
// directory. The cookie jar survives across process invocations so
// sticky-session cookies keep pointing a CLI-style caller at the same app
// node between runs.
func WithFileCookieJar(dir string) Option {
	return func(cfg *clientConfig) {
		cfg.cookieJarDir = dir
	}
}

// WithTenantID configures the tenant ID used to build API URL paths.
// When set, all API paths include /tenant/{tenantID} in the URL.
// When not set, legacy internal paths are used.
func WithTenantID(id string) Option {
	return func(cfg *clientConfig) {
		cfg.tenantID = id
	}
}

// WithRetryOn4xx opts the client into retrying unexpected 4xx responses
// (400–499, excluding 401 and 403) with exponential backoff. Intended for
// API families that exhibit eventual consistency — e.g. a device-group DELETE
// returning 400 HAS_DEPENDENCIES immediately after the referencing blueprint
// was deleted. Backoff starts at 2s, caps at 10s; context timeout is the only
// bound. Default is off.
func WithRetryOn4xx(enabled bool) Option {
	return func(cfg *clientConfig) {
		cfg.retryOn4xx = enabled
	}
}
