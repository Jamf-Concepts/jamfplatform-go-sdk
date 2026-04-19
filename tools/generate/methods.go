// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// ---------------------------------------------------------------------------
// Operations → Go methods
// ---------------------------------------------------------------------------

func extractMethods(doc *openapi3.T, spec SpecDef) ([]GoMethod, error) {
	var methods []GoMethod
	for _, opDef := range spec.Operations {
		m, err := buildMethod(doc, spec, opDef)
		if err != nil {
			return nil, fmt.Errorf("operation %s: %w", opDef.Op, err)
		}
		methods = append(methods, m)
	}
	return methods, nil
}

// extractMultipartFields walks a multipart/form-data request body schema's
// properties and returns a list of multipart fields for generator use.
// Binary fields (format: binary) become file uploads; other scalars become
// string form fields.
func extractMultipartFields(schema *openapi3.Schema) []GoMultipartField {
	if schema == nil {
		return nil
	}
	fields := make([]GoMultipartField, 0, len(schema.Properties))
	for _, name := range sortedKeys(schema.Properties) {
		prop := schema.Properties[name].Value
		// A field is a file upload when the spec marks it as such
		// (string + format: binary) OR — as a fallback for spec bugs —
		// when the field is conventionally named "file" with string
		// type but no format. The Jamf Pro spec has at least two
		// endpoints (/v2/inventory-preload/csv, csv-validate) where
		// the author omitted format: binary, and treating those as a
		// plain form string would emit a callable signature that
		// doesn't actually accept a file. Path-name heuristic is safe
		// because any real JSON form field named "file" would be a
		// genuine file upload semantically.
		isFile := prop != nil && prop.Type != nil && prop.Type.Is("string") &&
			(prop.Format == "binary" || name == "file")
		f := GoMultipartField{
			Name:   name,
			GoName: toLowerCamelCase(name),
			IsFile: isFile,
		}
		if !isFile && prop != nil {
			f.Type = schemaRefToGoType(schema.Properties[name])
		}
		fields = append(fields, f)
	}
	return fields
}

// sortedContentEntries iterates a content map in a deterministic order so
// generator output doesn't flip on map iteration randomness.
func sortedContentEntries(content map[string]*openapi3.MediaType) func(yield func(string, *openapi3.MediaType) bool) {
	return func(yield func(string, *openapi3.MediaType) bool) {
		keys := make([]string, 0, len(content))
		for k := range content {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !yield(k, content[k]) {
				return
			}
		}
	}
}

// isRateLimited reports whether the operation carries x-rate-limit: true.
// kin-openapi stores vendor extensions as raw JSON bytes keyed by the
// extension name.
func isRateLimited(op *openapi3.Operation) bool {
	if op == nil {
		return false
	}
	raw, ok := op.Extensions["x-rate-limit"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case []byte:
		return string(v) == "true"
	case string:
		return v == "true"
	default:
		// Some kin-openapi versions return json.RawMessage.
		s := fmt.Sprintf("%s", v)
		return s == "true"
	}
}

// dropDeprecatedOps returns spec.Operations with any operations whose spec
// marks them deprecated removed. Logs each drop so the curator can see why
// the generated surface shrank.
func dropDeprecatedOps(doc *openapi3.T, spec SpecDef) []OperationDef {
	kept := make([]OperationDef, 0, len(spec.Operations))
	for _, opDef := range spec.Operations {
		httpMethod, specPath := opDef.parseOp()
		pathItem := doc.Paths.Find(specPath)
		if pathItem == nil {
			kept = append(kept, opDef)
			continue
		}
		op := pathItem.GetOperation(httpMethod)
		if op != nil && op.Deprecated {
			log.Printf("skipping deprecated operation %s (%s)", opDef.Name, opDef.Op)
			continue
		}
		kept = append(kept, opDef)
	}
	return kept
}

