// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// AmbiguousMatchError indicates a name-based lookup matched more than one
// resource. Consumers inspect Matches to surface disambiguation options.
type AmbiguousMatchError struct {
	Name    string
	Matches []string // IDs of the colliding resources, in the order returned by the API
}

// Error satisfies the error interface.
func (e *AmbiguousMatchError) Error() string {
	return fmt.Sprintf("ambiguous match for name %q: %d resources (ids: %s)",
		e.Name, len(e.Matches), strings.Join(e.Matches, ", "))
}

// ResolveByNameFiltered looks up a resource by name via server-side RSQL
// equality filtering. Use when the List endpoint supports
// filter=<nameField>=="<value>". Returns the IDField value of the matched
// element and the raw JSON bytes of that element so typed wrappers can
// decode into concrete types without a second round-trip.
//
// resultsField names the envelope key holding the array of elements
// ("results" for standard paginated responses, "benchmarks" for
// compliance benchmarks, etc.). Empty defaults to "results".
//
// Not-found surfaces as *APIResponseError with StatusCode 404 — matches the
// shape Classic's native /name/{name} endpoints produce naturally, so
// consumers can check apiErr.HasStatus(404) uniformly across all three
// resolver modes. Multiple matches surface as *AmbiguousMatchError.
func (t *Transport) ResolveByNameFiltered(ctx context.Context, listPath, resultsField, nameField, matchField, idField, name string) (string, json.RawMessage, error) {
	if name == "" {
		return "", nil, fmt.Errorf("name must not be empty")
	}
	filter := nameField + "==" + FormatArgument(name)
	fullPath := joinQuery(listPath, "filter="+url.QueryEscape(filter)+"&page-size=2")
	raw, err := t.getRaw(ctx, fullPath)
	if err != nil {
		return "", nil, err
	}
	return resolveMatch(raw, resultsField, matchField, idField, name, fullPath)
}

// ResolveByNameClient looks up a resource by name via client-side exact
// matching. Use when the List endpoint supports no RSQL filter, or only a
// coarse search=<term> parameter (e.g. blueprints — search matches against
// name and description, no equality semantics). When searchParam is
// non-empty the request appends <searchParam>=<name> to narrow the
// server-side result set; matching is always re-applied client-side against
// nameField so full-text hits that are not exact equals are dropped.
//
// resultsField names the envelope key holding the array of elements;
// empty defaults to "results". See ResolveByNameFiltered for the full
// envelope-handling contract.
//
// Error semantics match ResolveByNameFiltered: not-found surfaces as
// *APIResponseError(404); ambiguity as *AmbiguousMatchError.
func (t *Transport) ResolveByNameClient(ctx context.Context, listPath, searchParam, resultsField, nameField, idField, name string) (string, json.RawMessage, error) {
	if name == "" {
		return "", nil, fmt.Errorf("name must not be empty")
	}
	fullPath := listPath
	if searchParam != "" {
		fullPath = joinQuery(listPath, url.QueryEscape(searchParam)+"="+url.QueryEscape(name))
	}
	raw, err := t.getRaw(ctx, fullPath)
	if err != nil {
		return "", nil, err
	}
	return resolveMatch(raw, resultsField, nameField, idField, name, fullPath)
}

