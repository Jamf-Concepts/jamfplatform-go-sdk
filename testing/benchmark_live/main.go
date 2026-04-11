// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
)

func main() {
	baseURL := flag.String("url", "", "Jamf Platform base URL")
	clientID := flag.String("client-id", "", "OAuth2 client ID")
	clientSecret := flag.String("client-secret", "", "OAuth2 client secret")
	tenantID := flag.String("tenant-id", "", "Tenant UUID")
	flag.Parse()

	if *baseURL == "" || *clientID == "" || *clientSecret == "" || *tenantID == "" {
		log.Fatal("usage: -url <base-url> -client-id <id> -client-secret <secret> -tenant-id <uuid>")
	}

	client := jamfplatform.NewClient(*baseURL, *clientID, *clientSecret,
		jamfplatform.WithTenantID(*tenantID))
	ctx := context.Background()

	passed, failed := 0, 0
	pass := func(name string) { fmt.Printf("  PASS %s\n", name); passed++ }
	fail := func(name string, err error) { fmt.Printf("  FAIL %s: %v\n", name, err); failed++ }

	// 1. ValidateCredentials
	fmt.Println("=== Auth ===")
	if err := client.ValidateCredentials(ctx); err != nil {
		log.Fatalf("auth failed, aborting: %v", err)
	}
	pass("ValidateCredentials")
	fmt.Println()

	// 2. ListBenchmarks
	fmt.Println("=== ListBenchmarks ===")
	benchmarks, err := client.ListBenchmarks(ctx)
	if err != nil {
		fail("ListBenchmarks", err)
	} else {
		pass(fmt.Sprintf("ListBenchmarks (%d benchmarks)", len(benchmarks.Benchmarks)))
		for _, bm := range benchmarks.Benchmarks {
			fmt.Printf("       %s - %q (sync=%s)\n", bm.ID, bm.Title, bm.SyncState)
		}
	}
	fmt.Println()

	// 3. GetBenchmark (if we have one)
	if benchmarks != nil && len(benchmarks.Benchmarks) > 0 {
		bmID := benchmarks.Benchmarks[0].ID

		fmt.Println("=== GetBenchmark ===")
		bm, err := client.GetBenchmark(ctx, bmID)
		if err != nil {
			fail(fmt.Sprintf("GetBenchmark(%s)", bmID), err)
		} else {
			pass(fmt.Sprintf("GetBenchmark(%s) -> %q, enforcement=%s", bmID, bm.Title, bm.EnforcementMode))
			fmt.Printf("       rules: %d, target groups: %v\n", len(bm.Rules), bm.Target.DeviceGroups)
		}
		fmt.Println()

	}

	// 6. ListBaselines
	fmt.Println("=== ListBaselines ===")
	baselines, err := client.ListBaselines(ctx)
	if err != nil {
		fail("ListBaselines", err)
	} else {
		pass(fmt.Sprintf("ListBaselines (%d baselines)", len(baselines.Baselines)))
		for _, bl := range baselines.Baselines {
			fmt.Printf("       %s - %q (%d rules)\n", bl.ID, bl.Title, bl.RuleCount)
		}
	}
	fmt.Println()

	// 7. GetBaselineRules (pick first baseline)
	if baselines != nil && len(baselines.Baselines) > 0 {
		blID := baselines.Baselines[0].BaselineID
		fmt.Println("=== GetBaselineRules ===")
		rules, err := client.GetBaselineRules(ctx, blID)
		if err != nil {
			fail(fmt.Sprintf("GetBaselineRules(%s)", blID), err)
		} else {
			pass(fmt.Sprintf("GetBaselineRules(%s) -> %d rules, %d sources", blID, len(rules.Rules), len(rules.Sources)))
			if len(rules.Rules) > 0 {
				r := rules.Rules[0]
				fmt.Printf("       first rule: %s - %q (enabled=%v)\n", r.ID, r.Title, r.Enabled)
			}
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Printf("  Passed: %d, Failed: %d\n", passed, failed)
	if failed > 0 {
		log.Fatal("some tests failed")
	}
}
