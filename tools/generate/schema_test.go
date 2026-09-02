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

// A discriminator mapping may point several wire values at one variant schema.
// uem-connect does: nine UEM vendors share the generic ConnectorCreateRequest
// and only JAMF_PRO gets its own type. Deduping by Go type — which is right for
// the struct field, since nine identical pointers would be nonsense — used to
// drop the other eight values entirely, and with them their cases in the
// generated marshal switch. The failure mode is silence: a caller setting
// Vendor to one of the dropped values marshals to `{"vendor":"INTUNE"}` with
// every other field gone, and no error at any layer.
func TestSchemaToDiscriminatorTypeGroupsSharedVariants(t *testing.T) {
	ref := func(name string) *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Ref: "#/components/schemas/" + name}
	}
	schema := &openapi3.Schema{
		OneOf: openapi3.SchemaRefs{ref("JamfProConnectorCreateRequest"), ref("ConnectorCreateRequest")},
		Discriminator: &openapi3.Discriminator{
			PropertyName: "vendor",
			Mapping: map[string]openapi3.MappingRef{
				"JAMF_PRO":    {Ref: "#/components/schemas/JamfProConnectorCreateRequest"},
				"INTUNE":      {Ref: "#/components/schemas/ConnectorCreateRequest"},
				"AIRWATCH":    {Ref: "#/components/schemas/ConnectorCreateRequest"},
				"JAMF_SCHOOL": {Ref: "#/components/schemas/ConnectorCreateRequest"},
			},
		},
	}

	// schemaToDiscriminatorType registers the discriminator's enum into the
	// per-spec property-enum registry, which a real run initialises.
	currentPropertyEnums = map[string]GoType{}
	currentSpecTypeNames = map[string]bool{}
	t.Cleanup(func() { currentPropertyEnums, currentSpecTypeNames = nil, nil })

	gt := schemaToDiscriminatorType("ConnectorCreateRequestBody", schema)
	if gt.Discriminator == nil {
		t.Fatal("no discriminator emitted")
	}
	if got := len(gt.Discriminator.Variants); got != 2 {
		t.Fatalf("variants = %d, want 2 (one field per Go type)", got)
	}

	byType := map[string]GoDiscriminatorVariant{}
	for _, v := range gt.Discriminator.Variants {
		byType[v.TypeName] = v
	}

	// Every mapped value must survive as a case, or it marshals to nothing.
	generic, ok := byType["ConnectorCreateRequest"]
	if !ok {
		t.Fatal("no variant for ConnectorCreateRequest")
	}
	wantValues := map[string]bool{"AIRWATCH": true, "INTUNE": true, "JAMF_SCHOOL": true}
	if len(generic.Values) != len(wantValues) {
		t.Fatalf("generic variant values = %v, want the three sharing it", generic.Values)
	}
	for _, v := range generic.Values {
		if !wantValues[v] {
			t.Errorf("unexpected value %q on the generic variant", v)
		}
	}
	// Naming the shared field after one arbitrary member implies the variant is
	// only reachable through it, so it takes the schema name instead.
	if generic.FieldName != "ConnectorCreateRequest" {
		t.Errorf("shared variant field = %q, want the schema name ConnectorCreateRequest", generic.FieldName)
	}

	// A single-value variant keeps the value-derived name, so existing unions
	// (pro's MobileDeviceResponse, blueprints' BookmarkItem) do not churn.
	jamfPro, ok := byType["JamfProConnectorCreateRequest"]
	if !ok {
		t.Fatal("no variant for JamfProConnectorCreateRequest")
	}
	if len(jamfPro.Values) != 1 || jamfPro.Values[0] != "JAMF_PRO" {
		t.Errorf("JAMF_PRO variant values = %v, want exactly [JAMF_PRO]", jamfPro.Values)
	}
	if jamfPro.FieldName != exportedGoName("JAMF_PRO") {
		t.Errorf("single-value variant field = %q, want %q", jamfPro.FieldName, exportedGoName("JAMF_PRO"))
	}

	// The mapping is the only complete list of accepted values once a spec moves
	// a value out of a variant's own enum, so the union has to carry constants
	// for all of them — JAMF_PRO included, which ConnectorCreateRequest.vendor
	// no longer declares.
	if gt.Discriminator.EnumTypeName != "ConnectorCreateRequestBodyVendor" {
		t.Fatalf("EnumTypeName = %q, want ConnectorCreateRequestBodyVendor", gt.Discriminator.EnumTypeName)
	}
	enum, ok := currentPropertyEnums["ConnectorCreateRequestBodyVendor"]
	if !ok {
		t.Fatal("discriminator enum was not registered")
	}
	got := map[string]bool{}
	for _, c := range enum.EnumValues {
		got[c.Value] = true
	}
	for _, want := range []string{"JAMF_PRO", "INTUNE", "AIRWATCH", "JAMF_SCHOOL"} {
		if !got[want] {
			t.Errorf("discriminator enum missing %q", want)
		}
	}
}

