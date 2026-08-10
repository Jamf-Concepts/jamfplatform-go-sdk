// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Command acctargets prints the minimal set of acceptance tests that must run
// for a change set. It exists so a surgical PR doesn't pay for the whole ~10
// minute serial suite against a shared tenant.
//
// # Why this is not a package-level tool
//
// terraform-provider-jamfplatform scopes its acceptance run by Go package
// (scripts/acctargets there): changed packages plus their reverse-dependency
// closure, fed to `go test <import paths>`. That works because each provider
// resource is its own package.
//
// This repo cannot use that shape. Every acceptance test lives in ONE package —
// `jamfplatform_test`, the external test package in jamfplatform/. Any change
// under jamfplatform/** dirties that single package, so package-level scoping
// always resolves to the full suite. Selection therefore has to happen at
// test-function granularity, via `go test -run`.
//
// # How scope is computed
//
//  1. Changed declarations, not changed lines. Each changed .go file is parsed
//     at the merge base and at HEAD; top-level declarations are compared by
//     printed source (doc comments excluded, so a comment-only edit is a no-op).
//     The result is a set of declaration NAMES — including unexported ones.
//
//  2. A module-wide name-reference graph. Every .go file in the module (SDK
//     packages and test files alike) contributes one node per top-level
//     declaration, carrying the set of declared names its body references.
//     Edges are matched on bare identifier name rather than resolved type
//     identity, which can over-select when a name is shared across packages but
//     never under-selects.
//
//  3. Forward closure through callers. Starting from the changed names, the
//     graph is walked in the referencing direction: a changed unexported helper
//     reaches its exported callers, which reach the tests. This step is
//     load-bearing, not an optimisation. PR #45 changed only the unexported
//     proclassic.minimizePlistSourceEscaping; every acceptance test that
//     exercises it does so through PayloadsXMLText and then through test
//     helpers (payloadsXMLPtr, ptrProclassicPayloads, createWifiProfileFixture).
//     A direct grep for the changed identifier selects zero tests. The closure
//     selects eleven.
//
//  4. Tests in the closure are emitted as a `-run` regex.
//
// # Fail-safe behaviour
//
// Anything the tool cannot attribute confidently resolves to ALL. That includes
// the shared transport (internal/**), the handwritten client, the shared
// acceptance helpers, CI surface, and any changed non-Go file that isn't on the
// ignore list. Over-running is a cost; under-running is a missed regression.
//
// Changes under tools/generate/** and api/** are deliberately IGNORED. All
// generated output is committed, and the source specs under testing/ are
// gitignored, so a change set's effective SDK surface is exactly its committed
// .go diff. A generator edit whose regenerated files are absent from the diff
// has not changed anything the acceptance suite can exercise; when the files ARE
// present, they are scoped precisely by declaration. CI's "Generated Code" job
// remains the check that regeneration was actually committed.
//
// # Output
//
// Exactly one line on stdout:
//
//	ALL              run the whole suite
//	NONE             nothing acceptance-relevant changed
//	^(TestA|TestB)$  pass verbatim to `go test -run`
//
// # Usage
//
//	cd tools && go run ./acctargets [baseRef]     # baseRef default: origin/main
//	BASE_REF=origin/main go run ./acctargets
//	ACCTARGETS_MAX_TESTS=250 go run ./acctargets  # above this, prefer ALL
package main

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// maxTestsDefault caps how many individual tests are worth naming in a -run
// regex. Past this point the change is broad (a spec bump regenerating types.go,
// say) and a single full run is both simpler and less likely to be wrong than a
// 700-alternative regex.
const maxTestsDefault = 250

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "acctargets:", err)
		os.Exit(1)
	}
}

