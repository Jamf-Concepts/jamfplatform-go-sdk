// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

// App Installers acceptance coverage — all 24 operations ingested from GitOps
// v2043.
//
// # Why the write surface is gated rather than exercised
//
// A deployment is not a config object: creating one installs software on every
// computer in its scope, and there is no dry-run. So the four deployment writes,
// the three installation retries and the version update are all behind
// JAMFPLATFORM_ACC_PRO_APP_INSTALLERS_WRITE_OK, and the reads that need a
// deployment are probed with an ID that cannot exist instead. A 404 there is the
// pass: it proves the path is routed and looking the ID up, which is what the
// generated method's URL construction and response decoding are being tested
// for. Provisioning real software installs to assert a decode is the wrong
// trade.
//
// # The client
//
// accClient, like every other pro test. It prefers environment scope, and the
// environment credential is verified to hold the applications capability these
// operations need. Whether a post-GA TENANT credential can reach them is
// unverified — the tenant credentials available on 2026-09-03 had all been
// revoked by the time the gateway opened, so this is one to re-check rather than
// assume; see the note on JAMFPLATFORM_ACC_PRO_APP_INSTALLERS_WRITE_OK below.

import (
	"context"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// noSuchDeployment is an ID no deployment can have. Used to reach the
// deployment-scoped reads without provisioning one.
//
// It must be a POSITIVE numeric string. "0" looks like the natural choice and is
// rejected before lookup: the deployment endpoints validate the identifier's
// shape first, answering
// 400 "deploymentId: id field must be string of positive numeric value or -1"
// (wire-verified 2026-09-03 on all four deployment-scoped reads). That is a 400,
// not the 404 these probes assert, so "0" would have made every one of them fail
// for the wrong reason. -1 is accepted by the validator but is Jamf Pro's
// conventional "none" sentinel, so a large positive value is the honest probe.
//
// computerId on the per-computer retry carries the identical constraint and the
// identical message, so noSuchComputer exists for the same reason.
const (
	noSuchDeployment = "999999999"
	noSuchComputer   = "999999999"
)

// appInstallersWriteGate names the opt-in for anything that mutates a
// deployment. Deliberately its own gate rather than the suite-wide destructive
// one: the blast radius here is software installed on real computers, which is
// larger than most things that variable covers.
const appInstallersWriteGate = "JAMFPLATFORM_ACC_PRO_APP_INSTALLERS_WRITE_OK"

// TestAcceptance_Pro_AppInstallers_FeatureState pins the feature-availability
// probe. jss annotates it @InternalOnlyApiCall, and public-apis-oas published it
// anyway, so it is covered here to keep that decision honest: if it is ever
// withdrawn, this fails rather than the whole file quietly losing an operation.
func TestAcceptance_Pro_AppInstallers_FeatureState(t *testing.T) {
	p := pro.New(accClient(t))

	state, err := p.GetAppInstallersFeatureStateV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallersFeatureStateV1: %v", err)
	}
	t.Logf("cloudServicesEnabled=%v features=%v", state.CloudServicesEnabled, state.Features)
}

// TestAcceptance_Pro_AppInstallers_TitleReads walks the catalogue: the paginated
// title list, one title's detail, and that title's versions.
//
// The list is the only operation here with real data behind it on a fresh
// tenant, so it is also where pagination gets exercised — ListAppInstallerTitlesV1
// walks pages internally and the count it returns is the assertion.
func TestAcceptance_Pro_AppInstallers_TitleReads(t *testing.T) {
	p := pro.New(accClient(t))
	ctx := context.Background()

	titles, err := p.ListAppInstallerTitlesV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerTitlesV1: %v", err)
	}
	t.Logf("app installer titles: %d", len(titles))
	if len(titles) == 0 {
		t.Skip("catalogue is empty on this instance, so there is no title to read")
	}

	first := titles[0]
	if first.ID == "" {
		t.Fatalf("title 0 has no ID: %+v", first)
	}

	detail, err := p.GetAppInstallerTitleV1(ctx, first.ID, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerTitleV1(%s): %v", first.ID, err)
	}
	t.Logf("title %s: %+v", first.ID, detail)

	versions, err := p.ListAppInstallerTitleVersionsV1(ctx, first.ID, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerTitleVersionsV1(%s): %v", first.ID, err)
	}
	t.Logf("title %s versions: totalCount=%v", first.ID, versions.TotalCount)
}

// TestAcceptance_Pro_AppInstallers_GlobalSettingsReads covers the settings
// singleton, its deployment-control defaults and its history.
//
// Read-only: UpdateAppInstallerGlobalSettingsV1 is a full replacement of a real
// tenant's App Installers configuration, so it is gated separately below rather
// than round-tripped here.
func TestAcceptance_Pro_AppInstallers_GlobalSettingsReads(t *testing.T) {
	p := pro.New(accClient(t))
	ctx := context.Background()

	settings, err := p.GetAppInstallerGlobalSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerGlobalSettingsV1: %v", err)
	}
	t.Logf("global settings: %+v", settings)

	defaults, err := p.GetAppInstallerDeploymentControlsDefaultsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAppInstallerDeploymentControlsDefaultsV1: %v", err)
	}
	t.Logf("deployment-control defaults: %+v", defaults)

	history, err := p.ListAppInstallerGlobalSettingsHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerGlobalSettingsHistoryV1: %v", err)
	}
	t.Logf("global settings history entries: %d", len(history))
}

