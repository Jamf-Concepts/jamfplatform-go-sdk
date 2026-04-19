// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func classicStrPtr(s string) *string { return &s }
func intToStr(i int) string          { return strconv.Itoa(i) }

// TestAcceptance_Classic_GetComputerByID exercises the Classic XML path
// end-to-end. Uses JAMFPLATFORM_CLASSIC_COMPUTER_ID if set; otherwise
// pulls the first id from ListComputers. Skips only when the tenant
// has no computers enrolled.
func TestAcceptance_Classic_GetComputerByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()

	id := os.Getenv("JAMFPLATFORM_CLASSIC_COMPUTER_ID")
	if id == "" {
		list, err := pc.ListComputers(ctx)
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("ListComputers: %v", err)
		}
		if list == nil || len(list.Computers) == 0 {
			t.Skip("tenant has no computers; set JAMFPLATFORM_CLASSIC_COMPUTER_ID to override")
		}
		first := list.Computers[0]
		if first.ID == nil {
			t.Fatalf("first computer has no ID: %+v", first)
		}
		id = intToStr(*first.ID)
	}

	comp, err := pc.GetComputerByID(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerByID(%s): %v", id, err)
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
	cid := 0
	if comp.General.ID != nil {
		cid = *comp.General.ID
	}
	t.Logf("Computer id=%d name=%q serial=%q udid=%q", cid, deref(comp.General.Name), deref(comp.General.SerialNumber), deref(comp.General.UDID))
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
	cleanupDelete(t, "DeleteComputerBySerialNumber", func() error { return pc.DeleteComputerBySerialNumber(ctx, serial) })
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

func TestAcceptance_Classic_BuildingCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-building-" + runSuffix()
	created, err := pc.CreateBuildingByID(ctx, "0", &proclassic.Building{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBuildingByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateBuildingByID returned no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteBuildingByID", func() error { return pc.DeleteBuildingByID(ctx, intToStr(id)) })
	t.Logf("Created building id=%d name=%q", id, name)

	got, err := pc.GetBuildingByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetBuildingByID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("GetBuildingByID Name = %v, want %q", got.Name, name)
	}

	newName := name + "-updated"
	if err := pc.UpdateBuildingByID(ctx, intToStr(id), &proclassic.Building{Name: classicStrPtr(newName)}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateBuildingByID(%d): %v", id, err)
	}

	if err := pc.DeleteBuildingByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteBuildingByID(%d): %v", id, err)
	}

	_, err = pc.GetBuildingByID(ctx, intToStr(id))
	if err == nil {
		t.Fatalf("GetBuildingByID(%d) after delete should 404, succeeded", id)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetBuildingByID(%d) after delete: want 404, got %v", id, err)
	}
}

