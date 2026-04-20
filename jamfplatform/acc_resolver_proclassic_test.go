// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// assertResolvedID confirms that the ID the Resolve<X>IDByName call returned
// matches the fixture's numeric id. Shared between every direct-mode test
// below so the comparison and diagnostic phrasing stay consistent.
func assertResolvedID(t *testing.T, resolver string, got string, want int) {
	t.Helper()
	if got != strconv.Itoa(want) {
		t.Errorf("%s = %q, want %q", resolver, got, strconv.Itoa(want))
	}
}

// assertResolvedTyped confirms that the Resolve<X>ByName call returned a
// non-nil typed value whose top-level *int ID matches the fixture. The ID
// accessor is passed in because the types themselves vary.
func assertResolvedTyped[T any](t *testing.T, resolver string, got *T, id func(*T) *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s returned nil", resolver)
		return
	}
	gotID := id(got)
	if gotID == nil {
		t.Fatalf("%s returned result with nil ID", resolver)
		return
	}
	if *gotID != want {
		t.Errorf("%s ID = %d, want %d", resolver, *gotID, want)
	}
}

// assertResolverNotFound verifies that an unknown name surfaces as a
// *APIResponseError with status 404 — the shape Classic's /name/{name}
// endpoint produces and the contract every direct-mode resolver should
// preserve when unwrapped.
func assertResolverNotFound(t *testing.T, resolver string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected not-found error, got nil", resolver)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("%s: want *APIResponseError, got %T: %v", resolver, err, err)
	}
	if !apiErr.HasStatus(http.StatusNotFound) {
		t.Errorf("%s: want status 404, got %d: %v", resolver, apiErr.StatusCode, err)
	}
}

func TestAcceptance_Classic_ResolveBuildingByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-bld-" + runSuffix()
	created, err := pc.CreateBuildingByID(ctx, "0", &proclassic.Building{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBuildingByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteBuildingByID", func() error { return pc.DeleteBuildingByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveBuildingIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveBuildingIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveBuildingIDByName", gotID, id)

	gotTyped, err := pc.ResolveBuildingByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveBuildingByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveBuildingByName", gotTyped, func(b *proclassic.Building) *int { return b.ID }, id)

	_, err = pc.ResolveBuildingIDByName(ctx, "sdk-acc-nonexistent-"+runSuffix())
	assertResolverNotFound(t, "ResolveBuildingIDByName(unknown)", err)
}

func TestAcceptance_Classic_ResolveCategoryByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-cat-" + runSuffix()
	prio := 5
	created, err := pc.CreateCategoryByID(ctx, "0", &proclassic.Category{Name: classicStrPtr(name), Priority: &prio})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateCategoryByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteCategoryByID", func() error { return pc.DeleteCategoryByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveCategoryIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveCategoryIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveCategoryIDByName", gotID, id)

	gotTyped, err := pc.ResolveCategoryByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveCategoryByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveCategoryByName", gotTyped, func(c *proclassic.Category) *int { return c.ID }, id)
}

func TestAcceptance_Classic_ResolveClassByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-cls-" + runSuffix()
	created, err := pc.CreateClassByID(ctx, "0", &proclassic.ClassPost{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateClassByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteClassByID", func() error { return pc.DeleteClassByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveClassIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveClassIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveClassIDByName", gotID, id)

	gotTyped, err := pc.ResolveClassByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveClassByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveClassByName", gotTyped, func(c *proclassic.Class) *int { return c.ID }, id)
}

func TestAcceptance_Classic_ResolveDepartmentByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-dpt-" + runSuffix()
	created, err := pc.CreateDepartmentByID(ctx, "0", &proclassic.Department{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDepartmentByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteDepartmentByID", func() error { return pc.DeleteDepartmentByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveDepartmentIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDepartmentIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveDepartmentIDByName", gotID, id)

	gotTyped, err := pc.ResolveDepartmentByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDepartmentByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveDepartmentByName", gotTyped, func(d *proclassic.Department) *int { return d.ID }, id)
}

