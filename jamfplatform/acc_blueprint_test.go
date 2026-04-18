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
	t.Cleanup(func() { _ = bp.DeleteBlueprint(ctx, resp.ID) })

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

	steps := makeStep("com.jamf.ddm.passcode-settings", map[string]any{
		"RequirePasscode": true,
		"MinimumLength":   8,
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

	steps := makeStep("com.jamf.ddm.passcode-settings", map[string]any{
		"RequirePasscode": true,
		"MinimumLength":   6,
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

	steps := makeStep("com.jamf.ddm.passcode-settings", map[string]any{
		"RequirePasscode": true,
		"MinimumLength":   6,
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

	steps := makeStep("com.jamf.ddm.passcode-settings", map[string]any{
		"RequirePasscode": true,
		"MinimumLength":   6,
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
