// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// App Installers: catalog titles (read-only) + deployment CRUD. Titles
// come from Jamf's shared App Catalog — every tenant sees the same
// ~340 entries. Deployments target a smart computer group; each
// mutating test provisions a throwaway smart group with an always-
// false criterion (so zero devices match, no install fires), uses it
// as the deployment target, and tears it down on cleanup.

const appInstallerSweepPercent = 10 // CRUD this percentage of the catalog per run

// TestAcceptance_Pro_AppInstallerTitles pulls the full catalog and
// asserts pagination returns a plausible number of titles with the
// expected shape. 340 entries as of writing.
func TestAcceptance_Pro_AppInstallerTitles(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	titles, err := p.ListAppInstallerTitlesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerTitlesV1: %v", err)
	}
	if len(titles) < 10 {
		t.Errorf("expected at least 10 App Installer titles, got %d", len(titles))
	}
	t.Logf("App Installer titles: %d", len(titles))

	// Spot-check: first title should have id, titleName, publisher.
	first := titles[0]
	if first.ID == "" || first.TitleName == "" || first.Publisher == "" {
		t.Errorf("first title has missing required fields: %+v", first)
	}

	// Round-trip by-id lookup and verify the new fields are present.
	got, err := p.GetAppInstallerTitleV1(ctx, first.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerTitleV1(%s): %v", first.ID, err)
	}
	if got.ID != first.ID {
		t.Errorf("title round-trip id mismatch: got %s want %s", got.ID, first.ID)
	}
	// New fields added by the spec refresh — log so we can see real wire values.
	t.Logf("title %s (%s):", got.ID, got.TitleName)
	t.Logf("  packageSigningIdentity=%q", got.PackageSigningIdentity)
	t.Logf("  installerPackageHashType=%q installerPackageHash=%q", got.InstallerPackageHashType, got.InstallerPackageHash)
	t.Logf("  launchDaemonIncluded=%v notificationAvailable=%v suppressAutoUpdate=%v",
		got.LaunchDaemonIncluded, got.NotificationAvailable, got.SuppressAutoUpdate)
	t.Logf("  originalMediaSources=%d entries", len(got.OriginalMediaSources))
	for i, src := range got.OriginalMediaSources {
		t.Logf("    [%d] hashType=%q hash=%q url=%q", i, src.HashType, src.Hash, src.URL)
	}
}

