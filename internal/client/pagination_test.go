// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
	"testing"
)

func TestListAllPages(t *testing.T) {
	t.Run("single page", func(t *testing.T) {
		items, err := ListAllPages(context.Background(), func(_ context.Context, page, pageSize int) ([]string, bool, error) {
			if page != 0 {
				t.Fatalf("unexpected page %d", page)
			}
			return []string{"a", "b"}, false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || items[0] != "a" || items[1] != "b" {
			t.Fatalf("got %v, want [a b]", items)
		}
	})

	t.Run("multiple pages", func(t *testing.T) {
		items, err := ListAllPages(context.Background(), func(_ context.Context, page, _ int) ([]int, bool, error) {
			switch page {
			case 0:
				return []int{1, 2}, true, nil
			case 1:
				return []int{3, 4}, true, nil
			case 2:
				return []int{5}, false, nil
			default:
				t.Fatalf("unexpected page %d", page)
				return nil, false, nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 5 {
			t.Fatalf("got %d items, want 5", len(items))
		}
		for i, want := range []int{1, 2, 3, 4, 5} {
			if items[i] != want {
				t.Errorf("items[%d] = %d, want %d", i, items[i], want)
			}
		}
	})

	t.Run("empty first page", func(t *testing.T) {
		items, err := ListAllPages(context.Background(), func(_ context.Context, _, _ int) ([]string, bool, error) {
			return nil, false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Fatalf("got %d items, want 0", len(items))
		}
	})

	t.Run("error on first page", func(t *testing.T) {
		_, err := ListAllPages(context.Background(), func(_ context.Context, _, _ int) ([]string, bool, error) {
			return nil, false, fmt.Errorf("fetch error")
		})
		if err == nil || err.Error() != "fetch error" {
			t.Fatalf("got err=%v, want 'fetch error'", err)
		}
	})

	t.Run("error on second page", func(t *testing.T) {
		_, err := ListAllPages(context.Background(), func(_ context.Context, page, _ int) ([]string, bool, error) {
			if page == 0 {
				return []string{"a"}, true, nil
			}
			return nil, false, fmt.Errorf("page 1 error")
		})
		if err == nil || err.Error() != "page 1 error" {
			t.Fatalf("got err=%v, want 'page 1 error'", err)
		}
	})

	t.Run("hasMore true but empty results stops", func(t *testing.T) {
		calls := 0
		items, err := ListAllPages(context.Background(), func(_ context.Context, _, _ int) ([]string, bool, error) {
			calls++
			if calls == 1 {
				return []string{"a"}, true, nil
			}
			return nil, true, nil // hasMore=true but no items
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
	})
}
