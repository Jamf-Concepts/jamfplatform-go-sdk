// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/imports"
	"gopkg.in/yaml.v3"
)

// loadSpec reads the OpenAPI spec at path, upconverting Swagger 2.0 documents
// to OpenAPI 3 when necessary. Returns a kin-openapi v3 document the rest of
// the generator can treat uniformly.
//
// allowed is an optional allowlist of "METHOD /path" keys. For Swagger 2.0
// specs, paths not in the allowlist are pruned before conversion — Jamf's
// Classic spec has operations that openapi2conv refuses to convert (multiple
// body params) but that are outside any SDK whitelist anyway.
func loadSpec(path string, allowed map[string]bool) (*openapi3.T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Swagger 2.0 detection: the top-level "swagger" key contains "2.0".
	// Works for both JSON and YAML inputs.
	var probe struct {
		Swagger string `json:"swagger" yaml:"swagger"`
		OpenAPI string `json:"openapi" yaml:"openapi"`
	}
	if strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml") {
		_ = yaml.Unmarshal(data, &probe)
	} else {
		_ = json.Unmarshal(data, &probe)
	}
	if strings.HasPrefix(probe.Swagger, "2.") {
		// kin-openapi's openapi2.T unmarshal path expects JSON, because
		// OpenAPI 3 types nested within it have custom JSON decoders that
		// handle OAS 3.1's "type can be a string OR a list of strings"
		// union correctly. YAML decoded directly into the struct fails on
		// those fields. Convert YAML -> JSON in memory first.
		jsonData := data
		if strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml") {
			var generic any
			if err := yaml.Unmarshal(data, &generic); err != nil {
				return nil, fmt.Errorf("parsing swagger 2.0 yaml: %w", err)
			}
			generic = yamlMapsToJSON(generic)
			jsonData, err = json.Marshal(generic)
			if err != nil {
				return nil, fmt.Errorf("re-encoding swagger 2.0 yaml as json: %w", err)
			}
		}
		var v2 openapi2.T
		if err := json.Unmarshal(jsonData, &v2); err != nil {
			return nil, fmt.Errorf("parsing swagger 2.0: %w", err)
		}
		if allowed != nil {
			pruneSwagger2Paths(&v2, allowed)
		}
		basePath := v2.BasePath
		v3, err := openapi2conv.ToV3(&v2)
		if err != nil {
			return nil, err
		}
		// openapi2conv prepends v2.basePath to every path in the v3 output.
		// Strip it so path keys match what the SDK config uses (without
		// the Classic "/JSSResource/" prefix).
		if basePath != "" && basePath != "/" && v3.Paths != nil {
			trimmed := strings.TrimSuffix(basePath, "/")
			rewritten := openapi3.NewPaths()
			for _, p := range v3.Paths.InMatchingOrder() {
				key := p
				if after, ok := strings.CutPrefix(p, trimmed); ok {
					key = after
					if key == "" {
						key = "/"
					}
				}
				rewritten.Set(key, v3.Paths.Value(p))
			}
			v3.Paths = rewritten
		}
		return v3, nil
	}
	// OpenAPI 3 YAML: convert to JSON in memory so we can strip external
	// parameter $refs that kin-openapi can't resolve (no-op for specs without
	// external refs). Load from the stripped JSON bytes rather than the file.
	if strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml") {
		var generic any
		if err := yaml.Unmarshal(data, &generic); err != nil {
			return nil, fmt.Errorf("parsing yaml: %w", err)
		}
		generic = yamlMapsToJSON(generic)
		jsonData, err := json.Marshal(generic)
		if err != nil {
			return nil, fmt.Errorf("re-encoding yaml as json: %w", err)
		}
		jsonData = stripExternalParamRefs(jsonData)
		return openapi3.NewLoader().LoadFromData(jsonData)
	}
	return openapi3.NewLoader().LoadFromFile(path)
}

// stripExternalParamRefs removes parameter entries from path items and
// operation objects whose $ref points to an external file (value does not
// start with "#"). The generator adds pagination params internally; spec-level
// external param refs are pure documentation and cannot be resolved without
// the referenced file.
func stripExternalParamRefs(data []byte) []byte {
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		return data
	}
	paths, ok := top["paths"].(map[string]any)
	if !ok {
		return data
	}
	modified := false
	httpMethods := []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"}
	filterParams := func(obj map[string]any) {
		params, ok := obj["parameters"].([]any)
		if !ok {
			return
		}
		filtered := make([]any, 0, len(params))
		for _, p := range params {
			pm, ok := p.(map[string]any)
			if !ok {
				filtered = append(filtered, p)
				continue
			}
			ref, _ := pm["$ref"].(string)
			if ref == "" || strings.HasPrefix(ref, "#") {
				filtered = append(filtered, p)
			} else {
				modified = true
			}
		}
		if len(filtered) == 0 {
			delete(obj, "parameters")
		} else {
			obj["parameters"] = filtered
		}
	}
	for _, item := range paths {
		pathItem, ok := item.(map[string]any)
		if !ok {
			continue
		}
		filterParams(pathItem)
		for _, method := range httpMethods {
			if op, ok := pathItem[method].(map[string]any); ok {
				filterParams(op)
			}
		}
	}
	if !modified {
		return data
	}
	result, err := json.Marshal(top)
	if err != nil {
		return data
	}
	return result
}

// allowedOpsSet builds the "METHOD /path" allowlist for a spec from its
// operations + excludePaths lists.
func allowedOpsSet(spec SpecDef) map[string]bool {
	m := make(map[string]bool, len(spec.Operations))
	for _, op := range spec.Operations {
		m[normalizeOpKey(op.Op)] = true
	}
	return m
}

// pruneSwagger2Paths drops operations from v2.Paths that aren't in the
// allowlist (keys "METHOD /path"). Leaves path items intact if at least one
// of their methods survives; otherwise removes the path entry entirely.
func pruneSwagger2Paths(v2 *openapi2.T, allowed map[string]bool) {
	for path, item := range v2.Paths {
		if item == nil {
			continue
		}
		if !allowed["GET "+path] {
			item.Get = nil
		}
		if !allowed["POST "+path] {
			item.Post = nil
		}
		if !allowed["PUT "+path] {
			item.Put = nil
		}
		if !allowed["PATCH "+path] {
			item.Patch = nil
		}
		if !allowed["DELETE "+path] {
			item.Delete = nil
		}
		if !allowed["HEAD "+path] {
			item.Head = nil
		}
		if !allowed["OPTIONS "+path] {
			item.Options = nil
		}
		if item.Get == nil && item.Post == nil && item.Put == nil && item.Patch == nil &&
			item.Delete == nil && item.Head == nil && item.Options == nil {
			delete(v2.Paths, path)
		}
	}
}