// TestAcceptance_Pro_AppInstallerDeploymentCRUD exercises the full
// deployment lifecycle against a single title. Creates a throwaway
// empty smart group as the deployment target, picks the first
// catalog title, creates a disabled SELF_SERVICE deployment, round-
// trips GET, PUTs with SelfServiceCategory, then cleans up.
func TestAcceptance_Pro_AppInstallerDeploymentCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	groupID := createAppInstallerSmartGroup(t, p)

	titles, err := p.ListAppInstallerTitlesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerTitlesV1: %v", err)
	}
	if len(titles) == 0 {
		t.Skip("no App Installer titles available in catalog")
	}
	title := titles[0]

	dep := createDeployment(t, p, title.ID, groupID, "sdk-acc-appinst-"+runSuffix())
	id := dep.ID
	cleanupDelete(t, "AppInstallerDeployment "+id, func() error { return p.DeleteAppInstallerDeploymentV1(ctx, id) })
	t.Logf("Created app-installer deployment id=%s for title %s (%s)", id, title.ID, title.TitleName)

	got, err := p.GetAppInstallerDeploymentV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerDeploymentV1(%s): %v", id, err)
	}
	if got.AppTitleID != title.ID {
		t.Errorf("deployment round-trip appTitleId mismatch: got %q want %q", got.AppTitleID, title.ID)
	}
	if got.Enabled {
		t.Errorf("deployment enabled=%v, expected false (disabled create)", got.Enabled)
	}

	// Wire-test: PUT to verify update round-trip and selfServiceSettings shape.
	updateReq := &pro.AppInstallerDeploymentCreate{
		Name:           got.Name,
		AppTitleID:     got.AppTitleID,
		DeploymentType: got.DeploymentType,
		UpdateBehavior: got.UpdateBehavior,
		SmartGroupID:   &groupID,
		SelfServiceSettings: &pro.AppInstallerSelfServiceSettings{
			ForceViewDescription: ptrFalse(),
		},
	}
	updated, err := p.UpdateAppInstallerDeploymentV1(ctx, id, updateReq)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateAppInstallerDeploymentV1: %v", err)
	}
	// selfServiceSettings.categories is an []SelfServiceCategory in the response.
	// Log what the server echoes back so we can inspect the wire shape.
	if updated.SelfServiceSettings != nil && updated.SelfServiceSettings.Categories != nil {
		cats := *updated.SelfServiceSettings.Categories
		t.Logf("selfServiceSettings.categories after PUT: %d entries", len(cats))
		for i, cat := range cats {
			catIDVal, featuredVal := "<nil>", "<nil>"
			if cat.ID != nil {
				catIDVal = *cat.ID
			}
			if cat.Featured != nil {
				featuredVal = strconv.FormatBool(*cat.Featured)
			}
			t.Logf("  [%d] id=%s featured=%s", i, catIDVal, featuredVal)
		}
	} else {
		t.Logf("selfServiceSettings.categories: nil/absent (no categories configured on this tenant)")
	}

	// Wire-test: SelfServiceCategory type has both ID and Featured fields.
	// Compile-time proof: if the type were still {id,name} this wouldn't build.
	_ = pro.SelfServiceCategory{ID: ptrStr("test"), Featured: ptrFalse()}
}

// TestAcceptance_Pro_AppInstallerDeploymentListShape verifies the list
// endpoint returns AppInstallerDeploymentListEntry items with the
// expected wire fields (id, name, deploymentType, bundleId, etc.).
func TestAcceptance_Pro_AppInstallerDeploymentListShape(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	items, err := p.ListAppInstallerDeploymentsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerDeploymentsV1: %v", err)
	}
	if len(items) == 0 {
		t.Skip("no app installer deployments in tenant — skipping shape check")
	}
	t.Logf("App Installer deployments: %d", len(items))
	first := items[0]
	bundleID := ""
	if first.App != nil {
		bundleID = first.App.BundleID
	}
	t.Logf("first deployment: id=%s name=%q deploymentType=%s updateBehavior=%s bundleId=%q",
		first.ID, first.Name, first.DeploymentType, first.UpdateBehavior, bundleID)
	// Verify nested app object populated by wire
	if first.App == nil {
		t.Error("first list entry has nil App — nested app object missing from wire decode")
	} else {
		t.Logf("  app: titleId=%s bundleId=%q latestVersion=%q selectedVersion=%q deployedVersion=%q versionRemoved=%v titleAvailableInAis=%v mediaSourceType=%q",
			first.App.ID, first.App.BundleID, first.App.LatestVersion, first.App.SelectedVersion,
			first.App.DeployedVersion, first.App.VersionRemoved, first.App.TitleAvailableInAis, first.App.MediaSourceType)
	}
	if first.ID == "" {
		t.Error("first list entry has empty ID")
	}
	if first.Name == "" {
		t.Error("first list entry has empty Name")
	}
}

