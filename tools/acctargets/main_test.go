// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

func TestClassifyFile(t *testing.T) {
	tests := []struct {
		path string
		want fileClass
	}{
		// Scopeable Go source.
		{"jamfplatform/proclassic/xml_helpers.go", fileGo},
		{"jamfplatform/pro/types.go", fileGo},
		{"jamfplatform/acc_proclassic_test.go", fileGo},

		// Global: shared machinery, CI surface, this tool.
		{"go.mod", fileGlobal},
		{"go.sum", fileGlobal},
		{"GNUmakefile", fileGlobal},
		{".github/workflows/acceptance.yml", fileGlobal},
		{"tools/acctargets/main.go", fileGlobal},
		{"internal/client/client.go", fileGlobal},
		{"jamfplatform/client.go", fileGlobal},
		{"jamfplatform/acc_helpers_test.go", fileGlobal},
		{"jamfplatform/acc_main_test.go", fileGlobal},

		// Global by fail-safe: unrecognised non-Go files could be fixtures.
		{"jamfplatform/testdata/profile.plist", fileGlobal},

		// Ignore: prose, published artefacts, generator, non-workflow CI config.
		{"README.md", fileIgnore},
		{"CLAUDE.md", fileIgnore},
		{"LICENSE", fileIgnore},
		{"CODEOWNERS", fileIgnore},
		{"api/pro_api.json", fileIgnore},
		{"testing/openapi-jpapi.json", fileIgnore},
		{"tools/generate/emit.go", fileIgnore},
		{"tools/generate/config.json", fileIgnore},
		{".github/dependabot.yml", fileIgnore},
	}
	for _, tc := range tests {
		if got := classifyFile(tc.path); got != tc.want {
			t.Errorf("classifyFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsTestFuncName(t *testing.T) {
	tests := map[string]bool{
		"TestAcceptance_Classic_VPPInvitationRead": true,
		"TestFoo":      true,
		"Test":         false,
		"Testing":      false, // lowercase after Test
		"TestMain":     true,
		"BenchmarkFoo": false,
		"helperFunc":   false,
	}
	for name, want := range tests {
		if got := isTestFuncName(name); got != want {
			t.Errorf("isTestFuncName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFileDeclsKeysAndMeta(t *testing.T) {
	src := []byte(`package p

type PayloadsXMLText string

func (p PayloadsXMLText) MarshalXML(e any, s any) error { return nil }

func (b BigInt) MarshalXML(e any, s any) error { return nil }

func helper() string { return "a" }

type Config struct{ Name string }

var Sentinel = 1

const Answer = 42
`)
	decls, err := fileDecls("p.go", src)
	if err != nil {
		t.Fatalf("fileDecls: %v", err)
	}

	// Methods are keyed by receiver so same-named methods stay independent.
	for _, key := range []string{
		"PayloadsXMLText.MarshalXML", "BigInt.MarshalXML",
		"helper", "PayloadsXMLText", "Config", "Sentinel", "Answer",
	} {
		if _, ok := decls[key]; !ok {
			t.Errorf("missing declaration key %q (got %v)", key, keysOf(decls))
		}
	}
	if got := decls["PayloadsXMLText.MarshalXML"].recvType; got != "PayloadsXMLText" {
		t.Errorf("recvType = %q, want PayloadsXMLText", got)
	}
	if decls["PayloadsXMLText.MarshalXML"].hash == decls["BigInt.MarshalXML"].hash {
		t.Error("distinct method bodies hashed identically")
	}
}

func TestFileDeclsIgnoresDocComments(t *testing.T) {
	before, err := fileDecls("p.go", []byte("package p\n\n// Old doc.\nfunc F() int { return 1 }\n"))
	if err != nil {
		t.Fatalf("fileDecls: %v", err)
	}
	after, err := fileDecls("p.go", []byte("package p\n\n// New doc, rewritten entirely.\nfunc F() int { return 1 }\n"))
	if err != nil {
		t.Fatalf("fileDecls: %v", err)
	}
	if before["F"].hash != after["F"].hash {
		t.Error("doc-comment-only edit changed the declaration hash; comment churn would trigger acceptance runs")
	}
}

func TestMarkChangedEscalatesImplicitDispatch(t *testing.T) {
	// A changed MarshalXML is only ever invoked by encoding/xml, so the change
	// must be attributed to the receiver type for the graph to find its tests.
	changed := map[string]bool{}
	markChanged(changed, declMeta{name: "MarshalXML", recvType: "PayloadsXMLText"})
	if !changed["PayloadsXMLText"] {
		t.Error("MarshalXML change did not escalate to its receiver type")
	}

	// An ordinary method must NOT escalate: Client.GetBuildingV1 escalating to
	// Client would pull in every test in the suite.
	changed = map[string]bool{}
	markChanged(changed, declMeta{name: "GetBuildingV1", recvType: "Client"})
	if changed["Client"] {
		t.Error("ordinary method escalated to its receiver type")
	}
	if !changed["GetBuildingV1"] {
		t.Error("ordinary method name not recorded")
	}
}

// TestWalkContinuesThroughImplicitReceiver guards the reachability hole found
// after PR #45 landed: a change to a helper that is only called by an
// implicitly-dispatched method (PayloadsXMLText.MarshalXML) must still reach the
// tests that flow that type. Escalating only when the METHOD itself changes is
// not enough — the walk has to escalate when it arrives at one.
func TestWalkContinuesThroughImplicitReceiver(t *testing.T) {
	idx := &moduleIndex{byRef: map[string][]*decl{}}
	add := func(d *decl) *decl {
		idx.decls = append(idx.decls, d)
		return d
	}
	helper := add(&decl{name: "minimizePlistSourceEscaping"})
	marshal := add(&decl{name: "MarshalXML", recvType: "PayloadsXMLText", implicitRecv: true})
	payloadType := add(&decl{name: "PayloadsXMLText"})
	profileType := add(&decl{name: "OsxConfigurationProfile"})
	test := add(&decl{name: "TestAcceptance_Classic_OSXProfile_QuoteRoundtrip", isTest: true})

	// MarshalXML calls the helper; the profile struct fields the payload type;
	// the test references the profile struct. Nothing references MarshalXML —
	// that is the point.
	idx.byRef["minimizePlistSourceEscaping"] = []*decl{marshal}
	idx.byRef["PayloadsXMLText"] = []*decl{profileType}
	idx.byRef["OsxConfigurationProfile"] = []*decl{test}
	_, _ = helper, payloadType

	got := idx.testsAffectedBy(map[string]bool{"minimizePlistSourceEscaping": true})
	if len(got) != 1 || got[0] != test.name {
		t.Errorf("testsAffectedBy = %v, want [%s] — the walk stopped at the implicitly-dispatched method", got, test.name)
	}
}

func keysOf(m map[string]declMeta) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