// debugf writes scope-derivation detail to stderr when ACCTARGETS_DEBUG is set,
// so a surprising scope in CI can be explained without a local repro. stdout
// stays machine-readable either way.
func debugf(format string, args ...any) {
	if os.Getenv("ACCTARGETS_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "acctargets: "+format+"\n", args...)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func run() error {
	baseRef := os.Getenv("BASE_REF")
	if len(os.Args) > 1 && os.Args[1] != "" {
		baseRef = os.Args[1]
	}
	if baseRef == "" {
		baseRef = "origin/main"
	}

	root, err := gitOutput("", "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("locating repo root: %w", err)
	}
	root = strings.TrimSpace(root)

	diffBase, err := resolveDiffBase(root, baseRef)
	if err != nil {
		return err
	}

	changedPaths, err := changedFiles(root, diffBase)
	if err != nil {
		return fmt.Errorf("computing changed files: %w", err)
	}

	// Classify the change set. A single global or unattributable file is enough
	// to force the full suite, so bail as soon as one shows up.
	var goFiles []string
	for _, p := range changedPaths {
		switch classifyFile(p) {
		case fileIgnore:
			continue
		case fileGlobal:
			fmt.Println("ALL")
			return nil
		case fileGo:
			goFiles = append(goFiles, p)
		}
	}
	if len(goFiles) == 0 {
		fmt.Println("NONE")
		return nil
	}

	// The reference graph spans the whole module, not just the changed files:
	// the path from a changed declaration to a test usually runs through
	// untouched code.
	idx, err := indexModule(root)
	if err != nil {
		return fmt.Errorf("indexing module: %w", err)
	}

	changedNames, err := changedDeclNames(root, diffBase, goFiles)
	if err != nil {
		return err
	}
	if len(changedNames) == 0 {
		// Every changed .go file differed only in doc comments, imports, or
		// other non-declaration text.
		fmt.Println("NONE")
		return nil
	}

	tests := idx.testsAffectedBy(changedNames)
	debugf("changed declarations (%d): %s", len(changedNames), strings.Join(sortedKeys(changedNames), " "))
	debugf("acceptance tests selected: %d", len(tests))
	if len(tests) == 0 {
		fmt.Println("NONE")
		return nil
	}

	maxTests := maxTestsDefault
	if v := os.Getenv("ACCTARGETS_MAX_TESTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTests = n
		}
	}
	if len(tests) > maxTests {
		fmt.Println("ALL")
		return nil
	}

	sort.Strings(tests)
	fmt.Printf("^(%s)$\n", strings.Join(tests, "|"))
	return nil
}

// --- module index -----------------------------------------------------------

// decl is one top-level declaration: a func, method, type, var, or const.
// Methods are keyed by bare method name — receiver types are not resolved, which
// is what makes the graph type-free (and conservatively over-linked).
type decl struct {
	name string
	// recvType is set for methods, and implicitRecv marks the subset whose
	// callers are invisible to a name-matched graph (see implicitDispatch).
	// Reaching such a method during the walk continues through its receiver type.
	recvType     string
	implicitRecv bool
	isTest       bool            // `func TestXxx` in an acceptance-tagged file
	refs         map[string]bool // declared names referenced by this declaration
}

type moduleIndex struct {
	decls []*decl
	// byRef maps a declared name to every declaration that references it, i.e.
	// the edges are already reversed for a caller-ward walk.
	byRef map[string][]*decl
}

func indexModule(root string) (*moduleIndex, error) {
	files, err := moduleGoFiles(root)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	idx := &moduleIndex{byRef: map[string][]*decl{}}
	declared := map[string]bool{}

	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// ParseComments is required: build constraints are comments, and
		// //go:build acceptance is what marks a file as part of the suite.
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			// A file that doesn't parse can't be reasoned about. Treat the whole
			// run as unscopeable rather than silently dropping its edges.
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		acceptance := hasAcceptanceConstraint(f)

		for _, d := range topLevelDecls(f, fset, src) {
			dd := &decl{
				name:         d.name,
				recvType:     d.recvType,
				implicitRecv: d.recvType != "" && implicitDispatch[d.name],
				isTest:       acceptance && isTestFuncName(d.name) && d.isFunc && !d.hasRecv,
				refs:         referencedNames(d.node),
			}
			idx.decls = append(idx.decls, dd)
			declared[dd.name] = true
		}
	}

	// Prune references down to names the module actually declares. Without this
	// every `err`, `ctx`, and `t` in the tree becomes a graph node.
	for _, d := range idx.decls {
		for name := range d.refs {
			if !declared[name] || name == d.name {
				delete(d.refs, name)
				continue
			}
			idx.byRef[name] = append(idx.byRef[name], d)
		}
	}
	return idx, nil
}

