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
	Name            string
	Comment         string
	Category        string // get, create, update, action, actionWithResponse, paginated, unwrap, multipart
	HTTPMethod      string
	Namespace       string
	Version         string
	Tag             string // first OpenAPI tag of the operation, used when SplitByTag is enabled
	ResourcePath    string // path after version prefix, e.g. "/devices/{id}"
	MultipartFields []GoMultipartField
	PathParams      []GoPathParam
	QueryParams     []ExtraParam
	RequestType     string
	ResponseType    string
	ExpectedStatus  int
	ContentType     string
	PaginationStyle string
	PageSizeParam   string
	ItemType        string
	ResultsField    string
	ReturnsSlice    bool
	SpecPath        string
	UnwrapResults   string
	Format          string // carried from SpecDef so per-method templates can branch without $-scope
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
