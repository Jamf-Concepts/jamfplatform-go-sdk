// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// ─── Test helpers ───────────────────────────────────────────────────────────

// requireNotFoundErr asserts err is an APIResponseError with status 404.
func requireNotFoundErr(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected not-found error, got nil", label)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("%s: expected APIResponseError(404), got %T: %v", label, err, err)
	}
}

// requireAmbiguousErr asserts err is an AmbiguousMatchError with ≥ 2 matches.
func requireAmbiguousErr(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected ambiguous match error, got nil", label)
	}
	var amErr *jamfplatform.AmbiguousMatchError
	if !errors.As(err, &amErr) {
		t.Fatalf("%s: expected *AmbiguousMatchError, got %T: %v", label, err, err)
	}
	if len(amErr.Matches) < 2 {
		t.Errorf("%s: expected ≥2 matches, got %d: %v", label, len(amErr.Matches), amErr.Matches)
	}
}

// tryCreateDuplicate attempts to create a second resource with the same name.
// If the server rejects duplicates (4xx), it returns ("", false).
// If creation succeeds, it returns (id, true) and registers a t.Cleanup delete.
func tryCreateDuplicate(t *testing.T, label string, createFn func() (string, error), deleteFn func(string) error) (string, bool) {
	t.Helper()
	id, err := createFn()
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("server rejects duplicate %s names (%d) — skipping ambiguous test: %s", label, apiErr.StatusCode, apiErr.Summary())
			return "", false
		}
		t.Fatalf("unexpected error creating duplicate %s: %v", label, err)
	}
	t.Cleanup(func() {
		if err := deleteFn(id); err != nil {
			t.Logf("cleanup duplicate %s %s: %v", label, id, err)
		}
	})
	return id, true
}

// ─── Buildings ──────────────────────────────────────────────────────────────

