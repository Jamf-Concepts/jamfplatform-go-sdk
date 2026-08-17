// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
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
	SpecFile           string                       `json:"specFile,omitempty"`     // override published spec filename
	Package            string                       `json:"package,omitempty"`      // target Go sub-package under jamfplatform/; empty emits to root (legacy)
	SplitByTag         bool                         `json:"splitByTag,omitempty"`   // emit one methods file per OpenAPI tag instead of one per spec
	Format             string                       `json:"format,omitempty"`       // "json" (default) or "xml" — drives struct tag style and transport codec
	RawBody            bool                         `json:"rawBody,omitempty"`      // generate methods that take/return []byte instead of typed structs; consumer owns marshaling (used for Classic where spec has no useful types)
	TypesOnly          bool                         `json:"typesOnly,omitempty"`    // generate only Go types from all schemas — no client methods, no method tests, no client.go; used for specs that define types consumed in other API payloads (e.g. blueprint component configurations)
	Undocumented       bool                         `json:"undocumented,omitempty"` // all operations in this spec are unofficial/reverse-engineered; emits an "Unofficial:" godoc warning on every generated method
	Operations         []OperationDef               `json:"operations"`
	ExcludePaths       []string                     `json:"excludePaths,omitempty"`       // "METHOD /path" entries the generator must refuse to include
	FieldTypeOverrides map[string]string            `json:"fieldTypeOverrides,omitempty"` // "schema_name.property_name" -> Go type, used to correct spec bugs (e.g. `integer` fields where the server actually returns a non-int64 string). Applied per-spec so upstream spec updates don't get silently overwritten.
	SchemaAdditions    map[string]map[string]string `json:"schemaAdditions,omitempty"`    // "schema_name" -> { "property_name": "openapi_type" }, inject missing properties into a spec schema. Used when the spec omits a field the server accepts but we need to send (e.g. Classic's account schema has no `password` property). openapi_type is one of "string", "integer", "boolean", "string:password" (writeOnly string).

	// SchemaRenames renames schema keys in doc.Components.Schemas and updates
	// all $ref strings that reference the old name. Applied before all other
	// patches so downstream fixups see the corrected names. Use when a spec
	// uses generic schema names that would collide with schemas emitted by
	// other specs in the same Go package (e.g. "SelfServiceSettings" is a
	// top-level Pro API type; an app-installer spec reusing that name would
	// silently shadow it). Outer key: old schema name. Value: new schema name.
	SchemaRenames map[string]string `json:"schemaRenames,omitempty"`

	// SchemaCreations adds whole named component schemas the spec no longer
	// declares but the server still returns. Key: schema name. Value: raw JSON
	// for an OpenAPI 3 Schema object, which may `$ref` other created schemas.
	// Applied before every other pass, so a created schema is indistinguishable
	// from a spec-declared one downstream — SchemaPatches can `$ref` it, and it
	// is emitted as a Go type once a whitelisted operation reaches it.
	//
	// Only for a schema upstream *dropped*: panics if the name already exists,
	// which is the signal that the spec has been repaired and the entry (and
	// the patches pointing at it) should be deleted.
	SchemaCreations map[string]json.RawMessage `json:"schemaCreations,omitempty"`

	// SchemaPatches injects (or replaces) arbitrary OpenAPI schema fragments at
	// dotted property paths under a named component schema. Used when a spec
	// omits a richer sub-structure the server actually returns (e.g. policy
	// missing top-level `reboot`, or scope missing `jss_users`/`jss_user_groups`).
	// Outer key: schema name (e.g. "policy"). Inner key: dotted path of
	// properties to walk into, with the last segment being the property whose
	// schema we set (e.g. "self_service.notification" or
	// "general.date_time_limitations.no_execute_on.day"). Inner value: raw
	// JSON for an OpenAPI 3 Schema object. If every walked intermediate
	// segment is an `object` with `properties`, the patch slots in cleanly.
	// Existing properties at the final segment are replaced; missing ones are
	// added. Applied after SchemaAdditions and before PostSymmetry, so a patch
	// to the read schema also flows into the *_post sibling.
	SchemaPatches map[string]map[string]json.RawMessage `json:"schemaPatches,omitempty"`

	// PropertyRenames renames property keys at dotted paths under a named
	// component schema. Outer key: schema name. Inner key: dotted path to the
	// property to rename (e.g. "self_service.re-install_button_text"). Inner
	// value: new key name (e.g. "reinstall_button_text"). Used to repair spec
	// keys that don't match the actual wire (silent decode failures otherwise).
	// Renames the property *key* in the schema map; downstream Go field naming
	// follows from the corrected key. Applied before PostSymmetry so the
	// post sibling sees the corrected name.
	PropertyRenames map[string]map[string]string `json:"propertyRenames,omitempty"`

	// PropertyRemovals deletes properties at dotted paths under named component
	// schemas. Outer key: schema name. Value: list of dotted paths to delete
	// (same path syntax as SchemaPatches / PropertyRenames). Applied after
	// PropertyRenames and before PostSymmetry so the post sibling inherits the
	// trimmed shape. Panics on missing paths — a declared removal that can't be
	// walked is a config typo.
	//
	// Targets inline object schemas only. If a path resolves through a shared
	// $ref component, the deletion affects every schema that references it.
	PropertyRemovals map[string][]string `json:"propertyRemovals,omitempty"`

	// PostSymmetryExcludes lists, per *_post schema, property names whose
	// presence in the read sibling should NOT be copied across when the
	// generator's Post-symmetry pass mirrors read fields onto a post type.
	// Default is full symmetry — only add an entry here when the server
	// genuinely cannot accept a field on write (e.g. server-assigned audit
	// timestamps). Outer key: post schema name (e.g. "policy_post"). Value:
	// list of property names (top-level on the schema) to skip.
	PostSymmetryExcludes map[string][]string `json:"postSymmetryExcludes,omitempty"`

	// EmitNullForOptional lists schema names whose optional pointer fields
	// must marshal as explicit JSON null when nil, rather than being omitted.
	// Required for endpoints whose server distinguishes "field omitted" (keep
	// existing tenant value) from "field present with null" (clear / reset).
	//
	// Example: Pro v3 PUT /sso. A SAML metadata_source transition from "URL"
	// to "FILE" succeeds only when the request body sends "idpUrl": null and
	// "metadataFileName": null alongside the new "metadataSource": "FILE". If
	// those keys are omitted (the default behaviour produced by ",omitempty"
	// on a nil *string), the server merges cached URL-mode values against the
	// new FILE-mode request and rejects with 400 "SAML settings validation
	// failed". Listing SamlSettings / OidcSettings / EnrollmentSsoConfig here
	// drops ",omitempty" from their pointer field JSON tags so consumers can
	// emit explicit nulls.
	//
	// Schema names may be supplied in either spec form (snake_case) or Go
	// form (PascalCase); the generator normalises via toSnakeCase before
	// matching. Affects JSON marshalling only — has no impact on unmarshal
	// behaviour or XML tags.
	EmitNullForOptional []string `json:"emitNullForOptional,omitempty"`

	// FieldOrder specifies an explicit property emission order for named
	// schemas, overriding the default alphabetical sort. Outer key: spec-form
	// schema name (e.g. "vpp_invitation_general"). Value: ordered list of
	// property names as they appear in the spec (without deprecation metadata).
	// Properties not listed are appended alphabetically after the explicit
	// entries. Used when the server is order-sensitive within an XML element
	// (e.g. Classic /vppinvitations <general> requires a fixed field order).
	FieldOrder map[string][]string `json:"fieldOrder,omitempty"`

	// Version supplies the URL version segment for every operation in this
	// spec whose own path carries none. Precedence is operation override >
	// version prefix on the spec path > this default, so a spec mixing
	// versioned and unversioned paths still resolves each one correctly.
	//
	// Needed because the published Security Cloud specs (dns, ztna,
	// categories) carry tenant-scoped paths with the version segment
	// dropped — `/tenant/{tenantId}/dns/zones`, not
	// `/v1/tenant/{tenantId}/dns/zones`. The gateway does not route the
	// versionless form (wire-probed: 403 BAD_PERMISSIONS), so the version has
	// to come from somewhere, and per-operation entries would mean 29 copies
	// of the same "v1" across three specs.
	Version string `json:"version,omitempty"`

	// TagRenames remaps an OpenAPI tag before it picks the output filename,
	// and nothing else — method names, godoc and the published spec are
	// untouched. Needed when two specs in one package share a tag, since
	// splitByTag writes one <tag>.go per spec and emitMethodsByTag errors on
	// the collision rather than letting the second spec overwrite the first's
	// file.
	//
	// Live use: Security Cloud's uem-connect spec tags one operation
	// "activation-profiles", which is also the tag of every operation in the
	// enrollment spec. That spec is not ingested today (it is absent from the
	// GitOps build), so the rename is what keeps uem-connect's operation in
	// uem_connect_activation_profiles.go and leaves activation_profiles.go
	// free for the enrollment spec if it gets published.
	TagRenames map[string]string `json:"tagRenames,omitempty"`
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
	MaxPageSize    int               `json:"maxPageSize,omitempty"` // page-size requested per page; defaults to 100. Only raise this once the endpoint's true server-side cap is wire-verified — see CLAUDE.md "Wire-verified pagination limits".
	Version        string            `json:"version,omitempty"`     // override version for tenantPrefix
	PathNames      map[string]string `json:"pathNames,omitempty"`   // spec param -> Go param name
	Params         []string          `json:"params,omitempty"`      // "name", "name:type", "spec:type:goName"
	UnwrapResults  string            `json:"unwrapResults,omitempty"`
	RequestType    string            `json:"requestType,omitempty"`    // explicit request schema name (used when spec body is untyped, e.g. Classic)
	ResponseType   string            `json:"responseType,omitempty"`   // explicit response schema name (same)
	ExpectedStatus int               `json:"expectedStatus,omitempty"` // explicit success status code (default 200)
	Resolver       *ResolverConfig   `json:"resolver,omitempty"`       // attach name->ID resolver emission to this operation (typically a List op)
	Resolvers      []ResolverConfig  `json:"resolvers,omitempty"`      // attach multiple resolvers to one operation (e.g. resolve device by name AND by serialNumber)

	// NoRetry opts this operation out of the transport's automatic 5xx retry
	// (internal/client/retry.go's isRetryableWriteStatus), even though its
	// HTTP method would otherwise qualify. Set this ONLY when the endpoint
	// requires a side-channel precondition in its request body — an
	// optimistic-lock field sourced from a GET taken before the write — that
	// a blind byte-for-byte retry would replay stale, turning a
	// successful-but-500ing write into a masked conflict on the retried
	// attempt (see computer/mobile-device prestage enrollment for the
	// confirmed live case). This is a small, enumerable exception list, not
	// a general escape hatch: most PUT endpoints have no such precondition
	// and should keep the default auto-retry.
	NoRetry bool `json:"noRetry,omitempty"`
}

