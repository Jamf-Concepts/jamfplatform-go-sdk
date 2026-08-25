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

// ErrUnexpectedResponse indicates the server returned a non-JSON body where a
// JSON response was expected — typically an HTML error page from an edge proxy
// or WAF — and is distinct from a genuine JSON syntax error from the API.
//
// The one sentinel the SDK exposes, deliberately. Everything else is
// *APIResponseError, because a status code and structured details already say
// what a caller needs. This case is different: the condition is inferred from
// the body shape rather than reported by Jamf, and acting on it means doing
// something extra — terraform-provider-jamfprotect looks up the host's public
// egress IP and prints a support block. That is a branch, so it needs a
// matchable error and not a string to grep.
//
// Named to match jamfprotect-go-sdk. The Protect provider's resources move into
// terraform-provider-jamfplatform once Protect has Platform API support, and a
// shared error surface is one less thing to rewrite when they do.
var ErrUnexpectedResponse = errors.New("jamfplatform: unexpected non-JSON response")

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

// ApiError is the on-the-wire shape of an API error response body. Not
// re-exported from the public jamfplatform package — consumers reach
// structured details via APIResponseError accessors, not via this
// intermediate shape.
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
			details[i] = formatDetail(err)
		}
		return fmt.Sprintf("API request failed with status %d, traceId %s (%s): %s",
			e.StatusCode, e.TraceID, requestInfo, strings.Join(details, "; "))
	}

	if e.TraceID != "" {
		return fmt.Sprintf("API request failed with status %s, traceId %s (%s): %s", statusDetail, e.TraceID, requestInfo, e.Body)
	}
	return fmt.Sprintf("API request failed with status %s (%s): %s", statusDetail, requestInfo, e.Body)
}

// formatDetail renders a single structured error detail, omitting the Code or
// Field segment when empty so a code-less or field-less detail — such as one
// synthesised from a Classic API HTML error page (Description only) — does not
// produce stray "[] :" noise. Also tidies the Pro case of a present Code with
// an empty Field.
func formatDetail(d Error) string {
	switch {
	case d.Code != "" && d.Field != "":
		return fmt.Sprintf("[%s] %s: %s", d.Code, d.Field, d.Description)
	case d.Code != "":
		return fmt.Sprintf("[%s] %s", d.Code, d.Description)
	case d.Field != "":
		return fmt.Sprintf("%s: %s", d.Field, d.Description)
	default:
		return d.Description
	}
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
	if apiErr, ok := errors.AsType[*APIResponseError](err); ok {
		return apiErr
	}
	return nil
}
