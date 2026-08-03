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
	// kebab-case → lowerCamel: "panel-id" → "panelId", "some-thing" → "someThing".
	// Required because Go identifiers cannot contain hyphens; Jamf Pro spec
	// uses kebab-case for a handful of path-param names.
	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		var out strings.Builder
		out.WriteString(parts[0])
		for _, p := range parts[1:] {
			if p == "" {
				continue
			}
			out.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
		s = out.String()
	}
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

// specHTMLTagRe matches the inline HTML fragments Jamf embeds in spec prose
// (<br/>, </br>, <p>, <li>, …). Godoc has no HTML, so the tags are dropped.
// The alternation is an explicit tag allowlist rather than a generic <...>
// pattern because some descriptions use angle brackets as placeholder syntax
// ("sort in the format <field_name>:asc") and those must survive.
var specHTMLTagRe = regexp.MustCompile(`(?i)</?(br|p|div|span|ul|ol|li|b|i|em|strong|code|pre|a|table|tr|td|th)\b[^>]*>`)

// specHTMLEntities decodes the handful of entities Jamf's spec prose uses.
var specHTMLEntities = strings.NewReplacer(
	"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&nbsp;", " ",
)

// stripSpecHTML renders spec prose as plain text for godoc: known tags become
// a space (so "a</br>b" doesn't fuse into "ab") and entities are decoded.
// Whitespace normalisation is left to cleanComment.
func stripSpecHTML(s string) string {
	return specHTMLEntities.Replace(specHTMLTagRe.ReplaceAllString(s, " "))
}

// wrapCommentText greedily wraps s to width columns without splitting a word.
// Needed for parameter documentation: a single Jamf description can run to
// thousands of characters (the Pro list endpoints inline every sortable field
// name), and cleanComment collapses it to one unreadable line.
func wrapCommentText(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var (
		lines []string
		cur   strings.Builder
	)
	for _, w := range words {
		switch {
		case cur.Len() == 0:
			cur.WriteString(w)
		case cur.Len()+1+len(w) <= width:
			cur.WriteByte(' ')
			cur.WriteString(w)
		default:
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
		}
	}
	return append(lines, cur.String())
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

// orderedProps returns map keys in the order given by explicit, with any
// remaining keys appended alphabetically. If explicit is nil or empty it
// degenerates to sortedKeys. Prop keys that carry deprecation metadata
// (e.g. "field_name deprecated=10.48") are matched by their bare name.
func orderedProps[V any](m map[string]V, explicit []string) []string {
	if len(explicit) == 0 {
		return sortedKeys(m)
	}
	// Build a stripped-name → raw-key index so we can match explicit entries
	// against keys that may carry deprecation suffixes.
	stripped := make(map[string]string, len(m)) // stripped name → raw map key
	for k := range m {
		bare := k
		if i := strings.IndexAny(bare, " \t"); i >= 0 {
			bare = bare[:i]
		}
		stripped[bare] = k
	}
	seen := make(map[string]bool, len(m))
	result := make([]string, 0, len(m))
	for _, name := range explicit {
		raw, ok := stripped[name]
		if !ok {
			continue // name in explicit list but absent from map — skip
		}
		result = append(result, raw)
		seen[raw] = true
	}
	// Append remaining keys alphabetically.
	remaining := make([]string, 0, len(m)-len(seen))
	for k := range m {
		if !seen[k] {
			remaining = append(remaining, k)
		}
	}
	sort.Strings(remaining)
	return append(result, remaining...)
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
