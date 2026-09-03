// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// privilegeSetsAreNotPairs documents, in one place, why the scoped and legacy
// privilege arrays must never be zipped and why the legacy one is deliberately
// left unsorted. The tests below assert both halves against the real spec, so
// the reasoning cannot be lost to a well-meaning cleanup.
//
// A downstream consumer (terraform-provider-jamfplatform) rendered a "Required
// Jamf privileges" table by pairing Scoped[i] with Legacy[i]. That shipped two
// swapped labels for RedeployJamfManagementFrameworkV1 before anyone noticed,
// and wrong privilege guidance sends an operator to grant the opposite
// privilege. The pairing is not merely mis-ordered — it does not exist.
const privilegeSetsAreNotPairs = `
Scoped and Legacy are independent sets:

  - 29 pro operations have different lengths, because the GA capability
    consolidation mapped several legacy privileges onto one capability. There is
    no bijection to zip.
  - 9 of the 24 equal-length multi-privilege operations disagree on order in the
    spec itself, so even a length check does not make a zip safe.

Sorting the legacy array would be the wrong repair. Upstream already ships every
scoped array alphabetical, so sorting legacy too would make an incorrect
positional pairing come out right on 23 of those 24 operations instead of 16 —
turning a bug that is visibly wrong on nine operations into one that is silently
wrong on one. The visible disorder is the only signal a consumer gets.`

// specPrivileges returns (scoped, legacy) per "METHOD /path" from a spec file,
// for operations that declare either extension.
//
// It reads the *published* api/pro_api.json rather than testing/openapi-jpapi.json
// so the assertions run in CI: testing/ is gitignored, and a skipped tripwire
// protects nothing. The published spec is filtered to the whitelisted surface,
// so its counts are lower than the source spec's — the assertions are therefore
// written as "at least one", not as exact totals.
func specPrivileges(t *testing.T, path string) map[string][2][]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	// A path item mixes method keys with siblings like "parameters", so each
	// value has to be decoded lazily rather than into one struct.
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	methods := map[string]bool{"get": true, "put": true, "post": true, "delete": true, "patch": true}
	out := map[string][2][]string{}
	for p, item := range doc.Paths {
		for m, rawOp := range item {
			if !methods[strings.ToLower(m)] {
				continue
			}
			var op struct {
				Scoped []string `json:"x-required-privileges"`
				Legacy []string `json:"x-required-privileges-legacy"`
			}
			if err := json.Unmarshal(rawOp, &op); err != nil {
				t.Fatalf("parsing %s %s: %v", m, p, err)
			}
			if len(op.Scoped) == 0 && len(op.Legacy) == 0 {
				continue
			}
			out[strings.ToUpper(m)+" "+p] = [2][]string{op.Scoped, op.Legacy}
		}
	}
	return out
}

func proSpecPath() string { return filepath.Join("..", "..", "api", "pro_api.json") }

// TestPrivilegeSetsAreNotIndexAligned asserts the two facts that make a
// positional pairing impossible. If either count drops to zero the hazard has
// gone away upstream and the contract doc can be relaxed — verify that
// deliberately rather than deleting this test.
func TestPrivilegeSetsAreNotIndexAligned(t *testing.T) {
	privs := specPrivileges(t, proSpecPath())
	if len(privs) == 0 {
		t.Fatal("no privileged operations found — the extension names have changed")
	}

	var lengthMismatch, orderMismatch int
	for _, sl := range privs {
		scoped, legacy := sl[0], sl[1]
		if len(legacy) == 0 {
			continue
		}
		if len(scoped) != len(legacy) {
			lengthMismatch++
			continue
		}
		if len(scoped) > 1 && !verbOrdersAgree(scoped, legacy) {
			orderMismatch++
		}
	}
	t.Logf("privileged ops: %d; length mismatches: %d; equal-length order mismatches: %d",
		len(privs), lengthMismatch, orderMismatch)

	if lengthMismatch == 0 {
		t.Errorf("expected operations whose scoped and legacy privilege counts differ; found none.%s",
			privilegeSetsAreNotPairs)
	}
	if orderMismatch == 0 {
		t.Errorf("expected equal-length operations whose scoped and legacy orders disagree; found none.%s",
			privilegeSetsAreNotPairs)
	}
}

