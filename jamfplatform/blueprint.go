// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Blueprint API client
// https://developer.jamf.com/platform-api/reference/blueprints-1

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

// Blueprint API path constants
const blueprintV1Prefix = "/api/blueprints/v1"

// Blueprint API Types

// BlueprintComponentDescriptionV1 describes a component within a blueprint.
type BlueprintComponentDescriptionV1 struct {
	Identifier  string          `json:"identifier"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Meta        BlueprintMetaV1 `json:"meta"`
}

// BlueprintMetaV1 describes metadata about a blueprint.
type BlueprintMetaV1 struct {
	SupportedOs map[string][]BlueprintSupportedOsV1 `json:"supportedOs"`
}

// BlueprintSupportedOsV1 describes a supported operating system for a blueprint.
type BlueprintSupportedOsV1 struct {
	Version string `json:"version"`
}

// BlueprintComponentV1 describes a component within a blueprint.
type BlueprintComponentV1 struct {
	Identifier    string          `json:"identifier"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

// BlueprintStepV1 describes a step within a blueprint.
type BlueprintStepV1 struct {
	Name       string                 `json:"name"`
	Components []BlueprintComponentV1 `json:"components,omitempty"`
}

// BlueprintCreateScopeV1 defines the scope for creating a blueprint.
type BlueprintCreateScopeV1 struct {
	DeviceGroups []string `json:"deviceGroups"`
}

// BlueprintCreateRequestV1 represents a request to create a blueprint.
type BlueprintCreateRequestV1 struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Scope       BlueprintCreateScopeV1 `json:"scope"`
	Steps       []BlueprintStepV1      `json:"steps"`
}

// BlueprintUpdateRequestV1 represents a request to update an existing blueprint.
type BlueprintUpdateRequestV1 struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Scope       BlueprintUpdateScopeV1 `json:"scope"`
	Steps       []BlueprintStepV1      `json:"steps"`
}

// BlueprintDeploymentV1 describes the deployment status of a blueprint.
type BlueprintDeploymentV1 struct {
	Started string `json:"started"`
	State   string `json:"state"`
}

// BlueprintDeploymentStateV1 describes the state of a blueprint deployment.
type BlueprintDeploymentStateV1 struct {
	State          string                 `json:"state"`
	LastDeployment *BlueprintDeploymentV1 `json:"lastDeployment"`
}

// BlueprintDetailV1 describes the details of a blueprint.
type BlueprintDetailV1 struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Description     string                     `json:"description,omitempty"`
	Scope           BlueprintUpdateScopeV1     `json:"scope"`
	Created         string                     `json:"created"`
	Updated         string                     `json:"updated"`
	DeploymentState BlueprintDeploymentStateV1 `json:"deploymentState"`
	Steps           []BlueprintStepV1          `json:"steps"`
}

// BlueprintOverviewV1 describes a summary of a blueprint.
type BlueprintOverviewV1 struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Description     string                     `json:"description,omitempty"`
	Created         string                     `json:"created"`
	Updated         string                     `json:"updated"`
	DeploymentState BlueprintDeploymentStateV1 `json:"deploymentState"`
}

// BlueprintUpdateScopeV1 defines the scope for updating a blueprint.
type BlueprintUpdateScopeV1 struct {
	DeviceGroups []string `json:"deviceGroups"`
}

// BlueprintCreateResponseV1 represents the response for creating a blueprint.
type BlueprintCreateResponseV1 struct {
	ID   string `json:"id"`
	Href string `json:"href"`
}

// blueprintPath returns the endpoint path and optional tenant headers for
// a blueprint API resource. When tenantID is configured, adds X-Tenant-Id header.
func (c *Client) blueprintPath(resource string) (string, http.Header) {
	if c.tenantID != "" {
		return blueprintV1Prefix + resource, http.Header{"X-Tenant-Id": {c.tenantID}}
	}
	return blueprintV1Prefix + resource, nil
}

// ListBlueprints returns all blueprints, automatically handling pagination.
func (c *Client) ListBlueprints(ctx context.Context, sort []string, search string) ([]BlueprintOverviewV1, error) {
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]BlueprintOverviewV1, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))
		if len(sort) > 0 {
			params.Set("sort", strings.Join(sort, ","))
		}
		if search != "" {
			params.Set("search", search)
		}
		endpoint, headers := c.blueprintPath("/blueprints?" + params.Encode())

		var result struct {
			Results    []BlueprintOverviewV1 `json:"results"`
			TotalCount int64                 `json:"totalCount"`
		}
		if err := c.transport.DoWithHeaders(ctx, http.MethodGet, endpoint, nil, headers, &result); err != nil {
			return nil, false, err
		}
		return result.Results, len(result.Results) >= pageSize && len(result.Results) > 0, nil
	})
}

