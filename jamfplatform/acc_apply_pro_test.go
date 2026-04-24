// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// ---------------------------------------------------------------------------
// Apply acceptance tests — Pro resources
//
// Each test exercises the full Apply lifecycle:
//  1. Apply (create) — resource does not exist → created=true
//  2. Apply (update) — resource exists → created=false
//  3. Delete — clean up
//  4. Resolve — confirm not found (404)
//
// Convention: resource names are prefixed "sdk-acc-apply-" with runSuffix()
// to avoid collisions with other tests. All tests create and delete their
// own fixtures; no shared state.
//
// The user explicitly instructed: do NOT skip 500 errors.
// ---------------------------------------------------------------------------

// ---------- BuildingV1 ----------

func TestAcceptance_ApplyBuildingV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-building-" + runSuffix()

	// 1. Apply creates
	id, created, err := p.ApplyBuildingV1(ctx, &pro.Building{Name: name})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "Building "+id, func() error { return p.DeleteBuildingV1(ctx, id) })
	if !created {
		t.Error("expected created = true on first apply")
	}
	t.Logf("created building id=%s", id)

	// 2. Apply updates
	city := "Minneapolis"
	id2, created2, err := p.ApplyBuildingV1(ctx, &pro.Building{Name: name, City: &city})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on second apply")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	// 3. Delete
	if err := p.DeleteBuildingV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 4. Resolve not found
	_, err = p.ResolveBuildingV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
	if apiErr := jamfplatform.AsAPIError(err); apiErr == nil || !apiErr.HasStatus(404) {
		t.Fatalf("expected 404, got: %v", err)
	}
}

// ---------- DepartmentV1 ----------