// TestLegacyPrivilegesAreNotSorted is the tripwire for the cleanup that looks
// obviously right and is not: sorting the legacy array for stable diffs.
//
// It reads the generated jamfplatform/pro/permissions.go, not a spec. That
// distinction is the whole point and cost one wrong attempt: publishSpecs copies
// the extension values straight from the source spec, so a sort applied in the
// generator reaches permissions.go and the method godoc but never the published
// spec. A spec-reading test therefore cannot see the change it exists to catch.
func TestLegacyPrivilegesAreNotSorted(t *testing.T) {
	path := filepath.Join("..", "..", "jamfplatform", "pro", "permissions.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	legacyLiteral := regexp.MustCompile(`Legacy: \[\]string\{([^}]*)\}`)
	matches := legacyLiteral.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("no Legacy literals found in the generated registry — the emitted shape has changed")
	}

	multi, unsorted := 0, 0
	for _, m := range matches {
		names := splitGoStringLiteralList(m[1])
		if len(names) < 2 {
			continue
		}
		multi++
		if !sort.StringsAreSorted(names) {
			unsorted++
		}
	}
	t.Logf("generated registry: %d Legacy entries, %d with >1 name, %d deliberately not alphabetical",
		len(matches), multi, unsorted)
	if multi == 0 {
		t.Fatal("no multi-name Legacy entries — cannot tell whether sorting was introduced")
	}
	if unsorted == 0 {
		t.Errorf("every multi-name Legacy entry in the generated registry is alphabetical. "+
			"Either upstream started sorting them, or the generator began sorting them here. "+
			"The generator must NOT.%s", privilegeSetsAreNotPairs)
	}
}

// TestSpecLegacyArraysStillUnsorted is the upstream half: if Jamf ever ships the
// legacy arrays sorted, the signal described in privilegeSetsAreNotPairs is gone
// and consumers lose their only clue that the lists are not parallel. Worth
// knowing about, and worth reporting rather than silently absorbing.
func TestSpecLegacyArraysStillUnsorted(t *testing.T) {
	privs := specPrivileges(t, proSpecPath())
	unsorted := 0
	for _, sl := range privs {
		if legacy := sl[1]; len(legacy) >= 2 && !sort.StringsAreSorted(legacy) {
			unsorted++
		}
	}
	if unsorted == 0 {
		t.Errorf("upstream now ships every legacy privilege array alphabetical. The visible disorder "+
			"was the only signal a consumer got that the two lists are not pairs.%s", privilegeSetsAreNotPairs)
	}
	t.Logf("%d legacy arrays in the published spec are not alphabetical", unsorted)
}

// splitGoStringLiteralList splits the body of a Go []string{...} literal into
// its unquoted elements. Deliberately minimal: the generated literals are
// produced by goStringSliceLiteral and contain no escaped quotes.
func splitGoStringLiteralList(body string) []string {
	var out []string
	for part := range strings.SplitSeq(body, ",") {
		part = strings.TrimSpace(part)
		if len(part) >= 2 && strings.HasPrefix(part, `"`) && strings.HasSuffix(part, `"`) {
			out = append(out, part[1:len(part)-1])
		}
	}
	return out
}

// TestPrivilegeCommentWarnsAgainstPairing pins the emitted godoc: wherever a
// reader sees two comma-joined lists side by side, the line has to say they are
// not pairs.
func TestPrivilegeCommentWarnsAgainstPairing(t *testing.T) {
	const warning = "independent sets, not pairs"
	cases := []struct {
		name       string
		scoped     []string
		legacy     []string
		wantWarn   bool
		wantAbsent string
	}{
		{
			name:     "multiple scoped with legacy warns",
			scoped:   []string{"computer-check-in:read", "device-actions:execute"},
			legacy:   []string{"Send Computer Remote Command to Install Package", "Read Computer Check-In"},
			wantWarn: true,
		},
		{
			name:     "one scoped but several legacy warns",
			scoped:   []string{"device-groups:read"},
			legacy:   []string{"Read Smart Computer Groups", "Read Static Computer Groups"},
			wantWarn: true,
		},
		{
			name:     "single pair needs no warning",
			scoped:   []string{"buildings:create"},
			legacy:   []string{"Create Buildings"},
			wantWarn: false,
		},
		{
			name:     "no legacy names, nothing to mispair",
			scoped:   []string{"ztna:read", "ztna:update"},
			legacy:   nil,
			wantWarn: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := privilegeComment(GoMethod{
				PrivilegesKnown:  true,
				ScopedPrivileges: tc.scoped,
				LegacyPrivileges: tc.legacy,
			})
			if strings.Contains(got, warning) != tc.wantWarn {
				t.Errorf("warning present = %v, want %v\ngot: %s",
					strings.Contains(got, warning), tc.wantWarn, got)
			}
		})
	}
}

