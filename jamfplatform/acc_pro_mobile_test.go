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

// --- advanced mobile device searches -----------------------------------

func TestAcceptance_Pro_Mobile_ListAdvancedSearchesV1(t *testing.T) {
	c := accClient(t)

	res, err := pro.New(c).ListAdvancedMobileDeviceSearchesV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAdvancedMobileDeviceSearchesV1: %v", err)
	}
	t.Logf("Found %d advanced mobile device searches", len(res.Results))
}

func TestAcceptance_Pro_Mobile_ListAdvancedSearchChoicesV1(t *testing.T) {
	c := accClient(t)

	// Probe with any criterion — plumbing-only; empty criteria is acceptable.
	choices, err := pro.New(c).ListAdvancedMobileDeviceSearchChoicesV1(context.Background(), "Last Inventory Update", "", "")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("ListAdvancedMobileDeviceSearchChoicesV1 rejected (tenant-specific criteria): %v — plumbing OK", err)
			return
		}
		t.Fatalf("ListAdvancedMobileDeviceSearchChoicesV1: %v", err)
	}
	t.Logf("Advanced search criterion returned %d choices", len(choices.Choices))
}

func TestAcceptance_Pro_Mobile_AdvancedSearchCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-adv-mobile-search-" + runSuffix()
	displayFields := []string{"Display Name"}

	created, err := p.CreateAdvancedMobileDeviceSearchV1(ctx, &pro.AdvancedSearch{
		Name:          name,
		DisplayFields: &displayFields,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAdvancedMobileDeviceSearchV1: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateAdvancedMobileDeviceSearchV1 returned no ID (href=%q)", created.Href)
	}
	cleanupDelete(t, "DeleteAdvancedMobileDeviceSearchV1", func() error { return p.DeleteAdvancedMobileDeviceSearchV1(ctx, created.ID) })
	t.Logf("Created advanced mobile search %s", created.ID)

	got, err := p.GetAdvancedMobileDeviceSearchV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAdvancedMobileDeviceSearchV1(%s): %v", created.ID, err)
	}
	if got.Name != name {
		t.Errorf("Name = %q, want %q", got.Name, name)
	}

	renamed := name + "-updated"
	got.Name = renamed
	updated, err := p.UpdateAdvancedMobileDeviceSearchV1(ctx, created.ID, got)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateAdvancedMobileDeviceSearchV1(%s): %v", created.ID, err)
	}
	if updated.Name != renamed {
		t.Errorf("UpdateAdvancedMobileDeviceSearchV1 Name = %q, want %q", updated.Name, renamed)
	}

	if err := p.DeleteAdvancedMobileDeviceSearchV1(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteAdvancedMobileDeviceSearchV1(%s): %v", created.ID, err)
	}

	_, err = p.GetAdvancedMobileDeviceSearchV1(ctx, created.ID)
	if err == nil {
		t.Fatalf("GetAdvancedMobileDeviceSearchV1(%s) after delete should 404", created.ID)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetAdvancedMobileDeviceSearchV1(%s) after delete: want 404, got %v", created.ID, err)
	}
}

func TestAcceptance_Pro_Mobile_DeleteMultipleAdvancedSearchesV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	suffix := runSuffix()
	var ids []string
	for _, tag := range []string{"a", "b"} {
		resp, err := p.CreateAdvancedMobileDeviceSearchV1(ctx, &pro.AdvancedSearch{
			Name: "sdk-acc-adv-mobile-bulk-" + suffix + "-" + tag,
		})
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("CreateAdvancedMobileDeviceSearchV1[%s]: %v", tag, err)
		}
		ids = append(ids, resp.ID)
		id := resp.ID
		cleanupDelete(t, "DeleteAdvancedMobileDeviceSearchV1(fallback)", func() error { return p.DeleteAdvancedMobileDeviceSearchV1(ctx, id) })
	}

	if err := p.DeleteMultipleAdvancedMobileDeviceSearchesV1(ctx, &pro.Ids{IDs: &ids}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteMultipleAdvancedMobileDeviceSearchesV1: %v", err)
	}

	for _, id := range ids {
		if _, err := p.GetAdvancedMobileDeviceSearchV1(ctx, id); err == nil {
			t.Errorf("GetAdvancedMobileDeviceSearchV1(%s) after bulk delete should 404", id)
		}
	}
}