func TestAcceptance_Classic_ResolveSiteByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-site-" + runSuffix()
	created, err := pc.CreateSiteByID(ctx, "0", &proclassic.Site{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSiteByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteSiteByID", func() error { return pc.DeleteSiteByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveSiteIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveSiteIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveSiteIDByName", gotID, id)

	gotTyped, err := pc.ResolveSiteByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveSiteByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveSiteByName", gotTyped, func(s *proclassic.Site) *int { return s.ID }, id)
}

func TestAcceptance_Classic_ResolveScriptByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-scr-" + runSuffix()
	created, err := pc.CreateScriptByID(ctx, "0", &proclassic.Script{
		Name:           classicStrPtr(name),
		ScriptContents: classicStrPtr("#!/bin/sh\necho hello\n"),
		Priority:       classicStrPtr("After"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateScriptByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteScriptByID", func() error { return pc.DeleteScriptByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveScriptIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveScriptIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveScriptIDByName", gotID, id)

	gotTyped, err := pc.ResolveScriptByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveScriptByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveScriptByName", gotTyped, func(s *proclassic.Script) *int { return s.ID }, id)
}

func TestAcceptance_Classic_ResolveDirectoryBindingByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-dirbind-" + runSuffix()
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

	gotID, err := pc.ResolveDirectoryBindingIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDirectoryBindingIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveDirectoryBindingIDByName", gotID, id)

	gotTyped, err := pc.ResolveDirectoryBindingByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDirectoryBindingByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveDirectoryBindingByName", gotTyped, func(d *proclassic.DirectoryBinding) *int { return d.ID }, id)
}

func TestAcceptance_Classic_ResolveDockItemByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-dock-" + runSuffix()
	created, err := pc.CreateDockItemByID(ctx, "0", &proclassic.DockItem{
		Name: classicStrPtr(name),
		Path: classicStrPtr("file:///Applications/Safari.app/"),
		Type: classicStrPtr("App"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDockItemByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteDockItemByID", func() error { return pc.DeleteDockItemByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveDockItemIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDockItemIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveDockItemIDByName", gotID, id)

	gotTyped, err := pc.ResolveDockItemByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDockItemByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveDockItemByName", gotTyped, func(d *proclassic.DockItem) *int { return d.ID }, id)
}

func TestAcceptance_Classic_ResolveIBeaconByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-ibcn-" + runSuffix()
	created, err := pc.CreateIBeaconByID(ctx, "0", &proclassic.Ibeacon{
		Name: classicStrPtr(name),
		UUID: classicStrPtr("12345678-1234-1234-1234-123456789012"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateIBeaconByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteIBeaconByID", func() error { return pc.DeleteIBeaconByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveIBeaconIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveIBeaconIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveIBeaconIDByName", gotID, id)

	gotTyped, err := pc.ResolveIBeaconByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveIBeaconByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveIBeaconByName", gotTyped, func(i *proclassic.Ibeacon) *int { return i.ID }, id)
}

func TestAcceptance_Classic_ResolveLicensedSoftwareByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-licsw-" + runSuffix()
	created, err := pc.CreateLicensedSoftwareByID(ctx, "0", &proclassic.LicensedSoftware{
		General: &proclassic.LicensedSoftwareGeneral{Name: classicStrPtr(name)},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateLicensedSoftwareByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteLicensedSoftwareByID", func() error { return pc.DeleteLicensedSoftwareByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveLicensedSoftwareIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveLicensedSoftwareIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveLicensedSoftwareIDByName", gotID, id)

	gotTyped, err := pc.ResolveLicensedSoftwareByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveLicensedSoftwareByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveLicensedSoftwareByName", gotTyped, func(l *proclassic.LicensedSoftware) *int {
		if l.General == nil {
			return nil
		}
		return l.General.ID
	}, id)
}

func TestAcceptance_Classic_ResolveRestrictedSoftwareByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-restsw-" + runSuffix()
	created, err := pc.CreateRestrictedSoftwareByID(ctx, "0", &proclassic.RestrictedSoftware{
		General: &proclassic.RestrictedSoftwareGeneral{Name: classicStrPtr(name), ProcessName: classicStrPtr("evil.app")},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateRestrictedSoftwareByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteRestrictedSoftwareByID", func() error { return pc.DeleteRestrictedSoftwareByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveRestrictedSoftwareIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveRestrictedSoftwareIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveRestrictedSoftwareIDByName", gotID, id)

	gotTyped, err := pc.ResolveRestrictedSoftwareByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveRestrictedSoftwareByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveRestrictedSoftwareByName", gotTyped, func(r *proclassic.RestrictedSoftware) *int {
		if r.General == nil {
			return nil
		}
		return r.General.ID
	}, id)
}

