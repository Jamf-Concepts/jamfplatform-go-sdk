// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Command acclanes splits an acceptance test scope into per-product lanes and
// prints the GitHub matrix include[] for them.
//
// It is the second half of the acceptance plan step: tools/acctargets decides
// WHAT must run, acclanes decides HOW that is split across jobs. Keeping them
// separate keeps scope computation ignorant of lanes.
//
// # Why Go rather than a shell or Python script
//
// The lane patterns are ultimately handed to `go test -run`, which matches with
// RE2. Partitioning here with the same regexp package means the engine that
// decides a test's lane is the engine that will run it; any other language's
// regexp dialect could accept a pattern whose semantics differ, and mis-file a
// test into a lane whose credential cannot reach its endpoints. It also puts the
// logic under `go test`, so the partition is covered by CI rather than by
// whoever last ran the script by hand.
//
// # Why a test list rather than a regex complement
//
// The default lane is "everything no named lane claimed". As a pattern that
// needs negative lookahead, which RE2 rejects outright — regexp.Compile refuses
// `(?!` — so `go test -run` could never consume it. Partitioning a real list
// has no such limit and makes ALL and change-scoped runs one code path.
//
// Usage:
//
//	go test -tags acceptance -list '.*' ./jamfplatform/ |
//	  go run ./acclanes -table ../.github/acceptance-lanes.json -scope ALL
//
// The scope is whatever acctargets printed: ALL, NONE, or ^(TestA|TestB)$.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// laneDef is one row of .github/acceptance-lanes.json. That file is the single
// source of truth, shared with jamfplatform/acc_lanes_test.go, which asserts the
// name-based partition agrees with the credential each test actually uses.
type laneDef struct {
	Lane    string `json:"lane"`
	Match   string `json:"match"`
	Require string `json:"require"`
	Lock    bool   `json:"lock"`
	// Planned reserves a lane's name and pattern before its product exists. A
	// planned lane must match nothing; if it matches, partition fails rather
	// than emitting a job for a lane with no credential wired. Reserving the
	// name is what stops a new product's tests being named into the pro lane.
	Planned bool `json:"planned"`
}

type laneTable struct {
	Lanes       []laneDef `json:"lanes"`
	DefaultLane laneDef   `json:"default_lane"`
}

// matrixEntry is one GitHub matrix include[] element. Count is carried for the
// job summary: a lane's size is the first thing a reader wants when a run is
// slower or emptier than expected.
type matrixEntry struct {
	Lane    string `json:"lane"`
	Require string `json:"require"`
	Lock    bool   `json:"lock"`
	Run     string `json:"run"`
	Count   int    `json:"count"`
}

const testPrefix = "TestAcceptance_"

func loadTable(path string) (laneTable, error) {
	var table laneTable
	raw, err := os.ReadFile(path)
	if err != nil {
		return table, fmt.Errorf("reading lane table: %w", err)
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		return table, fmt.Errorf("parsing lane table: %w", err)
	}
	if len(table.Lanes) == 0 || table.DefaultLane.Lane == "" {
		return table, fmt.Errorf("lane table declares no lanes")
	}
	return table, nil
}

// partition assigns each test to the first lane whose pattern matches, else the
// default lane. First-match-wins makes the table readable as a priority list;
// the conformance test rejects a table where two lanes claim the same test, so
// order never silently decides anything.
func partition(table laneTable, names []string) ([]matrixEntry, error) {
	patterns := make([]*regexp.Regexp, len(table.Lanes))
	for i, lane := range table.Lanes {
		re, err := regexp.Compile(lane.Match)
		if err != nil {
			return nil, fmt.Errorf("lane %q: pattern %q does not compile (go test -run uses RE2, so no lookahead): %w", lane.Lane, lane.Match, err)
		}
		patterns[i] = re
	}

	buckets := map[string][]string{}
	for _, name := range names {
		placed := false
		for i, re := range patterns {
			if re.MatchString(name) {
				buckets[table.Lanes[i].Lane] = append(buckets[table.Lanes[i].Lane], name)
				placed = true
				break
			}
		}
		if !placed {
			buckets[table.DefaultLane.Lane] = append(buckets[table.DefaultLane.Lane], name)
		}
	}

	// A planned lane that matched tests means the product arrived and the wiring
	// did not. Fail the plan step: running those tests in any lane would use a
	// credential nobody chose for them.
	for _, lane := range table.Lanes {
		if lane.Planned && len(buckets[lane.Lane]) > 0 {
			return nil, fmt.Errorf("lane %q is still marked planned but now matches %d test(s) including %q: add a credential factory and a factoryLane row, give it a require token, wire its secrets, then drop \"planned\" from the lane table",
				lane.Lane, len(buckets[lane.Lane]), buckets[lane.Lane][0])
		}
	}

	// Emit in table order, default last, and drop empty lanes: a job that runs
	// zero tests reports a passing check having asserted nothing, which is the
	// failure mode JAMFPLATFORM_ACC_REQUIRE exists to close.
	var out []matrixEntry
	for _, lane := range append(append([]laneDef{}, table.Lanes...), table.DefaultLane) {
		names := buckets[lane.Lane]
		if len(names) == 0 {
			continue
		}
		out = append(out, matrixEntry{
			Lane:    lane.Lane,
			Require: lane.Require,
			Lock:    lane.Lock,
			Run:     "^(" + strings.Join(names, "|") + ")$",
			Count:   len(names),
		})
	}
	return out, nil
}

// parseScope turns acctargets' output into the list of test names to split.
// ALL consumes the authoritative list on stdin; a scope regex is unpacked from
// its own alternation rather than re-derived, so acclanes can never widen what
// acctargets chose.
func parseScope(scope string, stdin *bufio.Scanner) ([]string, error) {
	if scope == "ALL" {
		var listed []string
		for stdin.Scan() {
			if line := strings.TrimSpace(stdin.Text()); strings.HasPrefix(line, testPrefix) {
				listed = append(listed, line)
			}
		}
		if err := stdin.Err(); err != nil {
			return nil, fmt.Errorf("reading test list: %w", err)
		}
		if len(listed) == 0 {
			return nil, fmt.Errorf("scope is ALL but no %s* names arrived on stdin — pipe `go test -tags acceptance -list '.*' ./jamfplatform/` in", testPrefix)
		}
		return listed, nil
	}

	body := strings.TrimSuffix(strings.TrimPrefix(scope, "^("), ")$")
	var names []string
	for n := range strings.SplitSeq(body, "|") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("could not parse scope %q", scope)
	}
	return names, nil
}

func main() {
	table := flag.String("table", "../.github/acceptance-lanes.json", "path to the lane table")
	scope := flag.String("scope", "ALL", "scope as printed by acctargets: ALL, NONE, or ^(TestA|TestB)$")
	flag.Parse()

	if *scope == "NONE" {
		fmt.Println("[]")
		return
	}

	loaded, err := loadTable(*table)
	if err != nil {
		fmt.Fprintln(os.Stderr, "acclanes:", err)
		os.Exit(1)
	}
	names, err := parseScope(*scope, bufio.NewScanner(os.Stdin))
	if err != nil {
		fmt.Fprintln(os.Stderr, "acclanes:", err)
		os.Exit(1)
	}
	entries, err := partition(loaded, names)
	if err != nil {
		fmt.Fprintln(os.Stderr, "acclanes:", err)
		os.Exit(1)
	}
	out, err := json.Marshal(entries)
	if err != nil {
		fmt.Fprintln(os.Stderr, "acclanes:", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
