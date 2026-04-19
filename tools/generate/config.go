// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Configuration types (loaded from config.json)
// ---------------------------------------------------------------------------

type Config struct {
	Package string    `json:"package"`
	Module  string    `json:"module"`
	SpecDir string    `json:"specDir"`
	Specs   []SpecDef `json:"specs"`
}

type SpecDef struct {
	File           string            `json:"file"`
	Namespace      string            `json:"namespace"`
	SpecFile       string            `json:"specFile,omitempty"`       // override published spec filename
	Package        string            `json:"package,omitempty"`        // target Go sub-package under jamfplatform/; empty emits to root (legacy)
	SplitByTag     bool              `json:"splitByTag,omitempty"`     // emit one methods file per OpenAPI tag instead of one per spec
	Format         string            `json:"format,omitempty"`         // "json" (default) or "xml" — drives struct tag style and transport codec
	RawBody        bool              `json:"rawBody,omitempty"`        // generate methods that take/return []byte instead of typed structs; consumer owns marshaling (used for Classic where spec has no useful types)
	Operations     []OperationDef    `json:"operations"`
	ExcludePaths   []string          `json:"excludePaths,omitempty"`   // "METHOD /path" entries the generator must refuse to include
	SkipDeprecated bool              `json:"skipDeprecated,omitempty"` // omit operations marked deprecated in the spec
	FieldTypeOverrides map[string]string            `json:"fieldTypeOverrides,omitempty"` // "schema_name.property_name" -> Go type, used to correct spec bugs (e.g. `integer` fields where the server actually returns a non-int64 string). Applied per-spec so upstream spec updates don't get silently overwritten.
	SchemaAdditions    map[string]map[string]string `json:"schemaAdditions,omitempty"`    // "schema_name" -> { "property_name": "openapi_type" }, inject missing properties into a spec schema. Used when the spec omits a field the server accepts but we need to send (e.g. Classic's account schema has no `password` property). openapi_type is one of "string", "integer", "boolean", "string:password" (writeOnly string).
}

// baseName derives a Go file base name from the spec file path.
// "testing/device-inventory.yaml" → "device_inventory"
// "testing/benchmarks-report.yaml" → "benchmarks_report"
func (s SpecDef) baseName() string {
	name := filepath.Base(s.File)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	return strings.ReplaceAll(name, "-", "_")
}

func (s SpecDef) outputFile() string     { return "jamfplatform/" + s.baseName() + ".go" }
func (s SpecDef) testOutputFile() string { return "jamfplatform/" + s.baseName() + "_test.go" }

type OperationDef struct {
	Op            string            `json:"op"`                   // "GET /v1/devices/{id}"
	Name          string            `json:"name"`                 // Go method name
	ContentType   string            `json:"contentType,omitempty"`
	Pagination    string            `json:"pagination,omitempty"` // hasNext, sizeCheck, totalCount, rawArray
	PageSizeParam string            `json:"pageSizeParam,omitempty"`
	Version       string            `json:"version,omitempty"`    // override version for tenantPrefix
	PathNames     map[string]string `json:"pathNames,omitempty"`  // spec param -> Go param name
	Params        []string          `json:"params,omitempty"`     // "name", "name:type", "spec:type:goName"
	UnwrapResults string            `json:"unwrapResults,omitempty"`
	RequestType   string            `json:"requestType,omitempty"`  // explicit request schema name (used when spec body is untyped, e.g. Classic)
	ResponseType  string            `json:"responseType,omitempty"` // explicit response schema name (same)
	ExpectedStatus int              `json:"expectedStatus,omitempty"` // explicit success status code (default 200)
}

// parseOp splits "GET /v1/devices/{id}" into method and path.
func (o OperationDef) parseOp() (method, path string) {
	parts := strings.SplitN(o.Op, " ", 2)
	return strings.ToUpper(parts[0]), parts[1]
}

// parseParams expands compact param notation into ExtraParam structs.
//
//	"sort"                → {Spec:"sort", Go:"sort", Type:"string"}
//	"sort:[]string"       → {Spec:"sort", Go:"sort", Type:"[]string"}
//	"rule-id:string:ruleID" → {Spec:"rule-id", Go:"ruleID", Type:"string"}
func (o OperationDef) parseParams() []ExtraParam {
	params := make([]ExtraParam, 0, len(o.Params))
	for _, p := range o.Params {
		parts := strings.Split(p, ":")
		ep := ExtraParam{Spec: parts[0], Go: toLowerCamelCase(parts[0]), Type: "string"}
		if len(parts) >= 2 {
			ep.Type = parts[1]
		}
		if len(parts) >= 3 {
			ep.Go = parts[2]
		}
		params = append(params, ep)
	}
	return params
}

type ExtraParam struct {
	Spec string
	Go   string
	Type string
}

// validateConfig rejects misconfigured specs before generation runs.
// Currently enforces that no operation in Operations appears in ExcludePaths —
// the deny list is meant to catch accidental re-adds, so a conflict means
// either the entry should be removed from one side or the other.
func validateConfig(cfg Config) error {
	for _, spec := range cfg.Specs {
		excluded := make(map[string]bool, len(spec.ExcludePaths))
		for _, p := range spec.ExcludePaths {
			norm := normalizeOpKey(p)
			if excluded[norm] {
				return fmt.Errorf("spec %q: duplicate entry in excludePaths: %q", spec.File, p)
			}
			excluded[norm] = true
		}
		for _, op := range spec.Operations {
			if excluded[normalizeOpKey(op.Op)] {
				return fmt.Errorf("spec %q: operation %q is listed in both operations and excludePaths", spec.File, op.Op)
			}
		}
	}
	return nil
}

// normalizeOpKey canonicalises "METHOD /path" for comparison — uppercase
// method, single space, trimmed.
func normalizeOpKey(s string) string {
	parts := strings.SplitN(strings.TrimSpace(s), " ", 2)
	if len(parts) != 2 {
		return strings.ToUpper(strings.TrimSpace(s))
	}
	return strings.ToUpper(parts[0]) + " " + strings.TrimSpace(parts[1])
}
