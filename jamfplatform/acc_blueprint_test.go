// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/bpcomponents/declarations"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/bpcomponents/swupdate"
)

func createTestBlueprint(t *testing.T, c *jamfplatform.Client, name string, groupID string, steps []blueprints.BlueprintStep) *blueprints.BlueprintDetail {
	t.Helper()
	ctx := context.Background()
	bp := blueprints.New(c)

	desc := "SDK acceptance test — safe to delete"
	resp, err := bp.CreateBlueprint(ctx, &blueprints.CreateBlueprintRequest{
		Name:        name,
		Description: &desc,
		Scope:       blueprints.CreateScope{DeviceGroups: []string{groupID}},
		Steps:       steps,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBlueprint failed for %q: %v", name, err)
	}
	cleanupDelete(t, "DeleteBlueprint", func() error { return bp.DeleteBlueprint(ctx, resp.ID) })

	got, err := bp.GetBlueprint(ctx, resp.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetBlueprint failed for %q: %v", name, err)
	}
	return got
}

func makeStep(identifier string, config any) []blueprints.BlueprintStep {
	configJSON, _ := json.Marshal(config)
	stepName := "Step 1"
	return []blueprints.BlueprintStep{
		{
			Name: &stepName,
			Components: []blueprints.Component{
				{
					Identifier:    identifier,
					Configuration: json.RawMessage(configJSON),
				},
			},
		},
	}
}

func TestAcceptance_ListBlueprints(t *testing.T) {
	c := accClient(t)

	bps, err := blueprints.New(c).ListBlueprints(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBlueprints failed: %v", err)
	}
	t.Logf("Found %d blueprints", len(bps))
}

func TestAcceptance_ListBlueprintsWithSearch(t *testing.T) {
	c := accClient(t)

	bps, err := blueprints.New(c).ListBlueprints(context.Background(), nil, "sdk-acc")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBlueprints with search failed: %v", err)
	}
	t.Logf("Found %d blueprints matching 'sdk-acc'", len(bps))
}

func TestAcceptance_ListBlueprintComponents(t *testing.T) {
	c := accClient(t)

	comps, err := blueprints.New(c).ListBlueprintComponents(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBlueprintComponents failed: %v", err)
	}
	if len(comps) == 0 {
		t.Log("No blueprint components found")
		return
	}
	t.Logf("Found %d blueprint components", len(comps))
	for _, comp := range comps {
		t.Logf("  %s (%s)", comp.Name, comp.Identifier)
	}
}

func TestAcceptance_GetBlueprintComponent(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	bp := blueprints.New(c)

	comps, err := bp.ListBlueprintComponents(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBlueprintComponents failed: %v", err)
	}
	if len(comps) == 0 {
		t.Skip("No blueprint components available")
	}

	comp, err := bp.GetBlueprintComponent(ctx, comps[0].Identifier)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetBlueprintComponent failed for %q: %v", comps[0].Identifier, err)
	}
	if comp.Identifier != comps[0].Identifier {
		t.Errorf("expected %q, got %q", comps[0].Identifier, comp.Identifier)
	}
	t.Logf("Read component: %s (%s)", comp.Name, comp.Identifier)
}

func TestAcceptance_Blueprint_EmptyBlueprint(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)

	name := "sdk-acc-empty-bp-" + runSuffix()
	bp := createTestBlueprint(t, c, name, groupID, []blueprints.BlueprintStep{})

	if bp.Name != name {
		t.Errorf("expected name %q, got %q", name, bp.Name)
	}
	if len(bp.Scope.DeviceGroups) != 1 || bp.Scope.DeviceGroups[0] != groupID {
		t.Errorf("unexpected scope: %v", bp.Scope.DeviceGroups)
	}
	t.Logf("Created empty blueprint ID: %s", bp.ID)
}

