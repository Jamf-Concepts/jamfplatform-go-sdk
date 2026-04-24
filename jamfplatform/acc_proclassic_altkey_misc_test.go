// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

// Remaining plumbing probes for methods not covered by the bulk
// list / update / delete / get probe files. Each test exercises the
// transport + codec path end-to-end and is tolerant of either a
// server-side rejection (4xx) or an accepted-but-empty create. The
// latter matters because several Classic endpoints silently accept
// empty bodies and fill in defaults — without cleanup, every run
// leaks a stray tenant record. Probes that do create something
// register a best-effort delete in t.Cleanup so the tenant stays
// clean between runs. AccountByUserID is the sole exception: we
// can't safely delete an account without risking lockout of the
// credential running the test, so that probe fails the test if the
// empty-body create succeeds — the operator must revisit the
// assumption.

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// probeCreateHandleErr interprets the err returned by a probe-create
// call: treats 5xx as skip, any other APIResponseError as "rejected as
// expected" (returns rejected=true), and anything else as a transport
// failure (t.Fatal). Returns rejected=false when err is nil — caller
// must then clean up the stray resource.
func probeCreateHandleErr(t *testing.T, resource string, err error) (rejected bool) {
	t.Helper()
	if err == nil {
		return false
	}
	skipOnServerError(t, err)
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) {
		return true
	}
	t.Fatalf("%s transport error: %v", resource, err)
	return false
}

// --- Create probes ---
//
// Most create-probes have been superseded by their real CRUD counterparts
// (TestAcceptance_Classic_FooCRUD), which round-trip create → get →
// update → delete with populated bodies and assert 404-after-delete.
// The probes that remain target endpoints where full CRUD is infeasible
// on a shared tenant: no DELETE endpoint (HealthcareListenerRule),
// destructive on real devices (bogus-id form for PatchPolicy), or
// requires real external credentials the test harness can't synthesize
// (VPP tokens, DEP tokens, upstream patch sources). Each such probe
// either registers Cleanup for any returned id or t.Fatals when cleanup
// is impossible, so the tenant stays leak-free between runs.

// TestAcceptance_Classic_ProbeCreate_CreateHealthcareListenerRuleByID — the
// Classic spec doesn't expose a DELETE for healthcare_listener_rule, so a
// stray record can't be cleaned up by the SDK. Treat unexpected
// acceptance as a hard failure so the operator can manually purge and
// reassess the probe.
func TestAcceptance_Classic_ProbeCreate_CreateHealthcareListenerRuleByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreateHealthcareListenerRuleByID(ctx, "0", &proclassic.HealthcareListenerRule{})
	if probeCreateHandleErr(t, "CreateHealthcareListenerRuleByID", err) {
		return
	}
	id := 0
	if created != nil && created.ID != nil {
		id = *created.ID
	}
	t.Fatalf("empty-body create unexpectedly succeeded (id=%d) — no DELETE endpoint, manual cleanup required", id)
}

// CreateJsonWebTokenConfigurationByID — covered by
// TestAcceptance_Classic_JsonWebTokenConfigurationCRUD.
// CreateMobileDeviceByID — covered by TestAcceptance_Classic_MobileDeviceCRUD.
// CreateMobileDeviceInvitationByID — covered by
// TestAcceptance_Classic_MobileDeviceInvitationCRUD.

