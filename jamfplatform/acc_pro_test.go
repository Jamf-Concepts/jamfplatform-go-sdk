// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAcceptance_Pro_GetStartupStatus(t *testing.T) {
	c := accClient(t)

	status, err := pro.New(c).GetStartupStatus(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetStartupStatus failed: %v", err)
	}
	t.Logf("Startup status: step=%s stepCode=%s percentage=%d", status.Step, status.StepCode, status.Percentage)
}

func TestAcceptance_Pro_ListBuildings(t *testing.T) {
	c := accClient(t)

	buildings, err := pro.New(c).ListBuildingsV1(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBuildingsV1 failed: %v", err)
	}
	t.Logf("Found %d buildings", len(buildings))
}

func TestAcceptance_Pro_ListBuildingsWithSort(t *testing.T) {
	c := accClient(t)

	buildings, err := pro.New(c).ListBuildingsV1(context.Background(), []string{"name:asc"}, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBuildingsV1 sorted failed: %v", err)
	}
	t.Logf("Found %d buildings (sorted by name asc)", len(buildings))
}

// TestAcceptance_Pro_BuildingCRUD covers the full create → get → update →
// delete → verify-gone flow for a single Building, with t.Cleanup insuring
// against leaks if any assertion fails mid-test.
func TestAcceptance_Pro_BuildingCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-building-" + runSuffix()
	city := "Cupertino"
	country := "United States"

	// Create
	created, err := p.CreateBuildingV1(ctx, &pro.Building{
		Name:    name,
		City:    &city,
		Country: &country,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBuildingV1 failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateBuildingV1 returned no ID (href=%q)", created.Href)
	}
	cleanupDelete(t, "DeleteBuildingV1", func() error { return p.DeleteBuildingV1(ctx, created.ID) })
	t.Logf("Created building %s (%s)", created.ID, created.Href)

	// Get — round-trip confirmation
	got, err := p.GetBuildingV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetBuildingV1(%s) failed: %v", created.ID, err)
	}
	if got.Name != name {
		t.Errorf("GetBuildingV1 Name = %q, want %q", got.Name, name)
	}
	if got.City == nil || *got.City != city {
		t.Errorf("GetBuildingV1 City = %v, want %q", got.City, city)
	}

	// Update — change city, verify round-trip
	newCity := "Eau Claire"
	got.City = &newCity
	updated, err := p.UpdateBuildingV1(ctx, created.ID, got)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateBuildingV1(%s) failed: %v", created.ID, err)
	}
	if updated.City == nil || *updated.City != newCity {
		t.Errorf("UpdateBuildingV1 City = %v, want %q", updated.City, newCity)
	}

	// Delete
	if err := p.DeleteBuildingV1(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteBuildingV1(%s) failed: %v", created.ID, err)
	}

	// Verify gone — GetBuildingV1 should 404
	_, err = p.GetBuildingV1(ctx, created.ID)
	if err == nil {
		t.Fatalf("GetBuildingV1(%s) after delete should have failed, succeeded", created.ID)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetBuildingV1(%s) after delete: want 404, got %v", created.ID, err)
	}
}

func TestAcceptance_Pro_ExportBuildings(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Ensure at least one building exists so the export isn't empty-header-only.
	name := "sdk-acc-export-" + runSuffix()
	created, err := p.CreateBuildingV1(ctx, &pro.Building{Name: name})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBuildingV1: %v", err)
	}
	cleanupDelete(t, "DeleteBuildingV1", func() error { return p.DeleteBuildingV1(ctx, created.ID) })

	csv, err := p.ExportBuildingsV1(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ExportBuildingsV1: %v", err)
	}
	if len(csv) == 0 {
		t.Fatal("ExportBuildingsV1 returned empty body")
	}
	// Sanity: first line should look like a CSV header row (contains "name" column).
	firstLine := string(csv)
	if nl := strings.IndexByte(firstLine, '\n'); nl >= 0 {
		firstLine = firstLine[:nl]
	}
	if !strings.Contains(strings.ToLower(firstLine), "name") {
		t.Errorf("export header %q does not contain 'name'", firstLine)
	}
	t.Logf("Exported %d bytes; header: %s", len(csv), firstLine)
}

// TestAcceptance_Pro_ChangeUserPassword intentionally calls with a
// clearly-wrong current password and expects the API to reject. The
// alternative — actually rotating a credential — would lock out either the
// OAuth API client (our test auth) or an admin user. The test still
// exercises the transport path and payload encoding end-to-end.
// TestAcceptance_Pro_ListMobileDevicesDetail exercises the oneOf/discriminator
// path: the response carries a paginated slice of MobileDeviceResponse
// where each element is one of iOS / tvOS / watchOS variants keyed by the
// deviceType discriminator. The generated UnmarshalJSON dispatches each
// element to the matching variant pointer.
func TestAcceptance_Pro_ListMobileDevicesDetail(t *testing.T) {
	c := accClient(t)

	devices, err := pro.New(c).ListMobileDevicesDetailV2(context.Background(), nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDevicesDetailV2: %v", err)
	}
	t.Logf("Found %d mobile devices", len(devices))
	for i, d := range devices {
		if i >= 5 {
			break
		}
		switch d.DeviceType {
		case "iOS":
			if d.IOS == nil {
				t.Errorf("device[%d] DeviceType=iOS but IOS variant is nil", i)
			}
		case "tvOS":
			if d.TvOS == nil {
				t.Errorf("device[%d] DeviceType=tvOS but TvOS variant is nil", i)
			}
		case "watchOS":
			if d.WatchOS == nil {
				t.Errorf("device[%d] DeviceType=watchOS but WatchOS variant is nil", i)
			}
		}
		t.Logf("device[%d] type=%s", i, d.DeviceType)
	}
}

