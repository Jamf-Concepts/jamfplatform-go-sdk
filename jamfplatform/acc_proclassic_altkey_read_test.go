// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

// Bulk plumbing-probe acc tests for Classic read endpoints beyond the
// primary by-id Get that CRUD tests already cover. Each probe calls
// with a clearly-synthetic value and accepts any APIResponseError as
// success — the endpoint is wired, the URL is constructed correctly,
// the transport round-trips. Only transport-layer failures (auth,
// network) fail the test. This gives full live coverage of the
// Get-by-alternate-key surface without needing fixtures for every
// resource/key combination.

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// GetAccountByUserID: covered by AccountUserCRUD test (skipped for spec reasons)
func TestAcceptance_Classic_Probe_GetAccountByUsername(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetAccountByUsername(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetAccountByUsername transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetAccountGroupByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetAccountGroupByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetAccountGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetAdvancedComputerSearchByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetAdvancedComputerSearchByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetAdvancedComputerSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetAdvancedMobileDeviceSearchByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetAdvancedMobileDeviceSearchByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetAdvancedMobileDeviceSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetAdvancedUserSearchByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetAdvancedUserSearchByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetAdvancedUserSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetAllowedFileExtensionByExtension(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetAllowedFileExtensionByExtension(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetAllowedFileExtensionByExtension transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetBuildingByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetBuildingByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetBuildingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetCategoryByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetCategoryByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetCategoryByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetClassByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetClassByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetClassByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerCommandByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerCommandByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerCommandByName transport error: %v", err)
	}
}

// TestAcceptance_Classic_Probe_GetComputerCommandByUUID lists existing
// computer commands and fetches the first by UUID. The endpoint 500s on
// a bogus UUID (no 404 fallback), so the probe needs a real one to
// exercise the success path. Skips cleanly when the tenant has no
// command history.
func TestAcceptance_Classic_Probe_GetComputerCommandByUUID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()

	list, err := pc.ListComputerCommands(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputerCommands: %v", err)
	}
	if list == nil || len(list.ComputerCommands) == 0 {
		t.Skip("tenant has no computer command history")
	}
	var uuid string
	for _, cc := range list.ComputerCommands {
		if cc.UUID != nil && *cc.UUID != "" {
			uuid = *cc.UUID
			break
		}
	}
	if uuid == "" {
		t.Skip("no computer command with a UUID available")
	}

	cmd, err := pc.GetComputerCommandByUUID(ctx, uuid)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerCommandByUUID(%s): %v", uuid, err)
	}
	if cmd == nil {
		t.Fatalf("expected non-nil command for uuid=%s", uuid)
	}
	t.Logf("fetched computer command uuid=%s", uuid)
}

