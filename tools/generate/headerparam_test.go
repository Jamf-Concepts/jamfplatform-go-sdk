// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func headerParam(spec, goName string) ExtraParam {
	return ExtraParam{Spec: spec, Go: goName, Type: "string"}
}

func renderWithHeaders(t *testing.T, category string, headers []ExtraParam) string {
	t.Helper()
	m := GoMethod{
		Name:           "ProbeOp",
		Category:       category,
		HTTPMethod:     "PATCH",
		Namespace:      "probe",
		Version:        "v1",
		ResourcePath:   "/things",
		ResponseType:   "ThingResponse",
		RequestType:    "ThingRequest",
		ExpectedStatus: 204,
		HeaderParams:   headers,
	}
	var buf bytes.Buffer
	if err := sourceTmpl.ExecuteTemplate(&buf, category, m); err != nil {
		t.Fatalf("render %s: %v", category, err)
	}
	return buf.String()
}

// Every category that accepts header params must actually stamp them. Without
// this the pairing of headerParamCategories with the templates is an
// assertion nobody checks: a template could be added to the map and never
// gain its "buildHeaderParams" call, and the only symptom would be a header
// silently never sent.
func TestEveryHeaderParamCategoryStampsTheHeader(t *testing.T) {
	for category := range headerParamCategories {
		t.Run(category, func(t *testing.T) {
			src := renderWithHeaders(t, category, []ExtraParam{headerParam("If-Match", "ifMatch")})
			if !strings.Contains(src, `headers.Set("If-Match", ifMatch)`) {
				t.Errorf("%s does not stamp the header:\n%s", category, src)
			}
			if !strings.Contains(src, "DoWithOptions") {
				t.Errorf("%s does not route through DoWithOptions:\n%s", category, src)
			}
			if !strings.Contains(src, "Headers: headers") {
				t.Errorf("%s does not pass the headers to the transport:\n%s", category, src)
			}
			if !strings.Contains(src, ", ifMatch string)") {
				t.Errorf("%s does not take the header as an argument:\n%s", category, src)
			}
		})
	}
}

// An optional header keeps a zero-value guard, because an absent header and an
// empty one are different requests: for If-Match, absent means "update
// unconditionally" while an empty value is a malformed precondition.
func TestOptionalHeaderIsGuardedAndRequiredIsNot(t *testing.T) {
	optional := renderWithHeaders(t, "update", []ExtraParam{headerParam("If-Match", "ifMatch")})
	if !strings.Contains(optional, `if ifMatch != "" {`) {
		t.Errorf("optional header lost its zero-value guard:\n%s", optional)
	}

	required := headerParam("If-Match", "ifMatch")
	required.AlwaysSend = true
	src := renderWithHeaders(t, "update", []ExtraParam{required})
	if strings.Contains(src, `if ifMatch != "" {`) {
		t.Errorf("spec-required header is still guarded, so a caller passing \"\" omits it:\n%s", src)
	}
}

// A method with no header params must render byte-identically to how it did
// before the mechanism existed — the named shorthand, not DoWithOptions. This
// is what confines the regenerated diff to the operations that gained a
// header, and it is the reason transportCall keeps the old entry points rather
// than routing everything through the general form.
func TestNoHeaderParamsKeepsTheNamedTransportCall(t *testing.T) {
	for _, tc := range []struct {
		category string
		want     string
	}{
		{"get", "c.transport.Do(ctx, http.MethodPatch, endpoint, nil, &result)"},
		{"update", "c.transport.DoExpect(ctx, http.MethodPatch, endpoint, request, http.StatusNoContent, nil)"},
		{"action", "c.transport.DoExpect(ctx, http.MethodPatch, endpoint, nil, http.StatusNoContent, nil)"},
	} {
		src := renderWithHeaders(t, tc.category, nil)
		if !strings.Contains(src, tc.want) {
			t.Errorf("%s: want %q in:\n%s", tc.category, tc.want, src)
		}
		if strings.Contains(src, "DoWithOptions") || strings.Contains(src, "http.Header{}") {
			t.Errorf("%s: a header-free method reached the options form:\n%s", tc.category, src)
		}
	}
}

// contentType and noRetry compose with headers rather than excluding them,
// which is the whole reason RequestOptions replaced a wrapper per combination.
func TestHeaderParamsComposeWithContentTypeAndNoRetry(t *testing.T) {
	m := GoMethod{
		Name: "ProbeOp", Category: "update", HTTPMethod: "PATCH",
		Namespace: "probe", Version: "v1", ResourcePath: "/things",
		RequestType: "ThingRequest", ExpectedStatus: 204,
		ContentType: "application/json", NoRetry: true,
		HeaderParams: []ExtraParam{headerParam("If-Match", "ifMatch")},
	}
	got := transportCall(m)
	for _, want := range []string{
		"DoWithOptions", "ExpectedStatus: http.StatusNoContent",
		`ContentType: "application/json"`, "NoRetry: true", "Headers: headers",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("transportCall = %q, missing %q", got, want)
		}
	}
}

