// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildArchive writes a zip whose members are the given name -> content pairs
// and opens it as the command would.
func buildArchive(t *testing.T, members map[string]string) *archive {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bundle.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := openArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.close)
	return a
}

// specDoc is the minimum a member needs for summarize and the title check.
func specDoc(title string, paths ...string) string {
	var b strings.Builder
	b.WriteString("openapi: 3.0.1\ninfo:\n  title: " + title + "\n  version: 1.0.0\npaths:\n")
	for _, p := range paths {
		b.WriteString("  " + p + ":\n    get:\n      operationId: x\n")
	}
	return b.String()
}

func TestOpenArchiveReadsBuildFromManifest(t *testing.T) {
	a := buildArchive(t, map[string]string{
		"MANIFEST.md": "# GitOps Platform API Archive - Build 2056\n\n**GitOps Build**: v2056\n",
	})
	if a.build != "v2056" {
		t.Fatalf("build = %q, want v2056", a.build)
	}
	if a.prefix != "" {
		t.Fatalf("prefix = %q, want empty", a.prefix)
	}
	if a.sha256 == "" {
		t.Fatal("sha256 not computed")
	}
}

// A wrapped archive is one whose members all sit under a single top-level
// directory. The bundles are unwrapped today, so this is only tolerance —
// but tolerance that silently failed would look like "no changes".
func TestOpenArchiveToleratesWrapperDirectory(t *testing.T) {
	a := buildArchive(t, map[string]string{
		"bundle-v9/MANIFEST.md":                    "**GitOps Build**: v9\n",
		"bundle-v9/external/jpapi/openapi.yaml":    specDoc("Jamf Pro API", "/a"),
		"bundle-v9/external/nonesuch/openapi.yaml": specDoc("Nonesuch API", "/b"),
	})
	if a.prefix != "bundle-v9/" {
		t.Fatalf("prefix = %q, want bundle-v9/", a.prefix)
	}
	if a.build != "v9" {
		t.Fatalf("build = %q, want v9", a.build)
	}
	if got := a.families("external"); len(got) != 2 || got[0] != "jpapi" || got[1] != "nonesuch" {
		t.Fatalf("families = %v", got)
	}
}

// The single easiest thing to miss in a build is a newly published API: it
// appears in no diff of the specs already carried.
func TestCheckUnmappedRejectsANewlyPublishedFamily(t *testing.T) {
	a := buildArchive(t, map[string]string{
		"MANIFEST.md":                         "**GitOps Build**: v1\n",
		"external/jpapi/openapi.yaml":         specDoc("Jamf Pro API", "/a"),
		"external/users/openapi.yaml":         specDoc("User Inventory API", "/b"),
		"external/brand-new-api/openapi.yaml": specDoc("Brand New API", "/c"),
		"external/_permissions/routes.yaml":   "routes: []\n",
		"external/unified-platform-api.yaml":  "openapi: 3.0.1\n",
		"internal/stage/jpapi-x/openapi.yaml": specDoc("Whatever", "/d"),
	})
	err := checkUnmapped(a, "external")
	if err == nil {
		t.Fatal("want an error for the unmapped family")
	}
	if !strings.Contains(err.Error(), "brand-new-api") {
		t.Fatalf("error does not name the family: %v", err)
	}
	// users is in knownUnmapped and a rollup at the tree root is not a family,
	// so neither may be reported.
	if strings.Contains(err.Error(), "users") || strings.Contains(err.Error(), "unified") {
		t.Fatalf("error names something it should tolerate: %v", err)
	}
}

