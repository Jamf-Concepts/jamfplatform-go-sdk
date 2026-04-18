// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// ---------------------------------------------------------------------------
// Shared schema walker
// ---------------------------------------------------------------------------

// newSchemaWalker returns a function that transitively walks $ref'd schemas.
// onRef is called for each unique $ref name encountered; it should return true
// if the walker should recurse into the referenced schema (i.e. it hasn't been
// visited yet in this walk's context).
func newSchemaWalker(doc *openapi3.T, onRef func(name string) bool) func(ref *openapi3.SchemaRef) {
	var walk func(ref *openapi3.SchemaRef)
	walk = func(ref *openapi3.SchemaRef) {
		if ref == nil {
			return
		}
		if ref.Ref != "" {
			parts := strings.Split(ref.Ref, "/")
			name := parts[len(parts)-1]
			if !onRef(name) {
				return
			}
			if schema, ok := doc.Components.Schemas[name]; ok {
				walk(schema)
			}
		}
		if ref.Value == nil {
			return
		}
		for _, prop := range ref.Value.Properties {
			walk(prop)
		}
		if ref.Value.Items != nil {
			walk(ref.Value.Items)
		}
		if ref.Value.AdditionalProperties.Schema != nil {
			walk(ref.Value.AdditionalProperties.Schema)
		}
		for _, s := range ref.Value.AllOf {
			walk(s)
		}
		for _, s := range ref.Value.OneOf {
			walk(s)
		}
		for _, s := range ref.Value.AnyOf {
			walk(s)
		}
	}
	return walk
}

