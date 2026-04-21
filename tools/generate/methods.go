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
	methods, err := appendResolverMethods(methods, spec)
	if err != nil {
		return nil, err
	}
	methods, err = appendApplyMethods(doc, methods, spec)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// appendResolverMethods synthesizes one pair of resolver methods per
// operation that carries a Resolver config. Each pair consists of:
//
//   - Resolve<ResourceType>IDByName(ctx, name) (string, error)
//   - Resolve<ResourceType>ByName(ctx, name) (*<TypedReturn>, error)
//
// Synthetic methods inherit Namespace/Version/ResourcePath/Tag from the
// source operation so they land in the same per-tag file and build the
// list URL identically to the source List method.
func appendResolverMethods(methods []GoMethod, spec SpecDef) ([]GoMethod, error) {
	byName := make(map[string]*GoMethod, len(methods))
	for i := range methods {
		byName[methods[i].Name] = &methods[i]
	}
	out := append([]GoMethod(nil), methods...)
	for _, opDef := range spec.Operations {
		// Merge singular and plural resolver configs into one slice.
		var resolvers []ResolverConfig
		if opDef.Resolver != nil {
			resolvers = append(resolvers, *opDef.Resolver)
		}
		resolvers = append(resolvers, opDef.Resolvers...)
		if len(resolvers) == 0 {
			continue
		}
		for _, r := range resolvers {
		switch r.Mode {
		case "filtered", "clientFilter", "direct":
			// supported
		case "":
			return nil, fmt.Errorf("resolver on %s: mode required (filtered, clientFilter, or direct)", opDef.Name)
		default:
			return nil, fmt.Errorf("resolver on %s: unknown mode %q", opDef.Name, r.Mode)
		}
		if r.ResourceType == "" {
			return nil, fmt.Errorf("resolver on %s: resourceType is required", opDef.Name)
		}
		// filtered/clientFilter require name/id field paths; direct mode
		// doesn't — the typed source method already decodes the response
		// and the ID is taken from the top-level *int ID field uniformly.
		if r.Mode != "direct" && (r.NameField == "" || r.IDField == "") {
			return nil, fmt.Errorf("resolver on %s: nameField and idField are required for mode %q", opDef.Name, r.Mode)
		}
		src, ok := byName[opDef.Name]
		if !ok {
			return nil, fmt.Errorf("resolver on %s: source operation not found in extracted methods", opDef.Name)
		}
		typedReturn := r.TypedReturn
		if typedReturn == "" {
			typedReturn = r.ResourceType
		}
		byField := r.ByField
		if byField == "" {
			byField = "ByName"
		}
		matchField := r.MatchField
		if matchField == "" {
			matchField = r.NameField
		}
		gr := &GoResolver{
			ResourceType: r.ResourceType,
			Mode:         r.Mode,
			NameField:    r.NameField,
			MatchField:   matchField,
			IDField:      r.IDField,
			IDNumeric:    r.IDNumeric,
			SearchParam:  r.SearchParam,
			ResultsField: r.ResultsField,
			TypedReturn:  typedReturn,
			ExtraParams:  r.ExtraParams,
			Paginated:    r.Mode == "clientFilter" && opDef.Pagination != "",
			ByField:      byField,
			SourceMethod: opDef.Name,
		}
		if r.Mode == "direct" {
			// Pre-expand the Go field chain for direct-mode emission.
			// Composite Classic resources return ID nested under General
			// (policies, mac_application, ebook, …); flat resources have a
			// top-level ID. The config's idField is a Go-struct dot-path —
			// "ID" (default) or "General.ID" for composites. Generator
			// emits per-step nil checks so the resolver surfaces a clean
			// "response missing id" error instead of nil-derefing.
			path := r.IDField
			if path == "" {
				path = "ID"
			}
			parts := strings.Split(path, ".")
			checks := []string{"r == nil"}
			expr := "r"
			for _, p := range parts {
				expr += "." + p
				checks = append(checks, expr+" == nil")
			}
			gr.IDNilCheck = strings.Join(checks, " || ")
			gr.IDDeref = "*" + expr
			xmlBody := "42"
			for i := len(parts) - 1; i >= 0; i-- {
				tag := strings.ToLower(parts[i])
				xmlBody = "<" + tag + ">" + xmlBody + "</" + tag + ">"
			}
			gr.IDTestInnerXML = xmlBody
		}
		// Base synthetic method — shared fields. ResourcePath/SpecPath
		// come from the source op: a List op for filtered/clientFilter
		// (no {name} placeholder), or the GetByName op for direct (which
		// does carry {name}; the test-stub handler resolves that to
		// "test-id" the same way typed GET stubs do).
		base := GoMethod{
			Namespace:        src.Namespace,
			Version:          src.Version,
			Tag:              src.Tag,
			Format:           src.Format,
			ResourcePath:     src.ResourcePath,
			SpecPath:         src.SpecPath,
			HTTPMethod:       http.MethodGet,
			ResponseWireName: src.ResponseWireName,
			Resolver:         gr,
		}
		idMethod := base
		idMethod.Name = "Resolve" + r.ResourceType + "ID" + byField
		typedMethod := base
		typedMethod.Name = "Resolve" + r.ResourceType + byField
		if r.Mode == "direct" {
			idMethod.Category = "resolverIDDirect"
			idMethod.Comment = idMethod.Name + " looks up a " + r.ResourceType + " by name via " + opDef.Name + " and returns its ID as a string. Returns an error when the underlying call returns a nil ID."
			typedMethod.Category = "resolverTypedDirect"
			typedMethod.Comment = typedMethod.Name + " looks up a " + r.ResourceType + " by name. Alias for " + opDef.Name + "; present so callers can use the same Resolve<X>ByName spelling across all resources regardless of resolver mode."
		} else {
			idMethod.Category = "resolverID"
			idMethod.Comment = idMethod.Name + " looks up a " + r.ResourceType + " by its " + r.NameField + " field and returns the ID. Returns *APIResponseError with HasStatus(404) when no match exists, or *AmbiguousMatchError when multiple resources share the name."
			typedMethod.Category = "resolverTyped"
			typedMethod.Comment = typedMethod.Name + " looks up a " + r.ResourceType + " by its " + r.NameField + " field and returns the decoded resource. Shares the same HTTP call as the ID-only variant; error semantics are identical."
		}

		out = append(out, idMethod, typedMethod)
		} // end for resolvers
	}
	return out, nil
}

