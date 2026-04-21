// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Configuration types (loaded from config.json)
// ---------------------------------------------------------------------------

type Config struct {
	Package string    `json:"package"`
	Module  string    `json:"module"`
	SpecDir string    `json:"specDir"`
	Specs   []SpecDef `json:"specs"`
}

type SpecDef struct {
	File               string                       `json:"file"`
	Namespace          string                       `json:"namespace"`
	SpecFile           string                       `json:"specFile,omitempty"`   // override published spec filename
	Package            string                       `json:"package,omitempty"`    // target Go sub-package under jamfplatform/; empty emits to root (legacy)
	SplitByTag         bool                         `json:"splitByTag,omitempty"` // emit one methods file per OpenAPI tag instead of one per spec
	Format             string                       `json:"format,omitempty"`     // "json" (default) or "xml" — drives struct tag style and transport codec
	RawBody            bool                         `json:"rawBody,omitempty"`    // generate methods that take/return []byte instead of typed structs; consumer owns marshaling (used for Classic where spec has no useful types)
	Operations         []OperationDef               `json:"operations"`
	ExcludePaths       []string                     `json:"excludePaths,omitempty"`       // "METHOD /path" entries the generator must refuse to include
	SkipDeprecated     bool                         `json:"skipDeprecated,omitempty"`     // omit operations marked deprecated in the spec
	FieldTypeOverrides map[string]string            `json:"fieldTypeOverrides,omitempty"` // "schema_name.property_name" -> Go type, used to correct spec bugs (e.g. `integer` fields where the server actually returns a non-int64 string). Applied per-spec so upstream spec updates don't get silently overwritten.
	SchemaAdditions    map[string]map[string]string `json:"schemaAdditions,omitempty"`    // "schema_name" -> { "property_name": "openapi_type" }, inject missing properties into a spec schema. Used when the spec omits a field the server accepts but we need to send (e.g. Classic's account schema has no `password` property). openapi_type is one of "string", "integer", "boolean", "string:password" (writeOnly string).
}

// baseName derives a Go file base name from the spec file path.
// "testing/device-inventory.yaml" → "device_inventory"
// "testing/benchmarks-report.yaml" → "benchmarks_report"
func (s SpecDef) baseName() string {
	name := filepath.Base(s.File)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	return strings.ReplaceAll(name, "-", "_")
}

func (s SpecDef) outputFile() string     { return "jamfplatform/" + s.baseName() + ".go" }
func (s SpecDef) testOutputFile() string { return "jamfplatform/" + s.baseName() + "_test.go" }

type OperationDef struct {
	Op             string            `json:"op"`   // "GET /v1/devices/{id}"
	Name           string            `json:"name"` // Go method name
	ContentType    string            `json:"contentType,omitempty"`
	Pagination     string            `json:"pagination,omitempty"` // hasNext, sizeCheck, totalCount, rawArray
	PageSizeParam  string            `json:"pageSizeParam,omitempty"`
	Version        string            `json:"version,omitempty"`   // override version for tenantPrefix
	PathNames      map[string]string `json:"pathNames,omitempty"` // spec param -> Go param name
	Params         []string          `json:"params,omitempty"`    // "name", "name:type", "spec:type:goName"
	UnwrapResults  string            `json:"unwrapResults,omitempty"`
	RequestType    string            `json:"requestType,omitempty"`    // explicit request schema name (used when spec body is untyped, e.g. Classic)
	ResponseType   string            `json:"responseType,omitempty"`   // explicit response schema name (same)
	ExpectedStatus int               `json:"expectedStatus,omitempty"` // explicit success status code (default 200)
	Resolver       *ResolverConfig   `json:"resolver,omitempty"`       // attach name->ID resolver emission to this operation (typically a List op)
	Resolvers      []ResolverConfig  `json:"resolvers,omitempty"`      // attach multiple resolvers to one operation (e.g. resolve device by name AND by serialNumber)
}

