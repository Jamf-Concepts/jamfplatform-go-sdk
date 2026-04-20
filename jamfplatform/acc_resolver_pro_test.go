// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// ─── Buildings ──────────────────────────────────────────────────────────────

func createTestBuilding(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateBuildingV1(ctx, &pro.Building{Name: name})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBuildingV1(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteBuildingV1", func() error { return c.DeleteBuildingV1(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveBuildingV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-bldg-id-" + runSuffix()
	wantID := createTestBuilding(t, c, name)

	gotID, err := c.ResolveBuildingV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveBuildingV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveBuildingV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveBuildingV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-bldg-typed-" + runSuffix()
	wantID := createTestBuilding(t, c, name)

	got, err := c.ResolveBuildingV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveBuildingV1ByName(%q): %v", name, err)
	}
	if got == nil || (got.ID != nil && *got.ID != wantID) {
		t.Errorf("result.ID = %v, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveBuildingV1ByName %q -> %v", name, got.ID)
}

func TestAcceptance_ResolveBuildingV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveBuildingV1IDByName(context.Background(), "sdk-does-not-exist-bldg-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveBuildingV1IDByName not-found surfaced 404 as expected")
}

// ─── Categories ─────────────────────────────────────────────────────────────

func createTestCategory(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateCategoryV1(ctx, &pro.Category{Name: name, Priority: 9})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateCategoryV1(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteCategoryV1", func() error { return c.DeleteCategoryV1(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveCategoryV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-cat-id-" + runSuffix()
	wantID := createTestCategory(t, c, name)

	gotID, err := c.ResolveCategoryV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveCategoryV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveCategoryV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveCategoryV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-cat-typed-" + runSuffix()
	wantID := createTestCategory(t, c, name)

	got, err := c.ResolveCategoryV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveCategoryV1ByName(%q): %v", name, err)
	}
	if got == nil || got.ID == nil || *got.ID != wantID {
		t.Errorf("result.ID = %v, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveCategoryV1ByName %q -> %s", name, *got.ID)
}

func TestAcceptance_ResolveCategoryV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveCategoryV1IDByName(context.Background(), "sdk-does-not-exist-cat-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveCategoryV1IDByName not-found surfaced 404 as expected")
}

// ─── Departments ─────────────────────────────────────────────────────────────

func createTestDepartment(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateDepartmentV1(ctx, &pro.Department{Name: name})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDepartmentV1(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteDepartmentV1", func() error { return c.DeleteDepartmentV1(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveDepartmentV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-dept-id-" + runSuffix()
	wantID := createTestDepartment(t, c, name)

	gotID, err := c.ResolveDepartmentV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDepartmentV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveDepartmentV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveDepartmentV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-dept-typed-" + runSuffix()
	wantID := createTestDepartment(t, c, name)

	got, err := c.ResolveDepartmentV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDepartmentV1ByName(%q): %v", name, err)
	}
	if got == nil || got.ID == nil || *got.ID != wantID {
		t.Errorf("result.ID = %v, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveDepartmentV1ByName %q -> %s", name, *got.ID)
}

func TestAcceptance_ResolveDepartmentV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveDepartmentV1IDByName(context.Background(), "sdk-does-not-exist-dept-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveDepartmentV1IDByName not-found surfaced 404 as expected")
}

// ─── Scripts ─────────────────────────────────────────────────────────────────

func createTestScript(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	contents := "#!/bin/bash\necho hello"
	resp, err := c.CreateScriptV1(ctx, &pro.Script{Name: name, ScriptContents: &contents})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateScriptV1(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteScriptV1", func() error { return c.DeleteScriptV1(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveScriptV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-script-id-" + runSuffix()
	wantID := createTestScript(t, c, name)

	gotID, err := c.ResolveScriptV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveScriptV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveScriptV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveScriptV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-script-typed-" + runSuffix()
	wantID := createTestScript(t, c, name)

	got, err := c.ResolveScriptV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveScriptV1ByName(%q): %v", name, err)
	}
	if got == nil || got.ID == nil || *got.ID != wantID {
		t.Errorf("result.ID = %v, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveScriptV1ByName %q -> %s", name, *got.ID)
}

func TestAcceptance_ResolveScriptV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveScriptV1IDByName(context.Background(), "sdk-does-not-exist-script-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveScriptV1IDByName not-found surfaced 404 as expected")
}

