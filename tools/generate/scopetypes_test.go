// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// docWithScopeTypes builds a minimal document carrying the given root
// x-scope-types, or none at all when values is nil.
func docWithScopeTypes(values []string) *openapi3.T {
	doc := &openapi3.T{}
	if values == nil {
		return doc
	}
	items := make([]any, len(values))
	for i, v := range values {
		items[i] = v
	}
	doc.Extensions = map[string]any{"x-scope-types": items}
	return doc
}

func TestResolveScopeTypesReadsTheSpecRoot(t *testing.T) {
	got, _, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant", "environment"}), SpecDef{File: "s.yaml"})
	if err != nil {
		t.Fatalf("resolveScopeTypes: %v", err)
	}
	want := []string{"ScopeTenant", "ScopeEnvironment"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// The order the spec declares is preserved: a consumer rendering the set reads
// it in the order upstream lists it, and "tenant, environment" is the order
// every dual-scope spec uses.
func TestResolveScopeTypesPreservesDeclarationOrder(t *testing.T) {
	got, _, err := resolveScopeTypes(docWithScopeTypes([]string{"environment", "tenant"}), SpecDef{File: "s.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "ScopeEnvironment" || got[1] != "ScopeTenant" {
		t.Fatalf("order not preserved: %v", got)
	}
}

// A held spec can understate the scopes the gateway serves, which is what
// config.scopeTypes is for. securitycloud-device-groups-api.yaml is the live
// case: pinned at v1897 declaring [tenant] while v2082 declares
// [tenant, environment] for the same operations.
func TestResolveScopeTypesConfigOverridesAnUnderstatedSpec(t *testing.T) {
	spec := SpecDef{File: "held.yaml", ScopeTypes: []string{"tenant", "environment"}}
	got, _, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant"}), spec)
	if err != nil {
		t.Fatalf("resolveScopeTypes: %v", err)
	}
	if len(got) != 2 || got[0] != "ScopeTenant" || got[1] != "ScopeEnvironment" {
		t.Fatalf("override not applied: %v", got)
	}
}

// The override is self-expiring. Once the ingested spec declares the same set
// the entry is redundant, and generation says so rather than letting a stale
// override outlive the repair it stood in for. This is the check that makes
// "delete the config entry when upstream fixes it" not depend on anyone
// remembering.
func TestResolveScopeTypesOverrideExpiresWhenTheSpecCatchesUp(t *testing.T) {
	spec := SpecDef{File: "held.yaml", ScopeTypes: []string{"tenant", "environment"}}

	// Same set, declared in the other order: still redundant, because the
	// comparison is on sets rather than sequences.
	_, _, err := resolveScopeTypes(docWithScopeTypes([]string{"environment", "tenant"}), spec)
	if err == nil {
		t.Fatal("want an error once the spec declares the same set")
	}
	for _, want := range []string{"held.yaml", "delete the config entry"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error omits %q: %v", want, err)
		}
	}
}

// An unknown value is refused rather than skipped. Dropping it silently would
// understate the scopes an endpoint accepts, which is the direction that
// misleads a consumer into thinking their credential cannot reach it.
func TestResolveScopeTypesRejectsAnUnknownKind(t *testing.T) {
	_, _, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant", "galaxy"}), SpecDef{File: "s.yaml"})
	if err == nil {
		t.Fatal("want an error for an unknown scope kind")
	}
	if !strings.Contains(err.Error(), `unknown scope kind "galaxy"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A spec declaring nothing, with no override, fails generation. The registry
// must never carry an empty Scopes slice: a consumer would read it as "no
// scope required", which is never true. The account trio is the only family
// with no x-scope-types and it carries an explicit organization entry, so a
// new spec arriving without the extension lands here rather than shipping a
// silent gap.
func TestResolveScopeTypesRefusesAnUndeclaredSpec(t *testing.T) {
	_, _, err := resolveScopeTypes(docWithScopeTypes(nil), SpecDef{File: "new.yaml"})
	if err == nil {
		t.Fatal("want an error when neither the spec nor config declares a scope")
	}
	if !strings.Contains(err.Error(), "declares no x-scope-types") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Duplicates in a declaration collapse rather than emitting the same constant
// twice into the registry literal.
func TestResolveScopeTypesDedupes(t *testing.T) {
	got, _, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant", "tenant"}), SpecDef{File: "s.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "ScopeTenant" {
		t.Fatalf("duplicates not collapsed: %v", got)
	}
}

// docWithRawScopeTypes builds a document whose x-scope-types carries raw
// verbatim, so a malformed declaration can be exercised: docWithScopeTypes
// takes []string and so can only ever produce a well-formed one.
func docWithRawScopeTypes(raw any) *openapi3.T {
	return &openapi3.T{Extensions: map[string]any{"x-scope-types": raw}}
}

// The provenance travels with the set, because the registry publishes it and a
// consumer reading one entry has to be able to tell an ingested declaration
// from a correction this repo made. The Scopes godoc used to assert spec
// provenance unconditionally, which was false for the 25 methods — the account
// trio's 18 and securitycloud-device-groups' 7 — whose scopes come from
// config.scopeTypes.
func TestResolveScopeTypesReportsProvenance(t *testing.T) {
	_, source, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant"}), SpecDef{File: "s.yaml"})
	if err != nil {
		t.Fatalf("resolveScopeTypes: %v", err)
	}
	if source != "spec" {
		t.Fatalf("root extension supplied the set, got source %q, want %q", source, "spec")
	}

	spec := SpecDef{File: "held.yaml", ScopeTypes: []string{"tenant", "environment"}}
	_, source, err = resolveScopeTypes(docWithScopeTypes([]string{"tenant"}), spec)
	if err != nil {
		t.Fatalf("resolveScopeTypes: %v", err)
	}
	if source != "config-override" {
		t.Fatalf("config supplied the set, got source %q, want %q", source, "config-override")
	}
}

// The other half of the self-expiry, and the half exact equality cannot see.
// An override exists to widen a spec that understates the gateway, so a spec
// declaring anything the override omits means the override has gone stale in
// the direction that loses information: replacing the spec's set with a subset
// silently drops a scope upstream now publishes, and the registry then tells
// consumers an endpoint refuses a credential it accepts.
func TestResolveScopeTypesRefusesAnOverrideTheSpecHasOutgrown(t *testing.T) {
	spec := SpecDef{File: "held.yaml", ScopeTypes: []string{"tenant"}}

	_, _, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant", "environment"}), spec)
	if err == nil {
		t.Fatal("want an error once the spec declares a scope the override omits")
	}
	for _, want := range []string{"held.yaml", "does not cover"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error omits %q: %v", want, err)
		}
	}
}

// A malformed declaration is refused rather than partially read. The extractor
// used to append only the elements that type-asserted to string, so
// ["tenant", true] extracted as ["tenant"] and generated a plausible
// single-scope registry entry — the unknown-kind guard never saw the bad value,
// because it only ever inspects what survived extraction as a Go string.
func TestResolveScopeTypesRefusesANonStringElement(t *testing.T) {
	_, _, err := resolveScopeTypes(docWithRawScopeTypes([]any{"tenant", true}), SpecDef{File: "s.yaml"})
	if err == nil {
		t.Fatal("want an error for a non-string element")
	}
	for _, want := range []string{"s.yaml", "not a string"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error omits %q: %v", want, err)
		}
	}

	// A scalar where an array belongs is the same class of fault and gets the
	// same treatment: returning nil for it would route a malformed spec into
	// the "declares nothing" refusal, which names the wrong repair.
	_, _, err = resolveScopeTypes(docWithRawScopeTypes("tenant"), SpecDef{File: "s.yaml"})
	if err == nil {
		t.Fatal("want an error for a non-array extension value")
	}
	if !strings.Contains(err.Error(), "want an array of strings") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// An absent extension must stay a nil slice with no error, so a spec that
// declares nothing still lands on the "declares no x-scope-types" refusal that
// names the config.scopeTypes repair — and so the config-override path, which
// runs after extraction, is still reached at all. Tightening extraction must
// not turn absence into a malformed-declaration error.
func TestResolveScopeTypesAbsentExtensionKeepsItsOwnRefusal(t *testing.T) {
	_, _, err := resolveScopeTypes(docWithScopeTypes(nil), SpecDef{File: "new.yaml"})
	if err == nil {
		t.Fatal("want an error when neither the spec nor config declares a scope")
	}
	if !strings.Contains(err.Error(), "declares no x-scope-types") {
		t.Fatalf("absence took the malformed-declaration path: %v", err)
	}

	// And the override still applies over an absent declaration, which is the
	// account trio's case: no published build carries the extension, so
	// extraction returns nothing and config supplies the whole set.
	spec := SpecDef{File: "account-sso-api.yaml", ScopeTypes: []string{"organization"}}
	got, source, err := resolveScopeTypes(docWithScopeTypes(nil), spec)
	if err != nil {
		t.Fatalf("resolveScopeTypes: %v", err)
	}
	if len(got) != 1 || got[0] != "ScopeOrganization" || source != "config-override" {
		t.Fatalf("override over an absent declaration: got %v from %q", got, source)
	}
}

// The literal is emitted into generated source, so its exact shape is the
// contract: identifiers rather than quoted strings, package-qualified, and
// "nil" for the empty case. The empty case is unreachable through
// resolveScopeTypes, which is precisely why it needs pinning here — nothing
// else exercises it.
func TestGoScopeKindSliceLiteral(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
		want  string
	}{
		{"empty", nil, "nil"},
		{"one", []string{"ScopeEnvironment"}, "[]jamfplatform.ScopeKind{jamfplatform.ScopeEnvironment}"},
		{"two", []string{"ScopeTenant", "ScopeEnvironment"}, "[]jamfplatform.ScopeKind{jamfplatform.ScopeTenant, jamfplatform.ScopeEnvironment}"},
		{"three", []string{"ScopeTenant", "ScopeEnvironment", "ScopeOrganization"}, "[]jamfplatform.ScopeKind{jamfplatform.ScopeTenant, jamfplatform.ScopeEnvironment, jamfplatform.ScopeOrganization}"},
	} {
		if got := goScopeKindSliceLiteral(tc.names); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

// Every spec in config.json must resolve a non-empty scope set. This is the
// tripwire for the whole feature: an ingest that drops x-scope-types from a
// spec fails here rather than quietly emitting a registry that tells consumers
// an endpoint needs no scope.
func TestEveryConfiguredSpecResolvesAScope(t *testing.T) {
	data, err := os.ReadFile("config.json")
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing config.json: %v", err)
	}

	for _, spec := range cfg.Specs {
		// Resolve the spec the way the generator does rather than reaching
		// straight for testing/: that directory is gitignored, so in CI the
		// only copy is the published api/*.json and a hardcoded testing/ path
		// fails there and nowhere else. resolveSpecPath is the same function
		// main.go uses, so this test cannot drift from the real fallback.
		specPath, _, err := resolveSpecPath("../..", cfg, spec)
		if err != nil {
			t.Fatalf("%s: resolveSpecPath: %v", spec.File, err)
		}
		raw, err := os.ReadFile(specPath)
		if err != nil {
			t.Fatalf("%s: %v", specPath, err)
		}
		// Only the root extension matters here, so decode the document
		// directly rather than through loadSpec: this test is about what the
		// spec declares, not about whether it resolves. YAML is a JSON
		// superset, so one decoder reads both the verbatim testing/ YAML and
		// the published api/ JSON.
		var root struct {
			ScopeTypes []string `yaml:"x-scope-types"`
		}
		if err := yaml.Unmarshal(raw, &root); err != nil {
			t.Fatalf("%s: %v", specPath, err)
		}

		scopes, source, err := resolveScopeTypes(docWithScopeTypes(root.ScopeTypes), spec)
		if err != nil {
			t.Errorf("%s: %v", spec.File, err)
			continue
		}
		if len(scopes) == 0 {
			t.Errorf("%s: resolved an empty scope set", spec.File)
		}
		if source == "" {
			t.Errorf("%s: resolved a scope set with no provenance", spec.File)
		}
		t.Logf("%-44s %-12s %v", spec.File, source, scopes)
	}
}
