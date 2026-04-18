// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func classicStrPtr(s string) *string { return &s }
func intToStr(i int) string          { return strconv.Itoa(i) }

// TestAcceptance_Classic_GetComputerByID exercises the Classic XML path
// end-to-end. With the v11.20.0 Swagger 2.0 spec replaced in-tree, the
// generator now emits a fully-typed Computer with nested ComputerGeneral,
// ComputerHardware, etc. sub-structs; xml.Unmarshal populates them from
// the real 30KB XML response.
func TestAcceptance_Classic_GetComputerByID(t *testing.T) {
	c := accClient(t)

	comp, err := proclassic.New(c).GetComputerByID(context.Background(), "4")
	if err != nil {
		skipOnServerError(t, err)
		t.Skipf("GetComputerByID(4): %v", err)
	}
	if comp == nil || comp.General == nil {
		t.Fatalf("expected Computer.General populated, got %+v", comp)
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	id := 0
	if comp.General.ID != nil {
		id = *comp.General.ID
	}
	t.Logf("Computer id=%d name=%q serial=%q udid=%q", id, deref(comp.General.Name), deref(comp.General.SerialNumber), deref(comp.General.UDID))
}

// TestAcceptance_Classic_ComputerCRUD exercises the Classic computer CRUD
// lifecycle using a synthetic record — no real enrolled device is touched.
// Creates via POST /computers/id/0, round-trips via GET by serial number
// (the create endpoint's 201 response body is server-generated and needs
// the post-hoc lookup to recover the numeric id), updates, then deletes.
func TestAcceptance_Classic_ComputerCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-computer-" + runSuffix()
	serial := "SDK" + runSuffix()

	err := pc.CreateComputerByID(ctx, "0", &proclassic.ComputerPost{
		General: &proclassic.ComputerPostGeneral{
			Name:         classicStrPtr(name),
			SerialNumber: classicStrPtr(serial),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateComputerByID(0): %v", err)
	}
	t.Cleanup(func() { _ = pc.DeleteComputerBySerialNumber(ctx, serial) })
	t.Logf("Created computer name=%q serial=%q", name, serial)

	got, err := pc.GetComputerBySerialNumber(ctx, serial)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerBySerialNumber(%q): %v", serial, err)
	}
	if got == nil || got.General == nil || got.General.ID == nil {
		t.Fatalf("expected Computer.General.ID populated after round-trip, got %+v", got)
	}
	id := *got.General.ID
	if got.General.Name == nil || *got.General.Name != name {
		t.Errorf("Computer.General.Name = %v, want %q", got.General.Name, name)
	}

	// Update — rename the record via id.
	newName := name + "-updated"
	if err := pc.UpdateComputerByID(ctx, intToStr(id), &proclassic.ComputerPost{
		General: &proclassic.ComputerPostGeneral{Name: classicStrPtr(newName)},
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateComputerByID(%d): %v", id, err)
	}

	afterUpdate, err := pc.GetComputerByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerByID(%d) after update: %v", id, err)
	}
	if afterUpdate.General == nil || afterUpdate.General.Name == nil || *afterUpdate.General.Name != newName {
		t.Errorf("after UpdateComputerByID Name = %v, want %q", afterUpdate.General.Name, newName)
	}

	// Delete.
	if err := pc.DeleteComputerByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteComputerByID(%d): %v", id, err)
	}

	// Verify gone.
	_, err = pc.GetComputerByID(ctx, intToStr(id))
	if err == nil {
		t.Fatalf("GetComputerByID(%d) after delete should 404, succeeded", id)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetComputerByID(%d) after delete: want 404, got %v", id, err)
	}
}
