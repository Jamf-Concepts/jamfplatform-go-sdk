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

// Misc Pro endpoints that don't cluster with any other resource theme:
// startup-status, mobile-devices detail (oneOf/discriminator round-trip
// check), change-user-password (expected-rejection probe).

func TestAcceptance_Pro_GetStartupStatus(t *testing.T) {
	c := accClient(t)

	status, err := pro.New(c).GetStartupStatus(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetStartupStatus failed: %v", err)
	}
	t.Logf("Startup status: step=%s stepCode=%s percentage=%d", status.Step, status.StepCode, status.Percentage)
}

// TestAcceptance_Pro_ListMobileDevicesDetail exercises the oneOf/discriminator
// path: the response carries a paginated slice of MobileDeviceResponse where
// each element is one of iOS / tvOS / watchOS variants keyed by the
// deviceType discriminator. The generated UnmarshalJSON dispatches each
// element to the matching variant pointer.
func TestAcceptance_Pro_ListMobileDevicesDetail(t *testing.T) {
	c := accClient(t)

	devices, err := pro.New(c).ListMobileDevicesDetailV2(context.Background(), nil, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListMobileDevicesDetailV2: %v", err)
	}
	t.Logf("Found %d mobile devices", len(devices))
	for i, d := range devices {
		if i >= 5 {
			break
		}
		switch d.DeviceType {
		case "iOS":
			if d.IOS == nil {
				t.Errorf("device[%d] DeviceType=iOS but IOS variant is nil", i)
			}
		case "tvOS":
			if d.TvOS == nil {
				t.Errorf("device[%d] DeviceType=tvOS but TvOS variant is nil", i)
			}
		case "watchOS":
			if d.WatchOS == nil {
				t.Errorf("device[%d] DeviceType=watchOS but WatchOS variant is nil", i)
			}
		}
		t.Logf("device[%d] type=%s", i, d.DeviceType)
	}
}

// TestAcceptance_Pro_ChangeUserPassword intentionally calls with a
// clearly-wrong current password and expects the API to reject. The
// alternative — actually rotating a credential — would lock out either the
// OAuth API client (our test auth) or an admin user. The test still
// exercises the transport path and payload encoding end-to-end.
func TestAcceptance_Pro_ChangeUserPassword(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	err := pro.New(c).ChangeUserPasswordV1(ctx, &pro.ChangePassword{
		CurrentPassword: "sdk-acc-clearly-not-valid-" + runSuffix(),
		NewPassword:     "sdk-acc-unused",
	})
	if err == nil {
		t.Fatal("expected server to reject wrong currentPassword, got nil error (did credentials actually rotate?)")
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) {
		t.Logf("ChangeUserPasswordV1 rejected as expected: status=%d", apiErr.StatusCode)
		return
	}
	t.Logf("ChangeUserPasswordV1 rejected as expected: %v", err)
}