// collectRefs walks the remaining spec paths and collects all referenced schema names,
// following nested $refs transitively. Used for pruning published specs.
func collectRefs(doc *openapi3.T, used map[string]bool) {
	walk := newSchemaWalker(doc, func(name string) bool {
		if used[name] {
			return false
		}
		used[name] = true
		return true
	})

	for _, path := range doc.Paths.InMatchingOrder() {
		item := doc.Paths.Find(path)
		if item == nil {
			continue
		}
		for _, p := range item.Parameters {
			if p.Value != nil && p.Value.Schema != nil {
				walk(p.Value.Schema)
			}
		}
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			op := item.GetOperation(method)
			if op == nil {
				continue
			}
			for _, p := range op.Parameters {
				if p.Value != nil && p.Value.Schema != nil {
					walk(p.Value.Schema)
				}
			}
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for _, content := range op.RequestBody.Value.Content {
					if content.Schema != nil {
						walk(content.Schema)
					}
				}
			}
			if op.Responses != nil {
				for _, respRef := range op.Responses.Map() {
					if respRef.Value == nil {
						continue
					}
					for _, content := range respRef.Value.Content {
						if content.Schema != nil {
							walk(content.Schema)
						}
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Schema reference collection — determines which schemas to generate
// ---------------------------------------------------------------------------

// schemaUsage tracks whether a schema is used as a request body, response body, or both.
// Request schemas get pointer fields for unrequired scalars (to distinguish omit vs zero value).
type schemaUsage struct {
	isRequest  bool
	isResponse bool
}

// collectReferencedSchemas walks all whitelisted operations in a spec and
// transitively collects every schema name referenced by request bodies,
// responses, and their nested properties, tracking request vs response usage.
func collectReferencedSchemas(doc *openapi3.T, spec SpecDef) map[string]*schemaUsage {
	used := make(map[string]*schemaUsage)

	makeWalker := func(isRequest bool) func(ref *openapi3.SchemaRef) {
		visited := make(map[string]bool)
		return newSchemaWalker(doc, func(name string) bool {
			if visited[name] {
				return false
			}
			visited[name] = true
			if used[name] == nil {
				used[name] = &schemaUsage{}
			}
			if isRequest {
				used[name].isRequest = true
			} else {
				used[name].isResponse = true
			}
			return true
		})
	}

	for _, opDef := range spec.Operations {
		method, path := opDef.parseOp()
		pathItem := doc.Paths.Find(path)
		if pathItem == nil {
			continue
		}
		op := pathItem.GetOperation(method)
		if op == nil {
			continue
		}
		if op.RequestBody != nil && op.RequestBody.Value != nil {
			walkReq := makeWalker(true)
			for _, content := range op.RequestBody.Value.Content {
				if content.Schema != nil {
					walkReq(content.Schema)
				}
			}
		}
		if op.Responses != nil {
			walkResp := makeWalker(false)
			for _, respRef := range op.Responses.Map() {
				if respRef.Value == nil {
					continue
				}
				for _, content := range respRef.Value.Content {
					if content.Schema != nil {
						walkResp(content.Schema)
					}
				}
			}
		}
		// Config-level type overrides: when the spec is untyped (e.g. Classic)
		// the curator names request/response schemas explicitly. Record those
		// as referenced and descend into their properties.
		walkNamed := func(name string, isRequest bool) {
			if doc.Components == nil || doc.Components.Schemas == nil {
				return
			}
			ref, ok := doc.Components.Schemas[name]
			if !ok {
				return
			}
			// The walker only calls onRef when it encounters a $ref. A
			// top-level schema we're told to walk by name has no $ref, so
			// register it manually before descending.
			if used[name] == nil {
				used[name] = &schemaUsage{}
			}
			if isRequest {
				used[name].isRequest = true
			} else {
				used[name].isResponse = true
			}
			makeWalker(isRequest)(ref)
		}
		if opDef.RequestType != "" {
			walkNamed(opDef.RequestType, true)
		}
		if opDef.ResponseType != "" {
			walkNamed(opDef.ResponseType, false)
		}
	}
	return used
}

// ---------------------------------------------------------------------------
// Schema → Go types
// ---------------------------------------------------------------------------

func extractTypes(doc *openapi3.T, allow map[string]*schemaUsage) []GoType {
	names := sortedKeys(doc.Components.Schemas)
	var types []GoType

	for _, name := range names {
		usage := allow[name]
		if usage == nil {
			continue
		}
		schema := doc.Components.Schemas[name].Value
		if schema == nil {
			continue
		}
		// allOf composition without an explicit type: merge properties from
		// each composed schema into a single flat struct.
		if len(schema.AllOf) > 0 && (schema.Type == nil || !schema.Type.Is("object")) {
			types = append(types, schemaToGoType(name, schema, false))
			continue
		}
		if schema.Type == nil {
			// Swagger 2.0 often omits type: object on definitions that are
			// clearly objects (Classic spec does this). If there are
			// properties, treat it as an object anyway.
			if len(schema.Properties) > 0 {
				types = append(types, schemaToGoType(name, schema, usage.isRequest))
			}
			continue
		}

		// Enum string → type alias
		if schema.Type.Is("string") && len(schema.Enum) > 0 {
			types = append(types, GoType{
				Name:    name,
				Comment: fmt.Sprintf("%s represents a %s value.", name, camelToWords(name)),
			})
			continue
		}

		if !schema.Type.Is("object") {
			continue
		}

		// oneOf + discriminator → union type with per-variant pointer fields.
		if schema.Discriminator != nil && len(schema.OneOf) > 0 {
			types = append(types, schemaToDiscriminatorType(name, schema))
			continue
		}

		// Freeform object (no properties) → json.RawMessage
		if len(schema.Properties) == 0 && schema.AdditionalProperties.Schema == nil {
			comment := name + " represents a freeform JSON object."
			if schema.Description != "" {
				comment = name + " " + lowerFirst(cleanComment(schema.Description))
			}
			types = append(types, GoType{Name: name, Comment: comment, IsRawJSON: true})
			continue
		}

		types = append(types, schemaToGoType(name, schema, usage.isRequest))
	}
	return types
}

// schemaToDiscriminatorType builds a GoType for a oneOf+discriminator schema.
// Variants come from the discriminator Mapping if present, else from the
// OneOf refs directly. The on-the-wire discriminator value lives in the
// mapping keys (or falls back to the Go type name) — important for specs
// where the wire value differs from the variant schema name.
func schemaToDiscriminatorType(name string, schema *openapi3.Schema) GoType {
	gt := GoType{
		Name:    name,
		Comment: fmt.Sprintf("%s is a polymorphic response keyed by %s. Exactly one variant pointer is populated after unmarshaling.", name, schema.Discriminator.PropertyName),
	}
	if schema.Description != "" {
		gt.Comment = name + " " + cleanComment(schema.Description)
	}
	gt.Discriminator = &GoDiscriminator{
		PropertyName: schema.Discriminator.PropertyName,
		GoFieldName:  exportedGoName(schema.Discriminator.PropertyName),
	}
	seen := make(map[string]bool)
	addVariant := func(value, typeName string) {
		if value == "" || typeName == "" || seen[typeName] {
			return
		}
		seen[typeName] = true
		gt.Discriminator.Variants = append(gt.Discriminator.Variants, GoDiscriminatorVariant{
			Value:     value,
			TypeName:  typeName,
			FieldName: exportedGoName(value),
		})
	}
	for _, mapKey := range sortedMapKeys(schema.Discriminator.Mapping) {
		ref := schema.Discriminator.Mapping[mapKey]
		parts := strings.Split(ref.Ref, "/")
		addVariant(mapKey, parts[len(parts)-1])
	}
	if len(gt.Discriminator.Variants) == 0 {
		for _, one := range schema.OneOf {
			if one.Ref == "" {
				continue
			}
			parts := strings.Split(one.Ref, "/")
			tn := parts[len(parts)-1]
			addVariant(tn, tn)
		}
	}
	return gt
}

// sortedMapKeys returns deterministically-ordered keys for a string-keyed map.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flattenAllOf merges properties and required lists from an allOf composition
// into a single property map. Resolved schemas carried on SchemaRef.Value
// (kin-openapi pre-populates these) let us walk $refs without a separate
// lookup. Later properties override earlier ones on name collision, matching
// OpenAPI's "latest wins" semantic.
func flattenAllOf(schema *openapi3.Schema) (map[string]*openapi3.SchemaRef, []string) {
	props := make(map[string]*openapi3.SchemaRef)
	reqSet := make(map[string]bool)
	var walk func(s *openapi3.Schema)
	walk = func(s *openapi3.Schema) {
		if s == nil {
			return
		}
		for k, v := range s.Properties {
			props[k] = v
		}
		for _, r := range s.Required {
			reqSet[r] = true
		}
		for _, one := range s.AllOf {
			if one.Value != nil {
				walk(one.Value)
			}
		}
	}
	walk(schema)
	required := make([]string, 0, len(reqSet))
	for r := range reqSet {
		required = append(required, r)
	}
	sort.Strings(required)
	return props, required
}

func schemaToGoType(name string, schema *openapi3.Schema, isRequest bool) GoType {
	gt := GoType{
		Name:    name,
		Comment: fmt.Sprintf("%s represents a %s.", name, camelToWords(name)),
	}
	if schema.Description != "" {
		gt.Comment = name + " " + cleanComment(schema.Description)
	}

	props, requiredList := flattenAllOf(schema)
	required := toSet(requiredList)
	for _, pname := range sortedKeys(props) {
		propRef := props[pname]
		prop := propRef.Value

		goType := schemaRefToGoType(propRef)
		jsonTag := pname

		isNullable := prop != nil && prop.Nullable
		isRequired := required[pname]

		// Pointer for: nullable, unrequired non-scalars, or $ref to object with properties.
		// For request types, unrequired scalars also get pointers so callers can
		// distinguish "omit field" from "send zero value" (critical for PATCH).
		// $ref struct pointers only apply to response types; for request types the
		// (isRequest && !isRequired) term handles optional fields instead.
		isStructRef := propRef.Ref != "" && prop != nil && prop.Type != nil &&
			prop.Type.Is("object") && len(prop.Properties) > 0
		needsPtr := isNullable || (isStructRef && !isRequest) || (!isRequired && !isScalar(goType)) ||
			(isRequest && !isRequired)

		if isRequest && !isRequired && !strings.HasPrefix(goType, "*") && (strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[")) {
			// For request types, unrequired slices/maps get pointer-wrapped so
			// callers can distinguish "omit field" (nil) from "send empty" (&[]T{}).
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if needsPtr && !strings.HasPrefix(goType, "*") && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if isNullable && !strings.HasPrefix(goType, "*") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		}

		var fieldComment string
		if prop != nil && (prop.WriteOnly || prop.Format == "password") {
			fieldComment = "Write-only. Servers MUST NOT return this field in responses; the SDK preserves it only so the caller can supply a value on update."
		}

		gt.Fields = append(gt.Fields, GoField{
			Name:    exportedGoName(pname),
			Type:    goType,
			JSONTag: jsonTag,
			Comment: fieldComment,
		})
	}
	return gt
}

// schemaRefToGoType returns the Go type string for a schema reference.
// kin-openapi populates Value for all refs at load time, so we never
// need to manually resolve.
func schemaRefToGoType(ref *openapi3.SchemaRef) string {
	if ref.Ref != "" {
		parts := strings.Split(ref.Ref, "/")
		return parts[len(parts)-1]
	}
	schema := ref.Value
	if schema == nil {
		return "any"
	}
	switch {
	case schema.Type.Is("string"):
		// OpenAPI format: byte means base64-encoded bytes. Go's encoding/json
		// handles base64 natively for []byte so callers work with raw bytes.
		if schema.Format == "byte" {
			return "[]byte"
		}
		return "string"
	case schema.Type.Is("integer"):
		if schema.Format == "int64" {
			return "int64"
		}
		return "int"
	case schema.Type.Is("number"):
		if schema.Format == "float" {
			return "float32"
		}
		return "float64"
	case schema.Type.Is("boolean"):
		return "bool"
	case schema.Type.Is("array"):
		if schema.Items != nil {
			return "[]" + schemaRefToGoType(schema.Items)
		}
		return "[]any"
	case schema.Type.Is("object"):
		if schema.AdditionalProperties.Schema != nil {
			return "map[string]" + schemaRefToGoType(schema.AdditionalProperties.Schema)
		}
		return "map[string]any"
	default:
		return "any"
	}
}

// refName extracts the schema name from a $ref string, or falls back to
// computing the Go type from the inline schema.
func refName(ref *openapi3.SchemaRef) string {
	if ref.Ref != "" {
		parts := strings.Split(ref.Ref, "/")
		return parts[len(parts)-1]
	}
	return schemaRefToGoType(ref)
}
