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
	"go/format"
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
)

// ---------------------------------------------------------------------------
// Configuration types (loaded from config.json)
// ---------------------------------------------------------------------------

// Config is the top-level generator configuration.
type Config struct {
	Package string    `json:"package"`
	Module  string    `json:"module"`
	Specs   []SpecDef `json:"specs"`
}

// SpecDef maps one OpenAPI spec file to one generated Go source file.
type SpecDef struct {
	File            string        `json:"file"`
	Namespace       string        `json:"namespace"`
	Version         string        `json:"version"`
	OutputFile      string        `json:"outputFile"`
	TestFile        string        `json:"testFile"`
	SkipSchemas     []string      `json:"skipSchemas"`
	Operations      []OperationDef `json:"operations"`
	Comment         string        `json:"comment,omitempty"`
}

// OperationDef configures how one OpenAPI operation maps to a Go method.
type OperationDef struct {
	Path            string            `json:"path"`
	Method          string            `json:"method"`
	GoName          string            `json:"goName"`
	ContentType     string            `json:"contentType,omitempty"`
	Pagination      string            `json:"pagination,omitempty"`      // "hasNext", "sizeCheck", "totalCount"
	PageSizeParam   string            `json:"pageSizeParam,omitempty"`   // default "page-size"
	VersionOverride string            `json:"versionOverride,omitempty"` // override spec path version
	PathParamNames  map[string]string `json:"pathParamNames,omitempty"`  // spec param -> Go param name
	ExtraParams     []ExtraParam      `json:"extraParams,omitempty"`
	UnwrapResults   string            `json:"unwrapResults,omitempty"`   // e.g. "[]string" — unwrap {results, totalCount} wrapper
	Skip            bool              `json:"skip,omitempty"`
}

// ExtraParam describes a query parameter beyond the standard pagination params.
type ExtraParam struct {
	Spec string `json:"spec"` // URL query param name
	Go   string `json:"go"`   // Go method parameter name
	Type string `json:"type"` // "string" or "[]string"
}

// ---------------------------------------------------------------------------
// Intermediate representation — what the templates render
// ---------------------------------------------------------------------------

// GoType represents a generated Go struct.
type GoType struct {
	Name    string
	Comment string
	Fields  []GoField
}

// GoField represents one field of a generated struct.
type GoField struct {
	Name    string
	Type    string
	JSONTag string
	Comment string
}

// GoMethod represents a generated SDK method on Client.
type GoMethod struct {
	Name            string
	Comment         string
	HTTPMethod      string // GET, POST, PATCH, DELETE
	Namespace       string
	Version         string
	EndpointExpr    string // Go expression building the URL (used in template)
	PathParams      []GoPathParam
	QueryParams     []ExtraParam
	RequestType     string // empty = no body
	ResponseType    string // empty = no response body
	ExpectedStatus  int
	UseDo           bool // true → c.transport.Do, false → DoExpect/DoWithContentType
	ContentType     string
	Paginated       bool
	PaginationStyle string // "hasNext", "sizeCheck", "totalCount"
	PageSizeParam   string // "page-size" or "size"
	ItemType        string // paginated item type
	ResultsField    string // JSON field name ("results")
	ErrorWrapArgs   string // args for fmt.Errorf context
	ReturnsSlice    bool   // true when response is an array type (return []T not *T)
	SpecPath        string // original spec path, e.g. "/v1/devices/{id}"
	UnwrapResults   string // non-empty = unwrap {results, totalCount} wrapper, value is the Go return type e.g. "[]string"
}

// GoPathParam is a path parameter.
type GoPathParam struct {
	SpecName string // as in the spec, e.g. "id", "blueprintId"
	GoName   string // Go parameter name, e.g. "id", "blueprintID"
}

