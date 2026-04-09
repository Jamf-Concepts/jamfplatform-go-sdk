// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"net/http"
	"testing"
)

func TestEraseDevice(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-actions/v1/tenant/t-test/devices/dev-1/erase", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body EraseDeviceRequestV1
		readJSON(t, r, &body)
		if body.Pin == nil || *body.Pin != "123456" {
			t.Errorf("Pin = %v, want 123456", body.Pin)
		}
		writeJSON(t, w, http.StatusCreated, []DeviceCommandResponseV1{
			{DeviceID: "dev-1", CommandID: "cmd-1"},
		})
	})

	pin := "123456"
	cmds, err := c.EraseDevice(context.Background(), "dev-1", &EraseDeviceRequestV1{Pin: &pin})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 || cmds[0].CommandID != "cmd-1" {
		t.Errorf("got %+v", cmds)
	}
}

func TestRestartDevice(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-actions/v1/tenant/t-test/devices/dev-1/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		writeJSON(t, w, http.StatusCreated, []DeviceCommandResponseV1{
			{DeviceID: "dev-1", CommandID: "cmd-2"},
		})
	})

	cmds, err := c.RestartDevice(context.Background(), "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 || cmds[0].CommandID != "cmd-2" {
		t.Errorf("got %+v", cmds)
	}
}

func TestShutdownDevice(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-actions/v1/tenant/t-test/devices/dev-1/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		writeJSON(t, w, http.StatusCreated, []DeviceCommandResponseV1{
			{DeviceID: "dev-1", CommandID: "cmd-3"},
		})
	})

	cmds, err := c.ShutdownDevice(context.Background(), "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Errorf("len = %d, want 1", len(cmds))
	}
}

func TestUnmanageDevice(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-actions/v1/tenant/t-test/devices/dev-1/unmanage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		writeJSON(t, w, http.StatusCreated, []DeviceCommandResponseV1{
			{DeviceID: "dev-1", CommandID: "cmd-4"},
		})
	})

	cmds, err := c.UnmanageDevice(context.Background(), "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Errorf("len = %d, want 1", len(cmds))
	}
}

func TestDeviceAction_EmptyID(t *testing.T) {
	c, _ := testServerWithOpts(t, WithTenantID("t-test"))
	_, err := c.EraseDevice(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for empty device ID")
	}
}

func TestDeviceAction_APIError(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-actions/v1/tenant/t-test/devices/dev-1/restart", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "device not found"}},
		})
	})

	_, err := c.RestartDevice(context.Background(), "dev-1")
	if err == nil {
		t.Fatal("expected error for missing device")
	}
}
