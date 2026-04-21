// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

// ---------------------------------------------------------------------------
// Intermediate representation
// ---------------------------------------------------------------------------

type GoType struct {
	Name          string
	Comment       string
	Fields        []GoField
	IsRawJSON     bool
	Discriminator *GoDiscriminator
	XMLName       string // wire element name when format=xml and it differs from Go type name; emitted as XMLName xml.Name `xml:"..."` field
	AliasTarget   string // non-empty → emit as `type Name = AliasTarget` (used for top-level array schemas)
	IsListWrapper bool   // true when this is a Classic list wrapper (flattens {size, resource} array items into sibling fields). Excludes the type from heuristics that inject top-level id or carry id as a resource signal.
}

// GoDiscriminator describes a oneOf-with-discriminator polymorphic schema.
// The Go representation is a single struct carrying the discriminator value
// plus one pointer per variant; a generated UnmarshalJSON dispatches on
// the discriminator and populates the matching variant field.
type GoDiscriminator struct {
	PropertyName string // JSON property name of the discriminator field (e.g. "deviceType")
	GoFieldName  string // Go exported name for the discriminator field (e.g. "DeviceType")
	Variants     []GoDiscriminatorVariant
}

type GoDiscriminatorVariant struct {
	Value     string // discriminator value as seen on the wire (e.g. "iOS")
	TypeName  string // Go type name (e.g. "MobileDeviceIosInventory")
	FieldName string // exported Go field name in the union struct (e.g. "IOS")
}

type GoField struct {
	Name    string
	Type    string
	JSONTag string
	Comment string // godoc line emitted immediately above the field, if non-empty
}

type GoMethod struct {
	Name             string
	Comment          string
	Category         string // get, create, update, action, actionWithResponse, paginated, unwrap, multipart, resolverID, resolverTyped, resolverIDDirect, resolverTypedDirect, apply
	HTTPMethod       string
	Namespace        string
	Version          string
	Tag              string // first OpenAPI tag of the operation, used when SplitByTag is enabled
	ResourcePath     string // path after version prefix, e.g. "/devices/{id}"
	MultipartFields  []GoMultipartField
	PathParams       []GoPathParam
	QueryParams      []ExtraParam
	RequestType      string
	ResponseType     string
	ResponseWireName string // XML element name of the response root (format=xml only); used by test stubs to emit valid wire bodies
	ExpectedStatus   int
	ContentType      string
	PaginationStyle  string
	PageSizeParam    string
	ItemType         string
	ResultsField     string
	ReturnsSlice     bool
	SpecPath         string
	UnwrapResults    string
	Format           string      // carried from SpecDef so per-method templates can branch without $-scope
	Resolver         *GoResolver // populated on synthetic resolver methods (Category resolverID/resolverTyped)
	Apply            *GoApply    // populated on synthetic apply (upsert) methods
}

// GoResolver carries the config needed by resolver method templates.
// Populated on synthetic methods produced by appendResolverMethods; never
// present on spec-derived methods. Namespace/Version/ResourcePath on the
// parent GoMethod are inherited from the source op — the List op for
// filtered/clientFilter, the GetByName op for direct.
type GoResolver struct {
	ResourceType string // drives emitted method name suffix
	Mode         string // "filtered", "clientFilter", or "direct"
	NameField    string // filtered/clientFilter only — used in RSQL filter expression
	MatchField   string // client-side match verification path; equals NameField when the RSQL field matches the JSON response path
	IDField      string // filtered/clientFilter only
	IDNumeric    bool   // when true, test stubs emit numeric ID values (42 instead of "resolved-id")
	SearchParam  string // clientFilter only
	ResultsField string // envelope key for the element array; empty → transport defaults to "results"
	TypedReturn  string // Go type of the typed wrapper's return
	ExtraParams  string // filtered mode only: appended to list path before filter (e.g. "section=GENERAL")
	Paginated    bool   // clientFilter only: source list op is paginated — use paged transport walk
	ByField      string // suffix override: "BySerialNumber" → ResolveDeviceIDBySerialNumber. Empty → "ByName"
	SourceMethod string // direct only: existing Get<X>ByName method the wrappers delegate to
	// IDNilCheck and IDDeref are pre-computed expressions the direct-mode
	// template emits verbatim. They cover the nested-ID case: Classic
	// responses for composite resources (policies, mac_application,
	// ebook, …) populate only `<general><id>N</id></general>` on the wire,
	// even when the Go struct also has a top-level ID *int. The config's
	// idField path ("ID" or "General.ID") drives what Go field chain we
	// walk; the generator expands the chain with nil guards per step so
	// callers see "response missing id" rather than a nil-deref panic.
	IDNilCheck string
	IDDeref    string
	// IDTestInnerXML is the XML body fragment the direct-mode test stub
	// emits inside the response's wire-root element. Flat path ("ID")
	// produces "<id>42</id>"; nested path ("General.ID") produces
	// "<general><id>42</id></general>" so the typed decoder populates
	// r.General.ID and the resolver's walk succeeds.
	IDTestInnerXML string
}