func TestAcceptance_Blueprint_PasscodePolicy(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)

	boolTrue := true
	minLen := 8
	maxFailed := 5
	reuseLimit := 1

	steps := makeStep("com.jamf.ddm.passcode-settings", declarations.PasscodeSettingsConfigurationV2{
		RequirePasscode: &declarations.RequirePasscode{
			Included: &boolTrue,
			Value:    &boolTrue,
		},
		MinimumLength: &declarations.MinimumLength{
			Included: &boolTrue,
			Value:    &minLen,
		},
		MaximumFailedAttempts: &declarations.MaximumFailedAttempts{
			Included: &boolTrue,
			Value:    &maxFailed,
		},
		PasscodeReuseLimit: &declarations.PasscodeReuseLimit{
			Included: &boolTrue,
			Value:    &reuseLimit,
		},
		Version: 2,
	})
	bp := createTestBlueprint(t, c, "sdk-acc-passcode-"+runSuffix(), groupID, steps)

	if len(bp.Steps) == 0 || len(bp.Steps[0].Components) == 0 {
		t.Fatal("expected at least one step with one component")
	}
	if bp.Steps[0].Components[0].Identifier != "com.jamf.ddm.passcode-settings" {
		t.Errorf("unexpected identifier: %q", bp.Steps[0].Components[0].Identifier)
	}
	t.Logf("Created passcode blueprint ID: %s", bp.ID)
}

func TestAcceptance_Blueprint_UpdateAndRead(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)
	ctx := context.Background()
	suffix := runSuffix()
	bpClient := blueprints.New(c)

	boolTrue := true
	minLen := 6
	maxFailed := 3
	reuseLimit := 1

	steps := makeStep("com.jamf.ddm.passcode-settings", declarations.PasscodeSettingsConfigurationV2{
		RequirePasscode: &declarations.RequirePasscode{
			Included: &boolTrue,
			Value:    &boolTrue,
		},
		MinimumLength: &declarations.MinimumLength{
			Included: &boolTrue,
			Value:    &minLen,
		},
		MaximumFailedAttempts: &declarations.MaximumFailedAttempts{
			Included: &boolTrue,
			Value:    &maxFailed,
		},
		PasscodeReuseLimit: &declarations.PasscodeReuseLimit{
			Included: &boolTrue,
			Value:    &reuseLimit,
		},
		Version: 2,
	})
	bp := createTestBlueprint(t, c, "sdk-acc-update-test-"+suffix, groupID, steps)

	renamedName := "sdk-acc-update-renamed-" + suffix
	updatedDesc := "Updated description"
	err := bpClient.UpdateBlueprint(ctx, bp.ID, &blueprints.UpdateBlueprintRequest{
		Name:        &renamedName,
		Description: &updatedDesc,
		Scope:       &blueprints.BlueprintScope{DeviceGroups: []string{groupID}},
		Steps:       &steps,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateBlueprint failed: %v", err)
	}

	updated, err := bpClient.GetBlueprint(ctx, bp.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetBlueprint after update failed: %v", err)
	}
	if updated.Name != renamedName {
		t.Errorf("expected name %q, got %q", renamedName, updated.Name)
	}
	if updated.Description == nil || *updated.Description != "Updated description" {
		t.Errorf("expected updated description, got %v", updated.Description)
	}
	t.Logf("Updated blueprint ID: %s", bp.ID)
}

func TestAcceptance_Blueprint_PartialUpdatePreservesSteps(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)
	ctx := context.Background()
	suffix := runSuffix()
	bpClient := blueprints.New(c)

	boolTrue := true
	minLen := 6
	maxFailed := 3
	reuseLimit := 1

	steps := makeStep("com.jamf.ddm.passcode-settings", declarations.PasscodeSettingsConfigurationV2{
		RequirePasscode: &declarations.RequirePasscode{
			Included: &boolTrue,
			Value:    &boolTrue,
		},
		MinimumLength: &declarations.MinimumLength{
			Included: &boolTrue,
			Value:    &minLen,
		},
		MaximumFailedAttempts: &declarations.MaximumFailedAttempts{
			Included: &boolTrue,
			Value:    &maxFailed,
		},
		PasscodeReuseLimit: &declarations.PasscodeReuseLimit{
			Included: &boolTrue,
			Value:    &reuseLimit,
		},
		Version: 2,
	})
	bp := createTestBlueprint(t, c, "sdk-acc-partial-update-"+suffix, groupID, steps)

	if len(bp.Steps) == 0 {
		t.Fatal("expected blueprint to have steps after creation")
	}

	// Update only the name — omit Steps entirely.
	// Before the fix, this would serialize "steps":[] and wipe them.
	renamedName := "sdk-acc-partial-renamed-" + suffix
	err := bpClient.UpdateBlueprint(ctx, bp.ID, &blueprints.UpdateBlueprintRequest{
		Name: &renamedName,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateBlueprint (partial) failed: %v", err)
	}

	updated, err := bpClient.GetBlueprint(ctx, bp.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetBlueprint after partial update failed: %v", err)
	}
	if updated.Name != renamedName {
		t.Errorf("expected name %q, got %q", renamedName, updated.Name)
	}
	if len(updated.Steps) != len(bp.Steps) {
		t.Errorf("steps were lost: expected %d steps, got %d", len(bp.Steps), len(updated.Steps))
	}
	t.Logf("Partial update preserved %d steps on blueprint %s", len(updated.Steps), bp.ID)
}

