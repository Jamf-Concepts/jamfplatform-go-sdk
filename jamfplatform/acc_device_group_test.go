// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"context"
	"testing"
)

func TestAcceptance_ListDeviceGroups(t *testing.T) {
	c := accClient(t)

	groups, err := c.ListDeviceGroups(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("ListDeviceGroups failed: %v", err)
	}
	t.Logf("Found %d device groups", len(groups))
}

func TestAcceptance_ListDeviceGroupsWithSort(t *testing.T) {
	c := accClient(t)

	groups, err := c.ListDeviceGroups(context.Background(), []string{"name:asc"}, "")
	if err != nil {
		t.Fatalf("ListDeviceGroups with sort failed: %v", err)
	}
	t.Logf("Found %d device groups (sorted)", len(groups))
}

func TestAcceptance_DeviceGroup_SmartGroupFixture(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)

	group, err := c.GetDeviceGroup(context.Background(), groupID)
	if err != nil {
		t.Fatalf("GetDeviceGroup failed: %v", err)
	}
	if group.GroupType != "SMART" {
		t.Errorf("expected SMART, got %q", group.GroupType)
	}
	if group.DeviceType != "COMPUTER" {
		t.Errorf("expected COMPUTER, got %q", group.DeviceType)
	}
	t.Logf("Fixture smart group ID: %s, members: %d", groupID, group.MemberCount)
}