// ─── Packages ────────────────────────────────────────────────────────────────
// Create requires a binary file upload; positive tests are skipped.
// Probe-only: not-found path validates the resolver wiring end-to-end.

func TestAcceptance_ResolvePackageV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolvePackageV1IDByName(context.Background(), "sdk-does-not-exist-pkg-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolvePackageV1IDByName not-found surfaced 404 as expected")
}

// ─── Smart Computer Groups (V2) ───────────────────────────────────────────────

func createTestSmartComputerGroup(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateSmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{Name: name}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSmartComputerGroupV2(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteSmartComputerGroupV2", func() error { return c.DeleteSmartComputerGroupV2(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveSmartComputerGroupV2IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-scg-id-" + runSuffix()
	wantID := createTestSmartComputerGroup(t, c, name)

	gotID, err := c.ResolveSmartComputerGroupV2IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveSmartComputerGroupV2IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveSmartComputerGroupV2IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveSmartComputerGroupV2ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-scg-typed-" + runSuffix()
	wantID := createTestSmartComputerGroup(t, c, name)

	got, err := c.ResolveSmartComputerGroupV2ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveSmartComputerGroupV2ByName(%q): %v", name, err)
	}
	if got == nil || got.ID != wantID {
		t.Errorf("result.ID = %q, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveSmartComputerGroupV2ByName %q -> %s", name, got.ID)
}

func TestAcceptance_ResolveSmartComputerGroupV2_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveSmartComputerGroupV2IDByName(context.Background(), "sdk-does-not-exist-scg-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveSmartComputerGroupV2IDByName not-found surfaced 404 as expected")
}

// ─── Static Computer Groups (V2) ─────────────────────────────────────────────

