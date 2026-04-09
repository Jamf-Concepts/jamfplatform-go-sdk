// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"context"
	"testing"
)

func TestAcceptance_ListBaselines(t *testing.T) {
	c := accClient(t)

	baselines, err := c.ListBaselines(context.Background())
	if err != nil {
		t.Fatalf("ListBaselines failed: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Log("No baselines found — CB Engine may not be enabled")
		return
	}
	t.Logf("Found %d baselines:", len(baselines.Baselines))
	for _, b := range baselines.Baselines {
		t.Logf("  %s (%s) — %d rules", b.Title, b.BaselineID, b.RuleCount)
	}
}

func TestAcceptance_GetBaselineRules(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	baselines, err := c.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("ListBaselines failed: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — cannot fetch rules")
	}

	baseline := baselines.Baselines[0]
	rules, err := c.GetBaselineRules(ctx, baseline.BaselineID)
	if err != nil {
		t.Fatalf("GetBaselineRules failed for %q: %v", baseline.BaselineID, err)
	}

	t.Logf("Found %d rules for baseline %q", len(rules.Rules), baseline.Title)
	t.Logf("  Sources: %d", len(rules.Sources))

	rulesWithODV := 0
	for _, r := range rules.Rules {
		if r.ODV != nil {
			rulesWithODV++
		}
	}
	t.Logf("  Rules with ODV: %d / %d", rulesWithODV, len(rules.Rules))
}

func TestAcceptance_ListBenchmarks(t *testing.T) {
	c := accClient(t)

	benchmarks, err := c.ListBenchmarks(context.Background())
	if err != nil {
		t.Fatalf("ListBenchmarks failed: %v", err)
	}
	t.Logf("Found %d benchmarks", len(benchmarks.Benchmarks))
	for _, b := range benchmarks.Benchmarks {
		t.Logf("  %s (%s) — sync: %s", b.Title, b.ID, b.SyncState)
	}
}

func TestAcceptance_Benchmark_CreateAndDelete(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	baselines, err := c.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("ListBaselines failed: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — CB Engine may not be enabled")
	}

	groupID := requireSmartGroupFixture(t)
	baseline := baselines.Baselines[0]

	rules, err := c.GetBaselineRules(ctx, baseline.BaselineID)
	if err != nil {
		t.Fatalf("GetBaselineRules failed: %v", err)
	}
	if len(rules.Rules) == 0 {
		t.Skip("No rules available for baseline")
	}

	// Build rule requests — enable first 5 rules (or fewer), including ODV values
	var ruleRequests []CBEngineRuleRequestV2
	limit := 5
	if len(rules.Rules) < limit {
		limit = len(rules.Rules)
	}
	for _, r := range rules.Rules[:limit] {
		rr := CBEngineRuleRequestV2{
			ID:      r.ID,
			Enabled: true,
		}
		if r.ODV != nil {
			rr.ODV = &CBEngineODVRequestV2{Value: r.ODV.Value}
		}
		ruleRequests = append(ruleRequests, rr)
	}

	title := "sdk-acc-benchmark-" + runSuffix()

	// Clean up any leftover from a previous run
	if existing, err := c.GetBenchmarkByTitle(ctx, title); err == nil {
		ensureBenchmarkDeletedByID(t, c, ctx, existing.BenchmarkID)
	}

	resp, err := c.CreateBenchmark(ctx, &CBEngineBenchmarkRequestV2{
		Title:            title,
		Description:      "SDK acceptance test — safe to delete",
		SourceBaselineID: baseline.BaselineID,
		Sources:          rules.Sources,
		Rules:            ruleRequests,
		Target:           CBEngineTargetV2{DeviceGroups: []string{groupID}},
		EnforcementMode:  "MONITOR",
	})
	if err != nil {
		t.Fatalf("CreateBenchmark failed: %v", err)
	}
	t.Logf("Created benchmark %q (ID: %s)", title, resp.BenchmarkID)

	t.Cleanup(func() {
		ensureBenchmarkDeletedByID(t, c, ctx, resp.BenchmarkID)
	})

	// Wait for sync then verify
	waitForBenchmarkSyncState(t, c, ctx, resp.BenchmarkID)

	bm, err := c.GetBenchmark(ctx, resp.BenchmarkID)
	if err != nil {
		t.Fatalf("GetBenchmark failed: %v", err)
	}
	if bm.Title != title {
		t.Errorf("expected title %q, got %q", title, bm.Title)
	}
	if bm.EnforcementMode != "MONITOR" {
		t.Errorf("expected MONITOR, got %q", bm.EnforcementMode)
	}
	t.Logf("Benchmark synced: %s, rules: %d", resp.BenchmarkID, len(bm.Rules))
}