// ResolverConfig declares a name->ID resolver the generator should emit
// alongside the operation it attaches to. Produces two methods per resource:
// Resolve<ResourceType>IDByName (returns string ID) and
// Resolve<ResourceType>ByName (returns the typed resource).
type ResolverConfig struct {
	ResourceType string `json:"resourceType"`            // Go type name used in emitted method names (e.g. "Blueprint")
	NameField    string `json:"nameField"`               // dot-notation JSON path for the name field on each list element (e.g. "name", "general.name", "title")
	MatchField   string `json:"matchField,omitempty"`    // optional: dot-notation JSON path for client-side match verification when it differs from nameField (e.g. RSQL uses "displayName" but response nests it at "general.displayName"). Empty defaults to nameField.
	IDField      string `json:"idField"`                 // dot-notation JSON path for the ID field (e.g. "id")
	IDNumeric    bool   `json:"idNumeric,omitempty"`     // when true, the ID field is a number in JSON (int in Go); test stubs emit numeric IDs
	IDPointer    bool   `json:"idPointer,omitempty"`     // when true, the ID field is a pointer (*int) in Go; overrides IDNumeric's strconv.Itoa to use fmt.Sprintf with dereference
	Mode         string `json:"mode"`                    // "filtered" (server-side RSQL) or "clientFilter" (walk list in memory). "direct" reserved for proclassic by-name endpoints and handled in a later phase.
	SearchParam  string `json:"searchParam,omitempty"`   // clientFilter mode only: server-side search query key to narrow results (e.g. "search"). Empty → fetch full list.
	ResultsField string `json:"resultsField,omitempty"`  // envelope key containing the array of list elements. Empty defaults to "results"; set to e.g. "benchmarks" for non-standard wrappers.
	TypedReturn  string `json:"typedReturn,omitempty"`   // Go type returned by the typed wrapper (e.g. "BlueprintOverview"). Defaults to ResourceType when empty.
	ExtraParams  string `json:"extraParams,omitempty"`   // filtered mode only: additional query params appended to the list path before the filter (e.g. "section=GENERAL" for endpoints that require a section param to populate filterable fields).
	ByField      string `json:"byField,omitempty"`       // override the "ByName" suffix in method names (e.g. "BySerialNumber" emits ResolveDeviceIDBySerialNumber). Empty defaults to "ByName".
	Apply        *ApplyConfig `json:"apply,omitempty"`   // when set, generates an Apply<ResourceType> upsert method that resolves by name, then creates or updates
}

// ApplyConfig declares an upsert method the generator should emit alongside
// a resolver. Apply<ResourceType>(ctx, request) resolves the name, creates
// if not found (404), or updates if found.
type ApplyConfig struct {
	CreateOp    string `json:"createOp"`              // name of the Create operation (e.g. "CreateBuildingV1")
	UpdateOp    string `json:"updateOp"`              // name of the Update operation (e.g. "UpdateBuildingV1")
	DeleteOp    string `json:"deleteOp,omitempty"`    // name of the Delete operation (for test generation)
	NameGoField string `json:"nameGoField"`           // Go struct field path to extract the name (e.g. "Name", "DisplayName")
	UpdateType  string `json:"updateType,omitempty"`  // Go type for the update request when it differs from create (triggers JSON round-trip conversion)
	GetOp       string `json:"getOp,omitempty"`       // GET operation name for fetching current resource (required when versionLock is true)
	VersionLock bool   `json:"versionLock,omitempty"` // when true, zeros VersionLock on create and fetches+injects current VersionLock on update

	// Token-upload mode: for resources created via token upload (e.g. DEP).
	// The Apply method takes (ctx, request, token) where token is optional on update.
	// Create path: upload token → then update metadata. Update path: optionally re-upload token, then update metadata.
	TokenUploadMode     bool   `json:"tokenUploadMode,omitempty"`     // enables token-upload apply mode
	TokenUploadCreateOp string `json:"tokenUploadCreateOp,omitempty"` // op that uploads token to create the resource (e.g. "UploadDeviceEnrollmentTokenV1")
	TokenReplaceOp      string `json:"tokenReplaceOp,omitempty"`      // op that re-uploads token to an existing resource (e.g. "ReplaceDeviceEnrollmentTokenV1")

	// MembershipPreFetch mode: for resources whose PATCH requires the current
	// member list to be re-specified (e.g. static mobile device groups). On
	// update, the Apply method fetches current membership via a list op, maps
	// each member's ID into an Assignment-like struct with Selected=true, and
	// injects the result into the request before calling the patch op.
	MembershipPreFetch *MembershipPreFetchConfig `json:"membershipPreFetch,omitempty"`
}

