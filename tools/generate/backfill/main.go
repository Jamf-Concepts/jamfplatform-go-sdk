// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Command backfill inserts older-version Pro endpoints into
// tools/generate/config.json.
//
// Jamf publishes endpoints under /v1/, /v2/, /v3/ paths and keeps prior
// versions in the spec until they are physically removed. The SDK's policy
// (issue #19) is to retain every spec version side-by-side so downstream
// consumers (notably terraform-provider-jamfplatform) get a real migration
// window when Jamf marks an endpoint deprecated.
//
// This command reconciles config.json with the Pro spec by inserting any spec
// version of a multi-version base path that is missing from config. For each
// missing version it:
//
//   - Synthesizes an operation entry from a sibling already in config.
//   - Adjusts the operation name's trailing V<n> suffix to the new version.
//   - Replicates resolver / resolvers / apply blocks with version-suffixed
//     resourceType / typedReturn / op-name / updateType / membershipPreFetch
//     fields so each version has its own Resolve<X>V<N>ByName and Apply<X>V<N>
//     sugar.
//   - For V1 typedReturn lookups, falls back to the unsuffixed schema name when
//     present (Jamf's V1 schemas often lack a version suffix —
//     `ComputerInventory` instead of `ComputerInventoryV1`).
//
// ExpectedStatus is deliberately not carried over: it varies between versions
// (e.g. PATCH detail returns 200 on V1 but 204 on V2/V3) and the generator's
// detectResponse infers it from each spec operation directly.
//
// config.json is hand-authored, so the command round-trips it byte-for-byte via
// the order-preserving JSON model in ojson.go: a run that inserts nothing leaves
// the file untouched, and inserted entries are the only diff. It is idempotent.
//
//	cd tools/generate && go run ./backfill -root <repo-root>
//	make generate   # materialise the new methods
//
// It reads the private spec at testing/openapi-jpapi.json (a maintainer step,
// like ingest); CI never runs it.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	reOp        = regexp.MustCompile(`^(\w+)\s+(.+)$`)
	reVerPath   = regexp.MustCompile(`^/v([0-9]+)(.*)$`)
	reTrailingV = regexp.MustCompile(`V(\d+)$`)
)

// httpMethods are the path-item keys treated as operations (in a fixed order so
// multiple inserts under one path land deterministically).
var httpMethods = []string{"get", "post", "put", "patch", "delete"}

// opKey identifies an operation by HTTP method + path, matching config's "op".
type opKey struct{ method, path string }

// parseOp splits an "op" string ("GET /v1/...") into (METHOD upper, path).
func parseOp(s string) (opKey, bool) {
	m := reOp.FindStringSubmatch(s)
	if m == nil {
		return opKey{}, false
	}
	return opKey{strings.ToUpper(m[1]), m[2]}, true
}

// stripTrailingV removes a trailing V<n> suffix if present.
func stripTrailingV(name string) string {
	if loc := reTrailingV.FindStringIndex(name); loc != nil {
		return name[:loc[0]]
	}
	return name
}

// adjustOpName re-suffixes a method name to the target version.
func adjustOpName(name string, targetV int) string {
	return fmt.Sprintf("%sV%d", stripTrailingV(name), targetV)
}

// adjustTypeName re-suffixes a Go type name to the target version, preferring
// the version-suffixed schema, then the unsuffixed schema (Jamf reuses one
// schema across versions), and finally the original name (surfaces as a build
// error rather than silently emitting a wrong type).
func adjustTypeName(name string, targetV int, schemas map[string]bool) string {
	if name == "" {
		return name
	}
	base := stripTrailingV(name)
	if suffixed := fmt.Sprintf("%sV%d", base, targetV); schemas[suffixed] {
		return suffixed
	}
	if schemas[base] {
		return base
	}
	return name
}

// adjustResolver clones a resolver/resolvers block, version-suffixing every
// name-bearing field. Key order is preserved so the output diff is minimal.
func adjustResolver(r *omap, targetV int, schemas map[string]bool) *omap {
	out := newOmap()
	for _, k := range r.keys {
		v := r.m[k]
		switch k {
		case "resourceType":
			out.set(k, adjustOpName(v.(string), targetV))
		case "typedReturn":
			out.set(k, adjustTypeName(v.(string), targetV, schemas))
		case "apply":
			out.set(k, adjustApply(v.(*omap), targetV, schemas))
		default:
			out.set(k, v)
		}
	}
	return out
}

func adjustApply(a *omap, targetV int, schemas map[string]bool) *omap {
	ap := newOmap()
	for _, k := range a.keys {
		v := a.m[k]
		switch k {
		case "createOp", "updateOp", "deleteOp", "getOp", "tokenUploadCreateOp", "tokenReplaceOp":
			ap.set(k, adjustOpName(v.(string), targetV))
		case "updateType":
			ap.set(k, adjustTypeName(v.(string), targetV, schemas))
		case "membershipPreFetch":
			ap.set(k, adjustMembershipPreFetch(v.(*omap), targetV, schemas))
		default:
			ap.set(k, v)
		}
	}
	return ap
}

