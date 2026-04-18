// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

// Remaining plumbing probes for methods not covered by the bulk
// list / update / delete / get probe files. Each test either
// exercises a live CRUD round-trip against the tenant or probes
// with a synthetic value and accepts any APIResponseError as
// success. Primary-key creates that mutate tenant state are called
// with bodies that fail server-side validation so no record is
// actually created.

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// --- Create probes (call with empty body, server should reject with 4xx) ---

func TestAcceptance_Classic_ProbeCreate_CreateAccountByUserID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateAccountByUserID(context.Background(), "0", &proclassic.Account{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateAccountByUserID transport error: %v", err)
	}
	t.Fatal("empty-body create unexpectedly succeeded — would have created an account")
}

func TestAcceptance_Classic_ProbeCreate_CreateHealthcareListenerRuleByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateHealthcareListenerRuleByID(context.Background(), "0", &proclassic.HealthcareListenerRule{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateHealthcareListenerRuleByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateJsonWebTokenConfigurationByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateJsonWebTokenConfigurationByID(context.Background(), "0", &proclassic.JsonWebTokenConfiguration{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateJsonWebTokenConfigurationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateMobileDeviceByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateMobileDeviceByID(context.Background(), "0", &proclassic.MobileDevicePost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateMobileDeviceInvitationByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateMobileDeviceInvitationByID(context.Background(), "0", &proclassic.MobileDeviceInvitationPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceInvitationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateMobileDeviceProvisioningProfileByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateMobileDeviceProvisioningProfileByID(context.Background(), "0", &proclassic.MobileDeviceProvisioningProfile{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceProvisioningProfileByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePatchByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreatePatchByID(context.Background(), "0", &proclassic.SoftwareTitle{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreatePatchByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePatchPolicyBySoftwareTitleConfigID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreatePatchPolicyBySoftwareTitleConfigID(context.Background(), "999999999", &proclassic.PatchPolicy{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreatePatchPolicyBySoftwareTitleConfigID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePatchSoftwareTitleByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreatePatchSoftwareTitleByID(context.Background(), "0", &proclassic.PatchSoftwareTitle{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreatePatchSoftwareTitleByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePeripheralByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreatePeripheralByID(context.Background(), "0", &proclassic.PeripheralPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreatePeripheralByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateVPPAccountByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateVPPAccountByID(context.Background(), "0", &proclassic.VppAccount{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateVPPAccountByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateVPPAssignmentByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateVPPAssignmentByID(context.Background(), "0", &proclassic.VppAssignmentPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateVPPAssignmentByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateVPPInvitationByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).CreateVPPInvitationByID(context.Background(), "0", &proclassic.VppInvitation{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateVPPInvitationByID transport error: %v", err)
	}
}

// --- Get variants that take multiple args ---

func TestAcceptance_Classic_Probe_GetAccountByUserID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetAccountByUserID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetAccountByUserID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetComputerApplicationUsageByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetComputerApplicationUsageByID(context.Background(), "999999999", "2020-01-01", "2020-01-31"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetComputerApplicationUsageByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_GetPeripheralByIDSubset(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).GetPeripheralByIDSubset(context.Background(), "999999999", "General"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("GetPeripheralByIDSubset transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_ListPatchPoliciesBySoftwareTitleConfigID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).ListPatchPoliciesBySoftwareTitleConfigID(context.Background(), "999999999"); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("ListPatchPoliciesBySoftwareTitleConfigID transport error: %v", err)
	}
}

// --- Command issue probes — destructive on real devices. Call with bogus
// ids so the server rejects before dispatching the command. ---

func TestAcceptance_Classic_Probe_IssueComputerCommandByID(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).IssueComputerCommandByID(context.Background(), "BlankPush", "999999999", &proclassic.ComputerCommandPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("IssueComputerCommandByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_IssueMobileDeviceCommand(t *testing.T) {
	c := accClient(t)
	if _, err := proclassic.New(c).IssueMobileDeviceCommand(context.Background(), &proclassic.MobileDeviceCommandPost{}); err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("IssueMobileDeviceCommand transport error: %v", err)
	}
}
