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
// uses — all 727 acceptance tests agreed when this was written, 727/727. If a
// test is named outside its product's prefix, CI would run it in another lane
// against another credential: `TestAcceptance_ProtectFoo` calling
// accSecurityCloudClient would land in the pro lane and 403 in a way that reads
// as a broken endpoint rather than a misfiled test.
//
// So this asserts the two definitions agree: the lane implied by the NAME, and
// the lane implied by the acc*Client FACTORY the test body calls.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	return table
}

// factoryLane maps the credential factory a test calls to the lane that must own
// it. Precedence matters for the one test that builds two clients: a test that
// touches Security Cloud at all belongs in the Security Cloud lane, because that
// is the credential whose absence must skip it.
var factoryLane = []struct {
	factory string
	lane    string
}{
	{"accSecurityCloudClient", "securitycloud"},
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

// acceptanceTestFactories parses the suite's sources and reports, per test
// function, which acc*Client factories its body calls. Source parsing rather
// than reflection because these files are behind a build tag this test
// deliberately does not set.
func acceptanceTestFactories(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("acc_*_test.go")
	if err != nil {
		t.Fatalf("globbing acceptance sources: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no acc_*_test.go files found — has the suite moved?")
	}

	out := map[string][]string{}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "TestAcceptance_") {
				continue
			}
			var called []string
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				// Every acc*Client call is collected, not only the ones
				// factoryLane knows about, so a new product's factory shows up
				// as an unmapped factory rather than as a credential-free test.
				if credentialFactory.MatchString(ident.Name) {
					called = append(called, ident.Name)
				}
				return true
			})
			out[fn.Name.Name] = called
		}
	}
	return out
}

func nameLane(table laneTable, name string) string {
	for _, lane := range table.Lanes {
		if regexp.MustCompile(lane.Match).MatchString(name) {
			return lane.Lane
		}
	}
	return table.DefaultLane.Lane
}

func expectedLane(called []string) string {
	for _, fl := range factoryLane {
		for _, c := range called {
			if c == fl.factory {
				return fl.lane
			}
		}
	}
	return "" // credential-free: valid in any lane
}

// TestAcceptanceLaneNamesMatchTheirCredential is the load-bearing assertion: a
// test's name must place it in the same lane as the credential it uses.
func TestAcceptanceLaneNamesMatchTheirCredential(t *testing.T) {
	table := loadLaneTable(t)
	tests := acceptanceTestFactories(t)
	if len(tests) == 0 {
		t.Fatal("parsed no TestAcceptance_ functions")
	}

	var mismatches []string
	for name, called := range tests {
		want := expectedLane(called)
		if want == "" {
			continue
		}
		if got := nameLane(table, name); got != want {
			mismatches = append(mismatches, name+": name puts it in "+got+", but it calls "+strings.Join(called, "+")+" so it belongs in "+want)
		}
	}
	sort.Strings(mismatches)
	for _, m := range mismatches {
		t.Error(m)
	}
	if len(mismatches) > 0 {
		t.Logf("rename the test to its lane's prefix (see %s), or add a lane for its product", laneTablePath)
	}
	t.Logf("%d acceptance tests, %d credential-free, all lanes consistent", len(tests), countCredentialFree(tests))
}

func countCredentialFree(tests map[string][]string) int {
	n := 0
	for _, called := range tests {
		if expectedLane(called) == "" {
			n++
		}
	}
	return n
}

// TestAcceptanceLaneTableIsUsable guards the table itself: every pattern must be
// a valid Go regexp, every lane must own at least one test, and no two lanes may
// claim the same test. A pattern Go cannot compile is not a theoretical worry —
// the first draft of the matrix expressed the default lane as a negative
// lookahead, which RE2 rejects outright and `go test -run` would refuse.
func TestAcceptanceLaneTableIsUsable(t *testing.T) {
	table := loadLaneTable(t)
	tests := acceptanceTestFactories(t)

	seen := map[string]int{}
	for _, lane := range table.Lanes {
		re, err := regexp.Compile(lane.Match)
		if err != nil {
			t.Errorf("lane %q: pattern %q does not compile as a Go regexp (go test -run uses RE2, so no lookahead): %v", lane.Lane, lane.Match, err)
			continue
		}
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
