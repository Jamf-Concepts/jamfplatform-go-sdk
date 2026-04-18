// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// hoistInlineObjects promotes every inline object-with-properties found in
// component schemas to its own named top-level schema and replaces the
// property with a $ref. Specs that model deeply-nested XML resources
// (Classic's Computer has general/hardware/software sections defined
// inline) depend on this pass — without it those nested objects collapse
// to map[string]any, which encoding/xml can't populate from structured
// XML content. Runs in-place on the doc; safe for specs that already use
// named schemas (no inline objects found → no-op).
func hoistInlineObjects(doc *openapi3.T) {
	if doc == nil || doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	changed := true
	for changed {
		changed = false
		for _, name := range sortedKeys(doc.Components.Schemas) {
			schema := doc.Components.Schemas[name].Value
			if schema == nil {
				continue
			}
			if hoistInlineObjectsInSchema(name, schema, doc) {
				changed = true
			}
		}
	}
}

// hoistInlineObjectsInSchema walks one schema's properties and items, lifting
// inline typed objects into named top-level schemas. Returns true when any
// lift happened so the outer loop can revisit schemas added mid-walk.
func hoistInlineObjectsInSchema(parentName string, schema *openapi3.Schema, doc *openapi3.T) bool {
	if schema == nil {
		return false
	}
	hoisted := false
	// Top-level array schemas whose items are an inline object need
	// their items promoted so the generated alias has a named element
	// type. The Classic spec is riddled with `type: array, items: {
	// properties: {size, building} }` list shapes; without hoisting
	// the items collapse to map[string]any.
	if schema.Type.Is("array") && schema.Items != nil && schema.Items.Ref == "" &&
		schema.Items.Value != nil && len(schema.Items.Value.Properties) > 0 {
		nested := uniqueSchemaName(doc, parentName+"Item")
		if schema.Items.Value.XML == nil {
			schema.Items.Value.XML = &openapi3.XML{}
		}
		if schema.Items.Value.XML.Name == "" {
			schema.Items.Value.XML.Name = singularize(parentName)
		}
		doc.Components.Schemas[nested] = &openapi3.SchemaRef{Value: schema.Items.Value}
		schema.Items = &openapi3.SchemaRef{Ref: "#/components/schemas/" + nested, Value: schema.Items.Value}
		hoisted = true
	}
	lift := func(propName string, ref *openapi3.SchemaRef) *openapi3.SchemaRef {
		if ref == nil || ref.Ref != "" || ref.Value == nil {
			return ref
		}
		v := ref.Value
		inlineObject := v.Type.Is("object") && len(v.Properties) > 0
		inlineArrayOfObject := v.Type.Is("array") && v.Items != nil && v.Items.Ref == "" &&
			v.Items.Value != nil && v.Items.Value.Type.Is("object") && len(v.Items.Value.Properties) > 0
		if !inlineObject && !inlineArrayOfObject {
			return ref
		}
		if inlineObject {
			nested := parentName + exportedGoName(propName)
			nested = uniqueSchemaName(doc, nested)
			// Preserve the original property name as the hoisted schema's
			// XML wire name so its XMLName tag matches the containing
			// field's xml tag (Go's encoding/xml enforces agreement).
			if v.XML == nil {
				v.XML = &openapi3.XML{}
			}
			if v.XML.Name == "" {
				v.XML.Name = propName
			}
			doc.Components.Schemas[nested] = &openapi3.SchemaRef{Value: v}
			hoisted = true
			return &openapi3.SchemaRef{Ref: "#/components/schemas/" + nested, Value: v}
		}
		// inline array of object — hoist the element schema.
		nested := parentName + exportedGoName(propName) + "Item"
		nested = uniqueSchemaName(doc, nested)
		if v.Items.Value.XML == nil {
			v.Items.Value.XML = &openapi3.XML{}
		}
		if v.Items.Value.XML.Name == "" {
			// Array element wire name defaults to the Jamf convention of
			// singular of the plural property name. Best-effort — curator
			// can fix via xml metadata if needed.
			v.Items.Value.XML.Name = singularize(propName)
		}
		doc.Components.Schemas[nested] = &openapi3.SchemaRef{Value: v.Items.Value}
		v.Items = &openapi3.SchemaRef{Ref: "#/components/schemas/" + nested, Value: v.Items.Value}
		hoisted = true
		return ref
	}
	for _, propName := range sortedKeys(schema.Properties) {
		schema.Properties[propName] = lift(propName, schema.Properties[propName])
	}
	return hoisted
}