// yamlMapsToJSON recursively rewrites map[any]any (yaml.v3's map type) as
// map[string]any so encoding/json can round-trip cleanly.
func yamlMapsToJSON(v any) any {
	switch x := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[fmt.Sprint(k)] = yamlMapsToJSON(v)
		}
		return out
	case map[string]any:
		for k, v := range x {
			x[k] = yamlMapsToJSON(v)
		}
		return x
	case []any:
		for i, v := range x {
			x[i] = yamlMapsToJSON(v)
		}
		return x
	default:
		return v
	}
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
	doc, err := loadSpec(specPath, allowedOpsSet(spec))
	if err != nil {
		return fmt.Errorf("loading spec: %w", err)
	}

	applySchemaRenames(doc, spec.SchemaRenames)
	applySchemaAdditions(doc, spec.SchemaAdditions)
	applySchemaPatches(doc, spec.SchemaPatches)
	applyPropertyRenames(doc, spec.PropertyRenames)
	applyPropertyRemovals(doc, spec.PropertyRemovals)
	applyPostSymmetry(doc, spec.PostSymmetryExcludes)
	if spec.Format == "xml" {
		flattenClassicSizeWrappers(doc)
	}
	hoistInlineObjects(doc, spec.Format)

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
	currentFieldOverrides = spec.FieldTypeOverrides
	currentEmitNullForOptional = buildEmitNullForOptionalSet(spec.EmitNullForOptional)
	currentFieldOrder = spec.FieldOrder
	types := extractTypes(doc, referencedSchemas, spec.Format)
	currentFieldOverrides = nil
	currentEmitNullForOptional = nil
	currentFieldOrder = nil

	for _, t := range types {
		emittedTypes[t.Name] = true
	}

	// All root-package specs share a single Go package (legacy path), so the
	// validator has visibility into every type already emitted across prior
	// specs — any method reference must resolve against that accumulated set.
	declared := make([]GoType, 0, len(emittedTypes))
	for name := range emittedTypes {
		declared = append(declared, GoType{Name: name})
	}
	if err := validateTypeReferences(fmt.Sprintf("spec %s", spec.File), declared, methods); err != nil {
		return err
	}

	gf := GeneratedFile{
		Package: cfg.Package,
		Module:  cfg.Module,
		Format:  spec.Format,
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
// When all specs in the package are typesOnly, the output is limited to:
//   - types.go        all schemas from the spec (not just operation-referenced)
//   - types_test.go   JSON round-trip tests for each struct type
//
// Types deduplicate within the package only — sub-packages do not share type
// namespace with the root or with each other.
//
// pkgName may contain slashes (e.g. "bpcomponents/swupdate") to create nested
// directories. The Go package declaration uses only the last path segment.
func processPackage(root string, cfg Config, pkgName string, specs []loadedSpec) error {
	pkgDir := filepath.Join(root, "jamfplatform", pkgName)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return fmt.Errorf("creating package dir: %w", err)
	}

	// The Go package name is the last segment of a potentially slashed path.
	goPkgName := filepath.Base(pkgName)

	allTypesOnly := true
	for _, ls := range specs {
		if !ls.spec.TypesOnly {
			allTypesOnly = false
			break
		}
	}

	if allTypesOnly {
		return processPackageTypesOnly(root, cfg, pkgDir, goPkgName, specs)
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
		doc, err := loadSpec(ls.specPath, allowedOpsSet(ls.spec))
		if err == nil {
			applySchemaRenames(doc, ls.spec.SchemaRenames)
			applySchemaAdditions(doc, ls.spec.SchemaAdditions)
			applySchemaPatches(doc, ls.spec.SchemaPatches)
			applyPropertyRenames(doc, ls.spec.PropertyRenames)
			applyPropertyRemovals(doc, ls.spec.PropertyRemovals)
			applyPostSymmetry(doc, ls.spec.PostSymmetryExcludes)
			if ls.spec.Format == "xml" {
				flattenClassicSizeWrappers(doc)
			}
			hoistInlineObjects(doc, ls.spec.Format)
		}
		if err != nil {
			return fmt.Errorf("loading %s: %w", ls.spec.File, err)
		}
		spec := ls.spec
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
		currentFieldOverrides = spec.FieldTypeOverrides
		currentEmitNullForOptional = buildEmitNullForOptionalSet(spec.EmitNullForOptional)
		currentFieldOrder = spec.FieldOrder
		types := extractTypes(doc, refs, spec.Format)
		currentFieldOverrides = nil
		currentEmitNullForOptional = nil
		currentFieldOrder = nil
		for _, t := range types {
			pkgEmitted[t.Name] = true
		}
		allTypes = append(allTypes, types...)
	}

	pkgFormat := ""
	if len(allSpecs) > 0 {
		pkgFormat = allSpecs[0].spec.Format
	}

	// Validate references before writing any file — an unresolved Go type
	// reference in a method will surface later as a go build error with no
	// pointer back to the spec/op responsible. The validator works on the
	// union of types emitted across all specs in this package, matching
	// the way the templates will actually see them.
	for _, sm := range allSpecs {
		if err := validateTypeReferences(fmt.Sprintf("spec %s (package %s)", sm.spec.File, pkgName), allTypes, sm.methods); err != nil {
			return err
		}
	}

	if err := emitPkgClient(pkgDir, cfg, goPkgName); err != nil {
		return err
	}

	var allMethods []GoMethod
	for _, sm := range allSpecs {
		allMethods = append(allMethods, sm.methods...)
	}
	if err := emitPkgPrivileges(pkgDir, cfg, goPkgName, allMethods); err != nil {
		return err
	}

	typesGF := GeneratedFile{Package: goPkgName, Module: cfg.Module, Format: pkgFormat, Types: allTypes}
	if err := emitTemplated(sourceTmpl, typesGF, filepath.Join(pkgDir, "types.go")); err != nil {
		return err
	}

	for _, sm := range allSpecs {
		if sm.spec.SplitByTag {
			if err := emitMethodsByTag(pkgDir, cfg, goPkgName, sm.spec, sm.methods); err != nil {
				return err
			}
			continue
		}
		mf := GeneratedFile{Package: goPkgName, Module: cfg.Module, Format: sm.spec.Format, Methods: sm.methods}
		if err := emitTemplated(sourceTmpl, mf, filepath.Join(pkgDir, sm.baseName+".go")); err != nil {
			return err
		}
		if err := emitTemplated(testTmpl, mf, filepath.Join(pkgDir, sm.baseName+"_test.go")); err != nil {
			return err
		}
	}

	if err := emitPkgHelpersTest(pkgDir, cfg, goPkgName, pkgFormat); err != nil {
		return err
	}
	if err := emitPkgXMLSupplements(pkgDir, goPkgName, pkgFormat); err != nil {
		return err
	}
	if err := emitPkgXMLSupplementsTest(pkgDir, goPkgName, pkgFormat); err != nil {
		return err
	}
	if err := emitPkgPrivilegesRoundTripTest(pkgDir, goPkgName); err != nil {
		return err
	}

	// Emit versionLock helpers if any Apply method uses optimistic locking.
	hasVersionLock := false
	for _, sm := range allSpecs {
		for _, m := range sm.methods {
			if m.Apply != nil && m.Apply.VersionLock {
				hasVersionLock = true
				break
			}
		}
		if hasVersionLock {
			break
		}
	}
	if hasVersionLock {
		if err := emitPkgVersionLockHelpers(pkgDir, cfg, goPkgName); err != nil {
			return err
		}
	}
	return nil
}

// processPackageTypesOnly emits a types-only sub-package: types.go with all
// schemas from the spec, and types_test.go with JSON round-trip tests. No
// client struct, no method files, no test helpers.
func processPackageTypesOnly(root string, cfg Config, pkgDir, goPkgName string, specs []loadedSpec) error {
	pkgEmitted := make(map[string]bool)
	var allTypes []GoType
	pkgFormat := ""

	for _, ls := range specs {
		doc, err := loadSpec(ls.specPath, nil)
		if err != nil {
			return fmt.Errorf("loading %s: %w", ls.spec.File, err)
		}
		applySchemaRenames(doc, ls.spec.SchemaRenames)
		applySchemaAdditions(doc, ls.spec.SchemaAdditions)
		applySchemaPatches(doc, ls.spec.SchemaPatches)
		applyPropertyRenames(doc, ls.spec.PropertyRenames)
		applyPropertyRemovals(doc, ls.spec.PropertyRemovals)
		applyPostSymmetry(doc, ls.spec.PostSymmetryExcludes)
		if ls.spec.Format == "xml" {
			flattenClassicSizeWrappers(doc)
		}
		hoistInlineObjects(doc, ls.spec.Format)

		refs := collectAllSchemas(doc)
		for name := range refs {
			if pkgEmitted[name] {
				delete(refs, name)
			}
		}
		currentFieldOverrides = ls.spec.FieldTypeOverrides
		currentEmitNullForOptional = buildEmitNullForOptionalSet(ls.spec.EmitNullForOptional)
		currentFieldOrder = ls.spec.FieldOrder
		suppressWriteOnly = true
		types := extractTypes(doc, refs, ls.spec.Format)
		suppressWriteOnly = false
		currentFieldOverrides = nil
		currentEmitNullForOptional = nil
		currentFieldOrder = nil
		for _, t := range types {
			pkgEmitted[t.Name] = true
		}
		allTypes = append(allTypes, types...)
		if pkgFormat == "" {
			pkgFormat = ls.spec.Format
		}
	}

	typesGF := GeneratedFile{Package: goPkgName, Module: cfg.Module, Format: pkgFormat, Types: allTypes}
	if err := emitTemplated(sourceTmpl, typesGF, filepath.Join(pkgDir, "types.go")); err != nil {
		return err
	}
	if err := emitTypesOnlyTest(pkgDir, goPkgName, allTypes); err != nil {
		return err
	}
	return nil
}