// --- mobile device extension attributes --------------------------------

func TestAcceptance_Pro_Mobile_ListMDEAV1(t *testing.T) {
	c := accClient(t)

	items, err := pro.New(c).ListMobileDeviceExtensionAttributesV1(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDeviceExtensionAttributesV1: %v", err)
	}
	t.Logf("Found %d mobile device extension attributes", len(items))
}

func TestAcceptance_Pro_Mobile_MDEACRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-mdea-" + runSuffix()

	// Server-enforced enum: TEXT, POPUP, DIRECTORY_SERVICE_ATTRIBUTE_MAPPING
	// (the spec's list of inputType values differs from the server — flagged
	// to the API team). TEXT is simplest for round-tripping.
	created, err := p.CreateMobileDeviceExtensionAttributeV1(ctx, &pro.MobileDeviceExtensionAttributes{
		Name:                 name,
		Description:          ptr("SDK acceptance test fixture"),
		InputType:            "TEXT",
		InventoryDisplayType: "GENERAL",
		DataType:             "STRING",
		PopupMenuChoices:     &[]string{},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceExtensionAttributeV1: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateMobileDeviceExtensionAttributeV1 returned no ID")
	}
	cleanupDelete(t, "DeleteMobileDeviceExtensionAttributeV1", func() error { return p.DeleteMobileDeviceExtensionAttributeV1(ctx, created.ID) })

	got, err := p.GetMobileDeviceExtensionAttributeV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceExtensionAttributeV1(%s): %v", created.ID, err)
	}
	if got.Name != name {
		t.Errorf("Name = %q, want %q", got.Name, name)
	}

	got.Description = ptr("updated")
	if _, err := p.UpdateMobileDeviceExtensionAttributeV1(ctx, created.ID, got); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateMobileDeviceExtensionAttributeV1(%s): %v", created.ID, err)
	}

	deps, err := p.GetMobileDeviceExtensionAttributeDataDependencyV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceExtensionAttributeDataDependencyV1(%s): %v", created.ID, err)
	}
	t.Logf("MDEA %s has %d data-dependency entries", created.ID, len(deps.Results))

	note, err := p.CreateMobileDeviceExtensionAttributeHistoryNoteV1(ctx, created.ID, &pro.ObjectHistoryNote{
		Note: "sdk-acc test history entry",
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceExtensionAttributeHistoryNoteV1(%s): %v", created.ID, err)
	}
	if note.Note == "" {
		t.Errorf("CreateMobileDeviceExtensionAttributeHistoryNoteV1 returned empty note body")
	}

	hist, err := p.ListMobileDeviceExtensionAttributeHistoryV1(ctx, created.ID, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDeviceExtensionAttributeHistoryV1(%s): %v", created.ID, err)
	}
	t.Logf("MDEA %s history has %d entries", created.ID, len(hist))

	if err := p.DeleteMobileDeviceExtensionAttributeV1(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteMobileDeviceExtensionAttributeV1(%s): %v", created.ID, err)
	}

	_, err = p.GetMobileDeviceExtensionAttributeV1(ctx, created.ID)
	if err == nil {
		t.Fatalf("GetMobileDeviceExtensionAttributeV1(%s) after delete should 404", created.ID)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetMobileDeviceExtensionAttributeV1(%s) after delete: want 404, got %v", created.ID, err)
	}
}

// --- mobile device groups ----------------------------------------------

func TestAcceptance_Pro_Mobile_ListDeviceGroupsV1(t *testing.T) {
	c := accClient(t)

	groups, err := pro.New(c).ListMobileDeviceGroupsV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDeviceGroupsV1: %v", err)
	}
	t.Logf("Found %d mobile device groups (legacy list)", len(groups))
}

func TestAcceptance_Pro_Mobile_ListSmartGroupsV1(t *testing.T) {
	c := accClient(t)

	groups, err := pro.New(c).ListSmartMobileDeviceGroupsV1(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListSmartMobileDeviceGroupsV1: %v", err)
	}
	t.Logf("Found %d smart mobile device groups", len(groups))
}

