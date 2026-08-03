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

	got := parameterComment(m, pathItem, op, nil)
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
	if got := parameterComment(bare, bareItem, &openapi3.Operation{}, nil); got != "" {
		t.Errorf("undescribed param produced %q, want empty", got)
	}
}

func TestParameterCommentNamesEmittedEnumTypes(t *testing.T) {
	// A $ref'd enum schema that the generator emits as a type is named, not
	// re-listed; the constants under that type carry the values.
	refd := &openapi3.ParameterRef{Value: &openapi3.Parameter{
		Name:        "notificationType",
		Description: "type of the notification",
		Schema: &openapi3.SchemaRef{
			Ref:   "#/components/schemas/NotificationType",
			Value: &openapi3.Schema{Enum: []any{"APNS_CERT_REVOKED", "GSX_CERT_EXPIRED"}},
		},
	}}
	m := GoMethod{PathParams: []GoPathParam{{SpecName: "notificationType", GoName: "notificationType"}}}
	op := &openapi3.Operation{Parameters: openapi3.Parameters{refd}}

	got := parameterComment(m, nil, op, map[string]bool{"NotificationType": true})
	if !strings.Contains(got, "Allowed values: see the NotificationType constants.") {
		t.Errorf("expected a pointer to the emitted type, got:\n%s", got)
	}
	if strings.Contains(got, "APNS_CERT_REVOKED") {
		t.Error("values were listed inline despite an emitted type existing")
	}

	// The same schema when NO type is emitted for it (enum reachable only from
	// a parameter) must fall back to the inline list.
	got = parameterComment(m, nil, op, nil)
	if !strings.Contains(got, `"APNS_CERT_REVOKED", "GSX_CERT_EXPIRED"`) {
		t.Errorf("expected an inline list with no emitted type, got:\n%s", got)
	}
}

func TestEnumConstIdent(t *testing.T) {
	cases := map[string]string{
		"APNS_CERT_REVOKED":                   "ApnsCertRevoked",
		"APPLE_SCHOOL_MANAGER_T_C_NOT_SIGNED": "AppleSchoolManagerTCNotSigned",
		"mdm-byod":                            "MDMByod",
		"General":                             "General",
		"Pending+Failed":                      "PendingFailed",
		// A leading digit is fine — the fragment is only ever a suffix.
		"10.15": "1015",
		// No alphanumerics at all → caller skips and logs.
		"+/+": "",
		"":    "",
	}
	for in, want := range cases {
		if got := enumConstIdent(in); got != want {
			t.Errorf("enumConstIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnumConsts(t *testing.T) {
	got := enumConsts("Subset", []any{"General", "Location", 42, ""})
	if len(got) != 2 {
		t.Fatalf("got %d consts, want 2 (non-string and empty values dropped): %+v", len(got), got)
	}
	if got[0].Name != "SubsetGeneral" || got[0].Value != "General" {
		t.Errorf("first const = %+v", got[0])
	}

	// Two values colliding on one identifier: the first wins, the second is
	// skipped rather than shadowing it.
	coll := enumConsts("T", []any{"FOO_BAR", "foo.bar"})
	if len(coll) != 1 || coll[0].Value != "FOO_BAR" {
		t.Errorf("collision handling produced %+v", coll)
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
