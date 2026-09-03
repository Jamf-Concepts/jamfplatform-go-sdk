// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// queryParamCategories are every method template that emits a query string.
// Six route through the shared "buildQueryParams" sub-template; "paginated"
// and "paginatedCursor" carry their own copies of the same logic, inside the
// fetch-page closure, and are the reason a fix applied in one place used to
// miss two others.
var queryParamCategories = []string{
	"get", "create", "actionWithResponse", "update", "action", "unwrap",
	"paginated", "paginatedCursor",
}

func renderCategory(t *testing.T, category string, params []ExtraParam) string {
	t.Helper()
	m := GoMethod{
		Name:           "ProbeOp",
		Category:       category,
		HTTPMethod:     "GET",
		Namespace:      "probe",
		Version:        "v1",
		ResourcePath:   "/things",
		ResponseType:   "ThingResponse",
		RequestType:    "ThingRequest",
		ExpectedStatus: 200,
		ItemType:       "Thing",
		ResultsField:   "results",
		CursorField:    "nextCursor",
		CursorParam:    "cursor",
		PageSizeParam:  "page-size",
		MaxPageSize:    100,
		UnwrapResults:  "[]Thing",
		QueryParams:    params,
	}
	var buf bytes.Buffer
	if err := sourceTmpl.ExecuteTemplate(&buf, category, m); err != nil {
		t.Fatalf("render %s: %v", category, err)
	}
	return buf.String()
}

// setLineIsGuarded reports whether the params.Set call for a given wire name is
// wrapped in a zero-value guard, by looking at whether the line above it opens
// an if block. Returns false for "emitted unguarded" and errors the test when
// the param was not emitted at all.
func setLineIsGuarded(t *testing.T, src, specName string) bool {
	t.Helper()
	needle := `params.Set("` + specName + `"`
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if !strings.Contains(line, needle) {
			continue
		}
		if i == 0 {
			return false
		}
		prev := strings.TrimSpace(lines[i-1])
		return strings.HasPrefix(prev, "if ") && strings.HasSuffix(prev, "{")
	}
	t.Fatalf("params.Set for %q not emitted at all:\n%s", specName, src)
	return false
}

// TestRequiredQueryParamsAreEmittedUnguarded is the bug-class guard, not a
// per-param assertion. Every query parameter used to be emitted behind a
// zero-value guard, spec-required ones included, so a caller passing the zero
// value sent a request with a required parameter silently missing and got back
// a 400 whose wording ("Required parameter 'origin' is not present.") reads
// like a server or auth fault rather than a caller error. The guard belongs on
// optional params only — dropping an unset optional param is correct and
// load-bearing — so both directions are asserted, for every type branch, in
// every template that emits a query string.
func TestRequiredQueryParamsAreEmittedUnguarded(t *testing.T) {
	types := []string{"string", "[]string", "bool", "int", "int64"}
	for _, category := range queryParamCategories {
		t.Run(category, func(t *testing.T) {
			for _, typ := range types {
				params := []ExtraParam{
					{Spec: "req", Go: "req", Type: typ, AlwaysSend: true},
					{Spec: "opt", Go: "opt", Type: typ},
				}
				src := renderCategory(t, category, params)
				if setLineIsGuarded(t, src, "req") {
					t.Errorf("%s param is required but emitted behind a zero-value guard:\n%s", typ, src)
				}
				if !setLineIsGuarded(t, src, "opt") {
					t.Errorf("%s param is optional but emitted unguarded:\n%s", typ, src)
				}
			}
		})
	}
}

// TestRequiredQueryParamValueExpressions pins the conversion each type gets.
// The bool row is the one that is not mechanical: an optional bool is only
// ever emitted when true, so "true" as a literal is exact — but a required
// bool has to carry a false the caller meant, which is why it formats the
// argument instead.
func TestRequiredQueryParamValueExpressions(t *testing.T) {
	cases := []struct {
		typ      string
		required string
		optional string
	}{
		{"string", "req", "opt"},
		{"[]string", `strings.Join(req, ",")`, `strings.Join(opt, ",")`},
		{"bool", "strconv.FormatBool(req)", `"true"`},
		{"int", "strconv.Itoa(req)", "strconv.Itoa(opt)"},
		{"int64", "strconv.FormatInt(req, 10)", "strconv.FormatInt(opt, 10)"},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			src := renderCategory(t, "get", []ExtraParam{
				{Spec: "req", Go: "req", Type: c.typ, AlwaysSend: true},
				{Spec: "opt", Go: "opt", Type: c.typ},
			})
			if want := `params.Set("req", ` + c.required + `)`; !strings.Contains(src, want) {
				t.Errorf("missing %q in:\n%s", want, src)
			}
			if want := `params.Set("opt", ` + c.optional + `)`; !strings.Contains(src, want) {
				t.Errorf("missing %q in:\n%s", want, src)
			}
		})
	}
}

// TestOptionalQueryParamGuardsUnchanged pins the exact guard expression per
// type. Loosening one of these is how an optional param starts travelling as
// an empty value, which several Jamf endpoints treat differently from an
// absent one.
func TestOptionalQueryParamGuardsUnchanged(t *testing.T) {
	want := map[string]string{
		"string":   `if opt != "" {`,
		"[]string": `if len(opt) > 0 {`,
		"bool":     `if opt {`,
		"int":      `if opt != 0 {`,
		"int64":    `if opt != 0 {`,
	}
	for typ, guard := range want {
		src := renderCategory(t, "get", []ExtraParam{{Spec: "opt", Go: "opt", Type: typ}})
		if !strings.Contains(src, guard) {
			t.Errorf("%s: missing guard %q in:\n%s", typ, guard, src)
		}
	}
}

// TestGeneratedTestAssertsRequiredQueryParams pins the second layer: the
// generated httptest stub calls every method with zero-value arguments, so a
// required param emitted behind a zero-value guard is absent from the request
// the handler sees. The emitted Has() check turns that into a failing unit
// test in the shipped suite for every required param the specs declare, now
// and after the next ingest — the per-method half of what this file's first
// test does at the template level.
func TestGeneratedTestAssertsRequiredQueryParams(t *testing.T) {
	m := GoMethod{
		Name:           "ProbeOp",
		Category:       "get",
		HTTPMethod:     "GET",
		Namespace:      "probe",
		Version:        "v1",
		ResourcePath:   "/things",
		ResponseType:   "ThingResponse",
		ExpectedStatus: 200,
		QueryParams: []ExtraParam{
			{Spec: "origin", Go: "origin", Type: "string", AlwaysSend: true},
			{Spec: "sort", Go: "sort", Type: "[]string"},
		},
	}
	var buf bytes.Buffer
	if err := testTmpl.ExecuteTemplate(&buf, "testGet", m); err != nil {
		t.Fatalf("render testGet: %v", err)
	}
	src := buf.String()
	if !strings.Contains(src, `if !r.URL.Query().Has("origin")`) {
		t.Errorf("generated test does not assert the required param reached the wire:\n%s", src)
	}
	if strings.Contains(src, `Has("sort")`) {
		t.Errorf("generated test asserts an OPTIONAL param is present:\n%s", src)
	}
}