func TestAcceptance_Pro_Mobile_SmartGroupCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-smart-mdg-" + runSuffix()
	desc := "SDK acceptance test fixture"

	// Explicit siteId "-1" (Default site) — server returns 403 "siteId: Access
	// denied" when the field is omitted for clients without all-sites
	// privilege. Same applies to static groups below.
	siteID := "-1"
	created, err := p.CreateSmartMobileDeviceGroupV1(ctx, &pro.SmartGroupAssignment{
		GroupName:        name,
		GroupDescription: &desc,
		SiteID:           &siteID,
	}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSmartMobileDeviceGroupV1: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateSmartMobileDeviceGroupV1 returned no ID")
	}
	cleanupDelete(t, "DeleteSmartMobileDeviceGroupV1", func() error { return p.DeleteSmartMobileDeviceGroupV1(ctx, created.ID) })

	got, err := p.GetSmartMobileDeviceGroupV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSmartMobileDeviceGroupV1(%s): %v", created.ID, err)
	}
	t.Logf("Created smart mobile group %s (name=%q)", created.ID, got.GroupName)

	update := &pro.SmartGroupAssignment{
		GroupName:        got.GroupName,
		GroupDescription: ptr(desc + " (updated)"),
		SiteID:           &siteID,
	}
	if _, err := p.UpdateSmartMobileDeviceGroupV1(ctx, created.ID, update); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateSmartMobileDeviceGroupV1(%s): %v", created.ID, err)
	}

	members, err := p.ListSmartMobileDeviceGroupMembershipV1(ctx, created.ID, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListSmartMobileDeviceGroupMembershipV1(%s): %v", created.ID, err)
	}
	t.Logf("Smart mobile group %s has %d members", created.ID, len(members))

	if err := p.DeleteSmartMobileDeviceGroupV1(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteSmartMobileDeviceGroupV1(%s): %v", created.ID, err)
	}
}

func TestAcceptance_Pro_Mobile_StaticGroupCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-static-mdg-" + runSuffix()
	desc := "SDK acceptance test fixture"

	// Empty assignments array — same NPE guard as computer-groups batch 2.
	// Explicit siteId — same 403-avoidance as smart groups above.
	emptyAssignments := []pro.Assignment{}
	siteID := "-1"
	created, err := p.CreateStaticMobileDeviceGroupV1(ctx, &pro.StaticGroupAssignment{
		GroupName:        name,
		GroupDescription: &desc,
		Assignments:      &emptyAssignments,
		SiteID:           &siteID,
	}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateStaticMobileDeviceGroupV1: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateStaticMobileDeviceGroupV1 returned no ID")
	}
	cleanupDelete(t, "DeleteStaticMobileDeviceGroupV1", func() error { return p.DeleteStaticMobileDeviceGroupV1(ctx, created.ID) })
	t.Logf("Created static mobile group %s", created.ID)

	got, err := p.GetStaticMobileDeviceGroupV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetStaticMobileDeviceGroupV1(%s): %v", created.ID, err)
	}
	if got.GroupName != name {
		t.Errorf("GroupName = %q, want %q", got.GroupName, name)
	}

	// PATCH — partial update. Same non-null assignments guard.
	patch := &pro.StaticGroupAssignment{
		GroupName:        name,
		GroupDescription: ptr(desc + " (patched)"),
		Assignments:      &emptyAssignments,
		SiteID:           &siteID,
	}
	if _, err := p.PatchStaticMobileDeviceGroupV1(ctx, created.ID, patch); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("PatchStaticMobileDeviceGroupV1(%s): %v", created.ID, err)
	}

	members, err := p.ListStaticMobileDeviceGroupMembershipV1(ctx, created.ID, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListStaticMobileDeviceGroupMembershipV1(%s): %v", created.ID, err)
	}
	t.Logf("Static mobile group %s has %d members", created.ID, len(members))

	if err := p.DeleteStaticMobileDeviceGroupV1(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteStaticMobileDeviceGroupV1(%s): %v", created.ID, err)
	}
}

