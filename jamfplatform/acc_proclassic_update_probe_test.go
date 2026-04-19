// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

// Update-by-alt-key plumbing probes. Calls each UpdateX method with a
// synthetic path value and a zero-value body; accepts any APIResponseError
// as confirmation the endpoint is wired. Primary-key UPDATEs are covered
// by the CRUD round-trip tests.

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAcceptance_Classic_ProbeUpdate_UpdateAccountByUserID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateAccountByUserID(context.Background(), "999999999", &proclassic.Account{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateAccountByUserID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateAccountByUsername(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateAccountByUsername(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Account{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateAccountByUsername transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateAccountGroupByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateAccountGroupByID(context.Background(), "999999999", &proclassic.Group{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateAccountGroupByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateAccountGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateAccountGroupByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Group{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateAccountGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateActivationCode(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateActivationCode(context.Background(), &proclassic.ActivationCode{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateActivationCode transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateAdvancedComputerSearchByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateAdvancedComputerSearchByID(context.Background(), "999999999", &proclassic.AdvancedComputerSearch{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateAdvancedComputerSearchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateAdvancedMobileDeviceSearchByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateAdvancedMobileDeviceSearchByID(context.Background(), "999999999", &proclassic.AdvancedMobileDeviceSearch{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateAdvancedMobileDeviceSearchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateAdvancedUserSearchByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateAdvancedUserSearchByID(context.Background(), "999999999", &proclassic.AdvancedUserSearch{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateAdvancedUserSearchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateBuildingByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateBuildingByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Building{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateBuildingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateCategoryByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateCategoryByID(context.Background(), "999999999", &proclassic.Category{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateCategoryByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateCategoryByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateCategoryByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Category{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateCategoryByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateClassByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateClassByID(context.Background(), "999999999", &proclassic.ClassPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateClassByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateClassByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateClassByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.ClassPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateClassByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateClassicPackageByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateClassicPackageByID(context.Background(), "999999999", &proclassic.Package{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateClassicPackageByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateComputerByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateComputerByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.ComputerPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateComputerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateComputerCheckIn(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateComputerCheckIn(context.Background(), &proclassic.ComputerCheckIn{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateComputerCheckIn transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateComputerExtensionAttributeByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateComputerExtensionAttributeByID(context.Background(), "999999999", &proclassic.ComputerExtensionAttribute{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateComputerExtensionAttributeByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateComputerExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateComputerExtensionAttributeByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.ComputerExtensionAttribute{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateComputerExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateComputerGroupByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateComputerGroupByID(context.Background(), "999999999", &proclassic.ComputerGroupPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateComputerGroupByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateComputerGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateComputerGroupByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.ComputerGroupPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateComputerGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateComputerInventoryCollection(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateComputerInventoryCollection(context.Background(), &proclassic.ComputerInventoryCollection{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateComputerInventoryCollection transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDepartmentByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDepartmentByID(context.Background(), "999999999", &proclassic.Department{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDepartmentByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDepartmentByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDepartmentByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Department{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDepartmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDirectoryBindingByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDirectoryBindingByID(context.Background(), "999999999", &proclassic.DirectoryBinding{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDirectoryBindingByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDirectoryBindingByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDirectoryBindingByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.DirectoryBinding{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDirectoryBindingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDiskEncryptionConfigurationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDiskEncryptionConfigurationByID(context.Background(), "999999999", &proclassic.DiskEncryptionConfiguration{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDiskEncryptionConfigurationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDiskEncryptionConfigurationByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDiskEncryptionConfigurationByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.DiskEncryptionConfiguration{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDiskEncryptionConfigurationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDistributionPointByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDistributionPointByID(context.Background(), "999999999", &proclassic.DistributionPointPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDistributionPointByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDistributionPointByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDistributionPointByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.DistributionPointPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDistributionPointByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDockItemByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDockItemByID(context.Background(), "999999999", &proclassic.DockItem{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDockItemByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateDockItemByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateDockItemByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.DockItem{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateDockItemByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateEbookByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateEbookByID(context.Background(), "999999999", &proclassic.EbookPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateEbookByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateEbookByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateEbookByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.EbookPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateEbookByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateGSXConnection(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateGSXConnection(context.Background(), &proclassic.GsxConnection{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateGSXConnection transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateHealthcareListenerByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateHealthcareListenerByID(context.Background(), "999999999", &proclassic.HealthcareListener{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateHealthcareListenerByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateHealthcareListenerRuleByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateHealthcareListenerRuleByID(context.Background(), "999999999", &proclassic.HealthcareListenerRule{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateHealthcareListenerRuleByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateIBeaconByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateIBeaconByID(context.Background(), "999999999", &proclassic.Ibeacon{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateIBeaconByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateIBeaconByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateIBeaconByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Ibeacon{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateIBeaconByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateInfrastructureManagerByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateInfrastructureManagerByID(context.Background(), "999999999", &proclassic.InfrastructureManager{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateInfrastructureManagerByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateJsonWebTokenConfigurationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateJsonWebTokenConfigurationByID(context.Background(), "999999999", &proclassic.JsonWebTokenConfiguration{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateJsonWebTokenConfigurationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateLDAPServerByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateLDAPServerByID(context.Background(), "999999999", &proclassic.LdapServerPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateLDAPServerByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateLDAPServerByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateLDAPServerByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.LdapServerPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateLDAPServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateLicensedSoftwareByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateLicensedSoftwareByID(context.Background(), "999999999", &proclassic.LicensedSoftware{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateLicensedSoftwareByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateLicensedSoftwareByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateLicensedSoftwareByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.LicensedSoftware{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateLicensedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMacApplicationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMacApplicationByID(context.Background(), "999999999", &proclassic.MacApplication{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMacApplicationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMacApplicationByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMacApplicationByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MacApplication{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMacApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceApplicationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceApplicationByID(context.Background(), "999999999", &proclassic.MobileDeviceApplication{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceApplicationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceApplicationByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceApplicationByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDeviceApplication{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceByID(context.Background(), "999999999", &proclassic.MobileDevicePost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceByMacAddress(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceByMacAddress(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDevicePost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDevicePost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceBySerialNumber(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceBySerialNumber(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDevicePost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceByUDID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceByUDID(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDevicePost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceConfigurationProfileByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceConfigurationProfileByID(context.Background(), "999999999", &proclassic.MobileDeviceConfigurationProfile{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceConfigurationProfileByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceConfigurationProfileByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDeviceConfigurationProfile{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceEnrollmentProfileByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceEnrollmentProfileByID(context.Background(), "999999999", &proclassic.MobileDeviceEnrollmentProfilePost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceEnrollmentProfileByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceExtensionAttributeByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceExtensionAttributeByID(context.Background(), "999999999", &proclassic.MobileDeviceExtensionAttribute{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceExtensionAttributeByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceExtensionAttributeByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDeviceExtensionAttribute{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceGroupByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceGroupByID(context.Background(), "999999999", &proclassic.MobileDeviceGroup{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceGroupByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateMobileDeviceGroupByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.MobileDeviceGroup{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateMobileDeviceProvisioningProfileByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).UpdateMobileDeviceProvisioningProfileByID(context.Background(), "999999999", &proclassic.MobileDeviceProvisioningProfile{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceProvisioningProfileByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateNetworkSegmentByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateNetworkSegmentByID(context.Background(), "999999999", &proclassic.NetworkSegmentPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateNetworkSegmentByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateNetworkSegmentByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateNetworkSegmentByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.NetworkSegmentPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateNetworkSegmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateOSXConfigurationProfileByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateOSXConfigurationProfileByID(context.Background(), "999999999", &proclassic.OsXConfigurationProfile{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateOSXConfigurationProfileByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateOSXConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateOSXConfigurationProfileByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.OsXConfigurationProfile{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateOSXConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePatchByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePatchByID(context.Background(), "999999999", &proclassic.SoftwareTitle{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePatchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePatchExternalSourceByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePatchExternalSourceByID(context.Background(), "999999999", &proclassic.PatchExternalSource{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePatchExternalSourceByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePatchPolicyByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePatchPolicyByID(context.Background(), "999999999", &proclassic.PatchPolicy{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePatchPolicyByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePatchSoftwareTitleByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePatchSoftwareTitleByID(context.Background(), "999999999", &proclassic.PatchSoftwareTitle{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePatchSoftwareTitleByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePeripheralByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePeripheralByID(context.Background(), "999999999", &proclassic.PeripheralPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePeripheralByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePeripheralTypeByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePeripheralTypeByID(context.Background(), "999999999", &proclassic.PeripheralType{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePeripheralTypeByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePolicyByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePolicyByID(context.Background(), "999999999", &proclassic.PolicyPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePolicyByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePolicyByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePolicyByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.PolicyPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePolicyByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePrinterByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePrinterByID(context.Background(), "999999999", &proclassic.Printer{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePrinterByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdatePrinterByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdatePrinterByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Printer{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdatePrinterByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateRemovableMacAddressByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateRemovableMacAddressByID(context.Background(), "999999999", &proclassic.RemovableMacAddress{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateRemovableMacAddressByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateRemovableMacAddressByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateRemovableMacAddressByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.RemovableMacAddress{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateRemovableMacAddressByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateRestrictedSoftwareByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateRestrictedSoftwareByID(context.Background(), "999999999", &proclassic.RestrictedSoftware{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateRestrictedSoftwareByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateRestrictedSoftwareByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateRestrictedSoftwareByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.RestrictedSoftware{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateRestrictedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateSMTPServer(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateSMTPServer(context.Background(), &proclassic.SmtpServer{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateSMTPServer transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateScriptByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateScriptByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Script{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateScriptByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateSiteByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateSiteByID(context.Background(), "999999999", &proclassic.Site{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateSiteByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateSiteByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateSiteByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Site{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateSiteByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateSoftwareUpdateServerByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateSoftwareUpdateServerByID(context.Background(), "999999999", &proclassic.SoftwareUpdateServer{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateSoftwareUpdateServerByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateSoftwareUpdateServerByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateSoftwareUpdateServerByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.SoftwareUpdateServer{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateSoftwareUpdateServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateUserByEmail(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateUserByEmail(context.Background(), "sdk-probe-update-nonexistent", &proclassic.UserPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateUserByEmail transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateUserByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateUserByID(context.Background(), "999999999", &proclassic.UserPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateUserByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateUserByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateUserByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.UserPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateUserByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateUserExtensionAttributeByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateUserExtensionAttributeByID(context.Background(), "999999999", &proclassic.UserExtensionAttribute{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateUserExtensionAttributeByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateUserExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateUserExtensionAttributeByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.UserExtensionAttribute{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateUserExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateUserGroupByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateUserGroupByID(context.Background(), "999999999", &proclassic.UserGroup{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateUserGroupByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateUserGroupByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateUserGroupByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.UserGroup{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateUserGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateVPPAccountByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateVPPAccountByID(context.Background(), "999999999", &proclassic.VppAccount{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateVPPAccountByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateVPPAssignmentByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateVPPAssignmentByID(context.Background(), "999999999", &proclassic.VppAssignmentPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateVPPAssignmentByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateVPPInvitationByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateVPPInvitationByID(context.Background(), "999999999", &proclassic.VppInvitation{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateVPPInvitationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateWebhookByID(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateWebhookByID(context.Background(), "999999999", &proclassic.Webhook{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateWebhookByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeUpdate_UpdateWebhookByName(t *testing.T) {
	c := accClient(t)
	if err := proclassic.New(c).UpdateWebhookByName(context.Background(), "sdk-probe-update-nonexistent", &proclassic.Webhook{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateWebhookByName transport error: %v", err)
	}
}

