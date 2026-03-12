// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// CBEngine (Compliance Benchmarks) API client
// https://developer.jamf.com/platform-api/reference/benchmarks
// https://developer.jamf.com/platform-api/reference/rules
// https://developer.jamf.com/platform-api/reference/baselines

package jamfplatform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// CBEngine API path constants.
const (
	cbEngineV1Prefix = "/api/compliance-benchmarks/preview/v1"
	cbEngineV2Prefix = "/api/compliance-benchmarks/preview/v2"
)

// CBEngine Baseline Types

// CBEngineBaselinesResponseV1 represents the response for baselines listing.
type CBEngineBaselinesResponseV1 struct {
	Baselines []CBEngineBaselineInfoV1 `json:"baselines,omitempty"`
}

// CBEngineBaselineInfoV1 represents information about a baseline.
type CBEngineBaselineInfoV1 struct {
	ID          string `json:"id,omitempty"`
	BaselineID  string `json:"baselineId,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Title       string `json:"title,omitempty"`
	RuleCount   int64  `json:"ruleCount,omitempty"`
}

// CBEngineSourceV1 represents source information.
type CBEngineSourceV1 struct {
	Branch   string `json:"branch"`
	Revision string `json:"revision"`
}

// CBEngineTargetV2 represents the target configuration.
type CBEngineTargetV2 struct {
	DeviceGroups []string `json:"deviceGroups"`
}

// CBEngine Benchmark Types

// CBEngineBenchmarkRequestV2 represents the request body for creating/updating benchmarks.
type CBEngineBenchmarkRequestV2 struct {
	Title            string                  `json:"title"`
	Description      string                  `json:"description,omitempty"`
	SourceBaselineID string                  `json:"sourceBaselineId"`
	Sources          []CBEngineSourceV1      `json:"sources"`
	Rules            []CBEngineRuleRequestV2 `json:"rules"`
	Target           CBEngineTargetV2        `json:"target"`
	EnforcementMode  string                  `json:"enforcementMode"`
}

// CBEngineBenchmarkResponseV2 represents the response for benchmark operations.
type CBEngineBenchmarkResponseV2 struct {
	BenchmarkID     string               `json:"benchmarkId"`
	TenantID        string               `json:"tenantId"`
	Title           string               `json:"title"`
	Description     string               `json:"description,omitempty"`
	Sources         []CBEngineSourceV1   `json:"sources"`
	Rules           []CBEngineRuleInfoV1 `json:"rules"`
	Target          CBEngineTargetV2     `json:"target"`
	EnforcementMode string               `json:"enforcementMode"`
	Deleted         bool                 `json:"deleted"`
	UpdateAvailable bool                 `json:"updateAvailable"`
	LastUpdatedAt   time.Time            `json:"lastUpdatedAt"`
}

// CBEngineBenchmarksResponseV2 represents the response for listing benchmarks.
type CBEngineBenchmarksResponseV2 struct {
	Benchmarks []CBEngineBenchmarkV2 `json:"benchmarks"`
}

// CBEngineBenchmarkV2 represents a benchmark in the list response.
type CBEngineBenchmarkV2 struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Description     string           `json:"description,omitempty"`
	UpdateAvailable bool             `json:"updateAvailable"`
	Target          CBEngineTargetV2 `json:"target"`
	SyncState       string           `json:"syncState"`
}

// CBEngine Rule Types

// CBEngineRuleRequestV2 represents a rule in the request.
type CBEngineRuleRequestV2 struct {
	ID      string                `json:"id"`
	Enabled bool                  `json:"enabled"`
	ODV     *CBEngineODVRequestV2 `json:"odv,omitempty"`
}

// CBEngineODVRequestV2 represents an organization-defined value in requests.
type CBEngineODVRequestV2 struct {
	Value string `json:"value"`
}

// CBEngineRuleInfoV1 represents detailed rule information in responses.
type CBEngineRuleInfoV1 struct {
	ID                 string                                  `json:"id"`
	SectionName        string                                  `json:"sectionName"`
	Enabled            bool                                    `json:"enabled"`
	Title              string                                  `json:"title"`
	References         []string                                `json:"references,omitempty"`
	Description        string                                  `json:"description"`
	ODV                *CBEngineOrganizationDefinedValueV1     `json:"odv,omitempty"`
	SupportedOS        []CBEngineOSInfoV1                      `json:"supportedOs"`
	OSSpecificDefaults map[string]CBEngineOSSpecificRuleInfoV1 `json:"osSpecificDefaults"`
	RuleRelation       *CBEngineRuleRelationV1                 `json:"ruleRelation,omitempty"`
}

// CBEngineOrganizationDefinedValueV1 represents ODV with full details.
type CBEngineOrganizationDefinedValueV1 struct {
	Value       string                           `json:"value"`
	Hint        string                           `json:"hint,omitempty"`
	Placeholder string                           `json:"placeholder,omitempty"`
	Type        string                           `json:"type,omitempty"`
	Validation  *CBEngineValidationConstraintsV1 `json:"validation,omitempty"`
}

// CBEngineValidationConstraintsV1 represents validation rules for ODV.
type CBEngineValidationConstraintsV1 struct {
	Min        *int     `json:"min,omitempty"`
	Max        *int     `json:"max,omitempty"`
	EnumValues []string `json:"enumValues,omitempty"`
	Regex      string   `json:"regex,omitempty"`
}

// CBEngineOSInfoV1 represents operating system information.
type CBEngineOSInfoV1 struct {
	OSType         string `json:"osType"`
	OSVersion      int    `json:"osVersion"`
	ManagementType string `json:"managementType"`
}

// CBEngineOSSpecificRuleInfoV1 represents OS-specific rule details.
type CBEngineOSSpecificRuleInfoV1 struct {
	Title       string                       `json:"title"`
	Description string                       `json:"description"`
	ODV         *CBEngineODVRecommendationV1 `json:"odv,omitempty"`
}

// CBEngineODVRecommendationV1 represents ODV recommendation.
type CBEngineODVRecommendationV1 struct {
	Value string `json:"value,omitempty"`
	Hint  string `json:"hint,omitempty"`
}

// CBEngineRuleRelationV1 represents rule dependencies.
type CBEngineRuleRelationV1 struct {
	DependsOn []string `json:"dependsOn,omitempty"`
}

// CBEngineSourcedRulesV1 represents rules with their sources.
type CBEngineSourcedRulesV1 struct {
	Sources []CBEngineSourceV1   `json:"sources"`
	Rules   []CBEngineRuleInfoV1 `json:"rules"`
}

// CBEngine Baseline operations

// ListBaselines returns list of available mSCP baselines.
func (c *Client) ListBaselines(ctx context.Context) (*CBEngineBaselinesResponseV1, error) {
	var result CBEngineBaselinesResponseV1
	if err := c.transport.Do(ctx, http.MethodGet, cbEngineV1Prefix+"/baselines", nil, &result); err != nil {
		return nil, fmt.Errorf("ListBaselines: %w", err)
	}
	return &result, nil
}

// CBEngine Benchmark operations

// CreateBenchmark creates a new benchmark.
func (c *Client) CreateBenchmark(ctx context.Context, request *CBEngineBenchmarkRequestV2) (*CBEngineBenchmarkResponseV2, error) {
	var result CBEngineBenchmarkResponseV2
	if err := c.transport.DoExpect(ctx, http.MethodPost, cbEngineV2Prefix+"/benchmarks", request, http.StatusAccepted, &result); err != nil {
		return nil, fmt.Errorf("CreateBenchmark: %w", err)
	}
	return &result, nil
}

// ListBenchmarks retrieves all benchmarks for the tenant.
func (c *Client) ListBenchmarks(ctx context.Context) (*CBEngineBenchmarksResponseV2, error) {
	var result CBEngineBenchmarksResponseV2
	if err := c.transport.Do(ctx, http.MethodGet, cbEngineV2Prefix+"/benchmarks", nil, &result); err != nil {
		return nil, fmt.Errorf("ListBenchmarks: %w", err)
	}
	return &result, nil
}

// GetBenchmark retrieves a specific benchmark by ID.
func (c *Client) GetBenchmark(ctx context.Context, id string) (*CBEngineBenchmarkResponseV2, error) {
	var result CBEngineBenchmarkResponseV2
	endpoint := fmt.Sprintf("%s/benchmarks/%s", cbEngineV2Prefix, url.PathEscape(id))
	if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("GetBenchmark(%s): %w", id, err)
	}
	return &result, nil
}

// DeleteBenchmark removes a benchmark by ID.
func (c *Client) DeleteBenchmark(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/benchmarks/%s", cbEngineV1Prefix, url.PathEscape(id))
	if err := c.transport.DoExpect(ctx, http.MethodDelete, endpoint, nil, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("DeleteBenchmark(%s): %w", id, err)
	}
	return nil
}

// GetBenchmarkByTitle retrieves a specific benchmark by title.
func (c *Client) GetBenchmarkByTitle(ctx context.Context, title string) (*CBEngineBenchmarkResponseV2, error) {
	benchmarks, err := c.ListBenchmarks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get benchmarks list: %w", err)
	}

	for _, benchmark := range benchmarks.Benchmarks {
		if benchmark.Title == title {
			return c.GetBenchmark(ctx, benchmark.ID)
		}
	}

	return nil, fmt.Errorf("benchmark with title '%s' not found", title)
}

// CBEngine Rule operations

// GetBaselineRules returns list of rules for given baseline.
func (c *Client) GetBaselineRules(ctx context.Context, baselineID string) (*CBEngineSourcedRulesV1, error) {
	var result CBEngineSourcedRulesV1
	endpoint := fmt.Sprintf("%s/rules?baselineId=%s", cbEngineV1Prefix, url.QueryEscape(baselineID))
	if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("GetBaselineRules(%s): %w", baselineID, err)
	}
	return &result, nil
}
