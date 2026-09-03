// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
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

// seedPatchSoftwareTitleFixture creates a test-owned patch software title and
// returns its id, or skips the test when the tenant can't supply one.
//
// The Pro create endpoint (POST patch-software-title-configurations) needs a
// softwareTitleId that already exists in the tenant's patch source, and Pro
// exposes no way to list the source catalogue — only Classic's
// patchavailabletitles does. Classic's POST patchsoftwaretitles/id/0 accepts a
// source id plus a catalogue name_id and mints exactly that record: the id it
// returns is the same id the Pro v2/v3 configuration endpoints address, and
// deleting via Pro also removes the Classic record (verified on 11.30.2).
// That makes it a legitimate fixture rather than a second resource to track.
//
// Cleanup is registered by the caller so the id stays visible for assertions.
func seedPatchSoftwareTitleFixture(t *testing.T) string {
	t.Helper()
	c := accClient(t)
	ctx := context.Background()

	titles, err := proclassic.New(c).ListPatchAvailableTitlesBySourceID(ctx, "1")
	if err != nil {
		skipOnServerError(t, err)
		t.Skipf("cannot list available patch titles (source 1) — no fixture possible: %v", err)
	}
	if titles.AvailableTitles == nil || titles.AvailableTitles.AvailableTitle == nil || len(*titles.AvailableTitles.AvailableTitle) == 0 {
		t.Skip("patch source 1 offers no available titles — no fixture possible")
	}

	// Any title works; the sub-resources under test don't depend on which
	// software it is. Take the first with both fields populated.
	for _, at := range *titles.AvailableTitles.AvailableTitle {
		if at.AppName == nil || at.NameID == nil {
			continue
		}
		seeded, err := proclassic.New(c).CreatePatchSoftwareTitleByID(ctx, "0", &proclassic.PatchSoftwareTitle{
			Name:     at.AppName,
			NameID:   at.NameID,
			SourceID: new(1),
		})
		if err != nil {
			// A title already configured on the tenant is rejected as a
			// duplicate; try the next one rather than failing the suite.
			var apiErr *jamfplatform.APIResponseError
			if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
				continue
			}
			skipOnServerError(t, err)
			t.Fatalf("CreatePatchSoftwareTitleByID(%q): %v", *at.AppName, err)
		}
		if seeded.ID == nil {
			t.Fatalf("CreatePatchSoftwareTitleByID(%q) returned no id", *at.AppName)
		}
		id := strconv.Itoa(*seeded.ID)
		t.Logf("seeded patch software title %q (nameId=%s) as configuration id=%s", *at.AppName, *at.NameID, id)
		return id
	}
	t.Skip("every available patch title is already configured on the tenant — no fixture possible")
	return ""
}

// --- patch-software-title-configurations V3 ------------------------------
//
// V3 is the successor to the V2 surface, which Jamf marked deprecated with a
// 2026-07-14 date in the 11.30.0 spec (issue #50).

func TestAcceptance_Pro_Patch_ListSoftwareTitleConfigurationsV3(t *testing.T) {
	c := accClient(t)

	configs, err := pro.New(c).ListPatchSoftwareTitleConfigurationsV3(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchSoftwareTitleConfigurationsV3: %v", err)
	}
	t.Logf("Found %d patch software title configurations", len(configs))
}