func TestAcceptance_Classic_ProbeCreate_CreateMobileDeviceProvisioningProfileByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreateMobileDeviceProvisioningProfileByID(ctx, "0", &proclassic.MobileDeviceProvisioningProfile{})
	if probeCreateHandleErr(t, "CreateMobileDeviceProvisioningProfileByID", err) {
		return
	}
	if created != nil && created.ID != nil {
		id := *created.ID
		t.Cleanup(func() {
			if err := pc.DeleteMobileDeviceProvisioningProfileByID(ctx, intToStr(id)); err != nil {
				t.Logf("cleanup: DeleteMobileDeviceProvisioningProfileByID(%d): %v", id, err)
			}
		})
		t.Logf("probe-create accepted empty body; id=%d queued for cleanup", id)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePatchByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreatePatchByID(ctx, "0", &proclassic.SoftwareTitle{})
	if probeCreateHandleErr(t, "CreatePatchByID", err) {
		return
	}
	if created != nil && created.ID != nil {
		id := *created.ID
		t.Cleanup(func() {
			if err := pc.DeletePatchByID(ctx, intToStr(id)); err != nil {
				t.Logf("cleanup: DeletePatchByID(%d): %v", id, err)
			}
		})
		t.Logf("probe-create accepted empty body; id=%d queued for cleanup", id)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePatchPolicyBySoftwareTitleConfigID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreatePatchPolicyBySoftwareTitleConfigID(ctx, "999999999", &proclassic.PatchPolicy{})
	if probeCreateHandleErr(t, "CreatePatchPolicyBySoftwareTitleConfigID", err) {
		return
	}
	if created != nil && created.ID != nil {
		id := *created.ID
		t.Cleanup(func() {
			if err := pc.DeletePatchPolicyByID(ctx, intToStr(id)); err != nil {
				t.Logf("cleanup: DeletePatchPolicyByID(%d): %v", id, err)
			}
		})
		t.Logf("probe-create accepted empty body; id=%d queued for cleanup", id)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePatchSoftwareTitleByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreatePatchSoftwareTitleByID(ctx, "0", &proclassic.PatchSoftwareTitle{})
	if probeCreateHandleErr(t, "CreatePatchSoftwareTitleByID", err) {
		return
	}
	if created != nil && created.ID != nil {
		id := *created.ID
		t.Cleanup(func() {
			if err := pc.DeletePatchSoftwareTitleByID(ctx, intToStr(id)); err != nil {
				t.Logf("cleanup: DeletePatchSoftwareTitleByID(%d): %v", id, err)
			}
		})
		t.Logf("probe-create accepted empty body; id=%d queued for cleanup", id)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreatePeripheralByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreatePeripheralByID(ctx, "0", &proclassic.PeripheralPost{})
	if probeCreateHandleErr(t, "CreatePeripheralByID", err) {
		return
	}
	if created != nil && created.ID != nil {
		id := *created.ID
		t.Cleanup(func() {
			if err := pc.DeletePeripheralByID(ctx, intToStr(id)); err != nil {
				t.Logf("cleanup: DeletePeripheralByID(%d): %v", id, err)
			}
		})
		t.Logf("probe-create accepted empty body; id=%d queued for cleanup", id)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateVPPAccountByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreateVPPAccountByID(ctx, "0", &proclassic.VppAccount{})
	if probeCreateHandleErr(t, "CreateVPPAccountByID", err) {
		return
	}
	if created != nil && created.ID != nil {
		id := *created.ID
		t.Cleanup(func() {
			if err := pc.DeleteVPPAccountByID(ctx, intToStr(id)); err != nil {
				t.Logf("cleanup: DeleteVPPAccountByID(%d): %v", id, err)
			}
		})
		t.Logf("probe-create accepted empty body; id=%d queued for cleanup", id)
	}
}

func TestAcceptance_Classic_ProbeCreate_CreateVPPAssignmentByID(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()
	created, err := pc.CreateVPPAssignmentByID(ctx, "0", &proclassic.VppAssignmentPost{})
	if probeCreateHandleErr(t, "CreateVPPAssignmentByID", err) {
		return
	}
	if created != nil && created.ID != nil {
		id := *created.ID
		t.Cleanup(func() {
			if err := pc.DeleteVPPAssignmentByID(ctx, intToStr(id)); err != nil {
				t.Logf("cleanup: DeleteVPPAssignmentByID(%d): %v", id, err)
			}
		})
		t.Logf("probe-create accepted empty body; id=%d queued for cleanup", id)
	}
}

// CreateVPPInvitationByID — covered by TestAcceptance_Classic_VPPInvitationCRUD.

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

// TestAcceptance_Classic_Probe_IssueMobileDeviceCommand issues a harmless
// UpdateInventory command to a supervised mobile device. The endpoint 500s
// on an empty body, so the probe first lists mobile devices, picks the
// first supervised one (MDM commands only succeed against supervised
// targets), and constructs the wire payload from there. Skips cleanly
// when the tenant has no mobile devices, or none are supervised.
func TestAcceptance_Classic_Probe_IssueMobileDeviceCommand(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()

	list, err := pc.ListMobileDevices(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDevices: %v", err)
	}
	if list == nil || len(list.MobileDevices) == 0 {
		t.Skip("tenant has no mobile devices enrolled")
	}

	var targetID *int
	for i := range list.MobileDevices {
		d := &list.MobileDevices[i]
		if d.Supervised != nil && *d.Supervised && d.ID != nil {
			targetID = d.ID
			break
		}
	}
	if targetID == nil {
		t.Skip("no supervised mobile devices on this tenant; UpdateInventory needs a supervised target")
	}

	cmd := "UpdateInventory"
	_, err = pc.IssueMobileDeviceCommand(ctx, &proclassic.MobileDeviceCommandPost{
		General: &proclassic.MobileDeviceCommandPostGeneral{Command: &cmd},
		MobileDevices: &proclassic.MobileDeviceCommandPostMobileDevices{
			MobileDevice: &proclassic.MobileDeviceCommandPostMobileDevicesMobileDevice{ID: targetID},
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("IssueMobileDeviceCommand transport error: %v", err)
	}
	t.Logf("Issued UpdateInventory to mobile device id=%d", *targetID)
}

func TestAcceptance_Classic_Probe_CreateComputerCommandByCommand(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreateComputerCommandByCommand(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerCommandPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerCommandByCommand transport error: %v", err)
	}
}


func TestAcceptance_Classic_Probe_CreateComputerInvitationByInvitation(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreateComputerInvitationByInvitation(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerInvitation{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerInvitationByInvitation transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_UploadFileByResourceIDTypeID(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).UploadFileByResourceIDTypeID(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz")
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UploadFileByResourceIDTypeID transport error: %v", err)
	}
}



func TestAcceptance_Classic_Probe_CreateMobileDeviceCommandByCommandID(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceCommandByCommandID(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceCommandPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceCommandByCommandID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceInvitationByInvitation(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreateMobileDeviceInvitationByInvitation(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceInvitationPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceInvitationByInvitation transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceProvisioningProfileByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreateMobileDeviceProvisioningProfileByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceProvisioningProfile{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceProvisioningProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceProvisioningProfileByUUID(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreateMobileDeviceProvisioningProfileByUUID(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceProvisioningProfile{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceProvisioningProfileByUUID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreatePatchExternalSourceByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreatePatchExternalSourceByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.PatchExternalSource{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreatePatchExternalSourceByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateComputerInvitationByName(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreateComputerInvitationByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerInvitation{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerInvitationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateVPPInvitationByID(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).CreateVPPInvitationByID(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.VppInvitation{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateVPPInvitationByID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_UpdateMobileDeviceInvitationByInvitation(t *testing.T) {
	c := accClient(t)
	_, err := proclassic.New(c).UpdateMobileDeviceInvitationByInvitation(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceInvitationPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("UpdateMobileDeviceInvitationByInvitation transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceCommandByCommand(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceCommandByCommand(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceCommandPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceCommandByCommand transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceCommandByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceCommandByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceCommandPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceCommandByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceCommandWithParameterByIDList(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceCommandWithParameterByIDList(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceCommandPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceCommandWithParameterByIDList transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceCommandWithParameterVersionByIDList(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceCommandWithParameterVersionByIDList(context.Background(), "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceCommandPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceCommandWithParameterVersionByIDList transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateAccountByUsername(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateAccountByUsername(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Account{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateAccountByUsername transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateAccountGroupByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateAccountGroupByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Group{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateAccountGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateAdvancedComputerSearchByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateAdvancedComputerSearchByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.AdvancedComputerSearch{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateAdvancedComputerSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateAdvancedMobileDeviceSearchByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateAdvancedMobileDeviceSearchByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.AdvancedMobileDeviceSearch{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateAdvancedMobileDeviceSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateAdvancedUserSearchByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateAdvancedUserSearchByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.AdvancedUserSearch{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateAdvancedUserSearchByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateBuildingByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateBuildingByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Building{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateBuildingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateCategoryByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateCategoryByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Category{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateCategoryByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateClassByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateClassByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ClassPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateClassByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateClassicPackageByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateClassicPackageByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Package{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateClassicPackageByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateComputerByMacAddress(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateComputerByMacAddress(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateComputerByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateComputerByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateComputerBySerialNumber(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateComputerBySerialNumber(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateComputerByUDID(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateComputerByUDID(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateComputerExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateComputerExtensionAttributeByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerExtensionAttribute{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateComputerGroupByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateComputerGroupByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.ComputerGroupPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateComputerGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateDepartmentByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateDepartmentByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Department{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateDepartmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateDirectoryBindingByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateDirectoryBindingByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.DirectoryBinding{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateDirectoryBindingByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateDiskEncryptionConfigurationByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateDiskEncryptionConfigurationByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.DiskEncryptionConfiguration{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateDiskEncryptionConfigurationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateDistributionPointByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateDistributionPointByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.DistributionPointPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateDistributionPointByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateDockItemByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateDockItemByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.DockItem{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateDockItemByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateEbookByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateEbookByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.EbookPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateEbookByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateIBeaconByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateIBeaconByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Ibeacon{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateIBeaconByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateLDAPServerByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateLDAPServerByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.LdapServerPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateLDAPServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateLicensedSoftwareByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateLicensedSoftwareByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.LicensedSoftware{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateLicensedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMacApplicationByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMacApplicationByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MacApplication{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMacApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceApplicationByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceApplicationByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceApplication{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceApplicationByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceByMacAddress(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceByMacAddress(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDevicePost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceByMacAddress transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDevicePost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceBySerialNumber(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceBySerialNumber(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDevicePost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceBySerialNumber transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceByUDID(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceByUDID(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDevicePost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceByUDID transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceConfigurationProfileByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceConfigurationProfile{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceEnrollmentProfileByInvitation(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceEnrollmentProfileByInvitation(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceEnrollmentProfilePost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceEnrollmentProfileByInvitation transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceEnrollmentProfileByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceEnrollmentProfileByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceEnrollmentProfilePost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceEnrollmentProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceExtensionAttributeByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceExtensionAttribute{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateMobileDeviceGroupByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateMobileDeviceGroupByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.MobileDeviceGroup{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateMobileDeviceGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateNetworkSegmentByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateNetworkSegmentByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.NetworkSegmentPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateNetworkSegmentByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateOSXConfigurationProfileByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateOSXConfigurationProfileByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.OsXConfigurationProfile{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateOSXConfigurationProfileByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreatePolicyByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreatePolicyByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.PolicyPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreatePolicyByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreatePrinterByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreatePrinterByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Printer{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreatePrinterByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateRemovableMacAddressByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateRemovableMacAddressByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.RemovableMacAddress{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateRemovableMacAddressByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateRestrictedSoftwareByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateRestrictedSoftwareByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.RestrictedSoftware{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateRestrictedSoftwareByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateScriptByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateScriptByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Script{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateScriptByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateSiteByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateSiteByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Site{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateSiteByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateSoftwareUpdateServerByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateSoftwareUpdateServerByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.SoftwareUpdateServer{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateSoftwareUpdateServerByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateUserByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateUserByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.UserPost{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateUserByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateUserExtensionAttributeByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateUserExtensionAttributeByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.UserExtensionAttribute{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateUserExtensionAttributeByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateUserGroupByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateUserGroupByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.UserGroup{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateUserGroupByName transport error: %v", err)
	}
}

func TestAcceptance_Classic_Probe_CreateWebhookByName(t *testing.T) {
	c := accClient(t)
	err := proclassic.New(c).CreateWebhookByName(context.Background(), "sdk-probe-nonexistent-xyz", &proclassic.Webhook{})
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			return
		}
		t.Fatalf("CreateWebhookByName transport error: %v", err)
	}
}
