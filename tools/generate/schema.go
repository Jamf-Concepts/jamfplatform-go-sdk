// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// applySchemaAdditions injects missing property declarations into named
// component schemas. Used for specs that omit fields the server actually
// accepts (e.g. Classic's `account` schema has no `password` property
// even though creating a user requires one on the wire). Runs after
// openapi2conv + before hoistInlineObjects so hoisting still sees the
// injected properties. openapi_type supports plain scalar names
// ("string", "integer", "boolean") and the extended form
// "string:password" which sets format=password + writeOnly=true so the
// field comment notes the security sensitivity.
func applySchemaAdditions(doc *openapi3.T, additions map[string]map[string]string) {
	if doc == nil || doc.Components == nil || doc.Components.Schemas == nil || len(additions) == 0 {
		return
	}
	for schemaName, props := range additions {
		ref, ok := doc.Components.Schemas[schemaName]
		if !ok || ref == nil || ref.Value == nil {
			continue
		}
		schema := ref.Value
		if schema.Properties == nil {
			schema.Properties = openapi3.Schemas{}
		}
		for propName, openapiType := range props {
			if _, exists := schema.Properties[propName]; exists {
				continue
			}
			parts := strings.SplitN(openapiType, ":", 2)
			baseType := parts[0]
			propSchema := &openapi3.Schema{Type: &openapi3.Types{baseType}}
			if len(parts) == 2 && parts[1] == "password" {
				propSchema.Format = "password"
				propSchema.WriteOnly = true
			}
			// "object" with no properties → freeform JSON (decoded as json.RawMessage).
			// The schema has type:object and no further constraints so the generator's
			// freeform-object detector fires and emits the field as json.RawMessage.
			if baseType == "object" {
				propSchema.AdditionalProperties = openapi3.AdditionalProperties{}
			}
			schema.Properties[propName] = &openapi3.SchemaRef{Value: propSchema}
		}
	}
}

// applySchemaPatches inserts or replaces sub-schemas at dotted property paths
// under named component schemas. Used when a spec omits a structured field the
// server actually returns or accepts (e.g. policy missing top-level `reboot`,
// `scope.jss_users`, or self-service notification scalars). Each patch value
// is a raw OpenAPI 3 Schema JSON fragment; intermediate path segments must
// resolve to object-with-properties so the walker can descend. Replaces an
// existing property when the path already terminates somewhere; otherwise
// adds it. Runs after applySchemaAdditions and before applyPostSymmetry so
// that *_post schemas inherit the patched read shape.
func applySchemaPatches(doc *openapi3.T, patches map[string]map[string]json.RawMessage) {
	if doc == nil || doc.Components == nil || doc.Components.Schemas == nil || len(patches) == 0 {
		return
	}
	for schemaName, paths := range patches {
		ref, ok := doc.Components.Schemas[schemaName]
		if !ok || ref == nil || ref.Value == nil {
			continue
		}
		for path := range paths {
			raw := paths[path]
			if len(raw) == 0 {
				continue
			}
			sub, err := parsePatchSchema(raw)
			if err != nil {
				panic(fmt.Sprintf("schemaPatches[%q][%q]: %v", schemaName, path, err))
			}
			resolvePatchRefs(sub, doc)
			parent, leaf, ok := walkPropertyPath(ref.Value, path)
			if !ok {
				panic(fmt.Sprintf("schemaPatches[%q]: cannot reach path %q in schema", schemaName, path))
			}
			if parent.Properties == nil {
				parent.Properties = openapi3.Schemas{}
			}
			parent.Properties[leaf] = sub
		}
	}
}

// applyPropertyRenames renames property keys at dotted paths inside named
// component schemas. Used to repair spec keys that don't match the wire so
// decode actually succeeds (`re-install_button_text` → `reinstall_button_text`,
// `allow_user_to_defer` → `allow_users_to_defer`). Path's last segment is the
// current key on the parent; value is the new key. Runs before applyPostSymmetry
// so the post sibling sees the corrected key.
func applyPropertyRenames(doc *openapi3.T, renames map[string]map[string]string) {
	if doc == nil || doc.Components == nil || doc.Components.Schemas == nil || len(renames) == 0 {
		return
	}
	for schemaName, paths := range renames {
		ref, ok := doc.Components.Schemas[schemaName]
		if !ok || ref == nil || ref.Value == nil {
			continue
		}
		for path, newKey := range paths {
			parent, leaf, ok := walkPropertyPath(ref.Value, path)
			if !ok {
				panic(fmt.Sprintf("propertyRenames[%q]: cannot reach path %q", schemaName, path))
			}
			cur, exists := parent.Properties[leaf]
			if !exists {
				panic(fmt.Sprintf("propertyRenames[%q]: property %q missing at path %q", schemaName, leaf, path))
			}
			delete(parent.Properties, leaf)
			parent.Properties[newKey] = cur
		}
	}
}

