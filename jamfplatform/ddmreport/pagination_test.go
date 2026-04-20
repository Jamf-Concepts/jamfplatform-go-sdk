// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package ddmreport

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestListDeclarationReportClients_MultiPage(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	callCount := 0
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/declarations/decl-1", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"deviceId": fmt.Sprintf("dev-%d", i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 120,
			})
		case "1":
			items := make([]map[string]any, 20)
			for i := range items {
				items[i] = map[string]any{"deviceId": fmt.Sprintf("dev-%d", 100+i)}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    items,
				"totalCount": 120,
			})
		default:
			t.Errorf("unexpected page %s", page)
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	clients, err := c.ListDeclarationReportClients(context.Background(), "decl-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 120 {
		t.Fatalf("got %d clients, want 120", len(clients))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}
