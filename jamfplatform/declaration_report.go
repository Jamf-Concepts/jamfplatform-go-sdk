// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// DDM Declaration Reporting API client

package jamfplatform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/internal/client"
)

// DDM Declaration Reporting API path constants.
const ddmReportNamespace = "ddm/report"

// StatusReportDeclarationReasonDetailV1 represents a detail entry within a declaration reason.
type StatusReportDeclarationReasonDetailV1 struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

// StatusReportDeclarationReasonV1 represents a reason associated with a declaration status.
type StatusReportDeclarationReasonV1 struct {
	Code        string                                  `json:"code"`
	Description string                                  `json:"description"`
	Details     []StatusReportDeclarationReasonDetailV1 `json:"details,omitempty"`
}

// StatusReportDeclarationV1 represents a declaration within a device report channel.
type StatusReportDeclarationV1 struct {
	DeclarationIdentifier string                            `json:"declarationIdentifier"`
	Type                  string                            `json:"type"`
	Status                string                            `json:"status"`
	Active                bool                              `json:"active"`
	ValidityState         string                            `json:"validityState"`
	ServerToken           string                            `json:"serverToken"`
	DateUpdated           string                            `json:"dateUpdated,omitempty"`
	Reasons               []StatusReportDeclarationReasonV1 `json:"reasons,omitempty"`
}

// DeviceReportChannelV1 represents a channel within a device report.
type DeviceReportChannelV1 struct {
	Channel        string                      `json:"channel"`
	Declarations   []StatusReportDeclarationV1 `json:"declarations,omitempty"`
	LastReportTime string                      `json:"lastReportTime"`
}

// DeviceReportV1 represents the device declaration report response.
type DeviceReportV1 struct {
	Channels []DeviceReportChannelV1 `json:"channels,omitempty"`
}

// DeclarationReportClientV1 represents a device/client entry within a declaration report.
type DeclarationReportClientV1 struct {
	DeviceID      string                            `json:"deviceId"`
	Channel       string                            `json:"channel"`
	Active        bool                              `json:"active"`
	ValidityState string                            `json:"validityState"`
	ServerToken   string                            `json:"serverToken"`
	DateUpdated   string                            `json:"dateUpdated,omitempty"`
	Reasons       []StatusReportDeclarationReasonV1 `json:"reasons,omitempty"`
}

// GetDeviceDeclarationReport retrieves the declaration report for a device.
func (c *Client) GetDeviceDeclarationReport(ctx context.Context, deviceID string) (*DeviceReportV1, error) {
	prefix := c.tenantPrefix(ddmReportNamespace, "v1")
	endpoint := fmt.Sprintf("%s/devices/%s", prefix, url.PathEscape(deviceID))
	var result DeviceReportV1
	if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("GetDeviceDeclarationReport(%s): %w", deviceID, err)
	}
	return &result, nil
}

// ListDeclarationReportClients returns all clients reporting a declaration, handling pagination.
func (c *Client) ListDeclarationReportClients(ctx context.Context, declarationIdentifier string, sort []string) ([]DeclarationReportClientV1, error) {
	prefix := c.tenantPrefix(ddmReportNamespace, "v1")
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]DeclarationReportClientV1, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("size", strconv.Itoa(pageSize))
		if len(sort) > 0 {
			params.Set("sort", strings.Join(sort, ","))
		}

		endpoint := fmt.Sprintf("%s/declarations/%s", prefix, url.PathEscape(declarationIdentifier))
		if encoded := params.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}

		var result struct {
			DeclarationIdentifier string                      `json:"declarationIdentifier"`
			TotalCount            int                         `json:"totalCount"`
			Results               []DeclarationReportClientV1 `json:"results"`
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		hasNext := (page+1)*pageSize < result.TotalCount
		return result.Results, hasNext, nil
	})
}