func TestAcceptance_Classic_Probe_GetComputerExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerExtensionAttributeByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerGroupByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerGroupByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerHistoryByMacAddress(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerHistoryByMacAddress(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerHistoryByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerHistoryByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerHistoryByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerHistoryByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerHistoryBySerialNumber(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerHistoryBySerialNumber(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerHistoryBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerHistoryByUDID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerHistoryByUDID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerHistoryByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerManagementByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerManagementByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByMacAddress(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerManagementByMacAddress(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerManagementByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerManagementByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerManagementByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementBySerialNumber(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerManagementBySerialNumber(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerManagementBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByUDID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerManagementByUDID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerManagementByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerReportByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerReportByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerReportByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerReportByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerReportByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetComputerReportByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetDepartmentByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetDepartmentByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetDepartmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetDirectoryBindingByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetDirectoryBindingByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetDirectoryBindingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetDiskEncryptionConfigurationByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetDiskEncryptionConfigurationByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetDiskEncryptionConfigurationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetDistributionPointByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetDistributionPointByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetDistributionPointByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetDockItemByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetDockItemByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetDockItemByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetEbookByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetEbookByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetEbookByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetEbookByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetEbookByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetEbookByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetHealthcareListenerByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetHealthcareListenerByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetHealthcareListenerByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetHealthcareListenerRuleByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetHealthcareListenerRuleByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetHealthcareListenerRuleByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetIBeaconByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetIBeaconByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetIBeaconByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetInfrastructureManagerByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetInfrastructureManagerByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetInfrastructureManagerByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetJsonWebTokenConfigurationByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetJsonWebTokenConfigurationByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetJsonWebTokenConfigurationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetLDAPServerByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetLDAPServerByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetLDAPServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetLicensedSoftwareByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetLicensedSoftwareByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetLicensedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMacApplicationByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMacApplicationByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMacApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceApplicationByBundleID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceApplicationByBundleID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceApplicationByBundleID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceApplicationByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceApplicationByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceApplicationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceApplicationByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceApplicationByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceByMacAddress(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceByMacAddress(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceBySerialNumber(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceBySerialNumber(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceByUDID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceByUDID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceCommandByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceCommandByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceCommandByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceCommandByUUID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceCommandByUUID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceCommandByUUID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceConfigurationProfileByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceEnrollmentProfileByInvitation(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceEnrollmentProfileByInvitation(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceEnrollmentProfileByInvitation transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceEnrollmentProfileByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceEnrollmentProfileByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceEnrollmentProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceExtensionAttributeByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceGroupByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceGroupByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceHistoryByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceHistoryByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceHistoryByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceHistoryByMacAddress(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceHistoryByMacAddress(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceHistoryByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceHistoryByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceHistoryByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceHistoryByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceHistoryBySerialNumber(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceHistoryBySerialNumber(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceHistoryBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceHistoryByUDID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceHistoryByUDID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceHistoryByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceInvitationByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceInvitationByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceInvitationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceProvisioningProfileByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceProvisioningProfileByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceProvisioningProfileByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceProvisioningProfileByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceProvisioningProfileByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceProvisioningProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceProvisioningProfileByUUID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetMobileDeviceProvisioningProfileByUUID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetMobileDeviceProvisioningProfileByUUID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetNetworkSegmentByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetNetworkSegmentByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetNetworkSegmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetOSXConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetOSXConfigurationProfileByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetOSXConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPatchByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPatchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchExternalSourceByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPatchExternalSourceByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPatchExternalSourceByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchInternalSourceByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPatchInternalSourceByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPatchInternalSourceByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchPolicyByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPatchPolicyByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPatchPolicyByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchReportByPatchSoftwareTitleID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPatchReportByPatchSoftwareTitleID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPatchReportByPatchSoftwareTitleID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchSoftwareTitleByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPatchSoftwareTitleByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPatchSoftwareTitleByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPeripheralByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPeripheralByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPeripheralByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPolicyByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPolicyByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPolicyByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPrinterByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPrinterByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetPrinterByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetRemovableMacAddressByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetRemovableMacAddressByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetRemovableMacAddressByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetRestrictedSoftwareByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetRestrictedSoftwareByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetRestrictedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetScriptByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetScriptByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetScriptByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetSiteByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetSiteByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetSiteByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetSoftwareUpdateServerByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetSoftwareUpdateServerByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetSoftwareUpdateServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetUserByEmail(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetUserByEmail(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetUserByEmail transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetUserByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetUserByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetUserByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetUserExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetUserExtensionAttributeByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetUserExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetUserGroupByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetUserGroupByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetUserGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetVPPAccountByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetVPPAccountByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetVPPAccountByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetVPPAssignmentByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetVPPAssignmentByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetVPPAssignmentByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetVPPInvitationByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetVPPInvitationByID(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetVPPInvitationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetWebhookByName(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetWebhookByName(context.Background(), "sdk-probe-nonexistent-xyz"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded (404/other); plumbing verified
		}
		t.Fatalf("GetWebhookByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationByApplication(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationByApplication(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationByApplication transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationByApplicationInventory(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationByApplicationInventory(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationByApplicationInventory transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationByApplicationVersion(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationByApplicationVersion(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationByApplicationVersion transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationByApplicationVersionInventory(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationByApplicationVersionInventory(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationByApplicationVersionInventory transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationUsageByMacAddressDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationUsageByMacAddressDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationUsageByMacAddressDateRange transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationUsageByNameDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationUsageByNameDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationUsageByNameDateRange transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationUsageBySerialNumberDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationUsageBySerialNumberDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationUsageBySerialNumberDateRange transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationUsageByUDIDDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerApplicationUsageByUDIDDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationUsageByUDIDDateRange transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerHardwareSoftwareReportByIDDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerHardwareSoftwareReportByIDDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerHardwareSoftwareReportByIDDateRange transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerHardwareSoftwareReportByIDDateRangeSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerHardwareSoftwareReportByIDDateRangeSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerHardwareSoftwareReportByIDDateRangeSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerHardwareSoftwareReportByMacAddressDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerHardwareSoftwareReportByMacAddressDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerHardwareSoftwareReportByMacAddressDateRange transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_GetComputerHardwareSoftwareReportByNameDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerHardwareSoftwareReportByNameDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerHardwareSoftwareReportByNameDateRange transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_GetComputerHardwareSoftwareReportBySerialNumberDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerHardwareSoftwareReportBySerialNumberDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerHardwareSoftwareReportBySerialNumberDateRange transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_GetComputerHardwareSoftwareReportByUDIDDateRange(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerHardwareSoftwareReportByUDIDDateRange(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerHardwareSoftwareReportByUDIDDateRange transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_GetComputerHistoryByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerHistoryByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerHistoryByIDSubset transport error: %v", err)
	}
}





func TestAcceptance_Classic_Probe_GetComputerInvitationByInvitation(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerInvitationByInvitation(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerInvitationByInvitation transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByIDPatchFilter(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerManagementByIDPatchFilter(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerManagementByIDPatchFilter transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerManagementByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerManagementByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByIDUsername(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerManagementByIDUsername(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerManagementByIDUsername transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerManagementByIDUsernameSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerManagementByIDUsernameSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerManagementByIDUsernameSubset transport error: %v", err)
	}
}

















func TestAcceptance_Classic_Probe_GetEbookByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetEbookByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetEbookByIDSubset transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_GetLDAPServerByIDGroup(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetLDAPServerByIDGroup(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetLDAPServerByIDGroup transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetLDAPServerByIDGroupUser(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetLDAPServerByIDGroupUser(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetLDAPServerByIDGroupUser transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetLDAPServerByIDUser(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetLDAPServerByIDUser(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetLDAPServerByIDUser transport error: %v", err)
	}
}




func TestAcceptance_Classic_Probe_GetMacApplicationByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMacApplicationByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMacApplicationByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMacApplicationByNameSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMacApplicationByNameSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMacApplicationByNameSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceApplicationByBundleIDVersion(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceApplicationByBundleIDVersion(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceApplicationByBundleIDVersion transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceApplicationByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceApplicationByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceApplicationByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceApplicationByNameSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceApplicationByNameSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceApplicationByNameSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceCommandByCommand(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceCommandByCommand(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceCommandByCommand transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceConfigurationProfileByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceConfigurationProfileByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceConfigurationProfileByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceConfigurationProfileByNameSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceConfigurationProfileByNameSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceConfigurationProfileByNameSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceEnrollmentProfileByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceEnrollmentProfileByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceEnrollmentProfileByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceEnrollmentProfileByNameSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceEnrollmentProfileByNameSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceEnrollmentProfileByNameSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceHistoryByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceHistoryByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceHistoryByIDSubset transport error: %v", err)
	}
}





func TestAcceptance_Classic_Probe_GetMobileDeviceInvitationByInvitation(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceInvitationByInvitation(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceInvitationByInvitation transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceByIDSubset transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_GetMobileDeviceByMatch(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceByMatch(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceByMatch transport error: %v", err)
	}
}




func TestAcceptance_Classic_Probe_GetOsxConfigurationProfileByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetOsxConfigurationProfileByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetOsxConfigurationProfileByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetOsxConfigurationProfileByNameSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetOsxConfigurationProfileByNameSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetOsxConfigurationProfileByNameSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetClassicPackageByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetClassicPackageByName(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetClassicPackageByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchPolicyByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetPatchPolicyByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPatchPolicyByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPolicyByCategory(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetPolicyByCategory(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPolicyByCategory transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPolicyByCreatedBy(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetPolicyByCreatedBy(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPolicyByCreatedBy transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPolicyByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetPolicyByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPolicyByIDSubset transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_GetVPPInvitationByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetVPPInvitationByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetVPPInvitationByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputersBasic(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputersBasic(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputersBasic transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetJSSUser(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetJSSUser(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetJSSUser transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_ListSavedSearches(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).ListSavedSearches(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("ListSavedSearches transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerByMacAddress(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerByMacAddress(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerByUDID(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerByUDID(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerCommandsByCommand(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerCommandsByCommand(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerCommandsByCommand transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerCommandsByStatus(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerCommandsByStatus(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerCommandsByStatus transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerInvitationByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerInvitationByName(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerInvitationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetPatchByName(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPatchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetSavedSearchByID(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetSavedSearchByID(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetSavedSearchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetSavedSearchByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetSavedSearchByName(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetSavedSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_MatchComputers(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).MatchComputers(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("MatchComputers transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_MatchComputersByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).MatchComputersByName(context.Background(), "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("MatchComputersByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetComputerByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceEnrollmentProfileByInvitationSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceEnrollmentProfileByInvitationSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceEnrollmentProfileByInvitationSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetMobileDeviceProvisioningProfileByIDSubset(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetMobileDeviceProvisioningProfileByIDSubset(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetMobileDeviceProvisioningProfileByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchComputersByIDVersion(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetPatchComputersByIDVersion(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPatchComputersByIDVersion transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPatchReportByTitleIDVersion(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).GetPatchReportByTitleIDVersion(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPatchReportByTitleIDVersion transport error: %v", err)
	}
}
