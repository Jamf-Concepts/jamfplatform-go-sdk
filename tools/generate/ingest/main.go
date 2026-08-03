// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Command ingest populates the SDK's private source specs (the gitignored
// testing/ directory) from a single Jamf Platform APIs GitOps archive (zip).
//
// The generator's source specs were historically sourced by hand, one file
// per API family. This command replaces that manual step: point it at a single
// GitOps archive (`jamf-platform-apis-gitops-vNNN-*.zip`, produced from
// jamf/public-apis-oas) and it drops the correct spec for each family into
// testing/ under the exact filenames tools/generate/config.json already
// expects. No config change is needed — after ingest, run `make generate`.
//
//	cd tools/generate && go run ./ingest -root <repo-root> -zip path/to/archive.zip
//	make generate
//
// # Which archive member feeds which family
//
// The archive ships two environments (external/ = production gateway,
// internal/{dev,stage}/) and, per family, a production dir plus one or more
// -beta dirs. Two facts drive the mapping:
//
//   - Only the beta variants carry the path shape the SDK config whitelists
//     (/v1/tenant/{tenantId}/...). The production variants have the tenant
//     segment stripped, so every operation lookup would miss and the generator
//     would emit nothing.
//   - Only the beta variants carry the x-required-privileges extension that
//     becomes the SDK's `Required privileges:` godoc and per-package Privileges
//     registries. The production variants have zero.
//
// So each family resolves to its privilege-bearing beta variant. The selection
// is automatic: for family F we try F-beta-beta, then F-beta, then F, and pick
// the first that exists AND carries privileges. That rule handles the one
// naming quirk in the archive — compliance-benchmarks, whose privilege-bearing
// spec is compliance-benchmarks-beta-beta (plain compliance-benchmarks-beta has
// none) — without special-casing, and stays correct if a future build renames
// the variants. A privilege-bearing family that resolves to zero privileges is
// treated as a hard error (wrong archive / changed structure).
//
// # Exceptions (not sourced from the archive)
//
//   - App Installer Titles / Deployments / Global Settings — these three pro
//     specs are NOT present in the archive (the app-installers paths are absent
//     from jpapi too). They remain manually sourced in testing/. The generator
//     falls back to their committed api/*.json when the testing/ file is absent
//     (see resolveSpecPath in tools/generate/emit.go), so they keep working.
//   - jpapi version — the archive publishes whatever Jamf Pro API version the
//     upstream jamf/public-apis-oas repo has released, which historically lags
//     the shipping Jamf Pro version by a minor. Adopting an archive therefore
//     pins jpapi to that build's version. Check JamfProAPIVersion after
//     `make generate` and confirm the version is acceptable before committing.
//
// # Why CI does not need this command
//
// CI regenerates Go from the committed api/*.json published specs via the
// generator's fallback path, never from testing/. This command only refreshes
// the private testing/ sources a maintainer uses to regenerate the public api/*
// surface when it changes.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// privKey is the OpenAPI extension whose presence marks a privilege-bearing
// (beta) variant; its raw count is how we pick the right variant per family.
const privKey = "x-required-privileges"

// familyMapping is the dest filename in testing/ -> archive family directory
// stem. Candidate variants (family-beta-beta, family-beta, family) are derived
// from the stem; the privilege-bearing one is selected automatically. Ordered
// so the summary table reads the same way every run.
var familyMapping = []struct{ dest, family string }{
	{"openapi-jpapi.json", "jpapi"},
	{"Classic-openapi.json", "capi"},
	{"blueprints-api.json", "blueprints"},
	{"device-groups-api.json", "device-groups"},
	{"device-inventory-api.json", "devices"},
	{"device-management-actions-api.json", "device-management-action"},
	{"Declaration-reporting-openapi.json", "declaration-reporting"},
	{"jamf-compliance-benchmark-engine-api.yaml", "compliance-benchmarks"},
}

