// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"context"
	"testing"
)

func TestAcceptance_GetDeviceDeclarationReport(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available")
	}

	report, err := c.GetDeviceDeclarationReport(ctx, devices[0].ID)
	if err != nil {
		t.Fatalf("GetDeviceDeclarationReport failed: %v", err)
	}
	t.Logf("Device %s has %d channels", devices[0].ID, len(report.Channels))
	for _, ch := range report.Channels {
		t.Logf("  Channel %s: %d declarations, last report: %s", ch.Channel, len(ch.Declarations), ch.LastReportTime)
		for _, d := range ch.Declarations {
			t.Logf("    %s type=%s status=%s active=%v validity=%s", d.DeclarationIdentifier, d.Type, d.Status, d.Active, d.ValidityState)
		}
	}
}

func TestAcceptance_ListDeclarationReportClients(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	// Get a device report first to find a declaration identifier to query
	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available")
	}

	report, err := c.GetDeviceDeclarationReport(ctx, devices[0].ID)
	if err != nil {
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

	clients, err := c.ListDeclarationReportClients(ctx, declID, nil)
	if err != nil {
		t.Fatalf("ListDeclarationReportClients(%s) failed: %v", declID, err)
	}
	t.Logf("Declaration %s reported by %d clients", declID, len(clients))
	for _, cl := range clients {
		t.Logf("  device=%s channel=%s active=%v validity=%s", cl.DeviceID, cl.Channel, cl.Active, cl.ValidityState)
	}
}