// TestAcceptance_Pro_AppInstallerDeploymentsRandomSweep samples a
// random 10% of the catalog and runs create → get → delete on each.
// A single CRUD lifecycle (covered above) only proves one code path
// works; the sweep guards against title-specific regressions in the
// server's validation. 10% keeps runtime under a minute and the
// tenant's audit log manageable.
func TestAcceptance_Pro_AppInstallerDeploymentsRandomSweep(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	groupID := createAppInstallerSmartGroup(t, p)

	titles, err := p.ListAppInstallerTitlesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerTitlesV1: %v", err)
	}
	if len(titles) == 0 {
		t.Skip("no App Installer titles available in catalog")
	}

	sampleSize := len(titles) * appInstallerSweepPercent / 100
	if sampleSize < 1 {
		sampleSize = 1
	}
	perm := rand.Perm(len(titles))[:sampleSize]
	sample := make([]pro.AppInstallerTitle, len(perm))
	for i, idx := range perm {
		sample[i] = titles[idx]
	}
	t.Logf("Sweeping %d of %d titles (%d%%)", len(sample), len(titles), appInstallerSweepPercent)

	suffix := runSuffix()
	var created, failed int
	for i, title := range sample {
		name := "sdk-acc-sweep-" + suffix + "-" + strconv.Itoa(i)
		dep, err := p.CreateAppInstallerDeploymentV1(ctx, &pro.AppInstallerDeploymentCreate{
			Name:           name,
			AppTitleID:     title.ID,
			DeploymentType: "SELF_SERVICE",
			UpdateBehavior: "AUTOMATIC",
			CategoryID:     ptrStr("-1"),
			SiteID:         ptrStr("-1"),
			SmartGroupID:   &groupID,
		})
		if err != nil {
			var apiErr *jamfplatform.APIResponseError
			if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
				// Some titles reject create (e.g. deprecated, unavailable
				// for the tenant's region). Count and continue.
				failed++
				continue
			}
			skipOnServerError(t, err)
			t.Fatalf("create[%d] %s: %v", i, title.ID, err)
		}
		id := dep.ID

		// Round-trip GET.
		got, err := p.GetAppInstallerDeploymentV1(ctx, id)
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("get[%d] %s: %v", i, id, err)
		}
		if got.AppTitleID != title.ID {
			t.Errorf("get[%d] appTitleId mismatch: got %q want %q", i, got.AppTitleID, title.ID)
		}

		// Delete immediately to avoid accumulating state.
		if err := p.DeleteAppInstallerDeploymentV1(ctx, id); err != nil {
			skipOnServerError(t, err)
			t.Fatalf("delete[%d] %s: %v", i, id, err)
		}
		created++
	}
	t.Logf("Swept %d titles: %d CRUDed, %d rejected on create", len(sample), created, failed)
}

// TestAcceptance_Pro_AppInstallerGlobalSettings exercises GET and PUT
// on the singleton global settings resource. All fields are nullable;
// the test snapshots the current state on entry, writes a known payload,
// verifies the round-trip, then restores the original in t.Cleanup so
// the tenant isn't left dirty.
func TestAcceptance_Pro_AppInstallerGlobalSettings(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	original, err := p.GetAppInstallerGlobalSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerGlobalSettingsV1: %v", err)
	}
	t.Logf("AppInstaller global settings retrieved")

	t.Cleanup(func() {
		if _, err := p.UpdateAppInstallerGlobalSettingsV1(ctx, original); err != nil {
			t.Logf("WARNING: failed to restore AppInstaller global settings: %v", err)
		}
	})

	batchFreq := 60
	batchSize := 50
	notifMsg := "Test update pending."
	notifInterval := 30
	deadline := 24
	quitDelay := 5
	completeMsg := "Update complete."
	relaunch := true
	suppress := false

	put := &pro.AppInstallerGlobalSettings{
		EndUserExperienceSettings: &pro.AppInstallerEndUserExperienceSettings{
			NotificationMessage:  &notifMsg,
			NotificationInterval: &notifInterval,
			Deadline:             &deadline,
			QuitDelay:            &quitDelay,
			CompleteMessage:      &completeMsg,
			Relaunch:             &relaunch,
			Suppress:             &suppress,
		},
		DeploymentProcessControls: &pro.AppInstallerDeploymentProcessControls{
			CommandsBatchSize:       &batchSize,
			BatchFrequencyInMinutes: &batchFreq,
			DaysOfWeek:              &[]string{"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY"},
			FromTimeOfDay:           ptrStr("08:00:00Z"),
			ToTimeOfDay:             ptrStr("18:00:00Z"),
		},
	}

	updated, err := p.UpdateAppInstallerGlobalSettingsV1(ctx, put)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateAppInstallerGlobalSettingsV1: %v", err)
	}

	if updated.DeploymentProcessControls == nil {
		t.Fatal("UpdateAppInstallerGlobalSettingsV1: response missing deploymentProcessControls")
	}
	if updated.DeploymentProcessControls.BatchFrequencyInMinutes == nil ||
		*updated.DeploymentProcessControls.BatchFrequencyInMinutes != batchFreq {
		t.Errorf("batchFrequencyInMinutes: got %v want %d",
			updated.DeploymentProcessControls.BatchFrequencyInMinutes, batchFreq)
	}
	if updated.EndUserExperienceSettings == nil {
		t.Fatal("UpdateAppInstallerGlobalSettingsV1: response missing endUserExperienceSettings")
	}
	if updated.EndUserExperienceSettings.NotificationMessage == nil ||
		*updated.EndUserExperienceSettings.NotificationMessage != notifMsg {
		t.Errorf("notificationMessage: got %v want %q",
			updated.EndUserExperienceSettings.NotificationMessage, notifMsg)
	}

	t.Logf("AppInstaller global settings updated and verified")
}

