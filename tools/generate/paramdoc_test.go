// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestStripSpecHTML(t *testing.T) {
	cases := map[string]string{
		"Includes all results.</br> Fields allowed:": "Includes all results.  Fields allowed:",
		"one<br/>two": "one two",
		"a &amp; b":   "a & b",
		// Angle brackets used as placeholder syntax are not HTML and must
		// survive — Jamf documents sort as "<field_name>:asc".
		"format <field_name>:asc": "format <field_name>:asc",
		"plain text":              "plain text",
	}
	for in, want := range cases {
		if got := stripSpecHTML(in); got != want {
			t.Errorf("stripSpecHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWrapCommentText(t *testing.T) {
	got := wrapCommentText("alpha bravo charlie delta echo", 12)
	want := []string{"alpha bravo", "charlie", "delta echo"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
	if lines := wrapCommentText("   ", 10); lines != nil {
		t.Errorf("blank input produced %q, want nil", lines)
	}
	// A word longer than the width is never split.
	long := "supercalifragilisticexpialidocious"
	if lines := wrapCommentText(long, 10); len(lines) != 1 || lines[0] != long {
		t.Errorf("oversized word wrapped to %q", lines)
	}
}

func TestParameterEnumValues(t *testing.T) {
	strEnum := &openapi3.SchemaRef{Value: &openapi3.Schema{Enum: []any{"General", "Pending+Failed"}}}
	if got := parameterEnumValues(strEnum); strings.Join(got, ",") != `"General","Pending+Failed"` {
		t.Errorf("string enum = %q", got)
	}

	// Repeatable params carry the constraint on the item schema.
	itemEnum := &openapi3.SchemaRef{Value: &openapi3.Schema{
		Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Enum: []any{"asc", "desc"}}},
	}}
	if got := parameterEnumValues(itemEnum); strings.Join(got, ",") != `"asc","desc"` {
		t.Errorf("item enum = %q", got)
	}

	if got := parameterEnumValues(nil); got != nil {
		t.Errorf("nil schema = %q, want nil", got)
	}
	if got := parameterEnumValues(&openapi3.SchemaRef{Value: &openapi3.Schema{}}); len(got) != 0 {
		t.Errorf("no enum = %q, want empty", got)
	}
}

func TestParameterComment(t *testing.T) {
	param := func(name, desc string, enum ...any) *openapi3.ParameterRef {
		p := &openapi3.Parameter{Name: name, Description: desc}
		if len(enum) > 0 {
			p.Schema = &openapi3.SchemaRef{Value: &openapi3.Schema{Enum: enum}}
		}
		return &openapi3.ParameterRef{Value: p}
	}

	m := GoMethod{
		PathParams:  []GoPathParam{{SpecName: "id", GoName: "id"}, {SpecName: "subset", GoName: "subset"}},
		QueryParams: []ExtraParam{{Spec: "filter", Go: "filter"}, {Spec: "undocumented-key", Go: "undocumentedKey"}},
	}
	pathItem := &openapi3.PathItem{Parameters: openapi3.Parameters{param("id", "ID to filter by")}}
	op := &openapi3.Operation{Parameters: openapi3.Parameters{
		param("subset", "Subset to filter by", "General", "Location"),
		param("filter", "RSQL query.\n\nFields allowed in the query: name"),
		param("ignored", "Not in the Go signature, must not appear"),
	}}

	got := parameterComment(m, pathItem, op)
	wantLines := []string{
		"\n//\n// Parameters:",
		"//   - id: ID to filter by.",
		"//   - subset: Subset to filter by.",
		`//     Allowed values: "General", "Location".`,
		"//   - filter: RSQL query.",
		"//     Fields allowed in the query: name.",
	}
	if want := strings.Join(wantLines, "\n"); got != want {
		t.Errorf("parameterComment mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	// Params declared in config but absent from the spec are skipped, not
	// emitted as empty entries.
	if strings.Contains(got, "undocumentedKey") {
		t.Error("config-only param leaked into the comment")
	}

	// Nothing documentable → no block at all, so the method keeps its
	// single-line godoc.
	bare := GoMethod{PathParams: []GoPathParam{{SpecName: "id", GoName: "id"}}}
	bareItem := &openapi3.PathItem{Parameters: openapi3.Parameters{param("id", "")}}
	if got := parameterComment(bare, bareItem, &openapi3.Operation{}); got != "" {
		t.Errorf("undescribed param produced %q, want empty", got)
	}
}

func TestDefaultMethodComment(t *testing.T) {
	if got := defaultMethodComment("GetX", SpecDef{}); got != "GetX calls a Jamf Platform API endpoint." {
		t.Errorf("documented spec = %q", got)
	}
	if got := defaultMethodComment("GetX", SpecDef{Undocumented: true}); got != "GetX calls an undocumented Jamf endpoint." {
		t.Errorf("undocumented spec = %q", got)
	}
}
