// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Command generate reads OpenAPI spec files and produces typed Go SDK methods
// and unit tests that call into the internal/client transport layer.
//
// Usage:
//
//	go run ./tools/generate [flags]
//	  -config  path to config.json  (default: tools/generate/config.json)
//	  -root    repo root directory   (default: auto-detected from git)
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/imports"
)

// ---------------------------------------------------------------------------
// Configuration types (loaded from config.json)
// ---------------------------------------------------------------------------

type Config struct {
	Package string    `json:"package"`
	Module  string    `json:"module"`
	Specs   []SpecDef `json:"specs"`
}

type SpecDef struct {
	File        string         `json:"file"`
	Namespace   string         `json:"namespace"`
	Version     string         `json:"version"`
	OutputFile  string         `json:"outputFile"`
	TestFile    string         `json:"testFile"`
	SkipSchemas []string       `json:"skipSchemas"`
	Operations  []OperationDef `json:"operations"`
}

type OperationDef struct {
	Path            string            `json:"path"`
	Method          string            `json:"method"`
	GoName          string            `json:"goName"`
	ContentType     string            `json:"contentType,omitempty"`
	Pagination      string            `json:"pagination,omitempty"`
	PageSizeParam   string            `json:"pageSizeParam,omitempty"`
	VersionOverride string            `json:"versionOverride,omitempty"`
	PathParamNames  map[string]string `json:"pathParamNames,omitempty"`
	ExtraParams     []ExtraParam      `json:"extraParams,omitempty"`
	UnwrapResults   string            `json:"unwrapResults,omitempty"`
	Skip            bool              `json:"skip,omitempty"`
}

type ExtraParam struct {
	Spec string `json:"spec"`
	Go   string `json:"go"`
	Type string `json:"type"`
}

// ---------------------------------------------------------------------------
// Intermediate representation
// ---------------------------------------------------------------------------

type GoType struct {
	Name      string
	Comment   string
	Fields    []GoField
	IsRawJSON bool
}

type GoField struct {
	Name    string
	Type    string
	JSONTag string
}

type GoMethod struct {
	Name         string
	Comment      string
	Category     string // get, create, update, action, actionWithResponse, paginated, unwrap
	HTTPMethod   string
	Namespace    string
	Version      string
	ResourcePath string // path after version prefix, e.g. "/devices/{id}"
	PathParams   []GoPathParam
	QueryParams  []ExtraParam
	RequestType  string
	ResponseType string
	ExpectedStatus  int
	ContentType     string
	PaginationStyle string
	PageSizeParam   string
	ItemType        string
	ResultsField    string
	ReturnsSlice    bool
	SpecPath        string
	UnwrapResults   string
}

type GoPathParam struct {
	SpecName string
	GoName   string
}

type GeneratedFile struct {
	Package string
	Module  string
	Types   []GoType
	Methods []GoMethod
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	configPath := flag.String("config", "", "path to config.json")
	rootDir := flag.String("root", "", "repo root directory")
	flag.Parse()

	if *rootDir == "" {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			log.Fatal("cannot detect repo root: ", err)
		}
		*rootDir = strings.TrimSpace(string(out))
	}
	if *configPath == "" {
		*configPath = filepath.Join(*rootDir, "tools", "generate", "config.json")
	}

	data, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("reading config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("parsing config: %v", err)
	}

	for _, spec := range cfg.Specs {
		if err := processSpec(*rootDir, cfg, spec); err != nil {
			log.Fatalf("spec %s: %v", spec.File, err)
		}
	}
	if err := writeStaticFiles(*rootDir, cfg); err != nil {
		log.Fatalf("static files: %v", err)
	}
	log.Println("generation complete")
}

// ---------------------------------------------------------------------------
// Per-spec processing
// ---------------------------------------------------------------------------

