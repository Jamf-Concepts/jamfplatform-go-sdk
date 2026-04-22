// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestAPIResponseError_HasStatus(t *testing.T) {
	e := &APIResponseError{StatusCode: 409}
	if !e.HasStatus(409) {
		t.Error("HasStatus(409) should be true for StatusCode=409")
	}
	if e.HasStatus(404) {
		t.Error("HasStatus(404) should be false for StatusCode=409")
	}
}

func TestAPIResponseError_Details(t *testing.T) {
	details := []Error{
		{Code: "INVALID_VALUE", Field: "name", Description: "must be unique"},
		{Code: "INVALID_VALUE", Field: "site", Description: "must reference existing site"},
	}
	e := &APIResponseError{Errors: details}

	got := e.Details()
	if !reflect.DeepEqual(got, details) {
		t.Errorf("Details() = %+v, want %+v", got, details)
	}

	empty := &APIResponseError{}
	if empty.Details() != nil {
		t.Errorf("Details() on empty error = %+v, want nil", empty.Details())
	}
}

func TestAPIResponseError_FieldErrors(t *testing.T) {
	tests := []struct {
		name   string
		errors []Error
		want   map[string][]string
	}{
		{
			name: "field-attributed details",
			errors: []Error{
				{Code: "INVALID_VALUE", Field: "name", Description: "must be unique"},
				{Code: "INVALID_VALUE", Field: "site", Description: "must reference existing site"},
			},
			want: map[string][]string{
				"name": {"must be unique"},
				"site": {"must reference existing site"},
			},
		},
		{
			name: "multiple details per field",
			errors: []Error{
				{Code: "TOO_SHORT", Field: "name", Description: "must be at least 3 chars"},
				{Code: "INVALID_CHARS", Field: "name", Description: "must not contain spaces"},
			},
			want: map[string][]string{
				"name": {"must be at least 3 chars", "must not contain spaces"},
			},
		},
		{
			name: "generic detail (no field)",
			errors: []Error{
				{Code: "QUOTA_EXCEEDED", Field: "", Description: "tenant quota exceeded"},
			},
			want: map[string][]string{
				"": {"tenant quota exceeded"},
			},
		},
		{
			name: "description empty, falls back to code",
			errors: []Error{
				{Code: "INVALID_VALUE", Field: "name", Description: ""},
			},
			want: map[string][]string{
				"name": {"INVALID_VALUE"},
			},
		},
		{
			name: "fully empty detail dropped",
			errors: []Error{
				{Code: "", Field: "name", Description: ""},
				{Code: "REAL", Field: "site", Description: "real problem"},
			},
			want: map[string][]string{
				"site": {"real problem"},
			},
		},
		{
			name:   "no details returns empty map",
			errors: nil,
			want:   map[string][]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &APIResponseError{Errors: tc.errors}
			got := e.FieldErrors()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FieldErrors() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAPIResponseError_Summary(t *testing.T) {
	tests := []struct {
		name  string
		err   *APIResponseError
		wants []string // substrings that must all appear
	}{
		{
			name: "field-attributed details",
			err: &APIResponseError{
				StatusCode: 400,
				Method:     "POST",
				URL:        "/api/pro/v1/tenant/x/buildings",
				Errors: []Error{
					{Code: "INVALID_VALUE", Field: "name", Description: "must be unique"},
				},
			},
			wants: []string{"POST", "buildings", "name: must be unique"},
		},
		{
			name: "generic detail",
			err: &APIResponseError{
				StatusCode: 403,
				Method:     "GET",
				URL:        "/api/pro/v1/tenant/x/buildings",
				Errors: []Error{
					{Code: "FORBIDDEN", Field: "", Description: "insufficient privileges"},
				},
			},
			wants: []string{"GET", "buildings", "insufficient privileges"},
		},
		{
			name: "no structured details falls back to status text",
			err: &APIResponseError{
				StatusCode: 500,
				Method:     "POST",
				URL:        "/api/pro/v1/tenant/x/buildings",
			},
			wants: []string{"POST", "buildings", "500", "Internal Server Error"},
		},
		{
			name: "unknown status code with no body",
			err: &APIResponseError{
				StatusCode: 599,
				Method:     "GET",
				URL:        "/api/pro/v1/tenant/x/foo",
			},
			wants: []string{"GET", "foo", "599"},
		},
		{
			name: "detail with empty description falls back to code",
			err: &APIResponseError{
				StatusCode: 400,
				Method:     "PUT",
				URL:        "/api/pro/v1/tenant/x/buildings/1",
				Errors: []Error{
					{Code: "INVALID_VALUE", Field: "name", Description: ""},
				},
			},
			wants: []string{"PUT", "buildings/1", "name: INVALID_VALUE"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Summary()
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Errorf("Summary() = %q, missing %q", got, want)
				}
			}
		})
	}
}

func TestAsAPIError(t *testing.T) {
	direct := &APIResponseError{StatusCode: 409}

	tests := []struct {
		name string
		err  error
		want *APIResponseError
	}{
		{"nil error", nil, nil},
		{"non-API error", errors.New("random"), nil},
		{"direct API error", direct, direct},
		{"wrapped once", fmt.Errorf("context: %w", direct), direct},
		{"wrapped twice", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", direct)), direct},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AsAPIError(tc.err)
			if got != tc.want {
				t.Errorf("AsAPIError() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPickTraceID(t *testing.T) {
	tests := []struct {
		name      string
		bodyTrace string
		headers   map[string]string
		want      string
	}{
		{
			name:      "body wins over headers",
			bodyTrace: "from-body",
			headers: map[string]string{
				"X-Traceid":      "from-header",
				"X-Tyk-Trace-Id": "from-tyk",
			},
			want: "from-body",
		},
		{
			name:      "x-traceid picked when body empty",
			bodyTrace: "",
			headers: map[string]string{
				"X-Traceid":      "from-jamf",
				"X-Tyk-Trace-Id": "from-tyk",
			},
			want: "from-jamf",
		},
		{
			name:      "x-tyk-trace-id picked when body and x-traceid empty",
			bodyTrace: "",
			headers: map[string]string{
				"X-Tyk-Trace-Id": "from-tyk",
			},
			want: "from-tyk",
		},
		{
			name:      "x-b3-traceid picked as last resort",
			bodyTrace: "",
			headers: map[string]string{
				"X-B3-Traceid": "from-b3",
			},
			want: "from-b3",
		},
		{
			name:      "all empty returns empty string",
			bodyTrace: "",
			headers:   map[string]string{},
			want:      "",
		},
		{
			name:      "header casing tolerated",
			bodyTrace: "",
			headers: map[string]string{
				"x-tyk-trace-id": "lowercase-ok",
			},
			want: "lowercase-ok",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tc.headers {
				h.Set(k, v)
			}
			if got := pickTraceID(tc.bodyTrace, h); got != tc.want {
				t.Errorf("pickTraceID = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHandleResponse_TraceIDFromHeader verifies end-to-end that a response
// with no body traceId but a header trace identifier surfaces the header
// value on APIResponseError.TraceID. Covers the real-world case seen on Pro
// API responses (no body traceId, x-tyk-trace-id in header).
func TestHandleResponse_TraceIDFromHeader(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/probe", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tyk-Trace-Id", "from-header-abc123")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"httpStatus":400,"errors":[{"code":"BOOM","field":"name","description":"broken"}]}`))
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/probe", nil, nil)
	apiErr := AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("AsAPIError returned nil for err=%v", err)
	}
	if apiErr.TraceID != "from-header-abc123" {
		t.Errorf("TraceID = %q, want %q", apiErr.TraceID, "from-header-abc123")
	}
}

// TestHandleResponse_TraceIDEmptyBodyHeader verifies the Compliance Benchmarks
// scenario: empty response body (content-length: 0) but a trace header.
// Before the fallback, TraceID was empty in this case.
func TestHandleResponse_TraceIDEmptyBodyHeader(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/probe-empty", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Traceid", "from-jamf-header")
		w.WriteHeader(http.StatusNotFound)
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/probe-empty", nil, nil)
	apiErr := AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("AsAPIError returned nil for err=%v", err)
	}
	if apiErr.TraceID != "from-jamf-header" {
		t.Errorf("TraceID = %q, want %q", apiErr.TraceID, "from-jamf-header")
	}
}

// TestHandleResponse_TraceIDBodyWinsOverHeader verifies the existing
// Platform-JSON case: body carries traceId, headers also have one; body wins.
func TestHandleResponse_TraceIDBodyWinsOverHeader(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/probe-body", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tyk-Trace-Id", "header-val")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"httpStatus":404,"traceId":"body-val","errors":[{"code":"X","description":"y"}]}`))
	})

	err := c.Do(context.Background(), http.MethodGet, "/api/probe-body", nil, nil)
	apiErr := AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("AsAPIError returned nil for err=%v", err)
	}
	if apiErr.TraceID != "body-val" {
		t.Errorf("TraceID = %q, want body-val (body must win over header)", apiErr.TraceID)
	}
}

func TestFieldErrorsIsRangeable(t *testing.T) {
	// Consumers range over FieldErrors() unconditionally; make sure the
	// empty-error case returns something rangeable, not nil.
	e := &APIResponseError{StatusCode: 500}
	got := e.FieldErrors()

	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) != 0 {
		t.Errorf("empty FieldErrors() expected no keys, got %v", keys)
	}
}
