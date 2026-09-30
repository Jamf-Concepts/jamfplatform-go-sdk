// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// itemUnionDoc is the v2362 blueprints shape reduced to what matters: a
// Component-style envelope with a content-addressed discriminator and
// properties of its own, a configuration whose array items are an inline
// oneOf with a declared, content-addressed discriminator, and a pure map
// schema.
const itemUnionDoc = `
openapi: 3.0.1
info: {title: t, version: "1"}
paths: {}
components:
  schemas:
    Envelope:
      type: object
      required: [identifier]
      properties:
        identifier: {type: string}
        configuration: {type: object}
      oneOf:
        - $ref: '#/components/schemas/Holder'
      discriminator:
        propertyName: identifier
        mapping:
          com.jamf.ddm-strict: '#/components/schemas/Holder'
    Holder:
      type: object
      required: [declarations]
      properties:
        declarations:
          type: array
          items:
            oneOf:
              - $ref: '#/components/schemas/PkgDecl'
              - $ref: '#/components/schemas/SiriDecl'
            discriminator:
              propertyName: type
              mapping:
                com.apple.configuration.package: '#/components/schemas/PkgDecl'
                com.apple.configuration.siri.settings: '#/components/schemas/SiriDecl'
    PkgDecl:
      type: object
      properties:
        type: {type: string, enum: [com.apple.configuration.package]}
        payload: {$ref: '#/components/schemas/PermMap'}
    SiriDecl:
      type: object
      properties:
        type: {type: string, enum: [com.apple.configuration.siri.settings]}
    PermMap:
      type: object
      additionalProperties:
        $ref: '#/components/schemas/Perm'
    Perm:
      type: object
      properties:
        Camera: {type: string}
`

func itemUnionTypes(t *testing.T) map[string]GoType {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(itemUnionDoc))
	if err != nil {
		t.Fatal(err)
	}
	hoistInlineObjects(doc, "json")
	allow := map[string]*schemaUsage{}
	for name := range doc.Components.Schemas {
		allow[name] = &schemaUsage{isRequest: true, isResponse: true}
	}
	out := map[string]GoType{}
	for _, gt := range extractTypes(doc, allow, "json") {
		out[gt.Name] = gt
	}
	return out
}

// v2362's DeclarationsComponentConfiguration.declarations generated as []any,
// with all eleven declaration types emitted and referenced by nothing.
func TestInlineArrayItemUnionIsHoistedAndTyped(t *testing.T) {
	types := itemUnionTypes(t)
	holder, ok := types["Holder"]
	if !ok {
		t.Fatal("Holder not emitted")
	}
	if len(holder.Fields) != 1 || holder.Fields[0].Type != "[]HolderDeclarationsItem" {
		t.Fatalf("Holder.declarations = %+v, want []HolderDeclarationsItem", holder.Fields)
	}
	item, ok := types["HolderDeclarationsItem"]
	if !ok || item.Discriminator == nil {
		t.Fatalf("HolderDeclarationsItem is not a discriminated union: %+v", item)
	}
	got := map[string]string{}
	for _, v := range item.Discriminator.Variants {
		for _, val := range v.Values {
			got[val] = v.FieldName
		}
	}
	// A dotted key names its field after the schema, not the key.
	want := map[string]string{
		"com.apple.configuration.package":       "PkgDecl",
		"com.apple.configuration.siri.settings": "SiriDecl",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("variant for %q = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}

// The dotted-key refusal still holds on an envelope: blueprints' Component
// carries identifier plus a json.RawMessage configuration that consumers
// decode themselves, and flipping it to a union would break every one.
func TestContentAddressedEnvelopeStaysAStruct(t *testing.T) {
	env := itemUnionTypes(t)["Envelope"]
	if env.Discriminator != nil || env.Union != nil {
		t.Fatalf("Envelope became a union: %+v", env)
	}
	if len(env.Fields) != 2 {
		t.Fatalf("Envelope fields = %+v, want identifier and configuration", env.Fields)
	}
}

// A field-less struct renders through the template's enum-alias branch as
// `= string`, which cannot decode the object the wire carries. v2362 shipped
// two: AppSettingsPermissionDefaultsMap and ManagedAppExtensionConfigs.
func TestPureMapSchemaIsAMapAlias(t *testing.T) {
	m := itemUnionTypes(t)["PermMap"]
	if m.AliasTarget != "map[string]Perm" {
		t.Fatalf("PermMap alias = %q, want map[string]Perm (fields %v)", m.AliasTarget, m.Fields)
	}
}

func TestIsInlineItemUnion(t *testing.T) {
	two := openapi3.SchemaRefs{schemaRef("A"), schemaRef("B")}
	tests := []struct {
		name   string
		schema *openapi3.Schema
		want   bool
	}{
		{"refs only", &openapi3.Schema{OneOf: two}, true},
		{"refs plus discriminator", &openapi3.Schema{OneOf: two, Discriminator: &openapi3.Discriminator{PropertyName: "type"}}, true},
		{"single ref", &openapi3.Schema{OneOf: openapi3.SchemaRefs{schemaRef("A")}}, false},
		{"own properties", &openapi3.Schema{OneOf: two, Properties: openapi3.Schemas{"x": {Value: openapi3.NewStringSchema()}}}, false},
		{"inline member", &openapi3.Schema{OneOf: openapi3.SchemaRefs{schemaRef("A"), {Value: openapi3.NewObjectSchema()}}}, false},
		{"scalar type", &openapi3.Schema{OneOf: two, Type: types("string")}, false},
	}
	for _, tt := range tests {
		if got := isInlineItemUnion(tt.schema); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
