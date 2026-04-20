// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package devices

import (
	"context"
	"net/http"
	"testing"
)

func TestListDevices_MultiPage(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	callCount := 0
	mux.HandleFunc("/api/devices/v1/tenant/t-test/devices", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results": []map[string]any{{"id": "d-1"}, {"id": "d-2"}},
				"hasNext": true,
			})
		case "1":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results": []map[string]any{{"id": "d-3"}},
				"hasNext": false,
			})
		default:
			t.Errorf("unexpected page %s", page)
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	devices, err := c.ListDevices(context.Background(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 3 {
		t.Fatalf("got %d devices, want 3", len(devices))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}
