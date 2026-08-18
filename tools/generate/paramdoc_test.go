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
		QueryParams: []ExtraParam{{Spec: "filter", Go: "filter"}, {Spec: "undocumented-key", Go: "undocumentedKey", Undocumented: true}},
	}
	pathItem := &openapi3.PathItem{Parameters: openapi3.Parameters{param("id", "ID to filter by")}}
	op := &openapi3.Operation{Parameters: openapi3.Parameters{
		param("subset", "Subset to filter by", "General", "Location"),
		param("filter", "RSQL query.\n\nFields allowed in the query: name"),
		param("ignored", "Not in the Go signature, must not appear"),
	}}

	got, err := parameterComment(m, pathItem, op, nil)
	if err != nil {
		t.Fatalf("parameterComment: %v", err)
	}
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
	// A param declared ":undocumented" is skipped, not emitted as an empty
	// entry — the spec genuinely says nothing about it.
	if strings.Contains(got, "undocumentedKey") {
		t.Error("undocumented param leaked into the comment")
	}

	// Nothing documentable → no block at all, so the method keeps its
	// single-line godoc.
	bare := GoMethod{PathParams: []GoPathParam{{SpecName: "id", GoName: "id"}}}
	bareItem := &openapi3.PathItem{Parameters: openapi3.Parameters{param("id", "")}}
	if got, err := parameterComment(bare, bareItem, &openapi3.Operation{}, nil); err != nil || got != "" {
		t.Errorf("undescribed param produced (%q, %v), want (\"\", nil)", got, err)
	}
}

func TestParameterCommentRejectsUnmatchedQueryParam(t *testing.T) {
	m := GoMethod{
		HTTPMethod:  "GET",
		SpecPath:    "/v1/tenant/{tenantId}/rules",
		QueryParams: []ExtraParam{{Spec: "baselineId", Go: "baselineID"}},
	}
	op := &openapi3.Operation{Parameters: openapi3.Parameters{
		{Value: &openapi3.Parameter{Name: "baseline-id", Description: "Given baseline ID"}},
	}}

	got, err := parameterComment(m, nil, op, nil)
	if err == nil {
		t.Fatal("expected an error for a config param absent from the spec, got nil")
	}
	if !strings.Contains(err.Error(), "baselineId") || !strings.Contains(err.Error(), "baseline-id") {
		t.Errorf("error %q should name both the unmatched config param and the spec's actual param", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string alongside the error", got)
	}

	// Marking the same mismatch ":undocumented" opts it out of the check.
	m.QueryParams[0].Undocumented = true
	if _, err := parameterComment(m, nil, op, nil); err != nil {
		t.Errorf("undocumented opt-out should suppress the error, got %v", err)
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

	got, err := parameterComment(m, nil, op, map[string]bool{"NotificationType": true})
	if err != nil {
		t.Fatalf("parameterComment: %v", err)
	}
	if !strings.Contains(got, "Allowed values: see the NotificationType constants.") {
		t.Errorf("expected a pointer to the emitted type, got:\n%s", got)
	}
	if strings.Contains(got, "APNS_CERT_REVOKED") {
		t.Error("values were listed inline despite an emitted type existing")
	}

	// The same schema when NO type is emitted for it (enum reachable only from
	// a parameter) must fall back to the inline list.
	got, err = parameterComment(m, nil, op, nil)
	if err != nil {
		t.Fatalf("parameterComment: %v", err)
	}
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

func TestRegisterPropertyEnum(t *testing.T) {
	strEnum := func(vals ...any) *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{"string"}, Enum: vals,
		}}
	}

	currentPropertyEnums = map[string]GoType{}
	currentSpecTypeNames = map[string]bool{"AccountAccessLevel": true}
	t.Cleanup(func() { currentPropertyEnums, currentSpecTypeNames = nil, nil })

	// Happy path: <Owner><Property>, registered and referenced.
	if got := registerPropertyEnum("Policy", "trigger", strEnum("EVENT", "USER_INITIATED")); got != "PolicyTrigger" {
		t.Errorf("got %q, want PolicyTrigger", got)
	}
	reg, ok := currentPropertyEnums["PolicyTrigger"]
	if !ok || len(reg.EnumValues) != 2 {
		t.Fatalf("PolicyTrigger not registered correctly: %+v", reg)
	}

	// A name the spec already declares must be abandoned, not referenced —
	// otherwise the field points at a type carrying no constants.
	if got := registerPropertyEnum("Account", "accessLevel", strEnum("FullAccess", "SiteAccess")); got != "" {
		t.Errorf("collision returned %q, want empty", got)
	}
	if _, dup := currentPropertyEnums["AccountAccessLevel"]; dup {
		t.Error("collision was registered anyway")
	}

	// Skips: $ref (field type already names it), non-string, single value.
	cases := map[string]*openapi3.SchemaRef{
		"ref":         {Ref: "#/components/schemas/NotificationType", Value: &openapi3.Schema{Enum: []any{"A", "B"}}},
		"non-string":  {Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Enum: []any{1, 2}}},
		"single":      strEnum("ONLY"),
		"no enum":     strEnum(),
		"nil":         nil,
		"item is ref": {Value: &openapi3.Schema{Items: &openapi3.SchemaRef{Ref: "#/components/schemas/X", Value: &openapi3.Schema{Enum: []any{"A", "B"}}}}},
	}
	for label, ref := range cases {
		if got := registerPropertyEnum("Owner", label, ref); got != "" {
			t.Errorf("%s: got %q, want empty", label, got)
		}
	}

	// A repeatable property carries the constraint on its inline item schema.
	items := &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type:  &openapi3.Types{"array"},
		Items: strEnum("MAC", "IOS"),
	}}
	if got := registerPropertyEnum("Device", "platforms", items); got != "DevicePlatforms" {
		t.Errorf("item enum: got %q, want DevicePlatforms", got)
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