func TestAcceptance_Classic_ResolvePrinterByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-prt-" + runSuffix()
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

	gotID, err := pc.ResolvePrinterIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolvePrinterIDByName: %v", err)
	}
	assertResolvedID(t, "ResolvePrinterIDByName", gotID, id)

	gotTyped, err := pc.ResolvePrinterByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolvePrinterByName: %v", err)
	}
	assertResolvedTyped(t, "ResolvePrinterByName", gotTyped, func(p *proclassic.Printer) *int { return p.ID }, id)
}

func TestAcceptance_Classic_ResolveAdvancedComputerSearchByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-acs-" + runSuffix()
	created, err := pc.CreateAdvancedComputerSearchByID(ctx, "0", &proclassic.AdvancedComputerSearch{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAdvancedComputerSearchByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteAdvancedComputerSearchByID", func() error { return pc.DeleteAdvancedComputerSearchByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveAdvancedComputerSearchIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveAdvancedComputerSearchIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveAdvancedComputerSearchIDByName", gotID, id)

	gotTyped, err := pc.ResolveAdvancedComputerSearchByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveAdvancedComputerSearchByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveAdvancedComputerSearchByName", gotTyped, func(a *proclassic.AdvancedComputerSearch) *int { return a.ID }, id)
}

func TestAcceptance_Classic_ResolveAdvancedMobileDeviceSearchByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-amds-" + runSuffix()
	created, err := pc.CreateAdvancedMobileDeviceSearchByID(ctx, "0", &proclassic.AdvancedMobileDeviceSearch{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAdvancedMobileDeviceSearchByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteAdvancedMobileDeviceSearchByID", func() error { return pc.DeleteAdvancedMobileDeviceSearchByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveAdvancedMobileDeviceSearchIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveAdvancedMobileDeviceSearchIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveAdvancedMobileDeviceSearchIDByName", gotID, id)

	gotTyped, err := pc.ResolveAdvancedMobileDeviceSearchByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveAdvancedMobileDeviceSearchByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveAdvancedMobileDeviceSearchByName", gotTyped, func(a *proclassic.AdvancedMobileDeviceSearch) *int { return a.ID }, id)
}

func TestAcceptance_Classic_ResolveAdvancedUserSearchByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-aus-" + runSuffix()
	created, err := pc.CreateAdvancedUserSearchByID(ctx, "0", &proclassic.AdvancedUserSearch{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAdvancedUserSearchByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteAdvancedUserSearchByID", func() error { return pc.DeleteAdvancedUserSearchByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveAdvancedUserSearchIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveAdvancedUserSearchIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveAdvancedUserSearchIDByName", gotID, id)

	gotTyped, err := pc.ResolveAdvancedUserSearchByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveAdvancedUserSearchByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveAdvancedUserSearchByName", gotTyped, func(a *proclassic.AdvancedUserSearch) *int { return a.ID }, id)
}

func TestAcceptance_Classic_ResolveComputerGroupByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-cg-" + runSuffix()
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

	gotID, err := pc.ResolveComputerGroupIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveComputerGroupIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveComputerGroupIDByName", gotID, id)

	gotTyped, err := pc.ResolveComputerGroupByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveComputerGroupByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveComputerGroupByName", gotTyped, func(g *proclassic.ComputerGroup) *int { return g.ID }, id)
}

func TestAcceptance_Classic_ResolveMobileDeviceGroupByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-mdg-" + runSuffix()
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

	gotID, err := pc.ResolveMobileDeviceGroupIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceGroupIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveMobileDeviceGroupIDByName", gotID, id)

	gotTyped, err := pc.ResolveMobileDeviceGroupByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceGroupByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveMobileDeviceGroupByName", gotTyped, func(g *proclassic.MobileDeviceGroup) *int { return g.ID }, id)
}