// singularize returns a best-effort singular form of a plural noun — used
// as the default XML element name for array items when the spec doesn't
// provide one. Handles the common English plural suffixes Jamf uses
// (-ies, -s). Curators can override via explicit xml metadata.
func singularize(plural string) string {
	switch {
	case strings.HasSuffix(plural, "ies"):
		return plural[:len(plural)-3] + "y"
	case strings.HasSuffix(plural, "ses"):
		return plural[:len(plural)-2]
	case strings.HasSuffix(plural, "s") && !strings.HasSuffix(plural, "ss"):
		return plural[:len(plural)-1]
	}
	return plural
}

// uniqueSchemaName disambiguates a proposed schema name if the name is
// already taken by an unrelated schema.
func uniqueSchemaName(doc *openapi3.T, base string) string {
	if _, exists := doc.Components.Schemas[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if _, exists := doc.Components.Schemas[candidate]; !exists {
			return candidate
		}
	}
}

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

func extractTypes(doc *openapi3.T, allow map[string]*schemaUsage, format string) []GoType {
	names := sortedKeys(doc.Components.Schemas)
	var types []GoType

	for _, specName := range names {
		usage := allow[specName]
		if usage == nil {
			continue
		}
		schema := doc.Components.Schemas[specName].Value
		if schema == nil {
			continue
		}
		name := goTypeName(specName)
		xmlName := xmlWireName(specName, schema)
		// allOf composition without an explicit type: merge properties from
		// each composed schema into a single flat struct.
		if len(schema.AllOf) > 0 && (schema.Type == nil || !schema.Type.Is("object")) {
			t := schemaToGoType(name, schema, false, format)
			t.XMLName = xmlName
			types = append(types, t)
			continue
		}
		if schema.Type == nil {
			// Swagger 2.0 often omits type: object on definitions that are
			// clearly objects (Classic spec does this). If there are
			// properties, treat it as an object anyway.
			if len(schema.Properties) > 0 {
				t := schemaToGoType(name, schema, usage.isRequest, format)
				t.XMLName = xmlName
				types = append(types, t)
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

		// Top-level array → type emission strategy depends on item shape.
		// For XML specs (Classic) the wire shape is `<root><size>N</size>
		// <resource>...</resource>*</root>`, modelled in Swagger 2.0 as
		// `type: array, items: {properties: {size, resource}}`. A raw
		// Go slice alias can't decode this — Go's xml.Unmarshal requires
		// a struct root to bind the wrapping element. Detect the {size,
		// single-ref} pattern and emit a wrapper struct that flattens
		// the item into a sibling `[]Resource` slice. Non-matching
		// arrays still get the alias treatment.
		if schema.Type.Is("array") {
			if format == "xml" {
				if wrapper, ok := classicListWrapper(name, specName, schema, doc); ok {
					types = append(types, wrapper)
					continue
				}
			}
			itemType := "any"
			if schema.Items != nil {
				itemType = schemaRefToGoType(schema.Items)
			}
			types = append(types, GoType{
				Name:        name,
				AliasTarget: "[]" + itemType,
				Comment:     fmt.Sprintf("%s is a list of %s.", name, itemType),
			})
			continue
		}

		// Top-level scalar (integer/number/string/boolean without enum) →
		// type alias. Classic uses these as shared field schemas (`size`,
		// `id_name`, etc.) referenced by $ref from other schemas. Skipping
		// them leaves the referencing struct with an undefined Go type.
		if schema.Type.Is("string") || schema.Type.Is("integer") || schema.Type.Is("number") || schema.Type.Is("boolean") {
			target := schemaRefToGoType(&openapi3.SchemaRef{Value: schema})
			types = append(types, GoType{
				Name:        name,
				AliasTarget: target,
				Comment:     fmt.Sprintf("%s is an alias for %s.", name, target),
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

		t := schemaToGoType(name, schema, usage.isRequest, format)
		t.XMLName = xmlName
		types = append(types, t)
	}
	if format == "xml" {
		stripConflictingXMLNames(types)
		addTopLevelIDsForClassic(types)
	}
	return types
}

// classicListWrapper detects the Jamf Classic list schema shape and
// returns a GoType representing a proper wrapper struct. The pattern is
// `type: array, items: {properties: {size, <resource>}}` where <resource>
// is a single named property (typically a $ref to the resource schema or
// a simple object). Returns (GoType, true) when the pattern matches;
// otherwise (GoType{}, false) so the caller falls back to the alias
// path.
//
// Emits roughly:
//
//	type Buildings struct {
//	    XMLName  xml.Name
//	    Size     *int        `xml:"size,omitempty"`
//	    Items    []Building  `xml:"building"`
//	}
//
// Tenants return the top-level <buildings> element with any number of
// <size> and <building> children at the same level, not nested under
// an intermediate wrapper. Flattening the {size, building} item pair
// into sibling fields on the wrapper matches the actual wire shape,
// where a naive []Item slice decodes to sparse pairs.
func classicListWrapper(goName, specName string, schema *openapi3.Schema, doc *openapi3.T) (GoType, bool) {
	if schema.Items == nil {
		return GoType{}, false
	}
	itemsVal := schema.Items.Value
	if itemsVal == nil || len(itemsVal.Properties) == 0 {
		return GoType{}, false
	}
	var sizeProp *openapi3.SchemaRef
	var resourceName string
	var resourceProp *openapi3.SchemaRef
	for name, prop := range itemsVal.Properties {
		if name == "size" {
			sizeProp = prop
			continue
		}
		if resourceName != "" {
			// More than one non-size property — not the Classic pattern.
			return GoType{}, false
		}
		resourceName = name
		resourceProp = prop
	}
	if sizeProp == nil || resourceName == "" || resourceProp == nil {
		return GoType{}, false
	}
	sizeGo := "*int"
	if sizeProp.Ref != "" {
		sizeGo = "*" + refName(sizeProp)
	}
	resourceGo := refName(resourceProp)
	wrapper := GoType{
		Name:          goName,
		XMLName:       specName,
		IsListWrapper: true,
		Comment:       fmt.Sprintf("%s wraps a Jamf Classic list response with a top-level size count and a flat slice of %s.", goName, resourceGo),
		Fields: []GoField{
			{Name: "Size", Type: sizeGo, JSONTag: "size,omitempty"},
			{Name: exportedGoName(plural(resourceName)), Type: "[]" + resourceGo, JSONTag: resourceName},
		},
	}
	_ = doc
	return wrapper, true
}

// plural returns a best-effort plural form for the items-field identifier
// in a Classic list wrapper. The resource element name on the wire is
// singular (e.g. `<building>`) while the Go field holding the slice reads
// more naturally as plural (e.g. `Buildings`).
func plural(singular string) string {
	switch {
	case strings.HasSuffix(singular, "y") && len(singular) > 1 && !strings.ContainsAny(singular[len(singular)-2:len(singular)-1], "aeiou"):
		return singular[:len(singular)-1] + "ies"
	case strings.HasSuffix(singular, "s"), strings.HasSuffix(singular, "sh"), strings.HasSuffix(singular, "ch"), strings.HasSuffix(singular, "x"):
		return singular + "es"
	}
	return singular + "s"
}

// addTopLevelIDsForClassic injects a top-level `ID *int` field on Classic
// types whose id lives inside a nested General sub-object. Classic servers
// return the new record's id at the top level of the create-response body
// (<policy><id>N</id></policy>), but Jamf's spec nests id under <general>
// in the shared read schema — so the generated struct has no top-level
// ID to capture the write-response id. Without this, callers must look
// up the new record by name after every Create. The injected field is a
// no-op on reads (server never populates it there) and populates cleanly
// on writes.
func addTopLevelIDsForClassic(types []GoType) {
	for i := range types {
		t := &types[i]
		if len(t.Fields) == 0 || t.AliasTarget != "" || t.IsRawJSON || t.IsListWrapper {
			continue
		}
		var hasSubObject, hasTopID bool
		for _, f := range t.Fields {
			if f.Name == "ID" {
				hasTopID = true
			}
			// Any pointer-to-struct field — i.e. a nested sub-object like
			// General, Connection, Scope — is a signal the server probably
			// returns id at the top level on create while the spec nests it
			// inside one of these children. Scalar ptr fields (*string, *int,
			// *bool) and slice/map types don't count.
			if strings.HasPrefix(f.Type, "*") && !strings.HasPrefix(f.Type, "*[]") &&
				!isScalar(strings.TrimPrefix(f.Type, "*")) &&
				!strings.HasPrefix(f.Type, "*map[") {
				hasSubObject = true
			}
		}
		if !hasSubObject || hasTopID {
			continue
		}
		t.Fields = append([]GoField{{
			Name:    "ID",
			Type:    "*int",
			JSONTag: "id,omitempty",
		}}, t.Fields...)
	}
}

// stripConflictingXMLNames clears the XMLName on any struct that is
// referenced as a field in another struct under a different tag. Go's
// encoding/xml refuses to unmarshal when a field tag and the target
// struct's XMLName disagree — Classic hits this via shared schemas like
// `id_name` that are embedded under parent-defined tags (`<computer>`,
// `<user>`, etc.). When a type is used only as a root (no referrers) or
// always as a matching tag, its XMLName stays. When it appears under at
// least one mismatching tag, we drop the XMLName — decoding relies on
// the parent field's tag to bind the element, and marshal of the root
// still works for fully-qualified request roots whose tag matches.
func stripConflictingXMLNames(types []GoType) {
	tagsByType := make(map[string]map[string]bool)
	for _, t := range types {
		for _, f := range t.Fields {
			ref := normalizeTypeRef(f.Type)
			if ref == "" {
				continue
			}
			tag := f.JSONTag
			if i := strings.Index(tag, ","); i >= 0 {
				tag = tag[:i]
			}
			if tagsByType[ref] == nil {
				tagsByType[ref] = map[string]bool{}
			}
			tagsByType[ref][tag] = true
		}
	}
	for i := range types {
		t := &types[i]
		if t.XMLName == "" {
			continue
		}
		tags, ok := tagsByType[t.Name]
		if !ok {
			continue
		}
		conflict := false
		for tag := range tags {
			if tag != "" && tag != t.XMLName {
				conflict = true
				break
			}
		}
		if conflict {
			t.XMLName = ""
		}
	}
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

func schemaToGoType(name string, schema *openapi3.Schema, isRequest bool, format string) GoType {
	gt := GoType{
		Name:    name,
		Comment: fmt.Sprintf("%s represents a %s.", name, camelToWords(name)),
	}
	if schema.Description != "" {
		gt.Comment = name + " " + cleanComment(schema.Description)
	}

	props, requiredList := flattenAllOf(schema)
	required := toSet(requiredList)
	for _, pnameRaw := range sortedKeys(props) {
		propRef := props[pnameRaw]
		// Classic's spec encodes deprecation inline in property names
		// (e.g. `management_username deprecated="10.48"`). Everything after
		// the first whitespace is metadata the generator doesn't model.
		pname := pnameRaw
		if i := strings.IndexAny(pname, " \t"); i >= 0 {
			pname = pname[:i]
		}
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
		//
		// For XML specs (Jamf Classic) every field becomes a pointer with
		// omitempty regardless of required/nullable flags. Classic consumers
		// (especially the TF provider) rely on three-state semantics: nil to
		// omit, &"" to clear, &value to set. The spec under-declares
		// nullability so the usual heuristic produces non-pointer scalars
		// that conflate "omit" and "clear" on the wire.
		isStructRef := propRef.Ref != "" && prop != nil && prop.Type != nil &&
			prop.Type.Is("object") && len(prop.Properties) > 0
		needsPtr := isNullable || (isStructRef && !isRequest) || (!isRequired && !isScalar(goType)) ||
			(isRequest && !isRequired) || format == "xml"

		if isRequest && !isRequired && !strings.HasPrefix(goType, "*") && (strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[")) {
			// For request types, unrequired slices/maps get pointer-wrapped so
			// callers can distinguish "omit field" (nil) from "send empty" (&[]T{}).
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if needsPtr && !strings.HasPrefix(goType, "*") && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[") && goType != "any" {
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if isNullable && !strings.HasPrefix(goType, "*") && goType != "any" {
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
		return goTypeName(parts[len(parts)-1])
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
		// OpenAPI format: date-time is ISO 8601 / RFC 3339 per the Jamf
		// style guide. Emit as time.Time so callers get parsed timestamps
		// rather than raw strings; encoding/json handles the RFC 3339
		// round-trip natively for time.Time. Classic's XML codec also
		// honors time.Time via xml.MarshalerAttr/Unmarshaler defaults.
		if schema.Format == "date-time" {
			return "time.Time"
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
		return goTypeName(parts[len(parts)-1])
	}
	return schemaRefToGoType(ref)
}

// goTypeName converts a spec schema name to a valid Go identifier: PascalCase,
// underscores/hyphens removed, leading lowercase letter capitalised. Platform
// specs already use PascalCase so these are no-ops; Classic uses snake_case.
func goTypeName(specName string) string {
	if specName == "" {
		return specName
	}
	// Already a canonical Go reserved name like []byte, any, map[...] etc.
	if strings.HasPrefix(specName, "[]") || strings.HasPrefix(specName, "map[") || specName == "any" {
		return specName
	}
	return exportedGoName(specName)
}

// xmlWireName returns the root XML element name a schema serializes to.
// Spec-level xml.name overrides take priority (e.g. computer_post -> <computer>);
// otherwise the schema's original name is used verbatim (which for Classic
// is already the wire shape since the spec is snake_case).
//
// When the spec is missing an xml.name override on a `_post` write-schema
// (Jamf Classic convention: computer_post / policy_post / user_post / etc.
// all ship with `xml.name: <resource>` except a few where it's forgotten),
// we default to the suffix-stripped name. Without this, marshal emits a
// `<ldap_server_post>` root the server rejects. Spec-level overrides
// always win when present.
func xmlWireName(specName string, schema *openapi3.Schema) string {
	if schema != nil && schema.XML != nil && schema.XML.Name != "" {
		return schema.XML.Name
	}
	if strings.HasSuffix(specName, "_post") {
		return strings.TrimSuffix(specName, "_post")
	}
	return specName
}
