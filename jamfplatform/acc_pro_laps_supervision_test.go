// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Batch 15 — LAPS v2 + supervision-identities. LAPS settings round-trip
// safely. Per-account endpoints require a clientManagementID that
// identifies a real managed device; we probe with a bogus id and
// tolerate 4xx. Supervision-identities exercises full CRUD against an
// ephemeral identity with deterministic cleanup.

// --- LAPS settings + pending rotations --------------------------------

func TestAcceptance_Pro_LAPS_SettingsV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetLAPSSettingsV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetLAPSSettingsV2: %v", err)
	}
	t.Logf("LAPS settings: autoDeploy=%v autoRotate=%v", current.AutoDeployEnabled, current.AutoRotateEnabled)

	req := &pro.LapsSettingsRequestV2{
		AutoDeployEnabled:        current.AutoDeployEnabled,
		AutoRotateEnabled:        current.AutoRotateEnabled,
		AutoRotateExpirationTime: current.AutoRotateExpirationTime,
		PasswordRotationTime:     current.PasswordRotationTime,
	}
	if _, err := p.UpdateLAPSSettingsV2(ctx, req); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateLAPSSettingsV2 round-trip: %v", err)
	}
}

func TestAcceptance_Pro_LAPS_PendingRotationsV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	pending, err := pro.New(c).GetLAPSPendingRotationsV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetLAPSPendingRotationsV2: %v", err)
	}
	t.Logf("LAPS pending rotations retrieved: %+v", pending)
}

// --- LAPS per-device probes -------------------------------------------

// Per-client endpoints require a real clientManagementID. Probe with a
// bogus id and tolerate 4xx — a real device fixture would be needed to
// exercise the happy path.
func TestAcceptance_Pro_LAPS_PerClientProbesV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	const bogus = "00000000-0000-0000-0000-000000000000"

	tolerate := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Logf("%s: unexpectedly succeeded for bogus id", label)
			return
		}
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("%s: status=%d — expected for bogus id", label, apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("%s: %v", label, err)
	}

	_, err := p.ListLAPSAccountsV2(ctx, bogus)
	tolerate("ListLAPSAccountsV2", err)

	_, err = p.ListLAPSHistoryV2(ctx, bogus)
	tolerate("ListLAPSHistoryV2", err)

	_, err = p.GetLAPSAccountAuditV2(ctx, bogus, "admin")
	tolerate("GetLAPSAccountAuditV2", err)

	_, err = p.GetLAPSAccountHistoryV2(ctx, bogus, "admin")
	tolerate("GetLAPSAccountHistoryV2", err)

	_, err = p.GetLAPSAccountPasswordV2(ctx, bogus, "admin")
	tolerate("GetLAPSAccountPasswordV2", err)

	_, err = p.GetLAPSAccountGuidAuditV2(ctx, bogus, "admin", bogus)
	tolerate("GetLAPSAccountGuidAuditV2", err)

	_, err = p.GetLAPSAccountGuidHistoryV2(ctx, bogus, "admin", bogus)
	tolerate("GetLAPSAccountGuidHistoryV2", err)

	_, err = p.GetLAPSAccountGuidPasswordV2(ctx, bogus, "admin", bogus)
	tolerate("GetLAPSAccountGuidPasswordV2", err)

	// SetLAPSPasswordV2 is destructive — only attempt with bogus ids so
	// it can't corrupt a real device's password. Expect 4xx rejection.
	_, err = p.SetLAPSPasswordV2(ctx, bogus, &pro.LapsUserPasswordRequestV2{})
	tolerate("SetLAPSPasswordV2", err)
}

// --- supervision-identities -------------------------------------------

func TestAcceptance_Pro_SupervisionIdentitiesV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// List is always available.
	existing, err := p.ListSupervisionIdentitiesV1(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListSupervisionIdentitiesV1: %v", err)
	}
	t.Logf("Supervision identities: %d existing", len(existing))

	// Full CRUD lifecycle. Server generates a certificate from the
	// display name + password; password is write-only and not echoed
	// back on GET.
	name := "sdk-acc-supervision-" + runSuffix()
	created, err := p.CreateSupervisionIdentityV1(ctx, &pro.SupervisionIdentityCreate{
		DisplayName: name,
		Password:    "sdk-acc-test-pwd",
	})
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("CreateSupervisionIdentityV1 rejected: status=%d — tenant may not allow supervision identities", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("CreateSupervisionIdentityV1: %v", err)
	}
	id := strconv.Itoa(created.ID)
	t.Logf("Created supervision identity id=%s displayName=%s", id, created.DisplayName)
	cleanupDelete(t, "SupervisionIdentity "+id, func() error { return p.DeleteSupervisionIdentityV1(ctx, id) })

	got, err := p.GetSupervisionIdentityV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSupervisionIdentityV1(%s): %v", id, err)
	}
	if got.DisplayName != name {
		t.Errorf("displayName round-trip mismatch: got %q, want %q", got.DisplayName, name)
	}

	if _, err := p.UpdateSupervisionIdentityV1(ctx, id, &pro.SupervisionIdentityUpdate{
		DisplayName: name + "-upd",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateSupervisionIdentityV1: %v", err)
	}

	// Download returns the p12 cert bundle.
	body, err := p.DownloadSupervisionIdentityV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DownloadSupervisionIdentityV1: %v", err)
	}
	t.Logf("DownloadSupervisionIdentityV1: %d bytes", len(body))
}

// Upload exercises the alternate create path (existing cert bundle vs.
// server-generated). An empty payload will 4xx; we tolerate, since a
// valid p12 + password would require a real code-signing fixture.
func TestAcceptance_Pro_SupervisionIdentitiesUploadProbeV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	_, err := pro.New(c).UploadSupervisionIdentityV1(ctx, &pro.SupervisionIdentityCertificateUpload{
		DisplayName: "sdk-acc-upload-probe-" + runSuffix(),
		Password:    "unused",
	})
	if err == nil {
		t.Log("UploadSupervisionIdentityV1 unexpectedly succeeded with empty certificateData")
		return
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("UploadSupervisionIdentityV1 rejected: status=%d — expected without a valid p12 fixture", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Fatalf("UploadSupervisionIdentityV1: %v", err)
}
