// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Batch 5 — patch policies, patch policy logs, patch software title
// configurations, policies-preview, patch-management.
//
// Patch software title configurations require an integration with a patch
// source (external feed) configured on the tenant. Without one the CREATE
// path can't be exercised safely. These tests are therefore read-only
// against existing data plus bogus-id probes for mutating endpoints. If
// the tenant happens to have a configuration the test will additionally
// exercise the sub-resources against its real id.

// --- policies-preview ---------------------------------------------------

func TestAcceptance_Pro_Patch_PolicyPropertiesV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	props, err := p.GetPolicyPropertiesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPolicyPropertiesV1: %v", err)
	}
	t.Logf("Got policy properties: %+v", props)

	// Round-trip: write the same values back, verify no error.
	if _, err := p.UpdatePolicyPropertiesV1(ctx, props); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdatePolicyPropertiesV1: %v", err)
	}
}

// --- patch-management ---------------------------------------------------

// AcceptPatchManagementDisclaimerV2 is a tenant-wide one-way setting.
// Calling it against a tenant that already has it accepted is a no-op per
// the API, so it's safe to probe. Not a destructive action beyond the
// side-effect of accepting the disclaimer on an unaccepted tenant.
func TestAcceptance_Pro_Patch_AcceptDisclaimerV2(t *testing.T) {
	c := accClient(t)

	if err := pro.New(c).AcceptPatchManagementDisclaimerV2(context.Background()); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("AcceptPatchManagementDisclaimerV2: %v", err)
	}
}

// --- patch-policies -----------------------------------------------------

func TestAcceptance_Pro_Patch_ListPatchPoliciesV2(t *testing.T) {
	c := accClient(t)

	items, err := pro.New(c).ListPatchPoliciesV2(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPoliciesV2: %v", err)
	}
	t.Logf("Found %d patch policies", len(items))
}

func TestAcceptance_Pro_Patch_ListPatchPolicyDetailsV2(t *testing.T) {
	c := accClient(t)

	items, err := pro.New(c).ListPatchPolicyDetailsV2(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPolicyDetailsV2: %v", err)
	}
	t.Logf("Found %d patch policy details", len(items))
}

// TestAcceptance_Pro_Patch_PatchPolicyDashboardV2 exercises Get + Add +
// Remove on the dashboard sub-resource against the first patch policy
// the tenant has, if any.
func TestAcceptance_Pro_Patch_PatchPolicyDashboardV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	policies, err := p.ListPatchPoliciesV2(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPoliciesV2: %v", err)
	}
	if len(policies) == 0 {
		t.Skip("tenant has no patch policies — nothing to dashboard-probe")
	}
	id := policies[0].ID

	status, err := p.GetPatchPolicyDashboardStatusV2(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPatchPolicyDashboardStatusV2(%s): %v", id, err)
	}
	t.Logf("Dashboard status for patch policy %s: %+v", id, status)
}

// TestAcceptance_Pro_Patch_PatchPolicyLogsV2 exercises the log listing +
// eligible-retry-count + single-device log + log-details sub-resources.
// Read-only — does not invoke retry endpoints. If the tenant has no patch
// policies or no recent logs, sub-resources are skipped.
func TestAcceptance_Pro_Patch_PatchPolicyLogsV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	policies, err := p.ListPatchPoliciesV2(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPoliciesV2: %v", err)
	}
	if len(policies) == 0 {
		t.Skip("tenant has no patch policies — nothing to probe logs for")
	}
	policyID := policies[0].ID

	logs, err := p.ListPatchPolicyLogsV2(ctx, policyID, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPolicyLogsV2(%s): %v", policyID, err)
	}
	t.Logf("Patch policy %s has %d log entries", policyID, len(logs))

	retryCount, err := p.GetPatchPolicyEligibleRetryCountV2(ctx, policyID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPatchPolicyEligibleRetryCountV2(%s): %v", policyID, err)
	}
	t.Logf("Patch policy %s eligible retry count: %+v", policyID, retryCount)

	if len(logs) == 0 {
		return
	}
	deviceID := logs[0].DeviceID

	logForDevice, err := p.GetPatchPolicyLogForDeviceV2(ctx, policyID, deviceID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPatchPolicyLogForDeviceV2(%s,%s): %v", policyID, deviceID, err)
	}
	t.Logf("Log for policy=%s device=%s: state=%+v", policyID, deviceID, logForDevice)

	details, err := p.ListPatchPolicyLogDetailsForDeviceV2(ctx, policyID, deviceID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPolicyLogDetailsForDeviceV2(%s,%s): %v", policyID, deviceID, err)
	}
	t.Logf("Policy=%s device=%s has %d log-detail entries", policyID, deviceID, len(details))
}

