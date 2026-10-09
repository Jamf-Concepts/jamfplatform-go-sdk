// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// The Jamf Pro static-group writes need fields their spec declares optional.
// StaticComputerGroupAssignment and StaticGroupAssignment mark only the name
// required, and the SDK follows the spec, so these fields stay omitempty
// pointers: marking them required locally would not help, because a required
// slice left unset marshals as null and a required string as "", and the server
// refuses both exactly as it refuses absence (wire-verified 2026-10-04). The
// cost is recorded in each method's godoc instead.
//
// Each refusal is asserted rather than skipped. The day one of these writes is
// accepted, the test fails, and that is the notification to delete the
// matching methodNotes entry and the standing-disagreements row.

// wantStaticGroupRefusal asserts err is the refusal recorded for a body missing
// an undeclared-required field. An empty code means the server's bare
// `500, errors: []`, which names nothing.
func wantStaticGroupRefusal(t *testing.T, label string, err error, status int, code, field string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: accepted. The server no longer needs the field the spec calls optional: "+
			"delete its methodNotes entry and the CLAUDE.md standing-disagreements row", label)
		return
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("%s: non-API error, the request did not reach Jamf Pro: %v", label, err)
	}
	if !apiErr.HasStatus(status) {
		t.Errorf("%s: status %d, want %d: %v", label, apiErr.StatusCode, status, err)
		return
	}
	details := apiErr.Details()
	if code == "" {
		if len(details) != 0 {
			t.Errorf("%s: %d with errors %v, want the bare `errors: []` — the server now says why; update the godoc", label, status, details)
		}
		return
	}
	for _, d := range details {
		if d.Code == code && d.Field == field {
			return
		}
	}
	t.Errorf("%s: want %d %s on field %q, got %v", label, status, code, field, details)
}

func TestAcceptance_Pro_StaticComputerGroupV3RequiresAssignments(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)
	name := "sdk-acc-static-cg-req-" + runSuffix()

	createOrRefuse := func(label string, req *pro.StaticComputerGroupAssignment) {
		t.Helper()
		created, err := p.CreateStaticComputerGroupV3(ctx, req, false)
		if err == nil {
			cleanupDelete(t, "DeleteStaticComputerGroupV3", func() error { return p.DeleteStaticComputerGroupV3(ctx, created.ID) })
		}
		wantStaticGroupRefusal(t, label, err, 500, "", "")
	}
	siteID := "-1"
	createOrRefuse("create without assignments", &pro.StaticComputerGroupAssignment{Name: name + "-a"})
	createOrRefuse("create with siteId, without assignments", &pro.StaticComputerGroupAssignment{Name: name + "-b", SiteID: &siteID})

	// The control: the same body with an empty list is accepted.
	empty := []string{}
	created, err := p.CreateStaticComputerGroupV3(ctx, &pro.StaticComputerGroupAssignment{Name: name, Assignments: &empty}, false)
	if err != nil {
		t.Fatalf("CreateStaticComputerGroupV3 with assignments []: %v", err)
	}
	cleanupDelete(t, "DeleteStaticComputerGroupV3", func() error { return p.DeleteStaticComputerGroupV3(ctx, created.ID) })

	_, err = p.UpdateStaticComputerGroupV3(ctx, created.ID, &pro.StaticComputerGroupAssignment{Name: name})
	wantStaticGroupRefusal(t, "update without assignments", err, 500, "", "")

	// The PUT replaces the member list, and an empty list empties the group.
	// GetStaticComputerGroupV3 returns no members and Jamf Pro has no static
	// computer membership endpoint, so Classic is the only read-back.
	computers, err := p.ListComputersInventoryV3(ctx, []string{"GENERAL"}, nil, "")
	if err != nil {
		t.Fatalf("ListComputersInventoryV3: %v", err)
	}
	if len(computers) < 2 {
		t.Skipf("replace semantics need two computers, tenant has %d", len(computers))
	}
	a, b := computers[0].ID, computers[1].ID
	pc := proclassic.New(c)
	members := func() []string {
		t.Helper()
		g, err := pc.GetComputerGroupByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetComputerGroupByID(%s): %v", created.ID, err)
		}
		if g.Name == nil || *g.Name != name {
			t.Fatalf("GetComputerGroupByID(%s) decoded name %v, want %q: wrong root element?", created.ID, g.Name, name)
		}
		var ids []string
		if g.Computers != nil && g.Computers.Computer != nil {
			for _, m := range *g.Computers.Computer {
				if m.ID != nil {
					ids = append(ids, strconv.Itoa(*m.ID))
				}
			}
		}
		slices.Sort(ids)
		return ids
	}
	for _, step := range []struct {
		label string
		put   []string
	}{
		{"assign two", []string{a, b}},
		{"assign one replaces both", []string{b}},
		{"assign none empties the group", []string{}},
	} {
		if _, err := p.UpdateStaticComputerGroupV3(ctx, created.ID, &pro.StaticComputerGroupAssignment{Name: name, Assignments: &step.put}); err != nil {
			t.Fatalf("%s: UpdateStaticComputerGroupV3: %v", step.label, err)
		}
		want := slices.Sorted(slices.Values(step.put))
		if got := members(); !slices.Equal(got, want) {
			t.Errorf("%s: members %v, want %v — the PUT no longer replaces; update its godoc", step.label, got, want)
		}
	}
}