// A "get" carries no ExpectedStatus, because the shape it replaces called Do,
// which hardcodes 200 and never read the field. Promoting the value here would
// change behaviour for any get whose spec declares something else.
func TestGetHeaderCallOmitsExpectedStatus(t *testing.T) {
	m := GoMethod{
		Name: "ProbeOp", Category: "get", HTTPMethod: "GET",
		Namespace: "probe", Version: "v1", ResourcePath: "/things",
		ResponseType: "ThingResponse", ExpectedStatus: 201,
		HeaderParams: []ExtraParam{headerParam("Accept-Language", "acceptLanguage")},
	}
	if got := transportCall(m); strings.Contains(got, "ExpectedStatus") {
		t.Errorf("get promoted ExpectedStatus into the options: %q", got)
	}
}

func TestResolveHeaderParamsRefusals(t *testing.T) {
	specParams := map[string]*openapi3.Parameter{
		"If-Match": {Name: "If-Match", In: openapi3.ParameterInHeader},
		"filter":   {Name: "filter", In: openapi3.ParameterInQuery},
	}
	for _, tc := range []struct {
		name   string
		params []ExtraParam
		want   string
	}{
		{"unknown name", []ExtraParam{headerParam("If-Modified-Since", "ifModifiedSince")}, "not found in spec"},
		{"declared in: query", []ExtraParam{headerParam("filter", "filter")}, `move it to "params"`},
		{"scope header", []ExtraParam{headerParam("X-Tenant-Id", "tenantID")}, "stamped by the transport"},
		{"non-string type", []ExtraParam{{Spec: "If-Match", Go: "ifMatch", Type: "[]string"}}, "only string is supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := GoMethod{HTTPMethod: "PATCH", SpecPath: "/things", HeaderParams: tc.params}
			err := resolveHeaderParams(&m, specParams)
			if err == nil {
				t.Fatalf("want an error mentioning %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}

	t.Run("undocumented opts out of the name match", func(t *testing.T) {
		m := GoMethod{HTTPMethod: "PATCH", SpecPath: "/things", HeaderParams: []ExtraParam{
			{Spec: "X-Probe", Go: "probe", Type: "string", Undocumented: true},
		}}
		if err := resolveHeaderParams(&m, specParams); err != nil {
			t.Fatalf("undocumented header param rejected: %v", err)
		}
	})
}

// The mirror of the above: a header parameter declared under "params" is
// refused rather than emitted as a query key the server ignores. This is the
// failure the split key exists to make impossible — collectSpecParams keys
// every parameter by wire name regardless of where it travels, so without this
// guard the name-match check passes and the request is silently wrong.
func TestResolveQueryParamsRefusesAHeaderParameter(t *testing.T) {
	specParams := map[string]*openapi3.Parameter{
		"If-Match": {Name: "If-Match", In: openapi3.ParameterInHeader},
	}
	m := GoMethod{HTTPMethod: "PATCH", SpecPath: "/things", QueryParams: []ExtraParam{
		headerParam("If-Match", "ifMatch"),
	}}
	err := resolveQueryParams(&m, specParams)
	if err == nil {
		t.Fatal("a header parameter declared under \"params\" was accepted")
	}
	if !strings.Contains(err.Error(), `move it to "headerParams"`) {
		t.Errorf("error = %v, want it to point at headerParams", err)
	}
}

// A header param on a template with no headers form fails generation rather
// than shipping a signature that promises a header the request never carries.
func TestValidateHeaderParamSupportRefusesUnsupportedCategories(t *testing.T) {
	for _, category := range []string{"multipart", "raw", "unwrap", "paginated", "paginatedCursor", "apply"} {
		m := GoMethod{Category: category, HeaderParams: []ExtraParam{headerParam("If-Match", "ifMatch")}}
		if err := validateHeaderParamSupport(m); err == nil {
			t.Errorf("category %q accepted a header param", category)
		}
	}
	for category := range headerParamCategories {
		m := GoMethod{Category: category, HeaderParams: []ExtraParam{headerParam("If-Match", "ifMatch")}}
		if err := validateHeaderParamSupport(m); err != nil {
			t.Errorf("category %q refused a header param it supports: %v", category, err)
		}
	}
}

// The generator's copy of the transport's reserved-header list has to stay in
// step with it. tools/generate is its own module, so the list is duplicated
// rather than imported — this pins the duplication to the scope headers the
// transport actually stamps.
func TestGeneratorReservedHeadersCoverTheScopeHeaders(t *testing.T) {
	for _, h := range []string{"X-Tenant-Id", "X-Environment-Id", "Authorization"} {
		if !generatorReservedHeaders[http.CanonicalHeaderKey(h)] {
			t.Errorf("%s is stamped by the transport but the generator would emit it as an argument", h)
		}
	}
	for name := range generatorReservedHeaders {
		if name != http.CanonicalHeaderKey(name) {
			t.Errorf("reserved header %q is not in canonical form, so the lookup misses it", name)
		}
	}
}
