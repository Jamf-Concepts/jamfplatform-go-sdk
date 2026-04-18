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
	t.Cleanup(func() { _ = pc.DeleteBuildingByID(ctx, intToStr(id)) })
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
	t.Cleanup(func() { _ = pc.DeleteDepartmentByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteCategoryByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteScriptByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteUserByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteComputerExtensionAttributeByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteMobileDeviceExtensionAttributeByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteUserExtensionAttributeByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteComputerGroupByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteMobileDeviceGroupByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteUserGroupByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteAdvancedComputerSearchByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteAdvancedMobileDeviceSearchByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteAdvancedUserSearchByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeletePolicyByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteOSXConfigurationProfileByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteMobileDeviceConfigurationProfileByID(ctx, intToStr(id)) })

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

// TestAcceptance_Classic_GetMobileDeviceByID is read-only: mobile-device
// endpoints target real enrolled devices on the tenant. Uses env-provided
// id if available; otherwise logs that the endpoint shape is exercised
// by unit tests only.
func TestAcceptance_Classic_GetMobileDeviceByID(t *testing.T) {
	id := os.Getenv("JAMFPLATFORM_CLASSIC_MOBILE_DEVICE_ID")
	if id == "" {
		t.Skip("set JAMFPLATFORM_CLASSIC_MOBILE_DEVICE_ID to exercise GetMobileDeviceByID against a real enrolled device")
	}
	c := accClient(t)
	md, err := proclassic.New(c).GetMobileDeviceByID(context.Background(), id)
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
	t.Cleanup(func() { _ = pc.DeletePrinterByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteDirectoryBindingByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteClassicPackageByID(ctx, intToStr(id)) })

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
	t.Cleanup(func() { _ = pc.DeleteSiteByID(ctx, intToStr(id)) })

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