func processSpec(root string, cfg Config, spec SpecDef) error {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(root, spec.File))
	if err != nil {
		return fmt.Errorf("loading spec: %w", err)
	}

	skipSet := make(map[string]bool, len(spec.SkipSchemas))
	for _, s := range spec.SkipSchemas {
		skipSet[s] = true
	}

	gf := GeneratedFile{
		Package: cfg.Package,
		Module:  cfg.Module,
		Types:   extractTypes(doc, skipSet),
	}
	methods, err := extractMethods(doc, spec)
	if err != nil {
		return err
	}
	gf.Methods = methods

	for _, pair := range []struct {
		tmpl *template.Template
		out  string
	}{
		{sourceTmpl, spec.OutputFile},
		{testTmpl, spec.TestFile},
	} {
		var buf bytes.Buffer
		if err := pair.tmpl.Execute(&buf, gf); err != nil {
			return fmt.Errorf("executing template for %s: %w", pair.out, err)
		}
		formatted, err := imports.Process(pair.out, buf.Bytes(), &imports.Options{Comments: true})
		if err != nil {
			return fmt.Errorf("goimports %s: %w\n---raw---\n%s", pair.out, err, buf.String())
		}
		if err := os.WriteFile(filepath.Join(root, pair.out), formatted, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", pair.out, err)
		}
		log.Printf("wrote %s", pair.out)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Schema → Go types
// ---------------------------------------------------------------------------

func extractTypes(doc *openapi3.T, skip map[string]bool) []GoType {
	names := sortedKeys(doc.Components.Schemas)
	var types []GoType

	for _, name := range names {
		if skip[name] {
			continue
		}
		schema := doc.Components.Schemas[name].Value
		if schema == nil || schema.Type == nil {
			continue
		}
		if len(schema.AllOf) > 0 {
			continue // pagination envelopes
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

		// Freeform object (no properties) → json.RawMessage
		if len(schema.Properties) == 0 && schema.AdditionalProperties.Schema == nil {
			comment := name + " represents a freeform JSON object."
			if schema.Description != "" {
				comment = name + " " + lowerFirst(cleanComment(schema.Description))
			}
			types = append(types, GoType{Name: name, Comment: comment, IsRawJSON: true})
			continue
		}

		types = append(types, schemaToGoType(name, schema))
	}
	return types
}

func schemaToGoType(name string, schema *openapi3.Schema) GoType {
	gt := GoType{
		Name:    name,
		Comment: fmt.Sprintf("%s represents a %s.", name, camelToWords(name)),
	}
	if schema.Description != "" {
		gt.Comment = name + " " + cleanComment(schema.Description)
	}

	required := toSet(schema.Required)
	for _, pname := range sortedKeys(schema.Properties) {
		propRef := schema.Properties[pname]
		prop := propRef.Value

		goType := schemaRefToGoType(propRef)
		jsonTag := pname

		isNullable := prop != nil && prop.Nullable
		isRequired := required[pname]

		// Pointer for: nullable, unrequired non-scalars, or $ref to object with properties.
		isStructRef := propRef.Ref != "" && prop != nil && prop.Type != nil &&
			prop.Type.Is("object") && len(prop.Properties) > 0
		needsPtr := isNullable || isStructRef || (!isRequired && !isScalar(goType))

		if needsPtr && !strings.HasPrefix(goType, "*") && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if isNullable && !strings.HasPrefix(goType, "*") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		}

		gt.Fields = append(gt.Fields, GoField{
			Name:    exportedGoName(pname),
			Type:    goType,
			JSONTag: jsonTag,
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

// ---------------------------------------------------------------------------
// Operations → Go methods
// ---------------------------------------------------------------------------

func extractMethods(doc *openapi3.T, spec SpecDef) ([]GoMethod, error) {
	var methods []GoMethod
	for _, opDef := range spec.Operations {
		if opDef.Skip {
			continue
		}
		m, err := buildMethod(doc, spec, opDef)
		if err != nil {
			return nil, fmt.Errorf("operation %s %s: %w", opDef.Method, opDef.Path, err)
		}
		methods = append(methods, m)
	}
	return methods, nil
}

func buildMethod(doc *openapi3.T, spec SpecDef, opDef OperationDef) (GoMethod, error) {
	pathItem := doc.Paths.Find(opDef.Path)
	if pathItem == nil {
		return GoMethod{}, fmt.Errorf("path %s not found in spec", opDef.Path)
	}
	op := pathItem.GetOperation(opDef.Method)
	if op == nil {
		return GoMethod{}, fmt.Errorf("%s %s: operation not found", opDef.Method, opDef.Path)
	}

	version := spec.Version
	if opDef.VersionOverride != "" {
		version = opDef.VersionOverride
	}

	m := GoMethod{
		Name:            opDef.GoName,
		HTTPMethod:      opDef.Method,
		Namespace:       spec.Namespace,
		Version:         version,
		ResourcePath:    stripVersionPrefix(opDef.Path),
		QueryParams:     opDef.ExtraParams,
		ContentType:     opDef.ContentType,
		PaginationStyle: opDef.Pagination,
		PageSizeParam:   cmp(opDef.PageSizeParam, "page-size"),
		ResultsField:    "results",
		SpecPath:        opDef.Path,
		UnwrapResults:   opDef.UnwrapResults,
	}

	if op.Summary != "" {
		m.Comment = opDef.GoName + " " + lowerFirst(cleanComment(op.Summary))
	}

	m.PathParams = extractPathParams(opDef.Path, opDef.PathParamNames)
	m.ExpectedStatus, m.ResponseType = detectResponse(op)

	// Request body
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for _, content := range op.RequestBody.Value.Content {
			if content.Schema != nil {
				m.RequestType = refName(content.Schema)
				break
			}
		}
	}

	// Paginated item type
	if m.PaginationStyle != "" {
		m.ItemType = detectPaginatedItemType(op)
		m.ResponseType = ""
	}

	m.ReturnsSlice = strings.HasPrefix(m.ResponseType, "[]")

	// Determine category
	m.Category = categorize(m)

	return m, nil
}

func categorize(m GoMethod) string {
	if m.UnwrapResults != "" {
		return "unwrap"
	}
	if m.PaginationStyle != "" {
		return "paginated"
	}
	hasReq := m.RequestType != ""
	hasResp := m.ResponseType != ""
	isOK := m.ExpectedStatus == 200

	switch {
	case isOK && hasResp:
		return "get"
	case !isOK && hasResp && hasReq:
		return "create"
	case !isOK && hasResp && !hasReq:
		return "actionWithResponse"
	case !isOK && !hasResp && hasReq:
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
		goName := specName
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

func detectResponse(op *openapi3.Operation) (int, string) {
	for _, code := range []int{200, 201, 202, 204} {
		resp := op.Responses.Status(code)
		if resp == nil {
			continue
		}
		if resp.Value == nil {
			return code, ""
		}
		for _, content := range resp.Value.Content {
			if content.Schema != nil {
				return code, refName(content.Schema)
			}
		}
		return code, ""
	}
	return 200, ""
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
	}
	return "any"
}

// ---------------------------------------------------------------------------
// Template rendering
// ---------------------------------------------------------------------------

// formatGo runs goimports which handles both formatting and unused import removal.
func formatGo(filename string, src []byte) ([]byte, error) {
	return imports.Process(filename, src, &imports.Options{Comments: true})
}

// ---------------------------------------------------------------------------
// Templates — parsed once at init
// ---------------------------------------------------------------------------

var funcMap = template.FuncMap{
	"httpConst": func(method string) string {
		m := map[string]string{
			"GET": "http.MethodGet", "POST": "http.MethodPost",
			"PATCH": "http.MethodPatch", "PUT": "http.MethodPut",
			"DELETE": "http.MethodDelete",
		}
		if v, ok := m[method]; ok {
			return v
		}
		return fmt.Sprintf("%q", method)
	},
	"statusConst": func(code int) string {
		m := map[int]string{200: "http.StatusOK", 201: "http.StatusCreated", 202: "http.StatusAccepted", 204: "http.StatusNoContent"}
		if v, ok := m[code]; ok {
			return v
		}
		return strconv.Itoa(code)
	},
	"fmtPath": func(m GoMethod) string {
		if len(m.PathParams) == 0 {
			return fmt.Sprintf(`prefix + "%s"`, m.ResourcePath)
		}
		fmtStr := pathParamRe.ReplaceAllString(m.ResourcePath, "%s")
		args := []string{"prefix"}
		for _, p := range m.PathParams {
			args = append(args, "url.PathEscape("+p.GoName+")")
		}
		return fmt.Sprintf(`fmt.Sprintf("%%s%s", %s)`, fmtStr, strings.Join(args, ", "))
	},
	"errWrap": func(m GoMethod) string {
		if len(m.PathParams) == 0 {
			return fmt.Sprintf(`"%s: %%w", err`, m.Name)
		}
		return fmt.Sprintf(`"%s(%%s): %%w", %s, err`, m.Name, m.PathParams[0].GoName)
	},
	"testPath": func(m GoMethod) string {
		base := fmt.Sprintf("/api/%s/%s/tenant/t-test", m.Namespace, m.Version)
		path := pathParamRe.ReplaceAllString(stripVersionPrefix(m.SpecPath), "test-id")
		return base + path
	},
	"testCallArgs": func(m GoMethod) string {
		if len(m.PathParams) == 0 {
			return ""
		}
		args := make([]string, len(m.PathParams))
		for i := range m.PathParams {
			args[i] = `"test-id"`
		}
		return ", " + strings.Join(args, ", ")
	},
	"testExtraArgs": func(m GoMethod) string {
		if len(m.QueryParams) == 0 {
			return ""
		}
		args := make([]string, len(m.QueryParams))
		for i, qp := range m.QueryParams {
			if qp.Type == "[]string" {
				args[i] = "nil"
			} else {
				args[i] = `""`
			}
		}
		return ", " + strings.Join(args, ", ")
	},
	"isStringSlice": func(s string) bool { return s == "[]string" },
}

var sourceTmpl = template.Must(template.New("source").Funcs(funcMap).Parse(sourceTemplate))
var testTmpl = template.Must(template.New("tests").Delims("<%", "%>").Funcs(funcMap).Parse(testTemplate))

// ---------------------------------------------------------------------------
// Source template
// ---------------------------------------------------------------------------

var sourceTemplate = `// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package {{ .Package }}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"{{ .Module }}/internal/client"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
{{ range .Types }}
{{- if .IsRawJSON }}
// {{ .Comment }}
type {{ .Name }} = json.RawMessage
{{- else if .Fields }}
// {{ .Comment }}
type {{ .Name }} struct {
{{- range .Fields }}
	{{ .Name }} {{ .Type }} ` + "`" + `json:"{{ .JSONTag }}"` + "`" + `
{{- end }}
}
{{- else }}
// {{ .Comment }}
type {{ .Name }} = string
{{- end }}
{{ end }}

// ---------------------------------------------------------------------------
// Methods
// ---------------------------------------------------------------------------
{{ range .Methods }}
{{- if eq .Category "paginated" }}
{{ template "paginated" . }}
{{- else if eq .Category "unwrap" }}
{{ template "unwrap" . }}
{{- else if eq .Category "get" }}
{{ template "get" . }}
{{- else if eq .Category "create" }}
{{ template "create" . }}
{{- else if eq .Category "actionWithResponse" }}
{{ template "actionWithResponse" . }}
{{- else if eq .Category "update" }}
{{ template "update" . }}
{{- else }}
{{ template "action" . }}
{{- end }}
{{- end }}

{{/* ---- Shared sub-templates ---- */}}

{{- define "buildQueryParams" -}}
{{- if .QueryParams }}
	params := url.Values{}
{{- range .QueryParams }}
{{- if eq .Type "[]string" }}
	if len({{ .Go }}) > 0 {
		params.Set("{{ .Spec }}", strings.Join({{ .Go }}, ","))
	}
{{- else }}
	if {{ .Go }} != "" {
		params.Set("{{ .Spec }}", {{ .Go }})
	}
{{- end }}
{{- end }}
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
{{- end }}
{{- end }}

{{- define "get" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .ResponseType }}, error) {
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) (*{{ .ResponseType }}, error) {
{{- end }}
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}
	if err := c.transport.Do(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
{{- if .ReturnsSlice }}
	return result, nil
{{- else }}
	return &result, nil
{{- end }}
}
{{ end }}

{{- define "create" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}) ({{ .ResponseType }}, error) {
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}) (*{{ .ResponseType }}, error) {
{{- end }}
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ fmtPath . }}
{{- if .ContentType }}
	if err := c.transport.DoWithContentType(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, "{{ .ContentType }}", {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- end }}
		return nil, fmt.Errorf({{ errWrap . }})
	}
{{- if .ReturnsSlice }}
	return result, nil
{{- else }}
	return &result, nil
{{- end }}
}
{{ end }}

{{- define "actionWithResponse" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) ({{ .ResponseType }}, error) {
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) (*{{ .ResponseType }}, error) {
{{- end }}
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ fmtPath . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
{{- if .ReturnsSlice }}
	return result, nil
{{- else }}
	return &result, nil
{{- end }}
}
{{ end }}

{{- define "update" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}) error {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
{{- if .ContentType }}
	if err := c.transport.DoWithContentType(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, "{{ .ContentType }}", {{ statusConst .ExpectedStatus }}, nil); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, {{ statusConst .ExpectedStatus }}, nil); err != nil {
{{- end }}
		return fmt.Errorf({{ errWrap . }})
	}
	return nil
}
{{ end }}

{{- define "action" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) error {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, nil); err != nil {
		return fmt.Errorf({{ errWrap . }})
	}
	return nil
}
{{ end }}

{{- define "unwrap" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .UnwrapResults }}, error) {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}

	var result struct {
		TotalCount int              ` + "`" + `json:"totalCount"` + "`" + `
		Results    {{ .UnwrapResults }} ` + "`" + `json:"results"` + "`" + `
	}
	if err := c.transport.Do(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
	return result.Results, nil
}
{{ end }}

{{- define "paginated" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ([]{{ .ItemType }}, error) {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]{{ .ItemType }}, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("{{ .PageSizeParam }}", strconv.Itoa(pageSize))
{{- range .QueryParams }}
{{- if eq .Type "[]string" }}
		if len({{ .Go }}) > 0 {
			params.Set("{{ .Spec }}", strings.Join({{ .Go }}, ","))
		}
{{- else }}
		if {{ .Go }} != "" {
			params.Set("{{ .Spec }}", {{ .Go }})
		}
{{- end }}
{{- end }}

		endpoint := {{ fmtPath . }}
		if encoded := params.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}

{{- if eq .PaginationStyle "hasNext" }}
		var result struct {
			client.PaginatedResponseRepresentation
			Results []{{ .ItemType }} ` + "`" + `json:"{{ .ResultsField }}"` + "`" + `
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
{{- else if eq .PaginationStyle "sizeCheck" }}
		var result struct {
			Results    []{{ .ItemType }} ` + "`" + `json:"{{ .ResultsField }}"` + "`" + `
			TotalCount int64 ` + "`" + `json:"totalCount"` + "`" + `
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, len(result.Results) >= pageSize && len(result.Results) > 0, nil
{{- else if eq .PaginationStyle "totalCount" }}
		var result struct {
			TotalCount int ` + "`" + `json:"totalCount"` + "`" + `
			Results    []{{ .ItemType }} ` + "`" + `json:"{{ .ResultsField }}"` + "`" + `
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		hasNext := (page+1)*pageSize < result.TotalCount
		return result.Results, hasNext, nil
{{- end }}
	})
}
{{ end }}
`

// ---------------------------------------------------------------------------
// Test template (uses <% %> delimiters to avoid {{ }} conflicts with Go maps)
// ---------------------------------------------------------------------------

var testTemplate = `// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package <% .Package %>

import (
	"context"
	"net/http"
	"testing"
)

<% range .Methods -%>
<%- if eq .Category "paginated" %>
<% template "testPaginated" . %>
<%- else if eq .Category "unwrap" %>
<% template "testUnwrap" . %>
<%- else if eq .Category "get" %>
<% template "testGet" . %>
<%- else if eq .Category "create" %>
<% template "testCreate" . %>
<%- else if eq .Category "actionWithResponse" %>
<% template "testActionWithResponse" . %>
<%- else if eq .Category "update" %>
<% template "testUpdate" . %>
<%- else %>
<% template "testAction" . %>
<%- end %>
<% end %>

<%- define "testGet" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{"id": "test-id"})
	})

	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func Test<% .Name %>_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	_, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err == nil {
		t.Fatal("expected error")
	}
}
<% end %>

<%- define "testCreate" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
<%- if .ReturnsSlice %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, []map[string]any{{"id": "new-id"}})
<%- else %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, map[string]any{"id": "new-id"})
<%- end %>
	})

	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %>, &<% .RequestType %>{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
<% end %>

<%- define "testUpdate" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		w.WriteHeader(<% statusConst .ExpectedStatus %>)
	})

	err := c.<% .Name %>(context.Background()<% testCallArgs . %>, &<% .RequestType %>{})
	if err != nil {
		t.Fatal(err)
	}
}
<% end %>

<%- define "testAction" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		w.WriteHeader(<% statusConst .ExpectedStatus %>)
	})

	err := c.<% .Name %>(context.Background()<% testCallArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
}
<% end %>

<%- define "testUnwrap" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
<%- if isStringSlice .UnwrapResults %>
		writeJSON(t, w, http.StatusOK, map[string]any{"totalCount": 1, "results": []string{"item-1"}})
<%- else %>
		writeJSON(t, w, http.StatusOK, map[string]any{"totalCount": 1, "results": []map[string]any{{"id": "item-1"}}})
<%- end %>
	})

	results, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
}
<% end %>

<%- define "testActionWithResponse" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, []map[string]any{{"id": "test-id"}})
	})

	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
<% end %>

<%- define "testPaginated" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results":    []map[string]any{{"id": "item-1"}},
			"totalCount": 1,
			"hasNext":    false,
		})
	})

	results, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
}
<% end %>
`

// ---------------------------------------------------------------------------
// Static files
// ---------------------------------------------------------------------------

func writeStaticFiles(root string, cfg Config) error {
	pkg := cfg.Package
	mod := cfg.Module

	staticFiles := map[string]string{
		"jamfplatform/zz_generated_doc.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Package %s provides a Go client for the Jamf Platform API.
//
// Create a client with [NewClient] and use the typed methods to manage
// Jamf Platform resources such as blueprints, device groups, benchmarks, and devices.
//
//	c := %s.NewClient(
//		"https://your-tenant.apigw.jamf.com",
//		os.Getenv("JAMFPLATFORM_CLIENT_ID"),
//		os.Getenv("JAMFPLATFORM_CLIENT_SECRET"),
//	)
//
//	devices, err := c.ListDevices(ctx, nil, "")
//
// The client handles OAuth2 authentication and token refresh automatically.
//
// Error handling uses [*APIResponseError] for structured API errors:
//
//	device, err := c.GetDevice(ctx, id)
//	if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
//		// handle not found
//	}
package %s
`, pkg, pkg, pkg),

		"jamfplatform/zz_generated_errors.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import "%s/internal/client"

var (
	ErrAuthentication = client.ErrAuthentication
	ErrNotFound       = client.ErrNotFound
)

type APIResponseError = client.APIResponseError
`, pkg, mod),

		"jamfplatform/zz_generated_rsql.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import "%s/internal/client"

type RSQLClause = client.RSQLClause

var BuildRSQLExpression = client.BuildRSQLExpression
var FormatArgument = client.FormatArgument
`, pkg, mod),

		"jamfplatform/zz_generated_poll.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"context"
	"time"

	"%s/internal/client"
)

func PollUntil(ctx context.Context, interval time.Duration, checker func(context.Context) (bool, error)) error {
	return client.PollUntil(ctx, interval, checker)
}
`, pkg, mod),

		"jamfplatform/zz_generated_types.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"context"
	"net/http"
	"time"
)

type TokenCache interface {
	Load(key string) (token string, expiresAt time.Time, ok bool)
	Store(key string, token string, expiresAt time.Time) error
}

type Logger interface {
	LogRequest(ctx context.Context, method, url string, body []byte)
	LogResponse(ctx context.Context, statusCode int, headers http.Header, body []byte)
}
`, pkg),

		"jamfplatform/zz_generated_helpers_test.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testServer(t *testing.T) (*Client, *http.ServeMux) {
	t.Helper()
	return testServerWithOpts(t)
}

func testServerWithOpts(t *testing.T, opts ...Option) (*Client, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "test-id", "test-secret", opts...)
	return c, mux
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			t.Fatalf("writeJSON: %%v", err)
		}
	}
}

func readJSON(t *testing.T, r *http.Request, v any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatalf("readJSON: %%v", err)
	}
}
`, pkg),
	}

	for relPath, content := range staticFiles {
		outPath := filepath.Join(root, relPath)
		formatted, err := formatGo(relPath, []byte(content))
		if err != nil {
			return fmt.Errorf("formatting %s: %w", relPath, err)
		}
		if err := os.WriteFile(outPath, formatted, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", relPath, err)
		}
		log.Printf("wrote %s", relPath)
	}
	return nil
}

// ---------------------------------------------------------------------------
// String utilities
// ---------------------------------------------------------------------------

// Regex for acronym fixup: matches "Id", "Url" etc. only when followed by
// uppercase, end-of-string, or a non-letter — so "Identifier" is not touched.
var acronymFixups = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`Ip([AV])`), "IP$1"},
	{regexp.MustCompile(`Uuid($|[A-Z])`), "UUID$1"},
	{regexp.MustCompile(`Udid($|[A-Z])`), "UDID$1"},
	{regexp.MustCompile(`Url($|[A-Z])`), "URL$1"},
	{regexp.MustCompile(`Odv($|[A-Z])`), "ODV$1"},
	{regexp.MustCompile(`Mdm($|[A-Z])`), "MDM$1"},
	{regexp.MustCompile(`Id($|[A-Z])`), "ID$1"},
}