// emitTypesOnlyTest writes a types_test.go file with JSON round-trip tests
// for each struct type in a types-only package. Each test constructs a
// zero-value instance, marshals it to JSON, then unmarshals back — proving
// the generated struct tags and any custom marshal/unmarshal methods work
// correctly. Discriminator types get a per-variant round-trip test that
// verifies the correct variant pointer is populated after unmarshaling.
func emitTypesOnlyTest(pkgDir, pkgName string, types []GoType) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, `// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"encoding/json"
	"testing"
)
`, pkgName)

	for _, t := range types {
		if t.AliasTarget != "" || t.IsRawJSON {
			continue
		}
		if t.Discriminator != nil {
			// Per-variant round-trip test for discriminator (union) types.
			for _, v := range t.Discriminator.Variants {
				fmt.Fprintf(&buf, `
func TestRoundTrip_%s_%s(t *testing.T) {
	original := %s{%s: "%s", %s: &%s{}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	var decoded %s
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %%v", err)
	}
	if decoded.%s != "%s" {
		t.Errorf("discriminator = %%q, want %%q", decoded.%s, "%s")
	}
	if decoded.%s == nil {
		t.Fatal("expected variant %s to be non-nil")
	}
}
`, t.Name, v.FieldName,
					t.Name, t.Discriminator.GoFieldName, v.Value, v.FieldName, v.TypeName,
					t.Name,
					t.Discriminator.GoFieldName, v.Value, t.Discriminator.GoFieldName, v.Value,
					v.FieldName, v.FieldName)
			}
			continue
		}
		if len(t.Fields) == 0 {
			// Enum string types — no fields, just verify it compiles.
			continue
		}
		fmt.Fprintf(&buf, `
func TestRoundTrip_%s(t *testing.T) {
	original := %s{}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	var decoded %s
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %%v", err)
	}
}
`, t.Name, t.Name, t.Name)
	}

	outPath := filepath.Join(pkgDir, "types_test.go")
	formatted, err := formatGo("types_test.go", buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting types_test.go: %w\n---raw---\n%s", err, buf.String())
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// emitPkgXMLSupplements writes xml_helpers.go into XML packages. The file
// declares the supplemental types (BigInt, NotificationValue) that
// FieldTypeOverrides target to paper over Classic spec-vs-wire mismatches.
// No-op for JSON packages. Keeping these in a single generated file
// enforces the invariant that sub-packages under jamfplatform/ contain
// only generator output — no handwritten code to drift out of sync with
// spec changes.
func emitPkgXMLSupplements(pkgDir, pkgName, pkgFormat string) error {
	if pkgFormat != "xml" {
		return nil
	}
	src := fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Supplemental types targeted by FieldTypeOverrides to work around Jamf
// Classic spec-vs-wire mismatches — the XML spec under-types some fields
// (integers that overflow int64, booleans whose wire form is repeated
// with a second string value) that need richer Go models to round-trip
// cleanly. Lives in the generated output so it stays in lock-step with
// any spec/override changes; do not add handwritten files to this
// package.

package %s

import (
	"encoding/xml"
	"math/big"
	"strconv"
)

// BigInt is an arbitrary-precision integer with XML/JSON codecs. The zero
// value is usable and equivalent to big.NewInt(0). Targeted by
// FieldTypeOverrides for Classic fields the spec types as `+"`integer`"+` whose
// actual wire values exceed int64 — canonically invitation codes and
// epoch millis beyond year ~2500.
type BigInt struct {
	v big.Int
}

// Int returns a pointer to the underlying math/big.Int so callers can do
// arithmetic without having to export the internal field. Mutations via
// the returned pointer are reflected in subsequent marshalling.
func (b *BigInt) Int() *big.Int { return &b.v }

// String returns the decimal representation, matching the wire form.
func (b BigInt) String() string { return b.v.String() }

// SetString parses a decimal integer and stores it. Returns false if the
// input isn't a valid base-10 integer.
func (b *BigInt) SetString(s string) bool {
	_, ok := b.v.SetString(s, 10)
	return ok
}

// UnmarshalXML reads the element's text value and parses it as a base-10
// integer. Empty content or a non-numeric sentinel (Classic occasionally
// emits "Unlimited" in otherwise-numeric fields) decodes to zero rather
// than erroring out. Consumers who care about sentinel detection can
// inspect the raw body via WithLogger.
func (b *BigInt) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	if s == "" {
		b.v.SetInt64(0)
		return nil
	}
	if _, ok := b.v.SetString(s, 10); !ok {
		b.v.SetInt64(0)
	}
	return nil
}

// MarshalXML emits the decimal string representation as the element's
// text content.
func (b BigInt) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(b.v.String(), start)
}

// UnmarshalJSON accepts either a JSON number (emitted unquoted) or a JSON
// string containing a decimal integer. Jamf APIs returning JSON
// responses can use either encoding depending on the renderer.
func (b *BigInt) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" || s == "null" {
		b.v.SetInt64(0)
		return nil
	}
	if _, ok := b.v.SetString(s, 10); !ok {
		b.v.SetInt64(0)
	}
	return nil
}

// MarshalJSON emits the value as a JSON number (unquoted) so consumers
// see the same numeric semantics they would for a regular integer.
func (b BigInt) MarshalJSON() ([]byte, error) {
	return []byte(b.v.String()), nil
}

// NotificationValue captures the self-service notification wire element.
// The Classic server emits two <notification> tags in one <self_service>
// block — one a boolean ("true"/"false") and one naming the method
// ("Self Service", ...). A scalar *bool or *string can only capture the
// last element, and Go's XML decoder fails outright when it tries to
// ParseBool the string form. NotificationValue decodes each occurrence
// into its semantic slot (Enabled or Method) so both pieces of
// information survive round-trip, and MarshalXML writes them back as
// separate <notification> elements to preserve the expected wire shape.
type NotificationValue struct {
	Enabled *bool
	Method  *string
}

// UnmarshalXML routes the element's text to Enabled when it parses as a
// bool, otherwise into Method. Called once per <notification> element
// the decoder encounters in the parent self_service block.
func (n *NotificationValue) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	if b, err := strconv.ParseBool(s); err == nil {
		n.Enabled = &b
		return nil
	}
	m := s
	n.Method = &m
	return nil
}

// MarshalXML emits up to two <notification> elements: the method form
// first if Method is set, then the bool form if Enabled is set. Omits
// both when neither is set.
//
// Order matters. The Classic server's self_service parser takes the
// second <notification> as the "primary" value; sending bool first then
// method silently drops the bool (server returns false on the next GET
// regardless of what was sent). Wire-probed on a live tenant and
// confirmed against the order the Jamf Pro admin UI itself writes —
// method first, bool second — which is the only order that round-trips
// cleanly for every resource carrying a Notification field.
func (n NotificationValue) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if n.Method != nil {
		if err := e.EncodeElement(*n.Method, start); err != nil {
			return err
		}
	}
	if n.Enabled != nil {
		if err := e.EncodeElement(strconv.FormatBool(*n.Enabled), start); err != nil {
			return err
		}
	}
	return nil
}