// appendApplyMethods synthesizes an Apply<ResourceType> upsert method for
// each resolver that has an Apply config block. The Apply method:
//  1. Extracts the name from the request struct
//  2. Calls Resolve<ResourceType>IDByName
//  3. If 404: calls Create, returns (newID, true, nil)
//  4. If found: calls Update with the resolved ID, returns (existingID, false, nil)
//  5. Ambiguous/other errors propagate as-is
func appendApplyMethods(doc *openapi3.T, methods []GoMethod, spec SpecDef) ([]GoMethod, error) {
	byName := make(map[string]*GoMethod, len(methods))
	for i := range methods {
		byName[methods[i].Name] = &methods[i]
	}
	out := append([]GoMethod(nil), methods...)
	for _, opDef := range spec.Operations {
		var resolvers []ResolverConfig
		if opDef.Resolver != nil {
			resolvers = append(resolvers, *opDef.Resolver)
		}
		resolvers = append(resolvers, opDef.Resolvers...)
		for _, r := range resolvers {
			if r.Apply == nil {
				continue
			}
			ac := r.Apply
			if ac.CreateOp == "" || ac.UpdateOp == "" || ac.NameGoField == "" {
				return nil, fmt.Errorf("apply on %s/%s: createOp, updateOp, and nameGoField are required", opDef.Name, r.ResourceType)
			}
			createM, ok := byName[ac.CreateOp]
			if !ok {
				return nil, fmt.Errorf("apply on %s: createOp %q not found", r.ResourceType, ac.CreateOp)
			}
			updateM, ok := byName[ac.UpdateOp]
			if !ok {
				return nil, fmt.Errorf("apply on %s: updateOp %q not found", r.ResourceType, ac.UpdateOp)
			}
			// Determine request type from the Create method.
			requestType := createM.RequestType
			if requestType == "" {
				return nil, fmt.Errorf("apply on %s: createOp %q has no request type", r.ResourceType, ac.CreateOp)
			}
			// Determine how to extract ID from create response.
			createReturnID := "resp.ID"
			switch createM.ResponseType {
			case "HrefResponse", "AppInstallerDeploymentHrefResponse":
				createReturnID = "resp.ID"
			default:
				// Non-HrefResponse: check if the response has a string or int ID.
				// The resolver's IDNumeric flag tells us.
				if r.IDNumeric {
					createReturnID = "strconv.Itoa(resp.ID)"
				} else {
					createReturnID = "resp.ID"
				}
			}
			// Determine if Update returns a value or just error.
			updateReturnsVal := updateM.ResponseType != ""
			// Determine extra args from Create's query params.
			var extraArgs, extraCallArgs, extraTestCallArgs string
			for _, qp := range createM.QueryParams {
				goType := qp.Type
				if goType == "" {
					goType = "string"
				}
				extraArgs += ", " + qp.Go + " " + goType
				extraCallArgs += ", " + qp.Go
				// Literal zero value for test calls.
				switch goType {
				case "bool":
					extraTestCallArgs += ", false"
				case "int", "int64":
					extraTestCallArgs += ", 0"
				default:
					extraTestCallArgs += ", \"\""
				}
			}
			// Determine if this is a Classic create (takes id as first path param).
			classicCreate := spec.Format == "xml"
			// Determine whether the name field is a pointer on the generated
			// Go struct. XML specs always pointer-ify; for JSON specs we
			// check whether the field is in the schema's required list —
			// non-required scalars become pointers per schema.go logic.
			nameIsPointer := spec.Format == "xml"
			if !nameIsPointer {
				if schemaRef, ok := doc.Components.Schemas[requestType]; ok && schemaRef != nil && schemaRef.Value != nil {
					// Map Go field name → JSON property name via the same
					// exportedGoName conversion the schema emitter uses.
					nameJSONField := ""
					for pname := range schemaRef.Value.Properties {
						if exportedGoName(pname) == ac.NameGoField {
							nameJSONField = pname
							break
						}
					}
					if nameJSONField != "" {
						isRequired := false
						for _, req := range schemaRef.Value.Required {
							if req == nameJSONField {
								isRequired = true
								break
							}
						}
						if !isRequired {
							nameIsPointer = true
						}
					}
				}
			}
			// Resolver method name (always the ByName variant).
			resolverMethod := "Resolve" + r.ResourceType + "IDByName"
			// Delete method (for test generation).
			deleteMethod := ac.DeleteOp

			// Inherit tag/namespace/version from the source list operation.
			src := byName[opDef.Name]
			if src == nil {
				return nil, fmt.Errorf("apply on %s: source op %q not found", r.ResourceType, opDef.Name)
			}

			ga := &GoApply{
				ResourceType:     r.ResourceType,
				RequestType:      requestType,
				NameGoField:      ac.NameGoField,
				ResolverMethod:   resolverMethod,
				CreateMethod:     ac.CreateOp,
				UpdateMethod:     ac.UpdateOp,
				DeleteMethod:     deleteMethod,
				CreateReturnID:    createReturnID,
				IDNumeric:         r.IDNumeric,
				UpdateReturnsVal: updateReturnsVal,
				ExtraArgs:        extraArgs,
				ExtraCallArgs:     extraCallArgs,
				ExtraTestCallArgs: extraTestCallArgs,
				ClassicCreate:    classicCreate,
				NameIsPointer:    nameIsPointer,
				// Test generation paths.
				ListNamespace: src.Namespace,
				ListVersion:   src.Version,
				ListPath:      src.ResourcePath,
				ListNameField: r.NameField,
				ListIDField:   r.IDField,
				CreateNS:      createM.Namespace,
				CreateVer:     createM.Version,
				CreatePath:    createM.ResourcePath,
				CreateStatus:  createM.ExpectedStatus,
				UpdateNS:      updateM.Namespace,
				UpdateVer:     updateM.Version,
				UpdatePath:    updateM.ResourcePath,
				UpdateStatus:  updateM.ExpectedStatus,
			}
			// Check if list and create share the same URL (both path and
			// namespace/version match). When true, the test template must
			// register a single mux handler that dispatches on HTTP method
			// instead of two separate handlers that would panic.
			ga.SameListCreatePath = ga.ListNamespace == ga.CreateNS &&
				ga.ListVersion == ga.CreateVer &&
				ga.ListPath == ga.CreatePath

			m := GoMethod{
				Name:      "Apply" + r.ResourceType,
				Category:  "apply",
				Comment:   "Apply" + r.ResourceType + " creates or updates a " + r.ResourceType + " by name. If a resource with the specified name exists, it is updated; if not found, a new resource is created. Returns the resource ID, whether it was created (true) or updated (false), and any error. An *AmbiguousMatchError is returned if multiple resources match the name.",
				Tag:       src.Tag,
				Namespace: src.Namespace,
				Version:   src.Version,
				Format:    src.Format,
				Apply:     ga,
			}
			out = append(out, m)
		}
	}
	return out, nil
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

// pathParamRe matches OpenAPI path parameter placeholders. Allows
// hyphens (e.g. {panel-id}) as well as the usual alphanumerics — the
// Jamf Pro spec uses kebab-case segment names in a handful of places
// (enrollment-customization panels).
var pathParamRe = regexp.MustCompile(`\{([\w-]+)\}`)
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
