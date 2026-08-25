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
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/oauth2/clientcredentials"
)

// isClassicPath reports whether an endpoint targets Jamf's Classic XML API
// via the platform gateway. The path prefix sniff is the single source of
// truth for codec selection: Classic paths marshal/unmarshal XML, everything
// else uses JSON.
func isClassicPath(path string) bool {
	return strings.Contains(path, "/proclassic/")
}

// Logger is an interface for logging HTTP requests and responses.
type Logger interface {
	LogRequest(ctx context.Context, method, url string, body []byte)
	LogResponse(ctx context.Context, statusCode int, headers http.Header, body []byte)
}

// Transport represents the HTTP transport layer for the Jamf Platform API.
// Sub-packages in jamfplatform/ construct service clients that wrap a Transport.
type Transport struct {
	baseURL         string
	scopeKind       ScopeKind    // which request-context header carries the scope
	scopeID         string       // the tenant (or environment) ID that header sends
	httpClient      *http.Client // retry.StandardClient() — used by execute() for all JSON/XML Do* calls
	uploadClient    *http.Client // authed + paced, NOT retry-wrapped — used directly by multipart.go; see retry.go's newRetryClient doc
	baseClient      *http.Client
	oauthConfig     *clientcredentials.Config
	logger          Logger
	userAgent       string
	tokenCache      TokenCache
	cacheKey        string
	cookieJar       http.CookieJar
	deprecationSeen sync.Map // dedup runtime Deprecation header warnings
	throttle        *requestThrottle
	retry           *retryablehttp.Client // backs httpClient; mutating retry.HTTPClient (see SetHTTPClient/SetUserAgent) updates httpClient's behavior in place
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

// Option configures a Client.
type Option func(*Transport)

// WithHTTPClient overrides the HTTP client used by the API client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Transport) {
		if httpClient != nil {
			if httpClient.Jar == nil {
				httpClient.Jar = newCookieJar()
			}
			c.baseClient = httpClient
			c.uploadClient = wrapWithOAuth2(c.oauthConfig, httpClient, c.throttle)
			c.retry.HTTPClient = c.uploadClient
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

// WithTenantID sets the tenant this client is scoped to. It is sent as the
// X-Tenant-Id request header on every API call; see ScopeHeader.
func WithTenantID(id string) Option {
	return func(c *Transport) {
		c.scopeKind = ScopeTenant
		c.scopeID = id
	}
}

// WithCookieJar overrides the default in-memory cookie jar. Typically used to
// install a persistent jar (e.g. FileCookieJar) so sticky-session cookies
// survive across process invocations.
func WithCookieJar(jar http.CookieJar) Option {
	return func(c *Transport) {
		c.cookieJar = jar
	}
}

// WithMinRequestInterval sets a client-level minimum elapsed wall-clock time
// between the start of consecutive outbound HTTP requests. It paces all traffic
// through the shared transport (which Terraform fans out across parallel
// goroutines), giving the server breathing room and reducing 429s. A value
// <= 0 disables the gate. The default is 100ms.
func WithMinRequestInterval(d time.Duration) Option {
	return func(c *Transport) {
		c.throttle.setInterval(d)
	}
}

// WithRetryPolicy overrides the transport's automatic-retry timing for
// transient failures (see retry.go's isRetryableWriteStatus for what gets
// retried). Exists primarily for test harnesses that mock a
// persistently-failing transient status (e.g. an always-500 GET) and need
// the retry loop to run in milliseconds rather than the production 1s-60s
// window with up to 5 total attempts — without this, such a test would hang
// for the full backoff duration on every run. maxRetries follows
// retryablehttp's own semantics: total attempts = maxRetries+1, so 0 means
// no retries at all.
func WithRetryPolicy(waitMin, waitMax time.Duration, maxRetries int) Option {
	return func(c *Transport) {
		c.retry.RetryWaitMin = waitMin
		c.retry.RetryWaitMax = waitMax
		c.retry.RetryMax = maxRetries
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

	throttle := newRequestThrottle(defaultMinRequestInterval)
	uploadClient, baseClient := newOAuth2Client(oauthConfig, userAgent, throttle)
	retry := newRetryClient(uploadClient)

	c := &Transport{
		baseURL:      baseURL,
		uploadClient: uploadClient,
		httpClient:   retry.StandardClient(),
		baseClient:   baseClient,
		oauthConfig:  oauthConfig,
		userAgent:    userAgent,
		throttle:     throttle,
		retry:        retry,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.tokenCache != nil {
		c.uploadClient = newCachingOAuth2Client(c.oauthConfig, c.baseClient, c.tokenCache, c.cacheKey)
		c.retry.HTTPClient = c.uploadClient
	}
	if c.cookieJar != nil {
		c.uploadClient.Jar = c.cookieJar
		c.baseClient.Jar = c.cookieJar
	}
	return c
}

// BaseURL returns the base URL configured for the client.
func (c *Transport) BaseURL() string {
	return c.baseURL
}

// ScopeKind identifies which kind of Jamf scope a client is bound to. The
// gateway calls this the request context, and each kind travels in its own
// request header.
type ScopeKind int

const (
	// ScopeTenant scopes requests to a single product tenant, sent as
	// X-Tenant-Id. This is what every API surface in this SDK uses today.
	ScopeTenant ScopeKind = iota + 1
	// ScopeEnvironment scopes requests to a platform environment — a grouping
	// of tenants — sent as X-Environment-Id. Declared because the gateway
	// accepts it on several namespaces and environment-scoped APIs exist
	// (ai-governance, audit); no operation this SDK generates is
	// environment-scoped yet, so no option sets it.
	ScopeEnvironment
)

// ScopeHeader returns the request header that carries this scope kind, or ""
// when the kind is unset.
//
// Organization scope deliberately has no entry: the gateway resolves it from
// the access token alone (request-context-allowed-sources is `token` for the
// account api-products, in every environment), so there is no header to send.
func (k ScopeKind) ScopeHeader() string {
	switch k {
	case ScopeTenant:
		return "X-Tenant-Id"
	case ScopeEnvironment:
		return "X-Environment-Id"
	default:
		return ""
	}
}

// setScopeHeader stamps the scope header on a request. It is applied before
// extraHeaders so an operation that needs to override the scope for one call
// still can, and it is a no-op when no scope was configured — the gateway then
// resolves the context from the access token, which is how organization-scoped
// products work.
func (c *Transport) setScopeHeader(req *http.Request) {
	if h := c.scopeKind.ScopeHeader(); h != "" && c.scopeID != "" {
		req.Header.Set(h, c.scopeID)
	}
}

// TenantID returns the tenant ID configured on the transport, or "" when this
// client is scoped to something other than a tenant.
func (c *Transport) TenantID() string {
	if c.scopeKind != ScopeTenant {
		return ""
	}
	return c.scopeID
}

// APIPrefix returns the /api/{namespace}/{version} URL prefix for a namespace.
// An empty version collapses that segment, for the APIs that carry no version
// in the URL (proclassic, Pro preview paths).
//
// The scope is NOT in the path. Until 2026-08-25 every Jamf URL embedded it —
// /api/{namespace}/{version}/tenant/{tenantID} — and the gateway's Tyk config
// resolved the request context from `path`. `header` was added as an allowed
// source in prod on that date (tyk-gateway-management 0793131b, "JSC-73421
// Enable header context support - Prod"), and the published specs dropped the
// path segment in GitOps build v1495 in favour of a required X-Tenant-Id
// header. Both forms answered 200 during the transition, wire-verified across
// EU securitycloud and US pro/blueprints/compliance-benchmarks/devices, so
// this moved to headers only rather than carrying a selectable mode: a second
// code path nothing exercises is how an earlier URL-shape bug went unnoticed
// for weeks.
func (c *Transport) APIPrefix(namespace, version string) string {
	if version == "" {
		return "/api/" + namespace
	}
	return "/api/" + namespace + "/" + version
}

// ValidateCredentials tests authentication by requesting an OAuth token.
func (c *Transport) ValidateCredentials(ctx context.Context) error {
	return validateCredentials(ctx, c.oauthConfig, c.baseClient)
}

// HTTPClient returns the underlying OAuth2-managed, retry-wrapped HTTP client for raw authenticated requests.
func (c *Transport) HTTPClient() *http.Client {
	return c.httpClient
}

// SetHTTPClient sets a custom base HTTP client (useful for testing).
func (c *Transport) SetHTTPClient(httpClient *http.Client) {
	c.baseClient = httpClient
	c.uploadClient = wrapWithOAuth2(c.oauthConfig, httpClient, c.throttle)
	c.retry.HTTPClient = c.uploadClient
}

// SetLogger sets the logger for the client.
func (c *Transport) SetLogger(logger Logger) {
	c.logger = logger
}

// SetUserAgent sets the User-Agent header value used for token and API requests.
func (c *Transport) SetUserAgent(ua string) {
	c.userAgent = ua
	c.uploadClient, c.baseClient = newOAuth2Client(c.oauthConfig, ua, c.throttle)
	c.retry.HTTPClient = c.uploadClient
}

// Do performs an authenticated API request and decodes the response.
// It expects HTTP 200 OK as the success status.
func (c *Transport) Do(ctx context.Context, method, path string, body, result any) error {
	return c.DoExpect(ctx, method, path, body, http.StatusOK, result)
}

// DoExpect performs an authenticated API request expecting the given HTTP status.
func (c *Transport) DoExpect(ctx context.Context, method, path string, body any, expectedStatus int, result any) error {
	return c.execute(ctx, method, path, body, "", nil, expectedStatus, result, c.httpClient)
}

// DoWithContentType performs an authenticated API request with a custom Content-Type header.
// It expects HTTP 200 OK as the success status.
func (c *Transport) DoWithContentType(ctx context.Context, method, path string, body any, contentType string, expectedStatus int, result any) error {
	return c.execute(ctx, method, path, body, contentType, nil, expectedStatus, result, c.httpClient)
}

// DoWithContentTypeNoRetry is DoWithContentType without the transport's
// automatic 5xx retry (see isRetryableWriteStatus). It exists for the small,
// enumerable set of PUT/PATCH endpoints that carry a side-channel
// precondition — an optimistic-lock field sourced from a GET taken before
// the write — where a successful-but-500ing write, if blindly retried,
// replays a now-stale precondition and turns into a genuine conflict the
// caller's own 500-specific compensation never expects. Route through this
// method instead of DoWithContentType for any such endpoint; see
// isRetryableWriteStatus's doc for the mechanism and the current callers
// (computer/mobile-device prestage enrollment). 429/503 retry still applies
// — those are gateway-level rejections that never reached the precondition
// check in the first place, so they carry none of this risk.
//
// This is a workaround for an upstream response-serializer bug, not a
// permanent architectural split: once Jamf fixes it, this opt-out and its
// callers' own GET-diff compensation (e.g. isPutSerializerBug in
// terraform-provider-jamfplatform) should both be removed together.
func (c *Transport) DoWithContentTypeNoRetry(ctx context.Context, method, path string, body any, contentType string, expectedStatus int, result any) error {
	return c.execute(ctx, method, path, body, contentType, nil, expectedStatus, result, c.uploadClient)
}

// DoWithHeaders performs an authenticated API request with extra headers and decodes the response.
// It expects HTTP 200 OK as the success status.
func (c *Transport) DoWithHeaders(ctx context.Context, method, path string, body any, headers http.Header, result any) error {
	return c.DoExpectWithHeaders(ctx, method, path, body, headers, http.StatusOK, result)
}

// DoExpectWithHeaders performs an authenticated API request with extra headers expecting the given HTTP status.
func (c *Transport) DoExpectWithHeaders(ctx context.Context, method, path string, body any, headers http.Header, expectedStatus int, result any) error {
	return c.execute(ctx, method, path, body, "", headers, expectedStatus, result, c.httpClient)
}

// execute funnels every Do* variant through one place so Deprecation-header
// logging lives in a single hook point. client selects whether transient
// failures (429, 503, and 500/502/504 on an idempotent method — see
// isRetryableWriteStatus) are retried one layer down: c.httpClient retries,
// c.uploadClient doesn't — see DoWithContentTypeNoRetry. Non-retried
// statuses surface immediately as an *APIResponseError —
// eventual-consistency handling of a specific endpoint's error semantics is
// a caller concern.
func (c *Transport) execute(ctx context.Context, method, path string, body any, contentType string, extraHeaders http.Header, expectedStatus int, result any, client *http.Client) error {
	resp, classic, err := c.doRequestFull(ctx, method, path, body, contentType, extraHeaders, client)
	if err != nil {
		return err
	}
	return c.handleResponse(ctx, resp, classic, expectedStatus, result)
}

// logDeprecation logs once per (method, path) when a server response includes
// a Deprecation header, so consumers see the notice even without a custom
// Logger installed. Runtime signal in addition to spec-level // Deprecated:
// godoc, which only catches endpoints marked in the spec at SDK build time.
func (c *Transport) logDeprecation(resp *http.Response) {
	if resp == nil || resp.Request == nil {
		return
	}
	v := resp.Header.Get("Deprecation")
	if v == "" {
		return
	}
	key := resp.Request.Method + " " + resp.Request.URL.Path
	if _, seen := c.deprecationSeen.LoadOrStore(key, struct{}{}); seen {
		return
	}
	log.Printf("jamfplatform: endpoint %s returned Deprecation header: %s — migrate callers", key, v)
}

// buildURL constructs the full API URL from a relative endpoint.
func (c *Transport) buildURL(endpoint string) string {
	if len(endpoint) > 0 && endpoint[0] == '/' {
		return c.baseURL + endpoint
	}
	return c.baseURL + "/" + endpoint
}

// doRequestFull performs an authenticated API request with optional content
// type and extra headers, dispatching through client (c.httpClient or
// c.uploadClient — see execute). Returns the response, the classic-codec
// flag captured from the request URL (used by the caller to route XML vs
// JSON unmarshal — avoids re-sniffing the response URL, which may have
// mutated through redirects), and any transport error.
func (c *Transport) doRequestFull(ctx context.Context, method, endpoint string, body any, contentType string, extraHeaders http.Header, client *http.Client) (*http.Response, bool, error) {
	var requestBodyBytes []byte

	fullURL := c.buildURL(endpoint)

	if err := checkDeniedPath(method, fullURL); err != nil {
		return nil, false, err
	}

	classic := isClassicPath(fullURL)

	if body != nil {
		// Raw byte bodies (e.g. Classic XML assembled by the caller) bypass
		// codec selection — send verbatim.
		if b, ok := body.([]byte); ok {
			requestBodyBytes = b
		} else {
			var err error
			if classic {
				requestBodyBytes, err = xml.Marshal(body)
			} else {
				requestBodyBytes, err = json.Marshal(body)
			}
			if err != nil {
				return nil, false, fmt.Errorf("failed to marshal request body: %w", err)
			}
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
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	c.setScopeHeader(req)

	for key, values := range extraHeaders {
		for _, v := range values {
			req.Header.Set(key, v)
		}
	}

	if classic {
		req.Header.Set("Accept", "application/xml")
	}
	if requestBodyBytes != nil {
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		} else if classic {
			req.Header.Set("Content-Type", "application/xml")
		} else if method == http.MethodPatch {
			req.Header.Set("Content-Type", "application/merge-patch+json")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("API request failed: %w", err)
	}

	return resp, classic, nil
}

// handleResponse processes API responses and handles common error cases.
// classic comes from the request URL captured by doRequestFull so codec
// selection is stable across any redirect path rewriting.
func (c *Transport) handleResponse(ctx context.Context, resp *http.Response, classic bool, expectedStatus int, result any) error {
	defer func() { _ = resp.Body.Close() }()

	c.logDeprecation(resp)

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
		_ = json.Unmarshal(body, &apiErr) // best-effort; non-JSON bodies leave apiErr zero
		switch {
		case len(apiErr.Errors) > 0:
			if apiErr.HTTPStatus > 0 {
				respErr.StatusCode = apiErr.HTTPStatus
			}
			respErr.Errors = apiErr.Errors
		default:
			// Classic API errors are an HTML "Status page", not JSON. Lift the
			// message into a synthetic structured detail so it renders through
			// the same path as Pro errors (Error/Summary/Details/FieldErrors,
			// and traceId). Non-classic, non-JSON bodies leave Errors empty and
			// fall back to the raw body.
			if msg, ok := parseClassicErrorMessage(resp.Header, body); ok {
				respErr.Errors = []Error{{Description: msg}}
			}
		}
		respErr.TraceID = pickTraceID(apiErr.TraceID, resp.Header)

		return respErr
	}

	if result != nil {
		// Raw byte responses (e.g. text/csv exports) bypass decoding.
		if bp, ok := result.(*[]byte); ok {
			*bp = append((*bp)[:0], body...)
		} else if classic {
			// Classic endpoints sometimes return 200/201 with an empty body
			// or an XML prolog-only body (e.g. computercommands/command/{cmd}
			// when no results exist). xml.Unmarshal returns io.EOF when it
			// finds no root element; treat that as a successful zero-value
			// decode, identical to a fully empty body.
			if len(bytes.TrimSpace(body)) > 0 {
				if err := xml.Unmarshal(body, result); err != nil && err != io.EOF {
					return fmt.Errorf("failed to decode XML response: %w", err)
				}
			}
		} else if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
