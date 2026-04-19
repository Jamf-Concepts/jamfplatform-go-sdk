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

// TeamViewer remote-administration preview. Full CRUD is exercised
// against the live tenant — the server accepts any script-token
// string at create time and defers Apple/TeamViewer validation to
// session creation, so we can round-trip a throwaway configuration
// with explicit cleanup. Session endpoints still need a real device
// fixture and stay tolerate-only.

func TestAcceptance_Pro_PreviewComputersAndRemoteAdmin(t *testing.T) {
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

	tolerate("DeleteTeamViewerConfiguration (bogus id)", p.DeleteTeamViewerConfiguration(ctx, bogus))
}
