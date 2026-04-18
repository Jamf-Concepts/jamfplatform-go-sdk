// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// String utilities
// ---------------------------------------------------------------------------

// Regex for acronym fixup: matches "Id", "Url" etc. only when followed by
// uppercase, end-of-string, or a non-letter — so "Identifier" is not touched.
var acronymFixups = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`Ip([AV])`), "IP$1"},
	{regexp.MustCompile(`Uuid($|[A-Z])`), "UUID$1"},
	{regexp.MustCompile(`Udid($|[A-Z])`), "UDID$1"},
	{regexp.MustCompile(`Url($|[A-Z])`), "URL$1"},
	{regexp.MustCompile(`Odv($|[A-Z])`), "ODV$1"},
	{regexp.MustCompile(`Mdm($|[A-Z])`), "MDM$1"},
	{regexp.MustCompile(`Id($|[A-Z])`), "ID$1"},
}

func exportedGoName(name string) string {
	// Exact matches for single-word properties.
	exact := map[string]string{
		"id": "ID", "ids": "IDs", "url": "URL", "urls": "URLs",
		"udid": "UDID", "ip": "IP", "os": "OS", "odv": "ODV",
		"mdm": "MDM", "uuid": "UUID", "uri": "URI", "href": "Href",
		"macAddress": "MacAddress",
	}
	if v, ok := exact[name]; ok {
		return v
	}

	// camelCase → PascalCase
	var b strings.Builder
	upper := true
	for _, r := range name {
		if r == '_' || r == '-' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	s := b.String()

	// Fix acronyms at word boundaries.
	for _, fix := range acronymFixups {
		s = fix.re.ReplaceAllString(s, fix.repl)
	}
	return s
}

func toLowerCamelCase(s string) string {
	if s == "id" {
		return "id"
	}
	if strings.HasSuffix(s, "Id") {
		return s[:len(s)-2] + "ID"
	}
	return s
}

func cleanComment(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func camelToWords(s string) string {
	var words []string
	var cur strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			if cur.Len() > 0 {
				words = append(words, strings.ToLower(cur.String()))
				cur.Reset()
			}
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		words = append(words, strings.ToLower(cur.String()))
	}
	return strings.Join(words, " ")
}

func isScalar(goType string) bool {
	switch goType {
	case "string", "int", "int32", "int64", "float32", "float64", "bool":
		return true
	}
	return false
}

// coalesce returns val if non-empty, otherwise fallback.
func coalesce(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// toSnakeCase converts titles to snake_case filenames.
// Handles spaces ("Device Inventory API" → "device_inventory_api"),
// camelCase ("DDMReport" → "ddm_report"), and mixed input.
func toSnakeCase(s string) string {
	// Insert underscore before uppercase runs: "DDMReport" → "DDM_Report"
	s = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`).ReplaceAllString(s, "${1}_${2}")
	// Insert underscore at lower→upper boundary: "deviceAction" → "device_Action"
	s = regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(s, "${1}_${2}")
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
