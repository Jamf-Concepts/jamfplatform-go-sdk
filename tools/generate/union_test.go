// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func schemaRef(name string) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Ref: "#/components/schemas/" + name, Value: openapi3.NewObjectSchema()}
}

// isRefUnion decides whether a property's `oneOf` becomes a typed union or
// keeps its old behaviour, and the old behaviour for a property was a bare
// `any` — see validateNoUntypedFields. Each rejection below is a shape that
// must keep going down its existing path, so a too-eager match here is a
// silent re-typing of something else's field.
func TestIsRefUnion(t *testing.T) {
	tests := []struct {
		name   string
		schema *openapi3.Schema
		want   bool
	}{
		{
			// account-sso's ConnectionRequest.connection.
			name:   "two or more bare $refs",
			schema: &openapi3.Schema{OneOf: openapi3.SchemaRefs{schemaRef("A"), schemaRef("B")}},
			want:   true,
		},
		{
			name:   "four bare $refs",
			schema: &openapi3.Schema{OneOf: openapi3.SchemaRefs{schemaRef("A"), schemaRef("B"), schemaRef("C"), schemaRef("D")}},
			want:   true,
		},
		{
			// 3.0's $ref-with-siblings idiom. isSingleRefAllOfWrapper and the
			// nullable collapse own this shape.
			name:   "single member is not a union",
			schema: &openapi3.Schema{OneOf: openapi3.SchemaRefs{schemaRef("A")}},
			want:   false,
		},
		{
			// normalizeNullableUnions has already rewritten this to a nullable
			// $ref by the time hoisting runs; matching it would resurrect the
			// union and give the field a pointless one-variant wrapper.
			name: "nullable oneOf is not a union",
			schema: &openapi3.Schema{OneOf: openapi3.SchemaRefs{
				schemaRef("A"),
				{Value: &openapi3.Schema{Type: types("null")}},
			}},
			want: false,
		},
		{
			// No named type to point a variant pointer at.
			name: "an inline member disqualifies the whole union",
			schema: &openapi3.Schema{OneOf: openapi3.SchemaRefs{
				schemaRef("A"),
				{Value: openapi3.NewObjectSchema()},
			}},
			want: false,
		},
		{
			// blueprints' SwUpdateConfiguration: type + oneOf + its own
			// properties. It goes down the object branch and must stay there.
			name: "explicit type disqualifies",
			schema: &openapi3.Schema{
				Type:  types("object"),
				OneOf: openapi3.SchemaRefs{schemaRef("A"), schemaRef("B")},
			},
			want: false,
		},
		{
			name: "own properties disqualify",
			schema: &openapi3.Schema{
				OneOf:      openapi3.SchemaRefs{schemaRef("A"), schemaRef("B")},
				Properties: openapi3.Schemas{"enforcementType": {Value: openapi3.NewStringSchema()}},
			},
			want: false,
		},
		{
			// A discriminated union is schemaToDiscriminatorType's job: it
			// produces a strictly better type, with dispatch on decode.
			name: "a discriminator disqualifies",
			schema: &openapi3.Schema{
				OneOf:         openapi3.SchemaRefs{schemaRef("A"), schemaRef("B")},
				Discriminator: &openapi3.Discriminator{PropertyName: "kind"},
			},
			want: false,
		},
		{
			name:   "no oneOf",
			schema: openapi3.NewObjectSchema(),
			want:   false,
		},
		{
			name:   "nil",
			schema: nil,
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRefUnion(tc.schema); got != tc.want {
				t.Fatalf("isRefUnion = %v, want %v", got, tc.want)
			}
		})
	}
}

// The hoist is what gives a property-reached union a name at all. Without it
// the schema is never a component, extractTypes never sees it, and
// schemaRefToGoType answers `any` for the containing field.
func TestHoistInlineObjectsLiftsRefUnionProperty(t *testing.T) {
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"ConnectionRequest": {Value: &openapi3.Schema{
			Type: types("object"),
			Properties: openapi3.Schemas{
				"connection": {Value: &openapi3.Schema{OneOf: openapi3.SchemaRefs{
					schemaRef("OidcConnectionSettings"),
					schemaRef("OktaConnectionSettings"),
				}}},
			},
		}},
		"OidcConnectionSettings": {Value: openapi3.NewObjectSchema()},
		"OktaConnectionSettings": {Value: openapi3.NewObjectSchema()},
	}}}

	hoistInlineObjects(doc, "json")

	hoisted, ok := doc.Components.Schemas["ConnectionRequestConnection"]
	if !ok {
		t.Fatal("union property was not hoisted into a named schema")
	}
	if !isRefUnion(hoisted.Value) {
		t.Fatal("hoisted schema is no longer a ref union")
	}
	prop := doc.Components.Schemas["ConnectionRequest"].Value.Properties["connection"]
	if prop.Ref != "#/components/schemas/ConnectionRequestConnection" {
		t.Fatalf("property $ref = %q, want the hoisted schema", prop.Ref)
	}
	if got := schemaRefToGoType(prop); got != "ConnectionRequestConnection" {
		t.Fatalf("field type = %q, want ConnectionRequestConnection (an %q here is the bug this fixes)", got, "any")
	}
}

