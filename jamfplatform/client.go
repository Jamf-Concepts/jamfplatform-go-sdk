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
	transport     *client.Client
	environmentID string
	tenantID      string
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

	transport := client.NewClientWithUserAgent(baseURL, clientID, clientSecret, cfg.userAgent, transportOpts...)
	if cfg.logger != nil {
		transport.SetLogger(cfg.logger)
	}

	return &Client{
		transport:     transport,
		environmentID: cfg.environmentID,
		tenantID:      cfg.tenantID,
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
	userAgent     string
	httpClient    *http.Client
	logger        Logger
	environmentID string
	tenantID      string
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

// WithEnvironmentID configures environment-scoped APIs (devices, device groups,
// device actions) to use public beta URL paths with the environment ID in the path.
// When not set, legacy internal paths are used.
func WithEnvironmentID(id string) Option {
	return func(cfg *clientConfig) {
		cfg.environmentID = id
	}
}

// WithTenantID configures tenant-scoped APIs (blueprints, compliance benchmarks)
// to send the X-Tenant-Id header. When not set, requests are sent without the header.
func WithTenantID(id string) Option {
	return func(cfg *clientConfig) {
		cfg.tenantID = id
	}
}

// environmentPrefix returns the API path prefix for environment-scoped resources.
// If an environment ID is configured, it returns the public beta path pattern.
// Otherwise it returns the legacy prefix unchanged.
func (c *Client) environmentPrefix(namespace, version, legacyPrefix string) string {
	if c.environmentID != "" {
		return "/api/" + namespace + "/" + version + "/environment/" + c.environmentID
	}
	return legacyPrefix
}