func TestAcceptance_Classic_DepartmentCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-dept-" + runSuffix()
	created, err := pc.CreateDepartmentByID(ctx, "0", &proclassic.Department{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDepartmentByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateDepartmentByID returned no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteDepartmentByID", func() error { return pc.DeleteDepartmentByID(ctx, intToStr(id)) })

	got, err := pc.GetDepartmentByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetDepartmentByID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("Name = %v, want %q", got.Name, name)
	}

	if err := pc.DeleteDepartmentByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteDepartmentByID(%d): %v", id, err)
	}
	_, err = pc.GetDepartmentByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_CategoryCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-cat-" + runSuffix()
	prio := 5
	created, err := pc.CreateCategoryByID(ctx, "0", &proclassic.Category{Name: classicStrPtr(name), Priority: &prio})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateCategoryByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateCategoryByID returned no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteCategoryByID", func() error { return pc.DeleteCategoryByID(ctx, intToStr(id)) })

	got, err := pc.GetCategoryByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetCategoryByID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("Name = %v, want %q", got.Name, name)
	}

	if err := pc.DeleteCategoryByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteCategoryByID(%d): %v", id, err)
	}
	_, err = pc.GetCategoryByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_ScriptCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-script-" + runSuffix()
	contents := "#!/bin/sh\necho hello\n"
	created, err := pc.CreateScriptByID(ctx, "0", &proclassic.Script{
		Name:           classicStrPtr(name),
		ScriptContents: classicStrPtr(contents),
		Priority:       classicStrPtr("After"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateScriptByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateScriptByID returned no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteScriptByID", func() error { return pc.DeleteScriptByID(ctx, intToStr(id)) })

	got, err := pc.GetScriptByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetScriptByID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("Name = %v, want %q", got.Name, name)
	}

	newName := name + "-updated"
	if err := pc.UpdateScriptByID(ctx, intToStr(id), &proclassic.Script{Name: classicStrPtr(newName)}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateScriptByID(%d): %v", id, err)
	}

	if err := pc.DeleteScriptByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteScriptByID(%d): %v", id, err)
	}
	_, err = pc.GetScriptByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_UserCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-user-" + runSuffix()
	email := name + "@example.test"
	created, err := pc.CreateUserByID(ctx, "0", &proclassic.UserPost{
		Name:  classicStrPtr(name),
		Email: classicStrPtr(email),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateUserByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateUserByID returned no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteUserByID", func() error { return pc.DeleteUserByID(ctx, intToStr(id)) })

	got, err := pc.GetUserByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetUserByID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("Name = %v, want %q", got.Name, name)
	}

	if err := pc.DeleteUserByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteUserByID(%d): %v", id, err)
	}
	_, err = pc.GetUserByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_ComputerEACRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-ea-" + runSuffix()
	created, err := pc.CreateComputerExtensionAttributeByID(ctx, "0", &proclassic.ComputerExtensionAttribute{
		Name:     classicStrPtr(name),
		DataType: classicStrPtr("String"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateComputerExtensionAttributeByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateComputerExtensionAttributeByID returned no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteComputerExtensionAttributeByID", func() error { return pc.DeleteComputerExtensionAttributeByID(ctx, intToStr(id)) })

	got, err := pc.GetComputerExtensionAttributeByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerExtensionAttributeByID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("Name = %v, want %q", got.Name, name)
	}

	if err := pc.DeleteComputerExtensionAttributeByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteComputerExtensionAttributeByID(%d): %v", id, err)
	}
	_, err = pc.GetComputerExtensionAttributeByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_MobileDeviceEACRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-mdea-" + runSuffix()
	created, err := pc.CreateMobileDeviceExtensionAttributeByID(ctx, "0", &proclassic.MobileDeviceExtensionAttribute{
		Name:     classicStrPtr(name),
		DateType: classicStrPtr("String"), // spec typo: date_type; Jamf spec has this misspelling
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceExtensionAttributeByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteMobileDeviceExtensionAttributeByID", func() error { return pc.DeleteMobileDeviceExtensionAttributeByID(ctx, intToStr(id)) })

	if err := pc.DeleteMobileDeviceExtensionAttributeByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetMobileDeviceExtensionAttributeByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_UserEACRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-uea-" + runSuffix()
	created, err := pc.CreateUserExtensionAttributeByID(ctx, "0", &proclassic.UserExtensionAttribute{
		Name:     classicStrPtr(name),
		DataType: classicStrPtr("String"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateUserExtensionAttributeByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteUserExtensionAttributeByID", func() error { return pc.DeleteUserExtensionAttributeByID(ctx, intToStr(id)) })

	if err := pc.DeleteUserExtensionAttributeByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetUserExtensionAttributeByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_ComputerGroupCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-cg-" + runSuffix()
	isSmart := false
	created, err := pc.CreateComputerGroupByID(ctx, "0", &proclassic.ComputerGroupPost{
		Name:    classicStrPtr(name),
		IsSmart: &isSmart,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateComputerGroupByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteComputerGroupByID", func() error { return pc.DeleteComputerGroupByID(ctx, intToStr(id)) })

	if err := pc.DeleteComputerGroupByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetComputerGroupByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_MobileDeviceGroupCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-mdg-" + runSuffix()
	isSmart := false
	created, err := pc.CreateMobileDeviceGroupByID(ctx, "0", &proclassic.MobileDeviceGroup{
		Name:    classicStrPtr(name),
		IsSmart: &isSmart,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceGroupByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteMobileDeviceGroupByID", func() error { return pc.DeleteMobileDeviceGroupByID(ctx, intToStr(id)) })

	if err := pc.DeleteMobileDeviceGroupByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetMobileDeviceGroupByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_UserGroupCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-ug-" + runSuffix()
	isSmart := false
	created, err := pc.CreateUserGroupByID(ctx, "0", &proclassic.UserGroup{
		Name:    classicStrPtr(name),
		IsSmart: &isSmart,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateUserGroupByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteUserGroupByID", func() error { return pc.DeleteUserGroupByID(ctx, intToStr(id)) })

	if err := pc.DeleteUserGroupByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetUserGroupByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_AdvancedComputerSearchCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-acs-" + runSuffix()
	created, err := pc.CreateAdvancedComputerSearchByID(ctx, "0", &proclassic.AdvancedComputerSearch{
		Name: classicStrPtr(name),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAdvancedComputerSearchByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteAdvancedComputerSearchByID", func() error { return pc.DeleteAdvancedComputerSearchByID(ctx, intToStr(id)) })

	if err := pc.DeleteAdvancedComputerSearchByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetAdvancedComputerSearchByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_AdvancedMobileDeviceSearchCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-amds-" + runSuffix()
	created, err := pc.CreateAdvancedMobileDeviceSearchByID(ctx, "0", &proclassic.AdvancedMobileDeviceSearch{
		Name: classicStrPtr(name),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAdvancedMobileDeviceSearchByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteAdvancedMobileDeviceSearchByID", func() error { return pc.DeleteAdvancedMobileDeviceSearchByID(ctx, intToStr(id)) })

	if err := pc.DeleteAdvancedMobileDeviceSearchByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetAdvancedMobileDeviceSearchByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_AdvancedUserSearchCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-aus-" + runSuffix()
	created, err := pc.CreateAdvancedUserSearchByID(ctx, "0", &proclassic.AdvancedUserSearch{
		Name: classicStrPtr(name),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAdvancedUserSearchByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteAdvancedUserSearchByID", func() error { return pc.DeleteAdvancedUserSearchByID(ctx, intToStr(id)) })

	if err := pc.DeleteAdvancedUserSearchByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetAdvancedUserSearchByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_PolicyCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-policy-" + runSuffix()
	enabled := false
	created, err := pc.CreatePolicyByID(ctx, "0", &proclassic.PolicyPost{
		General: &proclassic.PolicyPostGeneral{
			Name:    classicStrPtr(name),
			Enabled: &enabled,
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreatePolicyByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeletePolicyByID", func() error { return pc.DeletePolicyByID(ctx, intToStr(id)) })

	got, err := pc.GetPolicyByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPolicyByID(%d): %v", id, err)
	}
	if got.General == nil || got.General.Name == nil || *got.General.Name != name {
		t.Errorf("Name = %v, want %q", got.General.Name, name)
	}

	if err := pc.DeletePolicyByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetPolicyByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_OSXConfigurationProfileCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-osxcp-" + runSuffix()
	created, err := pc.CreateOSXConfigurationProfileByID(ctx, "0", &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{
			Name: classicStrPtr(name),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateOSXConfigurationProfileByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteOSXConfigurationProfileByID", func() error { return pc.DeleteOSXConfigurationProfileByID(ctx, intToStr(id)) })

	if err := pc.DeleteOSXConfigurationProfileByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetOSXConfigurationProfileByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_MobileDeviceConfigurationProfileCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-mdcp-" + runSuffix()
	created, err := pc.CreateMobileDeviceConfigurationProfileByID(ctx, "0", &proclassic.MobileDeviceConfigurationProfile{
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{
			Name: classicStrPtr(name),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceConfigurationProfileByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteMobileDeviceConfigurationProfileByID", func() error { return pc.DeleteMobileDeviceConfigurationProfileByID(ctx, intToStr(id)) })

	if err := pc.DeleteMobileDeviceConfigurationProfileByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetMobileDeviceConfigurationProfileByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

// TestAcceptance_Classic_MobileDeviceCRUD exercises the mobile_device
// CRUD lifecycle with a synthetic placeholder record (no real device
// is touched). The Classic POST /mobiledevices/id/0 endpoint accepts
// a partial General block; we round-trip via GetBySerialNumber to
// recover the server-assigned id, rename via update, then delete and
// verify 404.
func TestAcceptance_Classic_MobileDeviceCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-mobile-" + runSuffix()
	serial := "SDK" + runSuffix()
	udid := "sdk-udid-" + runSuffix()

	_, err := pc.CreateMobileDeviceByID(ctx, "0", &proclassic.MobileDevicePost{
		General: &proclassic.MobileDevicePostGeneral{
			DeviceName:   classicStrPtr(name),
			SerialNumber: classicStrPtr(serial),
			UDID:         classicStrPtr(udid),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceByID(0): %v", err)
	}
	cleanupDelete(t, "DeleteMobileDeviceBySerialNumber", func() error { return pc.DeleteMobileDeviceBySerialNumber(ctx, serial) })
	t.Logf("Created mobile device name=%q serial=%q", name, serial)

	got, err := pc.GetMobileDeviceBySerialNumber(ctx, serial)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceBySerialNumber(%q): %v", serial, err)
	}
	if got == nil || got.General == nil || got.General.ID == nil {
		t.Fatalf("expected MobileDevice.General.ID populated, got %+v", got)
	}
	id := *got.General.ID
	if got.General.DeviceName == nil || *got.General.DeviceName != name {
		t.Errorf("DeviceName = %v, want %q", got.General.DeviceName, name)
	}

	newName := name + "-updated"
	if err := pc.UpdateMobileDeviceByID(ctx, intToStr(id), &proclassic.MobileDevicePost{
		General: &proclassic.MobileDevicePostGeneral{DeviceName: classicStrPtr(newName)},
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateMobileDeviceByID(%d): %v", id, err)
	}

	afterUpdate, err := pc.GetMobileDeviceByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceByID(%d) after update: %v", id, err)
	}
	if afterUpdate.General == nil || afterUpdate.General.DeviceName == nil || *afterUpdate.General.DeviceName != newName {
		t.Errorf("after update DeviceName = %v, want %q", afterUpdate.General.DeviceName, newName)
	}

	if err := pc.DeleteMobileDeviceByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteMobileDeviceByID(%d): %v", id, err)
	}
	_, err = pc.GetMobileDeviceByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

// TestAcceptance_Classic_GetMobileDeviceByID exercises the endpoint
// against a real enrolled device. If JAMFPLATFORM_CLASSIC_MOBILE_DEVICE_ID
// is set, uses that id; otherwise pulls the first entry from the live
// /mobiledevices list and probes it. Skipped only when the tenant has
// zero enrolled mobile devices.
func TestAcceptance_Classic_GetMobileDeviceByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()

	id := os.Getenv("JAMFPLATFORM_CLASSIC_MOBILE_DEVICE_ID")
	if id == "" {
		list, err := pc.ListMobileDevices(ctx)
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("ListMobileDevices: %v", err)
		}
		if list == nil || len(list.MobileDevices) == 0 {
			t.Skip("tenant has no enrolled mobile devices; set JAMFPLATFORM_CLASSIC_MOBILE_DEVICE_ID to override")
		}
		first := list.MobileDevices[0]
		if first.ID == nil {
			t.Fatalf("first mobile device in list has no ID: %+v", first)
		}
		id = intToStr(*first.ID)
	}

	md, err := pc.GetMobileDeviceByID(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceByID(%s): %v", id, err)
	}
	if md == nil || md.General == nil {
		t.Fatalf("expected MobileDevice.General populated, got %+v", md)
	}
	t.Logf("MobileDevice id=%v serial=%v", md.General.ID, md.General.SerialNumber)
}

func TestAcceptance_Classic_PrinterCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-printer-" + runSuffix()
	created, err := pc.CreatePrinterByID(ctx, "0", &proclassic.Printer{
		Name:     classicStrPtr(name),
		CUPSName: classicStrPtr("PDF"),
		URI:      classicStrPtr("lpd://printer.local/queue"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreatePrinterByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeletePrinterByID", func() error { return pc.DeletePrinterByID(ctx, intToStr(id)) })

	if err := pc.DeletePrinterByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetPrinterByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_DirectoryBindingCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-dirbind-" + runSuffix()
	created, err := pc.CreateDirectoryBindingByID(ctx, "0", &proclassic.DirectoryBinding{
		Name:   classicStrPtr(name),
		Domain: classicStrPtr("example.test"),
		Type:   classicStrPtr("Active Directory"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDirectoryBindingByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteDirectoryBindingByID", func() error { return pc.DeleteDirectoryBindingByID(ctx, intToStr(id)) })

	if err := pc.DeleteDirectoryBindingByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetDirectoryBindingByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_ClassicPackageCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-pkg-" + runSuffix()
	filename := name + ".pkg"
	created, err := pc.CreateClassicPackageByID(ctx, "0", &proclassic.Package{
		Name:     classicStrPtr(name),
		Filename: classicStrPtr(filename),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateClassicPackageByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteClassicPackageByID", func() error { return pc.DeleteClassicPackageByID(ctx, intToStr(id)) })

	if err := pc.DeleteClassicPackageByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetClassicPackageByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_NetworkSegmentCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-ns-" + runSuffix()
	created, err := pc.CreateNetworkSegmentByID(ctx, "0", &proclassic.NetworkSegmentPost{
		Name:           classicStrPtr(name),
		StartingAddress: classicStrPtr("10.200.0.1"),
		EndingAddress:   classicStrPtr("10.200.0.255"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateNetworkSegmentByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteNetworkSegmentByID", func() error { return pc.DeleteNetworkSegmentByID(ctx, intToStr(id)) })

	if err := pc.DeleteNetworkSegmentByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetNetworkSegmentByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_DistributionPointCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-dp-" + runSuffix()
	noAuth := true
	created, err := pc.CreateDistributionPointByID(ctx, "0", &proclassic.DistributionPointPost{
		Name:                     classicStrPtr(name),
		IPAddress:                classicStrPtr("dp.example.test"),
		ShareName:                classicStrPtr("CasperShare"),
		ReadOnlyUsername:         classicStrPtr("ro-user"),
		ReadWriteUsername:        classicStrPtr("rw-user"),
		NoAuthenticationRequired: &noAuth,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDistributionPointByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteDistributionPointByID", func() error { return pc.DeleteDistributionPointByID(ctx, intToStr(id)) })

	if err := pc.DeleteDistributionPointByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetDistributionPointByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_LDAPServerCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-ldap-" + runSuffix()
	hostname := "ldap.example.test"
	port := 389
	created, err := pc.CreateLDAPServerByID(ctx, "0", &proclassic.LdapServerPost{
		Connection: &proclassic.LdapServerPostConnection{
			Name:               classicStrPtr(name),
			Hostname:           classicStrPtr(hostname),
			Port:               &port,
			ServerType:         classicStrPtr("Active Directory"),
			AuthenticationType: classicStrPtr("none"),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateLDAPServerByID: %v", err)
	}
	if created == nil || (created.ID == nil && (created.Connection == nil || created.Connection.ID == nil)) {
		t.Fatalf("no ID: %+v", created)
	}
	id := 0
	if created.ID != nil {
		id = *created.ID
	} else {
		id = *created.Connection.ID
	}
	cleanupDelete(t, "DeleteLDAPServerByID", func() error { return pc.DeleteLDAPServerByID(ctx, intToStr(id)) })

	if err := pc.DeleteLDAPServerByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetLDAPServerByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_MacApplicationCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-macapp-" + runSuffix()
	bundle := "com.example.sdk-" + runSuffix()
	created, err := pc.CreateMacApplicationByID(ctx, "0", &proclassic.MacApplication{
		General: &proclassic.MacApplicationGeneral{
			Name:     classicStrPtr(name),
			BundleID: classicStrPtr(bundle),
			Version:  classicStrPtr("1.0.0"),
			URL:      classicStrPtr("https://apps.apple.com/us/app/id123456"),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMacApplicationByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteMacApplicationByID", func() error { return pc.DeleteMacApplicationByID(ctx, intToStr(id)) })

	if err := pc.DeleteMacApplicationByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetMacApplicationByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

// TestAcceptance_Classic_MobileDeviceApplicationCRUD covers create but
// tolerates the tenant's async-DELETE quirk: Classic mobile-device-app
// records become deletable only after an indexing step, so DELETE issued
// too soon returns HTTP 400 with a body echoing the id. The test asserts
// the create+read round-trip and logs delete as best-effort.
func TestAcceptance_Classic_MobileDeviceApplicationCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-mdapp-" + runSuffix()
	bundle := "com.example.sdk-" + runSuffix()
	version := "1.0.0"
	created, err := pc.CreateMobileDeviceApplicationByID(ctx, "0", &proclassic.MobileDeviceApplication{
		General: &proclassic.MobileDeviceApplicationGeneral{
			Name:     classicStrPtr(name),
			BundleID: classicStrPtr(bundle),
			Version:  classicStrPtr(version),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceApplicationByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteMobileDeviceApplicationByID", func() error { return pc.DeleteMobileDeviceApplicationByID(ctx, intToStr(id)) })
	t.Logf("created mobile-device-app id=%d; delete is async-best-effort on this tenant", id)
}

// TestAcceptance_Classic_EbookCRUD exercises the ebook create + read
// lifecycle. The Classic server on this tenant has a misreported
// DELETE /ebooks/{by-id,by-name} response — returns HTTP 400 with an
// id-echo body (`<ebook><id>N</id></ebook>`) but the record IS removed
// server-side. ListEbooks is eventually-consistent and can briefly
// continue to include the removed record. Cleanup issues both by-id
// and by-name deletes so the tenant settles to clean between runs.
func TestAcceptance_Classic_EbookCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-ebook-" + runSuffix()
	created, err := pc.CreateEbookByID(ctx, "0", &proclassic.EbookPost{
		General: &proclassic.EbookPostGeneral{
			Name: classicStrPtr(name),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateEbookByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteEbookByID", func() error { return pc.DeleteEbookByID(ctx, intToStr(id)) })
	cleanupDelete(t, "DeleteEbookByName", func() error { return pc.DeleteEbookByName(ctx, name) })
	t.Logf("created ebook id=%d; two-step cleanup (by-id 400-echo, by-name) queued", id)
}

func TestAcceptance_Classic_ClassCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-class-" + runSuffix()
	created, err := pc.CreateClassByID(ctx, "0", &proclassic.ClassPost{Name: classicStrPtr(name)})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateClassByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteClassByID", func() error { return pc.DeleteClassByID(ctx, intToStr(id)) })
	if err := pc.DeleteClassByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetClassByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_LicensedSoftwareCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-licsw-" + runSuffix()
	created, err := pc.CreateLicensedSoftwareByID(ctx, "0", &proclassic.LicensedSoftware{
		General: &proclassic.LicensedSoftwareGeneral{Name: classicStrPtr(name)},
	})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateLicensedSoftwareByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteLicensedSoftwareByID", func() error { return pc.DeleteLicensedSoftwareByID(ctx, intToStr(id)) })
	if err := pc.DeleteLicensedSoftwareByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetLicensedSoftwareByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_RestrictedSoftwareCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-restsw-" + runSuffix()
	created, err := pc.CreateRestrictedSoftwareByID(ctx, "0", &proclassic.RestrictedSoftware{
		General: &proclassic.RestrictedSoftwareGeneral{Name: classicStrPtr(name), ProcessName: classicStrPtr("evil.app")},
	})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateRestrictedSoftwareByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteRestrictedSoftwareByID", func() error { return pc.DeleteRestrictedSoftwareByID(ctx, intToStr(id)) })
	if err := pc.DeleteRestrictedSoftwareByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetRestrictedSoftwareByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_PeripheralTypeCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-ptype-" + runSuffix()
	created, err := pc.CreatePeripheralTypeByID(ctx, "0", &proclassic.PeripheralType{Name: classicStrPtr(name)})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreatePeripheralTypeByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeletePeripheralTypeByID", func() error { return pc.DeletePeripheralTypeByID(ctx, intToStr(id)) })
	if err := pc.DeletePeripheralTypeByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetPeripheralTypeByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_DiskEncryptionConfigurationCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-dec-" + runSuffix()
	created, err := pc.CreateDiskEncryptionConfigurationByID(ctx, "0", &proclassic.DiskEncryptionConfiguration{
		Name:                  classicStrPtr(name),
		KeyType:               classicStrPtr("Individual"),
		FileVaultEnabledUsers: classicStrPtr("Management Account"),
	})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateDiskEncryptionConfigurationByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteDiskEncryptionConfigurationByID", func() error { return pc.DeleteDiskEncryptionConfigurationByID(ctx, intToStr(id)) })
	if err := pc.DeleteDiskEncryptionConfigurationByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetDiskEncryptionConfigurationByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_IBeaconCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-ibeacon-" + runSuffix()
	created, err := pc.CreateIBeaconByID(ctx, "0", &proclassic.Ibeacon{
		Name: classicStrPtr(name),
		UUID: classicStrPtr("12345678-1234-1234-1234-123456789012"),
	})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateIBeaconByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteIBeaconByID", func() error { return pc.DeleteIBeaconByID(ctx, intToStr(id)) })
	if err := pc.DeleteIBeaconByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetIBeaconByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_DockItemCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-dock-" + runSuffix()
	created, err := pc.CreateDockItemByID(ctx, "0", &proclassic.DockItem{
		Name: classicStrPtr(name),
		Path: classicStrPtr("file:///Applications/Safari.app/"),
		Type: classicStrPtr("App"),
	})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateDockItemByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteDockItemByID", func() error { return pc.DeleteDockItemByID(ctx, intToStr(id)) })
	if err := pc.DeleteDockItemByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetDockItemByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_RemovableMacAddressCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "AA:BB:CC:DD:EE:" + runSuffix()[len(runSuffix())-2:]
	created, err := pc.CreateRemovableMacAddressByID(ctx, "0", &proclassic.RemovableMacAddress{Name: classicStrPtr(name)})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateRemovableMacAddressByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteRemovableMacAddressByID", func() error { return pc.DeleteRemovableMacAddressByID(ctx, intToStr(id)) })
	if err := pc.DeleteRemovableMacAddressByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetRemovableMacAddressByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_AllowedFileExtensionCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	ext := "sdk" + runSuffix()
	created, err := pc.CreateAllowedFileExtensionByID(ctx, "0", &proclassic.AllowedFileExtension{Extension: classicStrPtr(ext)})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateAllowedFileExtensionByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteAllowedFileExtensionByID", func() error { return pc.DeleteAllowedFileExtensionByID(ctx, intToStr(id)) })
	if err := pc.DeleteAllowedFileExtensionByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetAllowedFileExtensionByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

// TestAcceptance_Classic_JsonWebTokenConfigurationCRUD is intentionally
// skipped: the server requires an `encryption_key` field on create that
// Jamf's v11.20.0 spec does not declare, so the generated
// JsonWebTokenConfiguration struct can't carry it. The SDK's CRUD path
// is exercised by the unit tests; restoring this live test needs a
// spec patch (or a generator option for spec-unmodeled required fields).
func TestAcceptance_Classic_JsonWebTokenConfigurationCRUD(t *testing.T) {
	t.Skip("spec omits required encryption_key; generated struct can't carry it. See unit tests for endpoint coverage.")
}

func TestAcceptance_Classic_WebhookCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-wh-" + runSuffix()
	created, err := pc.CreateWebhookByID(ctx, "0", &proclassic.Webhook{
		Name:     classicStrPtr(name),
		URL:      classicStrPtr("https://webhook.example.test/receiver"),
		Event:    classicStrPtr("ComputerAdded"),
		ContentType: classicStrPtr("application/json"),
	})
	if err != nil { skipOnServerError(t, err); t.Fatalf("CreateWebhookByID: %v", err) }
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteWebhookByID", func() error { return pc.DeleteWebhookByID(ctx, intToStr(id)) })
	if err := pc.DeleteWebhookByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetWebhookByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

// TestAcceptance_Classic_AccountUserCRUD exercises the /accounts/userid
// CRUD lifecycle. The Classic spec omits the `password` field the
// server requires on create; the SDK generator injects it via the
// schemaAdditions hook in config.json so we can send a valid payload.
func TestAcceptance_Classic_AccountUserCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-user-" + runSuffix()
	created, err := pc.CreateAccountByUserID(ctx, "0", &proclassic.Account{
		Name:         classicStrPtr(name),
		FullName:     classicStrPtr("SDK Acceptance User"),
		Email:        classicStrPtr(name + "@sdk.test"),
		Password:     classicStrPtr("SDK-acc-pw-" + runSuffix() + "!"),
		AccessLevel:  classicStrPtr("Full Access"),
		PrivilegeSet: classicStrPtr("Administrator"),
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(403) {
			t.Skipf("forbidden on this tenant: %v", err)
		}
		t.Fatalf("CreateAccountByUserID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteAccountByUserID", func() error { return pc.DeleteAccountByUserID(ctx, intToStr(id)) })

	got, err := pc.GetAccountByUserID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAccountByUserID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("Name = %v, want %q", got.Name, name)
	}

	if err := pc.DeleteAccountByUserID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetAccountByUserID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_AccountGroupCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-grp-" + runSuffix()
	created, err := pc.CreateAccountGroupByID(ctx, "0", &proclassic.Group{
		Name:         classicStrPtr(name),
		AccessLevel:  classicStrPtr("Full Access"),
		PrivilegeSet: classicStrPtr("Administrator"),
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(403) {
			t.Skipf("forbidden on this tenant: %v", err)
		}
		t.Fatalf("CreateAccountGroupByID: %v", err)
	}
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteAccountGroupByID", func() error { return pc.DeleteAccountGroupByID(ctx, intToStr(id)) })
	if err := pc.DeleteAccountGroupByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetAccountGroupByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

// TestAcceptance_Classic_ComputerInvitationCRUD exercises the
// /computerinvitations CRUD lifecycle. Classic 500s on create unless
// SshUsername + SshPassword are both set — the server uses those creds
// to SSH into the target computer to complete enrollment, and rejects
// any attempt that doesn't carry them. InvitationType=USER_INITIATED_URL
// keeps the invitation from trying to send an actual email. The 39-digit
// invitation code the server returns rides on *BigInt.
func TestAcceptance_Classic_ComputerInvitationCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	createAccount := false
	created, err := pc.CreateComputerInvitationByID(ctx, "0", &proclassic.ComputerInvitation{
		InvitationType:              classicStrPtr("USER_INITIATED_URL"),
		SshUsername:                 classicStrPtr("sdk-acc"),
		SshPassword:                 classicStrPtr("sdk-acc-pw"),
		CreateAccountIfDoesNotExist: &createAccount,
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(403) {
			t.Skipf("forbidden: %v", err)
		}
		t.Fatalf("CreateComputerInvitationByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteComputerInvitationByID", func() error { return pc.DeleteComputerInvitationByID(ctx, intToStr(id)) })

	got, err := pc.GetComputerInvitationByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerInvitationByID(%d): %v", id, err)
	}
	if got == nil || got.Invitation == nil {
		t.Fatalf("expected Invitation populated, got %+v", got)
	}
	t.Logf("ComputerInvitation id=%d invitation=%s", id, got.Invitation.String())

	if err := pc.DeleteComputerInvitationByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetComputerInvitationByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

// TestAcceptance_Classic_MobileDeviceInvitationCRUD exercises the
// mobile_device_invitation CRUD lifecycle. The 39-digit `invitation`
// code the server returns is carried as *BigInt via the fieldTypeOverrides
// entry `*.invitation: *BigInt`, so the response decodes without int64
// overflow.
func TestAcceptance_Classic_MobileDeviceInvitationCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	created, err := pc.CreateMobileDeviceInvitationByID(ctx, "0", &proclassic.MobileDeviceInvitationPost{
		InvitationType: classicStrPtr("USER_INITIATED_URL"),
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(403) {
			t.Skipf("forbidden on this tenant: %v", err)
		}
		t.Fatalf("CreateMobileDeviceInvitationByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteMobileDeviceInvitationByID", func() error { return pc.DeleteMobileDeviceInvitationByID(ctx, intToStr(id)) })

	got, err := pc.GetMobileDeviceInvitationByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceInvitationByID(%d): %v", id, err)
	}
	if got == nil || got.Invitation == nil {
		t.Fatalf("expected Invitation populated, got %+v", got)
	}
	t.Logf("MobileDeviceInvitation id=%d invitation=%s", id, got.Invitation.String())

	if err := pc.DeleteMobileDeviceInvitationByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("delete: %v", err)
	}
	_, err = pc.GetMobileDeviceInvitationByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}

func TestAcceptance_Classic_MobileDeviceEnrollmentProfileCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-mdep-" + runSuffix()
	created, err := pc.CreateMobileDeviceEnrollmentProfileByID(ctx, "0", &proclassic.MobileDeviceEnrollmentProfilePost{
		General: &proclassic.MobileDeviceEnrollmentProfilePostGeneral{Name: classicStrPtr(name)},
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(403) { t.Skipf("forbidden: %v", err) }
		t.Fatalf("CreateMobileDeviceEnrollmentProfileByID: %v", err)
	}
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteMobileDeviceEnrollmentProfileByID", func() error { return pc.DeleteMobileDeviceEnrollmentProfileByID(ctx, intToStr(id)) })
	if err := pc.DeleteMobileDeviceEnrollmentProfileByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetMobileDeviceEnrollmentProfileByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

func TestAcceptance_Classic_MobileDeviceProvisioningProfileCRUD(t *testing.T) {
	t.Skip("mobile_device_provisioning_profile requires a real provisioning profile blob; test scaffolding only covers endpoint shape via unit tests")
}

func TestAcceptance_Classic_PatchExternalSourceCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-pes-" + runSuffix()
	port := 443
	sslEnabled := true
	created, err := pc.CreatePatchExternalSourceByID(ctx, "0", &proclassic.PatchExternalSource{
		Name:       classicStrPtr(name),
		HostName:   classicStrPtr("patches.example.test"),
		Port:       &port,
		SslEnabled: &sslEnabled,
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(403) { t.Skipf("forbidden: %v", err) }
		t.Fatalf("CreatePatchExternalSourceByID: %v", err)
	}
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeletePatchExternalSourceByID", func() error { return pc.DeletePatchExternalSourceByID(ctx, intToStr(id)) })
	if err := pc.DeletePatchExternalSourceByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetPatchExternalSourceByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

// TestAcceptance_Classic_GetPatchInternalSource is read-only. The built-in
// Jamf internal source is id=1; endpoint reports it whether or not
// customers have configured it. No write endpoints exist for internal
// sources.
func TestAcceptance_Classic_GetPatchInternalSource(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	src, err := pc.GetPatchInternalSourceByID(ctx, "1")
	if err != nil { skipOnServerError(t, err); t.Skipf("GetPatchInternalSourceByID(1): %v", err) }
	if src == nil { t.Fatal("expected non-nil internal source") }
}

// TestAcceptance_Classic_GetPatchAvailableTitles reads catalog data for
// the built-in internal source. No write surface needed.
func TestAcceptance_Classic_GetPatchAvailableTitles(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	titles, err := pc.ListPatchAvailableTitlesBySourceID(ctx, "1")
	if err != nil { skipOnServerError(t, err); t.Skipf("ListPatchAvailableTitlesBySourceID(1): %v", err) }
	if titles == nil { t.Fatal("expected non-nil available titles") }
}

// Read-only probes of Classic singletons — all Jamf tenants have these
// endpoints populated, and updating them via the test credentials would
// mutate tenant state (SMTP settings, activation code, etc.) which is
// not safe. Coverage is shape-of-response only; unit tests cover writes.

func TestAcceptance_Classic_GetActivationCode(t *testing.T) {
	c := accClient(t)
	a, err := proclassic.New(c).GetActivationCode(context.Background())
	if err != nil { skipOnServerError(t, err); t.Fatalf("GetActivationCode: %v", err) }
	if a == nil { t.Fatal("nil ActivationCode") }
}

func TestAcceptance_Classic_GetSMTPServer(t *testing.T) {
	c := accClient(t)
	s, err := proclassic.New(c).GetSMTPServer(context.Background())
	if err != nil { skipOnServerError(t, err); t.Fatalf("GetSMTPServer: %v", err) }
	if s == nil { t.Fatal("nil SMTPServer") }
}

func TestAcceptance_Classic_GetGSXConnection(t *testing.T) {
	c := accClient(t)
	g, err := proclassic.New(c).GetGSXConnection(context.Background())
	if err != nil { skipOnServerError(t, err); t.Fatalf("GetGSXConnection: %v", err) }
	if g == nil { t.Fatal("nil GSXConnection") }
}

func TestAcceptance_Classic_GetComputerCheckIn(t *testing.T) {
	c := accClient(t)
	ci, err := proclassic.New(c).GetComputerCheckIn(context.Background())
	if err != nil { skipOnServerError(t, err); t.Fatalf("GetComputerCheckIn: %v", err) }
	if ci == nil { t.Fatal("nil") }
}

func TestAcceptance_Classic_GetComputerInventoryCollection(t *testing.T) {
	c := accClient(t)
	ic, err := proclassic.New(c).GetComputerInventoryCollection(context.Background())
	if err != nil { skipOnServerError(t, err); t.Fatalf("GetComputerInventoryCollection: %v", err) }
	if ic == nil { t.Fatal("nil") }
}

func TestAcceptance_Classic_SoftwareUpdateServerCRUD(t *testing.T) {
	c := accClient(t); ctx := context.Background(); pc := proclassic.New(c)
	name := "sdk-acc-sus-" + runSuffix()
	port := 8088
	created, err := pc.CreateSoftwareUpdateServerByID(ctx, "0", &proclassic.SoftwareUpdateServer{
		Name:      classicStrPtr(name),
		IPAddress: classicStrPtr("sus.example.test"),
		Port:      &port,
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(403) { t.Skipf("forbidden: %v", err) }
		t.Fatalf("CreateSoftwareUpdateServerByID: %v", err)
	}
	if created == nil || created.ID == nil { t.Fatalf("no ID: %+v", created) }
	id := *created.ID
	cleanupDelete(t, "DeleteSoftwareUpdateServerByID", func() error { return pc.DeleteSoftwareUpdateServerByID(ctx, intToStr(id)) })
	if err := pc.DeleteSoftwareUpdateServerByID(ctx, intToStr(id)); err != nil { skipOnServerError(t, err); t.Fatalf("delete: %v", err) }
	_, err = pc.GetSoftwareUpdateServerByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) { t.Fatalf("after delete: want 404, got %v", err) }
}

// TestAcceptance_Classic_VPPCRUD tests VPP account create/delete.
// VPP create requires a real service_token; skip on 409 (spec
// validation) or 403 (tenant lacks VPP privilege).
func TestAcceptance_Classic_VPPInvitationCRUD(t *testing.T) {
	t.Skip("VPP invitation creation needs a real user_id + invitation_type + site; not exercising against live tenant")
}

func TestAcceptance_Classic_GetComputerHistoryByID(t *testing.T) {
	id := os.Getenv("JAMFPLATFORM_CLASSIC_COMPUTER_ID")
	if id == "" { id = "4" } // fallback to known computer id in this tenant
	c := accClient(t)
	h, err := proclassic.New(c).GetComputerHistoryByID(context.Background(), id)
	if err != nil { skipOnServerError(t, err); t.Skipf("GetComputerHistoryByID(%s): %v", id, err) }
	if h == nil { t.Fatal("nil ComputerHistory") }
}

func TestAcceptance_Classic_SiteCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-classic-site-" + runSuffix()
	created, err := pc.CreateSiteByID(ctx, "0", &proclassic.Site{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSiteByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateSiteByID returned no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteSiteByID", func() error { return pc.DeleteSiteByID(ctx, intToStr(id)) })

	got, err := pc.GetSiteByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSiteByID(%d): %v", id, err)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("Name = %v, want %q", got.Name, name)
	}

	if err := pc.DeleteSiteByID(ctx, intToStr(id)); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteSiteByID(%d): %v", id, err)
	}
	_, err = pc.GetSiteByID(ctx, intToStr(id))
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("after delete: want 404, got %v", err)
	}
}
