// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Version-backfill probes — exercises the older-version siblings emitted
// alongside their current-version counterparts (issue #19). Each backfilled
// list/get endpoint gets a real-tenant read probe; create/update/delete/
// upload variants are not covered here because their destructive twins on
// the current version are already skipped in the resource-family files
// (see acc_pro_inventory_test.go header comment, etc.) and exercising them
// at older versions would compound the risk.

// --- computer-inventory V1/V2 -----------------------------------------------

func TestAcceptance_Pro_Inventory_ListComputersV1(t *testing.T) {
	c := accClient(t)
	items, err := pro.New(c).ListComputersInventoryV1(context.Background(), nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputersInventoryV1: %v", err)
	}
	t.Logf("V1: %d computers", len(items))
}

func TestAcceptance_Pro_Inventory_ListComputersV2(t *testing.T) {
	c := accClient(t)
	items, err := pro.New(c).ListComputersInventoryV2(context.Background(), nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputersInventoryV2: %v", err)
	}
	t.Logf("V2: %d computers", len(items))
}

func TestAcceptance_Pro_Inventory_ListComputerFileVaultsV1(t *testing.T) {
	c := accClient(t)
	items, err := pro.New(c).ListComputerInventoryFileVaultsV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputerInventoryFileVaultsV1: %v", err)
	}
	t.Logf("V1: %d FileVault records", len(items))
}

func TestAcceptance_Pro_Inventory_ListComputerFileVaultsV2(t *testing.T) {
	c := accClient(t)
	items, err := pro.New(c).ListComputerInventoryFileVaultsV2(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputerInventoryFileVaultsV2: %v", err)
	}
	t.Logf("V2: %d FileVault records", len(items))
}

func TestAcceptance_Pro_Inventory_ComputerReadChainV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)
	computers, err := p.ListComputersInventoryV1(ctx, nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputersInventoryV1: %v", err)
	}
	if len(computers) == 0 {
		t.Skip("tenant has no computers — no read probes possible")
	}
	id := computers[0].ID
	if _, err := p.GetComputerInventoryV1(ctx, id, nil); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetComputerInventoryV1(%s): %v", id, err)
	}
	if _, err := p.GetComputerInventoryDetailV1(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetComputerInventoryDetailV1(%s): %v", id, err)
	}
	probeFileVaultAndLocks(t, ctx, p, id, "V1")
}

func TestAcceptance_Pro_Inventory_ComputerReadChainV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)
	computers, err := p.ListComputersInventoryV2(ctx, nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputersInventoryV2: %v", err)
	}
	if len(computers) == 0 {
		t.Skip("tenant has no computers — no read probes possible")
	}
	id := computers[0].ID
	if _, err := p.GetComputerInventoryV2(ctx, id, nil); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetComputerInventoryV2(%s): %v", id, err)
	}
	if _, err := p.GetComputerInventoryDetailV2(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetComputerInventoryDetailV2(%s): %v", id, err)
	}
	if _, err := p.GetComputerInventoryFileVaultV2(ctx, id); err != nil {
		logIf4xx(t, err, "GetComputerInventoryFileVaultV2", id, "no FileVault record")
	}
	if _, err := p.GetComputerDeviceLockPinV2(ctx, id); err != nil {
		logIf4xx(t, err, "GetComputerDeviceLockPinV2", id, "no PIN set or not eligible")
	} else {
		t.Logf("GetComputerDeviceLockPinV2(%s): ok (value not logged)", id)
	}
	if _, err := p.GetComputerRecoveryLockPasswordV2(ctx, id); err != nil {
		logIf4xx(t, err, "GetComputerRecoveryLockPasswordV2", id, "no password set or not eligible")
	} else {
		t.Logf("GetComputerRecoveryLockPasswordV2(%s): ok (value not logged)", id)
	}
}

func probeFileVaultAndLocks(t *testing.T, ctx context.Context, p *pro.Client, id string, version string) {
	t.Helper()
	if _, err := p.GetComputerInventoryFileVaultV1(ctx, id); err != nil {
		logIf4xx(t, err, "GetComputerInventoryFileVault"+version, id, "no FileVault record")
	}
	if _, err := p.GetComputerDeviceLockPinV1(ctx, id); err != nil {
		logIf4xx(t, err, "GetComputerDeviceLockPin"+version, id, "no PIN set or not eligible")
	} else {
		t.Logf("GetComputerDeviceLockPin%s(%s): ok (value not logged)", version, id)
	}
	if _, err := p.GetComputerRecoveryLockPasswordV1(ctx, id); err != nil {
		logIf4xx(t, err, "GetComputerRecoveryLockPassword"+version, id, "no password set or not eligible")
	} else {
		t.Logf("GetComputerRecoveryLockPassword%s(%s): ok (value not logged)", version, id)
	}
}

func logIf4xx(t *testing.T, err error, fn, id, note string) {
	t.Helper()
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("%s(%s): %d — %s, plumbing OK", fn, id, apiErr.StatusCode, note)
		return
	}
	skipOnServerError(t, err)
	t.Errorf("%s(%s): %v", fn, id, err)
}

