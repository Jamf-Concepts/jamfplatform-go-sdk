// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"net/http"
	"testing"
)

func TestGetDeviceDeclarationReport(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/devices/dev-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, DeviceReportV1{
			Channels: []DeviceReportChannelV1{
				{
					Channel:        "SYSTEM",
					LastReportTime: "2026-01-15T10:00:00Z",
					Declarations: []StatusReportDeclarationV1{
						{
							DeclarationIdentifier: "decl-1",
							Type:                  "CONFIGURATION",
							Status:                "SUCCESSFUL",
							Active:                true,
							ValidityState:         "VALID",
							ServerToken:            "abc123",
						},
					},
				},
			},
		})
	})

	report, err := c.GetDeviceDeclarationReport(context.Background(), "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(report.Channels))
	}
	ch := report.Channels[0]
	if ch.Channel != "SYSTEM" {
		t.Errorf("channel = %q, want SYSTEM", ch.Channel)
	}
	if len(ch.Declarations) != 1 {
		t.Fatalf("declarations = %d, want 1", len(ch.Declarations))
	}
	decl := ch.Declarations[0]
	if decl.DeclarationIdentifier != "decl-1" {
		t.Errorf("declarationIdentifier = %q, want decl-1", decl.DeclarationIdentifier)
	}
	if decl.Status != "SUCCESSFUL" {
		t.Errorf("status = %q, want SUCCESSFUL", decl.Status)
	}
	if !decl.Active {
		t.Error("active = false, want true")
	}
}

func TestGetDeviceDeclarationReport_WithReasons(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/devices/dev-2", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, DeviceReportV1{
			Channels: []DeviceReportChannelV1{
				{
					Channel:        "USER",
					LastReportTime: "2026-01-15T10:00:00Z",
					Declarations: []StatusReportDeclarationV1{
						{
							DeclarationIdentifier: "decl-2",
							Type:                  "CONFIGURATION",
							Status:                "UNSUCCESSFUL",
							Active:                false,
							ValidityState:         "INVALID",
							ServerToken:            "token-2",
							Reasons: []StatusReportDeclarationReasonV1{
								{
									Code:        "Error.ConfigurationAlreadyPresent",
									Description: "Configuration cannot be applied",
									Details: []StatusReportDeclarationReasonDetailV1{
										{Key: "Identifier", Description: "f5d98fac-5f8a-4bde-b1f8-308a062dd591"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	report, err := c.GetDeviceDeclarationReport(context.Background(), "dev-2")
	if err != nil {
		t.Fatal(err)
	}
	decl := report.Channels[0].Declarations[0]
	if len(decl.Reasons) != 1 {
		t.Fatalf("reasons = %d, want 1", len(decl.Reasons))
	}
	if decl.Reasons[0].Code != "Error.ConfigurationAlreadyPresent" {
		t.Errorf("reason code = %q", decl.Reasons[0].Code)
	}
	if len(decl.Reasons[0].Details) != 1 || decl.Reasons[0].Details[0].Key != "Identifier" {
		t.Errorf("reason details = %+v", decl.Reasons[0].Details)
	}
}

func TestGetDeviceDeclarationReport_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/devices/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "deviceId", "description": "not found"}},
		})
	})

	_, err := c.GetDeviceDeclarationReport(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListDeclarationReportClients(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/declarations/decl-abc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("page"); got != "0" {
			t.Errorf("page = %q, want 0", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"declarationIdentifier": "decl-abc",
			"totalCount":            1,
			"results": []DeclarationReportClientV1{
				{
					DeviceID:      "dev-1",
					Channel:       "SYSTEM",
					Active:        true,
					ValidityState: "VALID",
					ServerToken:   "tok-1",
				},
			},
		})
	})

	clients, err := c.ListDeclarationReportClients(context.Background(), "decl-abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(clients))
	}
	if clients[0].DeviceID != "dev-1" {
		t.Errorf("deviceId = %q, want dev-1", clients[0].DeviceID)
	}
	if !clients[0].Active {
		t.Error("active = false, want true")
	}
}

func TestListDeclarationReportClients_WithSort(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/declarations/decl-abc", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sort"); got != "deviceId,asc" {
			t.Errorf("sort = %q, want deviceId,asc", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"declarationIdentifier": "decl-abc",
			"totalCount":            0,
			"results":               []any{},
		})
	})

	_, err := c.ListDeclarationReportClients(context.Background(), "decl-abc", []string{"deviceId,asc"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListDeclarationReportClients_APIError(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/ddm/report/v1/tenant/t-test/declarations/bad", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, map[string]any{
			"httpStatus": 400,
			"traceId":    "trace-bad",
			"errors":     []map[string]string{{"code": "BAD_REQUEST", "field": "", "description": "invalid"}},
		})
	})

	_, err := c.ListDeclarationReportClients(context.Background(), "bad", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
