// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import "context"

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