// ResolveByNameClientPaged looks up a resource by name via client-side exact
// matching across all pages of a paginated list endpoint. Use when the List
// endpoint returns paginated results but supports no RSQL filter for
// server-side equality matching. Fetches pages sequentially until all
// results are examined, accumulating exact-match hits on nameField.
//
// Error semantics match ResolveByNameFiltered: not-found surfaces as
// *APIResponseError(404); ambiguity as *AmbiguousMatchError. Early exit
// once two or more matches are found (ambiguity is certain).
func (t *Transport) ResolveByNameClientPaged(ctx context.Context, listPath, searchParam, resultsField, nameField, idField, name string) (string, json.RawMessage, error) {
	if name == "" {
		return "", nil, fmt.Errorf("name must not be empty")
	}

	const pageSize = 100
	var allMatches []json.RawMessage

	for page := 0; ; page++ {
		fullPath := joinQuery(listPath, fmt.Sprintf("page=%d&page-size=%d", page, pageSize))
		if searchParam != "" {
			fullPath = joinQuery(fullPath, url.QueryEscape(searchParam)+"="+url.QueryEscape(name))
		}
		raw, err := t.getRaw(ctx, fullPath)
		if err != nil {
			return "", nil, err
		}

		elems, err := extractListElements(raw, resultsField)
		if err != nil {
			return "", nil, fmt.Errorf("parsing list response: %w", err)
		}

		for _, el := range elems {
			var obj map[string]any
			if err := json.Unmarshal(el, &obj); err != nil {
				continue
			}
			v, ok := extractField(obj, nameField)
			if !ok || v != name {
				continue
			}
			allMatches = append(allMatches, el)
		}

		// Two or more matches already found — check if they're truly distinct.
		if len(allMatches) > 1 {
			deduped := deduplicateByID(allMatches, idField)
			if len(deduped) > 1 {
				return "", nil, &AmbiguousMatchError{Name: name, Matches: collectIDs(deduped, idField)}
			}
		}

		// Fewer elements than page size means this was the last page.
		if len(elems) < pageSize {
			break
		}
	}

	switch len(allMatches) {
	case 0:
		return "", nil, &APIResponseError{
			StatusCode: http.StatusNotFound,
			Method:     http.MethodGet,
			URL:        listPath,
			Body:       fmt.Sprintf("no resource found with %s == %q", nameField, name),
		}
	case 1:
		var obj map[string]any
		if err := json.Unmarshal(allMatches[0], &obj); err != nil {
			return "", nil, fmt.Errorf("decoding matched element: %w", err)
		}
		id, ok := extractField(obj, idField)
		if !ok {
			return "", allMatches[0], fmt.Errorf("matched element has no %s field", idField)
		}
		return id, allMatches[0], nil
	default:
		deduped := deduplicateByID(allMatches, idField)
		if len(deduped) == 1 {
			var obj map[string]any
			if err := json.Unmarshal(deduped[0], &obj); err != nil {
				return "", nil, fmt.Errorf("decoding matched element: %w", err)
			}
			id, ok := extractField(obj, idField)
			if !ok {
				return "", deduped[0], fmt.Errorf("matched element has no %s field", idField)
			}
			return id, deduped[0], nil
		}
		return "", nil, &AmbiguousMatchError{Name: name, Matches: collectIDs(deduped, idField)}
	}
}