// TestAcceptance_Pro_AppInstallers_DeploymentReads covers the deployment list
// and, via an ID that cannot exist, the four deployment-scoped reads.
//
// A 404 on the scoped reads is the pass. It proves the generated method built
// the right URL and that the error decodes as an API error rather than a
// transport failure — which is what can actually regress. Asserting the decoded
// body of a real deployment would require creating one, and creating one
// installs software.
func TestAcceptance_Pro_AppInstallers_DeploymentReads(t *testing.T) {
	p := pro.New(accClient(t))
	ctx := context.Background()

	deployments, err := p.ListAppInstallerDeploymentsV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerDeploymentsV1: %v", err)
	}
	t.Logf("deployments: %d", len(deployments))

	// If a deployment happens to exist, read it properly — that is strictly
	// better evidence than the doomed-ID probe, and costs nothing.
	if len(deployments) > 0 && deployments[0].ID != "" {
		id := deployments[0].ID
		if got, err := p.GetAppInstallerDeploymentV1(ctx, id); err != nil {
			t.Errorf("GetAppInstallerDeploymentV1(%s): %v", id, err)
		} else {
			t.Logf("deployment %s: %+v", id, got)
		}
		if sum, err := p.GetAppInstallerDeploymentInstallationSummaryV1(ctx, id); err != nil {
			t.Errorf("GetAppInstallerDeploymentInstallationSummaryV1(%s): %v", id, err)
		} else {
			t.Logf("installation summary %s: %+v", id, sum)
		}
		if computers, err := p.ListAppInstallerDeploymentComputersV1(ctx, id, nil, ""); err != nil {
			t.Errorf("ListAppInstallerDeploymentComputersV1(%s): %v", id, err)
		} else {
			t.Logf("deployment %s computers: %d", id, len(computers))
		}
		if hist, err := p.ListAppInstallerDeploymentHistoryV1(ctx, id, nil, ""); err != nil {
			t.Errorf("ListAppInstallerDeploymentHistoryV1(%s): %v", id, err)
		} else {
			t.Logf("deployment %s history: %d", id, len(hist))
		}
		return
	}

	t.Logf("no deployment exists, so the scoped reads are probed with ID %s", noSuchDeployment)
	assertNotFound(t, "GetAppInstallerDeploymentV1", func() error {
		_, err := p.GetAppInstallerDeploymentV1(ctx, noSuchDeployment)
		return err
	})
	assertNotFound(t, "GetAppInstallerDeploymentInstallationSummaryV1", func() error {
		_, err := p.GetAppInstallerDeploymentInstallationSummaryV1(ctx, noSuchDeployment)
		return err
	})
	assertNotFound(t, "ListAppInstallerDeploymentComputersV1", func() error {
		_, err := p.ListAppInstallerDeploymentComputersV1(ctx, noSuchDeployment, nil, "")
		return err
	})
	assertNotFound(t, "ListAppInstallerDeploymentHistoryV1", func() error {
		_, err := p.ListAppInstallerDeploymentHistoryV1(ctx, noSuchDeployment, nil, "")
		return err
	})
}

// TestAcceptance_Pro_AppInstallers_Export covers the deployments export.
//
// A POST, but read-only in effect: it renders the deployment list rather than
// changing anything, which is why it is here and not behind the write gate. The
// method returns []byte because the operation declares no JSON response schema.
func TestAcceptance_Pro_AppInstallers_Export(t *testing.T) {
	p := pro.New(accClient(t))

	out, err := p.ExportAppInstallerDeploymentsV1(context.Background(), &pro.ExportParameters{}, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ExportAppInstallerDeploymentsV1: %v", err)
	}
	t.Logf("export returned %d bytes", len(out))
}

// TestAcceptance_Pro_AppInstallers_CacheUpdateNeedsDebugMode pins a block rather
// than asserting success.
//
// POST /v1/app-installers/titles/{id}/cache-update calls jss's
// assertDebugModeEnabled() and answers 404 unless the DEBUG_MODE feature toggle
// is set, which it is not on a normal instance. public-apis-oas#430 published it
// anyway, and its own PR body flagged that as the one call resting on a decision
// rather than on the spec. So this asserts the 404 and fails the day it lifts,
// the same shape as the SDK's other generated-but-refused operations.
func TestAcceptance_Pro_AppInstallers_CacheUpdateNeedsDebugMode(t *testing.T) {
	p := pro.New(accClient(t))
	ctx := context.Background()

	titles, err := p.ListAppInstallerTitlesV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppInstallerTitlesV1: %v", err)
	}
	if len(titles) == 0 || titles[0].ID == "" {
		t.Skip("no title to address")
	}

	err = p.RefreshAppInstallerTitleCacheV1(ctx, titles[0].ID)
	if err == nil {
		t.Fatalf("RefreshAppInstallerTitleCacheV1(%s) succeeded — DEBUG_MODE is enabled on this instance, "+
			"or the assertDebugModeEnabled() guard has been removed. Replace this test with real coverage.", titles[0].ID)
	}
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("RefreshAppInstallerTitleCacheV1: non-API error, the request did not reach the endpoint: %v", err)
	}
	if !apiErr.HasStatus(404) {
		t.Fatalf("RefreshAppInstallerTitleCacheV1: want 404 (DEBUG_MODE off), got %d: %v", apiErr.StatusCode, err)
	}
	t.Log("RefreshAppInstallerTitleCacheV1: still gated behind DEBUG_MODE (404), as published")
}

