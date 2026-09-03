// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"strings"
	"testing"
)

// syntheticMap wraps capability rows in the surrounding structure the parser
// navigates: a conversion-example table before the section it reads, and a
// no-capability table after it. Both must be ignored.
func syntheticMap(rows string) string {
	return `# Permissions map

## Converting an old privilege name

| Old privilege  | GA capability permission |
| -------------- | ------------------------ |
| Read Buildings | ` + "`buildings:read`" + `             |

## Find the capability for an endpoint you already call

### Inventory

| Permission | Capability | Endpoints |
| ---------- | ---------- | --------- |
` + rows + `

## Endpoints with no permission

| Resource | Capability |
| -------- | ---------- |
| Legacy   | ` + "`not-a-capability:{r}`" + ` |
`
}

func TestParsePermissionsMapReadsTheCommittedSnapshot(t *testing.T) {
	raw, err := os.ReadFile("permissions-map.md")
	if err != nil {
		t.Fatal(err)
	}
	declared, err := parsePermissionsMap(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(declared) < minDeclaredCapabilities {
		t.Fatalf("parsed %d capabilities", len(declared))
	}
	// One from each shape the tables use: a full CRUD capability, one the SDK
	// supplies by hand because no published spec carries it, and the
	// multi-row capability whose union is the regression below.
	for _, tc := range []struct {
		name string
		want []string
	}{
		{"devices", []string{"create", "read", "update", "delete"}},
		{"sso-domains", []string{"create", "read", "update", "delete"}},
		{"licensing", []string{"read"}},
		{"compliance-benchmarks", []string{"create", "read", "delete"}},
	} {
		dc, ok := declared[tc.name]
		if !ok {
			t.Errorf("%s not declared", tc.name)
			continue
		}
		for _, a := range tc.want {
			if !dc[a] {
				t.Errorf("%s: want action %q, got {%s}", tc.name, a, strings.Join(sortedKeys(dc), ","))
			}
		}
	}
	// The Protect-only capabilities list GraphQL operation names rather than
	// paths, which must not stop them being declared.
	if _, ok := declared["threat-alerts"]; !ok {
		t.Error("threat-alerts not declared — the parser is dropping path-less rows")
	}
}

// A capability appears in as many rows as its resources need, and reading one
// row as the whole declaration manufactures a disagreement. That is exactly the
// bug that first populated permissionsMapExceptions: keeping only
// `compliance-benchmarks:{r}` over /baselines lost `{c,r,d}` over /benchmarks.
func TestParsePermissionsMapUnionsRowsForOneCapability(t *testing.T) {
	doc := syntheticMap(
		"| Widgets       | `widgets:{c,r,d}` | Platform `/widgets`  |\n" +
			"| Widget rules  | `widgets:{r}`     | Platform `/rules`    |")
	declared, err := parsePermissionsMapForTest(t, doc)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sortedKeys(declared["widgets"]), ",")
	if got != "create,delete,read" {
		t.Fatalf("widgets actions = %q, want create,delete,read", got)
	}
}

func TestParsePermissionsMapIgnoresRowsOutsideTheCapabilitySection(t *testing.T) {
	declared, err := parsePermissionsMapForTest(t,
		syntheticMap("| Widgets | `widgets:{r}` | Platform `/widgets` |"))
	if err != nil {
		t.Fatal(err)
	}
	// buildings is in the conversion-example table, not-a-capability in the
	// no-capability table. Neither is a declaration.
	for _, name := range []string{"buildings", "not-a-capability"} {
		if _, ok := declared[name]; ok {
			t.Errorf("%s parsed as a declared capability", name)
		}
	}
}

