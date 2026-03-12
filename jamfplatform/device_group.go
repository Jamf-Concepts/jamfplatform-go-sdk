// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Device Groups API client
// https://developer.jamf.com/platform-api/reference/device-groups

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

// Device Groups API path constants.
const (
	deviceGroupsV1Prefix = "/management/device-groups/v1"
)

// DeviceGroupListReadRepresentationV1 represents a device group in a list response.
type DeviceGroupListReadRepresentationV1 struct {
	ID          string                                `json:"id"`
	Name        string                                `json:"name"`
	Description string                                `json:"description,omitempty"`
	DeviceType  string                                `json:"deviceType"`
	GroupType   string                                `json:"groupType"`
	MemberCount int                                   `json:"memberCount"`
	Criteria    []DeviceGroupCriteriaRepresentationV1 `json:"criteria,omitempty"`
}

// DeviceGroupCriteriaRepresentationV1 represents a criterion for device groups.
type DeviceGroupCriteriaRepresentationV1 struct {
	Order                 int    `json:"order"`
	AttributeName         string `json:"attributeName"`
	Operator              string `json:"operator"`
	AttributeValue        string `json:"attributeValue"`
	JoinType              string `json:"joinType"`
	HasOpeningParenthesis bool   `json:"hasOpeningParenthesis,omitempty"`
	HasClosingParenthesis bool   `json:"hasClosingParenthesis,omitempty"`
}

// DeviceGroupReadRepresentationV1 represents a device group in a single-read response.
type DeviceGroupReadRepresentationV1 struct {
	ID          string                                `json:"id"`
	Name        string                                `json:"name"`
	Description string                                `json:"description,omitempty"`
	DeviceType  string                                `json:"deviceType"`
	GroupType   string                                `json:"groupType"`
	MemberCount int                                   `json:"memberCount"`
	Criteria    []DeviceGroupCriteriaRepresentationV1 `json:"criteria,omitempty"`
}

// DeviceGroupCreateRepresentationV1 represents the payload to create a device group.
type DeviceGroupCreateRepresentationV1 struct {
	Name        string                                `json:"name"`
	Description *string                               `json:"description,omitempty"`
	DeviceType  string                                `json:"deviceType"`
	GroupType   string                                `json:"groupType"`
	Criteria    []DeviceGroupCriteriaRepresentationV1 `json:"criteria,omitempty"`
	Members     []string                              `json:"members,omitempty"`
}

// DeviceGroupCreateResponseV1 represents the response after creating a device group.
type DeviceGroupCreateResponseV1 struct {
	ID   string `json:"id"`
	Href string `json:"href"`
}

// DeviceGroupUpdateRepresentationV1 represents the payload to update a device group.
type DeviceGroupUpdateRepresentationV1 struct {
	Name        string                                `json:"name"`
	Description *string                               `json:"description,omitempty"`
	Criteria    []DeviceGroupCriteriaRepresentationV1 `json:"criteria,omitempty"`
	DeviceIds   []string                              `json:"deviceIds,omitempty"`
}

// DeviceGroupMemberPatchRepresentationV1 represents the payload to patch device group members.
type DeviceGroupMemberPatchRepresentationV1 struct {
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

// DeviceGroupMemberV1 represents a device group member.
type DeviceGroupMemberV1 struct {
	DeviceID string `json:"deviceId"`
}

// DeviceGroupMemberOfRepresentationV1 represents a device group that a device belongs to.
type DeviceGroupMemberOfRepresentationV1 struct {
	GroupID   string `json:"groupId"`
	GroupName string `json:"groupName"`
}

// ListDeviceGroups returns all device groups, automatically handling pagination.
func (c *Client) ListDeviceGroups(ctx context.Context, sort []string, filter string) ([]DeviceGroupListReadRepresentationV1, error) {
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]DeviceGroupListReadRepresentationV1, bool, error) {
		params := url.Values{}
		if len(sort) > 0 {
			params.Set("sort", strings.Join(sort, ","))
		}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))
		if filter != "" {
			params.Set("filter", filter)
		}

		endpoint := deviceGroupsV1Prefix + "/device-groups"
		if len(params) > 0 {
			endpoint += "?" + params.Encode()
		}

		var result struct {
			client.PaginatedResponseRepresentation
			Results []DeviceGroupListReadRepresentationV1 `json:"results"`
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
	})
}