func adjustMembershipPreFetch(mpf *omap, targetV int, schemas map[string]bool) *omap {
	out := newOmap()
	for _, k := range mpf.keys {
		v := mpf.m[k]
		switch k {
		case "fetchOp":
			out.set(k, adjustOpName(v.(string), targetV))
		case "assignmentType":
			out.set(k, adjustTypeName(v.(string), targetV, schemas))
		default:
			out.set(k, v)
		}
	}
	return out
}

// specQueryParamNames returns the query-parameter names declared on the spec
// operation at path/method. Empty when the op or its parameters are absent.
func specQueryParamNames(spec *omap, path, method string) map[string]bool {
	out := map[string]bool{}
	paths, ok := spec.child("paths")
	if !ok {
		return out
	}
	pi, ok := paths.child(path)
	if !ok {
		return out
	}
	op, ok := pi.child(strings.ToLower(method))
	if !ok {
		return out
	}
	params, ok := op.slice("parameters")
	if !ok {
		return out
	}
	for _, p := range params {
		pm, ok := p.(*omap)
		if !ok {
			continue
		}
		in, _ := pm.str("in")
		name, hasName := pm.str("name")
		if in == "query" && hasName && name != "" {
			out[name] = true
		}
	}
	return out
}

// filterParamsForVersion drops sibling params whose spec name is not declared on
// the target version's op. Param format is "name", "name:type" or
// "name:type:goName" — the first segment is the spec name.
func filterParamsForVersion(siblingParams []any, spec *omap, path, method string) []any {
	if len(siblingParams) == 0 {
		return siblingParams
	}
	allowed := specQueryParamNames(spec, path, method)
	kept := []any{}
	for _, p := range siblingParams {
		s, ok := p.(string)
		if !ok {
			continue
		}
		name := s
		if before, _, ok0 := strings.Cut(s, ":"); ok0 {
			name = before
		}
		if allowed[name] {
			kept = append(kept, p)
		}
	}
	return kept
}

// isRawArrayResponse reports whether the spec's 2xx response body for
// path/method is a top-level JSON array rather than an object envelope.
func isRawArrayResponse(spec *omap, path, method string) bool {
	paths, ok := spec.child("paths")
	if !ok {
		return false
	}
	pi, ok := paths.child(path)
	if !ok {
		return false
	}
	op, ok := pi.child(strings.ToLower(method))
	if !ok {
		return false
	}
	responses, ok := op.child("responses")
	if !ok {
		return false
	}
	for _, status := range responses.keys {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		resp, ok := responses.child(status)
		if !ok {
			continue
		}
		content, ok := resp.child("content")
		if !ok {
			continue
		}
		for _, ct := range []string{"application/json", "*/*"} {
			media, ok := content.child(ct)
			if !ok {
				continue
			}
			sch, ok := media.child("schema")
			if !ok {
				continue
			}
			if t, _ := sch.str("type"); t == "array" {
				return true
			}
		}
	}
	return false
}

// carriedKeys are copied from the sibling into a synthesized op, in this order.
var carriedKeys = []string{
	"pagination", "pageSizeParam", "contentType", "params",
	"unwrapResults", "requestType", "responseType", "pathNames",
}

// synthesizeOp builds a new operation entry for targetV from a sibling.
func synthesizeOp(sibling *omap, targetV int, targetPath string, spec *omap, schemas map[string]bool) *omap {
	opStr, _ := sibling.str("op")
	key, _ := parseOp(opStr)
	method := key.method

	name, _ := sibling.str("name")

	out := newOmap()
	out.set("op", fmt.Sprintf("%s %s", method, targetPath))
	out.set("name", adjustOpName(name, targetV))

	for _, k := range carriedKeys {
		v, ok := sibling.get(k)
		if !ok {
			continue
		}
		if k == "params" {
			filtered := filterParamsForVersion(v.([]any), spec, targetPath, method)
			if len(filtered) > 0 {
				out.set(k, filtered)
			}
			continue
		}
		out.set(k, v)
	}

	// A raw-array response op has no paginated envelope; the generator's
	// paginators would send a page-size query the server rejects.
	if out.has("pagination") && isRawArrayResponse(spec, targetPath, method) {
		out.del("pagination")
		out.del("pageSizeParam")
	}

	if r, ok := sibling.child("resolver"); ok {
		out.set("resolver", adjustResolver(r, targetV, schemas))
	}
	if rs, ok := sibling.slice("resolvers"); ok {
		adjusted := make([]any, 0, len(rs))
		for _, r := range rs {
			adjusted = append(adjusted, adjustResolver(r.(*omap), targetV, schemas))
		}
		out.set("resolvers", adjusted)
	}
	return out
}

func indexOf(list []any, target *omap) int {
	for i, e := range list {
		if o, ok := e.(*omap); ok && o == target {
			return i
		}
	}
	return -1
}