// manualExceptions are sourced manually, not from the archive. Listed only so
// the summary is honest about the full picture; these files are never touched.
var manualExceptions = []struct{ file, why string }{
	{"AppInstallerTitles.yaml", "app-installers paths are not in the archive"},
	{"AppInstallerDeployments.yaml", "app-installers paths are not in the archive"},
	{"AppInstallerGlobalSettings.yaml", "app-installers paths are not in the archive"},
}

// summaryRow is one line of the post-ingest report.
type summaryRow struct {
	dest    string
	variant string
	npaths  int
	nprivs  int
}

// resolved is the winning variant for one family.
type resolved struct {
	variant string
	member  string
	doc     any
	raw     []byte
	npaths  int
	nprivs  int
}

// findMember returns the archive member for env/variant/openapi.yaml, tolerating
// an optional top-level wrapper directory. Empty string if absent.
func findMember(names []string, env, variant string) string {
	needle := env + "/" + variant + "/openapi.yaml"
	for _, n := range names {
		if n == needle || strings.HasSuffix(n, "/"+needle) {
			return n
		}
	}
	return ""
}

// pathCount counts entries under the spec's top-level `paths` mapping.
func pathCount(doc any) int {
	m, ok := doc.(map[string]any)
	if !ok {
		return 0
	}
	p, ok := m["paths"].(map[string]any)
	if !ok {
		return 0
	}
	return len(p)
}

// privCount counts x-required-privileges occurrences the same way `grep -c`
// would on the raw spec — robust to exactly where the extension is nested.
func privCount(raw []byte) int {
	return bytes.Count(raw, []byte(privKey))
}