// verbOrdersAgree reports whether the action verbs of the scoped identifiers
// line up positionally with the leading verbs of the legacy names. It is only
// a heuristic for spotting disagreement in the test above — never a pairing.
func verbOrdersAgree(scoped, legacy []string) bool {
	legacyVerb := map[string]string{
		"read": "read", "view": "read", "create": "create", "update": "update",
		"delete": "delete", "send": "execute", "change": "execute",
		"flush": "execute", "assign": "update",
	}
	for i := range scoped {
		parts := strings.Split(scoped[i], ":")
		want := strings.ToLower(parts[len(parts)-1])
		first := strings.ToLower(strings.SplitN(legacy[i], " ", 2)[0])
		if legacyVerb[first] != want {
			return false
		}
	}
	return true
}

// generatedScopedPrivileges returns every distinct Scoped identifier in every
// generated per-package registry, mapped to the packages it appears in.
//
// It reads the generated registries rather than the specs for the same reason
// TestLegacyPrivilegesAreNotSorted does: the registry is what a consumer
// actually reads, it is committed so CI can see it, and testing/ is gitignored.
func generatedScopedPrivileges(t *testing.T) map[string][]string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "..", "jamfplatform", "*", "permissions.go"))
	if err != nil {
		t.Fatalf("globbing generated registries: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("no generated permissions.go found — the emitted layout has changed")
	}
	scopedLiteral := regexp.MustCompile(`Scoped: \[\]string\{([^}]*)\}`)
	out := map[string][]string{}
	for _, f := range dirs {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		pkg := filepath.Base(filepath.Dir(f))
		for _, m := range scopedLiteral.FindAllStringSubmatch(string(raw), -1) {
			for _, id := range splitGoStringLiteralList(m[1]) {
				out[id] = append(out[id], pkg)
			}
		}
	}
	return out
}

// TestScopedPrivilegesUseGAVocabulary asserts every generated identifier is a
// {capability}:{action} pair the permissions-map article declares.
//
// Three failure modes it catches, none of which has any other tripwire:
// a mistyped capability or action in an ingested spec; an action applied to a
// capability that does not offer it (compliance-benchmarks:update, say, which
// the article says does not exist); and a regression to the retired three-part
// beta slug, which would silently change the meaning of every consumer's table.
func TestScopedPrivilegesUseGAVocabulary(t *testing.T) {
	ids := generatedScopedPrivileges(t)
	if len(ids) == 0 {
		t.Fatal("no Scoped identifiers found in the generated registries")
	}
	declared, err := loadPermissionsMap()
	if err != nil {
		t.Fatal(err)
	}

	r := checkAgainstPermissionsMap(declared, ids)
	t.Logf("%d distinct scoped identifiers across %d capabilities, checked against %d the "+
		"published map declares", r.Identifiers, r.Capabilities, len(declared))

	if len(r.BadForm) > 0 {
		t.Errorf("identifiers are not {capability}:{action}: %s\n"+
			"The three-part beta slug (create:pro:buildings) is retired; a return to it changes "+
			"the meaning of every consumer's permissions table.", strings.Join(r.BadForm, ", "))
	}
	if len(r.UnknownCapability) > 0 {
		t.Errorf("capabilities the published permissions map does not declare: %s\n"+
			"A new capability upstream is a legitimate and frequent event, so this failing is "+
			"the notification, not an accusation: run `make permmap` to refresh the snapshot. "+
			"If the refreshed map still omits it, the disagreement is real — wire-probe the "+
			"endpoint, report it upstream, and record it in permissionsMapExceptions with the "+
			"evidence. Do not delete this assertion to make an ingest pass.",
			strings.Join(r.UnknownCapability, ", "))
	}
	if len(r.UnknownAction) > 0 {
		t.Errorf("actions the map does not grant on that capability: %s\n"+
			"Either the spec is wrong (report upstream, do not patch locally) or the map's "+
			"action set for that capability has widened — `make permmap` settles which.",
			strings.Join(r.UnknownAction, ", "))
	}
	if len(r.ExpiredExceptions) > 0 {
		t.Errorf("permissionsMapExceptions has expired entr(y/ies): %s\n"+
			"Either the published map now declares the permission, or the SDK no longer emits "+
			"it. Delete the entry from permissionsMapExceptions in permmap.go — do not work "+
			"around this.", strings.Join(r.ExpiredExceptions, ", "))
	}

	// Coverage, the other direction. Not a defect on its own — the map covers
	// products this SDK does not — but it is the number to quote when
	// reporting a gap, and a capability going unreached can mean an operation
	// the whitelist has not caught up with.
	t.Logf("the map declares %d capabilit(y/ies) the SDK does not reach [%s]",
		len(r.UnreachedCapabilities), strings.Join(r.UnreachedCapabilities, " "))
	t.Logf("and %d further action(s) on capabilities it does reach [%s]",
		len(r.UnusedActions), strings.Join(r.UnusedActions, " "))
}