// getRaw fetches the response body bytes as-is by handing the transport a
// *[]byte target (see handleResponse's raw-bytes passthrough).
func (t *Transport) getRaw(ctx context.Context, path string) ([]byte, error) {
	var raw []byte
	if err := t.Do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// joinQuery appends a query-string fragment using the appropriate separator.
func joinQuery(path, frag string) string {
	if strings.Contains(path, "?") {
		return path + "&" + frag
	}
	return path + "?" + frag
}

// resolveMatch walks a list response body and picks the single element whose
// nameField equals name. Returns the idField value and the matched element's
// raw bytes, or a typed error for the not-found / ambiguous cases.
func resolveMatch(body []byte, resultsField, nameField, idField, name, requestURL string) (string, json.RawMessage, error) {
	elems, err := extractListElements(body, resultsField)
	if err != nil {
		return "", nil, fmt.Errorf("parsing list response: %w", err)
	}
	var matches []json.RawMessage
	for _, el := range elems {
		var obj map[string]any
		if err := json.Unmarshal(el, &obj); err != nil {
			continue
		}
		v, ok := extractField(obj, nameField)
		if !ok || v != name {
			continue
		}
		matches = append(matches, el)
	}
	switch len(matches) {
	case 0:
		return "", nil, &APIResponseError{
			StatusCode: http.StatusNotFound,
			Method:     http.MethodGet,
			URL:        requestURL,
			Body:       fmt.Sprintf("no resource found with %s == %q", nameField, name),
		}
	case 1:
		var obj map[string]any
		if err := json.Unmarshal(matches[0], &obj); err != nil {
			return "", nil, fmt.Errorf("decoding matched element: %w", err)
		}
		id, ok := extractField(obj, idField)
		if !ok {
			return "", matches[0], fmt.Errorf("matched element has no %s field", idField)
		}
		return id, matches[0], nil
	default:
		// Deduplicate by ID — some endpoints return the same resource
		// more than once (e.g. App Installer deployments scoped to
		// multiple groups). Only truly distinct IDs are ambiguous.
		matches = deduplicateByID(matches, idField)
		if len(matches) == 1 {
			var obj map[string]any
			if err := json.Unmarshal(matches[0], &obj); err != nil {
				return "", nil, fmt.Errorf("decoding matched element: %w", err)
			}
			id, ok := extractField(obj, idField)
			if !ok {
				return "", matches[0], fmt.Errorf("matched element has no %s field", idField)
			}
			return id, matches[0], nil
		}
		return "", nil, &AmbiguousMatchError{Name: name, Matches: collectIDs(matches, idField)}
	}
}

// collectIDs returns the idField values of each element. Best-effort —
// elements that fail to decode or lack the field are skipped rather than
// aborting the caller's error path.
func collectIDs(elems []json.RawMessage, idField string) []string {
	ids := make([]string, 0, len(elems))
	for _, el := range elems {
		var obj map[string]any
		if err := json.Unmarshal(el, &obj); err != nil {
			continue
		}
		if id, ok := extractField(obj, idField); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// deduplicateByID removes duplicate matches that share the same idField
// value, keeping the first occurrence. Some endpoints return the same
// resource more than once (e.g. scoped to multiple groups); these are not
// truly ambiguous.
func deduplicateByID(elems []json.RawMessage, idField string) []json.RawMessage {
	seen := make(map[string]bool, len(elems))
	out := make([]json.RawMessage, 0, len(elems))
	for _, el := range elems {
		var obj map[string]any
		if err := json.Unmarshal(el, &obj); err != nil {
			out = append(out, el) // can't decode → keep as-is
			continue
		}
		id, ok := extractField(obj, idField)
		if !ok {
			out = append(out, el) // no ID → keep as-is
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, el)
	}
	return out
}

// extractListElements returns the element array of a list response. Accepts
// three shapes:
//   - paginated envelope keyed by resultsField: {"<resultsField>": [...], ...}
//   - raw top-level array: [...]
//   - single object: {...}  (returned as a one-element slice so matching applies uniformly)
//
// resultsField defaults to "results" when empty.
func extractListElements(body []byte, resultsField string) ([]json.RawMessage, error) {
	if resultsField == "" {
		resultsField = "results"
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '{' {
		var env map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &env); err == nil {
			if rawArr, ok := env[resultsField]; ok {
				var arr []json.RawMessage
				if err := json.Unmarshal(rawArr, &arr); err == nil {
					return arr, nil
				}
			}
		}
		return []json.RawMessage{append(json.RawMessage(nil), trimmed...)}, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(trimmed, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// extractField reads a dot-notation field path from a decoded JSON object
// (e.g. "name", "general.name") and returns the terminal value's string
// form. Non-string scalars coerce via strconv so numeric IDs — common in
// JSON-encoded responses for Classic-derived resources — round-trip cleanly
// without forcing callers to know the wire type.
func extractField(obj map[string]any, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	parts := strings.Split(path, ".")
	var current any = obj
	for i, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		v, exists := m[part]
		if !exists {
			return "", false
		}
		if i == len(parts)-1 {
			return coerceString(v)
		}
		current = v
	}
	return "", false
}

func coerceString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), true
		}
		return strconv.FormatFloat(x, 'g', -1, 64), true
	case bool:
		return strconv.FormatBool(x), true
	case nil:
		return "", false
	default:
		return fmt.Sprint(x), true
	}
}