func createTestStaticComputerGroup(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateStaticComputerGroupV2(ctx, &pro.StaticComputerGroupAssignment{Name: name}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateStaticComputerGroupV2(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteStaticComputerGroupV2", func() error { return c.DeleteStaticComputerGroupV2(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveStaticComputerGroupV2IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-stcg-id-" + runSuffix()
	wantID := createTestStaticComputerGroup(t, c, name)

	gotID, err := c.ResolveStaticComputerGroupV2IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveStaticComputerGroupV2IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveStaticComputerGroupV2IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveStaticComputerGroupV2ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-stcg-typed-" + runSuffix()
	wantID := createTestStaticComputerGroup(t, c, name)

	got, err := c.ResolveStaticComputerGroupV2ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveStaticComputerGroupV2ByName(%q): %v", name, err)
	}
	if got == nil || got.ID != wantID {
		t.Errorf("result.ID = %q, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveStaticComputerGroupV2ByName %q -> %s", name, got.ID)
}

func TestAcceptance_ResolveStaticComputerGroupV2_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveStaticComputerGroupV2IDByName(context.Background(), "sdk-does-not-exist-stcg-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveStaticComputerGroupV2IDByName not-found surfaced 404 as expected")
}

// ─── Smart Mobile Device Groups (V1) ─────────────────────────────────────────

func createTestSmartMobileDeviceGroup(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateSmartMobileDeviceGroupV1(ctx, &pro.SmartGroupAssignment{GroupName: name, SiteID: strPtr("-1")}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSmartMobileDeviceGroupV1(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteSmartMobileDeviceGroupV1", func() error { return c.DeleteSmartMobileDeviceGroupV1(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveSmartMobileDeviceGroupV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-smg-id-" + runSuffix()
	wantID := createTestSmartMobileDeviceGroup(t, c, name)

	gotID, err := c.ResolveSmartMobileDeviceGroupV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveSmartMobileDeviceGroupV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveSmartMobileDeviceGroupV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveSmartMobileDeviceGroupV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-smg-typed-" + runSuffix()
	wantID := createTestSmartMobileDeviceGroup(t, c, name)

	got, err := c.ResolveSmartMobileDeviceGroupV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveSmartMobileDeviceGroupV1ByName(%q): %v", name, err)
	}
	if got == nil || got.GroupID != wantID {
		t.Errorf("result.GroupID = %q, want %q", got.GroupID, wantID)
	}
	if got.GroupName != name {
		t.Errorf("result.GroupName = %q, want %q", got.GroupName, name)
	}
	t.Logf("ResolveSmartMobileDeviceGroupV1ByName %q -> %s", name, got.GroupID)
}

func TestAcceptance_ResolveSmartMobileDeviceGroupV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveSmartMobileDeviceGroupV1IDByName(context.Background(), "sdk-does-not-exist-smg-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveSmartMobileDeviceGroupV1IDByName not-found surfaced 404 as expected")
}

// ─── Static Mobile Device Groups (V1) ────────────────────────────────────────

func createTestStaticMobileDeviceGroup(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateStaticMobileDeviceGroupV1(ctx, &pro.StaticGroupAssignment{GroupName: name, SiteID: strPtr("-1")}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateStaticMobileDeviceGroupV1(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteStaticMobileDeviceGroupV1", func() error { return c.DeleteStaticMobileDeviceGroupV1(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveStaticMobileDeviceGroupV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-stmg-id-" + runSuffix()
	wantID := createTestStaticMobileDeviceGroup(t, c, name)

	gotID, err := c.ResolveStaticMobileDeviceGroupV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveStaticMobileDeviceGroupV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveStaticMobileDeviceGroupV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveStaticMobileDeviceGroupV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-stmg-typed-" + runSuffix()
	wantID := createTestStaticMobileDeviceGroup(t, c, name)

	got, err := c.ResolveStaticMobileDeviceGroupV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveStaticMobileDeviceGroupV1ByName(%q): %v", name, err)
	}
	if got == nil || got.GroupID != wantID {
		t.Errorf("result.GroupID = %q, want %q", got.GroupID, wantID)
	}
	if got.GroupName != name {
		t.Errorf("result.GroupName = %q, want %q", got.GroupName, name)
	}
	t.Logf("ResolveStaticMobileDeviceGroupV1ByName %q -> %s", name, got.GroupID)
}

func TestAcceptance_ResolveStaticMobileDeviceGroupV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveStaticMobileDeviceGroupV1IDByName(context.Background(), "sdk-does-not-exist-stmg-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveStaticMobileDeviceGroupV1IDByName not-found surfaced 404 as expected")
}

// ─── Computer Extension Attributes (V1) ──────────────────────────────────────

func createTestComputerExtAttr(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	dataType := "STRING"
	inputType := "TEXT"
	inventoryDisplay := "GENERAL"
	manageExistingData := "DELETE_EXISTING_DATA"
	resp, err := c.CreateComputerExtensionAttributeV1(ctx, &pro.ComputerExtensionAttributes{
		Name:                 name,
		Enabled:              true, // server rejects false/omitted unless inputType is SCRIPT
		DataType:             dataType,
		InputType:            inputType,
		InventoryDisplayType: inventoryDisplay,
		ManageExistingData:   manageExistingData,
		PopupMenuChoices:     []string{}, // server NPEs when field is null even for TEXT type
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateComputerExtensionAttributeV1(%q): %v", name, err)
	}
	ids := []string{resp.ID}
	cleanupDelete(t, "DeleteMultipleComputerExtensionAttributesV1", func() error {
		return c.DeleteMultipleComputerExtensionAttributesV1(ctx, &pro.Ids{IDs: &ids})
	})
	return resp.ID
}

func TestAcceptance_ResolveComputerExtensionAttributeV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-cea-id-" + runSuffix()
	wantID := createTestComputerExtAttr(t, c, name)

	gotID, err := c.ResolveComputerExtensionAttributeV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveComputerExtensionAttributeV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveComputerExtensionAttributeV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveComputerExtensionAttributeV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-cea-typed-" + runSuffix()
	wantID := createTestComputerExtAttr(t, c, name)

	got, err := c.ResolveComputerExtensionAttributeV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveComputerExtensionAttributeV1ByName(%q): %v", name, err)
	}
	if got == nil || got.ID != wantID {
		t.Errorf("result.ID = %q, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveComputerExtensionAttributeV1ByName %q -> %s", name, got.ID)
}

func TestAcceptance_ResolveComputerExtensionAttributeV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveComputerExtensionAttributeV1IDByName(context.Background(), "sdk-does-not-exist-cea-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveComputerExtensionAttributeV1IDByName not-found surfaced 404 as expected")
}

// ─── Mobile Device Extension Attributes (V1) ─────────────────────────────────

