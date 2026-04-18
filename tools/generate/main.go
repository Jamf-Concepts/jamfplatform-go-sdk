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
	SpecDir string    `json:"specDir"`
	Specs   []SpecDef `json:"specs"`
}

type SpecDef struct {
	File           string         `json:"file"`
	Namespace      string         `json:"namespace"`
	SpecFile       string         `json:"specFile,omitempty"`       // override published spec filename
	Package        string         `json:"package,omitempty"`        // target Go sub-package under jamfplatform/; empty emits to root (legacy)
	SplitByTag     bool           `json:"splitByTag,omitempty"`     // emit one methods file per OpenAPI tag instead of one per spec
	Operations     []OperationDef `json:"operations"`
	ExcludePaths   []string       `json:"excludePaths,omitempty"`   // "METHOD /path" entries the generator must refuse to include
	SkipDeprecated bool           `json:"skipDeprecated,omitempty"` // omit operations marked deprecated in the spec
}

// baseName derives a Go file base name from the spec file path.
// "testing/device-inventory.yaml" → "device_inventory"
// "testing/benchmarks-report.yaml" → "benchmarks_report"
func (s SpecDef) baseName() string {
	name := filepath.Base(s.File)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	return strings.ReplaceAll(name, "-", "_")
}

func (s SpecDef) outputFile() string  { return "jamfplatform/" + s.baseName() + ".go" }
func (s SpecDef) testOutputFile() string { return "jamfplatform/" + s.baseName() + "_test.go" }

type OperationDef struct {
	Op            string            `json:"op"`                      // "GET /v1/devices/{id}"
	Name          string            `json:"name"`                    // Go method name
	ContentType   string            `json:"contentType,omitempty"`
	Pagination    string            `json:"pagination,omitempty"`    // hasNext, sizeCheck, totalCount
	PageSizeParam string            `json:"pageSizeParam,omitempty"`
	Version       string            `json:"version,omitempty"`       // override version for tenantPrefix
	PathNames     map[string]string `json:"pathNames,omitempty"`     // spec param -> Go param name
	Params        []string          `json:"params,omitempty"`        // "name", "name:type", "spec:type:goName"
	UnwrapResults string            `json:"unwrapResults,omitempty"`
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
	Comment string // godoc line emitted immediately above the field, if non-empty
}