func buildMethod(doc *openapi3.T, spec SpecDef, opDef OperationDef) (GoMethod, error) {
	httpMethod, specPath := opDef.parseOp()

	pathItem := doc.Paths.Find(specPath)
	if pathItem == nil {
		return GoMethod{}, fmt.Errorf("path %s not found in spec", specPath)
	}
	op := pathItem.GetOperation(httpMethod)
	if op == nil {
		return GoMethod{}, fmt.Errorf("%s not found", opDef.Op)
	}

	// Version: operation override > extract from path > "v1"
	version := coalesce(opDef.Version, extractVersion(specPath))

	m := GoMethod{
		Name:            opDef.Name,
		Format:          spec.Format,
		HTTPMethod:      httpMethod,
		Namespace:       spec.Namespace,
		Version:         version,
		ResourcePath:    stripVersionPrefix(specPath),
		QueryParams:     opDef.parseParams(),
		ContentType:     opDef.ContentType,
		PaginationStyle: opDef.Pagination,
		PageSizeParam:   coalesce(opDef.PageSizeParam, "page-size"),
		ResultsField:    "results",
		SpecPath:        specPath,
		UnwrapResults:   opDef.UnwrapResults,
	}

	if op.Summary != "" {
		m.Comment = opDef.Name + " " + lowerFirst(cleanComment(op.Summary))
	}

	if len(op.Tags) > 0 {
		m.Tag = op.Tags[0]
	}

	if isRateLimited(op) {
		if m.Comment != "" {
			m.Comment += "\n//\n// This endpoint is rate-limited. The transport retries a 429 response once if the server returns a bounded Retry-After; otherwise the 429 surfaces as an APIResponseError so the caller can apply its own backoff policy."
		} else {
			m.Comment = opDef.Name + " is rate-limited."
		}
	}

	if op.Deprecated {
		if m.Comment == "" {
			m.Comment = opDef.Name + " is deprecated."
		}
		m.Comment += "\n//\n// Deprecated: this endpoint is marked deprecated in the Jamf API spec and may be removed in a future release."
	}

	m.PathParams = extractPathParams(specPath, opDef.PathNames)
	m.ExpectedStatus, m.ResponseType = detectResponse(op)
	// When detectResponse populates ResponseType from the spec, capture
	// the matching schema's XML wire name so test stubs emit bodies the
	// generated decoder accepts. The later config-level responseType
	// override re-derives this, so only fill when both are unset.
	if m.ResponseType != "" && opDef.ResponseType == "" && doc.Components != nil && doc.Components.Schemas != nil {
		for specName, ref := range doc.Components.Schemas {
			if goTypeName(specName) != m.ResponseType {
				continue
			}
			if ref.Value != nil && ref.Value.XML != nil && ref.Value.XML.Name != "" {
				m.ResponseWireName = ref.Value.XML.Name
			} else {
				m.ResponseWireName = specName
			}
			break
		}
	}

	// Request body
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		if mpContent, ok := op.RequestBody.Value.Content["multipart/form-data"]; ok && mpContent.Schema != nil && mpContent.Schema.Value != nil {
			m.MultipartFields = extractMultipartFields(mpContent.Schema.Value)
		} else {
			// Pick the first content-type the spec declares. The generator
			// emits it verbatim so endpoints that spec application/merge-patch+json,
			// application/x-www-form-urlencoded, or application/xml travel with
			// the correct Content-Type header rather than relying on transport
			// heuristics.
			// Honor the spec's declared content-type verbatim. The transport
			// has method-based defaults (PATCH -> merge-patch+json) that
			// would override an endpoint spec'd as application/json — so
			// we always set it explicitly when declared.
			for ct, content := range sortedContentEntries(op.RequestBody.Value.Content) {
				if content.Schema != nil {
					m.RequestType = refName(content.Schema)
					m.ContentType = ct
					break
				}
			}
		}
	}

	// Config-level overrides for request/response types and expected status.
	// Used when the spec is untyped (e.g. Jamf Classic) so the curator
	// explicitly names the schema from definitions/. Names are spec-level
	// (may be snake_case); the generator normalises to Go PascalCase.
	if opDef.RequestType != "" {
		m.RequestType = goTypeName(opDef.RequestType)
	}
	if opDef.ResponseType != "" {
		m.ResponseType = goTypeName(opDef.ResponseType)
		// XML wire name is the raw spec name unless the schema overrides
		// via xml.name — test stubs emit <wireName> bodies so the generated
		// type's XMLName check passes.
		m.ResponseWireName = opDef.ResponseType
		if doc.Components != nil && doc.Components.Schemas != nil {
			if ref, ok := doc.Components.Schemas[opDef.ResponseType]; ok && ref.Value != nil && ref.Value.XML != nil && ref.Value.XML.Name != "" {
				m.ResponseWireName = ref.Value.XML.Name
			}
		}
	}
	if opDef.ExpectedStatus != 0 {
		m.ExpectedStatus = opDef.ExpectedStatus
	}

	// Paginated item type
	if m.PaginationStyle != "" {
		m.ItemType = detectPaginatedItemType(op)
		m.ResponseType = ""
	}

	m.ReturnsSlice = strings.HasPrefix(m.ResponseType, "[]")

	// Determine category
	m.Category = categorize(m)

	// rawBody specs: generator emits []byte methods with no struct marshaling.
	// Resets type-driven state so the "raw" template takes over.
	if spec.RawBody {
		m.Category = "raw"
		m.MultipartFields = nil
		m.PaginationStyle = ""
		m.UnwrapResults = ""
		m.ItemType = ""
		m.ReturnsSlice = false
		if httpMethod == http.MethodGet || httpMethod == http.MethodDelete {
			m.RequestType = ""
		} else {
			m.RequestType = "[]byte"
		}
		if httpMethod == http.MethodDelete {
			m.ResponseType = ""
		} else {
			m.ResponseType = "[]byte"
		}
	}

	return m, nil
}

