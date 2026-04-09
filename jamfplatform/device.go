// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Device Inventory API client
// https://developer.jamf.com/platform-api/reference/devices

package jamfplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/internal/client"
)

// Device Inventory API path constants.
const deviceNamespace = "devices"

// DeviceListReadRepresentationV1 represents a device in a list response.
type DeviceListReadRepresentationV1 struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Model                   string  `json:"model"`
	ModelIdentifier         string  `json:"modelIdentifier"`
	SerialNumber            string  `json:"serialNumber"`
	LastInventoryUpdateTime string  `json:"lastInventoryUpdateTime"`
	LastCheckInTime         *string `json:"lastCheckInTime,omitempty"`
	OperatingSystemVersion  string  `json:"operatingSystemVersion"`
	UserID                  *string `json:"userId,omitempty"`
	EnrollmentType          string  `json:"enrollmentType"`
	LastEnrollmentTime      string  `json:"lastEnrollmentTime"`
}

// DeviceReadRepresentationV1 represents the full device payload.
type DeviceReadRepresentationV1 struct {
	ID                      string                                     `json:"id"`
	Name                    string                                     `json:"name"`
	LastInventoryUpdateTime string                                     `json:"lastInventoryUpdateTime"`
	LastCheckInTime         *string                                    `json:"lastCheckInTime,omitempty"`
	UserID                  *string                                    `json:"userId,omitempty"`
	EnrollmentType          string                                     `json:"enrollmentType"`
	LastEnrollmentTime      string                                     `json:"lastEnrollmentTime"`
	Managed                 bool                                       `json:"managed"`
	MDMCapable              bool                                       `json:"mdmCapable"`
	Supervised              bool                                       `json:"supervised"`
	Hardware                *DeviceHardwareReadRepresentationV1        `json:"hardware,omitempty"`
	Network                 *DeviceNetworkReadRepresentationV1         `json:"network,omitempty"`
	OperatingSystem         *DeviceOperatingSystemReadRepresentationV1 `json:"operatingSystem,omitempty"`
	Security                *DeviceSecurityReadRepresentationV1        `json:"security,omitempty"`
}

// DeviceHardwareReadRepresentationV1 represents the hardware section of a device.
type DeviceHardwareReadRepresentationV1 struct {
	Make            string `json:"make"`
	Model           string `json:"model"`
	ModelIdentifier string `json:"modelIdentifier"`
	UDID            string `json:"udid"`
	SerialNumber    string `json:"serialNumber"`
	BatteryHealth   string `json:"batteryHealth"`
	MacAddress      string `json:"macAddress"`
	StorageCapacity int    `json:"storageCapacity"`
	StorageUsed     int    `json:"storageUsed"`
}

// DeviceOperatingSystemReadRepresentationV1 represents operating system information.
type DeviceOperatingSystemReadRepresentationV1 struct {
	Name                     string  `json:"name"`
	Version                  string  `json:"version"`
	Build                    string  `json:"build"`
	SupplementalBuildVersion *string `json:"supplementalBuildVersion,omitempty"`
	RapidSecurityResponse    *string `json:"rapidSecurityResponse,omitempty"`
}

// DeviceSecurityReadRepresentationV1 represents security information for a device.
type DeviceSecurityReadRepresentationV1 struct {
	BootstrapTokenEscrowedStatus string `json:"bootstrapTokenEscrowedStatus"`
	HardwareEncryption           *bool  `json:"hardwareEncryption,omitempty"`
	PasscodePresent              *bool  `json:"passcodePresent,omitempty"`
	PasscodeCompliant            *bool  `json:"passcodeCompliant,omitempty"`
	LostModeEnabled              *bool  `json:"lostModeEnabled,omitempty"`
}

// DeviceNetworkReadRepresentationV1 represents network information for a device.
type DeviceNetworkReadRepresentationV1 struct {
	LastIPAddress           *string `json:"lastIpAddress,omitempty"`
	LastReportedIPv4Address *string `json:"lastReportedIpV4Address,omitempty"`
	LastReportedIPv6Address *string `json:"lastReportedIpV6Address,omitempty"`
}

// DeviceInstalledApplicationReadRepresentationV1 represents an installed application on a device.
type DeviceInstalledApplicationReadRepresentationV1 struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DeviceUpdateRepresentationV1 represents the payload used to update a device.
type DeviceUpdateRepresentationV1 struct {
	Name   *string         `json:"name,omitempty"`
	UserID *NullableString `json:"userId,omitempty"`
}

// NullableString helps differentiate between explicit null and omitted string fields.
type NullableString struct {
	Value  string
	IsNull bool
}

// MarshalJSON implements json.Marshaler to emit either the string value or null.
func (ns NullableString) MarshalJSON() ([]byte, error) {
	if ns.IsNull {
		return []byte("null"), nil
	}
	return json.Marshal(ns.Value)
}

// NewNullableString returns a NullableString with a concrete value.
func NewNullableString(value string) *NullableString {
	return &NullableString{Value: value}
}

