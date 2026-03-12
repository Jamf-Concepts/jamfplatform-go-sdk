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
	transport *client.Client
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

	return &Client{transport: transport}
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
