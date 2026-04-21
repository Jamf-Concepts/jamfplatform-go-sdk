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
	Category         string // get, create, update, action, actionWithResponse, paginated, unwrap, multipart, resolverID, resolverTyped, resolverIDDirect, resolverTypedDirect
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
}

// GoResolver carries the config needed by resolver method templates.
// Populated on synthetic methods produced by appendResolverMethods; never
// present on spec-derived methods. Namespace/Version/ResourcePath on the
// parent GoMethod are inherited from the source op — the List op for
// filtered/clientFilter, the GetByName op for direct.
type GoResolver struct {
	ResourceType string // drives emitted method name suffix
	Mode         string // "filtered", "clientFilter", or "direct"
	NameField    string // filtered/clientFilter only
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