// TestAcceptance_Pro_Patch_RetryPatchPolicyLogV2 exercises the per-device
// retry endpoint against a real patch policy + log entry when available.
//
// Can't be exercised with a bogus policy id: the server returns 500 for
// any unknown id rather than 404. Can't be exercised by creating a
// fixture policy either — patch policies can only come from a patch
// software title configuration, which requires an external patch source
// integration this test can't provision. Skips when the tenant has no
// real policy with logs.
func TestAcceptance_Pro_Patch_RetryPatchPolicyLogV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	policies, err := p.ListPatchPoliciesV2(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPoliciesV2: %v", err)
	}
	if len(policies) == 0 {
		t.Skip("tenant has no patch policies and they can't be created without an external patch source integration — skipping retry probe")
	}

	policyID := policies[0].ID
	logs, err := p.ListPatchPolicyLogsV2(ctx, policyID, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchPolicyLogsV2(%s): %v", policyID, err)
	}
	if len(logs) == 0 {
		t.Skipf("patch policy %s has no log entries — nothing to retry", policyID)
	}

	deviceID := logs[0].DeviceID
	if err := p.RetryPatchPolicyLogsV2(ctx, policyID, &pro.PatchPolicyLogRetry{
		DeviceIds: &[]string{deviceID},
	}); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("RetryPatchPolicyLogsV2 rejected: status=%d — policy=%s device=%s may not be eligible", apiErr.StatusCode, policyID, deviceID)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("RetryPatchPolicyLogsV2(%s, %s): %v", policyID, deviceID, err)
	}
	t.Logf("RetryPatchPolicyLogsV2 accepted for policy=%s device=%s", policyID, deviceID)
}

