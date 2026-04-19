// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Batch 22 — preview team-viewer + smart-group recalcs + misc
// destructive. Preview endpoints are probe-heavy; recalcs run on
// bogus ids so they can't touch real groups. Destructive endpoints
// (redeploy, reinstall-app-config, download-profile) get bogus-id
// probes only.

// --- preview listings ------------------------------------------------

func TestAcceptance_Pro_PreviewListsV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	computers, err := p.ListPreviewComputers(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPreviewComputers: %v", err)
	}
	t.Logf("Preview computers: %d", len(computers))

	configs, err := p.ListRemoteAdminConfigurations(ctx)
	if err != nil {
		// 404 means tenant has no remote-admin integrations configured
		// — expected on a clean fixture.
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			t.Logf("ListRemoteAdminConfigurations: 404 — no configurations on tenant")
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("ListRemoteAdminConfigurations: %v", err)
	}
	t.Logf("Remote admin configurations: %d", len(configs))
}

// --- team-viewer probes ----------------------------------------------

// Team-viewer integrations require a real TeamViewer account + script
// token. Probe endpoints with bogus ids to confirm routing works.
func TestAcceptance_Pro_TeamViewerProbes(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	const bogus = "00000000-0000-0000-0000-000000000000"

	tolerate := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Logf("%s: unexpectedly succeeded for bogus id", label)
			return
		}
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("%s: status=%d — expected for bogus id", label, apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("%s: %v", label, err)
	}

	_, err := p.GetTeamViewerConfiguration(ctx, bogus)
	tolerate("GetTeamViewerConfiguration", err)

	_, err = p.GetTeamViewerConfigurationStatus(ctx, bogus)
	tolerate("GetTeamViewerConfigurationStatus", err)

	_, err = p.ListTeamViewerSessions(ctx, bogus, "")
	tolerate("ListTeamViewerSessions", err)

	_, err = p.GetTeamViewerSession(ctx, bogus, bogus)
	tolerate("GetTeamViewerSession", err)

	_, err = p.GetTeamViewerSessionStatus(ctx, bogus, bogus)
	tolerate("GetTeamViewerSessionStatus", err)

	tolerate("CloseTeamViewerSession", p.CloseTeamViewerSession(ctx, bogus, bogus))
	tolerate("ResendTeamViewerSessionNotification", p.ResendTeamViewerSessionNotification(ctx, bogus, bogus))

	// Surprise: the server accepts a bogus script token and persists
	// the configuration (no validation against TeamViewer's API until
	// a session is actually initiated). Exercise full CRUD with
	// explicit cleanup so the probe doesn't leak.
	created, err := p.CreateTeamViewerConfiguration(ctx, &pro.ConnectionConfigurationCandidateRequest{
		DisplayName:    "sdk-acc-tv-" + runSuffix(),
		Enabled:        false,
		ScriptToken:    "sdk-acc-fake-token",
		SessionTimeout: 60,
		SiteID:         "-1",
	})
	if err != nil {
		tolerate("CreateTeamViewerConfiguration", err)
	} else {
		id := created.ID
		t.Logf("Created team-viewer configuration id=%s", id)
		cleanupDelete(t, "TeamViewerConfiguration "+id, func() error { return p.DeleteTeamViewerConfiguration(ctx, id) })

		if _, err := p.GetTeamViewerConfiguration(ctx, id); err != nil {
			skipOnServerError(t, err)
			t.Fatalf("GetTeamViewerConfiguration(%s): %v", id, err)
		}
		if _, err := p.UpdateTeamViewerConfiguration(ctx, id, &pro.ConnectionConfigurationUpdateRequest{}); err != nil {
			skipOnServerError(t, err)
			t.Fatalf("UpdateTeamViewerConfiguration(%s): %v", id, err)
		}

		// Session creation needs a real device id + TeamViewer backend,
		// so the session probe stays tolerate-only.
		_, err = p.CreateTeamViewerSession(ctx, id, &pro.SessionCandidateRequest{})
		tolerate("CreateTeamViewerSession (real config, bogus device)", err)
	}

	// Delete probe against a bogus id covers the delete path for the
	// case where the server happens to have nothing matching.
	tolerate("DeleteTeamViewerConfiguration (bogus id)", p.DeleteTeamViewerConfiguration(ctx, bogus))
}

// --- smart-group recalcs ---------------------------------------------

// Each recalc endpoint is a best-effort background sweep. Run against
// a bogus id to verify routing without disturbing live groups; expect
// 4xx rejection.
func TestAcceptance_Pro_RecalculateProbesV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	const bogus = "-1"

	tolerate := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Logf("%s: unexpectedly succeeded for bogus id", label)
			return
		}
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("%s: status=%d — expected for bogus id", label, apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("%s: %v", label, err)
	}

	_, err := p.RecalculateSmartComputerGroupV1(ctx, bogus)
	tolerate("RecalculateSmartComputerGroupV1", err)

	_, err = p.RecalculateSmartMobileDeviceGroupV1(ctx, bogus)
	tolerate("RecalculateSmartMobileDeviceGroupV1", err)

	_, err = p.RecalculateSmartUserGroupV1(ctx, bogus)
	tolerate("RecalculateSmartUserGroupV1", err)

	_, err = p.RecalculateComputerSmartGroupsV1(ctx, bogus)
	tolerate("RecalculateComputerSmartGroupsV1", err)

	_, err = p.RecalculateMobileDeviceSmartGroupsV1(ctx, bogus)
	tolerate("RecalculateMobileDeviceSmartGroupsV1", err)

	_, err = p.RecalculateUserSmartGroupsV1(ctx, bogus)
	tolerate("RecalculateUserSmartGroupsV1", err)
}

// --- destructive misc ------------------------------------------------

// Each endpoint alters device state when given a real id. Bogus-id
// probes only — a real fixture device would be needed to exercise
// the happy paths safely.
func TestAcceptance_Pro_DestructiveProbesV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	tolerate := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Logf("%s: unexpectedly succeeded", label)
			return
		}
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("%s: status=%d — expected rejection", label, apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("%s: %v", label, err)
	}

	_, err := p.DownloadMobileDeviceEnrollmentProfileV1(ctx, "-1")
	tolerate("DownloadMobileDeviceEnrollmentProfileV1", err)

	_, err = p.RedeployJamfManagementFrameworkV1(ctx, "-1")
	tolerate("RedeployJamfManagementFrameworkV1", err)

	// Reinstall-app-config with a bogus code returns 500 with message
	// "There was an error while trying to issue App Config re-install
	// MDM Command" — the server upgrades a validation error into an
	// internal error instead of 4xx. Treat 500 as expected here rather
	// than glossing over it via skipOnServerError.
	fakeCode := "sdk-acc-fake-reinstall-code"
	if err := p.ReinstallMobileDeviceAppConfigV1(ctx, &pro.AppConfigReinstallCode{
		ReinstallCode: &fakeCode,
	}); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == 400 || apiErr.StatusCode == 500) {
			t.Logf("ReinstallMobileDeviceAppConfigV1: status=%d — expected rejection of fake code (server mislabels as 500 instead of 4xx)", apiErr.StatusCode)
		} else {
			t.Fatalf("ReinstallMobileDeviceAppConfigV1: unexpected error: %v", err)
		}
	}
}
