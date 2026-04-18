// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devicegroups"
)

// runSuffix computes a unique suffix (epoch timestamp) once for the entire test run.
var runSuffix = sync.OnceValue(func() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
})

// initAcceptanceClient creates and validates the singleton acceptance client once.
var initAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	clientID := os.Getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_CLIENT_SECRET")
	tenantID := os.Getenv("JAMFPLATFORM_TENANT_ID")

	if baseURL == "" || clientID == "" || clientSecret == "" || tenantID == "" {
		return nil, fmt.Errorf("missing required environment variables (JAMFPLATFORM_BASE_URL, JAMFPLATFORM_CLIENT_ID, JAMFPLATFORM_CLIENT_SECRET, JAMFPLATFORM_TENANT_ID)")
	}

	opts := []jamfplatform.Option{jamfplatform.WithTenantID(tenantID)}

	c := jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}

	return c, nil
})

// accClient returns a live Jamf Platform API client, skipping the test if credentials are not set.
func accClient(t *testing.T) *jamfplatform.Client {
	t.Helper()
	c, err := initAcceptanceClient()
	if err != nil {
		t.Skipf("Skipping acceptance test: %v", err)
	}
	return c
}

// skipOnServerError skips the test if err is an API 5xx response.
// Use instead of t.Fatalf for API calls that may hit transient server bugs.
func skipOnServerError(t *testing.T, err error) {
	t.Helper()
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 500 {
		t.Skipf("Skipping due to server error: %v", err)
	}
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
		dg := devicegroups.New(c)

		groups, err := dg.ListDeviceGroups(ctx, nil, fmt.Sprintf("name==%q", smartGroupFixtureName()))
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

		fixtureDesc := "SDK acceptance test fixture — safe to delete"
		resp, err := dg.CreateDeviceGroup(ctx, &devicegroups.DeviceGroupCreateRepresentationV1{
			Name:        smartGroupFixtureName(),
			Description: &fixtureDesc,
			DeviceType:  "COMPUTER",
			GroupType:   "SMART",
			Criteria: &[]devicegroups.DeviceGroupCriteriaRepresentationV1{
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
		_ = devicegroups.New(c).DeleteDeviceGroup(context.Background(), smartGroupID)
	}
}

// Benchmark cleanup helpers — handle async sync states and stuck DELETING.

func ensureBenchmarkDeletedByID(t *testing.T, c *jamfplatform.Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	cb := compliancebenchmarks.New(c)
	waitForBenchmarkSyncState(t, c, ctx, benchmarkID)

	if err := cb.DeleteBenchmark(ctx, benchmarkID); err != nil {
		t.Logf("Warning: failed to delete benchmark %s: %v", benchmarkID, err)
		return
	}
	t.Logf("Delete issued for benchmark %s", benchmarkID)

	deleteCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	lastDelete := time.Now()
	err := jamfplatform.PollUntil(deleteCtx, 2*time.Second, func(_ context.Context) (bool, error) {
		if _, found := benchmarkSyncState(c, ctx, benchmarkID); !found {
			t.Logf("Benchmark %s fully deleted", benchmarkID)
			return true, nil
		}
		if time.Since(lastDelete) > 20*time.Second {
			lastDelete = time.Now()
			t.Logf("Retrying delete for stuck benchmark %s", benchmarkID)
			_ = cb.DeleteBenchmark(ctx, benchmarkID)
		}
		return false, nil
	})
	if err != nil {
		t.Logf("Warning: benchmark %s still present after 2m", benchmarkID)
	}
}

func waitForBenchmarkSyncState(t *testing.T, c *jamfplatform.Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	err := jamfplatform.PollUntil(syncCtx, 3*time.Second, func(_ context.Context) (bool, error) {
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

func benchmarkSyncState(c *jamfplatform.Client, ctx context.Context, benchmarkID string) (string, bool) {
	cb := compliancebenchmarks.New(c)
	benchmarks, err := cb.ListBenchmarks(ctx)
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