// applyPostSymmetry copies properties from each read schema X into its *_post
// sibling X_post so write types accept the full field set the server actually
// honours. Default behaviour for every spec — Jamf's classic specs routinely
// declare X_post as a minimal "general only" object even when the server
// accepts (and persists) every other top-level block. Specs that genuinely
// have write-only fields list them under PostSymmetryExcludes.
//
// Copy semantics: when the read schema declares a property the post sibling
// doesn't, the property's *openapi3.SchemaRef is shared between read and post
// (cheap; hoistInlineObjects mutates per-schema property map entries, not the
// underlying inline objects). When the post sibling already declares the same
// property, the read shape REPLACES it — the brief is explicit that the post
// must mirror the read, not preserve a slimmer alternative shape (e.g. the
// existing EbookPostScope all_* booleans only). The post schema's own xml
// metadata (typically `xml.name = <read-name>`) is preserved.
func applyPostSymmetry(doc *openapi3.T, excludes map[string][]string) {
	if doc == nil || doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	for postName, ref := range doc.Components.Schemas {
		if !strings.HasSuffix(postName, "_post") || ref == nil || ref.Value == nil {
			continue
		}
		readName := strings.TrimSuffix(postName, "_post")
		readRef, ok := doc.Components.Schemas[readName]
		if !ok || readRef == nil || readRef.Value == nil {
			continue
		}
		post := ref.Value
		read := readRef.Value
		if len(read.Properties) == 0 {
			continue
		}
		skip := make(map[string]bool, len(excludes[postName]))
		for _, p := range excludes[postName] {
			skip[p] = true
		}
		if post.Properties == nil {
			post.Properties = openapi3.Schemas{}
		}
		for propName, propRef := range read.Properties {
			if skip[propName] {
				continue
			}
			post.Properties[propName] = propRef
		}
	}
}

// parsePatchSchema parses a raw JSON OpenAPI Schema fragment into a SchemaRef.
// Uses SchemaRef's own UnmarshalJSON so nested `$ref` strings populate the
// Ref field on the right SchemaRef levels (a direct json.Unmarshal into
// openapi3.Schema would skip that path for property values and items).
// Refs in the fragment are left unresolved on purpose — the parent document
// already carries the target schemas (e.g. id_name), and `openapi3.SchemaRef`
// callers downstream key off `.Ref` strings rather than `.Value` pointers.
func parsePatchSchema(raw json.RawMessage) (*openapi3.SchemaRef, error) {
	var ref openapi3.SchemaRef
	if err := ref.UnmarshalJSON(raw); err != nil {
		return nil, err
	}
	return &ref, nil
}

// resolvePatchRefs walks a freshly-parsed patch SchemaRef tree and points
// each unresolved `$ref` at the matching component schema in the parent
// doc. Manual unmarshal of a SchemaRef populates `Ref` but leaves `Value`
// nil; downstream passes (flattenClassicSizeWrappers, hoistInlineObjects)
// short-circuit on `Value == nil`, treating refs as unknown shape. Loader
// machinery would normally resolve these but bypassing the loader was
// chosen to avoid forcing each patch fragment into a standalone document
// with every transitive $ref target restated.
func resolvePatchRefs(ref *openapi3.SchemaRef, doc *openapi3.T) {
	if ref == nil {
		return
	}
	if ref.Ref != "" && ref.Value == nil {
		if name, ok := strings.CutPrefix(ref.Ref, "#/components/schemas/"); ok {
			if target, exists := doc.Components.Schemas[name]; exists && target != nil {
				ref.Value = target.Value
			}
		}
	}
	if ref.Value == nil {
		return
	}
	for _, p := range ref.Value.Properties {
		resolvePatchRefs(p, doc)
	}
	if ref.Value.Items != nil {
		resolvePatchRefs(ref.Value.Items, doc)
	}
	for _, s := range ref.Value.AllOf {
		resolvePatchRefs(s, doc)
	}
}

