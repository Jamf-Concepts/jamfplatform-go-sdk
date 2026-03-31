// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// runSuffix computes a unique suffix (epoch timestamp) once for the entire test run.
var runSuffix = sync.OnceValue(func() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
})

// initAcceptanceClient creates and validates the singleton acceptance client once.
var initAcceptanceClient = sync.OnceValues(func() (*Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	clientID := os.Getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_CLIENT_SECRET")

	if baseURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("missing required environment variables (JAMFPLATFORM_BASE_URL, JAMFPLATFORM_CLIENT_ID, JAMFPLATFORM_CLIENT_SECRET)")
	}

	var opts []Option
	if id := os.Getenv("JAMFPLATFORM_TENANT_ID"); id != "" {
		opts = append(opts, WithTenantID(id))
	}
	if id := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID"); id != "" {
		opts = append(opts, WithEnvironmentID(id))
	}

	c := NewClient(baseURL, clientID, clientSecret, opts...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}

	return c, nil
})

// accClient returns a live Jamf Platform API client, skipping the test if credentials are not set.
func accClient(t *testing.T) *Client {
	t.Helper()
	c, err := initAcceptanceClient()
	if err != nil {
		t.Skipf("Skipping acceptance test: %v", err)
	}
	return c
}

// Smart group fixture — shared across all tests that need a device group scope.

var smartGroupFixtureOnce sync.Once
var smartGroupID string
var smartGroupErr error

func smartGroupFixtureName() string {
	return "sdk-acc-fixture-" + runSuffix()
}

func requireSmartGroupFixture(t *testing.T) string {
	t.Helper()

	smartGroupFixtureOnce.Do(func() {
		// If a device group ID is provided via env var, use it directly.
		// This is useful when the device groups API is not available with the
		// current credentials (e.g. tenant-scoped credentials for blueprints/benchmarks).
		if id := os.Getenv("JAMFPLATFORM_DEVICE_GROUP_ID"); id != "" {
			smartGroupID = id
			return
		}

		c := accClient(t)
		ctx := context.Background()

		groups, err := c.ListDeviceGroups(ctx, nil, fmt.Sprintf("name==%q", smartGroupFixtureName()))
		if err != nil {
			smartGroupErr = fmt.Errorf("failed to look up fixture smart group: %w", err)
			return
		}
		for _, g := range groups {
			if g.Name == smartGroupFixtureName() {
				smartGroupID = g.ID
				return
			}
		}

		desc := "SDK acceptance test fixture — safe to delete"
		resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
			Name:        smartGroupFixtureName(),
			Description: &desc,
			DeviceType:  "COMPUTER",
			GroupType:   "SMART",
			Criteria: []DeviceGroupCriteriaRepresentationV1{
				{
					Order:          0,
					AttributeName:  "Serial Number",
					Operator:       "LIKE",
					AttributeValue: "",
					JoinType:       "AND",
				},
			},
		})
		if err != nil {
			smartGroupErr = fmt.Errorf("failed to create fixture smart group: %w", err)
			return
		}
		smartGroupID = resp.ID
	})

	if smartGroupErr != nil {
		t.Fatalf("Smart group fixture failed: %v", smartGroupErr)
	}
	return smartGroupID
}

// cleanupSmartGroupFixture deletes the shared fixture. Call from TestMain.
// Skips cleanup when the group was provided via JAMFPLATFORM_DEVICE_GROUP_ID
// since we don't own that resource.
func cleanupSmartGroupFixture() {
	if smartGroupID == "" || os.Getenv("JAMFPLATFORM_DEVICE_GROUP_ID") != "" {
		return
	}
	if c, err := initAcceptanceClient(); err == nil {
		_ = c.DeleteDeviceGroup(context.Background(), smartGroupID)
	}
}

// Benchmark cleanup helpers — handle async sync states and stuck DELETING.

func ensureBenchmarkDeletedByID(t *testing.T, c *Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	waitForBenchmarkSyncState(t, c, ctx, benchmarkID)

	if err := c.DeleteBenchmark(ctx, benchmarkID); err != nil {
		t.Logf("Warning: failed to delete benchmark %s: %v", benchmarkID, err)
		return
	}
	t.Logf("Delete issued for benchmark %s", benchmarkID)

	deleteCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	lastDelete := time.Now()
	err := PollUntil(deleteCtx, 2*time.Second, func(_ context.Context) (bool, error) {
		if _, found := benchmarkSyncState(c, ctx, benchmarkID); !found {
			t.Logf("Benchmark %s fully deleted", benchmarkID)
			return true, nil
		}
		if time.Since(lastDelete) > 20*time.Second {
			lastDelete = time.Now()
			t.Logf("Retrying delete for stuck benchmark %s", benchmarkID)
			_ = c.DeleteBenchmark(ctx, benchmarkID)
		}
		return false, nil
	})
	if err != nil {
		t.Logf("Warning: benchmark %s still present after 2m", benchmarkID)
	}
}

func waitForBenchmarkSyncState(t *testing.T, c *Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	err := PollUntil(syncCtx, 3*time.Second, func(_ context.Context) (bool, error) {
		state, found := benchmarkSyncState(c, ctx, benchmarkID)
		if !found {
			t.Logf("Benchmark %s not found, may already be deleted", benchmarkID)
			return true, nil
		}
		if state == "SYNCED" || state == "FAILED" {
			t.Logf("Benchmark %s reached state %s", benchmarkID, state)
			return true, nil
		}
		t.Logf("Benchmark %s in state %q, waiting for SYNCED", benchmarkID, state)
		return false, nil
	})
	if err != nil {
		t.Logf("Warning: benchmark %s did not reach SYNCED after 2m", benchmarkID)
	}
}

func benchmarkSyncState(c *Client, ctx context.Context, benchmarkID string) (string, bool) {
	benchmarks, err := c.ListBenchmarks(ctx)
	if err != nil {
		return "", false
	}
	for _, b := range benchmarks.Benchmarks {
		if b.ID == benchmarkID {
			return b.SyncState, true
		}
	}
	return "", false
}