func categorize(m GoMethod) string {
	if len(m.MultipartFields) > 0 {
		return "multipart"
	}
	if m.UnwrapResults != "" {
		return "unwrap"
	}
	if m.PaginationStyle != "" {
		return "paginated"
	}
	hasReq := m.RequestType != ""
	hasResp := m.ResponseType != ""
	isOK := m.ExpectedStatus == 200

	// "create" covers any shape that sends a body AND returns one — POST 201,
	// PUT/PATCH 200, etc. Naming is historical; the template is request+response.
	switch {
	case hasReq && hasResp:
		return "create"
	case isOK && hasResp:
		return "get"
	case hasResp:
		return "actionWithResponse"
	case hasReq:
		return "update"
	default:
		return "action"
	}
}

// ---------------------------------------------------------------------------
// Spec helpers
// ---------------------------------------------------------------------------

var pathParamRe = regexp.MustCompile(`\{(\w+)\}`)
var versionPrefixRe = regexp.MustCompile(`^/v\d+`)

func extractPathParams(path string, overrides map[string]string) []GoPathParam {
	matches := pathParamRe.FindAllStringSubmatch(path, -1)
	params := make([]GoPathParam, 0, len(matches))
	for _, m := range matches {
		specName := m[1]
		var goName string
		if override, ok := overrides[specName]; ok {
			goName = override
		} else {
			goName = toLowerCamelCase(specName)
		}
		params = append(params, GoPathParam{SpecName: specName, GoName: goName})
	}
	return params
}

func stripVersionPrefix(path string) string {
	return versionPrefixRe.ReplaceAllString(path, "")
}

// extractVersion returns "v1" from "/v1/devices" or "v2" from "/v2/benchmarks".
// Returns empty string for non-versioned paths (e.g. "/startup-status") so
// tenantPrefix collapses the segment instead of forcing "v1". Callers that
// need a specific version for a non-versioned path must set it in config.
func extractVersion(path string) string {
	match := versionPrefixRe.FindString(path)
	if match == "" {
		return ""
	}
	return match[1:] // strip leading "/"
}

func detectResponse(op *openapi3.Operation) (int, string) {
	for _, code := range []int{200, 201, 202, 204} {
		resp := op.Responses.Status(code)
		if resp == nil {
			continue
		}
		if resp.Value == nil {
			return code, ""
		}
		// Deterministic iteration order: Go maps randomize iteration, so
		// when a response declares multiple content types (e.g. Swagger 2.0
		// `produces: [application/xml, application/json]` converts to two
		// entries with the same schema), picking the "first" non-JSON
		// caused drift between runs (typed vs []byte). Prefer a typed
		// schema over raw bytes; prefer JSON over other content types
		// (the spec schema is the authoritative description of the typed
		// shape regardless of on-the-wire codec, and Classic resources
		// use JSON pagination wrappers in docs even when XML on the wire).
		cts := make([]string, 0, len(resp.Value.Content))
		for ct := range resp.Value.Content {
			cts = append(cts, ct)
		}
		sort.Slice(cts, func(i, j int) bool {
			a, b := cts[i], cts[j]
			aj, bj := isJSONContentType(a), isJSONContentType(b)
			if aj != bj {
				return aj
			}
			return a < b
		})
		for _, ct := range cts {
			content := resp.Value.Content[ct]
			if content.Schema == nil {
				continue
			}
			// Non-JSON content (text/csv, application/octet-stream,
			// application/xml for JSON-format specs, …) returns raw bytes
			// regardless of any schema hint — a CSV export schema for
			// example only carries `format: binary` and callers want the
			// bytes, not an any-typed deserialisation.
			if !isJSONContentType(ct) {
				return code, "[]byte"
			}
			if ref := refName(content.Schema); ref != "" {
				return code, ref
			}
		}
		return code, ""
	}
	return 200, ""
}

// isJSONContentType reports whether ct is a JSON content type we should
// decode via encoding/json. Anything else is treated as raw bytes.
func isJSONContentType(ct string) bool {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	return base == "" || base == "application/json" || strings.HasSuffix(base, "+json")
}

func detectPaginatedItemType(op *openapi3.Operation) string {
	resp := op.Responses.Status(200)
	if resp == nil || resp.Value == nil {
		return "any"
	}
	for _, content := range resp.Value.Content {
		if content.Schema == nil {
			continue
		}
		schema := content.Schema.Value
		if schema == nil {
			continue
		}
		// allOf composition (pagination wrapper)
		for _, part := range schema.AllOf {
			if part.Value == nil {
				continue
			}
			if r := part.Value.Properties["results"]; r != nil && r.Value != nil && r.Value.Items != nil {
				return refName(r.Value.Items)
			}
		}
		// Direct results field
		if r := schema.Properties["results"]; r != nil && r.Value != nil && r.Value.Items != nil {
			return refName(r.Value.Items)
		}
		// Raw array response — no wrapper, items live at the top level.
		// Paired with pagination style "rawArray" in config.
		if schema.Type.Is("array") && schema.Items != nil {
			return refName(schema.Items)
		}
	}
	return "any"
}
