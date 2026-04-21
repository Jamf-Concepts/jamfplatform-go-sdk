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
	return openapi3.NewLoader().LoadFromFile(path)
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

	applySchemaAdditions(doc, spec.SchemaAdditions)
	hoistInlineObjects(doc)

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
	currentFieldOverrides = spec.FieldTypeOverrides
	types := extractTypes(doc, referencedSchemas, spec.Format)
	currentFieldOverrides = nil

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
		doc, err := loadSpec(ls.specPath, allowedOpsSet(ls.spec))
		if err == nil {
			applySchemaAdditions(doc, ls.spec.SchemaAdditions)
			hoistInlineObjects(doc)
		}
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
		currentFieldOverrides = spec.FieldTypeOverrides
		types := extractTypes(doc, refs, spec.Format)
		currentFieldOverrides = nil
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

	if err := emitPkgClient(pkgDir, cfg, pkgName); err != nil {
		return err
	}

	typesGF := GeneratedFile{Package: pkgName, Module: cfg.Module, Format: pkgFormat, Types: allTypes}
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
		mf := GeneratedFile{Package: pkgName, Module: cfg.Module, Format: sm.spec.Format, Methods: sm.methods}
		if err := emitTemplated(sourceTmpl, mf, filepath.Join(pkgDir, sm.baseName+".go")); err != nil {
			return err
		}
		if err := emitTemplated(testTmpl, mf, filepath.Join(pkgDir, sm.baseName+"_test.go")); err != nil {
			return err
		}
	}

	if err := emitPkgHelpersTest(pkgDir, cfg, pkgName, pkgFormat); err != nil {
		return err
	}
	if err := emitPkgXMLSupplements(pkgDir, pkgName, pkgFormat); err != nil {
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
		if err := emitPkgVersionLockHelpers(pkgDir, cfg, pkgName); err != nil {
			return err
		}
	}
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

// MarshalXML emits up to two <notification> elements: the bool form if
// Enabled is set, then the method form if Method is set. Omits both
// when neither is set.
func (n NotificationValue) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if n.Enabled != nil {
		if err := e.EncodeElement(strconv.FormatBool(*n.Enabled), start); err != nil {
			return err
		}
	}
	if n.Method != nil {
		if err := e.EncodeElement(*n.Method, start); err != nil {
			return err
		}
	}
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
