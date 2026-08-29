// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Batch 21 — misc reads. All 14 endpoints are read-only metadata
// probes (health, versions, locales, zones, cloud info, jamf-package,
// static user groups, device-extension-attribute + device-group
// lookups). No lifecycle — just confirm they resolve and decode.

func TestAcceptance_Pro_MiscReadsV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	if err := p.HealthCheckV1(ctx); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("HealthCheckV1: %v", err)
	}
	t.Log("HealthCheckV1: 204")

	status, err := p.GetHealthStatusV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetHealthStatusV1: %v", err)
	}
	t.Logf("HealthStatus: %+v", status)

	ver, err := p.GetJamfProVersionV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetJamfProVersionV1: %v", err)
	}
	t.Logf("Jamf Pro version: %+v", ver)

	info, err := p.GetJamfProInformationV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetJamfProInformationV2: %v", err)
	}
	t.Logf("Jamf Pro information v2 retrieved: %+v", info)

	locales, err := p.ListLocalesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListLocalesV1: %v", err)
	}
	t.Logf("Locales: %d", len(locales))

	tzs, err := p.ListTimeZonesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListTimeZonesV1: %v", err)
	}
	t.Logf("Time zones: %d", len(tzs))

	// Asserted to fail, deliberately. GET /api/pro/v2/environment-type has no
	// rule in jamf/authorization-policies and never has had one — checked across
	// origin/main, all 60+ remote branches and all open PRs on 2026-08-29 — and
	// every domain's _default.rego carries `default allow := false`, so the
	// authorization service returns 403 BAD_PERMISSIONS by construction. The spec
	// declares no required privilege, so no scope grant can clear it either, and
	// the path was dropped from the published spec in public-apis-oas#395.
	//
	// A skip would report success while verifying nothing, so this asserts the
	// 403 and fails on anything else — including a 200. If it starts passing, a
	// policy has been authored: flip this to assert the payload and delete this
	// comment. The reads either side of it are unaffected.
	env, err := p.GetEnvironmentTypeV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if !errors.As(err, &apiErr) || !apiErr.HasStatus(403) {
			t.Fatalf("GetEnvironmentTypeV2: want 403 BAD_PERMISSIONS (no authorization policy exists for this path), got %v", err)
		}
		t.Logf("GetEnvironmentTypeV2: 403 as expected — no authorization policy exists for this path")
	} else {
		t.Errorf("GetEnvironmentTypeV2 unexpectedly succeeded: a policy has been authored for /v2/environment-type. "+
			"Update this test to assert the payload. Got environment=%q", env.Environment)
		t.Logf("Cloud services environment: %s", env.Environment)
		// Probed direct against an 11.31.0 sandbox instance (2026-08-16): returned
		// "production", one of the three values the spec enumerates.
		if !slices.Contains(pro.EnvironmentTypeEnvironmentValues(), env.Environment) {
			t.Errorf("environment %q absent from the generated constants %v — spec enum has drifted",
				env.Environment, pro.EnvironmentTypeEnvironmentValues())
		}
	}

	cloud, err := p.GetCloudInformationV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetCloudInformationV1: %v", err)
	}
	t.Logf("Cloud info: %+v", cloud)

	codes, err := p.ListAppStoreCountryCodesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAppStoreCountryCodesV1: %v", err)
	}
	t.Logf("App-store country codes retrieved: %+v", codes)
}

func TestAcceptance_Pro_JamfPackageV1V2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// `application` query is required by the endpoint. PROTECT is a
	// known app that every tenant should have an answer for.
	const app = "PROTECT"

	v1, err := p.ListJamfPackagesV1(ctx, app)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListJamfPackagesV1: %v", err)
	}
	t.Logf("JamfPackage v1 for %q: %d entries", app, len(v1))

	v2, err := p.GetJamfPackageV2(ctx, app)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetJamfPackageV2: %v", err)
	}
	t.Logf("JamfPackage v2 for %q retrieved: %+v", app, v2)
}

func TestAcceptance_Pro_StaticUserGroupsV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	groups, err := p.ListStaticUserGroupsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListStaticUserGroupsV1: %v", err)
	}
	t.Logf("Static user groups: %d", len(groups))

	// Probe GET by-id with a bogus id — tolerate 404 since we don't
	// assume any group exists on the tenant.
	if _, err := p.GetStaticUserGroupV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			t.Logf("GetStaticUserGroupV1(-1): 404 — expected for bogus id")
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetStaticUserGroupV1: %v", err)
		}
	}
}

func TestAcceptance_Pro_DeviceExtensionAttributesPreview(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	// Spec's default for `select` is "name"; server 400s when omitted.
	attrs, err := pro.New(c).ListDeviceExtensionAttributesPreview(ctx, "name")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListDeviceExtensionAttributesPreview: %v", err)
	}
	t.Logf("Mobile device extension attributes (preview): %+v", attrs)
}

func TestAcceptance_Pro_DeviceGroupsForDeviceV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Probe with a bogus device id — tolerate 404 so we don't need a
	// known-managed-device fixture.
	if _, err := p.GetDeviceGroupsForDeviceV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("GetDeviceGroupsForDeviceV1(-1): status=%d — expected for bogus device id", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetDeviceGroupsForDeviceV1: %v", err)
	}
	t.Log("GetDeviceGroupsForDeviceV1(-1) unexpectedly succeeded")
}