type GoMethod struct {
	Name         string
	Comment      string
	Category     string // get, create, update, action, actionWithResponse, paginated, unwrap
	HTTPMethod   string
	Namespace    string
	Version      string
	Tag          string // first OpenAPI tag of the operation, used when SplitByTag is enabled
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

	if err := validateConfig(cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	emittedTypes := make(map[string]bool) // dedup types across root-package specs
	pkgBuckets := make(map[string][]loadedSpec)
	hasSourceSpecs := true
	for _, spec := range cfg.Specs {
		specPath, usedFallback, err := resolveSpecPath(*rootDir, cfg, spec)
		if err != nil {
			log.Fatalf("spec %s: %v", spec.File, err)
		}
		if usedFallback {
			hasSourceSpecs = false
		}
		if spec.Package == "" {
			if err := processSpec(*rootDir, cfg, spec, specPath, emittedTypes); err != nil {
				log.Fatalf("spec %s: %v", spec.File, err)
			}
		} else {
			pkgBuckets[spec.Package] = append(pkgBuckets[spec.Package], loadedSpec{spec: spec, specPath: specPath})
		}
	}

	pkgNames := make([]string, 0, len(pkgBuckets))
	for name := range pkgBuckets {
		pkgNames = append(pkgNames, name)
	}
	sort.Strings(pkgNames)
	for _, pkgName := range pkgNames {
		if err := processPackage(*rootDir, cfg, pkgName, pkgBuckets[pkgName]); err != nil {
			log.Fatalf("package %s: %v", pkgName, err)
		}
	}
	if err := writeStaticFiles(*rootDir, cfg); err != nil {
		log.Fatalf("static files: %v", err)
	}
	// Only publish filtered specs when source specs are available.
	// In CI the source specs are private; the generator reads from the
	// already-published api/ specs and only regenerates Go code.
	if cfg.SpecDir != "" && hasSourceSpecs {
		if err := publishSpecs(*rootDir, cfg); err != nil {
			log.Fatalf("publishing specs: %v", err)
		}
	}
	log.Println("generation complete")
}

// ---------------------------------------------------------------------------
// Per-spec processing
// ---------------------------------------------------------------------------

// resolveSpecPath returns the path to load for a spec. It tries the source
// spec first (testing/), then falls back to the published spec in api/.
// This allows CI to regenerate Go code from the committed api/ specs when
// the private source specs are not available.
func resolveSpecPath(root string, cfg Config, spec SpecDef) (path string, usedFallback bool, err error) {
	primary := filepath.Join(root, spec.File)
	if _, err := os.Stat(primary); err == nil {
		return primary, false, nil
	}
	if spec.SpecFile == "" {
		return "", false, fmt.Errorf("source spec %s not found and no specFile configured for fallback", spec.File)
	}
	fallback := filepath.Join(root, cfg.SpecDir, spec.SpecFile)
	if _, err := os.Stat(fallback); err != nil {
		return "", false, fmt.Errorf("neither source spec %s nor published spec %s found", spec.File, fallback)
	}
	log.Printf("source spec %s not found, using published spec %s", spec.File, fallback)
	return fallback, true, nil
}

func processSpec(root string, cfg Config, spec SpecDef, specPath string, emittedTypes map[string]bool) error {
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		return fmt.Errorf("loading spec: %w", err)
	}

	if spec.SkipDeprecated {
		spec.Operations = dropDeprecatedOps(doc, spec)
	}

	methods, err := extractMethods(doc, spec)
	if err != nil {
		return err
	}

	// Only generate schemas that are actually referenced by the whitelisted operations
	// and haven't already been emitted by a previous spec.
	referencedSchemas := collectReferencedSchemas(doc, spec)
	for name := range referencedSchemas {
		if emittedTypes[name] {
			delete(referencedSchemas, name)
		}
	}
	types := extractTypes(doc, referencedSchemas)

	for _, t := range types {
		emittedTypes[t.Name] = true
	}

	gf := GeneratedFile{
		Package: cfg.Package,
		Module:  cfg.Module,
		Types:   types,
		Methods: methods,
	}

	for _, pair := range []struct {
		tmpl *template.Template
		out  string
	}{
		{sourceTmpl, spec.outputFile()},
		{testTmpl, spec.testOutputFile()},
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
// Per-package processing (sub-package emission)
// ---------------------------------------------------------------------------

// loadedSpec pairs a SpecDef with the resolved filesystem path to its spec.
type loadedSpec struct {
	spec     SpecDef
	specPath string
}

// processPackage emits a sub-package under jamfplatform/<pkgName>/ containing:
//   - client.go       Client struct + New(*jamfplatform.Client) constructor
//   - types.go        all referenced types deduped across specs in the package
//   - <spec>.go       methods from each spec, one file per spec
//   - <spec>_test.go  matching unit tests
//   - helpers_test.go test-only shims (testServer, WithTenantID alias, etc.)
//
// Types deduplicate within the package only — sub-packages do not share type
// namespace with the root or with each other.
func processPackage(root string, cfg Config, pkgName string, specs []loadedSpec) error {
	pkgDir := filepath.Join(root, "jamfplatform", pkgName)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return fmt.Errorf("creating package dir: %w", err)
	}

	type specWithMethods struct {
		spec     SpecDef
		methods  []GoMethod
		baseName string
	}
	var allSpecs []specWithMethods
	pkgEmitted := make(map[string]bool)
	var allTypes []GoType

	for _, ls := range specs {
		doc, err := openapi3.NewLoader().LoadFromFile(ls.specPath)
		if err != nil {
			return fmt.Errorf("loading %s: %w", ls.spec.File, err)
		}
		spec := ls.spec
		if spec.SkipDeprecated {
			spec.Operations = dropDeprecatedOps(doc, spec)
		}
		methods, err := extractMethods(doc, spec)
		if err != nil {
			return fmt.Errorf("spec %s: %w", spec.File, err)
		}
		allSpecs = append(allSpecs, specWithMethods{spec: spec, methods: methods, baseName: spec.baseName()})

		refs := collectReferencedSchemas(doc, spec)
		for name := range refs {
			if pkgEmitted[name] {
				delete(refs, name)
			}
		}
		types := extractTypes(doc, refs)
		for _, t := range types {
			pkgEmitted[t.Name] = true
		}
		allTypes = append(allTypes, types...)
	}

	if err := emitPkgClient(pkgDir, cfg, pkgName); err != nil {
		return err
	}

	typesGF := GeneratedFile{Package: pkgName, Module: cfg.Module, Types: allTypes}
	if err := emitTemplated(sourceTmpl, typesGF, filepath.Join(pkgDir, "types.go")); err != nil {
		return err
	}

	for _, sm := range allSpecs {
		if sm.spec.SplitByTag {
			if err := emitMethodsByTag(pkgDir, cfg, pkgName, sm.spec, sm.methods); err != nil {
				return err
			}
			continue
		}
		mf := GeneratedFile{Package: pkgName, Module: cfg.Module, Methods: sm.methods}
		if err := emitTemplated(sourceTmpl, mf, filepath.Join(pkgDir, sm.baseName+".go")); err != nil {
			return err
		}
		if err := emitTemplated(testTmpl, mf, filepath.Join(pkgDir, sm.baseName+"_test.go")); err != nil {
			return err
		}
	}

	if err := emitPkgHelpersTest(pkgDir, cfg, pkgName); err != nil {
		return err
	}
	return nil
}

// emitMethodsByTag buckets methods by their first OpenAPI tag and emits one
// source + test file per tag. Operations without a tag error out — untagged
// ops in splitByTag mode signal a spec bug the curator should see.
func emitMethodsByTag(pkgDir string, cfg Config, pkgName string, spec SpecDef, methods []GoMethod) error {
	buckets := make(map[string][]GoMethod)
	for _, m := range methods {
		if m.Tag == "" {
			return fmt.Errorf("spec %s: operation %s has no OpenAPI tag but splitByTag is enabled", spec.File, m.Name)
		}
		buckets[m.Tag] = append(buckets[m.Tag], m)
	}

	tags := make([]string, 0, len(buckets))
	for t := range buckets {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	for _, tag := range tags {
		base := tagToFileBase(tag)
		mf := GeneratedFile{Package: pkgName, Module: cfg.Module, Methods: buckets[tag]}
		if err := emitTemplated(sourceTmpl, mf, filepath.Join(pkgDir, base+".go")); err != nil {
			return err
		}
		if err := emitTemplated(testTmpl, mf, filepath.Join(pkgDir, base+"_test.go")); err != nil {
			return err
		}
	}
	return nil
}

// tagToFileBase converts an OpenAPI tag ("startup-status", "declaration report",
// "mobile-device-extension-attributes-preview") into a Go-friendly filename base.
// Hyphens and whitespace collapse to underscores; non-word characters are dropped.
func tagToFileBase(tag string) string {
	s := strings.ToLower(strings.TrimSpace(tag))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == ' ', r == '\t':
			b.WriteByte('_')
		}
	}
	return b.String()
}

