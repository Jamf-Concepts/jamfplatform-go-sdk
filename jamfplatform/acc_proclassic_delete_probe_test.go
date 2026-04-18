// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

// Delete-by-alternate-key plumbing probes. Calls each DeleteX method
// with a clearly-synthetic value that won't match any real record;
// accepts any APIResponseError (404 / 405 / 409) as confirmation the
// endpoint is wired. Only transport-layer failures fail the test.
// Primary-key DELETEs are covered by the CRUD round-trip tests.

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAcceptance_Classic_ProbeDelete_DeleteAccountByUserID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteAccountByUserID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteAccountByUserID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteAccountByUsername(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteAccountByUsername(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteAccountByUsername transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteAccountGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteAccountGroupByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteAccountGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteAdvancedComputerSearchByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteAdvancedComputerSearchByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteAdvancedComputerSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteAdvancedMobileDeviceSearchByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteAdvancedMobileDeviceSearchByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteAdvancedMobileDeviceSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteAdvancedUserSearchByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteAdvancedUserSearchByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteAdvancedUserSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteBYOProfileByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteBYOProfileByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteBYOProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteBuildingByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteBuildingByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteBuildingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteCategoryByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteCategoryByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteCategoryByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteClassByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteClassByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteClassByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteComputerByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteComputerByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteComputerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteComputerExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteComputerExtensionAttributeByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteComputerExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteComputerGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteComputerGroupByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteComputerGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteDepartmentByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteDepartmentByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteDepartmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteDirectoryBindingByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteDirectoryBindingByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteDirectoryBindingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteDiskEncryptionConfigurationByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteDiskEncryptionConfigurationByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteDiskEncryptionConfigurationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteDistributionPointByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteDistributionPointByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteDistributionPointByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteDockItemByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteDockItemByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteDockItemByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteEbookByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteEbookByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteEbookByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteIBeaconByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteIBeaconByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteIBeaconByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteJsonWebTokenConfigurationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteJsonWebTokenConfigurationByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteJsonWebTokenConfigurationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteLDAPServerByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteLDAPServerByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteLDAPServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteLicensedSoftwareByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteLicensedSoftwareByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteLicensedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMacApplicationByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMacApplicationByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMacApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceApplicationByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceApplicationByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceByMacAddress(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceByMacAddress(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceBySerialNumber(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceBySerialNumber(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceByUDID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceByUDID(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceConfigurationProfileByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceEnrollmentProfileByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceEnrollmentProfileByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceEnrollmentProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceExtensionAttributeByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceGroupByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceInvitationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceInvitationByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceInvitationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceProvisioningProfileByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceProvisioningProfileByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceProvisioningProfileByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceProvisioningProfileByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceProvisioningProfileByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceProvisioningProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteMobileDeviceProvisioningProfileByUUID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteMobileDeviceProvisioningProfileByUUID(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteMobileDeviceProvisioningProfileByUUID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteNetworkSegmentByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteNetworkSegmentByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteNetworkSegmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteOSXConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteOSXConfigurationProfileByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteOSXConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeletePatchByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeletePatchByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeletePatchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeletePatchPolicyByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeletePatchPolicyByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeletePatchPolicyByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeletePatchSoftwareTitleByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeletePatchSoftwareTitleByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeletePatchSoftwareTitleByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeletePeripheralByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeletePeripheralByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeletePeripheralByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeletePolicyByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeletePolicyByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeletePolicyByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeletePrinterByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeletePrinterByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeletePrinterByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteRemovableMacAddressByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteRemovableMacAddressByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteRemovableMacAddressByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteRestrictedSoftwareByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteRestrictedSoftwareByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteRestrictedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteScriptByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteScriptByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteScriptByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteSiteByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteSiteByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteSiteByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteSoftwareUpdateServerByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteSoftwareUpdateServerByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteSoftwareUpdateServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteUserByEmail(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteUserByEmail(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteUserByEmail transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteUserByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteUserByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteUserByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteUserExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteUserExtensionAttributeByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteUserExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteUserGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteUserGroupByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteUserGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteVPPAccountByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteVPPAccountByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteVPPAccountByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteVPPAssignmentByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteVPPAssignmentByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteVPPAssignmentByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteVPPInvitationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteVPPInvitationByID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteVPPInvitationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeDelete_DeleteWebhookByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).DeleteWebhookByName(context.Background(), "sdk-probe-delete-nonexistent"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return // endpoint responded; plumbing verified
		}
		t.Fatalf("DeleteWebhookByName transport error: %v", err)
	}
}

