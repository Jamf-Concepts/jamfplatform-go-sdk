// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devicegroups"
)

func createStaticTestGroup(t *testing.T, dg *devicegroups.Client, name string) string {
	t.Helper()
	ctx := context.Background()
	desc := "SDK acceptance test — safe to delete"
	empty := []string{}
	resp, err := dg.CreateDeviceGroup(ctx, &devicegroups.DeviceGroupCreateRepresentationV1{
		Name:        name,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		Members:     &empty,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDeviceGroup(%q): %v", name, err)
	}
	cleanupDelete(t, "DeleteDeviceGroup", func() error { return dg.DeleteDeviceGroup(ctx, resp.ID) })
	return resp.ID
}

func TestAcceptance_ResolveDeviceGroupIDByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	dg := devicegroups.New(c)

	name := "sdk-acc-resolver-dg-id-" + runSuffix()
	wantID := createStaticTestGroup(t, dg, name)

	gotID, err := dg.ResolveDeviceGroupIDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDeviceGroupIDByName(%q): %v", name, err)
	}
	if gotID != wantID {
		t.Errorf("resolved ID = %q, want %q", gotID, wantID)
	}
	t.Logf("Resolved %q -> %s (filtered mode, server-side RSQL)", name, gotID)
}

func TestAcceptance_ResolveDeviceGroupByName(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	dg := devicegroups.New(c)

	name := "sdk-acc-resolver-dg-typed-" + runSuffix()
	wantID := createStaticTestGroup(t, dg, name)

	got, err := dg.ResolveDeviceGroupByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ResolveDeviceGroupByName(%q): %v", name, err)
	}
	if got == nil {
		t.Fatal("ResolveDeviceGroupByName returned nil without error")
	}
	if got.ID != wantID {
		t.Errorf("typed result ID = %q, want %q", got.ID, wantID)
	}
	if got.Name != name {
		t.Errorf("typed result Name = %q, want %q", got.Name, name)
	}
	if got.GroupType != "STATIC" {
		t.Errorf("typed result GroupType = %q, want STATIC", got.GroupType)
	}
	t.Logf("Resolved typed %q -> ID %s (%s)", name, got.ID, got.GroupType)
}

func TestAcceptance_ResolveDeviceGroupIDByName_NotFound(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	dg := devicegroups.New(c)

	probe := "sdk-does-not-exist-dg-" + runSuffix()
	_, err := dg.ResolveDeviceGroupIDByName(ctx, probe)
	if err == nil {
		t.Fatalf("expected not-found error for %q, got nil", probe)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIResponseError, got %T: %v", err, err)
	}
	if !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected status 404, got %d: %v", apiErr.StatusCode, err)
	}
	t.Logf("Not-found probe surfaced APIResponseError(404) as expected")
}

func TestAcceptance_ResolveDeviceGroup_Ambiguous(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	dg := devicegroups.New(c)

	shared := "sdk-acc-resolver-dg-dup-" + runSuffix()
	firstID := createStaticTestGroup(t, dg, shared)

	// Try to create a second group with the same name. If the server
	// enforces uniqueness (4xx), the resolver has nothing to disambiguate
	// — log and skip rather than failing the whole resolver path.
	desc := "SDK acceptance test — duplicate-name probe"
	empty := []string{}
	resp, err := dg.CreateDeviceGroup(ctx, &devicegroups.DeviceGroupCreateRepresentationV1{
		Name:        shared,
		Description: &desc,
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		Members:     &empty,
	})
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Skipf("server rejects duplicate device-group names (%d) — nothing to disambiguate: %v", apiErr.StatusCode, apiErr.Summary())
		}
		skipOnServerError(t, err)
		t.Fatalf("CreateDeviceGroup (duplicate) failed unexpectedly: %v", err)
	}
	cleanupDelete(t, "DeleteDeviceGroup", func() error { return dg.DeleteDeviceGroup(ctx, resp.ID) })

	_, err = dg.ResolveDeviceGroupIDByName(ctx, shared)
	if err == nil {
		t.Fatalf("expected ambiguous match error for duplicate name %q, got nil", shared)
	}
	var amErr *jamfplatform.AmbiguousMatchError
	if !errors.As(err, &amErr) {
		t.Fatalf("expected *AmbiguousMatchError, got %T: %v", err, err)
	}
	if len(amErr.Matches) < 2 {
		t.Errorf("AmbiguousMatchError.Matches = %v, want at least 2", amErr.Matches)
	}
	foundFirst, foundSecond := false, false
	for _, m := range amErr.Matches {
		if m == firstID {
			foundFirst = true
		}
		if m == resp.ID {
			foundSecond = true
		}
	}
	if !foundFirst || !foundSecond {
		t.Errorf("AmbiguousMatchError.Matches = %v, want to contain both %q and %q", amErr.Matches, firstID, resp.ID)
	}
	t.Logf("Ambiguous device-group resolve surfaced %d matches: %v", len(amErr.Matches), amErr.Matches)
}
