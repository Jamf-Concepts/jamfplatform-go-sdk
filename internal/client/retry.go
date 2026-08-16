// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// retryWaitMin/retryWaitMax/retryMax bound the transport's automatic retry
// of transient failures (429/503, 500/502/504 on GET/DELETE/HEAD, and
// recoverable network errors — see isRetryableWriteStatus). Total attempts
// per request is retryMax+1. retryWaitMax also acts as a hard ceiling on a
// server-supplied Retry-After: retryablehttp honors that header verbatim
// and uncapped (see jamfBackoff), which would otherwise let a misconfigured
// or hostile value stall a request far longer than any caller-supplied
// context deadline is likely to allow for.
const (
	retryWaitMin = 1 * time.Second
	retryWaitMax = 60 * time.Second
	retryMax     = 4
)

// isRetryableWriteStatus reports whether a response with the given status,
// from a request using method, is safe to retry automatically.
//
// 429 and 503 are gateway-level rejections — the request never reached
// Jamf's application logic, so retrying is safe regardless of method. 501
// ("Not Implemented") is permanent and never retried. Any other 5xx (500,
// 502, 504, or a malformed/zero status) may mean the application partially
// processed the request before failing, so retrying is restricted to
// methods that are idempotent by definition (GET/DELETE/PUT/HEAD) — retrying
// a POST or PATCH risks double-applying a write the server actually
// completed before the client saw the failure (e.g. creating a duplicate
// object).
//
// PUT's idempotency assumption has one important carve-out: it only holds
// when the server has no side-channel precondition that advances as a side
// effect of a successful write. A handful of Jamf Pro V3 endpoints (computer
// and mobile-device prestage enrollment — see jamfplatform/pro's
// version_lock_helpers.go and its two callers, the only VersionLock users in
// the whole SDK) require an optimistic-lock field in the PUT body, sourced
// from a GET taken before the write. Confirmed live: such a PUT can commit
// successfully server-side (bumping the lock) and still return a 500 if the
// response serializer then crashes; blindly retrying it replays the
// now-stale lock and gets a genuine 409 OPTIMISTIC_LOCK_FAILED instead of the
// original 500 — turning a successful write into a reported failure.
//
// Given that's a small, enumerable set today rather than an inherent
// property of PUT in general, the default here stays retry-on for PUT, and
// the known exceptions opt OUT individually via DoWithContentTypeNoRetry
// (see its doc). Any future endpoint that gains a similar precondition
// (VersionLock, ETag, If-Match, or equivalent) must route through that
// no-retry entrypoint too — this function has no way to know from a bare
// method+status that a given PUT carries one.
//
// All other statuses (2xx, and 4xx other than 429) are left alone: a
// byte-identical retry won't change a client-request problem, and this
// function is never consulted for them in the first place since
// DefaultRetryPolicy already excludes them upstream of this check.
func isRetryableWriteStatus(method string, status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	case http.StatusNotImplemented:
		return false
	}
	if status == 0 || status >= 500 {
		switch method {
		case http.MethodGet, http.MethodDelete, http.MethodPut, http.MethodHead:
			return true
		default:
			return false
		}
	}
	return false
}

// jamfCheckRetry extends retryablehttp.DefaultRetryPolicy with the
// method-awareness it doesn't have: DefaultRetryPolicy alone would retry a
// 500 on a POST just as readily as one on a GET. See isRetryableWriteStatus
// for the policy. DefaultRetryPolicy already handles the connection-error
// case (resp == nil, err != nil) correctly on its own — those are always
// safe to retry regardless of method, since nothing reached the server —
// so this only adds a check when a response actually came back.
//
// Deliberately does NOT extend this to endpoint-specific 4xx semantics
// (e.g. a classic API's misleading "400" on an accepted-async delete, or a
// transient "409 problem matching limitation user group" during a
// from-scratch apply). The transport has no way to tell a permanent 4xx
// from a transient one without knowing what the specific endpoint's status
// code means for that specific payload — that eventual-consistency
// judgement is a caller concern, not a transport one. This mirrors the
// reasoning that removed the SDK's old blanket 4xx retry.
func jamfCheckRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	shouldRetry, checkErr := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	if !shouldRetry || checkErr != nil || resp == nil {
		return shouldRetry, checkErr
	}
	return isRetryableWriteStatus(resp.Request.Method, resp.StatusCode), nil
}

// jamfBackoff wraps retryablehttp.RateLimitLinearJitterBackoff — which
// honors a server-supplied Retry-After for 429/503 verbatim and uncapped —
// so the wait is always clamped to max. An oversized or malformed
// Retry-After should slow a retry down, not let it stall past what
// retryWaitMax already promises callers.
func jamfBackoff(minWait, maxWait time.Duration, attemptNum int, resp *http.Response) time.Duration {
	wait := retryablehttp.RateLimitLinearJitterBackoff(minWait, maxWait, attemptNum, resp)
	if wait > maxWait {
		return maxWait
	}
	return wait
}

// newRetryClient builds a retryablehttp.Client around authed — the
// oauth2+throttle-wrapped client also used directly (unwrapped) for
// multipart uploads, see multipart.go. authed is deliberately the
// *HTTPClient* field, not the retry client's own base transport: every
// retry attempt re-enters authed, so token refresh and request pacing both
// apply per attempt, not just to the first one.
//
// ErrorHandler is set to PassthroughErrorHandler, which is load-bearing and
// not a style choice: retryablehttp's default behavior on retry exhaustion
// is to drain and discard the final response body and return a synthetic
// "giving up after N attempts" error instead. That would break every caller
// that inspects the concrete *APIResponseError (helpers.IsNotFoundError,
// IsServerError, AsAPIError, .TraceID, .Errors, …) once retries run out on a
// persistent failure. PassthroughErrorHandler returns the underlying
// (resp, err) pair untouched — for a plain non-2xx response with no
// transport-level error, err is nil, so this call site's caller sees a
// final response exactly as if it had never been retried, and the existing
// handleResponse decoding path is unaffected.
func newRetryClient(authed *http.Client) *retryablehttp.Client {
	rc := retryablehttp.NewClient()
	rc.HTTPClient = authed
	rc.Logger = nil // the SDK has its own Logger interface (LogRequest/LogResponse); avoid a second, unrelated stderr logger.
	rc.CheckRetry = jamfCheckRetry
	rc.Backoff = jamfBackoff
	rc.RetryWaitMin = retryWaitMin
	rc.RetryWaitMax = retryWaitMax
	rc.RetryMax = retryMax
	rc.ErrorHandler = retryablehttp.PassthroughErrorHandler
	return rc
}