// One pointer field per variant, named after the variant's own type, and the
// contract stated in the godoc — the spec's own description says only what the
// payload is, never that setting two is an error.
func TestSchemaToUnionType(t *testing.T) {
	schema := &openapi3.Schema{
		Description: "Provider settings, in the shape matching `connectionType`.",
		OneOf: openapi3.SchemaRefs{
			schemaRef("OidcConnectionSettings"),
			schemaRef("EntraConnectionSettings"),
			// A repeated $ref must not produce a duplicate field: the struct
			// would not compile.
			schemaRef("OidcConnectionSettings"),
		},
	}

	gt := schemaToUnionType("ConnectionRequestConnection", schema)
	if gt.Union == nil {
		t.Fatal("no union emitted")
	}
	want := []string{"OidcConnectionSettings", "EntraConnectionSettings"}
	if len(gt.Union.Variants) != len(want) {
		t.Fatalf("variants = %v, want %v", gt.Union.Variants, want)
	}
	for i, v := range gt.Union.Variants {
		if v.TypeName != want[i] || v.FieldName != want[i] {
			t.Errorf("variant %d = %+v, want type and field %q", i, v, want[i])
		}
	}
	if !strings.Contains(gt.Comment, "Provider settings") {
		t.Errorf("comment dropped the spec's description: %q", gt.Comment)
	}
	if !strings.Contains(gt.Comment, "Set exactly one variant pointer") {
		t.Errorf("comment does not state the contract: %q", gt.Comment)
	}
}

// A value the server produces and the spec omits is a broken validator, not a
// documentation gap: RegionValues() feeds stringvalidator.OneOf, so the
// consumer rejects the server's own output. Region gained RAMP this way.
func TestApplyEnumAdditions(t *testing.T) {
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"Region": {Value: &openapi3.Schema{Type: types("string"), Enum: []any{"US", "EU"}}},
	}}}

	applyEnumAdditions(doc, map[string][]string{"Region": {"RAMP"}})

	got := doc.Components.Schemas["Region"].Value.Enum
	if len(got) != 3 || got[2] != "RAMP" {
		t.Fatalf("enum = %v, want the addition appended after the spec's own values", got)
	}
}

// The entry has to expire when upstream publishes the value, and a duplicate
// enum member is invisible: two constants with the same value compile fine, so
// nothing else would ever notice.
func TestApplyEnumAdditionsPanicsWhenAlreadyDeclared(t *testing.T) {
	cases := map[string]openapi3.Schemas{
		"value already declared": {
			"Region": {Value: &openapi3.Schema{Type: types("string"), Enum: []any{"US", "RAMP"}}},
		},
		"schema declares no enum": {
			"Region": {Value: &openapi3.Schema{Type: types("string")}},
		},
		"no such schema": {},
	}
	for name, schemas := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("applyEnumAdditions returned without panicking")
				}
			}()
			applyEnumAdditions(&openapi3.T{Components: &openapi3.Components{Schemas: schemas}},
				map[string][]string{"Region": {"RAMP"}})
		})
	}
}

// A bare `any` field is the tell of a schema construct with no branch: it
// type-checks nothing and, the transport setting no DisallowUnknownFields,
// decodes anything. Container `any`s are what a genuinely freeform schema
// should produce and must not trip this.
func TestValidateNoUntypedFields(t *testing.T) {
	ok := []GoType{{Name: "T", Fields: []GoField{
		{Name: "Freeform", Type: "map[string]any", JSONTag: "freeform"},
		{Name: "Items", Type: "[]any", JSONTag: "items"},
		{Name: "Typed", Type: "*Settings", JSONTag: "typed"},
	}}}
	if err := validateNoUntypedFields("pkg", ok); err != nil {
		t.Fatalf("clean types rejected: %v", err)
	}

	for _, bad := range []string{"any", "*any"} {
		t.Run(bad, func(t *testing.T) {
			types := []GoType{{Name: "ConnectionRequest", Fields: []GoField{
				{Name: "Connection", Type: bad, JSONTag: "connection"},
			}}}
			err := validateNoUntypedFields("pkg", types)
			if err == nil {
				t.Fatalf("field typed %s was accepted", bad)
			}
			if !strings.Contains(err.Error(), "ConnectionRequest.Connection") {
				t.Errorf("error does not name the field: %v", err)
			}
		})
	}
}
