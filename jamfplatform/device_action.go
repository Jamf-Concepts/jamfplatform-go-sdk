// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Device Management Action API client

package jamfplatform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Device Management Action API path constants.
const deviceActionsNamespace = "device-actions"

// DeviceCommandResponseV1 captures the command metadata returned by device actions.
type DeviceCommandResponseV1 struct {
	DeviceID  string `json:"deviceId"`
	CommandID string `json:"commandId"`
}

// EraseDeviceRequestV1 holds the optional payload properties for erase.
type EraseDeviceRequestV1 struct {
	PreserveDataPlan       *bool   `json:"preserveDataPlan,omitempty"`
	DisallowProximitySetup *bool   `json:"disallowProximitySetup,omitempty"`
	ClearActivationLock    *bool   `json:"clearActivationLock,omitempty"`
	ReturnToService        *bool   `json:"returnToService,omitempty"`
	Pin                    *string `json:"pin,omitempty"`
}

// CheckInDevice requests that the specified device check for pending commands.
func (c *Client) CheckInDevice(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("deviceID cannot be empty")
	}

	prefix := c.tenantPrefix(deviceActionsNamespace, "v1")
	endpoint := fmt.Sprintf("%s/devices/%s/check-in", prefix, url.PathEscape(id))
	if err := c.transport.DoExpect(ctx, http.MethodPost, endpoint, nil, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("check-in device %s: %w", id, err)
	}
	return nil
}

// EraseDevice requests that the specified device erase its content and settings.
func (c *Client) EraseDevice(ctx context.Context, id string, request *EraseDeviceRequestV1) ([]DeviceCommandResponseV1, error) {
	return c.invokeDeviceManagementAction(ctx, id, "erase", request)
}

// RestartDevice requests that the specified device perform a restart.
func (c *Client) RestartDevice(ctx context.Context, id string) ([]DeviceCommandResponseV1, error) {
	return c.invokeDeviceManagementAction(ctx, id, "restart", nil)
}

// ShutdownDevice requests that the specified device shut down.
func (c *Client) ShutdownDevice(ctx context.Context, id string) ([]DeviceCommandResponseV1, error) {
	return c.invokeDeviceManagementAction(ctx, id, "shutdown", nil)
}

// UnmanageDevice requests that the specified device remove remote management.
func (c *Client) UnmanageDevice(ctx context.Context, id string) ([]DeviceCommandResponseV1, error) {
	return c.invokeDeviceManagementAction(ctx, id, "unmanage", nil)
}

// invokeDeviceManagementAction is a helper to call device management action endpoints.
func (c *Client) invokeDeviceManagementAction(ctx context.Context, deviceID, action string, payload any) ([]DeviceCommandResponseV1, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("deviceID cannot be empty")
	}

	prefix := c.tenantPrefix(deviceActionsNamespace, "v1")
	endpoint := fmt.Sprintf("%s/devices/%s/%s", prefix, url.PathEscape(deviceID), action)
	var result []DeviceCommandResponseV1
	if err := c.transport.DoExpect(ctx, http.MethodPost, endpoint, payload, http.StatusCreated, &result); err != nil {
		return nil, fmt.Errorf("%s device %s: %w", action, deviceID, err)
	}
	return result, nil
}
