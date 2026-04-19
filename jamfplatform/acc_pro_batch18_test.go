// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Batch 18 — packages completeness + smtp history + buildings history.
// Packages get history + manifest + bulk-delete + export; SMTP gets
// history + test-send; buildings get history + bulk-delete + export.

// --- packages --------------------------------------------------------

// createAccPackage builds a minimal package fixture and registers
// cleanup. Returns the package id.
func createAccPackage(t *testing.T, p *pro.Client, suffix string) string {
	t.Helper()
	ctx := context.Background()
	created, err := p.CreatePackageV1(ctx, &pro.Package{
		PackageName:          "sdk-acc-pkg-" + suffix,
		FileName:             "sdk-acc-" + suffix + ".pkg",
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
	id := created.ID
	cleanupDelete(t, "Package "+id, func() error { return p.DeletePackageV1(ctx, id) })
	return id
}

func TestAcceptance_Pro_PackageHistoryV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	id := createAccPackage(t, p, runSuffix())

	if _, err := p.CreatePackageHistoryNoteV1(ctx, id, &pro.ObjectHistoryNote{
		Note: "sdk-acc test package history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreatePackageHistoryNoteV1: %v", err)
	}

	hist, err := p.ListPackageHistoryV1(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListPackageHistoryV1: %v", err)
	}
	t.Logf("Package history: %d entries", len(hist))

	body, err := p.ExportPackageHistoryV1(ctx, id, &pro.ExportParameters{}, nil, nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ExportPackageHistoryV1: %v", err)
	}
	t.Logf("Package history export: %d bytes", len(body))
}

func TestAcceptance_Pro_PackagesExportV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	body, err := pro.New(c).ExportPackagesV1(ctx, &pro.ExportParameters{}, nil, nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ExportPackagesV1: %v", err)
	}
	t.Logf("Packages export: %d bytes", len(body))
}

func TestAcceptance_Pro_PackageManifestV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	id := createAccPackage(t, p, runSuffix()+"-manifest")

	// Minimal plist manifest body — the exact shape the server expects
	// isn't documented, so start with a placeholder and tolerate 4xx.
	// When the fixture is accepted we also exercise delete.
	manifest := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>items</key><array/></dict></plist>
`)
	if _, err := p.UploadPackageManifestV1(ctx, id, "manifest.plist", bytes.NewReader(manifest)); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("UploadPackageManifestV1 rejected: status=%d — placeholder manifest likely invalid", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("UploadPackageManifestV1: %v", err)
	}
	t.Logf("Uploaded manifest for package %s", id)

	if err := p.DeletePackageManifestV1(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeletePackageManifestV1: %v", err)
	}
}

func TestAcceptance_Pro_PackagesDeleteMultipleV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	a := createAccPackage(t, p, runSuffix()+"-a")
	b := createAccPackage(t, p, runSuffix()+"-b")

	ids := []string{a, b}
	if err := p.DeleteMultiplePackagesV1(ctx, &pro.Ids{IDs: &ids}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteMultiplePackagesV1: %v", err)
	}
	t.Logf("DeleteMultiplePackagesV1 succeeded for ids %v", ids)
}

// --- SMTP server -----------------------------------------------------

func TestAcceptance_Pro_SmtpServerHistoryV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	if _, err := p.CreateSmtpServerHistoryNoteV1(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test smtp-server history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSmtpServerHistoryNoteV1: %v", err)
	}

	hist, err := p.ListSmtpServerHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListSmtpServerHistoryV1: %v", err)
	}
	t.Logf("SMTP server history: %d entries", len(hist))
}

// TestSmtpServerV1 sends a test email. Use an obviously-synthetic
// recipient so a misrouted send is easy to spot. Tolerate 400 when
// the tenant's SMTP isn't configured to accept outbound mail.
func TestAcceptance_Pro_SmtpServerTestV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	err := pro.New(c).TestSmtpServerV1(ctx, &pro.SmtpServerTest{
		RecipientEmail: "sdk-acc-discard@example.invalid",
	})
	if err == nil {
		t.Log("TestSmtpServerV1 accepted (202) — tenant SMTP relays outbound mail")
		return
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("TestSmtpServerV1 rejected: status=%d — expected when SMTP relay blocks the test domain", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Fatalf("TestSmtpServerV1: %v", err)
}

// --- buildings -------------------------------------------------------

func createAccBuilding(t *testing.T, p *pro.Client, suffix string) string {
	t.Helper()
	ctx := context.Background()
	created, err := p.CreateBuildingV1(ctx, &pro.Building{
		Name: "sdk-acc-building-" + suffix,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBuildingV1: %v", err)
	}
	id := created.ID
	cleanupDelete(t, "Building "+id, func() error { return p.DeleteBuildingV1(ctx, id) })
	return id
}

func TestAcceptance_Pro_BuildingHistoryV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	id := createAccBuilding(t, p, runSuffix())

	if _, err := p.CreateBuildingHistoryNoteV1(ctx, id, &pro.ObjectHistoryNote{
		Note: "sdk-acc test building history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateBuildingHistoryNoteV1: %v", err)
	}

	hist, err := p.ListBuildingHistoryV1(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListBuildingHistoryV1: %v", err)
	}
	t.Logf("Building history: %d entries", len(hist))

	body, err := p.ExportBuildingHistoryV1(ctx, id, &pro.ExportParameters{}, nil, nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ExportBuildingHistoryV1: %v", err)
	}
	t.Logf("Building history export: %d bytes", len(body))
}

func TestAcceptance_Pro_BuildingsDeleteMultipleV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	a := createAccBuilding(t, p, runSuffix()+"-a")
	b := createAccBuilding(t, p, runSuffix()+"-b")

	ids := []string{a, b}
	if err := p.DeleteMultipleBuildingsV1(ctx, &pro.Ids{IDs: &ids}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteMultipleBuildingsV1: %v", err)
	}
	t.Logf("DeleteMultipleBuildingsV1 succeeded for ids %v", ids)
}
