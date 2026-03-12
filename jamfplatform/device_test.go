// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestListDevices(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("filter"); got != "name==test" {
			t.Errorf("filter = %q, want name==test", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"id": "d1", "name": "Mac1", "model": "MacBook Pro", "serialNumber": "ABC123"},
			},
			"totalCount": 1,
			"page":       0,
			"pageSize":   100,
			"hasNext":    false,
		})
	})

	devices, err := c.ListDevices(context.Background(), nil, "name==test")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("len = %d, want 1", len(devices))
	}
	if devices[0].ID != "d1" {
		t.Errorf("ID = %q, want d1", devices[0].ID)
	}
	if devices[0].SerialNumber != "ABC123" {
		t.Errorf("SerialNumber = %q, want ABC123", devices[0].SerialNumber)
	}
}

func TestListDevices_WithSort(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sort"); got != "name:asc,model:desc" {
			t.Errorf("sort = %q, want name:asc,model:desc", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []any{},
			"hasNext": false,
		})
	})

	_, err := c.ListDevices(context.Background(), []string{"name:asc", "model:desc"}, "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetDevice(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/dev-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"id":   "dev-123",
			"name": "TestMac",
		})
	})

	device, err := c.GetDevice(context.Background(), "dev-123")
	if err != nil {
		t.Fatal(err)
	}
	if device.ID != "dev-123" {
		t.Errorf("ID = %q, want dev-123", device.ID)
	}
	if device.Name != "TestMac" {
		t.Errorf("Name = %q, want TestMac", device.Name)
	}
}

func TestGetDevice_NotFound(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-1",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	_, err := c.GetDevice(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateDevice(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/dev-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		var body map[string]any
		readJSON(t, r, &body)
		if body["name"] != "NewName" {
			t.Errorf("name = %v, want NewName", body["name"])
		}
		w.WriteHeader(http.StatusNoContent)
	})

	name := "NewName"
	err := c.UpdateDevice(context.Background(), "dev-1", &DeviceUpdateRepresentationV1{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteDevice(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/dev-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.DeleteDevice(context.Background(), "dev-1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListDeviceApplications(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/dev-1/applications", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]string{
				{"name": "Safari", "version": "17.0"},
			},
			"hasNext": false,
		})
	})

	apps, err := c.ListDeviceApplications(context.Background(), "dev-1", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Name != "Safari" {
		t.Errorf("got %+v, want Safari", apps)
	}
}

func TestListDevicesForUser(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/users/user-1/devices", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"id": "d1", "name": "UserMac"},
			},
			"hasNext": false,
		})
	})

	devices, err := c.ListDevicesForUser(context.Background(), "user-1", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != "d1" {
		t.Errorf("got %+v", devices)
	}
}

func TestGetDeviceBySerialNumber(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") == "" {
			t.Error("expected filter parameter")
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"id": "dev-abc", "name": "Mac1", "serialNumber": "ABC123"},
			},
			"hasNext": false,
		})
	})
	mux.HandleFunc("/management/devices/v1/devices/dev-abc", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"id": "dev-abc", "name": "Mac1",
		})
	})

	device, err := c.GetDeviceBySerialNumber(context.Background(), "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if device.ID != "dev-abc" {
		t.Errorf("ID = %q, want dev-abc", device.ID)
	}
}

func TestGetDeviceBySerialNumber_NotFound(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []any{},
			"hasNext": false,
		})
	})

	_, err := c.GetDeviceBySerialNumber(context.Background(), "NOSUCH")
	if err == nil {
		t.Fatal("expected error for missing device")
	}
}

func TestGetDeviceBySerialNumber_MultipleMatches(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"id": "d1", "serialNumber": "DUP"},
				{"id": "d2", "serialNumber": "DUP"},
			},
			"hasNext": false,
		})
	})

	_, err := c.GetDeviceBySerialNumber(context.Background(), "DUP")
	if err == nil {
		t.Fatal("expected error for multiple devices")
	}
}

func TestGetDeviceBySerialNumber_EmptySerial(t *testing.T) {
	c, _ := testServer(t)
	_, err := c.GetDeviceBySerialNumber(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty serial")
	}
}

func TestListDevices_APIError(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusInternalServerError, map[string]any{
			"httpStatus": 500,
			"traceId":    "trace-err",
			"errors":     []map[string]string{{"code": "SERVER_ERROR", "field": "", "description": "internal error"}},
		})
	})

	_, err := c.ListDevices(context.Background(), nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIResponseError, got %T", err)
	}
	if !apiErr.HasStatus(500) {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
}

func TestUpdateDevice_APIError(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/dev-1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, map[string]any{
			"httpStatus": 400,
			"traceId":    "trace-bad",
			"errors":     []map[string]string{{"code": "INVALID_INPUT", "field": "name", "description": "invalid"}},
		})
	})

	name := "x"
	err := c.UpdateDevice(context.Background(), "dev-1", &DeviceUpdateRepresentationV1{Name: &name})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteDevice_NotFound(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	err := c.DeleteDevice(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListDeviceApplications_APIError(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/management/devices/v1/devices/dev-1/applications", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusForbidden, map[string]any{
			"httpStatus": 403,
			"traceId":    "trace-403",
			"errors":     []map[string]string{{"code": "FORBIDDEN", "field": "", "description": "access denied"}},
		})
	})

	_, err := c.ListDeviceApplications(context.Background(), "dev-1", nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
}