func TestParsePermissionsMapRejectsABrokenSnapshot(t *testing.T) {
	t.Run("missing heading", func(t *testing.T) {
		_, err := parsePermissionsMap("# Permissions map\n\nSomething else entirely.\n")
		if err == nil || !strings.Contains(err.Error(), "restructured") {
			t.Fatalf("err = %v", err)
		}
	})
	// A missing *closing* heading must fail the same way as a missing opening
	// one — falling through to end-of-document would silently widen the parse
	// into the tables that follow, which list resources with no capability at
	// all. This document never mentions "## Endpoints with no permission", so
	// unlike parsePermissionsMapForTest's fixtures there is no filler to reach
	// minDeclaredCapabilities with — and none is needed, since the missing
	// heading must be reported before that count is ever checked.
	t.Run("missing end heading", func(t *testing.T) {
		doc := "# Permissions map\n\n" +
			"## Find the capability for an endpoint you already call\n\n" +
			"| Permission | Capability | Endpoints |\n" +
			"| ---------- | ---------- | --------- |\n" +
			"| Widgets    | `widgets:{r}` | Platform `/widgets` |\n"
		_, err := parsePermissionsMap(doc)
		if err == nil || !strings.Contains(err.Error(), "Endpoints with no permission") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("too few capabilities", func(t *testing.T) {
		_, err := parsePermissionsMap(syntheticMap("| Widgets | `widgets:{r}` | Platform `/widgets` |"))
		if err == nil || !strings.Contains(err.Error(), "at least") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unknown action code", func(t *testing.T) {
		_, err := parsePermissionsMapForTest(t,
			syntheticMap("| Widgets | `widgets:{r,zzz}` | Platform `/widgets` |"))
		if err == nil || !strings.Contains(err.Error(), "unknown action code") {
			t.Fatalf("err = %v", err)
		}
	})
}

// parsePermissionsMapForTest pads a synthetic document with filler capabilities
// so it clears minDeclaredCapabilities, which exists to catch a snapshot that
// parsed to nothing and would otherwise reject every synthetic fixture.
func parsePermissionsMapForTest(t *testing.T, doc string) (map[string]map[string]bool, error) {
	t.Helper()
	var filler strings.Builder
	for i := range minDeclaredCapabilities {
		filler.WriteString("| Filler | `filler-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ":{r}` | Platform `/f` |\n")
	}
	return parsePermissionsMap(strings.Replace(doc, "\n\n## Endpoints with no permission",
		"\n"+filler.String()+"\n## Endpoints with no permission", 1))
}

// withExceptions installs a synthetic exceptions list for one test. The list
// is package state the generator reads once, so no test here may run in
// parallel.
func withExceptions(t *testing.T, exceptions ...permissionsMapException) {
	t.Helper()
	prev := permissionsMapExceptions
	permissionsMapExceptions = exceptions
	t.Cleanup(func() { permissionsMapExceptions = prev })
}

// declare builds a parsed-map stand-in without going through markdown.
func declare(rows map[string][]string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for name, actions := range rows {
		set := map[string]bool{}
		for _, a := range actions {
			set[a] = true
		}
		out[name] = set
	}
	return out
}

func TestCheckAgainstPermissionsMapAcceptsAgreement(t *testing.T) {
	withExceptions(t)
	r := checkAgainstPermissionsMap(
		declare(map[string][]string{"widgets": {"create", "read"}}),
		map[string][]string{"widgets:read": {"pro"}, "widgets:create": {"pro"}})
	if !r.OK() {
		t.Fatalf("want agreement, got %+v", r)
	}
	if r.Identifiers != 2 || r.Capabilities != 1 {
		t.Fatalf("counts = %d identifiers, %d capabilities", r.Identifiers, r.Capabilities)
	}
}

func TestCheckAgainstPermissionsMapFlagsAnUndeclaredCapability(t *testing.T) {
	withExceptions(t)
	r := checkAgainstPermissionsMap(
		declare(map[string][]string{"widgets": {"read"}}),
		map[string][]string{"gadgets:read": {"pro", "proclassic", "pro"}})
	if r.OK() {
		t.Fatal("want a defect")
	}
	if len(r.UnknownCapability) != 1 || r.UnknownCapability[0] != "gadgets:read (pro,proclassic)" {
		t.Fatalf("UnknownCapability = %q", r.UnknownCapability)
	}
}

// The action matters as much as the capability: a spec declaring a write on a
// read-only capability is the disagreement most likely to be real.
func TestCheckAgainstPermissionsMapFlagsAnUndeclaredAction(t *testing.T) {
	withExceptions(t)
	r := checkAgainstPermissionsMap(
		declare(map[string][]string{"widgets": {"read"}}),
		map[string][]string{"widgets:delete": {"pro"}})
	if len(r.UnknownAction) != 1 || !strings.Contains(r.UnknownAction[0], "declares widgets with {read}") {
		t.Fatalf("UnknownAction = %q", r.UnknownAction)
	}
	if len(r.UnknownCapability) != 0 {
		t.Fatalf("a declared capability must not also be reported unknown: %q", r.UnknownCapability)
	}
}

// A regression to the retired three-part beta slug would silently change the
// meaning of every consumer's permissions table, so it is its own bucket.
func TestCheckAgainstPermissionsMapFlagsTheRetiredSlugForm(t *testing.T) {
	withExceptions(t)
	r := checkAgainstPermissionsMap(
		declare(map[string][]string{"buildings": {"create"}}),
		map[string][]string{"Read Buildings": {"pro"}, "buildings:": {"pro"}, ":create": {"pro"}})
	if len(r.BadForm) != 3 {
		t.Fatalf("BadForm = %q, want all three malformed", r.BadForm)
	}
	if len(r.UnknownCapability) != 0 || len(r.UnknownAction) != 0 {
		t.Fatal("a malformed identifier must not also be reported as a vocabulary defect")
	}
}

func TestCheckAgainstPermissionsMapToleratesARecordedException(t *testing.T) {
	withExceptions(t, permissionsMapException{
		Permission: "widgets:delete", Why: "wire-verified; reported upstream",
	})
	r := checkAgainstPermissionsMap(
		declare(map[string][]string{"widgets": {"read"}}),
		map[string][]string{"widgets:delete": {"pro"}})
	if !r.OK() {
		t.Fatalf("a recorded exception must be tolerated: %+v", r)
	}
}

// An exception is self-expiring in both directions, the same discipline as
// schemaPatchesRequireAbsent: whether the map caught up or the SDK stopped
// emitting the permission, the entry is dead and must be deleted.
func TestPermissionsMapExceptionsExpire(t *testing.T) {
	t.Run("the map caught up", func(t *testing.T) {
		withExceptions(t, permissionsMapException{Permission: "widgets:delete", Why: "stale"})
		r := checkAgainstPermissionsMap(
			declare(map[string][]string{"widgets": {"read", "delete"}}),
			map[string][]string{"widgets:delete": {"pro"}})
		if len(r.ExpiredExceptions) != 1 || r.OK() {
			t.Fatalf("ExpiredExceptions = %q, OK = %v", r.ExpiredExceptions, r.OK())
		}
	})
	t.Run("the SDK stopped emitting it", func(t *testing.T) {
		withExceptions(t, permissionsMapException{Permission: "widgets:delete", Why: "stale"})
		r := checkAgainstPermissionsMap(
			declare(map[string][]string{"widgets": {"read"}}),
			map[string][]string{"widgets:read": {"pro"}})
		if len(r.ExpiredExceptions) != 1 {
			t.Fatalf("ExpiredExceptions = %q", r.ExpiredExceptions)
		}
	})
}

// Coverage is reported, never failed: the map covers products this SDK does not.
func TestCheckAgainstPermissionsMapReportsCoverageWithoutFailing(t *testing.T) {
	withExceptions(t)
	r := checkAgainstPermissionsMap(
		declare(map[string][]string{
			"widgets":       {"create", "read"},
			"threat-alerts": {"read"},
		}),
		map[string][]string{"widgets:read": {"pro"}})
	if !r.OK() {
		t.Fatalf("coverage must not be a defect: %+v", r)
	}
	if len(r.UnreachedCapabilities) != 1 || r.UnreachedCapabilities[0] != "threat-alerts" {
		t.Fatalf("UnreachedCapabilities = %q", r.UnreachedCapabilities)
	}
	if len(r.UnusedActions) != 1 || r.UnusedActions[0] != "widgets:create" {
		t.Fatalf("UnusedActions = %q", r.UnusedActions)
	}
}

func TestLoadPermissionsMapReportsAMissingSnapshot(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := loadPermissionsMap(); err == nil || !strings.Contains(err.Error(), "make permmap") {
		t.Fatalf("err = %v", err)
	}
}
