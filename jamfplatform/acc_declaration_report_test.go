// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/ddmreport"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devices"
)

func TestAcceptance_GetDeviceDeclarationReport(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	d, err := devices.New(c).ListDevices(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(d) == 0 {
		t.Skip("No devices available")
	}

	report, err := ddmreport.New(c).GetDeviceDeclarationReport(ctx, d[0].ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetDeviceDeclarationReport failed: %v", err)
	}
	t.Logf("Device %s has %d channels", d[0].ID, len(report.Channels))
	for _, ch := range report.Channels {
		t.Logf("  Channel %s: %d declarations, last report: %s", ch.Channel, len(ch.Declarations), ch.LastReportTime)
		for _, decl := range ch.Declarations {
			t.Logf("    %s type=%s status=%s active=%v validity=%s", decl.DeclarationIdentifier, decl.Type, decl.Status, decl.Active, decl.ValidityState)
		}
	}
}

func TestAcceptance_ListDeclarationReportClients(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	dr := ddmreport.New(c)

	// Get a device report first to find a declaration identifier to query
	d, err := devices.New(c).ListDevices(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(d) == 0 {
		t.Skip("No devices available")
	}

	report, err := dr.GetDeviceDeclarationReport(ctx, d[0].ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetDeviceDeclarationReport failed: %v", err)
	}

	var declID string
	for _, ch := range report.Channels {
		if len(ch.Declarations) > 0 {
			declID = ch.Declarations[0].DeclarationIdentifier
			break
		}
	}
	if declID == "" {
		t.Skip("No declarations found on any device channel")
	}

	clients, err := dr.ListDeclarationReportClients(ctx, declID, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListDeclarationReportClients(%s) failed: %v", declID, err)
	}
	t.Logf("Declaration %s reported by %d clients", declID, len(clients))
	for _, cl := range clients {
		t.Logf("  device=%s channel=%s active=%v validity=%s", cl.DeviceID, cl.Channel, cl.Active, cl.ValidityState)
	}
}
