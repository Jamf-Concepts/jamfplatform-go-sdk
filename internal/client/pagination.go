// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import "context"

// ListAllPages fetches all pages from a paginated REST endpoint.
// The fetchPage function should return the items for that page and whether there are more pages.
func ListAllPages[T any](ctx context.Context, fetchPage func(ctx context.Context, page, pageSize int) ([]T, bool, error)) ([]T, error) {
	var allItems []T
	page := 0
	pageSize := 100
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
