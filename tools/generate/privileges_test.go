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
	for _, part := range strings.Split(body, ",") {
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

// gaCapabilityActions is the closed vocabulary of GA capability permissions,
// transcribed from Jamf's "Jamf Pro permissions map" documentation article
// (capability reference section, action codes c/r/u/d/dep/x). It is the only
// published artefact that enumerates them.
//
// It exists so that a capability or action arriving from a spec ingest cannot
// land silently in the generated registry — and from there in a consumer's
// permissions table — without a human checking it against the article. A new
// capability is a legitimate and frequent upstream event; the test failing is
// the notification, not an accusation. Extend the table in the same change that
// ingests the spec.
//
// The table is a superset of what the SDK generates: it includes the Jamf
// Protect capabilities (reached through the Protect GraphQL API, not covered
// here) and a handful the whitelist does not yet reach, so ingesting one of
// those does not produce a spurious failure.
var gaCapabilityActions = map[string][]string{
	// Organization management scope.
	"licensing": {"read"}, "deal-registration": {"create", "read"},
	"distributor-actions": {"create", "read", "update"},
	"sso-connections":     {"create", "read", "update", "delete"},
	"sso-domains":         {"create", "read", "update", "delete"},
	// Inventory.
	"devices": {"create", "read", "update", "delete"}, "device-groups": {"create", "read", "update", "delete"},
	"users": {"create", "read", "update", "delete"}, "user-groups": {"create", "read", "update", "delete"},
	"extension-attributes":      {"create", "read", "update", "delete"},
	"user-extension-attributes": {"create", "read", "update", "delete"},
	"advanced-device-searches":  {"create", "read", "update", "delete"},
	"advanced-user-searches":    {"create", "read", "update", "delete"},
	"device-history":            {"read"},
	// Organizational context.
	"sites": {"create", "read", "update", "delete"}, "buildings": {"create", "read", "update", "delete"},
	"departments": {"create", "read", "update", "delete"}, "categories": {"create", "read", "update", "delete"},
	"classes": {"create", "read", "update", "delete"}, "network-segments": {"create", "read", "update", "delete"},
	"ibeacon": {"create", "read", "update", "delete"},
	// Device actions. The split is deliberate: erase, unmanage and remove-MDM
	// destroy data or end management, so they sit under their own capability.
	"device-actions": {"read", "delete", "execute"}, "destructive-device-actions": {"execute"},
	// Device secrets.
	"disk-encryption-recovery-key": {"read"}, "recovery-lock": {"read"},
	"computer-device-lock-pin": {"read"}, "local-admin-passwords": {"read", "update", "execute"},
	// Deployment.
	"blueprints": {"create", "read", "update", "delete", "deploy"}, "declarations": {"read"},
	"configuration-profiles": {"create", "read", "update", "delete"},
	"policies":               {"create", "read", "update", "delete"}, "scripts": {"create", "read", "update", "delete"},
	"packages": {"create", "read", "update", "delete"}, "printers": {"create", "read", "update", "delete"},
	"dock-items": {"create", "read", "update", "delete"}, "managed-software-updates": {"create", "read", "update"},
	"disk-encryption-configurations": {"create", "read", "update", "delete"},
	"directory-bindings":             {"create", "read", "update", "delete"},
	"jamf-connect-deployments":       {"read", "update", "deploy"},
	"jamf-protect-deployments":       {"read", "update", "deploy"},
	// Enrollment.
	"prestage-enrollments":   {"create", "read", "update", "delete"},
	"enrollment-profiles":    {"create", "read", "update", "delete"},
	"enrollment-invitations": {"create", "read", "update", "delete"},
	"activation-profiles":    {"create", "read", "update", "delete"},
	// App lifecycle management.
	"applications": {"create", "read", "update", "delete"}, "jamf-packages-action": {"read"},
	"volume-purchasing-locations":      {"create", "read", "update", "delete"},
	"ebooks":                           {"create", "read", "update", "delete"},
	"provisioning-profiles":            {"create", "read", "update", "delete"},
	"licensed-software":                {"create", "read", "update", "delete"},
	"restricted-software":              {"create", "read", "update", "delete"},
	"patch-policies":                   {"create", "read", "update", "delete"},
	"patch-management-software-titles": {"create", "read", "update", "delete"},
	"patch-external-source":            {"create", "read", "update", "delete"},
	"patch-internal-source":            {"read"},
	// Compliance. compliance-benchmarks has no update action.
	"ai-policies":                   {"create", "read", "update", "delete"},
	"compliance-benchmarks":         {"create", "read", "delete"},
	"device-compliance-information": {"read"},
	// Endpoint security — Jamf Protect GraphQL, not generated here.
	"protection-plans": {"read", "update"}, "detection-analytics": {"read", "update"},
	"threat-alerts": {"read", "update"}, "prevent-lists": {"create", "read", "update", "delete"},
	"threat-definition-versions": {"read"},
	"unified-logging-filters":    {"create", "read", "update", "delete"},
	"security-audit-log":         {"read"},
	// Secure enterprise access.
	"ztna": {"create", "read", "update", "delete"}, "search-domains": {"read", "update", "delete"},
	"custom-hostname-mappings": {"read", "update", "delete"}, "content-categories": {"read"},
	// Admin identity and access.
	"audit": {"read"}, "accounts": {"create", "read", "update", "delete"},
	"change-password": {"execute"}, "account-groups": {"read"},
	"ldap-servers": {"create", "read", "update", "delete"}, "sso-settings": {"read", "update"},
	"access-management": {"read", "update"}, "user-sessions": {"read"},
	// Admin file uploads.
	"file-uploads": {"create"},
	// Global settings.
	"uem-connect": {"create", "read", "update", "delete"}, "conditional-access": {"read"},
	"self-service": {"create", "read", "update", "delete"}, "app-request": {"read", "update"},
	"onboarding": {"read", "update"}, "re-enrollment": {"read", "update"},
	"return-to-service": {"read", "update", "delete"}, "user-initiated-enrollment": {"read", "update"},
	"apple-configurator-enrollment": {"read", "update"},
	"enrollment-customization":      {"create", "read", "update", "delete"},
	"teacher-app":                   {"read", "update"}, "parent-app": {"read", "update"},
	"remote-assist": {"read"}, "remote-administration": {"create", "read", "update", "delete"},
	"computer-check-in":                      {"read", "update"},
	"computer-inventory-collection-settings": {"read", "update"},
	"custom-paths":                           {"create", "delete"},
	"removable-mac-address":                  {"create", "read", "update", "delete"},
	"inventory-preload-records":              {"create", "read", "update", "delete"},
	"mdm-profile-renewal-settings":           {"read", "update"},
	"impact-alert-notification-settings":     {"read", "update"},
	"dismiss-notifications":                  {"execute"}, "login-disclaimer": {"update"},
	"webhooks":               {"create", "read", "update", "delete"},
	"allowed-file-extension": {"create", "read", "delete"},
	// Infrastructure.
	"device-enrollment-program-instances": {"create", "read", "update", "delete"},
	"pki":                                 {"read", "update"}, "ad-cs-settings": {"create", "read", "update", "delete"},
	"digicert-settings": {"create", "read", "update", "delete"},
	"push-certificates": {"read", "update"}, "gsx-connection": {"read", "update"},
	"distribution-points":                   {"create", "read", "update", "delete"},
	"cloud-distribution-point":              {"read", "update"},
	"jamf-cloud-distribution-service-files": {"create", "read", "delete"},
	"json-web-token-configuration":          {"create", "read", "update", "delete"},
	"software-update-servers":               {"create", "read", "update", "delete"},
	"smtp-server":                           {"read", "update"}, "cache": {"read", "update"},
	"cloud-services-settings": {"read", "update"}, "apache-tomcat-settings": {"update"},
	"infrastructure-managers": {"create", "read", "update", "delete"},
	"retention-policy":        {"read", "update"}, "flush-policy-logs": {"execute"},
	"activation-code": {"read", "update"}, "jss-information": {"read"},
	"m2m": {"read"}, "jss-url": {"read", "update"},
}

// gaActions is the closed set of actions. Six exist, lowercase and
// case-sensitive. Anything else in a generated registry is a spec defect or a
// generator bug, never a new action to absorb quietly.
var gaActions = map[string]bool{
	"create": true, "read": true, "update": true,
	"delete": true, "deploy": true, "execute": true,
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

	var unknownCap, badAction, badForm []string
	for id, pkgs := range ids {
		sort.Strings(pkgs)
		where := strings.Join(uniqueStrings(pkgs), ",")
		parts := strings.Split(id, ":")
		if len(parts) != 2 {
			badForm = append(badForm, id+" ("+where+")")
			continue
		}
		cap, action := parts[0], parts[1]
		actions, ok := gaCapabilityActions[cap]
		if !ok {
			unknownCap = append(unknownCap, id+" ("+where+")")
			continue
		}
		if !gaActions[action] || !slicesContains(actions, action) {
			badAction = append(badAction, id+" ("+where+")")
		}
	}
	sort.Strings(unknownCap)
	sort.Strings(badAction)
	sort.Strings(badForm)

	t.Logf("%d distinct scoped identifiers across the generated registries", len(ids))
	if len(badForm) > 0 {
		t.Errorf("identifiers are not {capability}:{action}: %s\n"+
			"The three-part beta slug (create:pro:buildings) is retired; a return to it changes "+
			"the meaning of every consumer's permissions table.", strings.Join(badForm, ", "))
	}
	if len(unknownCap) > 0 {
		t.Errorf("capabilities absent from the permissions-map article: %s\n"+
			"Check each against the article's capability reference and add it to "+
			"gaCapabilityActions in the same change that ingested the spec. Do not delete "+
			"this assertion to make an ingest pass.", strings.Join(unknownCap, ", "))
	}
	if len(badAction) > 0 {
		t.Errorf("actions the article does not grant on that capability: %s\n"+
			"Either the spec is wrong (report upstream, do not patch locally) or the "+
			"article's action set for that capability has widened.", strings.Join(badAction, ", "))
	}
}

// uniqueStrings collapses a sorted slice to its distinct elements.
func uniqueStrings(in []string) []string {
	var out []string
	for i, s := range in {
		if i == 0 || in[i-1] != s {
			out = append(out, s)
		}
	}
	return out
}

// slicesContains is a local spelling to keep this file's Go version floor low.
func slicesContains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