// TestAcceptance_Pro_PackageCRUD exercises the full Package lifecycle:
// create metadata, upload the .pkg binary (multipart), fetch to confirm
// the round-trip, then delete and verify 404. Requires a .pkg fixture
// in jamfplatform/testdata/ — gitignored because real packages run in
// the tens of megabytes. Drop in any valid .pkg locally; the test
// skips if none is present.
func TestAcceptance_Pro_PackageCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Find a .pkg fixture in testdata/. Skip if none.
	matches, _ := filepath.Glob("testdata/*.pkg")
	if len(matches) == 0 {
		t.Skip("no .pkg fixture in testdata/ — drop a signed package file there to run this test")
	}
	pkgPath := matches[0]
	pkgFile, err := os.Open(pkgPath)
	if err != nil {
		t.Fatalf("open fixture %s: %v", pkgPath, err)
	}
	t.Cleanup(func() { _ = pkgFile.Close() })

	name := "sdk-acc-pkg-" + runSuffix()
	filename := filepath.Base(pkgPath)

	// Create — minimal metadata with required fields per Package schema.
	created, err := p.CreatePackageV1(ctx, &pro.Package{
		PackageName:          name,
		FileName:             filename,
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
		skipOnServerError(t, err)
		t.Fatalf("CreatePackageV1: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreatePackageV1 returned no ID (href=%q)", created.Href)
	}
	cleanupDelete(t, "DeletePackageV1", func() error { return p.DeletePackageV1(ctx, created.ID) })
	t.Logf("Created package %s (%s)", created.ID, name)

	// Upload — multipart .pkg binary.
	uploadResp, err := p.UploadPackageV1(ctx, created.ID, filename, pkgFile)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UploadPackageV1: %v", err)
	}
	t.Logf("Uploaded pkg bytes for package %s (href=%s)", created.ID, uploadResp.Href)

	// Get — verify round-trip of the metadata we sent.
	got, err := p.GetPackageV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPackageV1(%s): %v", created.ID, err)
	}
	if got.PackageName != name {
		t.Errorf("PackageName = %q, want %q", got.PackageName, name)
	}
	if got.FileName != filename {
		t.Errorf("FileName = %q, want %q", got.FileName, filename)
	}

	// Delete.
	if err := p.DeletePackageV1(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeletePackageV1(%s): %v", created.ID, err)
	}

	// Verify gone.
	_, err = p.GetPackageV1(ctx, created.ID)
	if err == nil {
		t.Fatalf("GetPackageV1(%s) after delete should 404, succeeded", created.ID)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetPackageV1(%s) after delete: want 404, got %v", created.ID, err)
	}
}

func TestAcceptance_Pro_ListPackages(t *testing.T) {
	c := accClient(t)

	pkgs, err := pro.New(c).ListPackagesV1(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPackagesV1: %v", err)
	}
	t.Logf("Found %d packages", len(pkgs))
}

// TestAcceptance_Pro_UploadIcon uploads a PNG fixture via the multipart
// endpoint and asserts the server returned a usable id + URL. Icons
// persist on the tenant (no delete endpoint) — a handful of test icons
// accumulating in the tenant is an acceptable cost for test coverage.
func TestAcceptance_Pro_UploadIcon(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	const fixturePath = "testdata/jamf-cli-icon-1024.png"
	f, err := os.Open(fixturePath)
	if err != nil {
		t.Skipf("fixture %s unavailable: %v", fixturePath, err)
	}
	t.Cleanup(func() { _ = f.Close() })

	resp, err := pro.New(c).UploadIconV1(ctx, "sdk-acc-icon-"+runSuffix()+".png", f)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UploadIconV1: %v", err)
	}
	if resp.ID == 0 {
		t.Errorf("expected non-zero icon id, got %+v", resp)
	}
	t.Logf("Uploaded icon id=%d url=%s", resp.ID, resp.URL)
}

func TestAcceptance_Pro_ChangeUserPassword(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	err := pro.New(c).ChangeUserPasswordV1(ctx, &pro.ChangePassword{
		CurrentPassword: "sdk-acc-clearly-not-valid-" + runSuffix(),
		NewPassword:     "sdk-acc-unused",
	})
	if err == nil {
		t.Fatal("expected server to reject wrong currentPassword, got nil error (did credentials actually rotate?)")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) {
		t.Logf("ChangeUserPasswordV1 rejected as expected: status=%d", apiErr.StatusCode)
		return
	}
	t.Logf("ChangeUserPasswordV1 rejected as expected: %v", err)
}