func TestAcceptance_ApplyDepartmentV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-dept-" + runSuffix()

	id, created, err := p.ApplyDepartmentV1(ctx, &pro.Department{Name: name})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "Department "+id, func() error { return p.DeleteDepartmentV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyDepartmentV1(ctx, &pro.Department{Name: name})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteDepartmentV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveDepartmentV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- CategoryV1 ----------

func TestAcceptance_ApplyCategoryV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-cat-" + runSuffix()

	id, created, err := p.ApplyCategoryV1(ctx, &pro.Category{Name: name, Priority: 5})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "Category "+id, func() error { return p.DeleteCategoryV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyCategoryV1(ctx, &pro.Category{Name: name, Priority: 10})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteCategoryV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveCategoryV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- ScriptV1 ----------

func TestAcceptance_ApplyScriptV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-script-" + runSuffix()

	id, created, err := p.ApplyScriptV1(ctx, &pro.Script{Name: name})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "Script "+id, func() error { return p.DeleteScriptV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	info := "updated"
	id2, created2, err := p.ApplyScriptV1(ctx, &pro.Script{Name: name, Info: &info})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteScriptV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveScriptV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- ApiRoleV1 ----------

func TestAcceptance_ApplyApiRoleV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-role-" + runSuffix()

	id, created, err := p.ApplyApiRoleV1(ctx, &pro.ApiRoleRequest{
		DisplayName: name,
		Privileges:  []string{"Read Buildings"},
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "ApiRole "+id, func() error { return p.DeleteApiRoleV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyApiRoleV1(ctx, &pro.ApiRoleRequest{
		DisplayName: name,
		Privileges:  []string{"Read Buildings", "Read Categories"},
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteApiRoleV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveApiRoleV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- ApiIntegrationV1 ----------

func TestAcceptance_ApplyApiIntegrationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// AuthorizationScopes requires existing API role display names.
	roles, err := p.ListApiRolesV1(ctx, nil, "")
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	if len(roles) == 0 {
		t.Skip("no API roles available — cannot test integration apply")
	}
	roleName := roles[0].DisplayName

	name := "sdk-acc-apply-integ-" + runSuffix()
	enabled := true

	id, created, err := p.ApplyApiIntegrationV1(ctx, &pro.ApiIntegrationRequest{
		DisplayName:         name,
		Enabled:             &enabled,
		AuthorizationScopes: []string{roleName},
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "ApiIntegration "+id, func() error { return p.DeleteApiIntegrationV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyApiIntegrationV1(ctx, &pro.ApiIntegrationRequest{
		DisplayName:         name,
		Enabled:             &enabled,
		AuthorizationScopes: []string{roleName},
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteApiIntegrationV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveApiIntegrationV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- ComputerExtensionAttributeV1 ----------

func TestAcceptance_ApplyComputerExtensionAttributeV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-cea-" + runSuffix()
	boolTrue := true
	boolFalse := false
	emptyStr := ""
	emptySlice := []string{}

	id, created, err := p.ApplyComputerExtensionAttributeV1(ctx, &pro.ComputerExtensionAttributes{
		Name:                          name,
		Enabled:                       &boolTrue,
		DataType:                      "STRING",
		InputType:                     "TEXT",
		InventoryDisplayType:          "GENERAL",
		Description:                   strPtr("SDK acceptance test"),
		LdapAttributeMapping:          &emptyStr,
		LdapExtensionAttributeAllowed: &boolFalse,
		PopupMenuChoices:              &emptySlice,
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "CEA "+id, func() error { return p.DeleteComputerExtensionAttributeV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyComputerExtensionAttributeV1(ctx, &pro.ComputerExtensionAttributes{
		Name:                          name,
		Enabled:                       &boolTrue,
		DataType:                      "STRING",
		InputType:                     "TEXT",
		InventoryDisplayType:          "GENERAL",
		Description:                   strPtr("SDK acceptance test updated"),
		LdapAttributeMapping:          &emptyStr,
		LdapExtensionAttributeAllowed: &boolFalse,
		PopupMenuChoices:              &emptySlice,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteComputerExtensionAttributeV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveComputerExtensionAttributeV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- MobileDeviceExtensionAttributeV1 ----------

func TestAcceptance_ApplyMobileDeviceExtensionAttributeV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-mdea-" + runSuffix()

	id, created, err := p.ApplyMobileDeviceExtensionAttributeV1(ctx, &pro.MobileDeviceExtensionAttributes{
		Name:                          name,
		DataType:                      "STRING",
		InputType:                     "TEXT",
		InventoryDisplayType:          "GENERAL",
		Description:                   ptr("SDK acceptance test"),
		LdapAttributeMapping:          ptr(""),
		LdapExtensionAttributeAllowed: ptr(false),
		PopupMenuChoices:              &[]string{},
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "MDEA "+id, func() error { return p.DeleteMobileDeviceExtensionAttributeV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyMobileDeviceExtensionAttributeV1(ctx, &pro.MobileDeviceExtensionAttributes{
		Name:                          name,
		DataType:                      "STRING",
		InputType:                     "TEXT",
		InventoryDisplayType:          "GENERAL",
		Description:                   ptr("SDK acceptance test updated"),
		LdapAttributeMapping:          ptr(""),
		LdapExtensionAttributeAllowed: ptr(false),
		PopupMenuChoices:              &[]string{},
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteMobileDeviceExtensionAttributeV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveMobileDeviceExtensionAttributeV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- SmartComputerGroupV2 ----------

func TestAcceptance_ApplySmartComputerGroupV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-scg-" + runSuffix()

	id, created, err := p.ApplySmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{Name: name}, false)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "SmartComputerGroup "+id, func() error { return p.DeleteSmartComputerGroupV2(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplySmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{Name: name}, false)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteSmartComputerGroupV2(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveSmartComputerGroupV2IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- StaticComputerGroupV2 ----------

func TestAcceptance_ApplyStaticComputerGroupV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-stcg-" + runSuffix()
	assignments := []string{}

	id, created, err := p.ApplyStaticComputerGroupV2(ctx, &pro.StaticComputerGroupAssignment{
		Name:        name,
		Assignments: &assignments,
	}, false)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "StaticComputerGroup "+id, func() error { return p.DeleteStaticComputerGroupV2(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyStaticComputerGroupV2(ctx, &pro.StaticComputerGroupAssignment{
		Name:        name,
		Assignments: &assignments,
	}, false)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteStaticComputerGroupV2(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveStaticComputerGroupV2IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- SmartMobileDeviceGroupV1 ----------

func TestAcceptance_ApplySmartMobileDeviceGroupV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-smdg-" + runSuffix()

	id, created, err := p.ApplySmartMobileDeviceGroupV1(ctx, &pro.SmartGroupAssignment{GroupName: name, SiteID: strPtr("-1")}, false)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "SmartMobileDeviceGroup "+id, func() error { return p.DeleteSmartMobileDeviceGroupV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplySmartMobileDeviceGroupV1(ctx, &pro.SmartGroupAssignment{GroupName: name, SiteID: strPtr("-1")}, false)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteSmartMobileDeviceGroupV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveSmartMobileDeviceGroupV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- PackageV1 ----------

func TestAcceptance_ApplyPackageV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-pkg-" + runSuffix()

	id, created, err := p.ApplyPackageV1(ctx, &pro.Package{
		PackageName:          name,
		FileName:             "test.pkg",
		CategoryID:           "-1",
		Priority:             5,
		FillUserTemplate:     false,
		OsInstall:            false,
		RebootRequired:       false,
		SuppressEula:         false,
		SuppressFromDock:     false,
		SuppressRegistration: false,
		SuppressUpdates:      false,
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "Package "+id, func() error { return p.DeletePackageV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyPackageV1(ctx, &pro.Package{
		PackageName:          name,
		FileName:             "test-updated.pkg",
		CategoryID:           "-1",
		Priority:             10,
		FillUserTemplate:     false,
		OsInstall:            false,
		RebootRequired:       false,
		SuppressEula:         false,
		SuppressFromDock:     false,
		SuppressRegistration: false,
		SuppressUpdates:      false,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeletePackageV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolvePackageV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- DistributionPointV1 ----------

func TestAcceptance_ApplyDistributionPointV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-dp-" + runSuffix()

	principal := true
	id, created, err := p.ApplyDistributionPointV1(ctx, &pro.DistributionPoint{
		Name:                      name,
		FileSharingConnectionType: "SMB",
		ServerName:                "test-server.example.com",
		ShareName:                 strPtr("share"),
		ReadOnlyUsername:          strPtr("rouser"),
		ReadOnlyPassword:          strPtr("ropass"),
		ReadWriteUsername:         strPtr("rwuser"),
		ReadWritePassword:         strPtr("rwpass"),
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "DistributionPoint "+id, func() error { return p.DeleteDistributionPointV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyDistributionPointV1(ctx, &pro.DistributionPoint{
		Name:                      name,
		FileSharingConnectionType: "SMB",
		ServerName:                "test-server-updated.example.com",
		ShareName:                 strPtr("share"),
		ReadOnlyUsername:          strPtr("rouser"),
		ReadOnlyPassword:          strPtr("ropass"),
		ReadWriteUsername:         strPtr("rwuser"),
		ReadWritePassword:         strPtr("rwpass"),
		Principal:                 &principal,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteDistributionPointV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveDistributionPointV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- VolumePurchasingSubscriptionV1 ----------

func TestAcceptance_ApplyVolumePurchasingSubscriptionV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-vps-" + runSuffix()

	id, created, err := p.ApplyVolumePurchasingSubscriptionV1(ctx, &pro.VolumePurchasingSubscriptionBase{
		Name: name,
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "VPSubscription "+id, func() error { return p.DeleteVolumePurchasingSubscriptionV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyVolumePurchasingSubscriptionV1(ctx, &pro.VolumePurchasingSubscriptionBase{
		Name: name,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteVolumePurchasingSubscriptionV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveVolumePurchasingSubscriptionV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- AdvancedMobileDeviceSearchV1 ----------

func TestAcceptance_ApplyAdvancedMobileDeviceSearchV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-amds-" + runSuffix()

	id, created, err := p.ApplyAdvancedMobileDeviceSearchV1(ctx, &pro.AdvancedSearch{Name: name})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "AdvMobileSearch "+id, func() error { return p.DeleteAdvancedMobileDeviceSearchV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyAdvancedMobileDeviceSearchV1(ctx, &pro.AdvancedSearch{Name: name})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteAdvancedMobileDeviceSearchV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveAdvancedMobileDeviceSearchV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- AdvancedUserContentSearchV1 ----------

func TestAcceptance_ApplyAdvancedUserContentSearchV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-aucs-" + runSuffix()

	id, created, err := p.ApplyAdvancedUserContentSearchV1(ctx, &pro.AdvancedUserContentSearch{Name: name})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "AdvUserContentSearch "+id, func() error { return p.DeleteAdvancedUserContentSearchV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyAdvancedUserContentSearchV1(ctx, &pro.AdvancedUserContentSearch{Name: name})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteAdvancedUserContentSearchV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveAdvancedUserContentSearchV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- EnrollmentCustomizationV2 ----------

func TestAcceptance_ApplyEnrollmentCustomizationV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-ec-" + runSuffix()

	id, created, err := p.ApplyEnrollmentCustomizationV2(ctx, &pro.EnrollmentCustomizationV2{
		DisplayName: name,
		Description: "SDK acceptance test",
		SiteID:      "-1",
		EnrollmentCustomizationBrandingSettings: pro.EnrollmentCustomizationBrandingSettings{
			TextColor:       "000000",
			ButtonColor:     "0070C9",
			ButtonTextColor: "FFFFFF",
			BackgroundColor: "FFFFFF",
		},
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "EnrollmentCustomization "+id, func() error { return p.DeleteEnrollmentCustomizationV2(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyEnrollmentCustomizationV2(ctx, &pro.EnrollmentCustomizationV2{
		DisplayName: name,
		Description: "SDK acceptance test updated",
		SiteID:      "-1",
		EnrollmentCustomizationBrandingSettings: pro.EnrollmentCustomizationBrandingSettings{
			TextColor:       "000000",
			ButtonColor:     "0070C9",
			ButtonTextColor: "FFFFFF",
			BackgroundColor: "F0F0F0",
		},
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteEnrollmentCustomizationV2(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveEnrollmentCustomizationV2IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- EnrollmentAccessGroupV3 ----------

func TestAcceptance_ApplyEnrollmentAccessGroupV3(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Requires at least one LDAP server; skip if none configured.
	ldapServers, err := p.ListLdapServersV1(ctx)
	if err != nil {
		t.Fatalf("list ldap servers: %v", err)
	}
	if len(ldapServers) == 0 {
		t.Skip("no LDAP servers configured — cannot test enrollment access group apply")
	}
	ldapServerID := fmt.Sprintf("%d", ldapServers[0].ID)
	t.Logf("using LDAP server id=%s (%s)", ldapServerID, ldapServers[0].Name)

	name := "sdk-acc-apply-eag-" + runSuffix()
	siteID := "-1"

	id, created, err := p.ApplyEnrollmentAccessGroupV3(ctx, &pro.EnrollmentAccessGroupPreview{
		Name:         name,
		GroupID:      "1",
		LdapServerID: ldapServerID,
		SiteID:       &siteID,
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "EnrollmentAccessGroup "+id, func() error { return p.DeleteEnrollmentAccessGroupV3(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyEnrollmentAccessGroupV3(ctx, &pro.EnrollmentAccessGroupPreview{
		Name:         name,
		GroupID:      "1",
		LdapServerID: ldapServerID,
		SiteID:       &siteID,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteEnrollmentAccessGroupV3(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveEnrollmentAccessGroupV3IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- IOSBrandingConfigurationV1 ----------

func TestAcceptance_ApplyIOSBrandingConfigurationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-iosb-" + runSuffix()

	id, created, err := p.ApplyIOSBrandingConfigurationV1(ctx, &pro.IosBrandingConfiguration{
		BrandingName:              name,
		BrandingNameColorCode:     "000000",
		HeaderBackgroundColorCode: "FFFFFF",
		MenuIconColorCode:         "000000",
		StatusBarTextColor:        "DARK",
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "IOSBranding "+id, func() error { return p.DeleteIOSBrandingConfigurationV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyIOSBrandingConfigurationV1(ctx, &pro.IosBrandingConfiguration{
		BrandingName:              name,
		BrandingNameColorCode:     "111111",
		HeaderBackgroundColorCode: "FFFFFF",
		MenuIconColorCode:         "000000",
		StatusBarTextColor:        "DARK",
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteIOSBrandingConfigurationV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveIOSBrandingConfigurationV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- MacOSBrandingConfigurationV1 ----------

func TestAcceptance_ApplyMacOSBrandingConfigurationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-macosb-" + runSuffix()

	id, created, err := p.ApplyMacOSBrandingConfigurationV1(ctx, &pro.MacOsBrandingConfiguration{
		BrandingName: &name,
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "MacOSBranding "+id, func() error { return p.DeleteMacOSBrandingConfigurationV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyMacOSBrandingConfigurationV1(ctx, &pro.MacOsBrandingConfiguration{
		BrandingName: &name,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteMacOSBrandingConfigurationV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveMacOSBrandingConfigurationV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- AppInstallerDeploymentV1 ----------

func TestAcceptance_ApplyAppInstallerDeploymentV1(t *testing.T) {
	// App Installer deployments require a valid app installer title reference.
	// List existing titles and use the first one.
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	titles, err := p.ListAppInstallerTitlesV1(ctx)
	if err != nil {
		t.Fatalf("list app installer titles: %v", err)
	}
	if len(titles) == 0 {
		t.Skip("no app installer titles available — cannot test deployment apply")
	}
	titleID := titles[0].ID

	name := "sdk-acc-apply-aid-" + runSuffix()
	deployType := "INSTALL_AUTOMATICALLY"
	updateBeh := "AUTOMATIC"

	id, created, err := p.ApplyAppInstallerDeploymentV1(ctx, &pro.AppInstallerDeployment{
		Name:           &name,
		AppTitleID:     &titleID,
		DeploymentType: &deployType,
		UpdateBehavior: &updateBeh,
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "AppInstallerDeployment "+id, func() error { return p.DeleteAppInstallerDeploymentV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyAppInstallerDeploymentV1(ctx, &pro.AppInstallerDeployment{
		Name:           &name,
		AppTitleID:     &titleID,
		DeploymentType: &deployType,
		UpdateBehavior: &updateBeh,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteAppInstallerDeploymentV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveAppInstallerDeploymentV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- ReturnToServiceConfigurationV1 ----------

// wifiMobileProfilePayload returns a minimal mobile-device Wi-Fi mobileconfig plist.
func wifiMobileProfilePayload(suffix string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>AutoJoin</key>
			<true/>
			<key>EncryptionType</key>
			<string>WPA2</string>
			<key>HIDDEN_NETWORK</key>
			<false/>
			<key>PayloadDisplayName</key>
			<string>WiFi</string>
			<key>PayloadIdentifier</key>
			<string>com.jamf.sdk.test.mobile.wifi.%s</string>
			<key>PayloadType</key>
			<string>com.apple.wifi.managed</string>
			<key>PayloadUUID</key>
			<string>sdk-mobile-wifi-%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>SSID_STR</key>
			<string>sdk-test-network</string>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>sdk-acc-mobile-wifi-%s</string>
	<key>PayloadIdentifier</key>
	<string>com.jamf.sdk.test.mobile.%s</string>
	<key>PayloadScope</key>
	<string>System</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>sdk-mobile-profile-%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`, suffix, suffix, suffix, suffix, suffix)
}

func TestAcceptance_ApplyReturnToServiceConfigurationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)
	pc := proclassic.New(c)

	suffix := runSuffix()

	// RTS wifiProfileId requires a mobile device configuration profile (not macOS).
	profileName := "sdk-acc-mobile-wifi-" + suffix
	payloads := wifiMobileProfilePayload(suffix)
	level := "Device Level"
	profile, err := pc.CreateMobileDeviceConfigurationProfileByID(ctx, "0", &proclassic.MobileDeviceConfigurationProfile{
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{
			Name:     &profileName,
			Level:    &level,
			Payloads: &payloads,
		},
	})
	if err != nil {
		t.Fatalf("create mobile wifi profile: %v", err)
	}
	if profile.ID == nil {
		t.Fatal("created mobile wifi profile has nil ID")
	}
	wifiProfileID := strconv.Itoa(*profile.ID)
	cleanupDelete(t, "MobileWifiProfile "+wifiProfileID, func() error {
		return pc.DeleteMobileDeviceConfigurationProfileByID(ctx, wifiProfileID)
	})
	t.Logf("created mobile wifi profile id=%s for RTS test", wifiProfileID)

	// Now test Apply for ReturnToServiceConfiguration using the real wifi profile ID.
	name := "sdk-acc-apply-rts-" + suffix

	id, created, err := p.ApplyReturnToServiceConfigurationV1(ctx, &pro.ReturnToServiceConfigurationRequest{
		DisplayName:   &name,
		WifiProfileID: &wifiProfileID,
	})
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "RTS "+id, func() error { return p.DeleteReturnToServiceConfigurationV1(ctx, id) })
	if !created {
		t.Error("expected created = true")
	}

	id2, created2, err := p.ApplyReturnToServiceConfigurationV1(ctx, &pro.ReturnToServiceConfigurationRequest{
		DisplayName:   &name,
		WifiProfileID: &wifiProfileID,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	if err := p.DeleteReturnToServiceConfigurationV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveReturnToServiceConfigurationV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- UserV1 ----------

func TestAcceptance_ApplyUserV1(t *testing.T) {
	// User is a known exception per user instruction — skip.
	t.Skip("UserV1 is a known exception — requires platform param and complex setup")
}

// ---------- helpers ----------

// ---------- PatchSoftwareTitleConfigurationV2 ----------

func TestAcceptance_ApplyPatchSoftwareTitleConfigurationV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Patch software title configurations require a valid softwareTitleId,
	// which depends on an external patch source (e.g. Jamf's built-in patch
	// management definitions). List existing configs to find one, or skip.
	existing, err := p.ListPatchSoftwareTitleConfigurationsV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPatchSoftwareTitleConfigurationsV2: %v", err)
	}
	if len(existing) == 0 {
		t.Skip("tenant has no patch software title configurations — cannot determine softwareTitleId for Apply test")
	}

	// Use the first config's softwareTitleId for the new config.
	ref := existing[0]
	softwareTitleID := ref.SoftwareTitleID
	t.Logf("using softwareTitleId=%s from existing config %s (%s)", softwareTitleID, ref.ID, ref.DisplayName)

	name := "sdk-acc-apply-patchtitle-" + runSuffix()

	// 1. Apply creates
	id, created, err := p.ApplyPatchSoftwareTitleConfigurationV2(ctx, &pro.PatchSoftwareTitleConfigurationBase{
		DisplayName:     name,
		SoftwareTitleID: softwareTitleID,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "PatchSoftwareTitleConfig "+id, func() error { return p.DeletePatchSoftwareTitleConfigurationV2(ctx, id) })
	if !created {
		t.Error("expected created = true on first apply")
	}
	t.Logf("created patch software title config id=%s", id)

	// 2. Apply updates (JSON round-trip: PatchSoftwareTitleConfigurationBase → PatchSoftwareTitleConfigurationPatch)
	id2, created2, err := p.ApplyPatchSoftwareTitleConfigurationV2(ctx, &pro.PatchSoftwareTitleConfigurationBase{
		DisplayName:     name,
		SoftwareTitleID: softwareTitleID,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on second apply")
	}
	if id2 != id {
		t.Errorf("id mismatch: first=%s second=%s", id, id2)
	}

	// 3. Delete + verify 404
	if err := p.DeletePatchSoftwareTitleConfigurationV2(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolvePatchSoftwareTitleConfigurationV2IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- SupervisionIdentityV1 ----------

func TestAcceptance_ApplySupervisionIdentityV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-apply-supervision-" + runSuffix()

	// 1. Apply creates (SupervisionIdentityCreate has Password; update type SupervisionIdentityUpdate doesn't)
	id, created, err := p.ApplySupervisionIdentityV1(ctx, &pro.SupervisionIdentityCreate{
		DisplayName: name,
		Password:    "sdk-acc-test-pwd",
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "SupervisionIdentity "+id, func() error { return p.DeleteSupervisionIdentityV1(ctx, id) })
	if !created {
		t.Error("expected created = true on first apply")
	}
	t.Logf("created supervision identity id=%s", id)

	// 2. Apply updates
	id2, created2, err := p.ApplySupervisionIdentityV1(ctx, &pro.SupervisionIdentityCreate{
		DisplayName: name,
		Password:    "sdk-acc-test-pwd-2",
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on second apply")
	}
	if id2 != id {
		t.Errorf("id mismatch: first=%s second=%s", id, id2)
	}

	// 3. Delete + verify 404
	if err := p.DeleteSupervisionIdentityV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveSupervisionIdentityV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- VolumePurchasingLocationV1 ----------

func TestAcceptance_ApplyVolumePurchasingLocationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	token := vppToken(t) // skips if JAMFPLATFORM_VPP_TOKEN not set
	name := "sdk-acc-apply-vpl-" + runSuffix()

	// 1. Apply creates (VolumePurchasingLocationPost has ServiceToken; update type VolumePurchasingLocationPatch doesn't require it)
	id, created, err := p.ApplyVolumePurchasingLocationV1(ctx, &pro.VolumePurchasingLocationPost{
		Name:         &name,
		ServiceToken: token,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "VolumePurchasingLocation "+id, func() error { return p.DeleteVolumePurchasingLocationV1(ctx, id) })
	if !created {
		t.Error("expected created = true on first apply")
	}
	t.Logf("created volume purchasing location id=%s", id)

	// 2. Apply updates
	id2, created2, err := p.ApplyVolumePurchasingLocationV1(ctx, &pro.VolumePurchasingLocationPost{
		Name:         &name,
		ServiceToken: token,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on second apply")
	}
	if id2 != id {
		t.Errorf("id mismatch: first=%s second=%s", id, id2)
	}

	// 3. Delete + verify 404
	if err := p.DeleteVolumePurchasingLocationV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ResolveVolumePurchasingLocationV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
}

// ---------- Prestage Apply (Computer + Mobile, shared DEP token) ----------

// TestAcceptance_ApplyPrestages exercises both ComputerPrestageV3 and
// MobileDevicePrestageV3 Apply lifecycle sharing a single DEP token:
//  1. Upload DEP token
//  2. Apply computer prestage (create)
//  3. Apply computer prestage (update)
//  4. Apply mobile device prestage (create)
//  5. Apply mobile device prestage (update)
//  6. Delete both prestages
//  7. Delete DEP token (via t.Cleanup registered by createDepInstance)
func TestAcceptance_ApplyPrestages(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// 1. Upload DEP token — shared by both prestage sub-tests.
	depID := createDepInstance(t, "apply-prestages")
	t.Logf("DEP instance %s ready", depID)

	emptyStr := ""
	falseVal := false
	zeroStr := "0"
	zeroVersion := 0
	prefillCustom := "CUSTOM"
	userAccountAdmin := "ADMINISTRATOR"

	t.Run("ComputerPrestageV3", func(t *testing.T) {
		name := "sdk-acc-apply-compprestage-" + runSuffix()

		req := &pro.PostComputerPrestageV3{
			DisplayName:                       name,
			DeviceEnrollmentProgramInstanceID: depID,
			AccountSettings: &pro.AccountSettingsRequest{
				ID:                                      &zeroStr,
				VersionLock:                             &zeroVersion,
				PayloadConfigured:                       &falseVal,
				HiddenAdminAccount:                      &falseVal,
				LocalAdminAccountEnabled:                &falseVal,
				LocalUserManaged:                        &falseVal,
				AdminUsername:                           &emptyStr,
				PrefillAccountFullName:                  &emptyStr,
				PrefillAccountUserName:                  &emptyStr,
				PrefillPrimaryAccountInfoFeatureEnabled: &falseVal,
				PrefillType:                             &prefillCustom,
				PreventPrefillInfoFromModification:      &falseVal,
				UserAccountType:                         &userAccountAdmin,
			},
			DefaultPrestage:                    false,
			Mandatory:                          false,
			MDMRemovable:                       true,
			AutoAdvanceSetup:                   false,
			PreventActivationLock:              false,
			EnableDeviceBasedActivationLock:    false,
			InstallProfilesDuringSetup:         false,
			KeepExistingSiteMembership:         false,
			KeepExistingLocationInformation:    false,
			RequireAuthentication:              false,
			EnableRecoveryLock:                 &falseVal,
			RotateRecoveryLockPassword:         &falseVal,
			RecoveryLockPasswordType:           ptr("MANUAL"),
			EnrollmentSiteID:                   "-1",
			CustomPackageDistributionPointID:   "-1",
			EnrollmentCustomizationID:          &zeroStr,
			PrestageMinimumOsTargetVersionType: ptr("NO_ENFORCEMENT"),
			SkipSetupItems:                     &map[string]bool{},
			AnchorCertificates:                 &[]string{},
			CustomPackageIds:                   []string{},
			PrestageInstalledProfileIds:        []string{},
			LocationInformation: pro.LocationInformationV2{
				ID:           "-1",
				VersionLock:  0,
				DepartmentID: "-1",
				BuildingID:   "-1",
			},
			PurchasingInformation: pro.PrestagePurchasingInformationV2{
				ID:           "-1",
				VersionLock:  0,
				Purchased:    true,
				LeaseDate:    "1970-01-01",
				PoDate:       "1970-01-01",
				WarrantyDate: "1970-01-01",
			},
		}

		// 2. Apply creates
		id, created, err := p.ApplyComputerPrestageV3(ctx, req)
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("apply create: %v", err)
		}
		cleanupDelete(t, "ComputerPrestage "+id, func() error { return p.DeleteComputerPrestageV3(ctx, id) })
		if !created {
			t.Error("expected created = true on first apply")
		}
		t.Logf("created computer prestage id=%s", id)

		got, err := p.GetComputerPrestageV3(ctx, id)
		if err != nil {
			t.Fatalf("get after create: %v", err)
		}
		t.Logf("after create: versionLock=%d", got.VersionLock)

		// 3. Apply updates — versionLock injection exercised
		req.Mandatory = true // change something to force a real update
		id2, created2, err := p.ApplyComputerPrestageV3(ctx, req)
		if err != nil {
			t.Fatalf("apply update: %v", err)
		}
		if created2 {
			t.Error("expected created = false on second apply")
		}
		if id2 != id {
			t.Errorf("id mismatch: first=%s second=%s", id, id2)
		}

		got2, err := p.GetComputerPrestageV3(ctx, id)
		if err != nil {
			t.Fatalf("get after update: %v", err)
		}
		if got2.VersionLock <= got.VersionLock {
			t.Errorf("versionLock did not advance: was %d, now %d", got.VersionLock, got2.VersionLock)
		}
		t.Logf("after update: versionLock=%d", got2.VersionLock)

		// 6. Delete + verify 404
		if err := p.DeleteComputerPrestageV3(ctx, id); err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, err = p.ResolveComputerPrestageV3IDByName(ctx, name)
		if err == nil {
			t.Fatal("expected 404 after delete")
		}
	})

	t.Run("MobileDevicePrestageV3", func(t *testing.T) {
		name := "sdk-acc-apply-mobprestage-" + runSuffix()

		strPtr := func(s string) *string { return &s }
		boolPtr := func(b bool) *bool { return &b }
		req := &pro.MobileDevicePrestageV3{
			DisplayName:                            name,
			DeviceEnrollmentProgramInstanceID:      depID,
			DefaultPrestage:                        false,
			Mandatory:                              true,
			MDMRemovable:                           true,
			AutoAdvanceSetup:                       false,
			AllowPairing:                           true,
			MultiUser:                              false,
			Supervised:                             true,
			MaximumSharedAccounts:                  10,
			ConfigureDeviceBeforeSetupAssistant:    false,
			RtsConfigProfileID:                     strPtr("-1"),
			Timezone:                               "UTC",
			SendTimezone:                           false,
			StorageQuotaSizeMegabytes:              1024,
			UseStorageQuotaSize:                    false,
			SkipSetupItems:                         &map[string]bool{},
			EnrollmentSiteID:                       "-1",
			EnrollmentCustomizationID:              strPtr("0"),
			PrestageMinimumOsTargetVersionTypeIos:  strPtr("NO_ENFORCEMENT"),
			PrestageMinimumOsTargetVersionTypeIpad: strPtr("NO_ENFORCEMENT"),
			AnchorCertificates:                     &[]string{},
			Names: &pro.MobileDevicePrestageNamesV3{
				AssignNamesUsing:       strPtr("Default Names"),
				DeviceNamePrefix:       strPtr(""),
				DeviceNameSuffix:       strPtr(""),
				DeviceNamingConfigured: boolPtr(false),
				ManageNames:            boolPtr(false),
				PrestageDeviceNames:    &[]pro.MobileDevicePrestageNameV3{},
				SingleDeviceName:       strPtr(""),
			},
			LocationInformation: pro.LocationInformationV3{
				ID:           "-1",
				VersionLock:  0,
				DepartmentID: "-1",
				BuildingID:   "-1",
			},
			PurchasingInformation: pro.PrestagePurchasingInformationV3{
				ID:           "-1",
				VersionLock:  0,
				Purchased:    true,
				LeaseDate:    "1970-01-01",
				PoDate:       "1970-01-01",
				WarrantyDate: "1970-01-01",
			},
		}

		// 4. Apply creates
		id, created, err := p.ApplyMobileDevicePrestageV3(ctx, req)
		if err != nil {
			t.Fatalf("apply create: %v", err)
		}
		cleanupDelete(t, "MobileDevicePrestage "+id, func() error { return p.DeleteMobileDevicePrestageV3(ctx, id) })
		if !created {
			t.Error("expected created = true on first apply")
		}
		t.Logf("created mobile device prestage id=%s", id)

		got, err := p.GetMobileDevicePrestageV3(ctx, id)
		if err != nil {
			t.Fatalf("get after create: %v", err)
		}
		t.Logf("after create: versionLock=%d", got.VersionLock)

		// 5. Apply updates — versionLock injection exercised
		req.Mandatory = false // flip from true to force a real update
		id2, created2, err := p.ApplyMobileDevicePrestageV3(ctx, req)
		if err != nil {
			t.Fatalf("apply update: %v", err)
		}
		if created2 {
			t.Error("expected created = false on second apply")
		}
		if id2 != id {
			t.Errorf("id mismatch: first=%s second=%s", id, id2)
		}

		got2, err := p.GetMobileDevicePrestageV3(ctx, id)
		if err != nil {
			t.Fatalf("get after update: %v", err)
		}
		if got2.VersionLock <= got.VersionLock {
			t.Errorf("versionLock did not advance: was %d, now %d", got.VersionLock, got2.VersionLock)
		}
		t.Logf("after update: versionLock=%d", got2.VersionLock)

		// 6. Delete + verify 404
		if err := p.DeleteMobileDevicePrestageV3(ctx, id); err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, err = p.ResolveMobileDevicePrestageV3IDByName(ctx, name)
		if err == nil {
			t.Fatal("expected 404 after delete")
		}
	})
}

// ---------- DeviceEnrollmentV1 (token-upload Apply) ----------

func TestAcceptance_ApplyDeviceEnrollmentV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	token := strings.Join(strings.Fields(os.Getenv("JAMFPLATFORM_DEP_TOKEN")), "")
	if token == "" {
		t.Skip("JAMFPLATFORM_DEP_TOKEN not set — DEP-token-dependent test skipped")
	}

	name := "sdk-acc-apply-dep-" + runSuffix()
	supID := "-1"

	// 1. Apply creates (uploads token + sets metadata name)
	id, created, err := p.ApplyDeviceEnrollmentV1(ctx, &pro.DeviceEnrollmentInstance{
		Name:                  name,
		SupervisionIdentityID: &supID,
	}, token)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "DeviceEnrollment "+id, func() error { return p.DeleteDeviceEnrollmentV1(ctx, id) })
	if !created {
		t.Error("expected created = true on first apply")
	}
	t.Logf("created DEP enrollment id=%s", id)

	// Verify the name was set via metadata update.
	got, err := p.GetDeviceEnrollmentV1(ctx, id)
	if err != nil {
		t.Fatalf("get after create: %v", err)
	}
	if got.Name != name {
		t.Errorf("name = %q, want %q", got.Name, name)
	}

	// 2. Apply updates (token empty = skip re-upload, just metadata update)
	// Keep the same name (it's the resolver key) but change supervisionIdentityId.
	newSupID := "-1"
	id2, created2, err := p.ApplyDeviceEnrollmentV1(ctx, &pro.DeviceEnrollmentInstance{
		Name:                  name,
		SupervisionIdentityID: &newSupID,
	}, "")
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on second apply")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	// Verify the name is still set correctly.
	got2, err := p.GetDeviceEnrollmentV1(ctx, id)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got2.Name != name {
		t.Errorf("name = %q, want %q", got2.Name, name)
	}
	t.Logf("updated DEP enrollment id=%s, name=%q", id, got2.Name)

	// 3. Delete
	if err := p.DeleteDeviceEnrollmentV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 4. Resolve not found
	_, err = p.ResolveDeviceEnrollmentV1IDByName(ctx, name)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
	if apiErr := jamfplatform.AsAPIError(err); apiErr == nil || !apiErr.HasStatus(404) {
		t.Fatalf("expected 404, got: %v", err)
	}
}

// ---------- AppRequestFormInputFieldV1 ----------

func TestAcceptance_ApplyAppRequestFormInputFieldV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	title := "sdk-acc-apply-appfield-" + runSuffix()

	// 1. Apply creates
	id, created, err := p.ApplyAppRequestFormInputFieldV1(ctx, &pro.AppRequestFormInputField{
		Title:    title,
		Priority: 1,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "AppRequestFormInputField "+id, func() error { return p.DeleteAppRequestFormInputFieldV1(ctx, id) })
	if !created {
		t.Error("expected created = true on first apply")
	}
	t.Logf("created app-request field id=%s", id)

	// 2. Apply updates
	desc := "updated description"
	id2, created2, err := p.ApplyAppRequestFormInputFieldV1(ctx, &pro.AppRequestFormInputField{
		Title:       title,
		Priority:    2,
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on second apply")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	// 3. Delete
	if err := p.DeleteAppRequestFormInputFieldV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 4. Resolve not found
	_, err = p.ResolveAppRequestFormInputFieldV1IDByName(ctx, title)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
	if apiErr := jamfplatform.AsAPIError(err); apiErr == nil || !apiErr.HasStatus(404) {
		t.Fatalf("expected 404, got: %v", err)
	}
}

// ---------- StaticMobileDeviceGroupV1 ----------

// TestAcceptance_ApplyStaticMobileDeviceGroupV1 verifies the Apply method's
// membership pre-fetch behaviour: on update, the method fetches current group
// membership and re-sends all members as selected=true so the PATCH does not
// wipe devices that were added outside of the Apply call.
func TestAcceptance_ApplyStaticMobileDeviceGroupV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)
	pc := proclassic.New(c)

	suffix := runSuffix()
	groupName := "sdk-acc-apply-static-mdm-group-" + suffix

	// Create 3 dummy mobile device records via Classic API.
	createDevice := func(n int) string {
		sn := fmt.Sprintf("SDKACC%s%d", strings.ToUpper(suffix), n)
		name := fmt.Sprintf("sdk-acc-device-%s-%d", suffix, n)
		managed := true
		dev, err := pc.CreateMobileDeviceByID(ctx, "0", &proclassic.MobileDevicePost{
			General: &proclassic.MobileDevicePostGeneral{
				Name:         &name,
				SerialNumber: &sn,
				UDID:         &sn,
				Managed:      &managed,
			},
		})
		if err != nil {
			t.Fatalf("create device %d: %v", n, err)
		}
		if dev.ID == nil {
			t.Fatalf("create device %d: missing ID in response", n)
		}
		return strconv.Itoa(*dev.ID)
	}

	dev1ID := createDevice(1)
	dev2ID := createDevice(2)
	dev3ID := createDevice(3)
	t.Logf("dummy devices: %s, %s, %s", dev1ID, dev2ID, dev3ID)

	t.Cleanup(func() {
		for _, id := range []string{dev1ID, dev2ID, dev3ID} {
			if err := pc.DeleteMobileDeviceByID(ctx, id); err != nil {
				t.Logf("cleanup device %s: %v", id, err)
			}
		}
	})

	trueVal := true

	// 1. Apply creates group with device 1.
	siteID := "-1"
	groupID, created, err := p.ApplyStaticMobileDeviceGroupV1(ctx, &pro.StaticGroupAssignment{
		GroupName: groupName,
		SiteID:    &siteID,
		Assignments: &[]pro.Assignment{
			{MobileDeviceID: &dev1ID, Selected: &trueVal},
		},
	}, false)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	if !created {
		t.Error("expected created = true")
	}
	t.Logf("created group id=%s", groupID)

	t.Cleanup(func() {
		if err := p.DeleteStaticMobileDeviceGroupV1(ctx, groupID); err != nil {
			t.Logf("cleanup group %s: %v", groupID, err)
		}
	})

	// 2. Add devices 2 and 3 via direct PATCH (outside of Apply).
	_, err = p.PatchStaticMobileDeviceGroupV1(ctx, groupID, &pro.StaticGroupAssignment{
		GroupName: groupName,
		SiteID:    &siteID,
		Assignments: &[]pro.Assignment{
			{MobileDeviceID: &dev2ID, Selected: &trueVal},
			{MobileDeviceID: &dev3ID, Selected: &trueVal},
		},
	})
	if err != nil {
		t.Fatalf("patch to add devices 2+3: %v", err)
	}

	// 3. Apply update: the method must fetch current membership (all 3 devices)
	// and include them in the PATCH so none are removed.
	groupID2, created2, err := p.ApplyStaticMobileDeviceGroupV1(ctx, &pro.StaticGroupAssignment{
		GroupName: groupName,
		SiteID:    &siteID,
	}, false)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on update")
	}
	if groupID2 != groupID {
		t.Errorf("apply update: id=%s, want %s", groupID2, groupID)
	}

	// 4. Verify all 3 devices are still members after the Apply update.
	members, err := p.ListStaticMobileDeviceGroupMembershipV1(ctx, groupID, nil, "")
	if err != nil {
		t.Fatalf("list membership: %v", err)
	}
	memberIDs := make(map[string]bool, len(members))
	for _, m := range members {
		memberIDs[m.MobileDeviceID] = true
	}
	for _, wantID := range []string{dev1ID, dev2ID, dev3ID} {
		if !memberIDs[wantID] {
			t.Errorf("device %s missing from membership after apply update", wantID)
		}
	}
	t.Logf("membership after apply update: %v", memberIDs)
}

// ---------- AccountV1 ----------

// POST /v1/accounts requires ldapServerId=-1 + distinguishedName="" as
// sentinels for non-LDAP accounts — omit them and the server 500s. Same
// for phone + changePasswordOnNextLogin; schema marks them optional but
// the create handler derefs them unconditionally.
func TestAcceptance_ApplyAccountV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	uname := "sdk-acc-apply-user-" + runSuffix()
	realname := "SDK Apply Test " + runSuffix()
	email := uname + "@example.invalid"
	password := "SDKAccTestPwd!" + runSuffix()
	accessLevel := "FullAccess"
	privilegeLevel := "ADMINISTRATOR"
	accountType := "DEFAULT"
	accountStatus := "Enabled"
	phone := "000-000-0000"
	distinguishedName := ""
	siteID := -1
	ldapServerID := -1
	changePassword := false

	req := &pro.UserAccount{
		Username:                  &uname,
		Realname:                  &realname,
		Email:                     &email,
		Phone:                     &phone,
		PlainPassword:             &password,
		LdapServerID:              &ldapServerID,
		DistinguishedName:         &distinguishedName,
		SiteID:                    &siteID,
		AccessLevel:               &accessLevel,
		PrivilegeLevel:            &privilegeLevel,
		AccountStatus:             &accountStatus,
		AccountType:               &accountType,
		ChangePasswordOnNextLogin: &changePassword,
	}

	// 1. Apply creates
	id, created, err := p.ApplyAccountV1(ctx, req)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	cleanupDelete(t, "Account "+id, func() error { return p.DeleteAccountV1(ctx, id) })
	if !created {
		t.Error("expected created = true on first apply")
	}
	t.Logf("created account id=%s username=%s", id, uname)

	// 2. Apply updates — change realname to prove the PUT fires
	updatedRealname := "SDK Apply Updated " + runSuffix()
	req.Realname = &updatedRealname
	id2, created2, err := p.ApplyAccountV1(ctx, req)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if created2 {
		t.Error("expected created = false on second apply")
	}
	if id2 != id {
		t.Errorf("id changed: %s → %s", id, id2)
	}

	got, err := p.GetAccountV1(ctx, id)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Realname == nil || *got.Realname != updatedRealname {
		t.Errorf("realname = %v, want %q", got.Realname, updatedRealname)
	}

	// 3. Delete
	if err := p.DeleteAccountV1(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 4. Resolve not found
	_, err = p.ResolveAccountV1IDByName(ctx, uname)
	if err == nil {
		t.Fatal("expected 404 after delete")
	}
	if apiErr := jamfplatform.AsAPIError(err); apiErr == nil || !apiErr.HasStatus(404) {
		t.Fatalf("expected 404, got: %v", err)
	}
}
