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

// Batch 13 — settings singletons. Each resource follows the GET / PUT /
// history pattern. Tests favour GET+round-trip-PUT where the request
// and response types match; when they diverge (login-customization,
// teacher-app) we construct the request from the response. Destructive
// singletons (activation-code, jamf-pro-server-url) are GET+history
// only — changing the live values would break the tenant.

// --- activation code ---------------------------------------------------

// Activation code PUT is intentionally not exercised — changing the live
// activation code would break licensing for the tenant. We cover the
// history surface only. UpdateActivationCodeOrganizationNameV1 is also
// skipped for the same reason (org name is tenant-wide, non-reversible).
func TestAcceptance_Pro_Settings_ActivationCodeHistoryV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	if _, err := p.CreateActivationCodeHistoryNoteV1(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test activation-code history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateActivationCodeHistoryNoteV1: %v", err)
	}

	hist, err := p.ListActivationCodeHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListActivationCodeHistoryV1: %v", err)
	}
	t.Logf("Activation-code history: %d entries", len(hist))

	body, err := p.ExportActivationCodeHistoryV1(ctx, &pro.ExportParameters{})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ExportActivationCodeHistoryV1: %v", err)
	}
	t.Logf("Activation-code history export: %d bytes", len(body))
}

// --- SMTP server v2 ----------------------------------------------------

// SMTP PUT is not round-tripped — echoing the current config back can
// trigger server-side validation on credentials the server redacts on
// GET. Read-only.
func TestAcceptance_Pro_Settings_SmtpServerV2Read(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	s, err := pro.New(c).GetSmtpServerV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSmtpServerV2: %v", err)
	}
	t.Logf("SMTP server: enabled=%v authType=%s", s.Enabled, s.AuthenticationType)
}

func TestAcceptance_Pro_Settings_SmtpServerHistoryV1(t *testing.T) {
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

// TestAcceptance_Pro_Settings_SmtpServerTestV1 sends a test email. Use an
// obviously-synthetic recipient so a misrouted send is easy to spot.
// Tolerate 4xx when the tenant's SMTP isn't configured to relay.
func TestAcceptance_Pro_Settings_SmtpServerTestV1(t *testing.T) {
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

// --- Jamf Pro server URL ----------------------------------------------

// Changing the Jamf Pro server URL would point clients at a different
// host. Read + history only.
func TestAcceptance_Pro_Settings_JamfProServerURLV1Read(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	url, err := p.GetJamfProServerURLV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetJamfProServerURLV1: %v", err)
	}
	t.Logf("Jamf Pro server URL: %s", url.URL)

	// History endpoints on jamf-pro-server-url require elevated
	// permissions the default OAuth client doesn't have. Tolerate 403.
	if _, err := p.CreateJamfProServerURLHistoryNoteV1(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test jamf-pro-server-url history entry",
	}); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
			t.Logf("CreateJamfProServerURLHistoryNoteV1 denied: 403 — elevated permission required")
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("CreateJamfProServerURLHistoryNoteV1: %v", err)
	}

	hist, err := p.ListJamfProServerURLHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListJamfProServerURLHistoryV1: %v", err)
	}
	t.Logf("Jamf Pro server URL history: %d entries", len(hist))
}

// --- device-communication settings ------------------------------------

func TestAcceptance_Pro_Settings_DeviceCommunicationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetDeviceCommunicationSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetDeviceCommunicationSettingsV1: %v", err)
	}
	t.Logf("Device-communication settings retrieved")

	if _, err := p.UpdateDeviceCommunicationSettingsV1(ctx, current); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateDeviceCommunicationSettingsV1 round-trip: %v", err)
	}

	if _, err := p.CreateDeviceCommunicationSettingsHistoryNoteV1(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test device-communication history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDeviceCommunicationSettingsHistoryNoteV1: %v", err)
	}

	hist, err := p.ListDeviceCommunicationSettingsHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListDeviceCommunicationSettingsHistoryV1: %v", err)
	}
	t.Logf("Device-communication history: %d entries", len(hist))
}

// --- check-in v3 ------------------------------------------------------

func TestAcceptance_Pro_Settings_CheckInV3(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetCheckInSettingsV3(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetCheckInSettingsV3: %v", err)
	}
	t.Logf("Check-in settings retrieved")

	if _, err := p.UpdateCheckInSettingsV3(ctx, current); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateCheckInSettingsV3 round-trip: %v", err)
	}

	if _, err := p.CreateCheckInHistoryNoteV3(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test check-in history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateCheckInHistoryNoteV3: %v", err)
	}

	hist, err := p.ListCheckInHistoryV3(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListCheckInHistoryV3: %v", err)
	}
	t.Logf("Check-in history: %d entries", len(hist))
}

// --- cache settings ---------------------------------------------------

// Cache settings drive server-side caching and are tenant-critical.
// Read-only — echo-PUT could surface latent 403s depending on role, and
// the blast radius of a corrupted write is tenant-wide.
func TestAcceptance_Pro_Settings_CacheSettingsV1Read(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	s, err := pro.New(c).GetCacheSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetCacheSettingsV1: %v", err)
	}
	t.Logf("Cache settings: type=%s ttl=%d", s.CacheType, s.TimeToLiveSeconds)
}

// --- login customization ----------------------------------------------