// Helpers -------------------------------------------------------------

// createAppInstallerSmartGroup provisions a throwaway Classic smart
// computer group with an always-false criterion (Computer Name is
// SDK_ACC_NEVER_MATCHES) so no device ever matches — guarantees the
// App Installer deployment won't push to anything real. Registers a
// cleanup that deletes the group after the test.
func createAppInstallerSmartGroup(t *testing.T, p *pro.Client) string {
	t.Helper()
	ctx := context.Background()
	name := "sdk-acc-appinst-group-" + runSuffix()
	resp, err := p.CreateSmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{
		Name: name,
		Criteria: &[]pro.SmartSearchCriterion{
			{
				AndOr:      "and",
				Name:       "Computer Name",
				SearchType: "is",
				Value:      "SDK_ACC_NEVER_MATCHES",
			},
		},
	}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSmartComputerGroupV2 for app-installer test: %v", err)
	}
	id := resp.ID
	t.Logf("Created throwaway smart group id=%s for app-installer test", id)
	cleanupDelete(t, "SmartComputerGroup "+id, func() error { return p.DeleteSmartComputerGroupV2(ctx, id) })
	return id
}

// createDeployment creates a disabled SELF_SERVICE deployment for a given
// title and smart group, returning the hydrated GET response.
func createDeployment(t *testing.T, p *pro.Client, titleID, smartGroupID, name string) *pro.AppInstallerDeployment {
	t.Helper()
	ctx := context.Background()
	ref, err := p.CreateAppInstallerDeploymentV1(ctx, &pro.AppInstallerDeploymentCreate{
		Name:           name,
		AppTitleID:     titleID,
		DeploymentType: "SELF_SERVICE",
		UpdateBehavior: "AUTOMATIC",
		CategoryID:     ptrStr("-1"),
		SiteID:         ptrStr("-1"),
		SmartGroupID:   &smartGroupID,
		Enabled:        ptrFalse(),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAppInstallerDeploymentV1: %v", err)
	}
	if ref == nil || ref.ID == "" {
		t.Fatal("CreateAppInstallerDeploymentV1 returned empty id")
	}
	// Hydrate to a full deployment by re-reading — the href response
	// only carries id + href.
	full, err := p.GetAppInstallerDeploymentV1(ctx, ref.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerDeploymentV1(%s) after create: %v", ref.ID, err)
	}
	return full
}

func ptrStr(s string) *string { return &s }
func ptrFalse() *bool         { b := false; return &b }