// GoApply carries the config needed by the apply (upsert) method template.
// Populated on synthetic methods produced by appendApplyMethods; never
// present on spec-derived methods.
type GoApply struct {
	ResourceType     string // "BuildingV1"
	RequestType      string // Go type for the request parameter (e.g. "Building")
	NameGoField      string // Go struct field path to extract name (e.g. "Name", "DisplayName")
	NameParentField  string // non-empty for nested paths: "General" when NameGoField is "General.Name"
	NameParentType   string // Go type for the parent struct: "PolicyGeneral" when NameGoField is "General.Name"
	NameLeafField    string // leaf field name for nested paths: "Name" when NameGoField is "General.Name"; equals NameGoField when flat
	ResolverMethod   string // "ResolveBuildingV1IDByName"
	CreateMethod     string // "CreateBuildingV1"
	UpdateMethod     string // "UpdateBuildingV1"
	DeleteMethod     string // "DeleteBuildingV1" (for test generation only)
	CreateReturnID   string // expression to extract ID from create response: "resp.ID" for HrefResponse, "strconv.Itoa(resp.ID)" for int, "fmt.Sprintf(\"%d\", *resp.ID)" for *int
	IDNumeric        bool   // true when the create response ID is int (test mock should use numeric JSON)
	UpdateReturnsVal bool   // true when Update returns (*T, error), false for error-only
	ExtraArgs         string // additional method signature args, e.g. ", platform bool"
	ExtraCallArgs     string // additional create call args, e.g. ", platform"
	ExtraTestCallArgs string // additional create call args with literal zero values for tests, e.g. ", false"
	ClassicCreate    bool   // true for classic API: Create takes (ctx, "0", request) instead of (ctx, request)
	NameIsPointer    bool   // true when the name field is a pointer (Classic XML types)
	NameNested       bool   // true when the name is inside a nested struct (e.g. General.Name)

	// UpdateType support — when the update operation takes a different Go type than create.
	UpdateType        string // Go type for the update request (empty = same as RequestType)
	HasUpdateType     bool   // true when UpdateType is set (different create/update types)

	// Optimistic locking (versionLock) — for prestages.
	VersionLock bool   // true when create must zero VersionLock fields and update must GET→inject them
	GetMethod   string // GET operation name (e.g. "GetComputerPrestageV3") — required when VersionLock is true
	GetNS       string // namespace for get
	GetVer      string // version for get
	GetPath     string // resource path for get endpoint
	GetType     string // Go type for the GET response (e.g. "GetComputerPrestageV3")
	SameGetUpdatePath bool // true when GET and Update share the same URL (need combined handler in tests)

	// Token-upload mode — for resources created via token upload (e.g. DEP instances).
	TokenUploadMode       bool   // true when Apply uses upload-token create + optional token replace on update
	TokenUploadMethod     string // method that uploads token to create the resource
	TokenReplaceMethod    string // method that re-uploads token to an existing resource
	TokenRequestType      string // Go type for the token request (e.g. "DeviceEnrollmentToken")
	TokenUploadNS         string // namespace for upload
	TokenUploadVer        string // version for upload
	TokenUploadPath       string // path for upload endpoint
	TokenReplaceNS        string // namespace for replace
	TokenReplaceVer       string // version for replace
	TokenReplacePath      string // path for replace endpoint

	// Membership pre-fetch mode — fetch current members before patch.
	MembershipPreFetch          bool   // true when Apply must fetch membership before patch
	MembershipFetchMethod       string // "ListStaticMobileDeviceGroupMembershipV1"
	MembershipFetchExtraArgs    string // ", nil, \"\"" — zero-value extra params for the fetch call
	MembershipFetchNS           string // "pro"
	MembershipFetchVer          string // "v1"
	MembershipFetchPath         string // "/mobile-device-groups/static-group-membership/{id}"
	MembershipSourceIDField     string // field on each result item (e.g. "MobileDeviceID")
	MembershipAssignmentType    string // Go type for each assignment item (e.g. "Assignment")
	MembershipAssignmentIDField string // ID field on assignment type (e.g. "MobileDeviceID")
	MembershipRequestField      string // field on request to inject into (e.g. "Assignments")
	MembershipRequestFieldIsPtr bool   // true when request.Assignments is *[]T

	// Test generation paths (pre-computed from the source ops).
	ListNamespace string // namespace for the list/resolver call
	ListVersion   string // version for the list/resolver call
	ListPath      string // resource path for the list endpoint
	ListNameField string // JSON name field for resolver response stubs
	ListIDField   string // JSON id field for resolver response stubs
	CreateNS      string // namespace for create
	CreateVer     string // version for create
	CreatePath    string // resource path for create endpoint (e.g. "/buildings")
	CreateStatus  int    // expected HTTP status for create response
	UpdateNS      string // namespace for update
	UpdateVer     string // version for update
	UpdatePath    string // resource path for update endpoint (e.g. "/buildings/{id}")
	UpdateStatus  int    // expected HTTP status for update response
	SameListCreatePath bool // true when list and create share the same URL (need combined handler in tests)

	// Classic (XML) test generation fields — only set when ClassicCreate is true.
	ClassicResolverWireName    string // XML root element for the resolver's GetByName response (e.g. "computer_extension_attribute")
	ClassicResolverIDInnerXML  string // inner XML for ID in resolver response (e.g. "<id>42</id>" or "<general><id>42</id></general>")
	ClassicCreateWireName      string // XML root element for the create response (e.g. "computer_extension_attribute")
}

type GoPathParam struct {
	SpecName string
	GoName   string
}

// GoMultipartField describes one part of a multipart/form-data request body.
// Binary fields (format: binary) emit two Go parameters: a filename string
// and an io.Reader content. Non-binary fields emit one typed parameter.
type GoMultipartField struct {
	Name   string // spec field name ("file")
	GoName string // Go param identifier (camelCase)
	IsFile bool
	Type   string // Go type for non-file fields
}

type GeneratedFile struct {
	Package string
	Module  string
	Format  string // "json" (default) or "xml" — drives struct tag style and transport codec
	Types   []GoType
	Methods []GoMethod
}