func TestAcceptance_Benchmark_Reporting(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	baselines, err := c.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("ListBaselines failed: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — CB Engine may not be enabled")
	}

	groupID := requireSmartGroupFixture(t)
	baseline := baselines.Baselines[0]

	rules, err := c.GetBaselineRules(ctx, baseline.BaselineID)
	if err != nil {
		t.Fatalf("GetBaselineRules failed: %v", err)
	}
	if len(rules.Rules) == 0 {
		t.Skip("No rules available for baseline")
	}

	var ruleRequests []CBEngineRuleRequestV2
	limit := 3
	if len(rules.Rules) < limit {
		limit = len(rules.Rules)
	}
	for _, r := range rules.Rules[:limit] {
		rr := CBEngineRuleRequestV2{
			ID:      r.ID,
			Enabled: true,
		}
		if r.ODV != nil {
			rr.ODV = &CBEngineODVRequestV2{Value: r.ODV.Value}
		}
		ruleRequests = append(ruleRequests, rr)
	}

	title := "sdk-acc-reporting-" + runSuffix()
	if existing, err := c.GetBenchmarkByTitle(ctx, title); err == nil {
		ensureBenchmarkDeletedByID(t, c, ctx, existing.BenchmarkID)
	}

	resp, err := c.CreateBenchmark(ctx, &CBEngineBenchmarkRequestV2{
		Title:            title,
		Description:      "SDK acceptance test — reporting endpoints",
		SourceBaselineID: baseline.BaselineID,
		Sources:          rules.Sources,
		Rules:            ruleRequests,
		Target:           CBEngineTargetV2{DeviceGroups: []string{groupID}},
		EnforcementMode:  "MONITOR",
	})
	if err != nil {
		t.Fatalf("CreateBenchmark failed: %v", err)
	}
	t.Cleanup(func() { ensureBenchmarkDeletedByID(t, c, ctx, resp.BenchmarkID) })

	waitForBenchmarkSyncState(t, c, ctx, resp.BenchmarkID)
	benchmarkID := resp.BenchmarkID

	t.Run("RulesStats", func(t *testing.T) {
		stats, err := c.ListBenchmarkRulesStats(ctx, benchmarkID, "", "")
		if err != nil {
			t.Fatalf("ListBenchmarkRulesStats failed: %v", err)
		}
		t.Logf("Found %d rule stats", len(stats))
		for _, s := range stats {
			t.Logf("  %s: passed=%d failed=%d unknown=%d (%.1f%%)", s.RuleTitle, s.Passed, s.Failed, s.Unknown, s.PassPercentage)
		}
	})

	t.Run("RuleDevices", func(t *testing.T) {
		stats, err := c.ListBenchmarkRulesStats(ctx, benchmarkID, "", "")
		if err != nil {
			t.Fatalf("ListBenchmarkRulesStats failed: %v", err)
		}
		if len(stats) == 0 {
			t.Skip("No rule stats — cannot query devices")
		}
		devices, err := c.ListBenchmarkRuleDevices(ctx, benchmarkID, stats[0].RuleID, "", "", "")
		if err != nil {
			t.Fatalf("ListBenchmarkRuleDevices failed: %v", err)
		}
		t.Logf("Found %d devices for rule %s", len(devices), stats[0].RuleTitle)
	})

	t.Run("CompliancePercentage", func(t *testing.T) {
		pct, err := c.GetBenchmarkCompliancePercentage(ctx, benchmarkID)
		if err != nil {
			t.Fatalf("GetBenchmarkCompliancePercentage failed: %v", err)
		}
		t.Logf("Compliance percentage: %.1f%%", pct.CompliancePercentage)
	})
}

func TestAcceptance_Benchmark_GetByTitle(t *testing.T) {
	c := accClient(t)

	benchmarks, err := c.ListBenchmarks(context.Background())
	if err != nil {
		t.Fatalf("ListBenchmarks failed: %v", err)
	}
	if len(benchmarks.Benchmarks) == 0 {
		t.Skip("No benchmarks available")
	}

	title := benchmarks.Benchmarks[0].Title
	bm, err := c.GetBenchmarkByTitle(context.Background(), title)
	if err != nil {
		t.Fatalf("GetBenchmarkByTitle(%q) failed: %v", title, err)
	}
	if bm.Title != title {
		t.Errorf("expected title %q, got %q", title, bm.Title)
	}
	t.Logf("Found benchmark by title: %s (ID: %s)", bm.Title, bm.BenchmarkID)
}