func TestAcceptance_DeviceGroup_CreateAndDeleteStaticGroup(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	suffix := runSuffix()

	name := "sdk-acc-static-group-" + suffix
	desc := "SDK acceptance test — safe to delete"
	emptyMembers := []string{}
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        name,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		Members:     &emptyMembers,
	})
	if err != nil {
		t.Fatalf("CreateDeviceGroup failed: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteDeviceGroup(ctx, resp.ID) })

	group, err := c.GetDeviceGroup(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetDeviceGroup failed: %v", err)
	}
	if group.Name != name {
		t.Errorf("expected name %q, got %q", name, group.Name)
	}
	if group.GroupType != "STATIC" {
		t.Errorf("expected STATIC, got %q", group.GroupType)
	}
	t.Logf("Created static group ID: %s", resp.ID)
}

func TestAcceptance_DeviceGroup_UpdateGroup(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	suffix := runSuffix()

	desc := "SDK acceptance test — safe to delete"
	emptyMembers := []string{}
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        "sdk-acc-update-original-" + suffix,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		Members:     &emptyMembers,
	})
	if err != nil {
		t.Fatalf("CreateDeviceGroup failed: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteDeviceGroup(ctx, resp.ID) })

	renamedName := "sdk-acc-update-renamed-" + suffix
	updatedDesc := "Updated description"
	err = c.UpdateDeviceGroup(ctx, resp.ID, &DeviceGroupUpdateRepresentationV1{
		Name:        &renamedName,
		Description: &updatedDesc,
	})
	if err != nil {
		t.Fatalf("UpdateDeviceGroup failed: %v", err)
	}

	group, err := c.GetDeviceGroup(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetDeviceGroup after update failed: %v", err)
	}
	if group.Name != renamedName {
		t.Errorf("expected name %q, got %q", renamedName, group.Name)
	}
	t.Logf("Updated device group ID: %s", resp.ID)
}

func TestAcceptance_DeviceGroup_SmartGroupWithCriteria(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	suffix := runSuffix()

	name := "sdk-acc-smart-criteria-" + suffix
	desc := "SDK acceptance test smart group — safe to delete"
	criteria := []DeviceGroupCriteriaRepresentationV1{
		{
			Order:          0,
			AttributeName:  "Serial Number",
			Operator:       "LIKE",
			AttributeValue: "",
			JoinType:       "AND",
		},
	}
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        name,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "SMART",
		Criteria:    &criteria,
	})
	if err != nil {
		t.Fatalf("CreateDeviceGroup failed: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteDeviceGroup(ctx, resp.ID) })

	group, err := c.GetDeviceGroup(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetDeviceGroup failed: %v", err)
	}
	if group.GroupType != "SMART" {
		t.Errorf("expected SMART, got %q", group.GroupType)
	}
	if group.Criteria == nil || len(*group.Criteria) != 1 {
		t.Errorf("expected 1 criterion, got %v", group.Criteria)
	}
	t.Logf("Created smart group ID: %s, members: %d", resp.ID, group.MemberCount)
}

func TestAcceptance_DeviceGroup_PartialUpdatePreservesCriteria(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	suffix := runSuffix()

	name := "sdk-acc-partial-criteria-" + suffix
	desc := "SDK acceptance test — safe to delete"
	criteria := []DeviceGroupCriteriaRepresentationV1{
		{
			Order:          0,
			AttributeName:  "Serial Number",
			Operator:       "LIKE",
			AttributeValue: "",
			JoinType:       "AND",
		},
	}
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        name,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "SMART",
		Criteria:    &criteria,
	})
	if err != nil {
		t.Fatalf("CreateDeviceGroup failed: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteDeviceGroup(ctx, resp.ID) })

	group, err := c.GetDeviceGroup(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetDeviceGroup failed: %v", err)
	}
	if group.Criteria == nil || len(*group.Criteria) != 1 {
		t.Fatalf("expected 1 criterion after creation, got %v", group.Criteria)
	}

	// Update only the name — omit Criteria entirely.
	// Before the fix, this would serialize "criteria":[] and wipe them.
	renamedName := "sdk-acc-partial-criteria-renamed-" + suffix
	err = c.UpdateDeviceGroup(ctx, resp.ID, &DeviceGroupUpdateRepresentationV1{
		Name: &renamedName,
	})
	if err != nil {
		t.Fatalf("UpdateDeviceGroup (partial) failed: %v", err)
	}

	updated, err := c.GetDeviceGroup(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetDeviceGroup after partial update failed: %v", err)
	}
	if updated.Name != renamedName {
		t.Errorf("expected name %q, got %q", renamedName, updated.Name)
	}
	if updated.Criteria == nil || len(*updated.Criteria) != 1 {
		t.Errorf("criteria were lost: expected 1 criterion, got %v", updated.Criteria)
	}
	t.Logf("Partial update preserved criteria on device group %s", resp.ID)
}

func TestAcceptance_DeviceGroup_ListMembers(t *testing.T) {
	groupID := requireSmartGroupFixture(t)
	c := accClient(t)

	members, err := c.ListDeviceGroupMembers(context.Background(), groupID)
	if err != nil {
		t.Fatalf("ListDeviceGroupMembers failed: %v", err)
	}
	t.Logf("Fixture group has %d members", len(members))
}

func TestAcceptance_DeviceGroup_UpdateMembers(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available — cannot test member updates")
	}
	deviceID := devices[0].ID

	suffix := runSuffix()
	desc := "SDK acceptance test — safe to delete"
	emptyMembers := []string{}
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        "sdk-acc-members-" + suffix,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		Members:     &emptyMembers,
	})
	if err != nil {
		t.Fatalf("CreateDeviceGroup failed: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteDeviceGroup(ctx, resp.ID) })

	// Add a device
	addIDs := []string{deviceID}
	err = c.UpdateDeviceGroupMembers(ctx, resp.ID, &DeviceGroupMemberPatchRepresentationV1{
		Added: &addIDs,
	})
	if err != nil {
		t.Fatalf("UpdateDeviceGroupMembers (add) failed: %v", err)
	}

	members, err := c.ListDeviceGroupMembers(ctx, resp.ID)
	if err != nil {
		t.Fatalf("ListDeviceGroupMembers failed: %v", err)
	}
	if len(members) != 1 || members[0] != deviceID {
		t.Errorf("expected [%s], got %v", deviceID, members)
	}

	// Remove the device
	removeIDs := []string{deviceID}
	err = c.UpdateDeviceGroupMembers(ctx, resp.ID, &DeviceGroupMemberPatchRepresentationV1{
		Removed: &removeIDs,
	})
	if err != nil {
		t.Fatalf("UpdateDeviceGroupMembers (remove) failed: %v", err)
	}

	members, err = c.ListDeviceGroupMembers(ctx, resp.ID)
	if err != nil {
		t.Fatalf("ListDeviceGroupMembers after remove failed: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected empty members, got %v", members)
	}
	t.Logf("Added and removed device %s from group %s", deviceID, resp.ID)
}

func TestAcceptance_DeviceGroup_ListGroupsForDevice(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available")
	}

	groups, err := c.ListDeviceGroupsForDevice(ctx, devices[0].ID)
	if err != nil {
		t.Fatalf("ListDeviceGroupsForDevice failed: %v", err)
	}
	t.Logf("Device %s belongs to %d groups", devices[0].ID, len(groups))
	for _, g := range groups {
		t.Logf("  %s (%s)", g.GroupName, g.GroupID)
	}
}
