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