// pro's GET /inventory-preload declares two content types that disagree:
// text/csv carries the pagination envelope directly, while application/json
// carries an *array of* that envelope. The wire sends the bare envelope
// (probed 2026-08-31), so the envelope form is the correct reading.
//
// Two failures are pinned here. Iterating the Content map directly made the
// answer depend on Go's randomised map order, so the same config generated
// either element type from run to run — and the array branch winning at all
// produced a list method returning []InventoryPreloadRecordSearchResults, one
// level of nesting off, which compiles and decodes into empty structs.
func TestDetectPaginatedItemTypePrefersEnvelopeOverArrayOfEnvelope(t *testing.T) {
	envelope := openapi3.NewObjectSchema()
	envelope.Properties = openapi3.Schemas{
		"totalCount": {Value: openapi3.NewIntegerSchema()},
		"results": {Value: &openapi3.Schema{
			Type:  types("array"),
			Items: &openapi3.SchemaRef{Ref: "#/components/schemas/InventoryPreloadRecord", Value: openapi3.NewObjectSchema()},
		}},
	}
	envelopeRef := &openapi3.SchemaRef{Ref: "#/components/schemas/InventoryPreloadRecordSearchResults", Value: envelope}

	resp := openapi3.NewResponse().WithDescription("OK")
	resp.Content = openapi3.Content{
		// Alphabetically first, and the mis-declared one — so a fix that only
		// sorted the keys without preferring the envelope would still fail.
		"application/json": &openapi3.MediaType{Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:  types("array"),
			Items: envelopeRef,
		}}},
		"text/csv": &openapi3.MediaType{Schema: envelopeRef},
	}
	op := &openapi3.Operation{Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: resp}))}

	// Run repeatedly: a single pass can pick the right answer by luck when the
	// selection depends on map order.
	for i := range 50 {
		if got := detectPaginatedItemType(op, ""); got != "InventoryPreloadRecord" {
			t.Fatalf("iteration %d: detectPaginatedItemType = %q, want InventoryPreloadRecord", i, got)
		}
	}
}

// The raw-array fallback must survive the envelope-first reordering: pagination
// style "rawArray" exists for endpoints that really do return a bare array.
func TestDetectPaginatedItemTypeStillHandlesRawArray(t *testing.T) {
	resp := openapi3.NewResponse().WithDescription("OK")
	resp.Content = openapi3.Content{
		"application/json": &openapi3.MediaType{Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:  types("array"),
			Items: &openapi3.SchemaRef{Ref: "#/components/schemas/SiteObject", Value: openapi3.NewObjectSchema()},
		}}},
	}
	op := &openapi3.Operation{Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: resp}))}

	if got := detectPaginatedItemType(op, ""); got != "SiteObject" {
		t.Fatalf("detectPaginatedItemType = %q, want SiteObject", got)
	}
}

// A whitelist that stops reaching a read schema must also stop that schema
// naming the nested types it shares with its *_post sibling. applyPostSymmetry
// makes the two share the very SchemaRef being lifted, and hoistInlineObjects
// names a lift after whichever parent it reaches first in sorted order — so
// while an unreachable `computer` sits in the document it keeps winning
// `computer_general` over `computer_post`'s claim to it.
//
// That mattered because publishSpecs prunes before writing api/*.json while Go
// generation read testing/*.json unpruned: dropping GET /computers/id/{id}
// from the whitelist made the two inputs hoist different names, and CI —
// which generates from api/ — failed `git diff --exit-code -- jamfplatform/`
// on a rename nothing in config.json mentions. pruneUnreferencedSchemas
// between the two passes is what makes the inputs identical by construction,
// and this test fails if it is moved before applyPostSymmetry (the post type
// loses the inherited section) or after hoistInlineObjects (the stale name
// comes back).
func TestPruneUnreferencedSchemasRunsBeforeHoistNaming(t *testing.T) {
	// general.remote_management is declared inline on the read schema only;
	// computer_post inherits it through post-symmetry, exactly as Classic's
	// spec does.
	newDoc := func() *openapi3.T {
		remote := openapi3.NewObjectSchema()
		remote.Properties = openapi3.Schemas{
			"managed": {Value: openapi3.NewBoolSchema()},
		}
		general := openapi3.NewObjectSchema()
		general.Properties = openapi3.Schemas{
			"name":              {Value: openapi3.NewStringSchema()},
			"remote_management": {Value: remote},
		}
		read := openapi3.NewObjectSchema()
		read.Properties = openapi3.Schemas{"general": {Value: general}}

		return &openapi3.T{
			Paths: openapi3.NewPaths(),
			Components: &openapi3.Components{
				Schemas: openapi3.Schemas{
					"computer":      {Value: read},
					"computer_post": {Value: openapi3.NewObjectSchema()},
				},
			},
		}
	}

	// Only the POST is whitelisted, and it names its body through config —
	// which is how every Classic write operation is declared.
	spec := SpecDef{
		Format: "xml",
		Operations: []OperationDef{
			{Op: "POST /computers/id/{id}", Name: "CreateComputerByID", RequestType: "computer_post"},
		},
	}

	doc := newDoc()
	applyPostSymmetry(doc, nil)
	pruneUnreferencedSchemas(doc, spec)
	hoistInlineObjects(doc, spec.Format)

	if _, ok := doc.Components.Schemas["computer"]; ok {
		t.Error("computer is unreachable from the whitelist but survived the prune")
	}
	if _, ok := doc.Components.Schemas["computer_postGeneral"]; !ok {
		t.Errorf("want computer_postGeneral hoisted; schemas = %v", sortedKeys(doc.Components.Schemas))
	}
	if _, ok := doc.Components.Schemas["computerGeneral"]; ok {
		t.Error("computerGeneral was hoisted from a schema the whitelist no longer reaches")
	}
	if _, ok := doc.Components.Schemas["computer_postGeneralRemoteManagement"]; !ok {
		t.Errorf("want computer_postGeneralRemoteManagement hoisted; schemas = %v", sortedKeys(doc.Components.Schemas))
	}

	// The section itself must still be there: pruning before post-symmetry
	// would have stripped it off the post type along with its read sibling.
	post := doc.Components.Schemas["computer_post"].Value
	if _, ok := post.Properties["general"]; !ok {
		t.Fatal("computer_post lost the inherited general section — prune ran before post-symmetry")
	}
}
