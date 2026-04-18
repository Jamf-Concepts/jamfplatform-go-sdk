// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"net/http"

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
	if cfg.tenantID != "" {
		transportOpts = append(transportOpts, client.WithTenantID(cfg.tenantID))
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

// clientConfig holds configuration applied via Option functions.
type clientConfig struct {
	userAgent  string
	httpClient *http.Client
	logger     Logger
	tenantID   string
	tokenCache TokenCache
	cacheDir   string
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

// WithTenantID configures the tenant ID used to build API URL paths.
// When set, all API paths include /tenant/{tenantID} in the URL.
// When not set, legacy internal paths are used.
func WithTenantID(id string) Option {
	return func(cfg *clientConfig) {
		cfg.tenantID = id
	}
}

// tenantPrefix delegates to the transport. Retained while generated code
// still calls it; sub-packages built after the package-per-API rework
// call c.transport.TenantPrefix directly.
func (c *Client) tenantPrefix(namespace, version string) string {
	return c.transport.TenantPrefix(namespace, version)
}