func TestAcceptance_Pro_StaticMobileDeviceGroupV2RequiresAssignmentsAndSiteID(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)
	name := "sdk-acc-static-mdg-req-" + runSuffix()
	siteID := "-1"
	empty := []pro.Assignment{}

	createOrRefuse := func(label string, req *pro.StaticGroupAssignment, status int, code, field string) {
		t.Helper()
		created, err := p.CreateStaticMobileDeviceGroupV2(ctx, req, false)
		if err == nil {
			cleanupDelete(t, "DeleteStaticMobileDeviceGroupV2", func() error { return p.DeleteStaticMobileDeviceGroupV2(ctx, created.ID) })
		}
		wantStaticGroupRefusal(t, label, err, status, code, field)
	}
	createOrRefuse("create without assignments or siteId", &pro.StaticGroupAssignment{GroupName: name + "-a"}, 500, "", "")
	createOrRefuse("create without assignments", &pro.StaticGroupAssignment{GroupName: name + "-b", SiteID: &siteID}, 500, "", "")
	createOrRefuse("create without siteId", &pro.StaticGroupAssignment{GroupName: name + "-c", Assignments: &empty}, 403, "INVALID_PRIVILEGE", "siteId")

	created, err := p.CreateStaticMobileDeviceGroupV2(ctx, &pro.StaticGroupAssignment{GroupName: name, SiteID: &siteID, Assignments: &empty}, false)
	if err != nil {
		t.Fatalf("CreateStaticMobileDeviceGroupV2 with siteId and assignments []: %v", err)
	}
	cleanupDelete(t, "DeleteStaticMobileDeviceGroupV2", func() error { return p.DeleteStaticMobileDeviceGroupV2(ctx, created.ID) })

	_, err = p.PatchStaticMobileDeviceGroupV2(ctx, created.ID, &pro.StaticGroupAssignment{GroupName: name, SiteID: &siteID})
	wantStaticGroupRefusal(t, "patch without assignments", err, 500, "", "")
	_, err = p.PatchStaticMobileDeviceGroupV2(ctx, created.ID, &pro.StaticGroupAssignment{GroupName: name, Assignments: &empty})
	wantStaticGroupRefusal(t, "patch without siteId", err, 400, "INVALID_FIELD", "")

	// Unlike the computer PUT, the PATCH is incremental: an entry adds or
	// removes one device, and an empty list changes nothing.
	devices, err := p.ListMobileDevicesV2(ctx, nil)
	if err != nil {
		t.Fatalf("ListMobileDevicesV2: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("incremental semantics need a mobile device, tenant has none")
	}
	dev := devices[0].ID
	selected, deselected := true, false
	members := func() []string {
		t.Helper()
		got, err := p.ListStaticMobileDeviceGroupMembershipV2(ctx, created.ID, nil, "")
		if err != nil {
			t.Fatalf("ListStaticMobileDeviceGroupMembershipV2(%s): %v", created.ID, err)
		}
		var ids []string
		for _, m := range got {
			ids = append(ids, m.MobileDeviceID)
		}
		return ids
	}
	for _, step := range []struct {
		label       string
		assignments []pro.Assignment
		want        []string
	}{
		{"add one", []pro.Assignment{{MobileDeviceID: &dev, Selected: &selected}}, []string{dev}},
		{"empty list keeps members", []pro.Assignment{}, []string{dev}},
		{"selected false removes", []pro.Assignment{{MobileDeviceID: &dev, Selected: &deselected}}, nil},
	} {
		if _, err := p.PatchStaticMobileDeviceGroupV2(ctx, created.ID, &pro.StaticGroupAssignment{GroupName: name, SiteID: &siteID, Assignments: &step.assignments}); err != nil {
			t.Fatalf("%s: PatchStaticMobileDeviceGroupV2: %v", step.label, err)
		}
		if got := members(); !slices.Equal(got, step.want) {
			t.Errorf("%s: members %v, want %v — the PATCH is no longer incremental; update its godoc", step.label, got, step.want)
		}
	}
}
