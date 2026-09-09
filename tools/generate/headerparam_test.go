// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"sort"
	"strconv"
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
		// Authorization is refused for a different reason than the scope
		// headers — oauth2 writes it and WithAuthorizationHeaderName may
		// relocate it — so it gets its own case rather than resting on the
		// map-membership check, which would pass while the refusal itself
		// was broken.
		{"authorization header", []ExtraParam{headerParam("Authorization", "auth")}, "stamped by the transport"},
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

// The generator's copy of the transport's scope headers has to stay in step
// with it, and tools/generate cannot import internal/client to check — its own
// generator importing the SDK would make the build circular.
//
// So this parses ScopeKind.ScopeHeader() out of internal/client/client.go and
// requires every header that switch can return to be present here. Pinning
// against ScopeHeader rather than against internal/client's own
// reservedHeaders is deliberate: that map is itself derived from ScopeHeader,
// so ScopeHeader is the source of truth and a pin against the derived copy
// would miss a scope kind that gained a header without the map being updated.
//
// A hardcoded list here would be no pin at all — it would assert the
// generator's literal against another literal in the same package and pass
// however far internal/client had drifted.
func TestGeneratorReservedHeadersPinScopeHeaders(t *testing.T) {
	headers := scopeHeadersFromTransportSource(t)
	if len(headers) < 2 {
		t.Fatalf("parsed %d scope headers out of ScopeHeader(), want at least the two known ones — the parse has broken and this test is no longer pinning anything", len(headers))
	}
	for _, h := range headers {
		if !generatorReservedHeaders[http.CanonicalHeaderKey(h)] {
			t.Errorf("ScopeKind.ScopeHeader() can return %q, but generatorReservedHeaders does not list it — the generator would emit it as a method argument, and doRequestFull applies extra headers after setScopeHeader, so the argument would overwrite the scope and produce 403 OWNERSHIP_FORBIDDEN", h)
		}
	}
	for name := range generatorReservedHeaders {
		if name != http.CanonicalHeaderKey(name) {
			t.Errorf("reserved header %q is not in canonical form, so the lookup misses it", name)
		}
	}
}

// scopeHeadersFromTransportSource returns every string literal
// ScopeKind.ScopeHeader() can return, read from the transport's source rather
// than from a copy of it. Empty returns (the organization case, which has no
// header) are skipped.
func scopeHeadersFromTransportSource(t *testing.T) []string {
	t.Helper()
	const src = "../../internal/client/client.go"
	file, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v — this test pins the generator against the transport, so it must fail rather than skip", src, err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		d, ok := decl.(*ast.FuncDecl)
		if ok && d.Name.Name == "ScopeHeader" && d.Recv != nil {
			fn = d
			break
		}
	}
	if fn == nil {
		t.Fatalf("no ScopeHeader method found in %s — it was renamed or moved, and this pin has to follow it", src)
	}

	var out []string
	ast.Inspect(fn, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		lit, ok := ret.Results[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(lit.Value)
		if err != nil || v == "" {
			return true
		}
		out = append(out, v)
		return true
	})
	sort.Strings(out)
	return out
}

// A resolver or apply built over a header-bearing operation is refused. The
// synthetic methods never copy HeaderParams and their templates have no
// headers form, so without this the header is dropped with nothing failing —
// and validateHeaderParamSupport cannot catch it, because these methods are
// assembled field by field rather than through buildMethod.
func TestRefuseHeaderParamsOnSynthetic(t *testing.T) {
	withHeader := &GoMethod{
		Name:         "UpdateThing",
		HeaderParams: []ExtraParam{headerParam("If-Match", "ifMatch")},
	}
	for _, kind := range []string{"resolver", "apply"} {
		err := refuseHeaderParamsOnSynthetic(kind, "Thing", "UpdateThing", withHeader)
		if err == nil {
			t.Fatalf("%s over a header-bearing op was accepted", kind)
		}
		for _, want := range []string{"If-Match", "silently dropped"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error = %v, want it to mention %q", kind, err, want)
			}
		}
	}

	// The common case must stay silent, or every existing resolver fails.
	if err := refuseHeaderParamsOnSynthetic("resolver", "Thing", "ListThings", &GoMethod{Name: "ListThings"}); err != nil {
		t.Errorf("a header-free source operation was refused: %v", err)
	}
	if err := refuseHeaderParamsOnSynthetic("apply", "Thing", "Missing", nil); err != nil {
		t.Errorf("a nil source operation was refused rather than left to the caller's own lookup check: %v", err)
	}
}

// wireRequiredParams is documentation for a parameter the signature cannot
// describe, so both of its refusals are what keep the note from outliving its
// own truth.
func TestResolveWireRequiredParams(t *testing.T) {
	optional := &openapi3.Parameter{Name: "accept", In: openapi3.ParameterInHeader}
	required := &openapi3.Parameter{Name: "accept", In: openapi3.ParameterInHeader, Required: true}

	t.Run("records an optional parameter", func(t *testing.T) {
		m := GoMethod{HTTPMethod: "GET", SpecPath: "/things"}
		if err := resolveWireRequiredParams(&m, []string{"accept"}, map[string]*openapi3.Parameter{"accept": optional}); err != nil {
			t.Fatalf("an optional parameter was refused: %v", err)
		}
		if !m.WireRequiredParams["accept"] {
			t.Error("accept was not recorded")
		}
		if lines := wireRequiredDocLines("accept", m); len(lines) == 0 || !strings.Contains(strings.Join(lines, " "), "400") {
			t.Errorf("doc lines = %v, want them to name the status the server answers", lines)
		}
	})

	// The self-expiry. Once the spec marks the parameter required its own
	// declaration says so, and the note would be restating it.
	t.Run("expires when the spec marks it required", func(t *testing.T) {
		m := GoMethod{HTTPMethod: "GET", SpecPath: "/things"}
		err := resolveWireRequiredParams(&m, []string{"accept"}, map[string]*openapi3.Parameter{"accept": required})
		if err == nil {
			t.Fatal("a now-required parameter was accepted, so the note will outlive the defect it records")
		}
		if !strings.Contains(err.Error(), "delete the entry") {
			t.Errorf("error = %v, want it to say to delete the entry", err)
		}
	})

	t.Run("refuses a name the spec does not declare", func(t *testing.T) {
		m := GoMethod{HTTPMethod: "GET", SpecPath: "/things"}
		err := resolveWireRequiredParams(&m, []string{"acccept"}, map[string]*openapi3.Parameter{"accept": optional})
		if err == nil {
			t.Fatal("a misspelled name was accepted, so the note would document nothing")
		}
		if !strings.Contains(err.Error(), "does not declare") {
			t.Errorf("error = %v, want it to report the name as undeclared", err)
		}
	})

	// It must not change what is emitted: sending the parameter empty is not
	// better than omitting it, so the guard stays.
	t.Run("does not flip AlwaysSend", func(t *testing.T) {
		m := GoMethod{
			Name: "ProbeOp", Category: "get", HTTPMethod: "GET",
			Namespace: "probe", Version: "v1", ResourcePath: "/things",
			ResponseType: "ThingResponse",
			HeaderParams: []ExtraParam{headerParam("accept", "accept")},
		}
		if err := resolveWireRequiredParams(&m, []string{"accept"}, map[string]*openapi3.Parameter{"accept": optional}); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := sourceTmpl.ExecuteTemplate(&buf, "get", m); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), `if accept != "" {`) {
			t.Errorf("wireRequiredParams removed the zero-value guard, so a caller passing \"\" now sends an empty header:\n%s", buf.String())
		}
	})
}