// TestAcceptance_Pro_AppInstallers_Writes is deliberately gated.
//
// Every operation here changes what is installed on real computers:
// Create/Update/Delete a deployment, the three installation retries, the version
// update, and the two history notes that write to a real object's audit trail.
// A deployment has no dry-run and no scope-free form, so there is no safe
// probe-only path the way there is for the reads above.
func TestAcceptance_Pro_AppInstallers_Writes(t *testing.T) {
	requireWriteOptIn(t, appInstallersWriteGate,
		"Creating an App Installer deployment installs software on every computer in its scope, and the "+
			"retries and version update act on real installations. Needs an instance reserved for it.")

	p := pro.New(accClient(t))
	ctx := context.Background()

	// Reached only under the gate, so the shape is asserted rather than the
	// blast radius risked: the doomed-ID probes confirm the retry, version-update
	// and history-note methods build the right URL and decode their errors.
	assertNotFound(t, "RetryAppInstallerDeploymentInstallationsV1", func() error {
		return p.RetryAppInstallerDeploymentInstallationsV1(ctx, noSuchDeployment)
	})
	assertNotFound(t, "RetryAppInstallerDeploymentComputerInstallationV1", func() error {
		return p.RetryAppInstallerDeploymentComputerInstallationV1(ctx, noSuchDeployment, noSuchComputer)
	})
	assertNotFound(t, "UpdateAppInstallerDeploymentVersionV1", func() error {
		return p.UpdateAppInstallerDeploymentVersionV1(ctx, noSuchDeployment, &pro.AppTitleVersion{})
	})
	assertNotFound(t, "CreateAppInstallerDeploymentHistoryNoteV1", func() error {
		_, err := p.CreateAppInstallerDeploymentHistoryNoteV1(ctx, noSuchDeployment, &pro.ObjectHistoryNote{Note: "sdk-acc probe"})
		return err
	})
	assertNotFound(t, "DeleteAppInstallerDeploymentV1", func() error {
		return p.DeleteAppInstallerDeploymentV1(ctx, noSuchDeployment)
	})
	assertNotFound(t, "UpdateAppInstallerDeploymentV1", func() error {
		_, err := p.UpdateAppInstallerDeploymentV1(ctx, noSuchDeployment, &pro.AppTitleDeployment{})
		return err
	})

	// RetryAppInstallerInstallationsV1 takes no ID at all, so it cannot be
	// probed harmlessly — it retries across the tenant. Only its reachability is
	// asserted, and only under the gate.
	if err := p.RetryAppInstallerInstallationsV1(ctx); err != nil {
		if apiErr := jamfplatform.AsAPIError(err); apiErr == nil {
			t.Errorf("RetryAppInstallerInstallationsV1: non-API error: %v", err)
		} else {
			t.Logf("RetryAppInstallerInstallationsV1 -> %d: %s", apiErr.StatusCode, apiErr.Summary())
		}
	} else {
		t.Log("RetryAppInstallerInstallationsV1 accepted (204)")
	}

	// CreateAppInstallerDeploymentV1 and the global-settings writers are left to
	// a human even under the gate: the first installs software, the second
	// replaces a real tenant's configuration wholesale.
	t.Log("CreateAppInstallerDeploymentV1, UpdateAppInstallerGlobalSettingsV1 and " +
		"CreateAppInstallerGlobalSettingsHistoryNoteV1 are not exercised even under the gate — see the file comment")
}

// assertNotFound runs an operation expected to 404 for an identifier that cannot
// exist, and fails on anything else. The point is the URL and the error decode,
// not the absence itself: a 404 says the request reached the endpoint and the
// endpoint looked the ID up.
func assertNotFound(t *testing.T, op string, call func() error) {
	t.Helper()
	err := call()
	if err == nil {
		t.Errorf("%s(%s) succeeded for an ID that cannot exist", op, noSuchDeployment)
		return
	}
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		t.Errorf("%s: non-API error, the request did not reach the endpoint: %v", op, err)
		return
	}
	if !apiErr.HasStatus(404) {
		t.Errorf("%s: want 404 for a nonexistent ID, got %d: %s", op, apiErr.StatusCode, apiErr.Summary())
		return
	}
	t.Logf("%s: 404 for a nonexistent ID, as expected", op)
}