// ResolverConfig declares a name->ID resolver the generator should emit
// alongside the operation it attaches to. Produces two methods per resource:
// Resolve<ResourceType>IDByName (returns string ID) and
// Resolve<ResourceType>ByName (returns the typed resource).
type ResolverConfig struct {
	ResourceType string       `json:"resourceType"`           // Go type name used in emitted method names (e.g. "Blueprint")
	NameField    string       `json:"nameField"`              // dot-notation JSON path for the name field on each list element (e.g. "name", "general.name", "title")
	MatchField   string       `json:"matchField,omitempty"`   // optional: dot-notation JSON path for client-side match verification when it differs from nameField (e.g. RSQL uses "displayName" but response nests it at "general.displayName"). Empty defaults to nameField.
	IDField      string       `json:"idField"`                // dot-notation JSON path for the ID field (e.g. "id")
	IDNumeric    bool         `json:"idNumeric,omitempty"`    // when true, the ID field is a number in JSON (int in Go); test stubs emit numeric IDs
	IDPointer    bool         `json:"idPointer,omitempty"`    // when true, the ID field is a pointer (*int) in Go; overrides IDNumeric's strconv.Itoa to use fmt.Sprintf with dereference
	Mode         string       `json:"mode"`                   // "filtered" (server-side RSQL) or "clientFilter" (walk list in memory). "direct" reserved for proclassic by-name endpoints and handled in a later phase.
	SearchParam  string       `json:"searchParam,omitempty"`  // clientFilter mode only: server-side search query key to narrow results (e.g. "search"). Empty → fetch full list.
	ResultsField string       `json:"resultsField,omitempty"` // envelope key containing the array of list elements. Empty defaults to "results"; set to e.g. "benchmarks" for non-standard wrappers.
	TypedReturn  string       `json:"typedReturn,omitempty"`  // Go type returned by the typed wrapper (e.g. "BlueprintOverview"). Defaults to ResourceType when empty.
	ExtraParams  string       `json:"extraParams,omitempty"`  // filtered mode only: additional query params appended to the list path before the filter (e.g. "section=GENERAL" for endpoints that require a section param to populate filterable fields).
	ByField      string       `json:"byField,omitempty"`      // override the "ByName" suffix in method names (e.g. "BySerialNumber" emits ResolveDeviceIDBySerialNumber). Empty defaults to "ByName".
	Apply        *ApplyConfig `json:"apply,omitempty"`        // when set, generates an Apply<ResourceType> upsert method that resolves by name, then creates or updates
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
	FetchOp              string `json:"fetchOp"`                             // list op to call for current membership (e.g. "ListStaticMobileDeviceGroupMembershipV1")
	SourceIDField        string `json:"sourceIdField"`                       // field on each result item (e.g. "MobileDeviceID")
	AssignmentType       string `json:"assignmentType"`                      // Go type for assignments (e.g. "Assignment")
	AssignmentIDField    string `json:"assignmentIdField"`                   // ID field on assignment type (e.g. "MobileDeviceID")
	RequestField         string `json:"requestField"`                        // field on request to inject into (e.g. "Assignments")
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
		if spec.TypesOnly && len(spec.Operations) > 0 {
			return fmt.Errorf("spec %q: typesOnly specs must not declare operations", spec.File)
		}
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