// emitTemplated executes a template and writes the goimports-formatted result
// to outPath (absolute).
func emitTemplated(tmpl *template.Template, data any, outPath string) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("executing template for %s: %w", outPath, err)
	}
	formatted, err := imports.Process(outPath, buf.Bytes(), &imports.Options{Comments: true})
	if err != nil {
		return fmt.Errorf("goimports %s: %w\n---raw---\n%s", outPath, err, buf.String())
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// emitPkgClient writes the per-sub-package client.go — a small Client struct
// wrapping a transport plus a New constructor that takes the root client.
func emitPkgClient(pkgDir string, cfg Config, pkgName string) error {
	src := fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Package %s provides typed access to Jamf Platform %s API endpoints.
package %s

import (
	"%s/internal/client"
	"%s/jamfplatform"
)

// Client provides typed methods for %s operations.
type Client struct {
	transport *client.Transport
}

// New creates a %s client that shares the authenticated transport of the
// given root client.
func New(base *jamfplatform.Client) *Client {
	return &Client{transport: base.Transport()}
}
`, pkgName, pkgName, pkgName, cfg.Module, cfg.Module, pkgName, pkgName)
	outPath := filepath.Join(pkgDir, "client.go")
	formatted, err := imports.Process(outPath, []byte(src), &imports.Options{Comments: true})
	if err != nil {
		return fmt.Errorf("goimports %s: %w", outPath, err)
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// emitPkgHelpersTest writes the per-sub-package helpers_test.go — test-only
// shims that alias jamfplatform.Option and WithTenantID into the sub-package
// namespace so generated test files can use them unqualified.
func emitPkgHelpersTest(pkgDir string, cfg Config, pkgName string) error {
	src := fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"%s/jamfplatform"
)

type Option = jamfplatform.Option

var WithTenantID = jamfplatform.WithTenantID

func testServer(t *testing.T) (*Client, *http.ServeMux) {
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
	base := jamfplatform.NewClient(srv.URL, "test-id", "test-secret", opts...)
	return New(base), mux
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
`, pkgName, cfg.Module)
	outPath := filepath.Join(pkgDir, "helpers_test.go")
	formatted, err := imports.Process(outPath, []byte(src), &imports.Options{Comments: true})
	if err != nil {
		return fmt.Errorf("goimports %s: %w", outPath, err)
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// ---------------------------------------------------------------------------
// Publish filtered specs
// ---------------------------------------------------------------------------

func publishSpecs(root string, cfg Config) error {
	outDir := filepath.Join(root, cfg.SpecDir)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating spec dir: %w", err)
	}

	for _, spec := range cfg.Specs {
		doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(root, spec.File))
		if err != nil {
			return fmt.Errorf("loading %s: %w", spec.File, err)
		}

		specFile := toSnakeCase(doc.Info.Title) + ".json"
		if spec.SpecFile != "" {
			specFile = spec.SpecFile
		}

		// Build whitelist of path+method pairs from config.
		type pathMethod struct{ path, method string }
		allowed := make(map[pathMethod]bool)
		for _, op := range spec.Operations {
			method, path := op.parseOp()
			allowed[pathMethod{path, method}] = true
		}

		// Filter paths: remove operations not in whitelist, remove empty path items.
		for _, path := range doc.Paths.InMatchingOrder() {
			item := doc.Paths.Find(path)
			if item == nil {
				continue
			}
			for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
				if item.GetOperation(method) != nil && !allowed[pathMethod{path, method}] {
					switch method {
					case "GET":
						item.Get = nil
					case "POST":
						item.Post = nil
					case "PUT":
						item.Put = nil
					case "PATCH":
						item.Patch = nil
					case "DELETE":
						item.Delete = nil
					}
				}
			}
			// Remove path entirely if no operations remain.
			hasOps := item.Get != nil || item.Post != nil || item.Put != nil ||
				item.Patch != nil || item.Delete != nil
			if !hasOps {
				doc.Paths.Delete(path)
			}
		}

		// Collect all $ref'd schemas from remaining operations.
		usedSchemas := make(map[string]bool)
		collectRefs(doc, usedSchemas)

		// Prune unreferenced schemas.
		if doc.Components != nil && doc.Components.Schemas != nil {
			for name := range doc.Components.Schemas {
				if !usedSchemas[name] {
					delete(doc.Components.Schemas, name)
				}
			}
		}

		// Remove internal paths (e.g. /internal/v1/...).
		for _, path := range doc.Paths.InMatchingOrder() {
			if strings.HasPrefix(path, "/internal/") {
				doc.Paths.Delete(path)
			}
		}

		// Marshal to JSON.
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling %s: %w", specFile, err)
		}

		outPath := filepath.Join(outDir, specFile)
		if err := os.WriteFile(outPath, append(data, '\n'), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		log.Printf("wrote %s/%s", cfg.SpecDir, specFile)
	}
	return nil
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
	}
	return used
}