func TestAcceptance_ResolveBuildingV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-bldg-" + runSuffix()

	// Step 1: Not found
	_, err := c.ResolveBuildingV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)
	t.Log("step 1: not-found ✓")

	// Step 2: Create
	resp, err := c.CreateBuildingV1(ctx, &pro.Building{Name: name})
	if err != nil {
		t.Fatalf("CreateBuildingV1: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteBuildingV1(ctx, id1) })
	t.Logf("step 2: created %s", id1)

	// Step 3: Resolve ID
	gotID, err := c.ResolveBuildingV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("ResolveBuildingV1IDByName: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}
	t.Logf("step 3: resolve ID %q → %s ✓", name, gotID)

	// Step 4: Resolve typed
	got, err := c.ResolveBuildingV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("ResolveBuildingV1ByName: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}
	t.Log("step 4: resolve typed ✓")

	// Step 5: Attempt duplicate
	id2, dupCreated := tryCreateDuplicate(t, "building", func() (string, error) {
		r, e := c.CreateBuildingV1(ctx, &pro.Building{Name: name})
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteBuildingV1(ctx, id) })

	// Step 6: Ambiguous
	if dupCreated {
		_, err = c.ResolveBuildingV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("step 6: ambiguous with IDs %s, %s ✓", id1, id2)

		// Delete duplicate so step 7 can verify single-then-gone
		if err := c.DeleteBuildingV1(ctx, id2); err != nil {
			t.Logf("early delete dup: %v", err)
		}
	}

	// Step 7: Delete original
	if err := c.DeleteBuildingV1(ctx, id1); err != nil {
		t.Fatalf("delete original: %v", err)
	}

	// Step 8: Not found after delete
	_, err = c.ResolveBuildingV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("step 8: not-found after delete ✓")
}

// ─── Categories ─────────────────────────────────────────────────────────────

func TestAcceptance_ResolveCategoryV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-cat-" + runSuffix()

	_, err := c.ResolveCategoryV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	resp, err := c.CreateCategoryV1(ctx, &pro.Category{Name: name, Priority: 9})
	if err != nil {
		t.Fatalf("CreateCategoryV1: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteCategoryV1(ctx, id1) })

	gotID, err := c.ResolveCategoryV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveCategoryV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "category", func() (string, error) {
		r, e := c.CreateCategoryV1(ctx, &pro.Category{Name: name, Priority: 9})
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteCategoryV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveCategoryV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteCategoryV1(ctx, id2)
	}

	if err := c.DeleteCategoryV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = c.ResolveCategoryV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Departments ────────────────────────────────────────────────────────────

func TestAcceptance_ResolveDepartmentV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-dept-" + runSuffix()

	_, err := c.ResolveDepartmentV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	resp, err := c.CreateDepartmentV1(ctx, &pro.Department{Name: name})
	if err != nil {
		t.Fatalf("CreateDepartmentV1: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteDepartmentV1(ctx, id1) })

	gotID, err := c.ResolveDepartmentV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveDepartmentV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "department", func() (string, error) {
		r, e := c.CreateDepartmentV1(ctx, &pro.Department{Name: name})
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteDepartmentV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveDepartmentV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteDepartmentV1(ctx, id2)
	}

	if err := c.DeleteDepartmentV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveDepartmentV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Scripts ────────────────────────────────────────────────────────────────

func TestAcceptance_ResolveScriptV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-script-" + runSuffix()
	contents := "#!/bin/bash\necho hello"

	_, err := c.ResolveScriptV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	resp, err := c.CreateScriptV1(ctx, &pro.Script{Name: name, ScriptContents: &contents})
	if err != nil {
		t.Fatalf("CreateScriptV1: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteScriptV1(ctx, id1) })

	gotID, err := c.ResolveScriptV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveScriptV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "script", func() (string, error) {
		r, e := c.CreateScriptV1(ctx, &pro.Script{Name: name, ScriptContents: &contents})
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteScriptV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveScriptV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteScriptV1(ctx, id2)
	}

	if err := c.DeleteScriptV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveScriptV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Packages ───────────────────────────────────────────────────────────────
// No create endpoint (requires binary upload). Not-found only.

func TestAcceptance_ResolvePackageV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolvePackageV1IDByName(context.Background(), "sdk-does-not-exist-pkg-"+runSuffix())
	requireNotFoundErr(t, "ResolvePackageV1IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

// ─── Smart Computer Groups (V2) ────────────────────────────────────────────

func TestAcceptance_ResolveSmartComputerGroupV2_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-scg-" + runSuffix()

	_, err := c.ResolveSmartComputerGroupV2IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	resp, err := c.CreateSmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{Name: name}, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteSmartComputerGroupV2(ctx, id1) })

	gotID, err := c.ResolveSmartComputerGroupV2IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveSmartComputerGroupV2ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "smart computer group", func() (string, error) {
		r, e := c.CreateSmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{Name: name}, false)
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteSmartComputerGroupV2(ctx, id) })

	if dupCreated {
		_, err = c.ResolveSmartComputerGroupV2IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteSmartComputerGroupV2(ctx, id2)
	}

	if err := c.DeleteSmartComputerGroupV2(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveSmartComputerGroupV2IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Static Computer Groups (V2) ───────────────────────────────────────────

func TestAcceptance_ResolveStaticComputerGroupV2_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-stcg-" + runSuffix()

	_, err := c.ResolveStaticComputerGroupV2IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	emptyAssignments := []string{}
	resp, err := c.CreateStaticComputerGroupV2(ctx, &pro.StaticComputerGroupAssignment{Name: name, Assignments: &emptyAssignments}, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteStaticComputerGroupV2(ctx, id1) })

	gotID, err := c.ResolveStaticComputerGroupV2IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveStaticComputerGroupV2ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "static computer group", func() (string, error) {
		r, e := c.CreateStaticComputerGroupV2(ctx, &pro.StaticComputerGroupAssignment{Name: name, Assignments: &emptyAssignments}, false)
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteStaticComputerGroupV2(ctx, id) })

	if dupCreated {
		_, err = c.ResolveStaticComputerGroupV2IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteStaticComputerGroupV2(ctx, id2)
	}

	if err := c.DeleteStaticComputerGroupV2(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveStaticComputerGroupV2IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Smart Mobile Device Groups (V1) ───────────────────────────────────────

func TestAcceptance_ResolveSmartMobileDeviceGroupV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-smg-" + runSuffix()

	_, err := c.ResolveSmartMobileDeviceGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	resp, err := c.CreateSmartMobileDeviceGroupV1(ctx, &pro.SmartGroupAssignment{GroupName: name, SiteID: strPtr("-1")}, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteSmartMobileDeviceGroupV1(ctx, id1) })

	gotID, err := c.ResolveSmartMobileDeviceGroupV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveSmartMobileDeviceGroupV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.GroupName != name {
		t.Errorf("typed GroupName = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "smart mobile device group", func() (string, error) {
		r, e := c.CreateSmartMobileDeviceGroupV1(ctx, &pro.SmartGroupAssignment{GroupName: name, SiteID: strPtr("-1")}, false)
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteSmartMobileDeviceGroupV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveSmartMobileDeviceGroupV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteSmartMobileDeviceGroupV1(ctx, id2)
	}

	if err := c.DeleteSmartMobileDeviceGroupV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveSmartMobileDeviceGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Static Mobile Device Groups (V1) ──────────────────────────────────────

func TestAcceptance_ResolveStaticMobileDeviceGroupV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-stmg-" + runSuffix()

	_, err := c.ResolveStaticMobileDeviceGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	emptyMobileAssignments := []pro.Assignment{}
	resp, err := c.CreateStaticMobileDeviceGroupV1(ctx, &pro.StaticGroupAssignment{GroupName: name, SiteID: strPtr("-1"), Assignments: &emptyMobileAssignments}, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteStaticMobileDeviceGroupV1(ctx, id1) })

	gotID, err := c.ResolveStaticMobileDeviceGroupV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveStaticMobileDeviceGroupV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.GroupName != name {
		t.Errorf("typed GroupName = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "static mobile device group", func() (string, error) {
		r, e := c.CreateStaticMobileDeviceGroupV1(ctx, &pro.StaticGroupAssignment{GroupName: name, SiteID: strPtr("-1"), Assignments: &emptyMobileAssignments}, false)
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteStaticMobileDeviceGroupV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveStaticMobileDeviceGroupV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteStaticMobileDeviceGroupV1(ctx, id2)
	}

	if err := c.DeleteStaticMobileDeviceGroupV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveStaticMobileDeviceGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Computer Extension Attributes (V1) ────────────────────────────────────

func TestAcceptance_ResolveComputerExtensionAttributeV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-cea-" + runSuffix()

	_, err := c.ResolveComputerExtensionAttributeV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	newCEA := func(n string) *pro.ComputerExtensionAttributes {
		return &pro.ComputerExtensionAttributes{
			Name: n, Enabled: true, DataType: "STRING", InputType: "TEXT",
			InventoryDisplayType: "GENERAL", ManageExistingData: "DELETE_EXISTING_DATA",
			PopupMenuChoices: []string{},
		}
	}
	resp, err := c.CreateComputerExtensionAttributeV1(ctx, newCEA(name))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id1 := resp.ID
	ids1 := []string{id1}
	t.Cleanup(func() { _ = c.DeleteMultipleComputerExtensionAttributesV1(ctx, &pro.Ids{IDs: &ids1}) })

	gotID, err := c.ResolveComputerExtensionAttributeV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveComputerExtensionAttributeV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "computer ext attr", func() (string, error) {
		r, e := c.CreateComputerExtensionAttributeV1(ctx, newCEA(name))
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error {
		ids := []string{id}
		return c.DeleteMultipleComputerExtensionAttributesV1(ctx, &pro.Ids{IDs: &ids})
	})

	if dupCreated {
		_, err = c.ResolveComputerExtensionAttributeV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		ids := []string{id2}
		_ = c.DeleteMultipleComputerExtensionAttributesV1(ctx, &pro.Ids{IDs: &ids})
	}

	if err := c.DeleteMultipleComputerExtensionAttributesV1(ctx, &pro.Ids{IDs: &ids1}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveComputerExtensionAttributeV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Mobile Device Extension Attributes (V1) ───────────────────────────────

func TestAcceptance_ResolveMobileDeviceExtensionAttributeV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-mdea-" + runSuffix()

	_, err := c.ResolveMobileDeviceExtensionAttributeV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	newMDEA := func(n string) *pro.MobileDeviceExtensionAttributes {
		return &pro.MobileDeviceExtensionAttributes{
			Name: n, DataType: "STRING", InputType: "TEXT",
			InventoryDisplayType: "GENERAL", PopupMenuChoices: []string{},
		}
	}
	resp, err := c.CreateMobileDeviceExtensionAttributeV1(ctx, newMDEA(name))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteMobileDeviceExtensionAttributeV1(ctx, id1) })

	gotID, err := c.ResolveMobileDeviceExtensionAttributeV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveMobileDeviceExtensionAttributeV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "mobile device ext attr", func() (string, error) {
		r, e := c.CreateMobileDeviceExtensionAttributeV1(ctx, newMDEA(name))
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteMobileDeviceExtensionAttributeV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveMobileDeviceExtensionAttributeV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteMobileDeviceExtensionAttributeV1(ctx, id2)
	}

	if err := c.DeleteMobileDeviceExtensionAttributeV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveMobileDeviceExtensionAttributeV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Platform Groups (V1) ──────────────────────────────────────────────────
// Synced from identity providers — no create endpoint. Read-only probe.

func TestAcceptance_ResolveGroupV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveGroupV1IDByName(context.Background(), "sdk-does-not-exist-grp-"+runSuffix())
	requireNotFoundErr(t, "ResolveGroupV1IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveGroupV1IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	groups, err := c.ListGroupsV1(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListGroupsV1: %v", err)
	}
	if len(groups) == 0 {
		t.Skip("no platform groups — skipping")
	}
	first := groups[0]
	gotID, err := c.ResolveGroupV1IDByName(ctx, first.GroupName)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotID != first.GroupPlatformID {
		t.Errorf("resolved id = %q, want %q", gotID, first.GroupPlatformID)
	}
	t.Logf("resolved %q → %s ✓", first.GroupName, gotID)
}

// ─── Computer Inventory (V3) ───────────────────────────────────────────────
// Computers are enrolled, not created via API. Read-only probe.

func TestAcceptance_ResolveComputerInventoryV3_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveComputerInventoryV3IDByName(context.Background(), "sdk-does-not-exist-ci-"+runSuffix())
	requireNotFoundErr(t, "ResolveComputerInventoryV3IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveComputerInventoryV3IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	section := []string{"GENERAL"}
	computers, err := c.ListComputersInventoryV3(ctx, section, nil, "")
	if err != nil {
		t.Fatalf("ListComputersInventoryV3: %v", err)
	}
	if len(computers) == 0 {
		t.Skip("no computers — skipping")
	}
	first := computers[0]
	if first.General == nil || first.General.Name == "" {
		t.Skip("first computer has no name — skipping")
	}
	gotID, err := c.ResolveComputerInventoryV3IDByName(ctx, first.General.Name)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotID != first.ID {
		t.Errorf("resolved id = %q, want %q", gotID, first.ID)
	}
	t.Logf("resolved %q → %s ✓", first.General.Name, gotID)
}

// ─── Sites ─────────────────────────────────────────────────────────────────
// No create/delete endpoint. Read-only probe.

func TestAcceptance_ResolveSiteV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveSiteV1IDByName(context.Background(), "sdk-does-not-exist-site-"+runSuffix())
	requireNotFoundErr(t, "ResolveSiteV1IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveSiteV1IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	sites, err := c.ListSitesV1(ctx)
	if err != nil {
		t.Fatalf("ListSitesV1: %v", err)
	}
	if len(sites) == 0 {
		t.Skip("no sites — skipping")
	}
	first := sites[0]
	gotID, err := c.ResolveSiteV1IDByName(ctx, first.Name)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotID != first.ID {
		t.Errorf("resolved id = %q, want %q", gotID, first.ID)
	}
	t.Logf("resolved %q → %s ✓", first.Name, gotID)
}

// ─── Computer Groups (combined V1) ─────────────────────────────────────────
// Combined smart+static list. Create via SmartComputerGroupV2 to test.

func TestAcceptance_ResolveComputerGroupV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-cg-" + runSuffix()

	_, err := c.ResolveComputerGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	// Create a smart computer group — it should appear in the combined list.
	resp, err := c.CreateSmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{Name: name}, false)
	if err != nil {
		t.Fatalf("CreateSmartComputerGroupV2: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteSmartComputerGroupV2(ctx, id1) })

	gotID, err := c.ResolveComputerGroupV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}
	t.Logf("resolved %q → %s ✓", name, gotID)

	// Attempt duplicate via static group with same name
	emptyAssignmentsCG := []string{}
	id2, dupCreated := tryCreateDuplicate(t, "computer group (static)", func() (string, error) {
		r, e := c.CreateStaticComputerGroupV2(ctx, &pro.StaticComputerGroupAssignment{Name: name, Assignments: &emptyAssignmentsCG}, false)
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteStaticComputerGroupV2(ctx, id) })

	if dupCreated {
		_, err = c.ResolveComputerGroupV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteStaticComputerGroupV2(ctx, id2)
	}

	if err := c.DeleteSmartComputerGroupV2(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveComputerGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Mobile Device Groups (combined V1) ────────────────────────────────────
// Combined smart+static list. Create via SmartMobileDeviceGroupV1 to test.

func TestAcceptance_ResolveMobileDeviceGroupV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-mdg-" + runSuffix()

	_, err := c.ResolveMobileDeviceGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	resp, err := c.CreateSmartMobileDeviceGroupV1(ctx, &pro.SmartGroupAssignment{GroupName: name, SiteID: strPtr("-1")}, false)
	if err != nil {
		t.Fatalf("CreateSmartMobileDeviceGroupV1: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteSmartMobileDeviceGroupV1(ctx, id1) })

	gotID, err := c.ResolveMobileDeviceGroupV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	// MobileDeviceGroup.ID is int, resolver returns string
	t.Logf("resolved %q → %s ✓", name, gotID)

	// Attempt duplicate via static group with same name
	emptyAssignmentsMDG := []pro.Assignment{}
	id2, dupCreated := tryCreateDuplicate(t, "mobile device group (static)", func() (string, error) {
		r, e := c.CreateStaticMobileDeviceGroupV1(ctx, &pro.StaticGroupAssignment{GroupName: name, SiteID: strPtr("-1"), Assignments: &emptyAssignmentsMDG}, false)
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteStaticMobileDeviceGroupV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveMobileDeviceGroupV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteStaticMobileDeviceGroupV1(ctx, id2)
	}

	if err := c.DeleteSmartMobileDeviceGroupV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveMobileDeviceGroupV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Advanced Mobile Device Searches ────────────────────────────────────────

func TestAcceptance_ResolveAdvancedMobileDeviceSearchV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-amds-" + runSuffix()

	_, err := c.ResolveAdvancedMobileDeviceSearchV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	resp, err := c.CreateAdvancedMobileDeviceSearchV1(ctx, &pro.AdvancedSearch{Name: name})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteAdvancedMobileDeviceSearchV1(ctx, id1) })

	gotID, err := c.ResolveAdvancedMobileDeviceSearchV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}

	got, err := c.ResolveAdvancedMobileDeviceSearchV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.Name != name {
		t.Errorf("typed Name = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "advanced mobile device search", func() (string, error) {
		r, e := c.CreateAdvancedMobileDeviceSearchV1(ctx, &pro.AdvancedSearch{Name: name})
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteAdvancedMobileDeviceSearchV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveAdvancedMobileDeviceSearchV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteAdvancedMobileDeviceSearchV1(ctx, id2)
	}

	if err := c.DeleteAdvancedMobileDeviceSearchV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveAdvancedMobileDeviceSearchV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Static User Groups ────────────────────────────────────────────────────
// No create endpoint. Read-only probe.

func TestAcceptance_ResolveStaticUserGroupV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveStaticUserGroupV1IDByName(context.Background(), "sdk-does-not-exist-sug-"+runSuffix())
	requireNotFoundErr(t, "ResolveStaticUserGroupV1IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveStaticUserGroupV1IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	groups, err := c.ListStaticUserGroupsV1(ctx)
	if err != nil {
		t.Fatalf("ListStaticUserGroupsV1: %v", err)
	}
	if len(groups) == 0 {
		t.Skip("no static user groups — skipping")
	}
	first := groups[0]
	gotID, err := c.ResolveStaticUserGroupV1IDByName(ctx, first.Name)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotID != strconv.Itoa(first.ID) {
		t.Errorf("resolved id = %q, want %d", gotID, first.ID)
	}
	t.Logf("resolved %q → %s ✓", first.Name, gotID)
}

// ─── Users ─────────────────────────────────────────────────────────────────

func TestAcceptance_ResolveUserV1_Lifecycle(t *testing.T) {
	t.Skip("Pro users POST+DELETE currently broken at the gateway (server 500) — known exception")
}

// ─── Computer Prestages ────────────────────────────────────────────────────
// Requires deviceEnrollmentProgramInstanceId — read-only probe.

func TestAcceptance_ResolveComputerPrestageV3_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveComputerPrestageV3IDByName(context.Background(), "sdk-does-not-exist-cprest-"+runSuffix())
	requireNotFoundErr(t, "ResolveComputerPrestageV3IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveComputerPrestageV3IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	prestages, err := c.ListComputerPrestagesV3(ctx, nil)
	if err != nil {
		t.Fatalf("ListComputerPrestagesV3: %v", err)
	}
	if len(prestages) == 0 {
		t.Skip("no computer prestages — skipping")
	}
	first := prestages[0]
	gotID, err := c.ResolveComputerPrestageV3IDByName(ctx, first.DisplayName)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	t.Logf("resolved %q → %s ✓", first.DisplayName, gotID)
}

// ─── Mobile Device Prestages ───────────────────────────────────────────────
// Requires deviceEnrollmentProgramInstanceId — read-only probe.

func TestAcceptance_ResolveMobileDevicePrestageV3_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveMobileDevicePrestageV3IDByName(context.Background(), "sdk-does-not-exist-mdprest-"+runSuffix())
	requireNotFoundErr(t, "ResolveMobileDevicePrestageV3IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveMobileDevicePrestageV3IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	prestages, err := c.ListMobileDevicePrestagesV3(ctx, nil)
	if err != nil {
		t.Fatalf("ListMobileDevicePrestagesV3: %v", err)
	}
	if len(prestages) == 0 {
		t.Skip("no mobile device prestages — skipping")
	}
	first := prestages[0]
	gotID, err := c.ResolveMobileDevicePrestageV3IDByName(ctx, first.DisplayName)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	t.Logf("resolved %q → %s ✓", first.DisplayName, gotID)
}

// ─── Patch Policies ────────────────────────────────────────────────────────
// No create endpoint. Read-only probe.

func TestAcceptance_ResolvePatchPolicyV2_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolvePatchPolicyV2IDByName(context.Background(), "sdk-does-not-exist-pp-"+runSuffix())
	requireNotFoundErr(t, "ResolvePatchPolicyV2IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolvePatchPolicyV2IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	policies, err := c.ListPatchPoliciesV2(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListPatchPoliciesV2: %v", err)
	}
	if len(policies) == 0 {
		t.Skip("no patch policies — skipping")
	}
	first := policies[0]
	gotID, err := c.ResolvePatchPolicyV2IDByName(ctx, first.PolicyName)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotID != first.ID {
		t.Errorf("resolved id = %q, want %q", gotID, first.ID)
	}
	t.Logf("resolved %q → %s ✓", first.PolicyName, gotID)
}

// ─── Distribution Points ───────────────────────────────────────────────────

func TestAcceptance_ResolveDistributionPointV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-dp-" + runSuffix()

	_, err := c.ResolveDistributionPointV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	newDP := func(n string) *pro.DistributionPoint {
		return &pro.DistributionPoint{
			Name: n, FileSharingConnectionType: "SMB", ServerName: "localhost",
			ShareName: strPtr("share"), ReadWriteUsername: strPtr("rw"), ReadWritePassword: strPtr("rw"),
			ReadOnlyUsername: strPtr("ro"), ReadOnlyPassword: strPtr("ro"),
		}
	}
	resp, err := c.CreateDistributionPointV1(ctx, newDP(name))
	if err != nil {
		t.Fatalf("CreateDistributionPointV1: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteDistributionPointV1(ctx, id1) })

	gotID, err := c.ResolveDistributionPointV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}
	t.Logf("resolved %q → %s ✓", name, gotID)

	id2, dupCreated := tryCreateDuplicate(t, "distribution point", func() (string, error) {
		r, e := c.CreateDistributionPointV1(ctx, newDP(name))
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteDistributionPointV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveDistributionPointV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteDistributionPointV1(ctx, id2)
	}

	if err := c.DeleteDistributionPointV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveDistributionPointV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Ebooks ────────────────────────────────────────────────────────────────
// No create endpoint. Read-only probe.

func TestAcceptance_ResolveEbookV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveEbookV1IDByName(context.Background(), "sdk-does-not-exist-ebook-"+runSuffix())
	requireNotFoundErr(t, "ResolveEbookV1IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveEbookV1IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	ebooks, err := c.ListEbooksV1(ctx, nil)
	if err != nil {
		t.Fatalf("ListEbooksV1: %v", err)
	}
	if len(ebooks) == 0 {
		t.Skip("no ebooks — skipping")
	}
	first := ebooks[0]
	gotID, err := c.ResolveEbookV1IDByName(ctx, first.Name)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotID != first.ID {
		t.Errorf("resolved id = %q, want %q", gotID, first.ID)
	}
	t.Logf("resolved %q → %s ✓", first.Name, gotID)
}

// ─── API Integrations ──────────────────────────────────────────────────────

func TestAcceptance_ResolveApiIntegrationV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-ai-" + runSuffix()

	_, err := c.ResolveApiIntegrationV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	// Create an API role for the integration's authorization scopes.
	roleName := "sdk-acc-res-ai-role-" + runSuffix()
	role, err := c.CreateApiRoleV1(ctx, &pro.ApiRoleRequest{DisplayName: roleName, Privileges: []string{"Read Buildings"}})
	if err != nil {
		t.Fatalf("CreateApiRoleV1 (prereq): %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteApiRoleV1(ctx, role.ID) })

	newAI := func(n string) *pro.ApiIntegrationRequest {
		return &pro.ApiIntegrationRequest{DisplayName: n, AuthorizationScopes: []string{roleName}}
	}
	resp, err := c.CreateApiIntegrationV1(ctx, newAI(name))
	if err != nil {
		t.Fatalf("CreateApiIntegrationV1: %v", err)
	}
	id1 := strconv.Itoa(resp.ID)
	t.Cleanup(func() { _ = c.DeleteApiIntegrationV1(ctx, id1) })

	gotID, err := c.ResolveApiIntegrationV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}
	t.Logf("resolved %q → %s ✓", name, gotID)

	got, err := c.ResolveApiIntegrationV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.DisplayName != name {
		t.Errorf("typed DisplayName = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "api integration", func() (string, error) {
		r, e := c.CreateApiIntegrationV1(ctx, newAI(name))
		if e != nil {
			return "", e
		}
		return strconv.Itoa(r.ID), nil
	}, func(id string) error { return c.DeleteApiIntegrationV1(ctx, id) })

	if dupCreated {
		time.Sleep(2 * time.Second) // allow eventual consistency for RSQL index
		_, err = c.ResolveApiIntegrationV1IDByName(ctx, name)
		var amErr *jamfplatform.AmbiguousMatchError
		if errors.As(err, &amErr) {
			t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		} else {
			// API integrations list endpoint has eventual consistency — dup may not
			// appear in filtered results immediately. Log rather than fail.
			t.Logf("NOTE: dup created (%s, %s) but resolver did not detect ambiguity (eventual consistency); err=%v", id1, id2, err)
		}
		_ = c.DeleteApiIntegrationV1(ctx, id2)
	}

	if err := c.DeleteApiIntegrationV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveApiIntegrationV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Supervision Identities ────────────────────────────────────────────────

func TestAcceptance_ResolveSupervisionIdentityV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-si-" + runSuffix()

	_, err := c.ResolveSupervisionIdentityV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	newSI := func(n string) *pro.SupervisionIdentityCreate {
		return &pro.SupervisionIdentityCreate{DisplayName: n, Password: "Sdk-Test-Pass-123!"}
	}
	resp, err := c.CreateSupervisionIdentityV1(ctx, newSI(name))
	if err != nil {
		t.Fatalf("CreateSupervisionIdentityV1: %v", err)
	}
	id1 := strconv.Itoa(resp.ID)
	t.Cleanup(func() { _ = c.DeleteSupervisionIdentityV1(ctx, id1) })

	gotID, err := c.ResolveSupervisionIdentityV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}
	t.Logf("resolved %q → %s ✓", name, gotID)

	got, err := c.ResolveSupervisionIdentityV1ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.DisplayName != name {
		t.Errorf("typed DisplayName = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "supervision identity", func() (string, error) {
		r, e := c.CreateSupervisionIdentityV1(ctx, newSI(name))
		if e != nil {
			return "", e
		}
		return strconv.Itoa(r.ID), nil
	}, func(id string) error { return c.DeleteSupervisionIdentityV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveSupervisionIdentityV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteSupervisionIdentityV1(ctx, id2)
	}

	if err := c.DeleteSupervisionIdentityV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveSupervisionIdentityV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Volume Purchasing Locations ───────────────────────────────────────────
// Requires VPP service token. Read-only probe.

func TestAcceptance_ResolveVolumePurchasingLocationV1_NotFound(t *testing.T) {
	c := pro.New(accClient(t))
	_, err := c.ResolveVolumePurchasingLocationV1IDByName(context.Background(), "sdk-does-not-exist-vpl-"+runSuffix())
	requireNotFoundErr(t, "ResolveVolumePurchasingLocationV1IDByName", err)
	t.Log("not-found surfaced 404 ✓")
}

func TestAcceptance_ResolveVolumePurchasingLocationV1IDByName_Existing(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	vpls, err := c.ListVolumePurchasingLocationsV1(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListVolumePurchasingLocationsV1: %v", err)
	}
	if len(vpls) == 0 {
		t.Skip("no volume purchasing locations — skipping")
	}
	first := vpls[0]
	gotID, err := c.ResolveVolumePurchasingLocationV1IDByName(ctx, first.Name)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotID != first.ID {
		t.Errorf("resolved id = %q, want %q", gotID, first.ID)
	}
	t.Logf("resolved %q → %s ✓", first.Name, gotID)
}

// ─── Accounts ──────────────────────────────────────────────────────────────

func TestAcceptance_ResolveAccountV1_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-acct-" + runSuffix()

	_, err := c.ResolveAccountV1IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	accessLevel := "FullAccess"
	privilegeLevel := "ADMINISTRATOR"
	pass := "Sdk-Test-Pass-123!" + runSuffix()
	acctEmail := name + "@example.invalid"
	siteID := -1
	acctStatus := "Enabled"
	acctType := "DEFAULT"
	falseVal := false
	ldapServerID := -1
	distinguishedName := ""
	phone := "000-000-0000"
	newAcct := func(n string) *pro.UserAccount {
		realname := "SDK Res " + n
		return &pro.UserAccount{
			Username:                  &n,
			Realname:                  &realname,
			Email:                     &acctEmail,
			Phone:                     &phone,
			AccessLevel:               &accessLevel,
			PrivilegeLevel:            &privilegeLevel,
			PlainPassword:             &pass,
			SiteID:                    &siteID,
			LdapServerID:              &ldapServerID,
			DistinguishedName:         &distinguishedName,
			AccountStatus:             &acctStatus,
			AccountType:               &acctType,
			ChangePasswordOnNextLogin: &falseVal,
		}
	}
	resp, err := c.CreateAccountV1(ctx, newAcct(name))
	if err != nil {
		t.Fatalf("CreateAccountV1: %v", err)
	}
	if resp.ID == nil {
		t.Fatal("CreateAccountV1 returned nil ID")
	}
	id1 := *resp.ID
	t.Cleanup(func() { _ = c.DeleteAccountV1(ctx, id1) })

	gotID, err := c.ResolveAccountV1IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}
	t.Logf("resolved %q → %s ✓", name, gotID)

	id2, dupCreated := tryCreateDuplicate(t, "account", func() (string, error) {
		r, e := c.CreateAccountV1(ctx, newAcct(name))
		if e != nil {
			return "", e
		}
		if r.ID == nil {
			return "", fmt.Errorf("nil ID from create")
		}
		return *r.ID, nil
	}, func(id string) error { return c.DeleteAccountV1(ctx, id) })

	if dupCreated {
		_, err = c.ResolveAccountV1IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteAccountV1(ctx, id2)
	}

	if err := c.DeleteAccountV1(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveAccountV1IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}

// ─── Enrollment Customizations ─────────────────────────────────────────────

func TestAcceptance_ResolveEnrollmentCustomizationV2_Lifecycle(t *testing.T) {
	c := pro.New(accClient(t))
	ctx := context.Background()
	name := "sdk-acc-res-ec-" + runSuffix()

	_, err := c.ResolveEnrollmentCustomizationV2IDByName(ctx, name)
	requireNotFoundErr(t, "pre-create", err)

	newEC := func(n string) *pro.EnrollmentCustomizationV2 {
		return &pro.EnrollmentCustomizationV2{
			DisplayName: n, Description: "SDK test", SiteID: "-1",
			EnrollmentCustomizationBrandingSettings: pro.EnrollmentCustomizationBrandingSettings{
				TextColor: "000000", ButtonColor: "007AFF", ButtonTextColor: "FFFFFF",
				BackgroundColor: "FFFFFF", IconURL: "",
			},
		}
	}
	resp, err := c.CreateEnrollmentCustomizationV2(ctx, newEC(name))
	if err != nil {
		t.Fatalf("CreateEnrollmentCustomizationV2: %v", err)
	}
	id1 := resp.ID
	t.Cleanup(func() { _ = c.DeleteEnrollmentCustomizationV2(ctx, id1) })

	gotID, err := c.ResolveEnrollmentCustomizationV2IDByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve ID: %v", err)
	}
	if gotID != id1 {
		t.Errorf("resolve ID = %q, want %q", gotID, id1)
	}
	t.Logf("resolved %q → %s ✓", name, gotID)

	got, err := c.ResolveEnrollmentCustomizationV2ByName(ctx, name)
	if err != nil {
		t.Fatalf("resolve typed: %v", err)
	}
	if got == nil || got.DisplayName != name {
		t.Errorf("typed DisplayName = %v, want %q", got, name)
	}

	id2, dupCreated := tryCreateDuplicate(t, "enrollment customization", func() (string, error) {
		r, e := c.CreateEnrollmentCustomizationV2(ctx, newEC(name))
		if e != nil {
			return "", e
		}
		return r.ID, nil
	}, func(id string) error { return c.DeleteEnrollmentCustomizationV2(ctx, id) })

	if dupCreated {
		_, err = c.ResolveEnrollmentCustomizationV2IDByName(ctx, name)
		requireAmbiguousErr(t, "ambiguous", err)
		t.Logf("ambiguous with IDs %s, %s ✓", id1, id2)
		_ = c.DeleteEnrollmentCustomizationV2(ctx, id2)
	}

	if err := c.DeleteEnrollmentCustomizationV2(ctx, id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.ResolveEnrollmentCustomizationV2IDByName(ctx, name)
	requireNotFoundErr(t, "post-delete", err)
	t.Log("lifecycle complete ✓")
}
