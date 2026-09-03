// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
)

// ListAllPages fetches all pages from a paginated REST endpoint, requesting
// pageSize items per page. Callers must pass a pageSize the endpoint is
// known to honor: some Jamf endpoints silently clamp an oversized page-size
// to a lower server-enforced maximum instead of rejecting it, and this
// function has no way to detect that — it assumes the server returned
// exactly pageSize items on every non-final page when computing the next
// page's offset. Requesting more than the true maximum causes those
// endpoints to silently skip the untransferred tail of one page on the next
// request. See ListGroupsV1/V2 in jamfplatform/pro for a verified example
// (server caps at 2000 regardless of the requested page-size).
func ListAllPages[T any](ctx context.Context, pageSize int, fetchPage func(ctx context.Context, page, pageSize int) ([]T, bool, error)) ([]T, error) {
	var allItems []T
	page := 0
	for {
		items, hasMore, err := fetchPage(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		allItems = append(allItems, items...)
		if !hasMore {
			break
		}
		page++
	}
	return allItems, nil
}

// ListAllCursorPages fetches all pages from a cursor-paginated REST endpoint,
// requesting pageSize items per page. fetchPage receives the cursor for the
// page to fetch — empty on the first call — and returns that page's items plus
// the cursor for the next page, empty when the page just returned was the last.
//
// Unlike ListAllPages this cannot skip data when the server clamps pageSize:
// the next page's position comes from the server's own cursor rather than from
// an offset the client computes, so a clamped page size costs extra round trips
// and nothing else. That is the whole reason cursor endpoints get their own
// walker instead of being forced into one of the offset styles — doing that
// would reintroduce exactly the silent-truncation risk documented above.
//
// An empty page is not a termination condition. Cursor endpoints may return one
// legitimately — a page whose every row was filtered out server-side still
// carries a cursor to the rows beyond it — so only an absent cursor ends the
// walk. That leaves a misbehaving server able to hand back the same cursor
// forever, which would hang the caller with no error, so a repeated cursor is
// treated as a protocol failure rather than trusted.
func ListAllCursorPages[T any](ctx context.Context, pageSize int, fetchPage func(ctx context.Context, cursor string, pageSize int) ([]T, string, error)) ([]T, error) {
	var allItems []T
	seen := make(map[string]bool)
	cursor := ""
	for {
		items, next, err := fetchPage(ctx, cursor, pageSize)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, items...)
		if next == "" {
			return allItems, nil
		}
		if seen[next] {
			return nil, fmt.Errorf("pagination cursor %q repeated after %d items: server is not advancing", next, len(allItems))
		}
		seen[next] = true
		cursor = next
	}
}