// testsAffectedBy walks from the changed names towards the code that references
// them and returns the acceptance tests reached.
func (idx *moduleIndex) testsAffectedBy(changed map[string]bool) []string {
	seenName := map[string]bool{}
	seenDecl := map[*decl]bool{}
	var queue []string

	enqueue := func(name string) {
		if name != "" && !seenName[name] {
			seenName[name] = true
			queue = append(queue, name)
		}
	}
	for name := range changed {
		enqueue(name)
	}

	found := map[string]bool{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		// A changed declaration may itself be a test.
		for _, d := range idx.decls {
			if d.name == name && d.isTest {
				found[d.name] = true
			}
		}

		for _, d := range idx.byRef[name] {
			if seenDecl[d] {
				continue
			}
			seenDecl[d] = true
			if d.isTest {
				found[d.name] = true
				// A test is a leaf: nothing references it, so there is no
				// reason to keep walking from its name.
				continue
			}
			// An implicitly-dispatched method is a dead end by name — nothing
			// mentions MarshalXML, encoding/xml calls it — so the walk has to
			// continue through the receiver type instead. Without this, a change
			// to a helper called only by such a method reaches no tests at all.
			if d.implicitRecv {
				enqueue(d.recvType)
			}
			enqueue(d.name)
		}
	}

	out := make([]string, 0, len(found))
	for name := range found {
		out = append(out, name)
	}
	return out
}

// --- declaration extraction -------------------------------------------------

type rawDecl struct {
	name     string
	recvType string // receiver type for methods, empty otherwise
	node     ast.Node
	body     string // printed source, doc comment excluded
	isFunc   bool
	hasRecv  bool
}

// key identifies a declaration within a file. Methods are qualified by receiver
// so that editing PayloadsXMLText.MarshalXML doesn't also mark BigInt.MarshalXML
// as changed — the three MarshalXML implementations share one file.
func (d rawDecl) key() string {
	if d.recvType != "" {
		return d.recvType + "." + d.name
	}
	return d.name
}

// recvTypeName extracts the bare receiver type name, dropping any pointer and
// type-parameter decoration.
func recvTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.IndexExpr: // generic receiver: Foo[T]
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}

// topLevelDecls flattens a file into individually addressable declarations.
// A `type ( A struct{}; B struct{} )` block yields one entry per spec so that
// editing A doesn't mark B as changed.
func topLevelDecls(f *ast.File, fset *token.FileSet, src []byte) []rawDecl {
	var out []rawDecl
	text := func(n ast.Node) string {
		start := fset.Position(n.Pos()).Offset
		end := fset.Position(n.End()).Offset
		if start < 0 || end > len(src) || start >= end {
			return ""
		}
		return string(src[start:end])
	}

	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			out = append(out, rawDecl{
				name:     d.Name.Name,
				recvType: recvTypeName(d.Recv),
				node:     d,
				body:     text(d),
				isFunc:   true,
				hasRecv:  d.Recv != nil,
			})
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					out = append(out, rawDecl{name: s.Name.Name, node: s, body: text(s)})
				case *ast.ValueSpec:
					for _, n := range s.Names {
						out = append(out, rawDecl{name: n.Name, node: s, body: text(s)})
					}
				}
			}
		}
	}
	return out
}

// referencedNames collects every identifier a declaration mentions: bare idents
// (locals and package-level alike) and selector fields, which is how method
// calls such as pc.GetVPPInvitationByID enter the graph.
func referencedNames(n ast.Node) map[string]bool {
	refs := map[string]bool{}
	ast.Inspect(n, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.Ident:
			refs[x.Name] = true
		case *ast.SelectorExpr:
			refs[x.Sel.Name] = true
		}
		return true
	})
	return refs
}

