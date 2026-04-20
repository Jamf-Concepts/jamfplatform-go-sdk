// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// traceIDHeaders lists the response headers the SDK consults to recover a
// trace identifier when the response body doesn't carry one. Order matters:
// Jamf-specific first, then the gateway's own header, then the distributed
// tracing (B3) convention used on some Platform endpoints.
var traceIDHeaders = []string{
	"X-Traceid",      // Jamf-emitted, Platform v1 endpoints
	"X-Tyk-Trace-Id", // API gateway (Tyk) — emitted on every response
	"X-B3-Traceid",   // Zipkin B3 propagation — emitted by some backends
}

// pickTraceID returns the first non-empty trace identifier from the response
// body's traceId field followed by the header fallback chain. Returns the
// empty string only when no source carried a trace id at all.
func pickTraceID(bodyTraceID string, h http.Header) string {
	if bodyTraceID != "" {
		return bodyTraceID
	}
	for _, name := range traceIDHeaders {
		if v := h.Get(name); v != "" {
			return v
		}
	}
	return ""
}

// ApiError is the on-the-wire shape of an API error response body. Kept
// unexported (lowercase accessors) because consumers should reach details
// via APIResponseError, not via this intermediate shape.
type ApiError struct {
	HTTPStatus int     `json:"httpStatus"`
	TraceID    string  `json:"traceId"`
	Errors     []Error `json:"errors"`
}

// Error represents an individual structured error detail returned by the
// API. Re-exported publicly as jamfplatform.ErrorDetail.
type Error struct {
	ID          string `json:"id,omitempty"`
	Code        string `json:"code"`
	Field       string `json:"field"`
	Description string `json:"description"`
}

// APIResponseError represents an unexpected HTTP status returned by the
// Jamf Platform API. Implements error; consumers access structured details
// via Details/FieldErrors/Summary.
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

// Error formats the API response error as a human-readable string. Kept
// verbose on purpose — this is the fallback when a consumer has not
// plugged in structured handling via Details/FieldErrors/Summary.
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

// Details returns the structured error details parsed from the API response
// body. Returns nil when the response had no structured error body (e.g. a
// 5xx with an HTML or empty body).
func (e *APIResponseError) Details() []Error {
	return e.Errors
}

// FieldErrors buckets structured error details by their Field property.
// Details with no associated field are bucketed under the empty-string key.
// Returns an empty map when no structured details are present, so callers
// can range over the result unconditionally.
func (e *APIResponseError) FieldErrors() map[string][]string {
	out := make(map[string][]string, len(e.Errors))
	for _, d := range e.Errors {
		msg := d.Description
		if msg == "" {
			msg = d.Code
		}
		if msg == "" {
			continue
		}
		out[d.Field] = append(out[d.Field], msg)
	}
	return out
}

// Summary returns a concise single-line description of the error suitable
// for CLI output, log lines, or generic diagnostic messages. Format prefers
// parsed details when present and falls back to HTTP status text otherwise.
func (e *APIResponseError) Summary() string {
	statusText := http.StatusText(e.StatusCode)
	statusDetail := strconv.Itoa(e.StatusCode)
	if statusText != "" {
		statusDetail = statusDetail + " " + statusText
	}

	if len(e.Errors) > 0 {
		parts := make([]string, 0, len(e.Errors))
		for _, d := range e.Errors {
			msg := d.Description
			if msg == "" {
				msg = d.Code
			}
			if d.Field != "" && msg != "" {
				parts = append(parts, d.Field+": "+msg)
			} else if msg != "" {
				parts = append(parts, msg)
			}
		}
		if len(parts) > 0 {
			return fmt.Sprintf("%s %s: %s", e.Method, e.URL, strings.Join(parts, "; "))
		}
	}

	return fmt.Sprintf("%s %s: %s", e.Method, e.URL, statusDetail)
}

// AsAPIError unwraps err and returns the underlying *APIResponseError if
// present, otherwise nil. Consumers use this instead of calling errors.As
// directly so they don't need to import the concrete error type or manage
// the target pointer themselves.
func AsAPIError(err error) *APIResponseError {
	var apiErr *APIResponseError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}