// NewNullableStringNull returns a NullableString that marshals to JSON null.
func NewNullableStringNull() *NullableString {
	return &NullableString{IsNull: true}
}

// ListDevices returns all devices, automatically handling pagination.
func (c *Client) ListDevices(ctx context.Context, sort []string, filter string) ([]DeviceListReadRepresentationV1, error) {
	prefix := c.tenantPrefix(deviceNamespace, "v1")
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]DeviceListReadRepresentationV1, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))
		if len(sort) > 0 {
			params.Set("sort", strings.Join(sort, ","))
		}
		if filter != "" {
			params.Set("filter", filter)
		}

		endpoint := prefix + "/devices"
		if encoded := params.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}

		var result struct {
			client.PaginatedResponseRepresentation
			Results []DeviceListReadRepresentationV1 `json:"results"`
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
	})
}

// GetDevice retrieves a device by ID.
func (c *Client) GetDevice(ctx context.Context, id string) (*DeviceReadRepresentationV1, error) {
	prefix := c.tenantPrefix(deviceNamespace, "v1")
	var result DeviceReadRepresentationV1
	endpoint := fmt.Sprintf("%s/devices/%s", prefix, url.PathEscape(id))
	if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("GetDevice(%s): %w", id, err)
	}
	return &result, nil
}

// UpdateDevice updates an existing device.
func (c *Client) UpdateDevice(ctx context.Context, id string, payload *DeviceUpdateRepresentationV1) error {
	prefix := c.tenantPrefix(deviceNamespace, "v1")
	endpoint := fmt.Sprintf("%s/devices/%s", prefix, url.PathEscape(id))
	if err := c.transport.DoExpect(ctx, http.MethodPatch, endpoint, payload, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("UpdateDevice(%s): %w", id, err)
	}
	return nil
}

// DeleteDevice deletes a device by ID.
func (c *Client) DeleteDevice(ctx context.Context, id string) error {
	prefix := c.tenantPrefix(deviceNamespace, "v1")
	endpoint := fmt.Sprintf("%s/devices/%s", prefix, url.PathEscape(id))
	if err := c.transport.DoExpect(ctx, http.MethodDelete, endpoint, nil, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("DeleteDevice(%s): %w", id, err)
	}
	return nil
}

// ListDeviceApplications returns all installed applications for a device, handling pagination internally.
func (c *Client) ListDeviceApplications(ctx context.Context, deviceID string, sort []string, filter string) ([]DeviceInstalledApplicationReadRepresentationV1, error) {
	prefix := c.tenantPrefix(deviceNamespace, "v1")
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]DeviceInstalledApplicationReadRepresentationV1, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))
		if len(sort) > 0 {
			params.Set("sort", strings.Join(sort, ","))
		}
		if filter != "" {
			params.Set("filter", filter)
		}

		endpoint := fmt.Sprintf("%s/devices/%s/applications", prefix, url.PathEscape(deviceID))
		if encoded := params.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}

		var result struct {
			client.PaginatedResponseRepresentation
			Results []DeviceInstalledApplicationReadRepresentationV1 `json:"results"`
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
	})
}

// ListDevicesForUser returns all devices assigned to the specified user, handling pagination internally.
func (c *Client) ListDevicesForUser(ctx context.Context, userID string, sort []string, filter string) ([]DeviceListReadRepresentationV1, error) {
	prefix := c.tenantPrefix(deviceNamespace, "v1")
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]DeviceListReadRepresentationV1, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))
		if len(sort) > 0 {
			params.Set("sort", strings.Join(sort, ","))
		}
		if filter != "" {
			params.Set("filter", filter)
		}

		endpoint := fmt.Sprintf("%s/users/%s/devices", prefix, url.PathEscape(userID))
		if encoded := params.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}

		var result struct {
			client.PaginatedResponseRepresentation
			Results []DeviceListReadRepresentationV1 `json:"results"`
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
	})
}

// GetDeviceBySerialNumber looks up a single device by its serial number.
// It returns an error if no device is found or if multiple devices match.
func (c *Client) GetDeviceBySerialNumber(ctx context.Context, serialNumber string) (*DeviceReadRepresentationV1, error) {
	if serialNumber == "" {
		return nil, fmt.Errorf("serial number cannot be empty")
	}

	filter := BuildRSQLExpression([]RSQLClause{
		{Selector: "serialNumber", Operator: "==", Argument: serialNumber},
	})
	devices, err := c.ListDevices(ctx, nil, filter)
	if err != nil {
		return nil, fmt.Errorf("GetDeviceBySerialNumber(%s): %w", serialNumber, err)
	}

	switch len(devices) {
	case 0:
		return nil, fmt.Errorf("GetDeviceBySerialNumber(%s): no device found", serialNumber)
	case 1:
		return c.GetDevice(ctx, devices[0].ID)
	default:
		return nil, fmt.Errorf("GetDeviceBySerialNumber(%s): multiple devices (%d) found", serialNumber, len(devices))
	}
}