// GeneratedFile bundles all the data needed to render a single output file.
type GeneratedFile struct {
	Package   string
	Namespace string
	Types     []GoType
	Methods   []GoMethod
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
	specPath := filepath.Join(root, spec.File)
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		return fmt.Errorf("loading spec: %w", err)
	}
	// Skip validation — spec examples may have minor type mismatches
	// that don't affect code generation.

	skipSet := make(map[string]bool)
	for _, s := range spec.SkipSchemas {
		skipSet[s] = true
	}

	// Generate types from schemas.
	types := extractTypes(doc, skipSet)

	// Generate methods from configured operations.
	methods, err := extractMethods(doc, spec)
	if err != nil {
		return err
	}

	gf := GeneratedFile{
		Package:   cfg.Package,
		Namespace: spec.Namespace,
		Types:     types,
		Methods:   methods,
	}

	// Render and write the source file.
	src, err := renderSource(gf)
	if err != nil {
		return fmt.Errorf("rendering source: %w", err)
	}
	outPath := filepath.Join(root, spec.OutputFile)
	if err := os.WriteFile(outPath, src, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", spec.OutputFile)

	// Render and write the test file.
	testSrc, err := renderTests(gf)
	if err != nil {
		return fmt.Errorf("rendering tests: %w", err)
	}
	testPath := filepath.Join(root, spec.TestFile)
	if err := os.WriteFile(testPath, testSrc, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", testPath, err)
	}
	log.Printf("wrote %s", spec.TestFile)

	return nil
}

// ---------------------------------------------------------------------------
// Static files — boilerplate that doesn't depend on specs but should
// follow the same generation convention.
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

import (
	"%s/internal/client"
)

// Sentinel errors re-exported from the transport layer.
var (
	ErrAuthentication = client.ErrAuthentication
	ErrNotFound       = client.ErrNotFound
)

