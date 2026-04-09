// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func createTestBlueprint(t *testing.T, c *Client, name string, groupID string, steps []BlueprintStepV1) *BlueprintDetailV1 {
	t.Helper()
	ctx := context.Background()

	resp, err := c.CreateBlueprint(ctx, &BlueprintCreateRequestV1{
		Name:        name,
		Description: "SDK acceptance test — safe to delete",
		Scope:       BlueprintCreateScopeV1{DeviceGroups: []string{groupID}},
		Steps:       steps,
	})
	if err != nil {
		t.Fatalf("CreateBlueprint failed for %q: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteBlueprint(ctx, resp.ID) })

	bp, err := c.GetBlueprint(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetBlueprint failed for %q: %v", name, err)
	}
	return bp
}

func makeStep(identifier string, config any) []BlueprintStepV1 {
	configJSON, _ := json.Marshal(config)
	return []BlueprintStepV1{
		{
			Name: "Step 1",
			Components: []BlueprintComponentV1{
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

	bps, err := c.ListBlueprints(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("ListBlueprints failed: %v", err)
	}
	t.Logf("Found %d blueprints", len(bps))
}

func TestAcceptance_ListBlueprintsWithSearch(t *testing.T) {
	c := accClient(t)

	bps, err := c.ListBlueprints(context.Background(), nil, "sdk-acc")
	if err != nil {
		t.Fatalf("ListBlueprints with search failed: %v", err)
	}
	t.Logf("Found %d blueprints matching 'sdk-acc'", len(bps))
}

func TestAcceptance_ListBlueprintComponents(t *testing.T) {
	c := accClient(t)

	comps, err := c.ListBlueprintComponents(context.Background())
	if err != nil {
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

	comps, err := c.ListBlueprintComponents(ctx)
	if err != nil {
		t.Fatalf("ListBlueprintComponents failed: %v", err)
	}
	if len(comps) == 0 {
		t.Skip("No blueprint components available")
	}

	comp, err := c.GetBlueprintComponent(ctx, comps[0].Identifier)
	if err != nil {
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
	bp := createTestBlueprint(t, c, name, groupID, []BlueprintStepV1{})

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

	steps := makeStep("com.jamf.ddm.passcode-settings", map[string]any{
		"RequirePasscode": true,
		"MinimumLength":   6,
	})
	bp := createTestBlueprint(t, c, "sdk-acc-update-test-"+suffix, groupID, steps)

	renamedName := "sdk-acc-update-renamed-" + suffix
	updatedDesc := "Updated description"
	err := c.UpdateBlueprint(ctx, bp.ID, &BlueprintUpdateRequestV1{
		Name:        &renamedName,
		Description: &updatedDesc,
		Scope:       &BlueprintUpdateScopeV1{DeviceGroups: []string{groupID}},
		Steps:       steps,
	})
	if err != nil {
		t.Fatalf("UpdateBlueprint failed: %v", err)
	}

	updated, err := c.GetBlueprint(ctx, bp.ID)
	if err != nil {
		t.Fatalf("GetBlueprint after update failed: %v", err)
	}
	if updated.Name != renamedName {
		t.Errorf("expected name %q, got %q", renamedName, updated.Name)
	}
	if updated.Description != "Updated description" {
		t.Errorf("expected updated description, got %q", updated.Description)
	}
	t.Logf("Updated blueprint ID: %s", bp.ID)
}

func TestAcceptance_Blueprint_Report(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)
	ctx := context.Background()

	steps := makeStep("com.jamf.ddm.passcode-settings", map[string]any{
		"RequirePasscode": true,
		"MinimumLength":   6,
	})
	bp := createTestBlueprint(t, c, "sdk-acc-report-"+runSuffix(), groupID, steps)

	if err := c.DeployBlueprint(ctx, bp.ID); err != nil {
		t.Fatalf("DeployBlueprint failed: %v", err)
	}
	t.Cleanup(func() { _ = c.UndeployBlueprint(ctx, bp.ID) })

	pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := PollUntil(pollCtx, 2*time.Second, func(ctx context.Context) (bool, error) {
		got, err := c.GetBlueprint(ctx, bp.ID)
		if err != nil {
			return false, err
		}
		return got.DeploymentState.State != "NOT_DEPLOYED", nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for deployment: %v", err)
	}

	report, err := c.GetBlueprintReport(ctx, bp.ID)
	if err != nil {
		t.Fatalf("GetBlueprintReport failed: %v", err)
	}
	t.Logf("Report for %s: succeeded=%d failed=%d pending=%d", bp.ID, report.Succeeded, report.Failed, report.Pending)
}

func TestAcceptance_Blueprint_GetByName(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)

	name := "sdk-acc-find-by-name-" + runSuffix()
	_ = createTestBlueprint(t, c, name, groupID, []BlueprintStepV1{})

	found, err := c.GetBlueprintByName(context.Background(), name)
	if err != nil {
		t.Fatalf("GetBlueprintByName failed: %v", err)
	}
	if found.Name != name {
		t.Errorf("expected name %q, got %q", name, found.Name)
	}
	t.Logf("Found blueprint by name: ID %s", found.ID)
}
