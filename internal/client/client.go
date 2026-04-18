// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Package client provides the HTTP transport layer for the Jamf Platform API.
//
// This package handles authentication, request/response processing, error handling,
// logging, and pagination. It does not contain any resource-specific types or methods;
// those belong in the jamfplatform package.
//
// https://developer.jamf.com/platform-api/docs/getting-started-with-the-platform-api

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2/clientcredentials"
)

// Logger is an interface for logging HTTP requests and responses.
type Logger interface {
	LogRequest(ctx context.Context, method, url string, body []byte)
	LogResponse(ctx context.Context, statusCode int, headers http.Header, body []byte)
}

// Transport represents the HTTP transport layer for the Jamf Platform API.
// Sub-packages in jamfplatform/ construct service clients that wrap a Transport.
type Transport struct {
	baseURL     string
	tenantID    string
	httpClient  *http.Client
	baseClient  *http.Client
	oauthConfig *clientcredentials.Config
	logger      Logger
	userAgent   string
	tokenCache  TokenCache
	cacheKey    string
}

// PaginatedResponseRepresentation captures pagination metadata shared by multiple endpoints.
type PaginatedResponseRepresentation struct {
	Page        int   `json:"page"`
	PageSize    int   `json:"pageSize"`
	TotalCount  int64 `json:"totalCount"`
	TotalPages  int   `json:"totalPages"`
	HasNext     bool  `json:"hasNext"`
	HasPrevious bool  `json:"hasPrevious"`
}

// ApiError represents an error response from the API.
type ApiError struct {
	HTTPStatus int     `json:"httpStatus"`
	TraceID    string  `json:"traceId"`
	Errors     []Error `json:"errors"`
}

// Error represents an individual error detail from an API response.
type Error struct {
	ID          string `json:"id,omitempty"`
	Code        string `json:"code"`
	Field       string `json:"field"`
	Description string `json:"description"`
}

// APIResponseError represents an unexpected HTTP status returned by the Jamf Platform API.
type APIResponseError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
	TraceID    string
	Errors     []Error
}

// HasStatus reports whether the error carries the given HTTP status code.
func (e *APIResponseError) HasStatus(code int) bool {
	return e.StatusCode == code
}

// Error formats the API response error as a human-readable string.
func (e *APIResponseError) Error() string {
	requestInfo := fmt.Sprintf("method=%s, url=%s", e.Method, e.URL)
	statusText := http.StatusText(e.StatusCode)
	statusDetail := strconv.Itoa(e.StatusCode)
	if statusText != "" {
		statusDetail = strconv.Itoa(e.StatusCode) + " " + statusText
	}

	if len(e.Errors) > 0 {
		details := make([]string, len(e.Errors))
		for i, err := range e.Errors {
			details[i] = fmt.Sprintf("[%s] %s: %s", err.Code, err.Field, err.Description)
		}
		return fmt.Sprintf("API request failed with status %d, traceId %s (%s): %s",
			e.StatusCode, e.TraceID, requestInfo, strings.Join(details, "; "))
	}

	return fmt.Sprintf("API request failed with status %s (%s): %s", statusDetail, requestInfo, e.Body)
}

// Option configures a Client.
type Option func(*Transport)

// WithHTTPClient overrides the HTTP client used by the API client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Transport) {
		if httpClient != nil {
			c.baseClient = httpClient
			c.httpClient = wrapWithOAuth2(c.oauthConfig, httpClient)
		}
	}
}

// WithTokenCache sets a persistent token cache and its lookup key.
func WithTokenCache(cache TokenCache, cacheKey string) Option {
	return func(c *Transport) {
		if cache != nil && cacheKey != "" {
			c.tokenCache = cache
			c.cacheKey = cacheKey
		}
	}
}

// WithTenantID sets the tenant ID used by TenantPrefix when building URLs.
func WithTenantID(id string) Option {
	return func(c *Transport) {
		c.tenantID = id
	}
}

// NewTransport creates a new Jamf Platform API transport.
func NewTransport(baseURL, clientID, clientSecret string) *Transport {
	return NewTransportWithUserAgent(baseURL, clientID, clientSecret, "jamfplatform-go-sdk/dev")
}

// NewTransportWithUserAgent creates a new Jamf Platform API transport with a custom user agent string.
func NewTransportWithUserAgent(baseURL, clientID, clientSecret, userAgent string, opts ...Option) *Transport {
	oauthConfig := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     baseURL + "/auth/token",
	}

	httpClient, baseClient := newOAuth2Client(oauthConfig, userAgent)

	c := &Transport{
		baseURL:     baseURL,
		httpClient:  httpClient,
		baseClient:  baseClient,
		oauthConfig: oauthConfig,
		userAgent:   userAgent,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.tokenCache != nil {
		c.httpClient = newCachingOAuth2Client(c.oauthConfig, c.baseClient, c.tokenCache, c.cacheKey)
	}
	return c
}

// BaseURL returns the base URL configured for the client.
func (c *Transport) BaseURL() string {
	return c.baseURL
}

// TenantID returns the tenant ID configured on the transport.
func (c *Transport) TenantID() string {
	return c.tenantID
}

// TenantPrefix returns the /api/{namespace}/{version}/tenant/{tenantID} URL
// prefix used by tenant-scoped resources. An empty version collapses the
// segment for APIs that don't use a version in the URL (proclassic, Pro
// preview paths).
func (c *Transport) TenantPrefix(namespace, version string) string {
	if version == "" {
		return "/api/" + namespace + "/tenant/" + c.tenantID
	}
	return "/api/" + namespace + "/" + version + "/tenant/" + c.tenantID
}

