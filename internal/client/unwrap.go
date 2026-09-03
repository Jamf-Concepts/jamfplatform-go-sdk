// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// UnwrapResults performs the request and returns the element slice of an
// unpaginated list response, accepting either shape such an endpoint sends:
//
//	{"totalCount": 2, "results": [...]}   the envelope the specs declare
//	[...]                                 a bare JSON array
//
// Both have been served in production by the same operation. The four account
// list endpoints answered with a bare array until 2026-09-01 and with the
// envelope after, and a method decoding into one shape fails on the other with
// `json: cannot unmarshal …` on every call — a break that no generated
// httptest handler can see, because the stub serves whatever the SDK assumed.
// Deciding on the byte the server actually sent costs one branch and makes the
// generated methods indifferent to which shape arrives, in either direction.
//
// resultsField names the envelope key; empty means "results".
func UnwrapResults[T any](ctx context.Context, t *Transport, method, endpoint, resultsField string) ([]T, error) {
	var raw json.RawMessage
	if err := t.Do(ctx, method, endpoint, nil, &raw); err != nil {
		return nil, err
	}
	return unwrapResultList[T](raw, resultsField)
}

// unwrapResultList is UnwrapResults' decode half, split out so both shapes and
// every malformed body are testable without a server.
func unwrapResultList[T any](body []byte, resultsField string) ([]T, error) {
	if resultsField == "" {
		resultsField = "results"
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}

	if trimmed[0] == '{' {
		var env map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &env); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		raw, ok := env[resultsField]
		if !ok {
			// An envelope omitting the key is an empty result set, which is
			// how the struct decode this replaced reported it too. Returning
			// an error here would turn a zero-row page into a failure.
			return nil, nil
		}
		var out []T
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("failed to decode response field %q: %w", resultsField, err)
		}
		return out, nil
	}

	// A bare array, or JSON null — which unmarshals to a nil slice.
	var out []T
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return out, nil
}
