// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// runSuffix computes a unique suffix (epoch timestamp) once for the entire test run.
var runSuffix = sync.OnceValue(func() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
})

// errAccCredsUnset marks the one condition under which skipping the whole suite
// is legitimate: no tenant was configured, as on a local run or a fork PR.
//
// Every other error from initAcceptanceClient means credentials WERE supplied and
// the tenant refused them, which must fail. Conflating the two is how a total
// auth outage reports success: on 2026-08-04 a WAF block made all 146 scoped
// tests skip, the package still printed `ok`, and the acceptance check went green
// having executed zero assertions against the tenant.
var errAccCredsUnset = errors.New("acceptance credentials not configured")

// initAcceptanceClient creates and validates the singleton acceptance client once.
var initAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	clientID := os.Getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_CLIENT_SECRET")
	tenantID := os.Getenv("JAMFPLATFORM_TENANT_ID")

	if baseURL == "" || clientID == "" || clientSecret == "" || tenantID == "" {
		return nil, fmt.Errorf("%w: set JAMFPLATFORM_BASE_URL, JAMFPLATFORM_CLIENT_ID, JAMFPLATFORM_CLIENT_SECRET, JAMFPLATFORM_TENANT_ID", errAccCredsUnset)
	}

	opts := []jamfplatform.Option{jamfplatform.WithTenantID(tenantID)}

	c := jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}

	return c, nil
})

// errAccJSCCredsUnset marks a Security Cloud acceptance run with no Security
// Cloud credentials configured. It is deliberately separate from
// errAccCredsUnset: Security Cloud is a different product with its own tenant
// ID, and a Jamf Pro API client cannot reach it at all — probed 2026-08-17, a
// Security Cloud client answers 403 BAD_PERMISSIONS on /api/pro and
// /api/devices, and the reverse holds. So the suite needs a second credential
// set, not just a second tenant ID, and a tenant configured for Jamf Pro only
// must skip these tests rather than fail them.
var errAccJSCCredsUnset = errors.New("Security Cloud acceptance credentials not configured")

// initJSCAcceptanceClient creates and validates the singleton Security Cloud
// acceptance client. JAMFPLATFORM_JSC_BASE_URL is optional and defaults to the
// Jamf Pro base URL, since both products are served by the same regional
// gateway; the credentials and tenant are not optional.
var initJSCAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_JSC_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("JAMFPLATFORM_BASE_URL")
	}
	clientID := os.Getenv("JAMFPLATFORM_JSC_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_JSC_CLIENT_SECRET")
	tenantID := os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")

	if baseURL == "" || clientID == "" || clientSecret == "" || tenantID == "" {
		return nil, fmt.Errorf("%w: set JAMFPLATFORM_JSC_CLIENT_ID, JAMFPLATFORM_JSC_CLIENT_SECRET, JAMFPLATFORM_JSC_TENANT_ID (and JAMFPLATFORM_JSC_BASE_URL when it differs from JAMFPLATFORM_BASE_URL)", errAccJSCCredsUnset)
	}

	// The tenant goes to WithSecurityCloudTenantID rather than WithTenantID so
	// this exercises the same option a dual-product consumer uses — a bug in
	// the per-namespace override would otherwise only ever surface downstream.
	c := jamfplatform.NewClient(baseURL, clientID, clientSecret,
		jamfplatform.WithSecurityCloudTenantID(tenantID))
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate Security Cloud credentials: %w", err)
	}

	return c, nil
})

// accSecurityCloudClient returns a live client for the securitycloud package.
// It skips when no Security Cloud credentials are configured and FAILS when
// they are configured but rejected — the same distinction accClient draws, for
// the same reason (see errAccCredsUnset).
func accSecurityCloudClient(t *testing.T) *securitycloud.Client {
	t.Helper()
	c, err := initJSCAcceptanceClient()
	switch {
	case errors.Is(err, errAccJSCCredsUnset):
		t.Skipf("Skipping Security Cloud acceptance test: %v", err)
	case err != nil:
		t.Fatal(credentialRejectedMessage(err))
	}
	return securitycloud.New(c)
}

// egressIP reports this host's public egress IP, resolved once per run because a
// rejected credential fails every test in the suite and a per-test lookup would
// add three seconds each. An empty result is itself a signal: a host that cannot
// reach checkip.amazonaws.com probably cannot reach Jamf either.
var egressIP = sync.OnceValue(func() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://checkip.amazonaws.com", nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
})

// credentialRejectedMessage builds the report to hand Jamf Support when the tenant
// refuses credentials that were supplied, mirroring what
// terraform-provider-jamfprotect emits for the same failure. The first question
// support asks about a blocked request is always "from which IP, and when", and on
// a CI runner nobody can go and check afterwards — the egress IP is gone with the
// runner. Capturing it in the failure output is the only chance to have it.
//
// The base URL is read from the environment rather than the client, which was
// never successfully constructed. In CI it renders as *** because it is a secret;
// that is fine, whoever opens the ticket has it.
func credentialRejectedMessage(err error) string {
	ip := egressIP()
	if ip == "" {
		ip = "(unable to determine — run `curl -s https://checkip.amazonaws.com` on this host)"
	}
	return fmt.Sprintf(`acceptance credentials were supplied but the tenant rejected them.
Failing rather than skipping: a skipped suite reports success while verifying nothing.

If this is an edge or WAF block rather than a bad secret, give Jamf Support:
  - Timestamp:    %s
  - Instance URL: %s
  - Egress IP:    %s

Technical details: %v`,
		time.Now().UTC().Format(time.RFC3339),
		os.Getenv("JAMFPLATFORM_BASE_URL"),
		ip,
		err)
}

// accClient returns a live Jamf Platform API client. It skips the test when no
// credentials are configured, and FAILS when credentials are configured but the
// tenant rejected them — see errAccCredsUnset for why that distinction matters.
func accClient(t *testing.T) *jamfplatform.Client {
	t.Helper()
	c, err := initAcceptanceClient()
	switch {
	case errors.Is(err, errAccCredsUnset):
		t.Skipf("Skipping acceptance test: %v", err)
	case err != nil:
		t.Fatal(credentialRejectedMessage(err))
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

// cleanupDelete registers a best-effort delete in t.Cleanup. Unlike
// `_ = pc.DeleteXxx(...)`, it surfaces delete failures via t.Logf so a
// server-side 5xx on cleanup doesn't silently leak a tenant record.
// Errors are intentionally non-fatal: one failed cleanup must not
// prevent other registered Cleanup hooks from running.
func cleanupDelete(t *testing.T, label string, fn func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := fn(); err != nil {
			t.Logf("cleanup %s: %v", label, err)
		}
	})
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
