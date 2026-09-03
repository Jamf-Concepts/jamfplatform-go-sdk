// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform_test

// Lane conformance for the acceptance suite.
//
// This file carries NO acceptance build tag on purpose: it needs no credentials
// and no network, and it must run in `make test` and in CI's normal test job.
// The whole point is to catch a misplaced test at PR time rather than on a
// tenant.
//
// # What a lane is, and why the check exists
//
// The acceptance matrix splits the suite by product space, and lane membership
// is decided by a test-name prefix in .github/acceptance-lanes.json. That is
// only sound while the naming convention actually tracks the credential a test
// uses — all 728 acceptance tests agreed when this was written, 728/728. If a
// test is named outside its product's prefix, CI would run it in another lane
// against another credential: `TestAcceptance_ProtectFoo` calling
// accSecurityCloudClient would land in the pro lane and 403 in a way that reads
// as a broken endpoint rather than a misfiled test.
//
// So this asserts the two definitions agree: the lane implied by the NAME, and
// the lane implied by the acc*Client FACTORY the test body calls.
//
// Two further gaps are closed here, because each of them is a way for a lane to
// skip green — which is the failure this whole file exists to make impossible:
//
//   - The lane name and the require token are DIFFERENT vocabularies (lane
//     `account` uses require token `organization`, lane `platform-env` uses
//     `environment`, lane `pro` uses `platform`), and the require token is
//     written twice: once in the JSON, once as a string literal handed to
//     skipOrFailUnset in acc_helpers_test.go. Disagree on one character and
//     accRequiredSets() misses the set, skipOrFailUnset degrades from t.Fatalf
//     to t.Skipf, and the whole lane reports green having run nothing.
//     TestEveryCredentialFactoryRequireTokenMatchesItsLane compares the two.
//
//   - A test can build a live, credentialed client WITHOUT calling any
//     acc*Client factory, by calling jamfplatform.NewClient itself. Such a test
//     used to be indistinguishable from a genuine credential-free no-op stub, so
//     the lane check exempted it. It is now detected and must be allow-listed by
//     name.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const laneTablePath = "../.github/acceptance-lanes.json"

type laneDef struct {
	Lane    string `json:"lane"`
	Match   string `json:"match"`
	Require string `json:"require"`
	Lock    bool   `json:"lock"`
	Planned bool   `json:"planned"`
}

type laneTable struct {
	Lanes       []laneDef `json:"lanes"`
	DefaultLane laneDef   `json:"default_lane"`

	// patterns holds Lanes[i].Match compiled, in the same order. Compiling in
	// loadLaneTable rather than at each use is what keeps a malformed pattern
	// from panicking: regexp.MustCompile in nameLane would take down the whole
	// package's test binary with a raw RE2 syntax error naming no lane, and it
	// would do so before any test that guards the table could report the
	// problem — Go runs a file's tests in declaration order, so which test
	// catches it would otherwise depend on where in this file it is declared.
	patterns []*regexp.Regexp
}

func loadLaneTable(t *testing.T) laneTable {
	t.Helper()
	raw, err := os.ReadFile(laneTablePath)
	if err != nil {
		t.Fatalf("reading %s: %v", laneTablePath, err)
	}
	var table laneTable
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("parsing %s: %v", laneTablePath, err)
	}
	if len(table.Lanes) == 0 || table.DefaultLane.Lane == "" {
		t.Fatalf("%s declares no lanes", laneTablePath)
	}
	// Every pattern is validated here, before any caller can reach it. A
	// pattern Go cannot compile is not a theoretical worry — the first draft of
	// the matrix expressed the default lane as a negative lookahead, which RE2
	// rejects outright and `go test -run` would refuse.
	table.patterns = make([]*regexp.Regexp, 0, len(table.Lanes))
	for _, lane := range table.Lanes {
		if lane.Match == "" {
			t.Fatalf("lane %q in %s declares no match pattern, so it can never own a test", lane.Lane, laneTablePath)
		}
		re, err := regexp.Compile(lane.Match)
		if err != nil {
			t.Fatalf("lane %q in %s: pattern %q does not compile as a Go regexp (go test -run uses RE2, so no lookahead): %v", lane.Lane, laneTablePath, lane.Match, err)
		}
		table.patterns = append(table.patterns, re)
	}
	return table
}