// walkPropertyPath traverses dotted `a.b.c` into nested object schemas. Returns
// the deepest reachable parent schema and the final leaf segment. Auto-descends
// through array schemas via `.items.value` (Classic spec routinely models
// repeating XML children as `prop: { type: array, items: { type: object,
// properties: { child: { … } } } }`; expressing the descent explicitly in the
// path would force every caller to thread `items.` through every array hop).
// Non-leaf segments must resolve to an object-with-properties (after array
// auto-descent); the leaf segment may or may not exist on the returned parent.
func walkPropertyPath(root *openapi3.Schema, path string) (*openapi3.Schema, string, bool) {
	if root == nil || path == "" {
		return nil, "", false
	}
	parts := strings.Split(path, ".")
	cur := root
	for i := 0; i < len(parts)-1; i++ {
		seg := parts[i]
		if cur.Type != nil && cur.Type.Is("array") && cur.Items != nil && cur.Items.Value != nil {
			cur = cur.Items.Value
		}
		if cur.Properties == nil {
			return nil, "", false
		}
		next, ok := cur.Properties[seg]
		if !ok || next == nil || next.Value == nil {
			return nil, "", false
		}
		cur = next.Value
	}
	if cur.Type != nil && cur.Type.Is("array") && cur.Items != nil && cur.Items.Value != nil {
		cur = cur.Items.Value
	}
	return cur, parts[len(parts)-1], true
}

// flattenClassicSizeWrappers rewrites Jamf Classic's `<wrapper><size>N</size><item>...</item>...</wrapper>`
// XML containers so they round-trip through encoding/xml without data loss.
//
// The Classic spec models these as `wrapper: type:array, items: {size, X}` —
// implying N wrapper elements, each carrying a size sentinel and a single item.
// The actual wire emits ONE `<wrapper>` element with a single `<size>` sibling
// and N repeated `<X>` children. Mapping the spec literally produces
// `Wrapper *[]Item` in Go; encoding/xml then sees one `<wrapper>` element,
// allocates one slot, and the N `<X>` children collide into the slot's
// non-slice X field — only the last survives. Smart groups silently lose every
// criterion except the final one (confirmed across UserGroup, MobileDeviceGroup,
// ComputerGroup on real tenant XML).
//
// Transform: `wrapper: array of {size, X}` → `wrapper: object {size, X: array of <X-shape>}`.
// hoistInlineObjects then lifts the new wrapper into its own named schema, and
// the inner X array hoists / dedups against existing named schemas (criterion,
// computer, mobile_device, etc.) as usual. Output: callers get
// `*UserGroupCriteria { Size *int, Criterion []Criterion }` and the full slice
// survives a decode.
//
// Pattern guard — applied only when ALL of the following hold so we don't
// rewrite legitimate JSON-style arrays-of-{size,item} (none exist in Classic
// today, but the heuristic should be tight regardless):
//   - property is `type: array` with inline items (no $ref)
//   - items.properties has 1 or 2 keys
//   - one is "size" (optional); exactly one other key, naming a singular wire child
//   - the other key references or inlines an object schema
//
// Runs before hoistInlineObjects so hoisting sees the transformed shape.
// XML-format specs only — JSON specs may legitimately use the lossy shape.
func flattenClassicSizeWrappers(doc *openapi3.T) {
	if doc == nil || doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	visited := map[*openapi3.Schema]bool{}
	for _, ref := range doc.Components.Schemas {
		if ref != nil && ref.Value != nil {
			flattenClassicSizeWrappersInSchema(ref.Value, visited)
		}
	}
}

func flattenClassicSizeWrappersInSchema(schema *openapi3.Schema, visited map[*openapi3.Schema]bool) {
	if schema == nil || visited[schema] {
		return
	}
	visited[schema] = true
	for propName, propRef := range schema.Properties {
		if propRef == nil || propRef.Value == nil || propRef.Ref != "" {
			continue
		}
		if rewritten := flattenIfClassicWrapper(propRef.Value); rewritten != nil {
			schema.Properties[propName] = &openapi3.SchemaRef{Value: rewritten}
		}
	}
	for _, propRef := range schema.Properties {
		if propRef != nil && propRef.Value != nil {
			flattenClassicSizeWrappersInSchema(propRef.Value, visited)
		}
	}
	if schema.Items != nil && schema.Items.Value != nil {
		flattenClassicSizeWrappersInSchema(schema.Items.Value, visited)
	}
	for _, s := range schema.AllOf {
		if s != nil && s.Value != nil {
			flattenClassicSizeWrappersInSchema(s.Value, visited)
		}
	}
}