// --- changed-declaration diff -----------------------------------------------

// implicitDispatch lists method names the runtime or stdlib calls without any
// syntactic reference at the call site. A name-matched graph cannot see those
// edges: nothing in this repo mentions MarshalXML, yet every proclassic profile
// write goes through PayloadsXMLText.MarshalXML via encoding/xml.
//
// When one of these methods changes, the change is attributed to its RECEIVER
// TYPE instead, so everything that flows a value of that type is pulled in.
var implicitDispatch = map[string]bool{
	"MarshalXML": true, "UnmarshalXML": true,
	"MarshalJSON": true, "UnmarshalJSON": true,
	"MarshalText": true, "UnmarshalText": true,
	"String": true, "Error": true,
}

// changedDeclNames returns the graph names affected by declarations that were
// added, removed, or whose body changed between diffBase and the working tree.
func changedDeclNames(root, diffBase string, goFiles []string) (map[string]bool, error) {
	changed := map[string]bool{}
	for _, rel := range goFiles {
		before, err := fileDeclsAtRef(root, diffBase, rel)
		if err != nil {
			return nil, err
		}
		after, err := fileDeclsOnDisk(root, rel)
		if err != nil {
			return nil, err
		}

		for key, d := range after {
			if b, ok := before[key]; !ok || b.hash != d.hash {
				markChanged(changed, d)
			}
		}
		for key, d := range before {
			if _, ok := after[key]; !ok {
				markChanged(changed, d)
			}
		}
	}
	return changed, nil
}

// markChanged records the graph entry points for one changed declaration: its
// own name, plus its receiver type when the method is dispatched implicitly.
func markChanged(changed map[string]bool, d declMeta) {
	changed[d.name] = true
	if d.recvType != "" && implicitDispatch[d.name] {
		changed[d.recvType] = true
	}
}

// declMeta is the per-declaration fingerprint used for diffing.
type declMeta struct {
	name     string
	recvType string
	hash     string
}

func fileDeclsAtRef(root, ref, rel string) (map[string]declMeta, error) {
	src, err := gitOutput(root, "show", ref+":"+rel)
	if err != nil {
		// Added in this branch, or renamed from another path.
		return map[string]declMeta{}, nil
	}
	return fileDecls(rel, []byte(src))
}

func fileDeclsOnDisk(root, rel string) (map[string]declMeta, error) {
	src, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		if os.IsNotExist(err) {
			// Deleted or renamed away; every base declaration counts as changed.
			return map[string]declMeta{}, nil
		}
		return nil, err
	}
	return fileDecls(rel, src)
}

func fileDecls(rel string, src []byte) (map[string]declMeta, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", rel, err)
	}
	out := map[string]declMeta{}
	for _, d := range topLevelDecls(f, fset, src) {
		sum := sha256.Sum256([]byte(d.body))
		k := d.key()
		m := out[k]
		m.name, m.recvType = d.name, d.recvType
		// A key can repeat (a var and a func of the same name in one file);
		// concatenating means either one changing counts.
		m.hash += fmt.Sprintf("%x", sum[:8])
		out[k] = m
	}
	return out, nil
}

// --- file discovery and classification --------------------------------------

// moduleGoFiles lists the .go files that can carry a path from a change to an
// acceptance test: the SDK packages and their tests. Generator sources, vendored
// specs, and published specs are excluded — see the package doc on why generator
// changes are scoped through their regenerated output instead.
func moduleGoFiles(root string) ([]string, error) {
	var out []string
	skipDirs := map[string]bool{
		".git": true, "tools": true, "api": true, "testing": true, "docs": true, "examples": true,
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if rel != "." && skipDirs[strings.Split(rel, string(filepath.Separator))[0]] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// hasAcceptanceConstraint reports whether the file is guarded by
// `//go:build acceptance`, which is what distinguishes an acceptance test from
// a generated unit test.
func hasAcceptanceConstraint(f *ast.File) bool {
	for _, group := range f.Comments {
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, "//go:build ") && strings.Contains(c.Text, "acceptance") {
				return true
			}
		}
		// Build constraints precede the package clause; anything after it is
		// ordinary doc text.
		if group.Pos() > f.Package {
			break
		}
	}
	return false
}