func TestAcceptance_Blueprint_Report(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)
	ctx := context.Background()
	bpClient := blueprints.New(c)

	boolTrue := true
	minLen := 6
	maxFailed := 3
	reuseLimit := 1

	steps := makeStep("com.jamf.ddm.passcode-settings", declarations.PasscodeSettingsConfigurationV2{
		RequirePasscode: &declarations.RequirePasscode{
			Included: &boolTrue,
			Value:    &boolTrue,
		},
		MinimumLength: &declarations.MinimumLength{
			Included: &boolTrue,
			Value:    &minLen,
		},
		MaximumFailedAttempts: &declarations.MaximumFailedAttempts{
			Included: &boolTrue,
			Value:    &maxFailed,
		},
		PasscodeReuseLimit: &declarations.PasscodeReuseLimit{
			Included: &boolTrue,
			Value:    &reuseLimit,
		},
		Version: 2,
	})
	bp := createTestBlueprint(t, c, "sdk-acc-report-"+runSuffix(), groupID, steps)

	if err := bpClient.DeployBlueprint(ctx, bp.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeployBlueprint failed: %v", err)
	}
	t.Cleanup(func() { _ = bpClient.UndeployBlueprint(ctx, bp.ID) })

	pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := jamfplatform.PollUntil(pollCtx, 2*time.Second, func(ctx context.Context) (bool, error) {
		got, err := bpClient.GetBlueprint(ctx, bp.ID)
		if err != nil {
			return false, err
		}
		return got.DeploymentState.State != "NOT_DEPLOYED", nil
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("Timed out waiting for deployment: %v", err)
	}

	report, err := bpClient.GetBlueprintReport(ctx, bp.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetBlueprintReport failed: %v", err)
	}
	t.Logf("Report for %s: succeeded=%d failed=%d pending=%d", bp.ID, report.Succeeded, report.Failed, report.Pending)
}