// GetDeviceGroup retrieves a device group by ID.
func (c *Client) GetDeviceGroup(ctx context.Context, id string) (*DeviceGroupReadRepresentationV1, error) {
	var result DeviceGroupReadRepresentationV1
	endpoint := fmt.Sprintf("%s/device-groups/%s", deviceGroupsV1Prefix, url.PathEscape(id))
	if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("GetDeviceGroup(%s): %w", id, err)
	}
	return &result, nil
}

// CreateDeviceGroup creates a new device group.
func (c *Client) CreateDeviceGroup(ctx context.Context, request *DeviceGroupCreateRepresentationV1) (*DeviceGroupCreateResponseV1, error) {
	var result DeviceGroupCreateResponseV1
	endpoint := deviceGroupsV1Prefix + "/device-groups"
	if err := c.transport.DoExpect(ctx, http.MethodPost, endpoint, request, http.StatusCreated, &result); err != nil {
		return nil, fmt.Errorf("CreateDeviceGroup: %w", err)
	}
	return &result, nil
}

// UpdateDeviceGroup updates a device group.
func (c *Client) UpdateDeviceGroup(ctx context.Context, id string, request *DeviceGroupUpdateRepresentationV1) error {
	endpoint := fmt.Sprintf("%s/device-groups/%s", deviceGroupsV1Prefix, url.PathEscape(id))
	if err := c.transport.DoWithContentType(ctx, http.MethodPatch, endpoint, request, "application/json", http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("UpdateDeviceGroup(%s): %w", id, err)
	}
	return nil
}

// DeleteDeviceGroup deletes a device group by ID.
func (c *Client) DeleteDeviceGroup(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/device-groups/%s", deviceGroupsV1Prefix, url.PathEscape(id))
	if err := c.transport.DoExpect(ctx, http.MethodDelete, endpoint, nil, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("DeleteDeviceGroup(%s): %w", id, err)
	}
	return nil
}

// ListDeviceGroupMembers returns all member IDs for a device group, handling pagination internally.
func (c *Client) ListDeviceGroupMembers(ctx context.Context, id string) ([]string, error) {
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]string, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))

		endpoint := fmt.Sprintf("%s/device-groups/%s/members?%s", deviceGroupsV1Prefix, url.PathEscape(id), params.Encode())

		var result struct {
			Results     []string `json:"results"`
			TotalCount  int      `json:"totalCount"`
			Page        int      `json:"page"`
			PageSize    int      `json:"pageSize"`
			TotalPages  int      `json:"totalPages"`
			HasNext     bool     `json:"hasNext"`
			HasPrevious bool     `json:"hasPrevious"`
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
	})
}

// UpdateDeviceGroupMembers patches the members of a static device group.
func (c *Client) UpdateDeviceGroupMembers(ctx context.Context, id string, patch *DeviceGroupMemberPatchRepresentationV1) error {
	endpoint := fmt.Sprintf("%s/device-groups/%s/members", deviceGroupsV1Prefix, url.PathEscape(id))
	if err := c.transport.DoWithContentType(ctx, http.MethodPatch, endpoint, patch, "application/json", http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("UpdateDeviceGroupMembers(%s): %w", id, err)
	}
	return nil
}

// ListDeviceGroupsForDevice returns all device groups a device belongs to, handling pagination internally.
func (c *Client) ListDeviceGroupsForDevice(ctx context.Context, deviceID string) ([]DeviceGroupMemberOfRepresentationV1, error) {
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]DeviceGroupMemberOfRepresentationV1, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))

		endpoint := fmt.Sprintf("%s/devices/%s/device-groups?%s", deviceGroupsV1Prefix, url.PathEscape(deviceID), params.Encode())

		var result struct {
			Results     []DeviceGroupMemberOfRepresentationV1 `json:"results"`
			TotalCount  int                                   `json:"totalCount"`
			Page        int                                   `json:"page"`
			PageSize    int                                   `json:"pageSize"`
			TotalPages  int                                   `json:"totalPages"`
			HasNext     bool                                  `json:"hasNext"`
			HasPrevious bool                                  `json:"hasPrevious"`
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
	})
}