// requireForLane returns the JAMFPLATFORM_ACC_REQUIRE token the table declares
// for a lane, looking in the default lane too since that one is not in Lanes.
func requireForLane(table laneTable, lane string) (string, bool) {
	if lane == table.DefaultLane.Lane {
		return table.DefaultLane.Require, true
	}
	for _, l := range table.Lanes {
		if l.Lane == lane {
			return l.Require, true
		}
	}
	return "", false
}

// factoryLane maps the credential factory a test calls to the lane that must own
// it. Precedence matters for the one test that builds two clients: a test that
// touches Security Cloud at all belongs in the Security Cloud lane, because that
// is the credential whose absence must skip it.
//
// The lane's require token is deliberately NOT repeated here: it is read out of
// the lane table so that a typo in the JSON cannot agree with a typo here.
var factoryLane = []struct {
	factory string
	lane    string
}{
	{"accSecurityCloudClient", "securitycloud"},
	{"accTenantClient", "pro-tenant"},
	{"accOrgClient", "account"},
	{"accEnvClient", "platform-env"},
	{"accClient", "pro"},
}

// credentialFactory matches the naming convention every credential factory in
// acc_helpers_test.go follows. Matching the shape rather than an allow-list is
// what makes a NEW product visible: when Jamf Protect or Jamf School lands with
// an accProtectClient, this picks it up and TestEveryCredentialFactoryOwnsALane
// fails until it has a lane, instead of the new tests being treated as
// credential-free and silently running in the pro lane against pro credentials.
var credentialFactory = regexp.MustCompile(`^acc[A-Za-z]*Client$`)

// rawClientConstructors are the calls that mint a live client outside the
// acc*Client factories. jamfplatform.NewClient is the load-bearing one: it is
// the only call in the SDK that turns credentials into a client, so every
// credentialed request in the suite descends from it. securitycloud.New is
// listed beside it because it is the sole door into the Security Cloud package
// and a helper could be handed a client built elsewhere; the other <pkg>.New
// wrappers cannot be reached without one of these two having run first, so
// listing them would add nothing.
var rawClientConstructors = map[string]bool{
	"jamfplatform.NewClient": true,
	"securitycloud.New":      true,
}

// directClientAllowList names the tests that legitimately build a live client
// without going through an acc*Client factory, and why. An entry is an
// assertion that the test's NAME still agrees with the credential it reads
// straight out of the environment — the very thing the factory path would
// otherwise prove — so each one is spelled out rather than pattern-matched.
//
// Prefer routing a new test through a named factory. An allow-list entry is for
// the case where the factory genuinely cannot serve it, and it costs a reviewer
// reading the environment variables by hand.
var directClientAllowList = map[string]string{
	// Needs a per-call `Accept` header, and WithHeaders applies to every request
	// a client makes, so the suite-wide Security Cloud client cannot carry it.
	// Reaches the credential through the helper jscClientWithHeaders, which
	// reads JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_* — the securitycloud lane's
	// own credential, matching the name's SecurityCloud prefix.
	"TestAcceptance_SecurityCloudUemConnectAcceptNegotiation": "per-call Accept header; reads JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_* via jscClientWithHeaders",

	// Crosses the two scopes over on purpose, so it must build one client per
	// scope with the wrong header attached. No factory can express that: every
	// factory's whole job is to pair a credential with its correct header. It
	// reads both JAMFPLATFORM_ACC_ENVIRONMENT_* and JAMFPLATFORM_ACC_PRO_TENANT_*,
	// and the platform-env lane its EnvironmentScope name selects is the one
	// whose credential it cannot run without.
	"TestAcceptance_EnvironmentScopeMismatch": "deliberately mismatches header and credential, which no factory can build; reads JAMFPLATFORM_ACC_ENVIRONMENT_* and JAMFPLATFORM_ACC_PRO_TENANT_*",

	// Asserts two independently constructed clients share one on-disk token
	// cache, so it needs the constructor twice with its own WithFileTokenCache
	// directory; a memoised factory would hand back the same client and assert
	// nothing. Reads JAMFPLATFORM_ACC_PRO_TENANT_*, which is the default pro
	// lane its unprefixed name selects.
	"TestAcceptance_FileTokenCache": "needs two separately constructed clients to share a token cache; reads JAMFPLATFORM_ACC_PRO_TENANT_*",
}

