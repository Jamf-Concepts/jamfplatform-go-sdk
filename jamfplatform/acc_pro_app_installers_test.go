// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// App Installers: catalog titles (read-only) + deployment CRUD. Titles
// come from Jamf's shared App Catalog — every tenant sees the same
// ~340 entries. Deployments are per-tenant; tests create + delete
// against the tenant's existing Active Directory Not Bound group
// (smart group id 29, or another empty group via env override) so
// no device actually installs the app during the CRUD cycle.

const (
	appInstallerSmartGroupIDEnv = "JAMFPLATFORM_APP_INSTALLER_SMART_GROUP_ID"
	appInstallerDefaultGroupID  = "29"
	appInstallerSweepPercent    = 10 // CRUD this percentage of the catalog per run
)

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

	// Round-trip by-id lookup.
	got, err := p.GetAppInstallerTitleV1(ctx, first.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerTitleV1(%s): %v", first.ID, err)
	}
	if got.ID != first.ID {
		t.Errorf("title round-trip id mismatch: got %s want %s", got.ID, first.ID)
	}
}

// TestAcceptance_Pro_AppInstallerDeploymentCRUD exercises the full
// deployment lifecycle against a single title. Picks the first catalog
// title, creates a disabled SELF_SERVICE deployment targeting a known-
// empty smart group (so no actual install fires), round-trips the GET,
// updates the enabled flag, then deletes.
func TestAcceptance_Pro_AppInstallerDeploymentCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	groupID := os.Getenv(appInstallerSmartGroupIDEnv)
	if groupID == "" {
		groupID = appInstallerDefaultGroupID
	}

	titles, err := p.ListAppInstallerTitlesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerTitlesV1: %v", err)
	}
	if len(titles) == 0 {
		t.Skip("no App Installer titles available in catalog")
	}
	title := titles[0]

	created := createDeployment(t, p, title.ID, groupID, "sdk-acc-appinst-"+runSuffix())
	id := *created.ID
	cleanupDelete(t, "AppInstallerDeployment "+id, func() error { return p.DeleteAppInstallerDeploymentV1(ctx, id) })
	t.Logf("Created app-installer deployment id=%s for title %s (%s)", id, title.ID, title.TitleName)

	got, err := p.GetAppInstallerDeploymentV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerDeploymentV1(%s): %v", id, err)
	}
	if got.AppTitleID == nil || *got.AppTitleID != title.ID {
		t.Errorf("deployment round-trip appTitleId mismatch: got %v want %s", got.AppTitleID, title.ID)
	}
	if got.Enabled == nil || *got.Enabled {
		t.Errorf("deployment enabled=%v, expected false (disabled create)", got.Enabled)
	}

	// Update — flip enabled still to false (no-op semantic but verifies PUT).
	disabled := false
	got.Enabled = &disabled
	if _, err := p.UpdateAppInstallerDeploymentV1(ctx, id, got); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateAppInstallerDeploymentV1: %v", err)
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

	groupID := os.Getenv(appInstallerSmartGroupIDEnv)
	if groupID == "" {
		groupID = appInstallerDefaultGroupID
	}

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
		dep, err := p.CreateAppInstallerDeploymentV1(ctx, &pro.AppInstallerDeployment{
			Name:           &name,
			AppTitleID:     &title.ID,
			Enabled:        ptrFalse(),
			DeploymentType: ptrStr("SELF_SERVICE"),
			UpdateBehavior: ptrStr("AUTOMATIC"),
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
		if got.AppTitleID == nil || *got.AppTitleID != title.ID {
			t.Errorf("get[%d] appTitleId mismatch: got %v want %s", i, got.AppTitleID, title.ID)
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

// Helpers -------------------------------------------------------------

func createDeployment(t *testing.T, p *pro.Client, titleID, smartGroupID, name string) *pro.AppInstallerDeployment {
	t.Helper()
	ctx := context.Background()
	dep, err := p.CreateAppInstallerDeploymentV1(ctx, &pro.AppInstallerDeployment{
		Name:           &name,
		AppTitleID:     &titleID,
		Enabled:        ptrFalse(),
		DeploymentType: ptrStr("SELF_SERVICE"),
		UpdateBehavior: ptrStr("AUTOMATIC"),
		CategoryID:     ptrStr("-1"),
		SiteID:         ptrStr("-1"),
		SmartGroupID:   &smartGroupID,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAppInstallerDeploymentV1: %v", err)
	}
	if dep == nil || dep.ID == "" {
		t.Fatal("CreateAppInstallerDeploymentV1 returned empty id")
	}
	// Hydrate to a full deployment by re-reading — the href response
	// only carries id + href.
	full, err := p.GetAppInstallerDeploymentV1(ctx, dep.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerDeploymentV1(%s) after create: %v", dep.ID, err)
	}
	return full
}

func ptrStr(s string) *string { return &s }
func ptrFalse() *bool         { b := false; return &b }