// APIResponseError is a type alias for the transport layer's structured API error.
// Users can use errors.As(err, &apiErr) to inspect API response details.
type APIResponseError = client.APIResponseError
`, pkg, mod),

		"jamfplatform/zz_generated_rsql.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"%s/internal/client"
)

// RSQLClause represents a single RSQL filter clause.
type RSQLClause = client.RSQLClause

// BuildRSQLExpression concatenates filter clauses into an RSQL query string.
var BuildRSQLExpression = client.BuildRSQLExpression

// FormatArgument prepares an RSQL argument value, adding quotes/escapes when needed.
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

// PollUntil repeatedly invokes checker until it reports completion or returns an error.
// Between attempts the function waits for the provided interval while respecting context cancellation.
// Use context.WithTimeout to bound the total polling duration.
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

// TokenCache persists OAuth2 tokens across process restarts.
type TokenCache interface {
	Load(key string) (token string, expiresAt time.Time, ok bool)
	Store(key string, token string, expiresAt time.Time) error
}

// Logger is an interface for logging HTTP requests and responses.
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

// testServer creates an httptest.Server with OAuth2 token endpoint and returns
// a Client pointed at it. Tests register additional handlers on the returned mux.
func testServer(t *testing.T) (*Client, *http.ServeMux) {
	t.Helper()
	return testServerWithOpts(t)
}

// testServerWithOpts creates an httptest.Server like testServer but accepts
// additional client options (e.g. WithTenantID).
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

// writeJSON is a test helper that writes a JSON response with the given status code.
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

// readJSON is a test helper that decodes a JSON request body.
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
		formatted, err := formatGo([]byte(content))
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
// Schema → Go types
// ---------------------------------------------------------------------------

func extractTypes(doc *openapi3.T, skip map[string]bool) []GoType {
	var types []GoType

	// Sort schema names for deterministic output.
	names := make([]string, 0, len(doc.Components.Schemas))
	for name := range doc.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if skip[name] {
			continue
		}
		ref := doc.Components.Schemas[name]
		schema := ref.Value
		if schema == nil || schema.Type == nil {
			continue
		}

		// Skip allOf wrappers (pagination envelopes).
		if len(schema.AllOf) > 0 {
			continue
		}

		// Enum string types → type alias.
		if schema.Type.Is("string") && len(schema.Enum) > 0 {
			gt := GoType{
				Name:    name,
				Comment: fmt.Sprintf("%s represents a %s value.", name, camelToWords(name)),
				// Empty fields = will render as type alias in template.
			}
			types = append(types, gt)
			continue
		}

		if !schema.Type.Is("object") {
			continue
		}

		gt := schemaToGoType(name, schema, doc)
		types = append(types, gt)
	}
	return types
}

func schemaToGoType(name string, schema *openapi3.Schema, doc *openapi3.T) GoType {
	gt := GoType{
		Name:    name,
		Comment: fmt.Sprintf("%s represents a %s.", name, camelToWords(name)),
	}
	if schema.Description != "" {
		gt.Comment = fmt.Sprintf("%s %s", name, cleanComment(schema.Description))
	}

	// Sort properties for deterministic output.
	propNames := make([]string, 0, len(schema.Properties))
	for pname := range schema.Properties {
		propNames = append(propNames, pname)
	}
	sort.Strings(propNames)

	required := make(map[string]bool)
	for _, r := range schema.Required {
		required[r] = true
	}

	for _, pname := range propNames {
		propRef := schema.Properties[pname]
		prop := propRef.Value

		goType := openAPITypeToGo(propRef, doc)
		jsonTag := pname

		// Nullable or optional object fields → pointer.
		isNullable := prop != nil && prop.Nullable
		isRequired := required[pname]
		// Only treat $ref as object ref if it resolves to an object schema (not enum).
		isObjectRef := false
		if propRef.Ref != "" {
			resolved := resolveSchema(propRef, doc)
			if resolved != nil && resolved.Type != nil && resolved.Type.Is("object") {
				isObjectRef = true
			}
		}

		needsPointer := isNullable || isObjectRef || (!isRequired && !isScalar(goType))
		if needsPointer && !strings.HasPrefix(goType, "*") && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if isNullable && !strings.HasPrefix(goType, "*") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		}

		gf := GoField{
			Name:    exportedGoName(pname),
			Type:    goType,
			JSONTag: jsonTag,
		}
		if prop != nil && prop.Description != "" {
			gf.Comment = cleanComment(prop.Description)
		}
		gt.Fields = append(gt.Fields, gf)
	}
	return gt
}

func openAPITypeToGo(ref *openapi3.SchemaRef, doc *openapi3.T) string {
	// If it's a $ref, resolve and use the type name.
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
		if schema.Format == "date-time" {
			return "string" // timestamps as strings, matching existing SDK pattern
		}
		return "string"
	case schema.Type.Is("integer"):
		switch schema.Format {
		case "int64":
			return "int64"
		case "int32":
			return "int"
		default:
			return "int"
		}
	case schema.Type.Is("number"):
		if schema.Format == "float" {
			return "float32"
		}
		return "float64"
	case schema.Type.Is("boolean"):
		return "bool"
	case schema.Type.Is("array"):
		if schema.Items != nil {
			itemType := openAPITypeToGo(schema.Items, doc)
			return "[]" + itemType
		}
		return "[]any"
	case schema.Type.Is("object"):
		if schema.AdditionalProperties.Schema != nil {
			valType := openAPITypeToGo(schema.AdditionalProperties.Schema, doc)
			return "map[string]" + valType
		}
		return "map[string]any"
	default:
		return "any"
	}
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
	// Find the operation in the spec.
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
		QueryParams:     opDef.ExtraParams,
		ContentType:     opDef.ContentType,
		Paginated:       opDef.Pagination != "",
		PaginationStyle: opDef.Pagination,
		PageSizeParam:   opDef.PageSizeParam,
		ResultsField:    "results",
	}
	if m.PageSizeParam == "" {
		m.PageSizeParam = "page-size"
	}

	// Build comment from spec summary.
	if op.Summary != "" {
		m.Comment = fmt.Sprintf("%s %s", opDef.GoName, lowerFirst(cleanComment(op.Summary)))
	}

	// Determine path params.
	pathParams := extractPathParams(opDef.Path, opDef.PathParamNames)
	m.PathParams = pathParams

	// Build the endpoint expression and error wrap args.
	resourcePath := stripVersionPrefix(opDef.Path)
	m.EndpointExpr = buildEndpointExpr(resourcePath, pathParams)
	m.ErrorWrapArgs = buildErrorWrapArgs(opDef.GoName, pathParams)

	// Determine expected status and response type from spec responses.
	m.ExpectedStatus, m.ResponseType = detectResponse(op, doc)
	m.UseDo = m.ExpectedStatus == 200

	// For paginated methods, detect item type from response schema.
	if m.Paginated {
		m.ItemType = detectPaginatedItemType(op, doc)
		m.ResponseType = "" // paginated methods return []ItemType, not the wrapper
	}

	// Detect request body type.
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for _, content := range op.RequestBody.Value.Content {
			if content.Schema != nil {
				m.RequestType = resolveTypeName(content.Schema)
				break
			}
		}
	}

	// Mark array response types.
	m.ReturnsSlice = strings.HasPrefix(m.ResponseType, "[]")

	// Store original spec path for test generation.
	m.SpecPath = opDef.Path

	// Unwrap response wrapper pattern.
	m.UnwrapResults = opDef.UnwrapResults

	return m, nil
}

// extractPathParams parses {param} placeholders from the spec path.
func extractPathParams(path string, nameOverrides map[string]string) []GoPathParam {
	re := regexp.MustCompile(`\{(\w+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	var params []GoPathParam
	for _, match := range matches {
		specName := match[1]
		goName := specName
		if override, ok := nameOverrides[specName]; ok {
			goName = override
		} else {
			goName = toLowerCamelCase(specName)
		}
		params = append(params, GoPathParam{SpecName: specName, GoName: goName})
	}
	return params
}

// stripVersionPrefix removes the /v{N}/ prefix from a spec path.
func stripVersionPrefix(path string) string {
	re := regexp.MustCompile(`^/v\d+`)
	return re.ReplaceAllString(path, "")
}

// buildEndpointExpr creates the Go expression that constructs the URL path.
//
// For paths without params: `prefix + "/devices"`
// For paths with params: `fmt.Sprintf("%s/devices/%s", prefix, url.PathEscape(id))`
func buildEndpointExpr(resourcePath string, params []GoPathParam) string {
	if len(params) == 0 {
		return fmt.Sprintf(`prefix + "%s"`, resourcePath)
	}

	fmtStr := regexp.MustCompile(`\{(\w+)\}`).ReplaceAllString(resourcePath, "%s")
	args := []string{"prefix"}
	for _, p := range params {
		args = append(args, fmt.Sprintf("url.PathEscape(%s)", p.GoName))
	}
	return fmt.Sprintf(`fmt.Sprintf("%%s%s", %s)`, fmtStr, strings.Join(args, ", "))
}

// buildErrorWrapArgs creates the arguments for fmt.Errorf error wrapping.
func buildErrorWrapArgs(methodName string, params []GoPathParam) string {
	if len(params) == 0 {
		return fmt.Sprintf(`"%s: %%w", err`, methodName)
	}
	// Use the first path param for error context.
	return fmt.Sprintf(`"%s(%%s): %%w", %s, err`, methodName, params[0].GoName)
}

// detectResponse finds the success status code and response type from the spec.
func detectResponse(op *openapi3.Operation, doc *openapi3.T) (int, string) {
	// Check responses in priority order: 200, 201, 202, 204.
	for _, code := range []int{200, 201, 202, 204} {
		codeStr := strconv.Itoa(code)
		resp := op.Responses.Status(code)
		if resp == nil {
			_ = codeStr
			continue
		}
		if resp.Value == nil {
			return code, ""
		}
		for _, content := range resp.Value.Content {
			if content.Schema != nil {
				typeName := resolveTypeName(content.Schema)
				return code, typeName
			}
		}
		return code, ""
	}
	return 200, ""
}

// detectPaginatedItemType extracts the item type from a paginated response.
func detectPaginatedItemType(op *openapi3.Operation, doc *openapi3.T) string {
	resp := op.Responses.Status(200)
	if resp == nil || resp.Value == nil {
		return "any"
	}
	for _, content := range resp.Value.Content {
		if content.Schema == nil {
			continue
		}
		schema := resolveSchema(content.Schema, doc)
		if schema == nil {
			continue
		}
		// Check allOf composition (pagination wrapper).
		if len(schema.AllOf) > 0 {
			for _, part := range schema.AllOf {
				s := resolveSchema(part, doc)
				if s == nil {
					continue
				}
				resultsProp := s.Properties["results"]
				if resultsProp != nil && resultsProp.Value != nil && resultsProp.Value.Items != nil {
					return resolveTypeName(resultsProp.Value.Items)
				}
			}
		}
		// Direct results field.
		resultsProp := schema.Properties["results"]
		if resultsProp != nil && resultsProp.Value != nil && resultsProp.Value.Items != nil {
			return resolveTypeName(resultsProp.Value.Items)
		}
	}
	return "any"
}

// resolveTypeName extracts the Go type name from a schema ref.
func resolveTypeName(ref *openapi3.SchemaRef) string {
	if ref.Ref != "" {
		parts := strings.Split(ref.Ref, "/")
		return parts[len(parts)-1]
	}
	if ref.Value != nil {
		// Inline schema — attempt to determine type.
		return openAPITypeToGo(ref, nil)
	}
	return "any"
}

// resolveSchema dereferences a $ref to get the actual schema.
func resolveSchema(ref *openapi3.SchemaRef, doc *openapi3.T) *openapi3.Schema {
	if ref.Value != nil {
		return ref.Value
	}
	if ref.Ref != "" {
		parts := strings.Split(ref.Ref, "/")
		name := parts[len(parts)-1]
		if s, ok := doc.Components.Schemas[name]; ok {
			return s.Value
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Template rendering — Source file
// ---------------------------------------------------------------------------

func renderSource(gf GeneratedFile) ([]byte, error) {
	tmpl, err := template.New("source").Funcs(templateFuncs).Parse(sourceTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, gf); err != nil {
		return nil, err
	}
	return formatGo(buf.Bytes())
}

func renderTests(gf GeneratedFile) ([]byte, error) {
	// Use custom delimiters to avoid conflicts with Go map literal {{ }} syntax.
	tmpl, err := template.New("tests").Delims("<%", "%>").Funcs(templateFuncs).Parse(testTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, gf); err != nil {
		return nil, err
	}
	return formatGo(buf.Bytes())
}

func formatGo(src []byte) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		// Return unformatted source for debugging.
		return src, fmt.Errorf("gofmt: %w\n---raw source---\n%s", err, src)
	}
	return formatted, nil
}

// ---------------------------------------------------------------------------
// Template functions
// ---------------------------------------------------------------------------

var templateFuncs = template.FuncMap{
	"httpConst":    httpConst,
	"statusConst":  statusConst,
	"hasBody":      func(s string) bool { return s != "" },
	"hasUnwrap":    func(s string) bool { return s != "" },
	"isStringSlice": func(s string) bool { return s == "[]string" },
	"isSliceType":  func(s string) bool { return strings.HasPrefix(s, "[]") },
	"lower":        strings.ToLower,
	"lowerFirst":   lowerFirst,
	"needsFmt":     needsFmt,
	"needsStrconv": needsStrconv,
	"needsStrings": needsStrings,
	"needsURL":     needsURL,
	"needsClient":  needsClient,
}

func httpConst(method string) string {
	switch method {
	case "GET":
		return "http.MethodGet"
	case "POST":
		return "http.MethodPost"
	case "PATCH":
		return "http.MethodPatch"
	case "PUT":
		return "http.MethodPut"
	case "DELETE":
		return "http.MethodDelete"
	default:
		return fmt.Sprintf("%q", method)
	}
}

func statusConst(code int) string {
	switch code {
	case 200:
		return "http.StatusOK"
	case 201:
		return "http.StatusCreated"
	case 202:
		return "http.StatusAccepted"
	case 204:
		return "http.StatusNoContent"
	default:
		return strconv.Itoa(code)
	}
}

func needsFmt(gf GeneratedFile) bool {
	for _, m := range gf.Methods {
		if len(m.PathParams) > 0 || m.RequestType != "" || m.ResponseType != "" || m.Paginated {
			return true
		}
	}
	return false
}

func needsStrconv(gf GeneratedFile) bool {
	for _, m := range gf.Methods {
		if m.Paginated {
			return true
		}
	}
	return false
}

func needsStrings(gf GeneratedFile) bool {
	for _, m := range gf.Methods {
		for _, qp := range m.QueryParams {
			if qp.Type == "[]string" {
				return true
			}
		}
	}
	return false
}

func needsURL(gf GeneratedFile) bool {
	for _, m := range gf.Methods {
		if len(m.PathParams) > 0 || m.Paginated || len(m.QueryParams) > 0 || m.UnwrapResults != "" {
			return true
		}
	}
	return false
}

func needsClient(gf GeneratedFile) bool {
	for _, m := range gf.Methods {
		if m.Paginated {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Source template
// ---------------------------------------------------------------------------

var sourceTemplate = `// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package {{ .Package }}

import (
	"context"
{{- if needsFmt . }}
	"fmt"
{{- end }}
	"net/http"
{{- if needsURL . }}
	"net/url"
{{- end }}
{{- if needsStrconv . }}
	"strconv"
{{- end }}
{{- if needsStrings . }}
	"strings"
{{- end }}
{{- if needsClient . }}

	"{{ "github.com/Jamf-Concepts/jamfplatform-go-sdk/internal/client" }}"
{{- end }}
)

{{ $ns := .Namespace -}}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
{{ range .Types }}
{{- if .Fields }}
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
{{- if .UnwrapResults }}
{{ template "unwrapMethod" . }}
{{- else if .Paginated }}
{{ template "paginatedMethod" . }}
{{- else if and .UseDo (hasBody .ResponseType) }}
{{ template "getMethod" . }}
{{- else if and (not .UseDo) (hasBody .ResponseType) (hasBody .RequestType) }}
{{ template "createMethod" . }}
{{- else if and (not .UseDo) (hasBody .ResponseType) (not (hasBody .RequestType)) }}
{{ template "actionWithResponseMethod" . }}
{{- else if and (not .UseDo) (not (hasBody .ResponseType)) (hasBody .RequestType) }}
{{ template "updateMethod" . }}
{{- else if and (not .UseDo) (not (hasBody .ResponseType)) (not (hasBody .RequestType)) }}
{{ template "actionMethod" . }}
{{- else }}
{{ template "actionMethod" . }}
{{- end }}
{{- end }}

{{- define "getMethod" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .ResponseType }}, error) {
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) (*{{ .ResponseType }}, error) {
{{- end }}
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ .EndpointExpr }}
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
	if err := c.transport.Do(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, &result); err != nil {
{{- if .ReturnsSlice }}
		return nil, fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return result, nil
{{- else }}
		return nil, fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return &result, nil
{{- end }}
}
{{ end }}

{{- define "createMethod" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}) ({{ .ResponseType }}, error) {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ .EndpointExpr }}
{{- if .ContentType }}
	if err := c.transport.DoWithContentType(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, "{{ .ContentType }}", {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- end }}
		return nil, fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return result, nil
}
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}) (*{{ .ResponseType }}, error) {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ .EndpointExpr }}
{{- if .ContentType }}
	if err := c.transport.DoWithContentType(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, "{{ .ContentType }}", {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- end }}
		return nil, fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return &result, nil
}
{{- end }}
{{ end }}

{{- define "actionWithResponseMethod" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) ({{ .ResponseType }}, error) {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ .EndpointExpr }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, &result); err != nil {
		return nil, fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return result, nil
}
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) (*{{ .ResponseType }}, error) {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ .EndpointExpr }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, &result); err != nil {
		return nil, fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return &result, nil
}
{{- end }}
{{ end }}

{{- define "unwrapMethod" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .UnwrapResults }}, error) {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ .EndpointExpr }}
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

	var result struct {
		TotalCount int              ` + "`" + `json:"totalCount"` + "`" + `
		Results    {{ .UnwrapResults }} ` + "`" + `json:"results"` + "`" + `
	}
	if err := c.transport.Do(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return result.Results, nil
}
{{ end }}

{{- define "updateMethod" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}) error {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ .EndpointExpr }}
{{- if .ContentType }}
	if err := c.transport.DoWithContentType(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, "{{ .ContentType }}", {{ statusConst .ExpectedStatus }}, nil); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, {{ statusConst .ExpectedStatus }}, nil); err != nil {
{{- end }}
		return fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return nil
}
{{ end }}

{{- define "actionMethod" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) error {
	prefix := c.tenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ .EndpointExpr }}
{{- if .UseDo }}
	if err := c.transport.Do(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, nil); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, nil); err != nil {
{{- end }}
		return fmt.Errorf({{ .ErrorWrapArgs }})
	}
	return nil
}
{{ end }}

{{- define "paginatedMethod" }}
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

		endpoint := {{ .EndpointExpr }}
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
// Test template
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
<%- if .UnwrapResults %>
<% template "testUnwrap" . %>
<%- else if .Paginated %>
<% template "testPaginated" . %>
<%- else if and .UseDo (hasBody .ResponseType) %>
<% template "testGet" . %>
<%- else if and (not .UseDo) (hasBody .ResponseType) (hasBody .RequestType) %>
<% template "testCreate" . %>
<%- else if and (not .UseDo) (hasBody .ResponseType) (not (hasBody .RequestType)) %>
<% template "testActionWithResponse" . %>
<%- else if and (not .UseDo) (not (hasBody .ResponseType)) (hasBody .RequestType) %>
<% template "testUpdate" . %>
<%- else if and (not .UseDo) (not (hasBody .ResponseType)) (not (hasBody .RequestType)) %>
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
		writeJSON(t, w, http.StatusOK, map[string]any{
			"id": "test-id",
		})
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
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, map[string]any{
			"id": "new-id",
		})
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
		writeJSON(t, w, http.StatusOK, map[string]any{
			"totalCount": 1,
			"results":    []string{"item-1"},
		})
<%- else %>
		writeJSON(t, w, http.StatusOK, map[string]any{
			"totalCount": 1,
			"results":    []map[string]any{{"id": "item-1"}},
		})
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
// Test template helpers (registered at init)
// ---------------------------------------------------------------------------

func init() {
	templateFuncs["testPath"] = testPath
	templateFuncs["testCallArgs"] = testCallArgs
	templateFuncs["testExtraArgs"] = testExtraArgs
}

// testPath builds the mux.HandleFunc path for a test.
func testPath(m GoMethod) string {
	base := fmt.Sprintf("/api/%s/%s/tenant/t-test", m.Namespace, m.Version)
	resourcePath := stripVersionPrefix(m.SpecPath)
	// Replace {param} with test values.
	re := regexp.MustCompile(`\{(\w+)\}`)
	path := re.ReplaceAllString(resourcePath, "test-id")
	return base + path
}

// testCallArgs builds the Go call arguments for path params in tests.
func testCallArgs(m GoMethod) string {
	if len(m.PathParams) == 0 {
		return ""
	}
	var args []string
	for range m.PathParams {
		args = append(args, `"test-id"`)
	}
	return ", " + strings.Join(args, ", ")
}

// testExtraArgs builds the Go call arguments for query params in tests.
func testExtraArgs(m GoMethod) string {
	if len(m.QueryParams) == 0 {
		return ""
	}
	var args []string
	for _, qp := range m.QueryParams {
		switch qp.Type {
		case "[]string":
			args = append(args, "nil")
		default:
			args = append(args, `""`)
		}
	}
	return ", " + strings.Join(args, ", ")
}

// ---------------------------------------------------------------------------
// String utilities
// ---------------------------------------------------------------------------

// exportedGoName converts a JSON property name to an exported Go field name.
func exportedGoName(name string) string {
	// Handle known acronyms.
	acronyms := map[string]string{
		"id": "ID", "ids": "IDs", "url": "URL", "urls": "URLs",
		"udid": "UDID", "ip": "IP", "os": "OS", "odv": "ODV",
		"mdm": "MDM", "uuid": "UUID", "uri": "URI", "href": "Href",
		"macAddress": "MacAddress",
	}
	if v, ok := acronyms[name]; ok {
		return v
	}

	var result strings.Builder
	upper := true
	for i, r := range name {
		if r == '_' || r == '-' {
			upper = true
			continue
		}
		if upper {
			result.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			result.WriteRune(r)
		}
		// Handle transitions like "userId" → "UserID"
		if i > 0 && unicode.IsLower(rune(name[i-1])) && unicode.IsUpper(r) {
			// Already correctly cased by the input.
		}
	}

	s := result.String()

	// Fix known acronyms. Order matters — longer matches before shorter ones
	// to prevent "Identifier" being corrupted by "Id" → "ID" replacement.
	s = strings.Replace(s, "Identifier", "\x00IDENT\x00", -1) // protect
	s = strings.Replace(s, "IpAddress", "IPAddress", -1)
	s = strings.Replace(s, "IpV", "IPv", -1)
	s = strings.Replace(s, "Uuid", "UUID", -1)
	s = strings.Replace(s, "Udid", "UDID", -1)
	s = strings.Replace(s, "Url", "URL", -1)
	s = strings.Replace(s, "Odv", "ODV", -1)
	s = strings.Replace(s, "Mdm", "MDM", -1)
	s = strings.Replace(s, "Id", "ID", -1)
	s = strings.Replace(s, "\x00IDENT\x00", "Identifier", -1) // restore

	return s
}

// toLowerCamelCase converts a name like "blueprintId" to "blueprintID" or "id" to "id".
func toLowerCamelCase(s string) string {
	if s == "id" {
		return "id"
	}
	// Fix trailing Id → ID for Go conventions.
	if strings.HasSuffix(s, "Id") {
		return s[:len(s)-2] + "ID"
	}
	return s
}

// cleanComment removes newlines and excessive whitespace from a description.
func cleanComment(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// lowerFirst lowercases the first character.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// camelToWords splits "DeviceReadRepresentationV1" into "device read representation v1".
func camelToWords(s string) string {
	var words []string
	var current strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			if current.Len() > 0 {
				words = append(words, strings.ToLower(current.String()))
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, strings.ToLower(current.String()))
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