// accFunc records what one function in the acceptance suite calls. Source
// parsing rather than reflection because these files are behind a build tag
// this test deliberately does not set.
type accFunc struct {
	// idents are the bare-identifier calls in the body, e.g. accClient.
	idents map[string]bool
	// selectors are the qualified calls in the body, keyed "pkg.Fun".
	selectors map[string]bool
	// requireTokens are the string literals passed as skipOrFailUnset's `set`
	// argument. Only a credential factory has any.
	requireTokens []string
}

// parseAcceptanceSuite parses every acc_*_test.go and reports, per top-level
// function, what it calls. ast.Inspect walks into t.Run closures, which is
// load-bearing: TestAcceptance_EnvironmentScopeMismatch builds both its clients
// inside subtests.
func parseAcceptanceSuite(t *testing.T) map[string]accFunc {
	t.Helper()
	files, err := filepath.Glob("acc_*_test.go")
	if err != nil {
		t.Fatalf("globbing acceptance sources: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no acc_*_test.go files found — has the suite moved?")
	}

	out := map[string]accFunc{}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil {
				continue
			}
			info := accFunc{idents: map[string]bool{}, selectors: map[string]bool{}}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					info.idents[fun.Name] = true
					if fun.Name == "skipOrFailUnset" {
						if tok, ok := stringLitArg(call, 1); ok {
							info.requireTokens = append(info.requireTokens, tok)
						}
					}
				case *ast.SelectorExpr:
					// Only pkg.Fun shapes; a method call on a value tells us
					// nothing about which package minted the client.
					if pkg, ok := fun.X.(*ast.Ident); ok {
						info.selectors[pkg.Name+"."+fun.Sel.Name] = true
					}
				}
				return true
			})
			out[fn.Name.Name] = info
		}
	}
	return out
}