// readMember reads a single archive member by name.
func readMember(files map[string]*zip.File, name string) ([]byte, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("member not found: %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// resolve picks the best variant for a family: F-beta-beta, then F-beta, then
// F, returning the first that exists AND carries privileges. Fails loudly if
// variants exist but none carry privileges (wrong archive / env / structure).
func resolve(files map[string]*zip.File, names []string, env, family string) (resolved, error) {
	candidates := []string{family + "-beta-beta", family + "-beta", family}
	var seen []string
	var fallback *resolved // first existing candidate, used only if none carry privs
	for _, variant := range candidates {
		member := findMember(names, env, variant)
		if member == "" {
			continue
		}
		raw, err := readMember(files, member)
		if err != nil {
			return resolved{}, fmt.Errorf("reading %s: %w", member, err)
		}
		var doc any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return resolved{}, fmt.Errorf("parsing %s: %w", member, err)
		}
		r := resolved{
			variant: variant,
			member:  member,
			doc:     doc,
			raw:     raw,
			npaths:  pathCount(doc),
			nprivs:  privCount(raw),
		}
		seen = append(seen, fmt.Sprintf("%s(paths=%d, privs=%d)", variant, r.npaths, r.nprivs))
		if fallback == nil {
			cp := r
			fallback = &cp
		}
		if r.nprivs > 0 {
			return r, nil
		}
	}
	if fallback == nil {
		return resolved{}, fmt.Errorf(
			"family %q: no variant found under %s/ (tried %s)",
			family, env, strings.Join(candidates, ", "),
		)
	}
	// Existed but zero privileges across every candidate — wrong archive/env or
	// the structure changed. Fail loudly rather than emit privilege-less code.
	return resolved{}, fmt.Errorf(
		"family %q: found variants but none carry %s [%s]. Wrong environment (%s)? "+
			"Production variants have no privileges — use -env external and beta variants",
		family, privKey, strings.Join(seen, ", "), env,
	)
}

// writeJSON serializes a parsed spec to indented JSON without HTML-escaping
// (spec descriptions routinely contain <, >, &), matching the prior Python
// json.dump(ensure_ascii=False, indent=2). Encode appends the trailing newline.
func writeJSON(path string, doc any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func main() {
	log.SetFlags(0)

	zipPath := flag.String("zip", "", "path to the GitOps archive (.zip) [required]")
	env := flag.String("env", "external", "environment tree within the archive: external | internal/dev | internal/stage")
	dest := flag.String("dest", "testing", "destination spec directory (relative paths resolve against -root)")
	root := flag.String("root", "", "repo root directory (default: auto-detected from git)")
	dryRun := flag.Bool("dry-run", false, "resolve and report but write nothing")
	flag.Parse()

	if *zipPath == "" {
		log.Fatal("-zip is required")
	}
	switch *env {
	case "external", "internal/dev", "internal/stage":
	default:
		log.Fatalf("invalid -env %q: must be external, internal/dev, or internal/stage", *env)
	}

	if *root == "" {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			log.Fatal("cannot detect repo root (pass -root): ", err)
		}
		*root = strings.TrimSpace(string(out))
	}
	// The Makefile runs this with cwd=tools/generate, so resolve relative paths
	// against the repo root, mirroring the generate command's -root handling.
	zipArg := *zipPath
	if !filepath.IsAbs(zipArg) {
		zipArg = filepath.Join(*root, zipArg)
	}
	destDir := *dest
	if !filepath.IsAbs(destDir) {
		destDir = filepath.Join(*root, destDir)
	}

	if fi, err := os.Stat(zipArg); err != nil || fi.IsDir() {
		log.Fatalf("archive not found: %s", zipArg)
	}

	zr, err := zip.OpenReader(zipArg)
	if err != nil {
		log.Fatalf("opening archive: %v", err)
	}
	defer func() { _ = zr.Close() }()

	files := make(map[string]*zip.File, len(zr.File))
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
		names = append(names, f.Name)
	}

	var rows []summaryRow
	for _, fm := range familyMapping {
		r, err := resolve(files, names, *env, fm.family)
		if err != nil {
			log.Fatal(err)
		}
		outPath := filepath.Join(destDir, fm.dest)
		if !*dryRun {
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				log.Fatalf("creating %s: %v", destDir, err)
			}
			if strings.HasSuffix(fm.dest, ".json") {
				if err := writeJSON(outPath, r.doc); err != nil {
					log.Fatalf("writing %s: %v", outPath, err)
				}
			} else { // .yaml — write the archive bytes verbatim
				if err := os.WriteFile(outPath, r.raw, 0o644); err != nil {
					log.Fatalf("writing %s: %v", outPath, err)
				}
			}
		}
		rows = append(rows, summaryRow{fm.dest, r.variant, r.npaths, r.nprivs})
	}

	printSummary(*dryRun, zipArg, *env, rows)
}

// printSummary renders the same aligned table the Python script produced.
func printSummary(dryRun bool, zipArg, env string, rows []summaryRow) {
	verb := "Ingested"
	if dryRun {
		verb = "Would ingest"
	}
	fmt.Printf("%s from %s (env: %s)\n\n", verb, zipArg, env)

	sources := make([]string, len(rows))
	w := len("dest")
	sw := len("source variant")
	for i, r := range rows {
		sources[i] = r.variant + "/openapi.yaml"
		if len(r.dest) > w {
			w = len(r.dest)
		}
		if len(sources[i]) > sw {
			sw = len(sources[i])
		}
	}

	fmt.Printf("  %-*s  %-*s  paths  privs\n", w, "dest", sw, "source variant")
	fmt.Printf("  %s  %s  -----  -----\n", strings.Repeat("-", w), strings.Repeat("-", sw))
	for i, r := range rows {
		fmt.Printf("  %-*s  %-*s  %5d  %5d\n", w, r.dest, sw, sources[i], r.npaths, r.nprivs)
	}

	fmt.Print("\n  manual (not in archive, untouched):\n")
	for _, e := range manualExceptions {
		fmt.Printf("    %-*s  — %s\n", w, e.file, e.why)
	}

	if !dryRun {
		fmt.Print(
			"\nNext: run `make generate`, then confirm JamfProAPIVersion in " +
				"jamfplatform/version.go is the version you expect before committing.\n",
		)
	}
}
