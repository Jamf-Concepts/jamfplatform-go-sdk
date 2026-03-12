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
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        name,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		Members:     []string{},
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
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        "sdk-acc-update-original-" + suffix,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		Members:     []string{},
	})
	if err != nil {
		t.Fatalf("CreateDeviceGroup failed: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteDeviceGroup(ctx, resp.ID) })

	renamedName := "sdk-acc-update-renamed-" + suffix
	updatedDesc := "Updated description"
	err = c.UpdateDeviceGroup(ctx, resp.ID, &DeviceGroupUpdateRepresentationV1{
		Name:        renamedName,
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
	resp, err := c.CreateDeviceGroup(ctx, &DeviceGroupCreateRepresentationV1{
		Name:        name,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "SMART",
		Criteria: []DeviceGroupCriteriaRepresentationV1{
			{
				Order:          0,
				AttributeName:  "Serial Number",
				Operator:       "LIKE",
				AttributeValue: "",
				JoinType:       "AND",
			},
		},
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
	if len(group.Criteria) != 1 {
		t.Errorf("expected 1 criterion, got %d", len(group.Criteria))
	}
	t.Logf("Created smart group ID: %s, members: %d", resp.ID, group.MemberCount)
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