// stringLitArg returns call's i'th argument as an unquoted string literal.
func stringLitArg(call *ast.CallExpr, i int) (string, bool) {
	if i >= len(call.Args) {
		return "", false
	}
	lit, ok := call.Args[i].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// acceptanceTestFactories reports, per test function, which acc*Client factories
// its body calls.
func acceptanceTestFactories(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for name, info := range parseAcceptanceSuite(t) {
		if !strings.HasPrefix(name, "TestAcceptance_") {
			continue
		}
		var called []string
		// Every acc*Client call is collected, not only the ones factoryLane
		// knows about, so a new product's factory shows up as an unmapped
		// factory rather than as a credential-free test.
		for ident := range info.idents {
			if credentialFactory.MatchString(ident) {
				called = append(called, ident)
			}
		}
		sort.Strings(called)
		out[name] = called
	}
	return out
}

// liveClientBuilders returns the suite's non-test helpers that end up calling a
// raw client constructor, transitively.
//
// Propagation deliberately STOPS at a credential factory: accClient reaches
// jamfplatform.NewClient through initAcceptanceClient, and following that edge
// would mark every helper built on accClient as a raw builder. A factory's
// callers are already classified by factoryLane, so what is wanted here is only
// the paths that bypass the factories.
func liveClientBuilders(funcs map[string]accFunc) map[string]bool {
	builders := map[string]bool{}
	for {
		grew := false
		for name, info := range funcs {
			if builders[name] || strings.HasPrefix(name, "TestAcceptance_") || credentialFactory.MatchString(name) {
				continue
			}
			if buildsLiveClient(info, builders) {
				builders[name] = true
				grew = true
			}
		}
		if !grew {
			return builders
		}
	}
}

func buildsLiveClient(info accFunc, builders map[string]bool) bool {
	for sel := range info.selectors {
		if rawClientConstructors[sel] {
			return true
		}
	}
	for ident := range info.idents {
		if builders[ident] {
			return true
		}
	}
	return false
}

// directClientTests reports the tests that build a live client without going
// through an acc*Client factory — directly, or through a helper that does.
//
// Before this existed, such a test looked exactly like a genuine no-op skip
// stub: acceptanceTestFactories collected only bare-identifier calls, so
// `jamfplatform.NewClient(...)` (a selector) was invisible, expectedLane
// returned "" and the lane check skipped the test entirely.
func directClientTests(t *testing.T) map[string]bool {
	t.Helper()
	funcs := parseAcceptanceSuite(t)
	builders := liveClientBuilders(funcs)
	out := map[string]bool{}
	for name, info := range funcs {
		if !strings.HasPrefix(name, "TestAcceptance_") {
			continue
		}
		if buildsLiveClient(info, builders) {
			out[name] = true
		}
	}
	return out
}

func nameLane(table laneTable, name string) string {
	for i, lane := range table.Lanes {
		if table.patterns[i].MatchString(name) {
			return lane.Lane
		}
	}
	return table.DefaultLane.Lane
}

func expectedLane(called []string) string {
	for _, fl := range factoryLane {
		if slices.Contains(called, fl.factory) {
			return fl.lane
		}
	}
	return "" // credential-free, or a client built without a factory
}

// TestAcceptanceLaneNamesMatchTheirCredential is the load-bearing assertion: a
// test's name must place it in the same lane as the credential it uses.
func TestAcceptanceLaneNamesMatchTheirCredential(t *testing.T) {
	table := loadLaneTable(t)
	tests := acceptanceTestFactories(t)
	if len(tests) == 0 {
		t.Fatal("parsed no TestAcceptance_ functions")
	}
	direct := directClientTests(t)

	var mismatches []string
	for name, called := range tests {
		want := expectedLane(called)
		if want == "" {
			// No factory. Either a genuine credential-free test, which any lane
			// may run, or one that built a client itself — which must be
			// allow-listed, because nothing here can tell which credential it
			// read out of the environment.
			if direct[name] && directClientAllowList[name] == "" {
				mismatches = append(mismatches, name+": builds a live client without an acc*Client factory, so its lane cannot be checked against its credential. Route it through a named factory, or add it to directClientAllowList with the credential it reads and why no factory can serve it")
			}
			continue
		}
		if got := nameLane(table, name); got != want {
			mismatches = append(mismatches, name+": name puts it in "+got+", but it calls "+strings.Join(called, "+")+" so it belongs in "+want)
		}
	}
	// A stale allow-list entry is as bad as a missing one: it silently exempts
	// the next test that reuses the name.
	for name, why := range directClientAllowList {
		if _, ok := tests[name]; !ok {
			mismatches = append(mismatches, name+": in directClientAllowList ("+why+") but the suite has no such test — drop the entry")
			continue
		}
		if !direct[name] {
			mismatches = append(mismatches, name+": in directClientAllowList ("+why+") but no longer builds a client of its own — drop the entry")
		}
	}
	sort.Strings(mismatches)
	for _, m := range mismatches {
		t.Error(m)
	}
	if len(mismatches) > 0 {
		t.Logf("rename the test to its lane's prefix (see %s), or add a lane for its product", laneTablePath)
		t.Logf("%d acceptance tests, %d credential-free, %d building their own client",
			len(tests), countCredentialFree(tests, direct), len(direct))
		return
	}
	t.Logf("%d acceptance tests, %d credential-free, %d building their own client (all allow-listed), all lanes consistent",
		len(tests), countCredentialFree(tests, direct), len(direct))
}

// countCredentialFree counts the tests that need no credential at all. A test
// that builds its own client is not one of them, however invisible it is to
// expectedLane.
func countCredentialFree(tests map[string][]string, direct map[string]bool) int {
	n := 0
	for name, called := range tests {
		if expectedLane(called) == "" && !direct[name] {
			n++
		}
	}
	return n
}

// TestEveryCredentialFactoryRequireTokenMatchesItsLane pins the two vocabularies
// together.
//
// A lane's `require` token is written twice and the two spellings are not the
// same word: .github/acceptance-lanes.json says `require: "organization"` for
// lane `account`, and acc_helpers_test.go's accOrgClient says
// skipOrFailUnset(t, "organization", err). Nothing but this test compares them,
// and a one-character disagreement is silent AND green: accRequiredSets() misses
// the set, skipOrFailUnset takes t.Skipf instead of t.Fatalf, and the lane
// reports success having run nothing — the exact skip-into-green failure the
// require mechanism exists to prevent.
func TestEveryCredentialFactoryRequireTokenMatchesItsLane(t *testing.T) {
	table := loadLaneTable(t)
	funcs := parseAcceptanceSuite(t)

	for _, fl := range factoryLane {
		info, ok := funcs[fl.factory]
		if !ok {
			t.Errorf("factoryLane maps %s but no such function exists in acc_*_test.go", fl.factory)
			continue
		}
		want, ok := requireForLane(table, fl.lane)
		if !ok {
			t.Errorf("%s maps to lane %q, which %s does not declare", fl.factory, fl.lane, laneTablePath)
			continue
		}
		if want == "" {
			t.Errorf("lane %q has no require token in %s, so %s would skip silently for a missing credential", fl.lane, laneTablePath, fl.factory)
			continue
		}
		switch len(info.requireTokens) {
		case 0:
			t.Errorf("%s calls skipOrFailUnset with no literal require token (or does not call it at all), so JAMFPLATFORM_ACC_REQUIRE=%s cannot promote its skip to a failure", fl.factory, want)
			continue
		case 1:
		default:
			if len(slices.Compact(slices.Sorted(slices.Values(info.requireTokens)))) > 1 {
				t.Errorf("%s passes %d different require tokens to skipOrFailUnset (%s) — one factory, one credential set, one token", fl.factory, len(info.requireTokens), strings.Join(info.requireTokens, ", "))
				continue
			}
		}
		if got := info.requireTokens[0]; got != want {
			t.Errorf("require token disagreement for lane %q: %s passes %q to skipOrFailUnset but %s declares %q. accRequiredSets() is keyed on the literal, so the mismatch makes JAMFPLATFORM_ACC_REQUIRE=%s a no-op and the lane skips green",
				fl.lane, fl.factory, got, laneTablePath, want, want)
		}
	}
}

// TestAcceptanceLaneTableIsUsable guards the table itself: every lane must own at
// least one test, and no two lanes may claim the same test. The patterns are
// compiled in loadLaneTable, so a pattern Go cannot compile fails there — named,
// and regardless of which test in this file runs first.
func TestAcceptanceLaneTableIsUsable(t *testing.T) {
	table := loadLaneTable(t)
	tests := acceptanceTestFactories(t)

	seen := map[string]int{}
	for i, lane := range table.Lanes {
		re := table.patterns[i]
		count := 0
		for name := range tests {
			if re.MatchString(name) {
				count++
				seen[name]++
			}
		}
		switch {
		case lane.Planned && count > 0:
			t.Errorf("lane %q is still marked planned but matches %d test(s): its product has arrived, so add a credential factory and a factoryLane row, wire the JAMFPLATFORM_ACC_%s_* secrets, then drop \"planned\" from %s",
				lane.Lane, count, strings.ToUpper(lane.Require), laneTablePath)
		case !lane.Planned && count == 0:
			t.Errorf("lane %q matches no test — a lane that runs nothing reports a passing check having asserted nothing. Mark it \"planned\": true if the product has not landed yet", lane.Lane)
		}
		if lane.Require == "" {
			t.Errorf("lane %q has no require token, so a missing credential would skip it silently", lane.Lane)
		}
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%s is claimed by %d lanes — first match wins, so lane order is silently deciding this", name, n)
		}
	}
	if table.DefaultLane.Require == "" {
		t.Error("default lane has no require token")
	}
	if !table.DefaultLane.Lock {
		t.Error("default lane must hold the shared-tenant lock: it is the lane that mutates the Jamf Pro tenant")
	}
}

