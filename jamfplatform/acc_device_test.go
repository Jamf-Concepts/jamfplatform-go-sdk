// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"context"
	"testing"
)

func TestAcceptance_ListDevices(t *testing.T) {
	c := accClient(t)

	devices, err := c.ListDevices(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	t.Logf("Found %d devices", len(devices))
}

func TestAcceptance_ListDevicesWithSort(t *testing.T) {
	c := accClient(t)

	devices, err := c.ListDevices(context.Background(), []string{"name:asc"}, "")
	if err != nil {
		t.Fatalf("ListDevices with sort failed: %v", err)
	}
	t.Logf("Found %d devices (sorted by name asc)", len(devices))
}

func TestAcceptance_GetDevice(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available to read by ID")
	}

	device, err := c.GetDevice(ctx, devices[0].ID)
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if device.ID != devices[0].ID {
		t.Errorf("expected ID %q, got %q", devices[0].ID, device.ID)
	}

	t.Logf("Read device: %s (%s), managed: %v", device.Name, device.ID, device.Managed)
	if device.Hardware != nil {
		t.Logf("  Hardware: %s %s, serial: %s", device.Hardware.Make, device.Hardware.Model, device.Hardware.SerialNumber)
	}
	if device.OperatingSystem != nil {
		t.Logf("  OS: %s %s (build %s)", device.OperatingSystem.Name, device.OperatingSystem.Version, device.OperatingSystem.Build)
	}
}

func TestAcceptance_GetDeviceBySerialNumber(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available for serial number lookup")
	}

	serial := devices[0].SerialNumber
	if serial == "" {
		t.Skip("First device has no serial number")
	}

	device, err := c.GetDeviceBySerialNumber(ctx, serial)
	if err != nil {
		t.Fatalf("GetDeviceBySerialNumber(%s) failed: %v", serial, err)
	}
	if device.ID != devices[0].ID {
		t.Errorf("expected ID %q, got %q", devices[0].ID, device.ID)
	}

	t.Logf("Resolved serial %s to device %s (%s)", serial, device.Name, device.ID)
}

func TestAcceptance_ListDeviceApplications(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available")
	}

	apps, err := c.ListDeviceApplications(ctx, devices[0].ID, nil, "")
	if err != nil {
		t.Fatalf("ListDeviceApplications failed: %v", err)
	}
	t.Logf("Device %s has %d applications", devices[0].ID, len(apps))
}

func TestAcceptance_ListDeviceGroupsForDevice(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	devices, err := c.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("No devices available")
	}

	groups, err := c.ListDeviceGroupsForDevice(ctx, devices[0].ID)
	if err != nil {
		t.Fatalf("ListDeviceGroupsForDevice failed: %v", err)
	}
	t.Logf("Device %s belongs to %d groups", devices[0].ID, len(groups))
	for _, g := range groups {
		t.Logf("  Group: %s (%s)", g.GroupName, g.GroupID)
	}
}