func isTestFuncName(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(name) == len("Test") {
		return false
	}
	// Test followed by a lowercase letter is not a test function per `go test`.
	r := name[len("Test")]
	return r < 'a' || r > 'z'
}

type fileClass int

const (
	fileGo     fileClass = iota // attributable Go source
	fileGlobal                  // forces the full suite
	fileIgnore                  // cannot affect acceptance results
)

func classifyFile(path string) fileClass {
	base := filepath.Base(path)

	// Global: shared machinery whose blast radius is the entire suite, plus the
	// CI surface and this tool itself.
	switch {
	case path == "go.mod" || path == "go.sum":
		return fileGlobal
	case base == "GNUmakefile" || base == "Makefile":
		return fileGlobal
	case strings.HasPrefix(path, ".github/workflows/"):
		return fileGlobal
	case strings.HasPrefix(path, "tools/acctargets/"):
		return fileGlobal
	case strings.HasPrefix(path, "internal/"):
		// The transport: auth, marshalling, pagination, polling, error mapping.
		return fileGlobal
	case path == "jamfplatform/client.go":
		return fileGlobal
	case path == "jamfplatform/acc_helpers_test.go" || path == "jamfplatform/acc_main_test.go":
		// Shared fixtures and TestMain. Name-level scoping would work here, but
		// these files gate credential setup and cleanup for everything.
		return fileGlobal
	}

	// Ignore: prose, published artefacts, and the generator. A generator or spec
	// change reaches the SDK only via regenerated files in the same diff, which
	// scope precisely; the caller must verify generated output is current first.
	switch {
	case strings.HasSuffix(base, ".md"):
		return fileIgnore
	case strings.HasPrefix(path, "docs/") || strings.HasPrefix(path, "examples/"):
		return fileIgnore
	case strings.HasPrefix(path, "api/") || strings.HasPrefix(path, "testing/"):
		return fileIgnore
	case strings.HasPrefix(path, "tools/"):
		return fileIgnore
	case strings.HasPrefix(path, ".github/"):
		return fileIgnore
	case base == "LICENSE" || base == "CODEOWNERS" || base == ".gitignore" ||
		base == ".golangci.yml" || base == ".copywrite.hcl":
		return fileIgnore
	}

	if strings.HasSuffix(path, ".go") {
		return fileGo
	}
	// Unrecognised: testdata fixtures, new config, anything else. Be safe.
	return fileGlobal
}

// --- git helpers ------------------------------------------------------------

// resolveDiffBase prefers the merge base so the diff shows only what this branch
// introduced, and falls back to the ref itself on a shallow checkout that lacks
// the common ancestor.
func resolveDiffBase(root, baseRef string) (string, error) {
	if out, err := gitOutput(root, "merge-base", baseRef, "HEAD"); err == nil {
		if mb := strings.TrimSpace(out); mb != "" {
			return mb, nil
		}
	}
	if _, err := gitOutput(root, "rev-parse", "--verify", baseRef); err != nil {
		return "", fmt.Errorf("base ref %q not found (fetch it, or pass an existing ref)", baseRef)
	}
	return baseRef, nil
}

func changedFiles(root, diffBase string) ([]string, error) {
	set := map[string]bool{}
	out, err := gitOutput(root, "diff", "--name-only", diffBase)
	if err != nil {
		return nil, err
	}
	addLines(set, out)
	// Untracked files matter for local runs; in CI there are none.
	if out, err := gitOutput(root, "ls-files", "--others", "--exclude-standard"); err == nil {
		addLines(set, out)
	}

	files := make([]string, 0, len(set))
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

func addLines(set map[string]bool, out string) {
	for l := range strings.SplitSeq(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			set[l] = true
		}
	}
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