// TestEveryCredentialFactoryOwnsALane closes the gap that adding a product opens.
//
// factoryLane is a hand-written table, so a new product is two edits: a factory
// in acc_helpers_test.go and a row here. Miss the second and expectedLane
// returns "" for every test of that product — the same answer it gives a
// genuinely credential-free test — so the lane check would pass while those
// tests ran in the pro lane against pro credentials, 403ing in a way that reads
// as a broken endpoint. This makes the omission a failure instead.
func TestEveryCredentialFactoryOwnsALane(t *testing.T) {
	table := loadLaneTable(t)
	tests := acceptanceTestFactories(t)

	mapped := map[string]string{}
	for _, fl := range factoryLane {
		mapped[fl.factory] = fl.lane
	}
	lanes := map[string]bool{table.DefaultLane.Lane: true}
	for _, lane := range table.Lanes {
		lanes[lane.Lane] = true
	}

	seen := map[string]bool{}
	for _, called := range tests {
		for _, c := range called {
			seen[c] = true
		}
	}

	for factory := range seen {
		lane, ok := mapped[factory]
		if !ok {
			t.Errorf("%s is called by the suite but maps to no lane: add it to factoryLane, and add its lane to %s", factory, laneTablePath)
			continue
		}
		if !lanes[lane] {
			t.Errorf("%s maps to lane %q, which %s does not declare", factory, lane, laneTablePath)
		}
	}
	for factory := range mapped {
		if !seen[factory] {
			t.Errorf("factoryLane maps %s but no test calls it — a stale row will mis-file the next product that reuses the name", factory)
		}
	}
}