func createTestMobileDeviceExtAttr(t *testing.T, c *pro.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := c.CreateMobileDeviceExtensionAttributeV1(ctx, &pro.MobileDeviceExtensionAttributes{
		Name:                 name,
		DataType:             "STRING",
		InputType:            "TEXT",
		InventoryDisplayType: "GENERAL",
		PopupMenuChoices:     []string{}, // server NPEs when field is null even for TEXT type
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceExtensionAttributeV1(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteMobileDeviceExtensionAttributeV1", func() error {
		return c.DeleteMobileDeviceExtensionAttributeV1(ctx, resp.ID)
	})
	return resp.ID
}

func TestAcceptance_ResolveMobileDeviceExtensionAttributeV1IDByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-mdea-id-" + runSuffix()
	wantID := createTestMobileDeviceExtAttr(t, c, name)

	gotID, err := c.ResolveMobileDeviceExtensionAttributeV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceExtensionAttributeV1IDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
	t.Logf("ResolveMobileDeviceExtensionAttributeV1IDByName %q -> %s", name, gotID)
}

func TestAcceptance_ResolveMobileDeviceExtensionAttributeV1ByName(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-mdea-typed-" + runSuffix()
	wantID := createTestMobileDeviceExtAttr(t, c, name)

	got, err := c.ResolveMobileDeviceExtensionAttributeV1ByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceExtensionAttributeV1ByName(%q): %v", name, err)
	}
	if got == nil || got.ID != wantID {
		t.Errorf("result.ID = %q, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("result.Name = %q, want %q", got.Name, name)
	}
	t.Logf("ResolveMobileDeviceExtensionAttributeV1ByName %q -> %s", name, got.ID)
}

func TestAcceptance_ResolveMobileDeviceExtensionAttributeV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveMobileDeviceExtensionAttributeV1IDByName(context.Background(), "sdk-does-not-exist-mdea-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveMobileDeviceExtensionAttributeV1IDByName not-found surfaced 404 as expected")
}

// ─── Platform Groups (V1) ─────────────────────────────────────────────────────
// Groups are synced from identity providers; no create endpoint.
// Probe-only: list groups and resolve first one if any exist.

func TestAcceptance_ResolveGroupV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveGroupV1IDByName(context.Background(), "sdk-does-not-exist-grp-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveGroupV1IDByName not-found surfaced 404 as expected")
}

func TestAcceptance_ResolveGroupV1IDByName_ExistingGroup(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()

	groups, err := c.ListGroupsV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListGroupsV1: %v", err)
	}
	if len(groups) == 0 {
		t.Skip("no platform groups on this tenant — skipping resolver round-trip")
	}
	first := groups[0]
	gotID, err := c.ResolveGroupV1IDByName(ctx, first.GroupName)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveGroupV1IDByName(%q): %v", first.GroupName, err)
	}
	if gotID != first.GroupPlatformID {
		t.Errorf("resolved id = %q, want %q", gotID, first.GroupPlatformID)
	}
	t.Logf("ResolveGroupV1IDByName %q -> %s", first.GroupName, gotID)
}

// ─── Computer Inventory (V3) ──────────────────────────────────────────────────
// Computers cannot be created via API. Probe-only: list first computer and
// resolve its name, or skip if the tenant has no computers.

func TestAcceptance_ResolveComputerInventoryV3_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveComputerInventoryV3IDByName(context.Background(), "sdk-does-not-exist-computer-"+runSuffix())
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %T: %v", err, err)
	}
	t.Logf("ResolveComputerInventoryV3IDByName not-found surfaced 404 as expected")
}

func TestAcceptance_ResolveComputerInventoryV3IDByName_ExistingComputer(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()

	section := []string{"GENERAL"}
	computers, err := c.ListComputersInventoryV3(ctx, section, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputersInventoryV3: %v", err)
	}
	if len(computers) == 0 {
		t.Skip("no computers enrolled on this tenant — skipping resolver round-trip")
	}
	first := computers[0]
	if first.General == nil || first.General.Name == "" {
		t.Skip("first computer has no general.name — skipping")
	}

	gotID, err := c.ResolveComputerInventoryV3IDByName(ctx, first.General.Name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveComputerInventoryV3IDByName(%q): %v", first.General.Name, err)
	}
	if gotID != first.ID {
		t.Errorf("resolved id = %q, want %q", gotID, first.ID)
	}
	t.Logf("ResolveComputerInventoryV3IDByName %q -> %s", first.General.Name, gotID)
}