func flattenIfClassicWrapper(s *openapi3.Schema) *openapi3.Schema {
	if s == nil || !s.Type.Is("array") || s.Items == nil || s.Items.Ref != "" || s.Items.Value == nil {
		return nil
	}
	items := s.Items.Value
	if len(items.Properties) == 0 || len(items.Properties) > 2 {
		return nil
	}
	var sizeRef, itemRef *openapi3.SchemaRef
	var itemKey string
	for k, v := range items.Properties {
		switch k {
		case "size":
			sizeRef = v
		default:
			if itemRef != nil {
				// More than one non-size key — not a wrapper shape.
				return nil
			}
			itemRef = v
			itemKey = k
		}
	}
	if itemRef == nil || itemRef.Value == nil {
		return nil
	}
	// The non-size key must reference (or inline) an object schema.
	if itemRef.Ref == "" && len(itemRef.Value.Properties) == 0 {
		return nil
	}
	newProps := openapi3.Schemas{}
	if sizeRef != nil {
		newProps["size"] = sizeRef
	}
	newProps[itemKey] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:  &openapi3.Types{"array"},
			Items: itemRef,
		},
	}
	return &openapi3.Schema{
		Type:       &openapi3.Types{"object"},
		Properties: newProps,
		XML:        s.XML,
	}
}

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
		// Treat (Type==nil && Properties>0) as object-shape. Classic's
		// spec frequently omits `type: object` on inline subschemas that
		// are clearly objects (e.g. user_group.criteria.items, user_group.users.items).
		// Mirrors the extractTypes nil-type tolerance at the top of this file.
		hasObjShape := func(s *openapi3.Schema) bool {
			return s != nil && len(s.Properties) > 0 && (s.Type.Is("object") || s.Type == nil)
		}
		inlineObject := hasObjShape(v)
		inlineArrayOfObject := v.Type.Is("array") && v.Items != nil && v.Items.Ref == "" &&
			hasObjShape(v.Items.Value)
		if !inlineObject && !inlineArrayOfObject {
			return ref
		}
		if inlineObject {
			// Reuse a top-level schema with matching name + property keyset
			// instead of emitting a near-duplicate hoisted type. Classic's
			// spec sometimes inlines the same shape that's also defined as
			// a named component (e.g. user_group.criteria.items.criterion
			// matches the top-level `criterion` schema verbatim).
			if existing, ok := doc.Components.Schemas[propName]; ok && sameInlineShapeAsNamed(v, existing) {
				return &openapi3.SchemaRef{Ref: "#/components/schemas/" + propName, Value: existing.Value}
			}
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
		// inline array of object — hoist the element schema. First try to
		// dedup against an existing top-level schema whose name matches the
		// array's element wire shape (typically the singular of the property
		// name, e.g. property `criterion` is already singular, property
		// `users` maps to schema `user`). Mirrors the inline-object dedup
		// above so wrapper-flattened criteria/users/computers/mobile_devices
		// arrays point at the shared component schema instead of emitting a
		// fresh per-parent Item type.
		for _, candidate := range []string{propName, singularize(propName)} {
			if existing, ok := doc.Components.Schemas[candidate]; ok && sameInlineShapeAsNamed(v.Items.Value, existing) {
				v.Items = &openapi3.SchemaRef{Ref: "#/components/schemas/" + candidate, Value: existing.Value}
				return ref
			}
		}
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

// sameInlineShapeAsNamed reports whether `inline` and `named` (a top-level
// schema ref) share the same property keyset. Used by hoistInlineObjectsInSchema
// to deduplicate inline objects against a name-matched top-level schema
// instead of emitting a near-duplicate hoisted type. Conservative — only
// matches when key counts AND keys agree exactly; nested property types
// are not compared, since OpenAPI key collisions across unrelated shapes
// are rare in Jamf's specs (and tests will catch any divergence).
func sameInlineShapeAsNamed(inline *openapi3.Schema, named *openapi3.SchemaRef) bool {
	if inline == nil || named == nil || named.Value == nil {
		return false
	}
	if len(inline.Properties) != len(named.Value.Properties) {
		return false
	}
	for k := range inline.Properties {
		if _, ok := named.Value.Properties[k]; !ok {
			return false
		}
	}
	return true
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
// already taken by an unrelated schema. Checks both the spec namespace
// (exact key collisions) AND the Go-identifier namespace produced by
// goTypeName (e.g. "computer_extension_attributesItem" and
// "computerExtensionAttributesItem" are distinct spec keys but map to
// the same exported Go type "ComputerExtensionAttributesItem"). Without
// the Go-name check the generator silently emits duplicate Go type
// declarations and the package fails to compile.
func uniqueSchemaName(doc *openapi3.T, base string) string {
	taken := func(s string) bool {
		if _, exists := doc.Components.Schemas[s]; exists {
			return true
		}
		want := goTypeName(s)
		for k := range doc.Components.Schemas {
			if goTypeName(k) == want {
				return true
			}
		}
		return false
	}
	if !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if !taken(candidate) {
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

// collectAllSchemas returns all component schemas in the doc, marked as both
// request and response. Used in typesOnly mode where every schema should be
// emitted regardless of operation references.
func collectAllSchemas(doc *openapi3.T) map[string]*schemaUsage {
	all := make(map[string]*schemaUsage)
	if doc.Components == nil || doc.Components.Schemas == nil {
		return all
	}
	for name := range doc.Components.Schemas {
		all[name] = &schemaUsage{isRequest: true, isResponse: true}
	}
	return all
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

// currentFieldOverrides threads the per-spec override map into extractTypes
// without adding a parameter to schemaToGoType. Set by the caller for the
// duration of the extractTypes call; intentionally package-level because
// the schema walker already relies on package-level helpers.
var currentFieldOverrides map[string]string

// suppressWriteOnly suppresses write-only comments during type extraction.
// Set to true for typesOnly specs where the writeOnly annotation is misleading.
var suppressWriteOnly bool

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
			t := schemaToGoType(name, schema, usage.isRequest, format)
			t.XMLName = xmlName
			types = append(types, t)
			continue
		}
		if schema.Type == nil {
			// oneOf + discriminator with no explicit type field (some specs
			// omit "type: object" on union roots, e.g. BookmarkItem).
			if schema.Discriminator != nil && len(schema.OneOf) > 0 && !hasContentAddressedDiscriminator(schema) {
				types = append(types, schemaToDiscriminatorType(name, schema))
				continue
			}
			// Swagger 2.0 often omits type: object on definitions that are
			// clearly objects (Classic spec does this). If there are
			// properties, treat it as an object anyway.
			if len(schema.Properties) > 0 {
				t := schemaToGoType(name, schema, usage.isRequest, format)
				t.XMLName = xmlName
				types = append(types, t)
			} else if len(schema.AllOf) == 0 && len(schema.OneOf) == 0 && len(schema.AnyOf) == 0 {
				// Completely empty schema (e.g. JsonNode: {}) — no type, no
				// properties, no composition. Treat as freeform → json.RawMessage.
				comment := name + " represents a freeform JSON value."
				if schema.Description != "" {
					comment = name + " " + lowerFirst(cleanComment(schema.Description))
				}
				types = append(types, GoType{Name: name, Comment: comment, IsRawJSON: true})
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
		// Skip when the discriminator mapping keys are content-addressing
		// identifiers (contain dots, e.g. "com.jamf.ddm.*"). Those are value
		// discriminants for routing/documentation, not Go-safe type tags. The
		// schema falls through to normal struct generation instead.
		if schema.Discriminator != nil && len(schema.OneOf) > 0 && !hasContentAddressedDiscriminator(schema) {
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
	if resourceName == "" || resourceProp == nil {
		return GoType{}, false
	}
	// Classic spec quirk: some list specs nest the resource property as
	// `type: array` inside the items wrapper (computer_groups does this —
	// `computer_groups.items.computer_group: type: array, items: {...}`)
	// while the wire is still `<computer_groups><size/><computer_group>...
	// </computer_group>*</computer_groups>`. Without descending, the
	// wrapper emits `[][]ComputerGroupsItemComputerGroupItem` — a
	// double-slice Go can't decode against the flat repeated-child wire.
	// Drop the inner array; use its element type as the slice element.
	if resourceProp.Ref == "" && resourceProp.Value != nil &&
		resourceProp.Value.Type.Is("array") && resourceProp.Value.Items != nil {
		resourceProp = resourceProp.Value.Items
	}
	resourceGo := refName(resourceProp)
	fields := []GoField{}
	if sizeProp != nil {
		sizeGo := "*int"
		if sizeProp.Ref != "" {
			sizeGo = "*" + refName(sizeProp)
		}
		fields = append(fields, GoField{Name: "Size", Type: sizeGo, JSONTag: "size,omitempty"})
	}
	fields = append(fields, GoField{Name: exportedGoName(plural(resourceName)), Type: "[]" + resourceGo, JSONTag: resourceName})
	wrapper := GoType{
		Name:          goName,
		XMLName:       specName,
		IsListWrapper: true,
		Comment:       fmt.Sprintf("%s wraps a Jamf Classic list response with a flat slice of %s.", goName, resourceGo),
		Fields:        fields,
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
	// Named aliases to scalar types (e.g. `type Size = int`) are written
	// as `*Size` in fields but are not real sub-objects; without this
	// registry the loop misclassifies any type with a `*Size` field as
	// "has a sub-object" and injects a bogus top-level ID into hoisted
	// wrapper types like UserGroupCriteria / ComputerGroupComputers.
	scalarAliases := map[string]bool{}
	for _, t := range types {
		if t.AliasTarget != "" && isScalar(t.AliasTarget) {
			scalarAliases[t.Name] = true
		}
	}
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
			// *bool), scalar-aliased ptr fields (*Size), and slice/map types
			// don't count.
			if strings.HasPrefix(f.Type, "*") && !strings.HasPrefix(f.Type, "*[]") &&
				!isScalar(strings.TrimPrefix(f.Type, "*")) &&
				!scalarAliases[strings.TrimPrefix(f.Type, "*")] &&
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

// hasContentAddressedDiscriminator reports whether any discriminator mapping
// key contains a dot. Dot-containing keys are reverse-DNS content addresses
// (e.g. "com.jamf.ddm.passcode-settings"), not simple type discriminators.
// Schemas with such keys should not be emitted as Go union structs because
// the keys cannot become valid Go identifiers via exportedGoName.
func hasContentAddressedDiscriminator(schema *openapi3.Schema) bool {
	if schema.Discriminator == nil {
		return false
	}
	for k := range schema.Discriminator.Mapping {
		if strings.Contains(k, ".") {
			return true
		}
	}
	return false
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
		addVariant(mapKey, exportedGoName(parts[len(parts)-1]))
	}
	if len(gt.Discriminator.Variants) == 0 {
		for _, one := range schema.OneOf {
			if one.Ref == "" {
				continue
			}
			parts := strings.Split(one.Ref, "/")
			tn := parts[len(parts)-1]
			addVariant(tn, exportedGoName(tn))
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
		maps.Copy(props, s.Properties)
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

	// specName is the lowercase_snake name the override table keys on.
	// Passed down through currentFieldOverrides; we recover it by lowering
	// the Go identifier back to snake case — good enough for the Classic
	// spec's naming which is already snake_case.
	specName := goNameToSpecName(name)

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
		// Per-spec field-type override (config.fieldTypeOverrides). Applied
		// at the shallowest opportunity so needsPtr/isNullable reasoning
		// below still drives pointer wrapping exactly as it would for the
		// spec-declared type. Override lookup: exact "schema.prop" wins
		// over the "*.prop" wildcard. The wildcard lets a single entry
		// cover both a canonical schema and its hoisted list-item clones
		// (e.g. computer_invitation's invitation field appears under the
		// parent schema AND under ComputerInvitationsItemComputerInvitation
		// after array hoisting; one "*.invitation" entry handles both).
		if currentFieldOverrides != nil {
			if v, ok := currentFieldOverrides[specName+"."+pname]; ok {
				goType = v
			} else if v, ok := currentFieldOverrides["*."+pname]; ok {
				goType = v
			}
		}
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

		if format == "xml" && !strings.HasPrefix(goType, "*") && goType != "any" {
			// XML mode: every field is `*T` with `,omitempty`, including
			// slices/maps. Matches the three-state null/empty/value
			// semantics the Terraform plugin framework depends on for
			// Classic-backed resources — a nil slice field must be
			// distinguishable from an empty slice so the provider can
			// send "omit" vs "clear" to the server.
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if isRequest && !isRequired && !strings.HasPrefix(goType, "*") && (strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[")) {
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
		if !suppressWriteOnly && prop != nil && (prop.WriteOnly || prop.Format == "password") {
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

// goNameToSpecName converts a PascalCase Go type name back to its spec
// snake_case form for looking up per-field config overrides. Relies on
// the generator's round-trip property: exportedGoName("computer_invitation")
// == "ComputerInvitation", so the inverse is just splitting on case
// boundaries and re-joining with underscores (lowercased). Good enough
// for the Classic/Pro specs whose schema names are either snake_case or
// camelCase; would need refinement for specs with exotic naming.
func goNameToSpecName(goName string) string {
	return toSnakeCase(goName)
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
	if before, ok := strings.CutSuffix(specName, "_post"); ok {
		return before
	}
	return specName
}