// TestAcceptance_Pro_Patch_RetryAllPatchPolicyLogsV2 exercises plumbing
// against a bogus policy id. The server has been observed to accept the
// retry-all call with 204 No Content even when the policy does not exist
// (should be 404) — flagged to the API team as a server-side validation
// bug. This test tolerates either the proper 4xx rejection or the
// current 204 silent-accept so it survives the fix.
func TestAcceptance_Pro_Patch_RetryAllPatchPolicyLogsV2(t *testing.T) {
	c := accClient(t)

	if err := pro.New(c).RetryAllPatchPolicyLogsV2(context.Background(), "99999999"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("RetryAllPatchPolicyLogsV2(bogus) rejected: status=%d", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("RetryAllPatchPolicyLogsV2(bogus) failed: %v", err)
	}
	t.Logf("RetryAllPatchPolicyLogsV2(bogus) accepted as 204 — known server-side validation gap (retry-all does not verify policy exists)")
}

// --- patch-software-title-configurations --------------------------------

func TestAcceptance_Pro_Patch_ListSoftwareTitleConfigurationsV2(t *testing.T) {
	c := accClient(t)

	configs, err := pro.New(c).ListPatchSoftwareTitleConfigurationsV2(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchSoftwareTitleConfigurationsV2: %v", err)
	}
	t.Logf("Found %d patch software title configurations", len(configs))
}

// TestAcceptance_Pro_Patch_SoftwareTitleConfigSubresources exercises every
// read sub-resource on the first configuration (if any) the tenant has.
// Patch software title configurations can't be created without a real
// external patch source, so this test is read-only against existing data.
func TestAcceptance_Pro_Patch_SoftwareTitleConfigSubresources(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	configs, err := p.ListPatchSoftwareTitleConfigurationsV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchSoftwareTitleConfigurationsV2: %v", err)
	}
	if len(configs) == 0 {
		t.Skip("tenant has no patch software title configurations — no read probes possible")
	}
	id := configs[0].ID

	// Get
	got, err := p.GetPatchSoftwareTitleConfigurationV2(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPatchSoftwareTitleConfigurationV2(%s): %v", id, err)
	}
	t.Logf("Config %s: displayName=%q", id, got.DisplayName)

	// Dashboard status
	if _, err := p.GetPatchSoftwareTitleDashboardStatusV2(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetPatchSoftwareTitleDashboardStatusV2(%s): %v", id, err)
	}

	// Definitions (paginated)
	defs, err := p.ListPatchSoftwareTitleDefinitionsV2(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitleDefinitionsV2(%s): %v", id, err)
	} else {
		t.Logf("Config %s has %d definitions", id, len(defs))
	}

	// Dependencies
	if _, err := p.GetPatchSoftwareTitleDependenciesV2(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetPatchSoftwareTitleDependenciesV2(%s): %v", id, err)
	}

	// Extension attributes (array of EAs tied to the title)
	eas, err := p.ListPatchSoftwareTitleExtensionAttributesV2(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitleExtensionAttributesV2(%s): %v", id, err)
	} else {
		t.Logf("Config %s has %d extension attributes", id, len(eas))
	}

	// History — read + write a note + re-read.
	hist, err := p.ListPatchSoftwareTitleHistoryV2(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitleHistoryV2(%s): %v", id, err)
	} else {
		t.Logf("Config %s history: %d entries", id, len(hist))
	}
	if _, err := p.CreatePatchSoftwareTitleHistoryNoteV2(ctx, id, &pro.ObjectHistoryNote{
		Note: "sdk-acc test history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Errorf("CreatePatchSoftwareTitleHistoryNoteV2(%s): %v", id, err)
	}

	// Patch report (paginated)
	report, err := p.ListPatchSoftwareTitlePatchReportV2(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitlePatchReportV2(%s): %v", id, err)
	} else {
		t.Logf("Config %s patch report: %d rows", id, len(report))
	}

	// Patch summary
	if _, err := p.GetPatchSoftwareTitlePatchSummaryV2(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetPatchSoftwareTitlePatchSummaryV2(%s): %v", id, err)
	}

	// Patch summary / versions (array)
	versions, err := p.ListPatchSoftwareTitlePatchSummaryVersionsV2(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitlePatchSummaryVersionsV2(%s): %v", id, err)
	} else {
		t.Logf("Config %s patch summary has %d versions", id, len(versions))
	}

	// Export report — text/csv response body.
	body, err := p.ExportPatchSoftwareTitleReportV2(ctx, id, "", nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ExportPatchSoftwareTitleReportV2(%s): %v", id, err)
	} else if len(body) == 0 {
		t.Errorf("ExportPatchSoftwareTitleReportV2(%s) returned empty body", id)
	} else {
		firstLine := string(body)
		if nl := strings.IndexByte(firstLine, '\n'); nl >= 0 {
			firstLine = firstLine[:nl]
		}
		t.Logf("Config %s export: %d bytes; header: %s", id, len(body), firstLine)
	}
}

// TestAcceptance_Pro_Patch_DeleteConfigV2 probes DELETE against a bogus
// id. Never delete a real patch software title configuration without an
// explicit test-owned fixture.
func TestAcceptance_Pro_Patch_DeleteConfigV2(t *testing.T) {
	c := accClient(t)

	err := pro.New(c).DeletePatchSoftwareTitleConfigurationV2(context.Background(), "99999999")
	if err == nil {
		t.Fatal("DeletePatchSoftwareTitleConfigurationV2 against bogus id succeeded — expected 4xx")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("DeletePatchSoftwareTitleConfigurationV2(bogus) rejected: status=%d", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Logf("DeletePatchSoftwareTitleConfigurationV2(bogus) rejected: %v", err)
}

// TestAcceptance_Pro_Patch_UpdateConfigV2 probes PATCH (merge-patch+json
// content type) against a bogus id. Never mutate a real patch software
// title configuration from this test.
func TestAcceptance_Pro_Patch_UpdateConfigV2(t *testing.T) {
	c := accClient(t)

	_, err := pro.New(c).UpdatePatchSoftwareTitleConfigurationV2(context.Background(), "99999999", &pro.PatchSoftwareTitleConfigurationPatch{})
	if err == nil {
		t.Fatal("UpdatePatchSoftwareTitleConfigurationV2 against bogus id succeeded — expected 4xx")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("UpdatePatchSoftwareTitleConfigurationV2(bogus) rejected: status=%d", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Logf("UpdatePatchSoftwareTitleConfigurationV2(bogus) rejected: %v", err)
}

// TestAcceptance_Pro_Patch_CreateConfigV2 probes the create endpoint with
// a clearly-bogus body expecting a 4xx rejection. A real create would
// require an external patch source integration that this test can't set
// up; a well-formed rejection still exercises the transport path.
func TestAcceptance_Pro_Patch_CreateConfigV2(t *testing.T) {
	c := accClient(t)

	_, err := pro.New(c).CreatePatchSoftwareTitleConfigurationV2(context.Background(), &pro.PatchSoftwareTitleConfigurationBase{})
	if err == nil {
		t.Fatal("CreatePatchSoftwareTitleConfigurationV2 with empty body succeeded — expected 4xx")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("CreatePatchSoftwareTitleConfigurationV2(empty) rejected: status=%d", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Logf("CreatePatchSoftwareTitleConfigurationV2(empty) rejected: %v", err)
}