// PayloadsXMLText wraps the configuration-profile <payloads> chardata
// (raw .mobileconfig plist XML embedded as element text). MarshalXML
// emits the standard single-level XML chardata encoding via
// encoding/xml's default escape (escapes `+"`<`"+`, `+"`>`"+`, `+"`&`"+`; leaves `+"`\"`"+`
// alone, which is legal in chardata). UnmarshalXML decodes one level
// symmetrically.
//
// Single-escape applies on both Create (POST) and Update (PUT), matching
// the production-tested approach jamf-upload has shipped across the
// AutoPkg community for years. An earlier double-escape design
// (commit 70b4cd6, 2026-05-26) was added in response to a POST 409
// "Unable to update the database" and assumed the server runs an extra
// entity-decode pass before its plist parser. Wire-probing on 2026-05-27
// showed that diagnosis was wrong: double-escaping silently corrupted
// every <string> value containing literal `+"`&`"+`, `+"`<`"+`, or `+"`>`"+` — POSTed
// values stored as `+"`&amp;`"+`, `+"`&lt;`"+`, `+"`&gt;`"+` doubled to `+"`&amp;amp;`"+` etc.,
// and the same content 409d (or worse, corrupted) on PUT. Single-escape
// for both methods matches what the server actually expects and what
// the Jamf Pro admin UI itself writes.
type PayloadsXMLText string

// MarshalXML emits the chardata at a single XML entity layer (encoding/
// xml's default behaviour). Used on both POST and PUT — the server
// canonicalises the parsed plist after a single chardata decode on POST
// and stores the post-decode bytes verbatim on PUT, both of which
// preserve single-escape content correctly.
func (p PayloadsXMLText) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(string(p), start)
}

// UnmarshalXML decodes the element's text using encoding/xml's default
// pass (one level of entity unescape) and stores the result verbatim.
// Symmetric to MarshalXML.
func (p *PayloadsXMLText) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	*p = PayloadsXMLText(s)
	return nil
}
`, pkgName)
	outPath := filepath.Join(pkgDir, "xml_helpers.go")
	formatted, err := formatGo("xml_helpers.go", []byte(src))
	if err != nil {
		return fmt.Errorf("formatting xml_helpers.go: %w", err)
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// emitPkgXMLSupplementsTest writes xml_helpers_test.go into XML packages,
// covering the supplemental wrapper types declared in xml_helpers.go.
// No-op for JSON packages. Lives in the generated output to keep the
// invariant that sub-packages under jamfplatform/ contain only generator
// output — no handwritten test files in package proclassic.
func emitPkgXMLSupplementsTest(pkgDir, pkgName, pkgFormat string) error {
	if pkgFormat != "xml" {
		return nil
	}
	src := fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"encoding/xml"
	"strings"
	"testing"
)

// payloadsTestParent exercises PayloadsXMLText inside a struct field so
// the test covers both the wrapper's MarshalXML/UnmarshalXML AND the
// surrounding encoding/xml machinery (omitempty, nested struct round
// trip). Marshalling the wrapper standalone would skip those paths.
type payloadsTestParent struct {
	XMLName  xml.Name          %sxml:"parent"%s
	Payloads *PayloadsXMLText  %sxml:"payloads,omitempty"%s
}

func TestPayloadsXMLText_MarshalSingleEncodesEntities(t *testing.T) {
	// Caller submits raw plist XML containing chardata-special characters
	// (%s, %s, %s). The wrapper must emit those at a single XML entity
	// layer — encoding/xml's default chardata escape — matching what
	// jamf-upload writes (production-tested across the AutoPkg community)
	// and what the Jamf Pro admin UI itself writes.
	v := PayloadsXMLText(%s<string>a & b</string>%s)
	parent := payloadsTestParent{Payloads: &v}
	out, err := xml.Marshal(parent)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	got := string(out)
	if strings.Contains(got, "&amp;amp;") {
		t.Fatalf("wire form is double-escaped (corrupts content): %%s", got)
	}
	if !strings.Contains(got, "&amp;") || !strings.Contains(got, "&lt;string&gt;") {
		t.Fatalf("wire form not single-escaped as expected: %%s", got)
	}
}

func TestPayloadsXMLText_MarshalSingleEscapesQuotes(t *testing.T) {
	// Go's encoding/xml conservatively escapes %s in chardata to %s
	// (legal but not strictly required per XML spec). The chosen form
	// must be a single entity layer — never %s (double-escape, the
	// regression form that breaks device-side parsing of CodeRequirement
	// / TCC / PPPC string content).
	v := PayloadsXMLText(%sidentifier "com.foo" and anchor apple generic%s)
	parent := payloadsTestParent{Payloads: &v}
	out, err := xml.Marshal(parent)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	got := string(out)
	if strings.Contains(got, "&amp;#34;") || strings.Contains(got, "&amp;quot;") {
		t.Fatalf("wire form double-escapes literal quote: %%s", got)
	}
}

func TestPayloadsXMLText_UnmarshalSingleDecodes(t *testing.T) {
	// Server returns chardata at one level of XML entity encoding. The
	// decoder unescapes once; the wrapper stores the result verbatim.
	wire := []byte("<parent><payloads>ab&amp;cd&lt;br/&gt;ef</payloads></parent>")
	var got payloadsTestParent
	if err := xml.Unmarshal(wire, &got); err != nil {
		t.Fatalf("unmarshal: %%v", err)
	}
	if got.Payloads == nil {
		t.Fatalf("decoded payloads is nil")
	}
	want := "ab&cd<br/>ef"
	if string(*got.Payloads) != want {
		t.Fatalf("decode mismatch:\n  want: %%s\n  got:  %%s", want, string(*got.Payloads))
	}
}

func TestPayloadsXMLText_NilOmitsField(t *testing.T) {
	parent := payloadsTestParent{}
	out, err := xml.Marshal(parent)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	if strings.Contains(string(out), "<payloads") {
		t.Fatalf("expected <payloads> omitted when field is nil, got: %%s", out)
	}
}
`, pkgName, "`", "`", "`", "`",
		"`<`", "`>`", "`&`",
		"`", "`",
		"`\"`", "`&#34;`", "`&amp;#34;`",
		"`", "`")
	outPath := filepath.Join(pkgDir, "xml_helpers_test.go")
	formatted, err := formatGo("xml_helpers_test.go", []byte(src))
	if err != nil {
		return fmt.Errorf("formatting xml_helpers_test.go: %w\n---raw---\n%s", err, src)
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// emitPkgPrivilegesRoundTripTest writes accounts_privileges_test.go into the
// proclassic package. It proves that AccountPrivileges and GroupPrivileges
// marshal/unmarshal without privilege collapse — each category must emit a
// single container element with N <privilege> children, not N repeated
// container elements (one per privilege). Also covers Group.LdapServer
// round-trip using the real DataJARLDAPS_JamfPro_Admins wire fixture.
// No-op for all other packages.
func emitPkgPrivilegesRoundTripTest(pkgDir, pkgName string) error {
	if pkgName != "proclassic" {
		return nil
	}
	src := fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"encoding/xml"
	"strings"
	"testing"
)

// TestGroupPrivileges_RoundTrip_MarshalXMLCategory verifies that a
// MarshalXML-bearing category (casper_admin) emits ONE container element
// with N <privilege> children — not N repeated container elements. Mirrors
// the class-fix regression guard.
func TestGroupPrivileges_RoundTrip_MarshalXMLCategory(t *testing.T) {
	privs := []string{"Read Computers", "Update Computers", "Create Computers"}
	g := Group{
		Privileges: &GroupPrivileges{
			CasperAdmin: &GroupPrivilegesCasperAdmin{Privilege: &privs},
		},
	}
	out, err := xml.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	got := string(out)
	if n := strings.Count(got, "<casper_admin>"); n != 1 {
		t.Fatalf("want 1 <casper_admin> wrapper, got %%d:\n%%s", n, got)
	}
	if n := strings.Count(got, "<privilege>"); n != 3 {
		t.Fatalf("want 3 <privilege> children, got %%d:\n%%s", n, got)
	}

	var back Group
	if err := xml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %%v", err)
	}
	if back.Privileges == nil || back.Privileges.CasperAdmin == nil || back.Privileges.CasperAdmin.Privilege == nil {
		t.Fatal("privileges.casper_admin decoded as nil")
	}
	got2 := *back.Privileges.CasperAdmin.Privilege
	if len(got2) != 3 {
		t.Fatalf("want 3 privileges after unmarshal, got %%d: %%v", len(got2), got2)
	}
	for i, want := range privs {
		if got2[i] != want {
			t.Fatalf("privilege[%%d]: want %%q, got %%q", i, want, got2[i])
		}
	}
}

