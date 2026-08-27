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
		items, err := ListAllPages(context.Background(), 100, func(_ context.Context, page, pageSize int) ([]string, bool, error) {
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
		items, err := ListAllPages(context.Background(), 100, func(_ context.Context, page, _ int) ([]int, bool, error) {
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
		items, err := ListAllPages(context.Background(), 100, func(_ context.Context, _, _ int) ([]string, bool, error) {
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
		_, err := ListAllPages(context.Background(), 100, func(_ context.Context, _, _ int) ([]string, bool, error) {
			return nil, false, fmt.Errorf("fetch error")
		})
		if err == nil || err.Error() != "fetch error" {
			t.Fatalf("got err=%v, want 'fetch error'", err)
		}
	})

	t.Run("error on second page", func(t *testing.T) {
		_, err := ListAllPages(context.Background(), 100, func(_ context.Context, page, _ int) ([]string, bool, error) {
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
		items, err := ListAllPages(context.Background(), 100, func(_ context.Context, _, _ int) ([]string, bool, error) {
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

func TestListAllCursorPages(t *testing.T) {
	t.Run("single page", func(t *testing.T) {
		items, err := ListAllCursorPages(context.Background(), 100, func(_ context.Context, cursor string, pageSize int) ([]string, string, error) {
			if cursor != "" {
				t.Fatalf("first call should carry no cursor, got %q", cursor)
			}
			if pageSize != 100 {
				t.Fatalf("pageSize = %d, want 100", pageSize)
			}
			return []string{"a", "b"}, "", nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || items[0] != "a" || items[1] != "b" {
			t.Fatalf("got %v, want [a b]", items)
		}
	})

	t.Run("follows the cursor across pages", func(t *testing.T) {
		var seen []string
		items, err := ListAllCursorPages(context.Background(), 50, func(_ context.Context, cursor string, _ int) ([]int, string, error) {
			seen = append(seen, cursor)
			switch cursor {
			case "":
				return []int{1, 2}, "c1", nil
			case "c1":
				return []int{3}, "c2", nil
			case "c2":
				return []int{4}, "", nil
			default:
				t.Fatalf("unexpected cursor %q", cursor)
				return nil, "", nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := []int{1, 2, 3, 4}; len(items) != len(want) {
			t.Fatalf("got %v, want %v", items, want)
		}
		if len(seen) != 3 || seen[0] != "" || seen[1] != "c1" || seen[2] != "c2" {
			t.Fatalf("cursors seen = %v, want [\"\" c1 c2]", seen)
		}
	})

	// An empty page must not end the walk: a cursor endpoint can return one
	// legitimately when every row on it was filtered out server-side, and the
	// rows beyond it are still reachable through the cursor it carries.
	t.Run("empty page with a cursor keeps going", func(t *testing.T) {
		items, err := ListAllCursorPages(context.Background(), 100, func(_ context.Context, cursor string, _ int) ([]string, string, error) {
			switch cursor {
			case "":
				return nil, "c1", nil
			case "c1":
				return []string{"z"}, "", nil
			default:
				t.Fatalf("unexpected cursor %q", cursor)
				return nil, "", nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0] != "z" {
			t.Fatalf("got %v, want [z]", items)
		}
	})

	// A server that keeps handing back the same cursor would otherwise hang the
	// caller forever with no error.
	t.Run("repeated cursor is an error, not a hang", func(t *testing.T) {
		calls := 0
		_, err := ListAllCursorPages(context.Background(), 100, func(_ context.Context, _ string, _ int) ([]string, string, error) {
			calls++
			if calls > 10 {
				t.Fatal("walker did not stop on a repeated cursor")
			}
			return []string{"a"}, "stuck", nil
		})
		if err == nil {
			t.Fatal("want an error on a repeated cursor, got nil")
		}
		if calls != 2 {
			t.Fatalf("calls = %d, want 2 (first page, then the repeat is detected)", calls)
		}
	})

	t.Run("propagates fetch errors", func(t *testing.T) {
		want := fmt.Errorf("boom")
		_, err := ListAllCursorPages(context.Background(), 100, func(_ context.Context, _ string, _ int) ([]string, string, error) {
			return nil, "", want
		})
		if err == nil {
			t.Fatal("want an error, got nil")
		}
	})
}