// ---------------------------------------------------------------------------
// Schema → Go types
// ---------------------------------------------------------------------------

func extractTypes(doc *openapi3.T, allow map[string]*schemaUsage) []GoType {
	names := sortedKeys(doc.Components.Schemas)
	var types []GoType

	for _, name := range names {
		usage := allow[name]
		if usage == nil {
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

		types = append(types, schemaToGoType(name, schema, usage.isRequest))
	}
	return types
}

func schemaToGoType(name string, schema *openapi3.Schema, isRequest bool) GoType {
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
		// For request types, unrequired scalars also get pointers so callers can
		// distinguish "omit field" from "send zero value" (critical for PATCH).
		// $ref struct pointers only apply to response types; for request types the
		// (isRequest && !isRequired) term handles optional fields instead.
		isStructRef := propRef.Ref != "" && prop != nil && prop.Type != nil &&
			prop.Type.Is("object") && len(prop.Properties) > 0
		needsPtr := isNullable || (isStructRef && !isRequest) || (!isRequired && !isScalar(goType)) ||
			(isRequest && !isRequired)

		if isRequest && !isRequired && !strings.HasPrefix(goType, "*") && (strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[")) {
			// For request types, unrequired slices/maps get pointer-wrapped so
			// callers can distinguish "omit field" (nil) from "send empty" (&[]T{}).
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if needsPtr && !strings.HasPrefix(goType, "*") && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		} else if isNullable && !strings.HasPrefix(goType, "*") {
			goType = "*" + goType
			jsonTag += ",omitempty"
		}

		var fieldComment string
		if prop != nil && (prop.WriteOnly || prop.Format == "password") {
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
		return parts[len(parts)-1]
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
		m, err := buildMethod(doc, spec, opDef)
		if err != nil {
			return nil, fmt.Errorf("operation %s: %w", opDef.Op, err)
		}
		methods = append(methods, m)
	}
	return methods, nil
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
		var base string
		if m.Version == "" {
			base = fmt.Sprintf("/api/%s/tenant/t-test", m.Namespace)
		} else {
			base = fmt.Sprintf("/api/%s/%s/tenant/t-test", m.Namespace, m.Version)
		}
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

{{- if .Types }}
{{ range .Types }}
{{- if .IsRawJSON }}
// {{ .Comment }}
type {{ .Name }} = json.RawMessage
{{- else if .Fields }}
// {{ .Comment }}
type {{ .Name }} struct {
{{- range .Fields }}
{{- if .Comment }}
	// {{ .Comment }}
{{- end }}
	{{ .Name }} {{ .Type }} ` + "`" + `json:"{{ .JSONTag }}"` + "`" + `
{{- end }}
}
{{- else }}
// {{ .Comment }}
type {{ .Name }} = string
{{- end }}
{{ end }}
{{- end }}
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
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
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
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
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
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
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
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
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
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
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
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
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
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
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
		"jamfplatform/doc.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Package %s provides a Go client for the Jamf Platform API.
//
// Create a root client with [NewClient], then construct service clients
// from the sub-packages under jamfplatform/ (devices, devicegroups,
// deviceactions, blueprints, ddmreport, compliancebenchmarks, pro, ...)
// to call typed methods.
//
//	c := %s.NewClient(
//		"https://your-tenant.apigw.jamf.com",
//		os.Getenv("JAMFPLATFORM_CLIENT_ID"),
//		os.Getenv("JAMFPLATFORM_CLIENT_SECRET"),
//		%s.WithTenantID(os.Getenv("JAMFPLATFORM_TENANT_ID")),
//	)
//
//	ds, err := devices.New(c).ListDevices(ctx, nil, "")
//
// The root client handles OAuth2 authentication and token refresh
// automatically; each sub-package shares the same transport via its
// [New] constructor.
//
// Error handling uses [*APIResponseError] for structured API errors:
//
//	d, err := devices.New(c).GetDevice(ctx, id)
//	if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
//		// handle not found
//	}
//
// # Response headers
//
// Generated methods return the decoded body only. Response headers —
// including Location on 201 Created, Retry-After on 429 (which the
// transport already honors with a bounded single retry), and
// Deprecation on soon-to-be-removed endpoints (logged automatically)
// — are available to consumers via the [WithLogger] option. Install a
// Logger whose LogResponse receives http.Header if you need to inspect
// Location or any other per-request header.
//
// Note that the body returned by create endpoints already carries an
// "href" field pointing at the new resource, equivalent to Location.
package %s
`, pkg, pkg, pkg, pkg),

		"jamfplatform/errors.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import "%s/internal/client"

var (
	ErrAuthentication   = client.ErrAuthentication
	ErrNotFound         = client.ErrNotFound
	ErrPathNotSupported = client.ErrPathNotSupported
)

type APIResponseError = client.APIResponseError
`, pkg, mod),

		"jamfplatform/rsql.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import "%s/internal/client"

type RSQLClause = client.RSQLClause

var BuildRSQLExpression = client.BuildRSQLExpression
var FormatArgument = client.FormatArgument
`, pkg, mod),

		"jamfplatform/poll.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

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

		"jamfplatform/types.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

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

		"jamfplatform/helpers_test.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

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

// coalesce returns val if non-empty, otherwise fallback.
func coalesce(val, fallback string) string {
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

// toSnakeCase converts titles to snake_case filenames.
// Handles spaces ("Device Inventory API" → "device_inventory_api"),
// camelCase ("DDMReport" → "ddm_report"), and mixed input.
func toSnakeCase(s string) string {
	// Insert underscore before uppercase runs: "DDMReport" → "DDM_Report"
	s = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`).ReplaceAllString(s, "${1}_${2}")
	// Insert underscore at lower→upper boundary: "deviceAction" → "device_Action"
	s = regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(s, "${1}_${2}")
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