// TestGroupPrivileges_RoundTrip_PlainCategory verifies that a plain category
// (jss_objects, no custom MarshalXML on the item type) also emits ONE
// container with N children.
func TestGroupPrivileges_RoundTrip_PlainCategory(t *testing.T) {
	privs := []string{"Create Computers", "Read Computers", "Update Computers"}
	g := Group{
		Privileges: &GroupPrivileges{
			JssObjects: &GroupPrivilegesJssObjects{Privilege: &privs},
		},
	}
	out, err := xml.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	got := string(out)
	if n := strings.Count(got, "<jss_objects>"); n != 1 {
		t.Fatalf("want 1 <jss_objects> wrapper, got %%d:\n%%s", n, got)
	}
	if n := strings.Count(got, "<privilege>"); n != 3 {
		t.Fatalf("want 3 <privilege> children, got %%d:\n%%s", n, got)
	}

	var back Group
	if err := xml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %%v", err)
	}
	if back.Privileges == nil || back.Privileges.JssObjects == nil || back.Privileges.JssObjects.Privilege == nil {
		t.Fatal("privileges.jss_objects decoded as nil")
	}
	got2 := *back.Privileges.JssObjects.Privilege
	if len(got2) != 3 {
		t.Fatalf("want 3 privileges after unmarshal, got %%d: %%v", len(got2), got2)
	}
	for i, want := range privs {
		if got2[i] != want {
			t.Fatalf("privilege[%%d]: want %%q, got %%q", i, want, got2[i])
		}
	}
}

// TestAccountPrivileges_RoundTrip_MarshalXMLCategory mirrors the group test
// for Account privileges (same spec defect, same fix, both must pass).
func TestAccountPrivileges_RoundTrip_MarshalXMLCategory(t *testing.T) {
	privs := []string{"Read Computers", "Update Computers", "Create Computers"}
	a := Account{
		Privileges: &AccountPrivileges{
			CasperAdmin: &AccountPrivilegesCasperAdmin{Privilege: &privs},
		},
	}
	out, err := xml.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	got := string(out)
	if n := strings.Count(got, "<casper_admin>"); n != 1 {
		t.Fatalf("want 1 <casper_admin> wrapper, got %%d:\n%%s", n, got)
	}
	if n := strings.Count(got, "<privilege>"); n != 3 {
		t.Fatalf("want 3 <privilege> children, got %%d:\n%%s", n, got)
	}

	var back Account
	if err := xml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %%v", err)
	}
	if back.Privileges == nil || back.Privileges.CasperAdmin == nil || back.Privileges.CasperAdmin.Privilege == nil {
		t.Fatal("privileges.casper_admin decoded as nil")
	}
	got2 := *back.Privileges.CasperAdmin.Privilege
	if len(got2) != 3 {
		t.Fatalf("want 3 privileges after unmarshal, got %%d: %%v", len(got2), got2)
	}
	for i, want := range privs {
		if got2[i] != want {
			t.Fatalf("privilege[%%d]: want %%q, got %%q", i, want, got2[i])
		}
	}
}

// TestAccountPrivileges_RoundTrip_PlainCategory mirrors the plain-category
// group test for Account.
func TestAccountPrivileges_RoundTrip_PlainCategory(t *testing.T) {
	privs := []string{"Create Computers", "Read Computers", "Update Computers"}
	a := Account{
		Privileges: &AccountPrivileges{
			JssObjects: &AccountPrivilegesJssObjects{Privilege: &privs},
		},
	}
	out, err := xml.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %%v", err)
	}
	got := string(out)
	if n := strings.Count(got, "<jss_objects>"); n != 1 {
		t.Fatalf("want 1 <jss_objects> wrapper, got %%d:\n%%s", n, got)
	}
	if n := strings.Count(got, "<privilege>"); n != 3 {
		t.Fatalf("want 3 <privilege> children, got %%d:\n%%s", n, got)
	}

	var back Account
	if err := xml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %%v", err)
	}
	if back.Privileges == nil || back.Privileges.JssObjects == nil || back.Privileges.JssObjects.Privilege == nil {
		t.Fatal("privileges.jss_objects decoded as nil")
	}
	got2 := *back.Privileges.JssObjects.Privilege
	if len(got2) != 3 {
		t.Fatalf("want 3 privileges after unmarshal, got %%d: %%v", len(got2), got2)
	}
	for i, want := range privs {
		if got2[i] != want {
			t.Fatalf("privilege[%%d]: want %%q, got %%q", i, want, got2[i])
		}
	}
}