func TestAcceptance_Classic_ResolveUserGroupByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-ug-" + runSuffix()
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

	gotID, err := pc.ResolveUserGroupIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveUserGroupIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveUserGroupIDByName", gotID, id)

	gotTyped, err := pc.ResolveUserGroupByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveUserGroupByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveUserGroupByName", gotTyped, func(g *proclassic.UserGroup) *int { return g.ID }, id)
}

func TestAcceptance_Classic_ResolveComputerExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-cea-" + runSuffix()
	created, err := pc.CreateComputerExtensionAttributeByID(ctx, "0", &proclassic.ComputerExtensionAttribute{
		Name:     classicStrPtr(name),
		DataType: classicStrPtr("String"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateComputerExtensionAttributeByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteComputerExtensionAttributeByID", func() error { return pc.DeleteComputerExtensionAttributeByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveComputerExtensionAttributeIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveComputerExtensionAttributeIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveComputerExtensionAttributeIDByName", gotID, id)

	gotTyped, err := pc.ResolveComputerExtensionAttributeByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveComputerExtensionAttributeByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveComputerExtensionAttributeByName", gotTyped, func(e *proclassic.ComputerExtensionAttribute) *int { return e.ID }, id)
}

func TestAcceptance_Classic_ResolveMobileDeviceExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-mdea-" + runSuffix()
	// Note: Classic spec has the typo `date_type` (should be `data_type`) —
	// preserved here because generator tracks the spec verbatim.
	created, err := pc.CreateMobileDeviceExtensionAttributeByID(ctx, "0", &proclassic.MobileDeviceExtensionAttribute{
		Name:     classicStrPtr(name),
		DateType: classicStrPtr("String"),
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

	gotID, err := pc.ResolveMobileDeviceExtensionAttributeIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceExtensionAttributeIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveMobileDeviceExtensionAttributeIDByName", gotID, id)

	gotTyped, err := pc.ResolveMobileDeviceExtensionAttributeByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceExtensionAttributeByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveMobileDeviceExtensionAttributeByName", gotTyped, func(e *proclassic.MobileDeviceExtensionAttribute) *int { return e.ID }, id)
}

func TestAcceptance_Classic_ResolveUserExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-uea-" + runSuffix()
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

	gotID, err := pc.ResolveUserExtensionAttributeIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveUserExtensionAttributeIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveUserExtensionAttributeIDByName", gotID, id)

	gotTyped, err := pc.ResolveUserExtensionAttributeByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveUserExtensionAttributeByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveUserExtensionAttributeByName", gotTyped, func(e *proclassic.UserExtensionAttribute) *int { return e.ID }, id)
}

// TestAcceptance_Classic_ResolveEbookByName creates an ebook, resolves it,
// then queues the Classic tenant's known two-step cleanup (by-id, by-name)
// to work around the server's 400-echo quirk documented in the underlying
// EbookCRUD test. The resolver itself is unaffected by that quirk — the
// GET by-name path is well-behaved.
func TestAcceptance_Classic_ResolveEbookByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-ebk-" + runSuffix()
	created, err := pc.CreateEbookByID(ctx, "0", &proclassic.EbookPost{
		General: &proclassic.EbookPostGeneral{Name: classicStrPtr(name)},
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

	gotID, err := pc.ResolveEbookIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveEbookIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveEbookIDByName", gotID, id)

	gotTyped, err := pc.ResolveEbookByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveEbookByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveEbookByName", gotTyped, func(e *proclassic.Ebook) *int {
		if e.General == nil {
			return nil
		}
		return e.General.ID
	}, id)
}

func TestAcceptance_Classic_ResolveNetworkSegmentByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-ns-" + runSuffix()
	created, err := pc.CreateNetworkSegmentByID(ctx, "0", &proclassic.NetworkSegmentPost{
		Name:            classicStrPtr(name),
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

	gotID, err := pc.ResolveNetworkSegmentIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveNetworkSegmentIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveNetworkSegmentIDByName", gotID, id)

	gotTyped, err := pc.ResolveNetworkSegmentByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveNetworkSegmentByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveNetworkSegmentByName", gotTyped, func(n *proclassic.NetworkSegment) *int { return n.ID }, id)
}

func TestAcceptance_Classic_ResolveMacApplicationByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-macapp-" + runSuffix()
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

	gotID, err := pc.ResolveMacApplicationIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMacApplicationIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveMacApplicationIDByName", gotID, id)

	gotTyped, err := pc.ResolveMacApplicationByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMacApplicationByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveMacApplicationByName", gotTyped, func(m *proclassic.MacApplication) *int {
		if m.General == nil {
			return nil
		}
		return m.General.ID
	}, id)
}