// TestAcceptance_Blueprint_TypedComponents creates a blueprint for each
// component type using the generated typed configuration structs from the
// bpcomponents packages. This proves that typed configs marshal correctly
// and are accepted by the Jamf API. Components not available on the tenant
// are skipped automatically.
func TestAcceptance_Blueprint_TypedComponents(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)

	// Query available components so we can skip identifiers not present on this tenant.
	available, err := blueprints.New(c).ListBlueprintComponents(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBlueprintComponents failed: %v", err)
	}
	enabledIDs := make(map[string]bool, len(available))
	for _, comp := range available {
		enabledIDs[comp.Identifier] = true
	}

	boolTrue := true
	boolFalse := false
	extState := "Allowed"
	minLen := 8
	maxFailed := 5
	reuseLimit := 2
	maxInactivity := 15
	maxGrace := 5
	maxAge := 90
	acceptCookies := "Always"
	unpairingHour := 17
	extStorage := "ReadOnly"
	deferral2 := 2
	deferral3 := 3
	deferral4 := 4
	deferral5 := 5

	cases := []struct {
		name       string
		identifier string
		config     any
	}{
		{
			name:       "PasscodeSettings",
			identifier: "com.jamf.ddm.passcode-settings",
			config: declarations.PasscodeSettingsConfigurationV2{
				RequirePasscode: &declarations.RequirePasscode{
					Included: &boolTrue,
					Value:    &boolTrue,
				},
				MinimumLength: &declarations.MinimumLength{
					Included: &boolTrue,
					Value:    &minLen,
				},
				MaximumFailedAttempts: &declarations.MaximumFailedAttempts{
					Included: &boolTrue,
					Value:    &maxFailed,
				},
				PasscodeReuseLimit: &declarations.PasscodeReuseLimit{
					Included: &boolTrue,
					Value:    &reuseLimit,
				},
				MaximumInactivityInMinutes: &declarations.MaximumInactivityInMinutes{
					Included: &boolTrue,
					Value:    &maxInactivity,
				},
				MaximumGracePeriodInMinutes: &declarations.MaximumGracePeriodInMinutes{
					Included: &boolTrue,
					Value:    &maxGrace,
				},
				MaximumPasscodeAgeInDays: &declarations.MaximumPasscodeAgeInDays{
					Included: &boolTrue,
					Value:    &maxAge,
				},
				RequireAlphanumericPasscode: &declarations.RequireAlphanumericPasscode{
					Included: &boolTrue,
					Value:    &boolFalse,
				},
				Version: 2,
			},
		},
		{
			name:       "SoftwareUpdateSettings",
			identifier: "com.jamf.ddm.software-update-settings",
			config: declarations.SoftwareUpdateSettingsConfiguration{
				AllowStandardUserOSUpdates: &declarations.OptionallyEnabled{
					Included: &boolTrue,
					Enabled:  true,
				},
				AutomaticActions: &declarations.AutomaticActions{
					Download: &declarations.AutomaticAction{
						Included: &boolTrue,
						Value:    "AlwaysOn",
					},
					InstallOSUpdates: &declarations.AutomaticAction{
						Included: &boolTrue,
						Value:    "AlwaysOn",
					},
					InstallSecurityUpdate: &declarations.AutomaticAction{
						Included: &boolTrue,
						Value:    "AlwaysOn",
					},
				},
				Beta: &declarations.Beta{
					Included: &boolTrue,
					Value: &declarations.BetaSettings{
						ProgramEnrollment: "Allowed",
						OfferPrograms: &[]declarations.BetaProgram{
							{Token: "test", Description: "test"},
						},
					},
				},
				Deferrals: &declarations.Deferrals{
					CombinedPeriodInDays: &declarations.OptionalPeriodInDays{Included: &boolTrue, Value: &deferral2},
					MajorPeriodInDays:    &declarations.OptionalPeriodInDays{Included: &boolTrue, Value: &deferral3},
					MinorPeriodInDays:    &declarations.OptionalPeriodInDays{Included: &boolTrue, Value: &deferral4},
					SystemPeriodInDays:   &declarations.OptionalPeriodInDays{Included: &boolTrue, Value: &deferral5},
				},
				Notifications: &declarations.OptionallyEnabled{
					Included: &boolTrue,
					Enabled:  false,
				},
				RapidSecurityResponse: &declarations.RapidSecurityResponse{
					Enable:         &declarations.OptionallyEnabled{Included: &boolTrue, Enabled: true},
					EnableRollback: &declarations.OptionallyEnabled{Included: &boolTrue, Enabled: false},
				},
				RecommendedCadence: &declarations.RecommendedCadence{
					Included: &boolTrue,
					Value:    "Newest",
				},
			},
		},
		{
			name:       "SafariSettings",
			identifier: "com.jamf.ddm.safari-settings",
			config: declarations.SafariSettingsConfiguration{
				AllowPopups: &declarations.AllowPopups{
					Included: &boolTrue,
					Value:    &boolFalse,
				},
				AllowJavaScript: &declarations.AllowJavaScript{
					Included: &boolTrue,
					Value:    &boolTrue,
				},
				AllowPrivateBrowsing: &declarations.AllowPrivateBrowsing{
					Included: &boolTrue,
					Value:    &boolTrue,
				},
				AcceptCookies: &declarations.AcceptCookies{
					Included: &boolTrue,
					Value:    &acceptCookies,
				},
			},
		},
		{
			name:       "SafariExtensions",
			identifier: "com.jamf.ddm.safari-extensions",
			config: declarations.SafariExtensionsConfiguration{
				ManagedExtensions: map[string]declarations.ManagedExtension{
					"com.example.test-extension": {State: &extState},
				},
			},
		},
		{
			name:       "SafariBookmarks",
			identifier: "com.jamf.ddm.safari-bookmarks",
			config: declarations.SafariBookmarksConfiguration{
				ManagedBookmarks: []declarations.BookmarkGroup{
					{
						Title:           "SDK Test Bookmarks",
						GroupIdentifier: "sdk-acc-test-group",
						Bookmarks:       []any{map[string]any{"Type": "BOOKMARK", "Title": "Jamf", "URL": "https://www.jamf.com"}},
					},
				},
			},
		},
		{
			name:       "DiskManagement",
			identifier: "com.jamf.ddm.disk-management",
			config: declarations.DiskManagementSettingsConfigurationV1{
				Restrictions: &declarations.RestrictionsV1{
					ExternalStorage: &extStorage,
				},
				Version: ptr(1),
			},
		},
		{
			name:       "AudioAccessorySettings",
			identifier: "com.jamf.ddm.audio-accessory-settings",
			config: declarations.AudioAccessorySettingsConfiguration{
				TemporaryPairing: &declarations.TemporaryPairing{
					Included: &boolTrue,
					Disabled: &boolFalse,
					Configuration: &declarations.Configuration{
						UnpairingTime: declarations.UnpairingTime{
							Policy: "Hour",
							Hour:   &unpairingHour,
						},
					},
				},
			},
		},
		{
			name:       "MathSettings",
			identifier: "com.jamf.ddm.math-settings",
			config: declarations.MathSettingsConfiguration{
				Calculator: &declarations.Calculator{
					BasicMode: &declarations.BasicMode{
						Included:      &boolTrue,
						AddSquareRoot: true,
					},
					InputModes: &declarations.InputModes{
						Included:       &boolTrue,
						RPN:            false,
						UnitConversion: true,
					},
				},
				SystemBehavior: &declarations.SystemBehavior{
					Included:            &boolTrue,
					KeyboardSuggestions: true,
					MathNotes:           true,
				},
			},
		},
		{
			name:       "FreeForm",
			identifier: "com.jamf.ddm.free-form",
			config: declarations.FreeFormConfiguration{
				Declarations: []declarations.Declaration{
					{Kind: "CONFIGURATION", Type: "com.apple.configuration.passcode.settings", Payload: json.RawMessage(`{}`)},
				},
			},
		},
		{
			name:       "ServicesBackgroundTasks",
			identifier: "com.jamf.ddm.service-background-tasks",
			config: declarations.ServicesBackgroundTasksConfiguration{
				BackgroundTasks: []declarations.ServiceBackgroundTasksConfiguration{
					{
						TaskType:        "daemon",
						TaskDescription: strPtr("SDK acceptance test task"),
						LaunchdConfigurations: &[]declarations.LaunchdItem{
							{
								Context: "daemon",
								FileAssetReference: declarations.DataAssetReference{
									Reference: declarations.AssetDataReference{DataURL: "https://example.com/test.plist"},
								},
							},
						},
					},
				},
			},
		},
		{
			name:       "ServicesConfigurationFiles",
			identifier: "com.jamf.ddm.service-configuration-files",
			config: declarations.ServicesConfigurationFilesConfiguration{
				ServiceConfigFiles: []declarations.ServiceConfigurationFilesConfiguration{
					{
						ServiceType: "APACHE",
						DataAssetReference: declarations.DataAssetReference{
							Reference: declarations.AssetDataReference{DataURL: "https://example.com/test.conf"},
						},
					},
				},
			},
		},
		{
			name:       "SwUpdate",
			identifier: "com.jamf.ddm.sw-updates",
			config: swupdate.SwUpdateAutomaticConfiguration{
				Strategy:        strPtr("SEMANTIC"),
				EnforcementType: "AUTOMATIC",
				DetailsURL: &swupdate.DetailsURL{
					Included: &boolFalse,
					Value:    strPtr(""),
				},
				Rules: &swupdate.UpdateRules{
					Minor: swupdate.UpdateRule{
						DeploymentTime:   "13:10",
						EnforceAfterDays: 0,
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !enabledIDs[tc.identifier] {
				t.Skipf("component %q not available on this tenant", tc.identifier)
			}

			steps := makeStep(tc.identifier, tc.config)
			bpName := "sdk-acc-typed-" + tc.name + "-" + runSuffix()
			bp := createTestBlueprint(t, c, bpName, groupID, steps)

			if len(bp.Steps) == 0 || len(bp.Steps[0].Components) == 0 {
				t.Fatal("expected at least one step with one component")
			}
			got := bp.Steps[0].Components[0].Identifier
			if got != tc.identifier {
				t.Errorf("expected identifier %q, got %q", tc.identifier, got)
			}
			t.Logf("Created %s blueprint ID: %s", tc.name, bp.ID)
		})
	}
}
