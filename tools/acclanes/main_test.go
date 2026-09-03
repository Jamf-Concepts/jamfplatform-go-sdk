// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realTable is the table CI actually uses. Tests read it rather than a fixture
// so a change to the shipped lanes is exercised here too.
const realTable = "../../.github/acceptance-lanes.json"

func mustLoadReal(t *testing.T) laneTable {
	t.Helper()
	table, err := loadTable(realTable)
	if err != nil {
		t.Fatalf("loading %s: %v", realTable, err)
	}
	return table
}

func TestPartitionSplitsByProductAndDefaultsToPro(t *testing.T) {
	table := mustLoadReal(t)
	entries, err := partition(table, []string{
		"TestAcceptance_AccountReads",
		"TestAcceptance_SecurityCloudZtnaReads",
		"TestAcceptance_AuditReads",
		"TestAcceptance_AiGovernanceReads",
		"TestAcceptance_EnvironmentScope",
		"TestAcceptance_ProCoreReads",
		"TestAcceptance_ClassicLists",
		"TestAcceptance_ResolveDeviceIDByName",
	})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}

	got := map[string]int{}
	for _, e := range entries {
		got[e.Lane] = e.Count
	}
	want := map[string]int{"account": 1, "securitycloud": 1, "platform-env": 3, "pro": 3}
	for lane, n := range want {
		if got[lane] != n {
			t.Errorf("lane %q: got %d tests, want %d (all lanes: %v)", lane, got[lane], n, got)
		}
	}
}

// A lane with nothing to run must not appear. A job that executes zero tests
// reports a passing check having asserted nothing, which is exactly the
// skip-into-green failure the require mechanism exists to prevent.
func TestPartitionDropsEmptyLanes(t *testing.T) {
	table := mustLoadReal(t)
	entries, err := partition(table, []string{"TestAcceptance_SecurityCloudZtnaReads"})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d lanes, want 1: %+v", len(entries), entries)
	}
	if entries[0].Lane != "securitycloud" {
		t.Errorf("got lane %q, want securitycloud", entries[0].Lane)
	}
	if entries[0].Lock {
		t.Error("the securitycloud lane must not hold the shared Pro tenant lock — that is the point of splitting it out")
	}
}

// The Pro lane is the only one that mutates the shared tenant, so it is the only
// one that may serialise. If this inverts, either every lane queues behind Pro
// again or Pro stops being serialised.
func TestOnlyTheDefaultLaneTakesTheTenantLock(t *testing.T) {
	table := mustLoadReal(t)
	for _, lane := range table.Lanes {
		if lane.Lock {
			t.Errorf("lane %q claims the tenant lock; only the default lane may", lane.Lane)
		}
	}
	if !table.DefaultLane.Lock {
		t.Error("default lane must hold the tenant lock")
	}
}

// Every lane needs its own require token, or a missing credential skips the lane
// green instead of failing it.
func TestEveryLaneDeclaresARequireToken(t *testing.T) {
	table := mustLoadReal(t)
	for _, lane := range append(append([]laneDef{}, table.Lanes...), table.DefaultLane) {
		if lane.Require == "" {
			t.Errorf("lane %q has no require token", lane.Lane)
		}
	}
}

// go test -run matches with RE2, so a pattern it cannot compile must be rejected
// here rather than at job runtime. The first draft of this tool expressed the
// default lane as a negative lookahead, which RE2 refuses outright.
func TestPartitionRejectsAPatternRE2CannotCompile(t *testing.T) {
	table := laneTable{
		Lanes:       []laneDef{{Lane: "bad", Match: "^(?!TestAcceptance_Account)Test", Require: "x"}},
		DefaultLane: laneDef{Lane: "pro", Require: "platform", Lock: true},
	}
	_, err := partition(table, []string{"TestAcceptance_ProCoreReads"})
	if err == nil {
		t.Fatal("expected a compile error for a negative lookahead")
	}
	if !strings.Contains(err.Error(), "does not compile") {
		t.Errorf("error should name the cause, got: %v", err)
	}
}

func TestParseScopeUnpacksAnAlternation(t *testing.T) {
	names, err := parseScope("^(TestAcceptance_A|TestAcceptance_B)$", bufio.NewScanner(strings.NewReader("")))
	if err != nil {
		t.Fatalf("parseScope: %v", err)
	}
	if len(names) != 2 || names[0] != "TestAcceptance_A" || names[1] != "TestAcceptance_B" {
		t.Errorf("got %v, want [TestAcceptance_A TestAcceptance_B]", names)
	}
}

// ALL reads the authoritative list from stdin and ignores anything that is not a
// test name, because `go test -list` also prints the trailing `ok <pkg>` line.
func TestParseScopeALLTakesTheListFromStdinAndIgnoresNoise(t *testing.T) {
	in := "TestAcceptance_One\nTestSomethingElse\nTestAcceptance_Two\nok  \tgithub.com/x/y\t0.4s\n"
	names, err := parseScope("ALL", bufio.NewScanner(strings.NewReader(in)))
	if err != nil {
		t.Fatalf("parseScope: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("got %v, want the two TestAcceptance_ names only", names)
	}
}

// An empty list under ALL must fail loudly. Emitting an empty matrix would skip
// the entire acceptance suite and still report success.
func TestParseScopeALLWithNoListIsAnError(t *testing.T) {
	if _, err := parseScope("ALL", bufio.NewScanner(strings.NewReader(""))); err == nil {
		t.Fatal("expected an error when ALL gets no test list")
	}
}

// The shipped table must exist at the path the workflow uses, relative to the
// tools module — a broken default is a plan step that fails only in CI.
func TestShippedTableResolvesFromTheDefaultFlagPath(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", ".github", "acceptance-lanes.json")); err != nil {
		t.Fatalf("shipped lane table not where the -table default expects it: %v", err)
	}
	if _, err := loadTable("../" + "../.github/acceptance-lanes.json"); err != nil {
		t.Fatalf("shipped lane table does not load: %v", err)
	}
}

// A planned lane reserves a name before its product exists, so it must match
// nothing and must not produce a job.
func TestPlannedLanesAreReservedAndEmitNoJob(t *testing.T) {
	table := mustLoadReal(t)

	var planned []string
	for _, lane := range table.Lanes {
		if lane.Planned {
			planned = append(planned, lane.Lane)
		}
	}
	if len(planned) == 0 {
		t.Skip("no planned lanes reserved")
	}

	entries, err := partition(table, []string{"TestAcceptance_ProCoreReads", "TestAcceptance_AccountReads"})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	for _, e := range entries {
		for _, name := range planned {
			if e.Lane == name {
				t.Errorf("planned lane %q emitted a matrix entry", name)
			}
		}
	}
}

// The moment a planned lane's product arrives, the plan step must refuse rather
// than silently route the new tests somewhere. This is the readiness guarantee:
// the first Protect test cannot run until its credential is wired.
func TestPlannedLaneThatMatchesTestsFailsThePlanStep(t *testing.T) {
	table := mustLoadReal(t)

	var probe string
	for _, lane := range table.Lanes {
		if lane.Planned && lane.Lane == "protect" {
			probe = "TestAcceptance_ProtectPlansRead"
		}
	}
	if probe == "" {
		t.Skip("no planned protect lane to probe")
	}

	_, err := partition(table, []string{"TestAcceptance_ProCoreReads", probe})
	if err == nil {
		t.Fatal("expected the plan step to fail when a planned lane matches a test")
	}
	for _, want := range []string{"planned", "factoryLane", "require"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q so the reader knows what to wire; got: %v", want, err)
		}
	}
}
