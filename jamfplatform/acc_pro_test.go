// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
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
	t.Cleanup(func() { _ = p.DeleteBuildingV1(ctx, created.ID) })
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
	t.Cleanup(func() { _ = p.DeleteBuildingV1(ctx, created.ID) })

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