func exportedGoName(name string) string {
	// Exact matches for single-word properties.
	exact := map[string]string{
		"id": "ID", "ids": "IDs", "url": "URL", "urls": "URLs",
		"udid": "UDID", "ip": "IP", "os": "OS", "odv": "ODV",
		"mdm": "MDM", "uuid": "UUID", "uri": "URI", "href": "Href",
		"macAddress": "MacAddress",
	}
	if v, ok := exact[name]; ok {
		return v
	}

	// camelCase → PascalCase
	var b strings.Builder
	upper := true
	for _, r := range name {
		if r == '_' || r == '-' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	s := b.String()

	// Fix acronyms at word boundaries.
	for _, fix := range acronymFixups {
		s = fix.re.ReplaceAllString(s, fix.repl)
	}
	return s
}

func toLowerCamelCase(s string) string {
	if s == "id" {
		return "id"
	}
	if strings.HasSuffix(s, "Id") {
		return s[:len(s)-2] + "ID"
	}
	return s
}

func cleanComment(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func camelToWords(s string) string {
	var words []string
	var cur strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			if cur.Len() > 0 {
				words = append(words, strings.ToLower(cur.String()))
				cur.Reset()
			}
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		words = append(words, strings.ToLower(cur.String()))
	}
	return strings.Join(words, " ")
}

func isScalar(goType string) bool {
	switch goType {
	case "string", "int", "int32", "int64", "float32", "float64", "bool":
		return true
	}
	return false
}

func cmp(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