func main() {
	log.SetFlags(0)

	root := flag.String("root", "", "repo root directory (default: auto-detected from git)")
	flag.Parse()

	if *root == "" {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			log.Fatal("cannot detect repo root (pass -root): ", err)
		}
		*root = strings.TrimSpace(string(out))
	}
	cfgPath := filepath.Join(*root, "tools", "generate", "config.json")
	specPath := filepath.Join(*root, "testing", "openapi-jpapi.json")

	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatalf("reading config: %v", err)
	}
	specData, err := os.ReadFile(specPath)
	if err != nil {
		log.Fatalf("reading spec (maintainer step needs the private testing/ source): %v", err)
	}

	cfgAny, err := decodeOrdered(cfgData)
	if err != nil {
		log.Fatalf("parsing config: %v", err)
	}
	specAny, err := decodeOrdered(specData)
	if err != nil {
		log.Fatalf("parsing spec: %v", err)
	}
	cfg, ok := cfgAny.(*omap)
	if !ok {
		log.Fatal("config root is not a JSON object")
	}
	spec, ok := specAny.(*omap)
	if !ok {
		log.Fatal("spec root is not a JSON object")
	}

	// Schema-name set for typedReturn resolution.
	schemas := map[string]bool{}
	if comps, ok := spec.child("components"); ok {
		if sch, ok := comps.child("schemas"); ok {
			for _, k := range sch.keys {
				schemas[k] = true
			}
		}
	}

	// Locate the pro spec entry and its operations list.
	specs, ok := cfg.slice("specs")
	if !ok {
		log.Fatal("config has no specs array")
	}
	var proSpec *omap
	for _, s := range specs {
		so, ok := s.(*omap)
		if !ok {
			continue
		}
		if pkg, _ := so.str("package"); pkg == "pro" {
			proSpec = so
			break
		}
	}
	if proSpec == nil {
		log.Fatal("no spec with package \"pro\" found in config")
	}
	opsList, ok := proSpec.slice("operations")
	if !ok {
		log.Fatal("pro spec has no operations array")
	}

	// Index config ops by (method, path) and by name.
	cfgOpsByKey := map[opKey]*omap{}
	cfgOpsByName := map[string]*omap{}
	for _, e := range opsList {
		op, ok := e.(*omap)
		if !ok {
			continue
		}
		if s, ok := op.str("op"); ok {
			if k, ok := parseOp(s); ok {
				cfgOpsByKey[k] = op
			}
		}
		if n, ok := op.str("name"); ok {
			cfgOpsByName[n] = op
		}
	}

	// Bucket spec paths by version-stripped base path.
	verByBase := map[string]map[int]bool{}
	if paths, ok := spec.child("paths"); ok {
		for _, p := range paths.keys {
			m := reVerPath.FindStringSubmatch(p)
			if m == nil {
				continue
			}
			v, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			base := m[2]
			if verByBase[base] == nil {
				verByBase[base] = map[int]bool{}
			}
			verByBase[base][v] = true
		}
	}

	// Multi-version bases, deterministically ordered.
	multiBases := make([]string, 0)
	for base, vs := range verByBase {
		if len(vs) > 1 {
			multiBases = append(multiBases, base)
		}
	}
	sort.Strings(multiBases)

	specPaths, _ := spec.child("paths")
	inserted := 0
	for _, base := range multiBases {
		vers := sortedInts(verByBase[base])
		for _, v := range vers {
			full := fmt.Sprintf("/v%d%s", v, base)
			pathItem, ok := specPaths.child(full)
			if !ok {
				continue
			}
			for _, httpm := range httpMethods {
				if !pathItem.has(httpm) {
					continue
				}
				key := opKey{strings.ToUpper(httpm), full}
				if _, exists := cfgOpsByKey[key]; exists {
					continue
				}
				// Find a sibling version already in config, newest first.
				var sibling *omap
				for _, v2 := range sortedIntsDesc(vers) {
					if v2 == v {
						continue
					}
					k2 := opKey{strings.ToUpper(httpm), fmt.Sprintf("/v%d%s", v2, base)}
					cand, ok := cfgOpsByKey[k2]
					if !ok {
						continue
					}
					if indexOf(opsList, cand) < 0 {
						continue
					}
					sibling = cand
					break
				}
				if sibling == nil {
					fmt.Fprintf(os.Stderr, "WARN: no sibling found for %s %s\n", strings.ToUpper(httpm), full)
					continue
				}
				newOp := synthesizeOp(sibling, v, full, spec, schemas)
				newName, _ := newOp.str("name")
				if _, clash := cfgOpsByName[newName]; clash {
					fmt.Fprintf(os.Stderr, "SKIP: name collision %s\n", newName)
					continue
				}
				idx := indexOf(opsList, sibling)
				opsList = append(opsList[:idx], append([]any{newOp}, opsList[idx:]...)...)
				cfgOpsByName[newName] = newOp
				if k, ok := parseOp(fmt.Sprintf("%s %s", strings.ToUpper(httpm), full)); ok {
					cfgOpsByKey[k] = newOp
				}
				inserted++
			}
		}
	}

	proSpec.set("operations", opsList)
	fmt.Printf("inserted %d ops\n", inserted)

	if err := os.WriteFile(cfgPath, encodeOrdered(cfg), 0o644); err != nil {
		log.Fatalf("writing config: %v", err)
	}
}

func sortedInts(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func sortedIntsDesc(vs []int) []int {
	out := append([]int(nil), vs...)
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}