func TestCheckUnmappedAcceptsAFullyAccountedArchive(t *testing.T) {
	members := map[string]string{"MANIFEST.md": "**GitOps Build**: v1\n"}
	for _, s := range specs {
		members["external/"+s.dir+"/openapi.yaml"] = specDoc(s.title, "/a")
	}
	members["external/users/openapi.yaml"] = specDoc("User Inventory API", "/a")
	if err := checkUnmapped(buildArchive(t, members), "external"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Directory names are not stable across builds, so a row that still resolves
// must additionally prove it points at the API it claims. The concrete trap:
// device-groups is the Platform API and securitycloud-devices is Security
// Cloud's, and copying one over the other silently replaces a spec.
func TestIngestRejectsAMisdirectedRow(t *testing.T) {
	members := map[string]string{"MANIFEST.md": "**GitOps Build**: v1\n"}
	for _, s := range specs {
		members["external/"+s.dir+"/openapi.yaml"] = specDoc(s.title, "/a")
	}
	members["external/securitycloud-devices/openapi.yaml"] = specDoc("Device Groups API", "/a")

	sel := map[string]bool{"securitycloud-device-groups-api.yaml": true}
	mf := &manifest{Entries: map[string]manifestEntry{}}
	_, err := ingest(buildArchive(t, members), "external", t.TempDir(), sel, mf, true)
	if err == nil {
		t.Fatal("want an error for the title mismatch")
	}
	for _, want := range []string{"Device Groups API", "Security Cloud Devices API", "securitycloud-device-groups-api.yaml"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error omits %q: %v", want, err)
		}
	}
}

// A held row that the archive predates is normal when reconstructing an older
// build; only a selected row has to resolve.
func TestIngestToleratesAnAbsentHeldFamilyButNotAnAbsentSelectedOne(t *testing.T) {
	members := map[string]string{"MANIFEST.md": "**GitOps Build**: v1\n"}
	for _, s := range specs {
		if s.heldAt != "" {
			continue
		}
		members["external/"+s.dir+"/openapi.yaml"] = specDoc(s.title, "/a")
	}
	a := buildArchive(t, members)

	sel, err := selection("", false)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ingest(a, "external", t.TempDir(), sel, &manifest{Entries: map[string]manifestEntry{}}, true)
	if err != nil {
		t.Fatalf("unheld-only selection should succeed: %v", err)
	}
	var absent int
	for _, r := range rows {
		if r.status == statusHeld && strings.Contains(r.note, "absent from this archive") {
			absent++
		}
	}
	if absent == 0 {
		t.Fatal("no held row reported as absent")
	}

	_, err = ingest(a, "external", t.TempDir(), map[string]bool{"Classic-openapi.yaml": true}, &manifest{Entries: map[string]manifestEntry{}}, true)
	if err == nil {
		t.Fatal("a selected family missing from the archive must fail")
	}
}

func TestSelection(t *testing.T) {
	unheld, err := selection("", false)
	if err != nil {
		t.Fatal(err)
	}
	all, err := selection("", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(specs) {
		t.Fatalf("-include-held selected %d of %d", len(all), len(specs))
	}
	if len(unheld) >= len(all) {
		t.Fatal("the default selection must exclude the held specs")
	}
	for _, s := range specs {
		if s.heldAt != "" && unheld[s.dest] {
			t.Fatalf("%s is held but selected by default", s.dest)
		}
	}
	// Naming a held spec explicitly is the act -include-held performs in bulk.
	one, err := selection("Classic-openapi.yaml", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || !one["Classic-openapi.yaml"] {
		t.Fatalf("-only selected %v", one)
	}
	if _, err := selection("no-such-spec.yaml", false); err == nil {
		t.Fatal("-only with an unknown destination must fail")
	}
}

func TestSummarizeCountsOperationsNotPaths(t *testing.T) {
	doc := `openapi: 3.0.1
info:
  title: Example API
  version: 9.9.9
paths:
  /a:
    get: {operationId: a}
    delete: {operationId: b}
    parameters: []
  /b:
    post: {operationId: c}
`
	title, version, npaths, nops, err := summarize([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if title != "Example API" || version != "9.9.9" {
		t.Fatalf("title/version = %q/%q", title, version)
	}
	if npaths != 2 {
		t.Fatalf("npaths = %d, want 2", npaths)
	}
	// parameters is a path-item sibling of the verbs and must not be counted.
	if nops != 3 {
		t.Fatalf("nops = %d, want 3", nops)
	}
}

// routes.yaml has been byte-different and sort-identical in five builds. The
// reordering check is what stops that reading as a content change.
func TestSortedSHA256IsBlindToLineOrderOnly(t *testing.T) {
	a := []byte("alpha\nbeta\ngamma\n")
	reordered := []byte("gamma\nalpha\nbeta\n")
	changed := []byte("alpha\nbeta\ndelta\n")
	if sortedSHA256(a) != sortedSHA256(reordered) {
		t.Fatal("a pure reordering must hash the same")
	}
	if sortedSHA256(a) == sortedSHA256(changed) {
		t.Fatal("a content change must not hash the same")
	}
	if sha256Bytes(a) == sha256Bytes(reordered) {
		t.Fatal("the plain hash must still see the reordering")
	}
}

// Every row must correspond to a config.json spec entry, or the ingest writes
// a file the generator never reads.
func TestEverySpecRowIsWhitelistedInConfig(t *testing.T) {
	raw, err := os.ReadFile("../config.json")
	if err != nil {
		t.Skipf("config.json unreadable: %v", err)
	}
	cfg := string(raw)
	for _, s := range specs {
		if !strings.Contains(cfg, `"testing/`+s.dest+`"`) {
			t.Errorf("%s has no config.json spec entry naming testing/%s", s.dest, s.dest)
		}
		if s.title == "" {
			t.Errorf("%s has no title assertion", s.dest)
		}
		if (s.heldAt == "") != (s.why == "") {
			t.Errorf("%s: heldAt and why must be set together", s.dest)
		}
	}
}