// ValidateCredentials tests authentication by requesting an OAuth token.
func (c *Transport) ValidateCredentials(ctx context.Context) error {
	return validateCredentials(ctx, c.oauthConfig, c.baseClient)
}

// HTTPClient returns the underlying OAuth2-managed HTTP client for raw authenticated requests.
func (c *Transport) HTTPClient() *http.Client {
	return c.httpClient
}

// SetHTTPClient sets a custom base HTTP client (useful for testing).
func (c *Transport) SetHTTPClient(httpClient *http.Client) {
	c.baseClient = httpClient
	c.httpClient = wrapWithOAuth2(c.oauthConfig, httpClient)
}

// SetLogger sets the logger for the client.
func (c *Transport) SetLogger(logger Logger) {
	c.logger = logger
}

// SetUserAgent sets the User-Agent header value used for token and API requests.
func (c *Transport) SetUserAgent(ua string) {
	c.userAgent = ua
	c.httpClient, c.baseClient = newOAuth2Client(c.oauthConfig, ua)
}

// Do performs an authenticated API request and decodes the response.
// It expects HTTP 200 OK as the success status.
func (c *Transport) Do(ctx context.Context, method, path string, body, result any) error {
	return c.DoExpect(ctx, method, path, body, http.StatusOK, result)
}

// DoExpect performs an authenticated API request expecting the given HTTP status.
func (c *Transport) DoExpect(ctx context.Context, method, path string, body any, expectedStatus int, result any) error {
	resp, err := c.doRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	return c.handleResponse(ctx, resp, expectedStatus, result)
}

// DoWithContentType performs an authenticated API request with a custom Content-Type header.
// It expects HTTP 200 OK as the success status.
func (c *Transport) DoWithContentType(ctx context.Context, method, path string, body any, contentType string, expectedStatus int, result any) error {
	resp, err := c.doRequestWithContentType(ctx, method, path, body, contentType)
	if err != nil {
		return err
	}
	return c.handleResponse(ctx, resp, expectedStatus, result)
}

// DoWithHeaders performs an authenticated API request with extra headers and decodes the response.
// It expects HTTP 200 OK as the success status.
func (c *Transport) DoWithHeaders(ctx context.Context, method, path string, body any, headers http.Header, result any) error {
	return c.DoExpectWithHeaders(ctx, method, path, body, headers, http.StatusOK, result)
}

// DoExpectWithHeaders performs an authenticated API request with extra headers expecting the given HTTP status.
func (c *Transport) DoExpectWithHeaders(ctx context.Context, method, path string, body any, headers http.Header, expectedStatus int, result any) error {
	resp, err := c.doRequestFull(ctx, method, path, body, "", headers)
	if err != nil {
		return err
	}
	return c.handleResponse(ctx, resp, expectedStatus, result)
}

// buildURL constructs the full API URL from a relative endpoint.
func (c *Transport) buildURL(endpoint string) string {
	if len(endpoint) > 0 && endpoint[0] == '/' {
		return c.baseURL + endpoint
	}
	return c.baseURL + "/" + endpoint
}

// doRequest performs an authenticated API request.
func (c *Transport) doRequest(ctx context.Context, method, endpoint string, body any) (*http.Response, error) {
	return c.doRequestFull(ctx, method, endpoint, body, "", nil)
}

// doRequestWithContentType performs an authenticated API request with an optional content type override.
func (c *Transport) doRequestWithContentType(ctx context.Context, method, endpoint string, body any, contentType string) (*http.Response, error) {
	return c.doRequestFull(ctx, method, endpoint, body, contentType, nil)
}

// doRequestFull performs an authenticated API request with optional content type and extra headers.
func (c *Transport) doRequestFull(ctx context.Context, method, endpoint string, body any, contentType string, extraHeaders http.Header) (*http.Response, error) {
	var requestBodyBytes []byte

	fullURL := c.buildURL(endpoint)

	if err := checkDeniedPath(method, fullURL); err != nil {
		return nil, err
	}

	if body != nil {
		var err error
		requestBodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	if c.logger != nil {
		c.logger.LogRequest(ctx, method, fullURL, requestBodyBytes)
	}

	var bodyReader io.Reader
	if requestBodyBytes != nil {
		bodyReader = bytes.NewReader(requestBodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, values := range extraHeaders {
		for _, v := range values {
			req.Header.Set(key, v)
		}
	}

	if requestBodyBytes != nil {
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		} else if method == http.MethodPatch {
			req.Header.Set("Content-Type", "application/merge-patch+json")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	return resp, nil
}

// handleResponse processes API responses and handles common error cases.
func (c *Transport) handleResponse(ctx context.Context, resp *http.Response, expectedStatus int, result any) error {
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read response body: %w", readErr)
	}

	if c.logger != nil {
		c.logger.LogResponse(ctx, resp.StatusCode, resp.Header, body)
	}

	if resp.StatusCode != expectedStatus {
		respErr := &APIResponseError{
			StatusCode: resp.StatusCode,
			Method:     resp.Request.Method,
			URL:        resp.Request.URL.String(),
			Body:       string(body),
		}

		var apiErr ApiError
		if err := json.Unmarshal(body, &apiErr); err == nil && len(apiErr.Errors) > 0 {
			respErr.StatusCode = apiErr.HTTPStatus
			respErr.TraceID = apiErr.TraceID
			respErr.Errors = apiErr.Errors
		}

		return respErr
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