// MembershipPreFetchConfig controls the membership pre-fetch step in Apply.
type MembershipPreFetchConfig struct {
	FetchOp              string `json:"fetchOp"`                        // list op to call for current membership (e.g. "ListStaticMobileDeviceGroupMembershipV1")
	SourceIDField        string `json:"sourceIdField"`                  // field on each result item (e.g. "MobileDeviceID")
	AssignmentType       string `json:"assignmentType"`                 // Go type for assignments (e.g. "Assignment")
	AssignmentIDField    string `json:"assignmentIdField"`              // ID field on assignment type (e.g. "MobileDeviceID")
	RequestField         string `json:"requestField"`                   // field on request to inject into (e.g. "Assignments")
	AssignmentFieldIsPtr bool   `json:"assignmentFieldIsSlicePtr,omitempty"` // true when request field is *[]T rather than []T
}

// parseOp splits "GET /v1/devices/{id}" into method and path.
func (o OperationDef) parseOp() (method, path string) {
	parts := strings.SplitN(o.Op, " ", 2)
	return strings.ToUpper(parts[0]), parts[1]
}

// parseParams expands compact param notation into ExtraParam structs.
//
//	"sort"                → {Spec:"sort", Go:"sort", Type:"string"}
//	"sort:[]string"       → {Spec:"sort", Go:"sort", Type:"[]string"}
//	"rule-id:string:ruleID" → {Spec:"rule-id", Go:"ruleID", Type:"string"}
func (o OperationDef) parseParams() []ExtraParam {
	params := make([]ExtraParam, 0, len(o.Params))
	for _, p := range o.Params {
		parts := strings.Split(p, ":")
		ep := ExtraParam{Spec: parts[0], Go: toLowerCamelCase(parts[0]), Type: "string"}
		if len(parts) >= 2 {
			ep.Type = parts[1]
		}
		if len(parts) >= 3 {
			ep.Go = parts[2]
		}
		params = append(params, ep)
	}
	return params
}

type ExtraParam struct {
	Spec string
	Go   string
	Type string
}

// validateConfig rejects misconfigured specs before generation runs.
// Currently enforces that no operation in Operations appears in ExcludePaths —
// the deny list is meant to catch accidental re-adds, so a conflict means
// either the entry should be removed from one side or the other.
func validateConfig(cfg Config) error {
	for _, spec := range cfg.Specs {
		excluded := make(map[string]bool, len(spec.ExcludePaths))
		for _, p := range spec.ExcludePaths {
			norm := normalizeOpKey(p)
			if excluded[norm] {
				return fmt.Errorf("spec %q: duplicate entry in excludePaths: %q", spec.File, p)
			}
			excluded[norm] = true
		}
		for _, op := range spec.Operations {
			if excluded[normalizeOpKey(op.Op)] {
				return fmt.Errorf("spec %q: operation %q is listed in both operations and excludePaths", spec.File, op.Op)
			}
		}
	}
	return nil
}

// normalizeOpKey canonicalises "METHOD /path" for comparison — uppercase
// method, single space, trimmed.
func normalizeOpKey(s string) string {
	parts := strings.SplitN(strings.TrimSpace(s), " ", 2)
	if len(parts) != 2 {
		return strings.ToUpper(strings.TrimSpace(s))
	}
	return strings.ToUpper(parts[0]) + " " + strings.TrimSpace(parts[1])
}