// --- computer-inventory-collection-settings V1 ------------------------------

func TestAcceptance_Pro_Inventory_GetCollectionSettingsV1(t *testing.T) {
	c := accClient(t)
	settings, err := pro.New(c).GetComputerInventoryCollectionSettingsV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerInventoryCollectionSettingsV1: %v", err)
	}
	appPaths := 0
	if settings.ApplicationPaths != nil {
		appPaths = len(*settings.ApplicationPaths)
	}
	t.Logf("V1 collection settings: applicationPaths=%d", appPaths)
}

// --- inventory-preload V1 ---------------------------------------------------

func TestAcceptance_Pro_Inventory_DownloadPreloadCsvTemplateV1(t *testing.T) {
	c := accClient(t)
	body, err := pro.New(c).DownloadInventoryPreloadCsvTemplateV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DownloadInventoryPreloadCsvTemplateV1: %v", err)
	}
	t.Logf("V1 CSV template bytes: %d", len(body))
}

func TestAcceptance_Pro_Inventory_ListPreloadHistoryV1(t *testing.T) {
	c := accClient(t)
	items, err := pro.New(c).ListInventoryPreloadHistoryV1(context.Background(), nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListInventoryPreloadHistoryV1: %v", err)
	}
	t.Logf("V1 preload history entries: %d", len(items))
}

// --- mdm V1 -----------------------------------------------------------------

func TestAcceptance_Pro_MDM_ListCommandsV1(t *testing.T) {
	c := accClient(t)
	// V1 requires uuids or client-management-id — unlike V2's sort/filter.
	// Probe with an empty uuids filter and an obviously-bogus client ID;
	// a 200 with an empty array confirms plumbing without needing a real
	// command UUID, and 4xx is also informative.
	items, err := pro.New(c).ListMdmCommandsV1(context.Background(), nil, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("ListMdmCommandsV1: %d — bogus client-management-id rejected, plumbing OK", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("ListMdmCommandsV1: %v", err)
	}
	t.Logf("V1 MDM commands: %d", len(items))
}

// --- account-preferences V2 -------------------------------------------------

func TestAcceptance_Pro_AccountPreferences_GetV2(t *testing.T) {
	c := accClient(t)
	prefs, err := pro.New(c).GetAccountPreferencesV2(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAccountPreferencesV2: %v", err)
	}
	t.Logf("V2 account preferences fetched (language=%v)", prefs.Language)
}

// --- mobile-device-prestages V2 ---------------------------------------------

func TestAcceptance_Pro_Enrollment_ListMobileDevicePrestagesV2(t *testing.T) {
	c := accClient(t)
	items, err := pro.New(c).ListMobileDevicePrestagesV2(context.Background(), nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDevicePrestagesV2: %v", err)
	}
	t.Logf("V2 mobile device prestages: %d", len(items))
}

func TestAcceptance_Pro_Enrollment_MobileDevicePrestageReadChainV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)
	prestages, err := p.ListMobileDevicePrestagesV2(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDevicePrestagesV2: %v", err)
	}
	if len(prestages) == 0 {
		t.Skip("tenant has no mobile device prestages — no read probes possible")
	}
	id := prestages[0].ID
	if _, err := p.GetMobileDevicePrestageV2(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetMobileDevicePrestageV2(%s): %v", id, err)
	}
	if _, err := p.ListMobileDevicePrestageAttachmentsV2(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListMobileDevicePrestageAttachmentsV2(%s): %v", id, err)
	}
	if _, err := p.ListMobileDevicePrestageHistoryV2(ctx, id, nil); err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListMobileDevicePrestageHistoryV2(%s): %v", id, err)
	}
}

// --- Destructive V1/V2 endpoints intentionally not exercised ----------------
//
// The following backfilled methods mutate tenant state and are not covered
// here. They share the same destructive semantics as their current-version
// counterparts; the policy in the resource-family acc files (e.g.
// acc_pro_inventory_test.go header) already documents why those are
// skipped, and the same reasoning applies version-down:
//
//   computer-inventory:
//     CreateComputerInventoryV1, V2
//     DeleteComputerInventoryV1, V2
//     UpdateComputerInventoryDetailV1, V2
//     UploadComputerInventoryAttachmentV1, V2
//     DownloadComputerInventoryAttachmentV1, V2  (needs attachment ID fixture)
//     DeleteComputerInventoryAttachmentV1, V2
//   computer-inventory-collection-settings:
//     UpdateComputerInventoryCollectionSettingsV1
//     CreateComputerInventoryCollectionCustomPathV1
//     DeleteComputerInventoryCollectionCustomPathV1
//   inventory-preload:
//     CreateInventoryPreloadHistoryNoteV1
//   account-preferences:
//     UpdateAccountPreferencesV2
//   mobile-device-prestages:
//     CreateMobileDevicePrestageV2
//     UpdateMobileDevicePrestageV2
//     DeleteMobileDevicePrestageV2
//     UploadMobileDevicePrestageAttachmentV2
//     DeleteMultipleMobileDevicePrestageAttachmentsV2
//     CreateMobileDevicePrestageHistoryNoteV2
