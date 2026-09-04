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
	got, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant", "environment"}), SpecDef{File: "s.yaml"})
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
	got, err := resolveScopeTypes(docWithScopeTypes([]string{"environment", "tenant"}), SpecDef{File: "s.yaml"})
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
	got, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant"}), spec)
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
	_, err := resolveScopeTypes(docWithScopeTypes([]string{"environment", "tenant"}), spec)
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
	_, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant", "galaxy"}), SpecDef{File: "s.yaml"})
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
	_, err := resolveScopeTypes(docWithScopeTypes(nil), SpecDef{File: "new.yaml"})
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
	got, err := resolveScopeTypes(docWithScopeTypes([]string{"tenant", "tenant"}), SpecDef{File: "s.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "ScopeTenant" {
		t.Fatalf("duplicates not collapsed: %v", got)
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
		raw, err := os.ReadFile("../../" + spec.File)
		if err != nil {
			t.Fatalf("%s: %v", spec.File, err)
		}
		// Only the root extension matters here, so decode the document
		// directly rather than through loadSpec: this test is about what the
		// spec declares, not about whether it resolves.
		var root struct {
			ScopeTypes []string `yaml:"x-scope-types"`
		}
		if err := yaml.Unmarshal(raw, &root); err != nil {
			t.Fatalf("%s: %v", spec.File, err)
		}

		scopes, err := resolveScopeTypes(docWithScopeTypes(root.ScopeTypes), spec)
		if err != nil {
			t.Errorf("%s: %v", spec.File, err)
			continue
		}
		if len(scopes) == 0 {
			t.Errorf("%s: resolved an empty scope set", spec.File)
		}
		t.Logf("%-44s %v", spec.File, scopes)
	}
}
