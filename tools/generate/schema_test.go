// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func types(names ...string) *openapi3.Types {
	t := openapi3.Types(names)
	return &t
}

// A single-member allOf is OpenAPI 3.0's only way to hang a description,
// `nullable` or an `example` off a $ref. Before this collapse the wrapper fell
// through schemaRefToGoType's default branch to `any`, which decodes anything
// and so never fails a test — the failure mode this exercises is silence.
func TestSchemaRefToGoTypeCollapsesSingleRefAllOf(t *testing.T) {
	target := &openapi3.SchemaRef{Ref: "#/components/schemas/DeploymentRun", Value: openapi3.NewObjectSchema()}

	tests := []struct {
		name   string
		schema *openapi3.Schema
		want   string
	}{
		{
			name:   "nullable wrapper collapses to the referenced type",
			schema: &openapi3.Schema{Nullable: true, Description: "d", AllOf: openapi3.SchemaRefs{target}},
			want:   "DeploymentRun",
		},
		{
			name:   "bare wrapper collapses",
			schema: &openapi3.Schema{AllOf: openapi3.SchemaRefs{target}},
			want:   "DeploymentRun",
		},
		{
			// Two members are a real composition: no single Go type names it,
			// and picking either one would silently discard the other's fields.
			name:   "multi-member allOf stays any",
			schema: &openapi3.Schema{AllOf: openapi3.SchemaRefs{target, target}},
			want:   "any",
		},
		{
			// allOf + own properties is the extend-a-base-schema shape
			// (PolicyDetail over PolicySummary). extractTypes flattens those
			// into their own struct, so collapsing here would drop the
			// extension's fields.
			name: "allOf with own properties stays any",
			schema: &openapi3.Schema{
				AllOf:      openapi3.SchemaRefs{target},
				Properties: openapi3.Schemas{"extra": {Value: openapi3.NewStringSchema()}},
			},
			want: "any",
		},
		{
			// An explicit type means the schema asserts its own shape; the
			// existing object branch answers map[string]any and must win.
			name:   "explicit object type is not collapsed",
			schema: &openapi3.Schema{Type: types("object"), AllOf: openapi3.SchemaRefs{target}},
			want:   "map[string]any",
		},
		{
			name: "allOf beside additionalProperties stays any",
			schema: &openapi3.Schema{
				AllOf: openapi3.SchemaRefs{target},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
				},
			},
			want: "any",
		},
		{
			name:   "allOf beside an enum stays any",
			schema: &openapi3.Schema{AllOf: openapi3.SchemaRefs{target}, Enum: []any{"A"}},
			want:   "any",
		},
		{
			name:   "allOf beside a oneOf stays any",
			schema: &openapi3.Schema{AllOf: openapi3.SchemaRefs{target}, OneOf: openapi3.SchemaRefs{target}},
			want:   "any",
		},
		{
			// Nesting is legal in 3.0 and the inner wrapper is the same idiom.
			name: "nested wrappers collapse through",
			schema: &openapi3.Schema{AllOf: openapi3.SchemaRefs{
				{Value: &openapi3.Schema{Nullable: true, AllOf: openapi3.SchemaRefs{target}}},
			}},
			want: "DeploymentRun",
		},
		{
			// A wrapper around an inline scalar still has a Go type to name.
			name: "wrapper around an inline scalar collapses",
			schema: &openapi3.Schema{AllOf: openapi3.SchemaRefs{
				{Value: openapi3.NewStringSchema()},
			}},
			want: "string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := schemaRefToGoType(&openapi3.SchemaRef{Value: tc.schema}); got != tc.want {
				t.Fatalf("schemaRefToGoType = %q, want %q", got, tc.want)
			}
		})
	}
}

// The collapse must never fire for a named component: a $ref returns the
// schema's own name before the switch is reached, so a component declared as a
// single-member allOf keeps emitting a type of its own.
func TestSchemaRefToGoTypeKeepsNamedComponentName(t *testing.T) {
	ref := &openapi3.SchemaRef{
		Ref: "#/components/schemas/PolicyDetail",
		Value: &openapi3.Schema{AllOf: openapi3.SchemaRefs{
			{Ref: "#/components/schemas/PolicySummary", Value: openapi3.NewObjectSchema()},
		}},
	}
	if got := schemaRefToGoType(ref); got != "PolicyDetail" {
		t.Fatalf("schemaRefToGoType = %q, want PolicyDetail", got)
	}
}

// The wrapper is inline, so `nullable` stays on the wrapper and the shared
// component the reference points at is never mutated. Collapsing by setting
// Nullable on the target — the way collapseNullableOneOf does for its $ref
// branch — would mark that component nullable for every field referencing it.
func TestSingleRefAllOfCollapseDoesNotMutateTarget(t *testing.T) {
	shared := openapi3.NewObjectSchema()
	wrapper := &openapi3.SchemaRef{Value: &openapi3.Schema{
		Nullable: true,
		AllOf:    openapi3.SchemaRefs{{Ref: "#/components/schemas/DeploymentRun", Value: shared}},
	}}
	if got := schemaRefToGoType(wrapper); got != "DeploymentRun" {
		t.Fatalf("schemaRefToGoType = %q, want DeploymentRun", got)
	}
	if shared.Nullable {
		t.Fatal("collapse marked the shared referenced component nullable")
	}
}