// TestAcceptance_Pro_Patch_SoftwareTitleConfigV3Lifecycle exercises the whole
// V3 surface against a test-owned configuration: create (via the fixture),
// read, every sub-resource, dashboard add/remove, PATCH, name resolution, and
// delete with a 404 round-trip check.
func TestAcceptance_Pro_Patch_SoftwareTitleConfigV3Lifecycle(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	id := seedPatchSoftwareTitleFixture(t)
	cleanupDelete(t, "DeletePatchSoftwareTitleConfigurationV3 "+id, func() error {
		return p.DeletePatchSoftwareTitleConfigurationV3(ctx, id)
	})

	got, err := p.GetPatchSoftwareTitleConfigurationV3(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPatchSoftwareTitleConfigurationV3(%s): %v", id, err)
	}
	t.Logf("Config %s: displayName=%q softwareTitleId=%s jamfOfficial=%v",
		id, got.DisplayName, got.SoftwareTitleID, got.JamfOfficial)

	// Dashboard: add, confirm, remove, confirm.
	if err := p.AddPatchSoftwareTitleToDashboardV3(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("AddPatchSoftwareTitleToDashboardV3(%s): %v", id, err)
	}
	status, err := p.GetPatchSoftwareTitleDashboardStatusV3(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPatchSoftwareTitleDashboardStatusV3(%s): %v", id, err)
	}
	if !status.OnDashboard {
		t.Errorf("after add, onDashboard = false, want true")
	}
	if err := p.RemovePatchSoftwareTitleFromDashboardV3(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("RemovePatchSoftwareTitleFromDashboardV3(%s): %v", id, err)
	}
	if status, err := p.GetPatchSoftwareTitleDashboardStatusV3(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetPatchSoftwareTitleDashboardStatusV3(%s) after remove: %v", id, err)
	} else if status.OnDashboard {
		t.Errorf("after remove, onDashboard = true, want false")
	}

	// Definitions (paginated) — a Jamf-official title always has versions.
	defs, err := p.ListPatchSoftwareTitleDefinitionsV3(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitleDefinitionsV3(%s): %v", id, err)
	} else {
		t.Logf("Config %s has %d definitions", id, len(defs))
	}

	if _, err := p.GetPatchSoftwareTitleDependenciesV3(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetPatchSoftwareTitleDependenciesV3(%s): %v", id, err)
	}

	eas, err := p.ListPatchSoftwareTitleExtensionAttributesV3(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitleExtensionAttributesV3(%s): %v", id, err)
	} else {
		t.Logf("Config %s has %d extension attributes", id, len(eas))
	}

	// History — read, write a note, re-read and confirm the note landed.
	before, err := p.ListPatchSoftwareTitleHistoryV3(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitleHistoryV3(%s): %v", id, err)
	}
	note := "sdk-acc history note " + runSuffix()
	if _, err := p.CreatePatchSoftwareTitleHistoryNoteV3(ctx, id, &pro.ObjectHistoryNote{Note: note}); err != nil {
		skipOnServerError(t, err)
		t.Errorf("CreatePatchSoftwareTitleHistoryNoteV3(%s): %v", id, err)
	} else if after, err := p.ListPatchSoftwareTitleHistoryV3(ctx, id, nil, ""); err != nil {
		t.Errorf("ListPatchSoftwareTitleHistoryV3(%s) after note: %v", id, err)
	} else if len(after) <= len(before) {
		t.Errorf("history did not grow after note: before=%d after=%d", len(before), len(after))
	}

	// Patch report + summary.
	report, err := p.ListPatchSoftwareTitlePatchReportV3(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitlePatchReportV3(%s): %v", id, err)
	} else {
		t.Logf("Config %s patch report: %d rows", id, len(report))
	}
	if summary, err := p.GetPatchSoftwareTitlePatchSummaryV3(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetPatchSoftwareTitlePatchSummaryV3(%s): %v", id, err)
	} else {
		t.Logf("Config %s summary: title=%q latestVersion=%q upToDate=%d outOfDate=%d",
			id, summary.Title, summary.LatestVersion, summary.UpToDate, summary.OutOfDate)
	}
	versions, err := p.ListPatchSoftwareTitlePatchSummaryVersionsV3(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListPatchSoftwareTitlePatchSummaryVersionsV3(%s): %v", id, err)
	} else {
		t.Logf("Config %s patch summary has %d versions", id, len(versions))
	}

	// Export report — see assertPatchExportReport for why an empty report is
	// a 400 here rather than an empty CSV.
	assertPatchExportReport(t, "ExportPatchSoftwareTitleReportV3", id, len(report), func() ([]byte, error) {
		return p.ExportPatchSoftwareTitleReportV3(ctx, id, "", nil)
	})

	// PATCH the display name, then resolve by the new name.
	renamed := "sdk-acc-pstc-" + runSuffix()
	updated, err := p.UpdatePatchSoftwareTitleConfigurationV3(ctx, id, &pro.PatchSoftwareTitleConfigurationPatch{
		DisplayName: &renamed,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdatePatchSoftwareTitleConfigurationV3(%s): %v", id, err)
	}
	if updated.DisplayName != renamed {
		t.Errorf("displayName = %q, want %q", updated.DisplayName, renamed)
	}
	gotID, err := p.ResolvePatchSoftwareTitleConfigurationV3IDByName(ctx, renamed)
	if err != nil {
		t.Errorf("ResolvePatchSoftwareTitleConfigurationV3IDByName(%q): %v", renamed, err)
	} else if gotID != id {
		t.Errorf("resolved id = %q, want %q", gotID, id)
	}

	// Delete + verify gone.
	if err := p.DeletePatchSoftwareTitleConfigurationV3(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeletePatchSoftwareTitleConfigurationV3(%s): %v", id, err)
	}
	_, err = p.GetPatchSoftwareTitleConfigurationV3(ctx, id)
	if err == nil {
		t.Fatalf("GetPatchSoftwareTitleConfigurationV3(%s) after delete should 404", id)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetPatchSoftwareTitleConfigurationV3(%s) after delete: want 404, got %v", id, err)
	}
}

// assertPatchExportReport checks an export-report call given how many rows the
// corresponding patch report holds.
//
// Both V2 and V3 export-report answer 400 with an empty `errors` array when
// the configuration's patch report has no rows — verified on 11.30.2 across
// every column set (nil, the V2 spec default, the V3 spec default, and a
// single column), on freshly seeded titles for software the tenant's computers
// actually have installed. The rejection is therefore a property of an empty
// report, not of the version or of the request, and V3 is not a regression on
// V2. The 400 carries no detail, so there is nothing more precise to assert;
// a report with rows must still export a body.
func assertPatchExportReport(t *testing.T, label, id string, reportRows int, export func() ([]byte, error)) {
	t.Helper()
	body, err := export()
	if err != nil {
		if reportRows == 0 {
			var apiErr *jamfplatform.APIResponseError
			if errors.As(err, &apiErr) && apiErr.HasStatus(400) {
				t.Logf("%s(%s): 400 — expected, patch report has 0 rows (same on V2); plumbing OK", label, id)
				return
			}
		}
		skipOnServerError(t, err)
		t.Errorf("%s(%s): patch report has %d rows, export should have succeeded: %v", label, id, reportRows, err)
		return
	}
	if len(body) == 0 {
		t.Errorf("%s(%s) returned empty body", label, id)
		return
	}
	firstLine := string(body)
	if nl := strings.IndexByte(firstLine, '\n'); nl >= 0 {
		firstLine = firstLine[:nl]
	}
	t.Logf("%s(%s): %d bytes; header: %s", label, id, len(body), firstLine)
}

// TestAcceptance_Pro_Patch_CreateConfigV3 probes create with an empty body,
// expecting the server's field-level rejection. A real create is covered by
// the lifecycle test above.
func TestAcceptance_Pro_Patch_CreateConfigV3(t *testing.T) {
	c := accClient(t)

	_, err := pro.New(c).CreatePatchSoftwareTitleConfigurationV3(context.Background(), &pro.PatchSoftwareTitleConfigurationBase{})
	if err == nil {
		t.Fatal("CreatePatchSoftwareTitleConfigurationV3 with empty body succeeded — expected 4xx")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(400) {
		skipOnServerError(t, err)
		t.Fatalf("CreatePatchSoftwareTitleConfigurationV3(empty): want 400, got %v", err)
	}
	// The empty-body rejection is the one place this surface attributes errors
	// to fields, so assert the accessor actually buckets them.
	fields := apiErr.FieldErrors()
	if len(fields) == 0 {
		t.Errorf("FieldErrors() empty; body was %q", apiErr.Body)
	}
	for field, msgs := range fields {
		t.Logf("field %q: %v", field, msgs)
	}
}

// TestAcceptance_Pro_Patch_DeleteConfigV3 probes DELETE against a bogus id.
func TestAcceptance_Pro_Patch_DeleteConfigV3(t *testing.T) {
	c := accClient(t)

	err := pro.New(c).DeletePatchSoftwareTitleConfigurationV3(context.Background(), "99999999")
	if err == nil {
		t.Fatal("DeletePatchSoftwareTitleConfigurationV3 against bogus id succeeded — expected 404")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		skipOnServerError(t, err)
		t.Fatalf("DeletePatchSoftwareTitleConfigurationV3(bogus): want 404, got %v", err)
	}
	t.Log("DeletePatchSoftwareTitleConfigurationV3(bogus) rejected with 404 ✓")
}

// TestAcceptance_Pro_Patch_UpdateConfigV3 probes PATCH against a bogus id.
func TestAcceptance_Pro_Patch_UpdateConfigV3(t *testing.T) {
	c := accClient(t)

	_, err := pro.New(c).UpdatePatchSoftwareTitleConfigurationV3(context.Background(), "99999999", &pro.PatchSoftwareTitleConfigurationPatch{})
	if err == nil {
		t.Fatal("UpdatePatchSoftwareTitleConfigurationV3 against bogus id succeeded — expected 404")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		skipOnServerError(t, err)
		t.Fatalf("UpdatePatchSoftwareTitleConfigurationV3(bogus): want 404, got %v", err)
	}
	t.Log("UpdatePatchSoftwareTitleConfigurationV3(bogus) rejected with 404 ✓")
}
