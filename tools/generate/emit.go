// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/imports"
)

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
