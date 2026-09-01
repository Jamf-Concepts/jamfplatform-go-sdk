// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAcceptance_Classic_AltIdentifierComputersStillRouted pins the twelve
// operations build v1988 deleted from the published Classic spec — GET, PUT
// and DELETE on each of /computers/{macaddress,name,serialnumber,udid}/{value}
// — as live at the gateway. `capi` is held at v1897 precisely because they are:
// none of the twelve declares an x-successor-endpoint, the same bundle's
// _permissions/routes.yaml still grants all four verbs on all four paths, and
// authorization-policies retains every allow block.
//
// Every request uses an identifier no computer can have, so the destructive
// verbs have nothing to mutate or delete: resource resolution answers before
// anything else runs. A routed-but-absent computer is Jamf Pro's own 404; an
// unrouted gateway path is a repeated 403 BAD_PERMISSIONS. So a 404 here is the
// assertion, and a 403 is the signal that the withdrawal has reached the
// server.
//
// This test pins a hold, not a capability. It should fail the day the gateway
// stops routing these paths — at which point take the v1988 removal into
// config.json rather than relaxing the assertion.
func TestAcceptance_Classic_AltIdentifierComputersStillRouted(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()

	suffix := runSuffix()
	body := &proclassic.ComputerPost{
		General: &proclassic.ComputerPostGeneral{
			AssetTag: classicStrPtr("sdk-acc-" + suffix),
		},
	}

	cases := []struct {
		identifier string
		value      string
		get        func(string) error
		update     func(string) error
		remove     func(string) error
	}{
		{
			identifier: "name",
			value:      "sdk-acc-no-such-computer-" + suffix,
			get:        func(v string) error { _, err := pc.GetComputerByName(ctx, v); return err },
			update:     func(v string) error { return pc.UpdateComputerByName(ctx, v, body) },
			remove:     func(v string) error { return pc.DeleteComputerByName(ctx, v) },
		},
		{
			identifier: "serialnumber",
			value:      "SDKNOSUCH" + suffix,
			get:        func(v string) error { _, err := pc.GetComputerBySerialNumber(ctx, v); return err },
			update:     func(v string) error { return pc.UpdateComputerBySerialNumber(ctx, v, body) },
			remove:     func(v string) error { return pc.DeleteComputerBySerialNumber(ctx, v) },
		},
		{
			identifier: "udid",
			value:      "00000000-0000-0000-0000-000000000000",
			get:        func(v string) error { _, err := pc.GetComputerByUDID(ctx, v); return err },
			update:     func(v string) error { return pc.UpdateComputerByUDID(ctx, v, body) },
			remove:     func(v string) error { return pc.DeleteComputerByUDID(ctx, v) },
		},
		{
			identifier: "macaddress",
			value:      "00:00:00:00:00:00",
			get:        func(v string) error { _, err := pc.GetComputerByMacAddress(ctx, v); return err },
			update:     func(v string) error { return pc.UpdateComputerByMacAddress(ctx, v, body) },
			remove:     func(v string) error { return pc.DeleteComputerByMacAddress(ctx, v) },
		},
	}

	for _, tc := range cases {
		for _, verb := range []struct {
			method string
			call   func(string) error
		}{
			{"GET", tc.get},
			{"PUT", tc.update},
			{"DELETE", tc.remove},
		} {
			label := verb.method + " /computers/" + tc.identifier + "/" + tc.value
			apiErr := requireAPIError(t, label, verb.call(tc.value))
			if !apiErr.HasStatus(404) {
				t.Errorf("%s: HasStatus(404) = false, StatusCode=%d — a 403 means the v1988 withdrawal reached the gateway; take the removal", label, apiErr.StatusCode)
				continue
			}
			t.Logf("%s: 404, routed", label)
		}
	}
}

// TestAcceptance_Classic_GetComputerByName reads a real enrolled computer by
// name, the one v1988-withdrawn read with no other acceptance coverage. Names
// are not unique in Classic; the server resolves a duplicate to the lowest id,
// so this asserts only that the lookup resolves to a populated record.
func TestAcceptance_Classic_GetComputerByName(t *testing.T) {
	c := accClient(t)
	pc := proclassic.New(c)
	ctx := context.Background()

	list, err := pc.ListComputers(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputers: %v", err)
	}
	if list == nil || len(list.Computers) == 0 {
		t.Skip("tenant has no computers enrolled")
	}
	var name string
	for _, comp := range list.Computers {
		if comp.Name != nil && *comp.Name != "" {
			name = *comp.Name
			break
		}
	}
	if name == "" {
		t.Fatalf("no computer in ListComputers carries a name: %+v", list.Computers)
	}

	got, err := pc.GetComputerByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerByName(%q): %v", name, err)
	}
	if got == nil || got.General == nil || got.General.ID == nil {
		t.Fatalf("expected Computer.General.ID populated, got %+v", got)
	}
	t.Logf("GetComputerByName(%q) resolved id=%d", name, *got.General.ID)
}