// TestAcceptance_Classic_ResolveMobileDeviceApplicationByName follows the
// same "delete is async-best-effort" caveat documented in the CRUD test —
// create succeeds and the resolver reads cleanly; delete is queued via
// t.Cleanup and may race indexing. The resolver contract is unaffected.
func TestAcceptance_Classic_ResolveMobileDeviceApplicationByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-mdapp-" + runSuffix()
	bundle := "com.example.sdk-" + runSuffix()
	created, err := pc.CreateMobileDeviceApplicationByID(ctx, "0", &proclassic.MobileDeviceApplication{
		General: &proclassic.MobileDeviceApplicationGeneral{
			Name:     classicStrPtr(name),
			BundleID: classicStrPtr(bundle),
			Version:  classicStrPtr("1.0.0"),
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

	gotID, err := pc.ResolveMobileDeviceApplicationIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceApplicationIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveMobileDeviceApplicationIDByName", gotID, id)

	gotTyped, err := pc.ResolveMobileDeviceApplicationByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveMobileDeviceApplicationByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveMobileDeviceApplicationByName", gotTyped, func(m *proclassic.MobileDeviceApplication) *int {
		if m.General == nil {
			return nil
		}
		return m.General.ID
	}, id)
}

func TestAcceptance_Classic_ResolveWebhookByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-wh-" + runSuffix()
	created, err := pc.CreateWebhookByID(ctx, "0", &proclassic.Webhook{
		Name:        classicStrPtr(name),
		URL:         classicStrPtr("https://webhook.example.test/receiver"),
		Event:       classicStrPtr("ComputerAdded"),
		ContentType: classicStrPtr("application/json"),
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateWebhookByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteWebhookByID", func() error { return pc.DeleteWebhookByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveWebhookIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveWebhookIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveWebhookIDByName", gotID, id)

	gotTyped, err := pc.ResolveWebhookByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveWebhookByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveWebhookByName", gotTyped, func(w *proclassic.Webhook) *int { return w.ID }, id)
}

func TestAcceptance_Classic_ResolveRemovableMacAddressByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	suffix := runSuffix()
	// MAC-looking name, but the "name" is just a string column on this
	// endpoint — no format enforcement. Keep the form readable.
	name := "AA:BB:CC:DD:EE:" + suffix[len(suffix)-2:]
	created, err := pc.CreateRemovableMacAddressByID(ctx, "0", &proclassic.RemovableMacAddress{Name: classicStrPtr(name)})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateRemovableMacAddressByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("no ID: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteRemovableMacAddressByID", func() error { return pc.DeleteRemovableMacAddressByID(ctx, intToStr(id)) })

	gotID, err := pc.ResolveRemovableMacAddressIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveRemovableMacAddressIDByName: %v", err)
	}
	assertResolvedID(t, "ResolveRemovableMacAddressIDByName", gotID, id)

	gotTyped, err := pc.ResolveRemovableMacAddressByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveRemovableMacAddressByName: %v", err)
	}
	assertResolvedTyped(t, "ResolveRemovableMacAddressByName", gotTyped, func(r *proclassic.RemovableMacAddress) *int { return r.ID }, id)
}

func TestAcceptance_Classic_ResolvePolicyByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-rsv-pol-" + runSuffix()
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

	gotID, err := pc.ResolvePolicyIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolvePolicyIDByName: %v", err)
	}
	assertResolvedID(t, "ResolvePolicyIDByName", gotID, id)

	gotTyped, err := pc.ResolvePolicyByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolvePolicyByName: %v", err)
	}
	assertResolvedTyped(t, "ResolvePolicyByName", gotTyped, func(p *proclassic.Policy) *int {
		if p.General == nil {
			return nil
		}
		return p.General.ID
	}, id)
}