// dataJARLDAPSFixture is a representative slice of the real
// DataJARLDAPS_JamfPro_Admins group GET response captured 2026-06-12
// (pro-nmartin). It carries ldap_server + three populated privilege
// categories (casper_* and recon are absent on this group — omitempty
// must leave them nil). Counts per category match the live dump.
var dataJARLDAPSFixture = []byte(`+"`"+`
<group>
  <id>8</id>
  <name>DataJARLDAPS_JamfPro_Admins</name>
  <access_level>Full Access</access_level>
  <privilege_set>Administrator</privilege_set>
  <ldap_server><id>31</id><name>ldap.datajar.mobi</name></ldap_server>
  <site><id>-1</id><name>NONE</name></site>
  <privileges>
    <jss_objects>
      <privilege>Create Cloud Distribution Point</privilege>
      <privilege>Read Cloud Distribution Point</privilege>
      <privilege>Update Cloud Distribution Point</privilege>
      <privilege>Create Custom Paths</privilege>
      <privilege>Read Custom Paths</privilege>
      <privilege>Update Custom Paths</privilege>
      <privilege>Delete Custom Paths</privilege>
    </jss_objects>
    <jss_settings>
      <privilege>Read AD CS Certificate Jobs</privilege>
      <privilege>Update AD CS Certificate Jobs</privilege>
      <privilege>Read SSO Settings</privilege>
      <privilege>Update SSO Settings</privilege>
      <privilege>Read Computer Inventory Collection Settings</privilege>
    </jss_settings>
    <jss_actions>
      <privilege>Allow User to Enroll</privilege>
      <privilege>Assign Users to Computers</privilege>
      <privilege>Assign Users to Mobile Devices</privilege>
      <privilege>Change Password</privilege>
    </jss_actions>
  </privileges>
  <members/>
</group>
`+"`"+`)

// TestGroup_WireFixture_LdapServerAndPrivileges unmarshal the real-dump
// fixture and asserts that:
//   - ldap_server.id == 31, ldap_server.name == "ldap.datajar.mobi"
//   - all three privilege categories survive with correct counts
//   - absent categories (casper_admin etc.) remain nil
//   - re-marshal produces ONE wrapper per populated category
func TestGroup_WireFixture_LdapServerAndPrivileges(t *testing.T) {
	var g Group
	if err := xml.Unmarshal(dataJARLDAPSFixture, &g); err != nil {
		t.Fatalf("unmarshal fixture: %%v", err)
	}

	// Gate C: ldap_server must survive.
	if g.LdapServer == nil {
		t.Fatal("LdapServer is nil after unmarshal")
	}
	if g.LdapServer.ID == nil || *g.LdapServer.ID != 31 {
		t.Fatalf("LdapServer.ID: want 31, got %%v", g.LdapServer.ID)
	}
	if g.LdapServer.Name == nil || *g.LdapServer.Name != "ldap.datajar.mobi" {
		t.Fatalf("LdapServer.Name: want ldap.datajar.mobi, got %%v", g.LdapServer.Name)
	}

	// Gate A: privilege counts must survive (no collapse).
	if g.Privileges == nil {
		t.Fatal("Privileges is nil after unmarshal")
	}
	checkPrivCount := func(label string, ptr *[]string, want int) {
		t.Helper()
		if ptr == nil {
			t.Fatalf("%%s: slice is nil, want %%d items", label, want)
		}
		if got := len(*ptr); got != want {
			t.Fatalf("%%s: want %%d privileges, got %%d: %%v", label, want, got, *ptr)
		}
	}
	if g.Privileges.JssObjects == nil {
		t.Fatal("Privileges.JssObjects is nil")
	}
	checkPrivCount("jss_objects", g.Privileges.JssObjects.Privilege, 7)
	if g.Privileges.JssSettings == nil {
		t.Fatal("Privileges.JssSettings is nil")
	}
	checkPrivCount("jss_settings", g.Privileges.JssSettings.Privilege, 5)
	if g.Privileges.JssActions == nil {
		t.Fatal("Privileges.JssActions is nil")
	}
	checkPrivCount("jss_actions", g.Privileges.JssActions.Privilege, 4)

	// Absent categories must stay nil (omitempty must not emit empty wrappers).
	if g.Privileges.CasperAdmin != nil {
		t.Fatal("CasperAdmin: want nil for absent category, got non-nil")
	}
	if g.Privileges.CasperImaging != nil {
		t.Fatal("CasperImaging: want nil for absent category, got non-nil")
	}
	if g.Privileges.CasperRemote != nil {
		t.Fatal("CasperRemote: want nil for absent category, got non-nil")
	}
	if g.Privileges.Recon != nil {
		t.Fatal("Recon: want nil for absent category, got non-nil")
	}

	// Re-marshal: each populated category must appear exactly once.
	out, err := xml.Marshal(g)
	if err != nil {
		t.Fatalf("re-marshal: %%v", err)
	}
	wire := string(out)
	for _, cat := range []string{"jss_objects", "jss_settings", "jss_actions"} {
		open := "<" + cat + ">"
		if n := strings.Count(wire, open); n != 1 {
			t.Fatalf("re-marshal: want 1 %%s wrapper, got %%d", open, n)
		}
	}
	// casper_admin must be absent from the re-marshalled output.
	if strings.Contains(wire, "<casper_admin>") {
		t.Fatal("re-marshal: casper_admin emitted for nil category")
	}

	// Round-trip: re-unmarshal the output and verify counts are stable.
	var g2 Group
	if err := xml.Unmarshal(out, &g2); err != nil {
		t.Fatalf("second unmarshal: %%v", err)
	}
	if g2.LdapServer == nil || g2.LdapServer.ID == nil || *g2.LdapServer.ID != 31 {
		t.Fatalf("second unmarshal LdapServer.ID: want 31, got %%v", g2.LdapServer)
	}
	if g2.Privileges == nil || g2.Privileges.JssObjects == nil {
		t.Fatal("second unmarshal: JssObjects nil")
	}
	checkPrivCount("jss_objects (2nd)", g2.Privileges.JssObjects.Privilege, 7)
}

// benTomsFixture is a representative slice of the real ben.toms@jamf.com
// directory account GET response captured 2026-06-12 (pro-nmartin).
// It exercises Account.Groups (group membership with inherited privileges),
// Account.LdapServer, and directory_user:true.
var benTomsFixture = []byte(`+"`"+`
<account>
  <id>66</id>
  <name>ben.toms@jamf.com</name>
  <directory_user>true</directory_user>
  <email>ben.toms@jamf.com</email>
  <email_address>ben.toms@jamf.com</email_address>
  <password_sha256/>
  <enabled>Enabled</enabled>
  <ldap_server><id>31</id><name>ldap.datajar.mobi</name></ldap_server>
  <access_level>Group Access</access_level>
  <privilege_set>Custom</privilege_set>
  <groups>
    <group>
      <id>18</id>
      <name>datajar.mobi Support</name>
      <site><id>-1</id><name>NONE</name></site>
      <privileges>
        <jss_objects>
          <privilege>Create Computers</privilege>
          <privilege>Read Computers</privilege>
          <privilege>Update Computers</privilege>
        </jss_objects>
        <jss_settings>
          <privilege>Read SSO Settings</privilege>
          <privilege>Read Password Policy</privilege>
        </jss_settings>
      </privileges>
    </group>
  </groups>
</account>
`+"`"+`)

// TestAccount_WireFixture_DirectoryUser asserts that a directory account
// round-trips correctly:
//   - LdapServer.ID == 31, LdapServer.Name == "ldap.datajar.mobi"
//   - DirectoryUser == true
//   - Groups contains exactly one group with id == 18
//   - Group privileges decoded without collapse (jss_objects=3, jss_settings=2)
//   - Write probe: re-marshal contains exactly ONE <groups> wrapper and
//     ONE <group> child; privilege counts stable after second unmarshal
func TestAccount_WireFixture_DirectoryUser(t *testing.T) {
	var a Account
	if err := xml.Unmarshal(benTomsFixture, &a); err != nil {
		t.Fatalf("unmarshal fixture: %%v", err)
	}

	if a.LdapServer == nil || a.LdapServer.ID == nil || *a.LdapServer.ID != 31 {
		t.Fatalf("LdapServer.ID: want 31, got %%v", a.LdapServer)
	}
	if a.LdapServer.Name == nil || *a.LdapServer.Name != "ldap.datajar.mobi" {
		t.Fatalf("LdapServer.Name: want ldap.datajar.mobi, got %%v", a.LdapServer.Name)
	}
	if a.DirectoryUser == nil || !*a.DirectoryUser {
		t.Fatalf("DirectoryUser: want true, got %%v", a.DirectoryUser)
	}
	if a.Groups == nil || a.Groups.Group == nil || len(*a.Groups.Group) != 1 {
		t.Fatalf("Groups: want 1 group, got %%v", a.Groups)
	}
	g0 := (*a.Groups.Group)[0]
	if g0.ID == nil || *g0.ID != 18 {
		t.Fatalf("Groups[0].ID: want 18, got %%v", g0.ID)
	}
	if g0.Name == nil || *g0.Name != "datajar.mobi Support" {
		t.Fatalf("Groups[0].Name: want datajar.mobi Support, got %%v", g0.Name)
	}
	if g0.Privileges == nil || g0.Privileges.JssObjects == nil || g0.Privileges.JssObjects.Privilege == nil {
		t.Fatal("Groups[0].Privileges.JssObjects is nil")
	}
	if got := len(*g0.Privileges.JssObjects.Privilege); got != 3 {
		t.Fatalf("Groups[0] jss_objects: want 3, got %%d", got)
	}
	if g0.Privileges.JssSettings == nil || g0.Privileges.JssSettings.Privilege == nil {
		t.Fatal("Groups[0].Privileges.JssSettings is nil")
	}
	if got := len(*g0.Privileges.JssSettings.Privilege); got != 2 {
		t.Fatalf("Groups[0] jss_settings: want 2, got %%d", got)
	}

	// Re-marshal and verify structure.
	out, err := xml.Marshal(a)
	if err != nil {
		t.Fatalf("re-marshal: %%v", err)
	}
	wire := string(out)
	if n := strings.Count(wire, "<groups>"); n != 1 {
		t.Fatalf("re-marshal: want 1 <groups> wrapper, got %%d", n)
	}
	if n := strings.Count(wire, "<group>"); n != 1 {
		t.Fatalf("re-marshal: want 1 <group> child, got %%d", n)
	}

	// Second unmarshal: confirm groups and privileges are stable.
	var a2 Account
	if err := xml.Unmarshal(out, &a2); err != nil {
		t.Fatalf("second unmarshal: %%v", err)
	}
	if a2.Groups == nil || a2.Groups.Group == nil || len(*a2.Groups.Group) != 1 {
		t.Fatalf("second unmarshal: Groups: want 1 group, got %%v", a2.Groups)
	}
	g0b := (*a2.Groups.Group)[0]
	if g0b.Privileges == nil || g0b.Privileges.JssObjects == nil || g0b.Privileges.JssObjects.Privilege == nil {
		t.Fatal("second unmarshal: Groups[0].Privileges.JssObjects is nil")
	}
	if got := len(*g0b.Privileges.JssObjects.Privilege); got != 3 {
		t.Fatalf("second unmarshal: Groups[0] jss_objects: want 3, got %%d", got)
	}
}
`, pkgName)
	outPath := filepath.Join(pkgDir, "accounts_privileges_test.go")
	formatted, err := formatGo("accounts_privileges_test.go", []byte(src))
	if err != nil {
		return fmt.Errorf("formatting accounts_privileges_test.go: %w\n---raw---\n%s", err, src)
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// emitPkgVersionLockHelpers writes version_lock_helpers.go into packages
// that have Apply methods with optimistic locking. The file provides two
// runtime helpers:
//   - zeroVersionLock(v any): recursively zeros all VersionLock int fields
//     on a struct pointer (used on create to satisfy the Jamf API requirement)
//   - convertAndInjectVersionLock[U, G any](src, current): JSON round-trips
//     the create request into the update type, then injects VersionLock
//     values from the GET response (used on update to satisfy optimistic locking)
func emitPkgVersionLockHelpers(pkgDir string, cfg Config, pkgName string) error {
	src := fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// zeroVersionLock recursively walks v (must be a pointer to a struct)
// and sets every field named "VersionLock" of type int to 0. This
// satisfies the Jamf API requirement that all versionLock fields be
// zero on resource creation.
func zeroVersionLock(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		ft := rt.Field(i)
		if ft.Name == "VersionLock" && f.Kind() == reflect.Int && f.CanSet() {
			f.SetInt(0)
			continue
		}
		switch f.Kind() {
		case reflect.Struct:
			zeroVersionLock(f.Addr().Interface())
		case reflect.Ptr:
			if !f.IsNil() && f.Elem().Kind() == reflect.Struct {
				zeroVersionLock(f.Interface())
			}
		}
	}
}

// convertAndInjectVersionLock converts a create request (src) to update
// type U via JSON round-trip, then injects all VersionLock field values
// from the current GET response (current) into the resulting update request.
// This implements the optimistic locking requirement: the update request
// must carry the current versionLock values so the server can detect
// concurrent modifications.
func convertAndInjectVersionLock[U any, G any](src any, current *G) (*U, error) {
	data, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("marshal for update: %%w", err)
	}
	var updateReq U
	if err := json.Unmarshal(data, &updateReq); err != nil {
		return nil, fmt.Errorf("unmarshal for update: %%w", err)
	}
	injectVersionLock(reflect.ValueOf(&updateReq).Elem(), reflect.ValueOf(current).Elem())
	return &updateReq, nil
}

// injectVersionLock recursively copies VersionLock field values from src
// into dst. Both must be struct values. Fields are matched by name; if a
// VersionLock field exists in both, the src value is copied to dst.
func injectVersionLock(dst, src reflect.Value) {
	if dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			return
		}
		dst = dst.Elem()
	}
	if src.Kind() == reflect.Ptr {
		if src.IsNil() {
			return
		}
		src = src.Elem()
	}
	if dst.Kind() != reflect.Struct || src.Kind() != reflect.Struct {
		return
	}
	dstType := dst.Type()
	for i := 0; i < dst.NumField(); i++ {
		df := dst.Field(i)
		dft := dstType.Field(i)
		if dft.Name == "VersionLock" && df.Kind() == reflect.Int && df.CanSet() {
			sf := src.FieldByName("VersionLock")
			if sf.IsValid() && sf.Kind() == reflect.Int {
				df.SetInt(sf.Int())
			}
			continue
		}
		sf := src.FieldByName(dft.Name)
		if !sf.IsValid() {
			continue
		}
		switch df.Kind() {
		case reflect.Struct:
			injectVersionLock(df, sf)
		case reflect.Ptr:
			if !df.IsNil() {
				injectVersionLock(df, sf)
			}
		}
	}
}
`, pkgName)
	outPath := filepath.Join(pkgDir, "version_lock_helpers.go")
	formatted, err := formatGo("version_lock_helpers.go", []byte(src))
	if err != nil {
		return fmt.Errorf("formatting version_lock_helpers.go: %w", err)
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("wrote %s", outPath)
	return nil
}

// emitMethodsByTag buckets methods by the filename their first OpenAPI tag
// maps to (post tagToFileBase normalization) and emits one source + test
// file per distinct filename. Operations without a tag error out —
// untagged ops in splitByTag mode signal a spec bug the curator should
// see. Bucketing by final filename (not raw tag) means two tags that
// normalize to the same base — e.g. `foo` + `foo-preview` after the
// -preview strip — merge into one file instead of the second overwriting
// the first.
func emitMethodsByTag(pkgDir string, cfg Config, pkgName string, spec SpecDef, methods []GoMethod) error {
	buckets := make(map[string][]GoMethod)
	for _, m := range methods {
		if m.Tag == "" {
			return fmt.Errorf("spec %s: operation %s has no OpenAPI tag but splitByTag is enabled", spec.File, m.Name)
		}
		base := tagToFileBase(m.Tag)
		buckets[base] = append(buckets[base], m)
	}

	bases := make([]string, 0, len(buckets))
	for b := range buckets {
		bases = append(bases, b)
	}
	sort.Strings(bases)

	for _, base := range bases {
		mf := GeneratedFile{Package: pkgName, Module: cfg.Module, Format: spec.Format, Methods: buckets[base]}
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
//
// Two post-processing rules:
//
//  1. Trailing "-preview" is stripped. Jamf's API team tags in-development
//     endpoints with "-preview"; when the endpoint graduates to stable the
//     tag loses the suffix and the filename would churn. Stripping at
//     generate-time keeps the SDK filename stable across that transition
//     and consolidates preview + stable variants of the same resource
//     (e.g. mobile-device-extension-attributes + *-preview) into one file.
//
//  2. Filenames ending in `_<goos>` or `_<goarch>` get `_api` appended so
//     the Go toolchain doesn't interpret them as implicit build constraints
//     — e.g. `self_service_branding_ios.go` would otherwise only compile
//     for GOOS=ios.
func tagToFileBase(tag string) string {
	s := strings.ToLower(strings.TrimSpace(tag))
	s = strings.TrimSuffix(s, "-preview")
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
	out := b.String()
	for _, suf := range reservedFileSuffixes {
		if strings.HasSuffix(out, "_"+suf) {
			return out + "_api"
		}
	}
	return out
}

// reservedFileSuffixes lists the GOOS and GOARCH values whose trailing
// use as `_<value>.go` would turn the whole file into a per-platform
// build-constrained source. Keep in sync with Go's build constraints.
var reservedFileSuffixes = []string{
	// GOOS
	"aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos",
	"ios", "js", "linux", "nacl", "netbsd", "openbsd", "plan9", "solaris",
	"wasip1", "windows", "zos",
	// GOARCH
	"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le",
	"mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm",
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

// goStringSliceLiteral renders ss as a Go source expression: "nil" for an
// empty slice, otherwise a []string composite literal. Used to emit the
// Scoped/Legacy fields of the per-package privilege registry.
func goStringSliceLiteral(ss []string) string {
	if len(ss) == 0 {
		return "nil"
	}
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = strconv.Quote(s)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

// emitPkgPrivileges writes permissions.go into a sub-package: a Privileges map
// keyed by method name plus a PrivilegesFor lookup helper. Only spec-derived
// methods (PrivilegesKnown) are included; synthetic resolver/apply methods are
// omitted because they compose multiple endpoints rather than mapping to one
// operation. Entries are emitted in method-name order for deterministic
// output so CI's git-diff check stays stable.
func emitPkgPrivileges(pkgDir string, cfg Config, pkgName string, methods []GoMethod) error {
	type entry struct {
		method GoMethod
		path   string
	}
	var entries []entry
	seen := make(map[string]bool)
	for _, m := range methods {
		if !m.PrivilegesKnown || seen[m.Name] {
			continue
		}
		seen[m.Name] = true
		path := "/" + m.Version + m.ResourcePath
		entries = append(entries, entry{method: m, path: path})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].method.Name < entries[j].method.Name })

	var b strings.Builder
	fmt.Fprintf(&b, `// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

import "%s/jamfplatform"

// Privileges maps each %s SDK method name to the Jamf API privileges it
// requires, sourced from the x-required-privileges vendor extensions in the
// Jamf OpenAPI specs. Methods that require no special privilege have an empty
// Scoped slice. Synthetic Resolve<X>ByName / Apply<X> methods are not present;
// document the privileges of the operations they call instead.
var Privileges = map[string]jamfplatform.MethodPrivileges{
`, pkgName, cfg.Module, pkgName)

	for _, e := range entries {
		m := e.method
		fmt.Fprintf(&b, "\t%s: {Method: %s, HTTPMethod: %s, Path: %s, Scoped: %s, Legacy: %s},\n",
			strconv.Quote(m.Name),
			strconv.Quote(m.Name),
			strconv.Quote(m.HTTPMethod),
			strconv.Quote(e.path),
			goStringSliceLiteral(m.ScopedPrivileges),
			goStringSliceLiteral(m.LegacyPrivileges),
		)
	}

	b.WriteString(`}

// PrivilegesFor returns the privilege metadata for the named SDK method and
// true when the method is present in the registry, or the zero value and
// false otherwise.
func PrivilegesFor(method string) (jamfplatform.MethodPrivileges, bool) {
	p, ok := Privileges[method]
	return p, ok
}
`)

	outPath := filepath.Join(pkgDir, "permissions.go")
	formatted, err := imports.Process(outPath, []byte(b.String()), &imports.Options{Comments: true})
	if err != nil {
		return fmt.Errorf("goimports %s: %w\n---raw---\n%s", outPath, err, b.String())
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
func emitPkgHelpersTest(pkgDir string, cfg Config, pkgName, format string) error {
	xmlHelpers := ""
	if format == "xml" {
		xmlHelpers = `

func writeXML(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	if body != "" {
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("writeXML: %v", err)
		}
	}
}`
	}
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
	// Disable inter-request pacing in unit tests so the per-method httptest
	// suite isn't throttled to the default 100ms cadence. The gate's own
	// behavior is covered in internal/client.
	clientOpts := append([]Option{jamfplatform.WithMinRequestInterval(0)}, opts...)
	base := jamfplatform.NewClient(srv.URL, "test-id", "test-secret", clientOpts...)
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

func ptrStr(s string) *string { return &s }%s
`, pkgName, cfg.Module, xmlHelpers)
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
// Static files
// ---------------------------------------------------------------------------

// formatGo runs goimports which handles both formatting and unused import removal.
func formatGo(filename string, src []byte) ([]byte, error) {
	return imports.Process(filename, src, &imports.Options{Comments: true})
}

// readJamfProAPIVersion returns info.version from the configured Jamf Pro
// spec (the entry with package "pro"). Generation MUST fail loud rather
// than bake a placeholder constant: the whole point of JamfProAPIVersion
// is build-time provenance, and "unknown" defeats that.
//
// Loads via resolveSpecPath so CI — which doesn't have access to the
// private testing/ source specs — transparently falls back to the
// already-published api/ spec, which carries the same info.version.
func readJamfProAPIVersion(root string, cfg Config) (string, error) {
	var proSpec SpecDef
	found := false
	for _, s := range cfg.Specs {
		if s.Package == "pro" {
			proSpec = s
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("readJamfProAPIVersion: no spec entry with package %q in config", "pro")
	}
	specPath, _, err := resolveSpecPath(root, cfg, proSpec)
	if err != nil {
		return "", fmt.Errorf("readJamfProAPIVersion: %w", err)
	}
	data, err := os.ReadFile(specPath)
	if err != nil {
		return "", fmt.Errorf("readJamfProAPIVersion: reading %s: %w", specPath, err)
	}
	var doc struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("readJamfProAPIVersion: parsing %s: %w", specPath, err)
	}
	if doc.Info.Version == "" {
		return "", fmt.Errorf("readJamfProAPIVersion: %s has empty info.version", specPath)
	}
	return doc.Info.Version, nil
}

func writeStaticFiles(root string, cfg Config) error {
	pkg := cfg.Package
	mod := cfg.Module

	jamfProAPIVersion, err := readJamfProAPIVersion(root, cfg)
	if err != nil {
		return err
	}

	staticFiles := map[string]string{
		"jamfplatform/version.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

// JamfProAPIVersion is the Jamf Pro release this SDK was generated from,
// read directly from the info.version field of the openapi-jpapi.json
// spec at generate time. Use it to log/report which API surface the
// linked SDK build targets.
const JamfProAPIVersion = %q
`, pkg, jamfProAPIVersion),
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

// APIResponseError is returned for any non-success HTTP status. Consumers
// should inspect it via AsAPIError plus the accessor methods
// (HasStatus/Details/FieldErrors/Summary) rather than string-matching
// the Error() output. Non-HTTP errors (denylist refusal, context
// cancellation, IO failures, etc.) surface as plain wrapped errors —
// format them with err.Error().
type APIResponseError = client.APIResponseError

// ErrorDetail is a single structured error entry parsed from an API response
// body. Consumers receive these via APIResponseError.Details() or
// APIResponseError.FieldErrors().
type ErrorDetail = client.Error

// AmbiguousMatchError is returned by Resolve<Resource>ByName methods when
// multiple resources share the requested name. Matches carries the IDs of
// all colliding resources so consumers can surface disambiguation options.
type AmbiguousMatchError = client.AmbiguousMatchError

// AsAPIError unwraps err and returns the underlying *APIResponseError if
// present, otherwise nil. Shorthand for errors.As that saves callers from
// managing the target pointer and importing the concrete error type.
func AsAPIError(err error) *APIResponseError {
	return client.AsAPIError(err)
}
`, pkg, mod),

		"jamfplatform/permissions.go": fmt.Sprintf(`// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package %s

// MethodPrivileges describes the Jamf API privileges required to call a
// generated SDK method. It is sourced from the x-required-privileges and
// x-required-privileges-legacy vendor extensions the Jamf OpenAPI specs
// attach to each operation.
//
// Each generated sub-package (pro, devices, devicegroups, proclassic, ...)
// exposes a package-level Privileges map keyed by method name plus a
// PrivilegesFor helper. Consumers that need to document the permissions a
// given operation requires — for example a Terraform provider emitting a
// table of required privileges per resource — look the method up there.
//
// Only methods built directly from a spec operation appear in the registry.
// Synthetic convenience methods (Resolve<X>ByName, Apply<X>) compose one or
// more underlying endpoints and are intentionally absent; document the
// privileges of the operations they call instead.
type MethodPrivileges struct {
	// Method is the generated Go method name, e.g. "CreateBuildingV1".
	Method string
	// HTTPMethod is the HTTP verb of the underlying endpoint, e.g. "POST".
	HTTPMethod string
	// Path is the endpoint's resource path relative to the tenant prefix,
	// e.g. "/buildings/{id}".
	Path string
	// Scoped lists the modern scoped privilege identifiers in
	// action:scope:resource form, e.g. "create:pro:buildings". An empty
	// slice means the endpoint requires no special privilege: any
	// authenticated API client may call it. Where more than one identifier
	// is present, the Jamf spec does not encode whether they are required
	// together (AND) or as alternatives (OR); consumers should present the
	// full set and avoid asserting a relationship.
	Scoped []string
	// Legacy lists the human-readable Jamf Pro privilege names, e.g.
	// "Create Buildings". It is populated for the Pro API family only —
	// other families do not publish legacy names. Legacy is NOT
	// index-aligned with Scoped: a single legacy name may correspond to
	// multiple scoped identifiers.
	Legacy []string
}
`, pkg),

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
	// Disable inter-request pacing in unit tests so the per-method httptest
	// suite isn't throttled to the default 100ms cadence. The gate's own
	// behavior is covered in internal/client.
	clientOpts := append([]Option{WithMinRequestInterval(0)}, opts...)
	c := NewClient(srv.URL, "test-id", "test-secret", clientOpts...)
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

func ptrStr(s string) *string { return &s }
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
