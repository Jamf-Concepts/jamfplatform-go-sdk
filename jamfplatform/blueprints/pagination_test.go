// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package blueprints

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestListBlueprints_MultiPage(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	callCount := 0
	mux.HandleFunc("/api/blueprints/v1/tenant/t-test/blueprints", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("bp-%d", i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 150,
			})
		case "1":
			items := make([]map[string]any, 50)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("bp-%d", 100+i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 150,
			})
		default:
			t.Errorf("unexpected page %s", page)
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	bps, err := c.ListBlueprints(context.Background(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(bps) != 150 {
		t.Fatalf("got %d blueprints, want 150", len(bps))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}