// GetBlueprint retrieves a blueprint by ID.
func (c *Client) GetBlueprint(ctx context.Context, id string) (*BlueprintDetailV1, error) {
	endpoint, headers := c.blueprintPath(fmt.Sprintf("/blueprints/%s", url.PathEscape(id)))
	var result BlueprintDetailV1
	if err := c.transport.DoWithHeaders(ctx, http.MethodGet, endpoint, nil, headers, &result); err != nil {
		return nil, fmt.Errorf("GetBlueprint(%s): %w", id, err)
	}
	return &result, nil
}

// GetBlueprintByName finds a blueprint by exact name and returns its details.
func (c *Client) GetBlueprintByName(ctx context.Context, name string) (*BlueprintDetailV1, error) {
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}
	blueprints, err := c.ListBlueprints(ctx, nil, name)
	if err != nil {
		return nil, fmt.Errorf("error searching for blueprint by name: %w", err)
	}
	for _, bp := range blueprints {
		if bp.Name == name {
			return c.GetBlueprint(ctx, bp.ID)
		}
	}
	return nil, fmt.Errorf("blueprint with name '%s' not found", name)
}

// CreateBlueprint creates a new blueprint.
func (c *Client) CreateBlueprint(ctx context.Context, request *BlueprintCreateRequestV1) (*BlueprintCreateResponseV1, error) {
	endpoint, headers := c.blueprintPath("/blueprints")
	var result BlueprintCreateResponseV1
	if err := c.transport.DoExpectWithHeaders(ctx, http.MethodPost, endpoint, request, headers, http.StatusCreated, &result); err != nil {
		return nil, fmt.Errorf("CreateBlueprint: %w", err)
	}
	return &result, nil
}

// UpdateBlueprint updates a blueprint configuration.
func (c *Client) UpdateBlueprint(ctx context.Context, id string, request *BlueprintUpdateRequestV1) error {
	endpoint, headers := c.blueprintPath(fmt.Sprintf("/blueprints/%s", url.PathEscape(id)))
	if err := c.transport.DoExpectWithHeaders(ctx, http.MethodPatch, endpoint, request, headers, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("UpdateBlueprint(%s): %w", id, err)
	}
	return nil
}

// DeleteBlueprint deletes a blueprint by ID.
func (c *Client) DeleteBlueprint(ctx context.Context, id string) error {
	endpoint, headers := c.blueprintPath(fmt.Sprintf("/blueprints/%s", url.PathEscape(id)))
	if err := c.transport.DoExpectWithHeaders(ctx, http.MethodDelete, endpoint, nil, headers, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("DeleteBlueprint(%s): %w", id, err)
	}
	return nil
}

// DeployBlueprint starts deployment of a blueprint.
func (c *Client) DeployBlueprint(ctx context.Context, id string) error {
	endpoint, headers := c.blueprintPath(fmt.Sprintf("/blueprints/%s/deploy", url.PathEscape(id)))
	if err := c.transport.DoExpectWithHeaders(ctx, http.MethodPost, endpoint, nil, headers, http.StatusAccepted, nil); err != nil {
		return fmt.Errorf("DeployBlueprint(%s): %w", id, err)
	}
	return nil
}

// UndeployBlueprint starts undeployment of a blueprint.
func (c *Client) UndeployBlueprint(ctx context.Context, id string) error {
	endpoint, headers := c.blueprintPath(fmt.Sprintf("/blueprints/%s/undeploy", url.PathEscape(id)))
	if err := c.transport.DoExpectWithHeaders(ctx, http.MethodPost, endpoint, nil, headers, http.StatusAccepted, nil); err != nil {
		return fmt.Errorf("UndeployBlueprint(%s): %w", id, err)
	}
	return nil
}

// ListBlueprintComponents returns all blueprint components, automatically handling pagination.
func (c *Client) ListBlueprintComponents(ctx context.Context) ([]BlueprintComponentDescriptionV1, error) {
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]BlueprintComponentDescriptionV1, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page-size", strconv.Itoa(pageSize))
		endpoint, headers := c.blueprintPath("/blueprint-components?" + params.Encode())

		var result struct {
			Results    []BlueprintComponentDescriptionV1 `json:"results"`
			TotalCount int64                             `json:"totalCount"`
		}
		if err := c.transport.DoWithHeaders(ctx, http.MethodGet, endpoint, nil, headers, &result); err != nil {
			return nil, false, err
		}
		return result.Results, len(result.Results) >= pageSize && len(result.Results) > 0, nil
	})
}

// GetBlueprintComponent gets a blueprint component by identifier.
func (c *Client) GetBlueprintComponent(ctx context.Context, id string) (*BlueprintComponentDescriptionV1, error) {
	endpoint, headers := c.blueprintPath(fmt.Sprintf("/blueprint-components/%s", url.PathEscape(id)))
	var result BlueprintComponentDescriptionV1
	if err := c.transport.DoWithHeaders(ctx, http.MethodGet, endpoint, nil, headers, &result); err != nil {
		return nil, fmt.Errorf("GetBlueprintComponent(%s): %w", id, err)
	}
	return &result, nil
}