func TestAcceptance_Pro_Settings_LoginCustomizationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetLoginCustomizationV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetLoginCustomizationV1: %v", err)
	}
	t.Logf("Login customization retrieved")

	// Request type is LoginContentPut; GET returns LoginContent. Map
	// the four shared fields (rampInstance is read-only). Server
	// rejects empty required fields even on round-trip, so skip the
	// PUT when the tenant has never populated disclaimer text — the
	// GET surface is still validated.
	if current.ActionText == "" || current.DisclaimerHeading == "" || current.DisclaimerMainText == "" {
		t.Logf("UpdateLoginCustomizationV1 skipped: tenant has empty required fields (server rejects empties)")
		return
	}
	put := &pro.LoginContentPut{
		ActionText:              &current.ActionText,
		DisclaimerHeading:       &current.DisclaimerHeading,
		DisclaimerMainText:      &current.DisclaimerMainText,
		IncludeCustomDisclaimer: current.IncludeCustomDisclaimer,
	}
	if _, err := p.UpdateLoginCustomizationV1(ctx, put); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateLoginCustomizationV1 round-trip: %v", err)
	}
}

// --- parent-app + teacher-app -----------------------------------------

func TestAcceptance_Pro_Settings_ParentAppV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetParentAppSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetParentAppSettingsV1: %v", err)
	}
	t.Logf("Parent-app settings: enabled=%v", current.IsEnabled)

	if _, err := p.UpdateParentAppSettingsV1(ctx, current); err != nil {
		// Round-trip may 400 on tenants without a configured device
		// group — the server enforces deviceGroupId referential
		// integrity on PUT but not on GET. Tolerate client errors.
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("UpdateParentAppSettingsV1 rejected: status=%d — expected on tenants without parent-app fixture", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("UpdateParentAppSettingsV1 round-trip: %v", err)
		}
	}

	if _, err := p.CreateParentAppHistoryNoteV1(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test parent-app history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateParentAppHistoryNoteV1: %v", err)
	}

	hist, err := p.ListParentAppHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListParentAppHistoryV1: %v", err)
	}
	t.Logf("Parent-app history: %d entries", len(hist))
}

func TestAcceptance_Pro_Settings_TeacherAppV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetTeacherAppSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetTeacherAppSettingsV1: %v", err)
	}
	t.Logf("Teacher-app settings: enabled=%v", current.IsEnabled)

	// Request type differs from response — map the writable fields.
	put := &pro.TeacherSettingsRequest{
		AutoClear:                   &current.AutoClear,
		IsEnabled:                   &current.IsEnabled,
		MaxRestrictionLengthSeconds: &current.MaxRestrictionLengthSeconds,
		TimezoneID:                  &current.TimezoneID,
		SafelistedApps:              &current.SafelistedApps,
	}
	if _, err := p.UpdateTeacherAppSettingsV1(ctx, put); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateTeacherAppSettingsV1 round-trip: %v", err)
	}

	if _, err := p.CreateTeacherAppHistoryNoteV1(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test teacher-app history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateTeacherAppHistoryNoteV1: %v", err)
	}

	hist, err := p.ListTeacherAppHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListTeacherAppHistoryV1: %v", err)
	}
	t.Logf("Teacher-app history: %d entries", len(hist))
}

// --- GSX connection ---------------------------------------------------

// GSX is Apple's Global Service Exchange — tenants without a
// provisioned Apple service account will 400 on mutate endpoints.
// Read-only and history are safe.
func TestAcceptance_Pro_Settings_GSXConnectionV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetGSXConnectionV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetGSXConnectionV1: %v", err)
	}
	t.Logf("GSX connection: enabled=%v", current.Enabled)

	if _, err := p.CreateGSXConnectionHistoryNoteV1(ctx, &pro.ObjectHistoryNote{
		Note: "sdk-acc test gsx-connection history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateGSXConnectionHistoryNoteV1: %v", err)
	}

	hist, err := p.ListGSXConnectionHistoryV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListGSXConnectionHistoryV1: %v", err)
	}
	t.Logf("GSX connection history: %d entries", len(hist))

	// Test endpoint calls out to Apple. If no keystore is configured
	// the server returns 400 — that's the expected path on a clean
	// tenant; fail only on 5xx.
	if err := p.TestGSXConnectionV1(ctx); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("TestGSXConnectionV1 rejected: status=%d — expected on tenants without Apple GSX fixture", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("TestGSXConnectionV1: %v", err)
		}
	}
}

// --- impact-alert notification settings -------------------------------

func TestAcceptance_Pro_Settings_ImpactAlertNotificationV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetImpactAlertNotificationSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetImpactAlertNotificationSettingsV1: %v", err)
	}
	t.Logf("Impact-alert settings retrieved")

	if err := p.UpdateImpactAlertNotificationSettingsV1(ctx, current); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateImpactAlertNotificationSettingsV1 round-trip: %v", err)
	}
}

// --- self-service-plus ------------------------------------------------

func TestAcceptance_Pro_Settings_SelfServicePlusV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Feature toggle returns 204 when enabled, 404 when not — both are
	// acceptable. Any other error fails.
	if err := p.GetSelfServicePlusFeatureToggleEnabledV1(ctx); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			t.Logf("Self-service-plus feature toggle: disabled (404)")
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetSelfServicePlusFeatureToggleEnabledV1: %v", err)
		}
	} else {
		t.Logf("Self-service-plus feature toggle: enabled (204)")
	}

	current, err := p.GetSelfServicePlusSettingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSelfServicePlusSettingsV1: %v", err)
	}
	t.Logf("Self-service-plus settings retrieved")

	if err := p.UpdateSelfServicePlusSettingsV1(ctx, current); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateSelfServicePlusSettingsV1 round-trip: %v", err)
	}
}