// TestAcceptance_Pro_Mobile_EraseMobileDeviceGroupV1 is a destructive
// bulk-wipe against every device in a group. Never run for real — probe
// with a bogus group id and verify the transport rejects rather than
// targeting real tenant devices.
func TestAcceptance_Pro_Mobile_EraseMobileDeviceGroupV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	err := pro.New(c).EraseMobileDeviceGroupV1(ctx, "99999999", &pro.GroupResetRequest{})
	if err == nil {
		t.Fatal("EraseMobileDeviceGroupV1 against bogus id succeeded — expected 4xx")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("EraseMobileDeviceGroupV1(bogus) rejected as expected: status=%d", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Logf("EraseMobileDeviceGroupV1(bogus) rejected: %v", err)
}

// --- mobile devices v2 -------------------------------------------------

func TestAcceptance_Pro_Mobile_ListMobileDevicesV2(t *testing.T) {
	c := accClient(t)

	devices, err := pro.New(c).ListMobileDevicesV2(context.Background(), nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDevicesV2: %v", err)
	}
	t.Logf("Found %d mobile devices (v2)", len(devices))
}

// TestAcceptance_Pro_Mobile_MobileDeviceReadChain exercises Get + GetDetail +
// ListPairedDevices against the first device the list returns. Skips if the
// tenant has no mobile devices.
func TestAcceptance_Pro_Mobile_MobileDeviceReadChain(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	devices, err := p.ListMobileDevicesV2(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDevicesV2: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("tenant has no mobile devices — nothing to read")
	}
	id := devices[0].ID

	dev, err := p.GetMobileDeviceV2(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceV2(%s): %v", id, err)
	}
	t.Logf("Device %s: name=%q model=%q", id, dev.Name, dev.Model)

	detail, err := p.GetMobileDeviceDetailV2(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceDetailV2(%s): %v", id, err)
	}
	if detail == nil {
		t.Error("GetMobileDeviceDetailV2 returned nil detail")
	}

	paired, err := p.ListMobileDevicePairedDevicesV2(ctx, id, nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		// paired-devices is iOS-specific; tvOS/watchOS devices may 404 or 400.
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("ListMobileDevicePairedDevicesV2(%s): %d — non-iOS or unsupported, plumbing OK", id, apiErr.StatusCode)
			return
		}
		t.Fatalf("ListMobileDevicePairedDevicesV2(%s): %v", id, err)
	}
	t.Logf("Device %s paired with %d devices", id, len(paired))
}

// TestAcceptance_Pro_Mobile_PatchMobileDeviceV2 is destructive against real
// tenant inventory if misapplied. Probe with a bogus id — verifies the
// transport + encoding rather than mutating a device.
func TestAcceptance_Pro_Mobile_PatchMobileDeviceV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	_, err := pro.New(c).PatchMobileDeviceV2(ctx, "99999999", &pro.UpdateMobileDeviceV2{
		Name: ptr("sdk-acc-should-not-apply"),
	})
	if err == nil {
		t.Fatal("PatchMobileDeviceV2 against bogus id succeeded — expected 4xx")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("PatchMobileDeviceV2(bogus) rejected as expected: status=%d", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Logf("PatchMobileDeviceV2(bogus) rejected: %v", err)
}

// TestAcceptance_Pro_Mobile_EraseMobileDeviceV2 is destructive — wipes the
// target device. Probe with bogus id only.
func TestAcceptance_Pro_Mobile_EraseMobileDeviceV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	_, err := pro.New(c).EraseMobileDeviceV2(ctx, "99999999", &pro.EraseDeviceMobileDeviceRequest{})
	if err == nil {
		t.Fatal("EraseMobileDeviceV2 against bogus id succeeded — expected 4xx")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("EraseMobileDeviceV2(bogus) rejected as expected: status=%d", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Logf("EraseMobileDeviceV2(bogus) rejected: %v", err)
}

// TestAcceptance_Pro_Mobile_UnmanageMobileDeviceV2 is permanently skipped
// because the endpoint is destructive (removes MDM management from the
// target device, requiring full re-enrollment) and cannot be exercised
// safely in an automated test. Bogus-id probing does not work either —
// the server returns 500 for unknown ids instead of 404, so a transport-
// only probe fails to distinguish "we shaped the request correctly" from
// "the server choked".
//
// The endpoint has been manually verified against a real managed +
// supervised device (SHARED-DMPZ9DMYMF3M, id=12) on the nmartin tenant
// during batch 3 development: POST returned 200 with
// {"deviceId":"12","commandUuid":"9f4a4ea3-83c1-4c35-bb94-5c768766ba0d"}
// confirming the transport + response shape work. No further automated
// coverage of this path.
func TestAcceptance_Pro_Mobile_UnmanageMobileDeviceV2(t *testing.T) {
	t.Skip("destructive (removes MDM management) and tenant 500s on bogus-id probes — manually verified against a real managed+supervised device; see function comment")
}
