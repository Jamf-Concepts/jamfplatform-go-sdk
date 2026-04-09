// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"net/http"
	"testing"
)

func TestListDeviceGroups(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"id": "g1", "name": "All Macs", "deviceType": "COMPUTER", "groupType": "SMART", "memberCount": 5},
			},
			"hasNext": false,
		})
	})

	groups, err := c.ListDeviceGroups(context.Background(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("len = %d, want 1", len(groups))
	}
	if groups[0].Name != "All Macs" {
		t.Errorf("Name = %q, want All Macs", groups[0].Name)
	}
}

func TestGetDeviceGroup(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/g1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"id": "g1", "name": "Test Group", "deviceType": "COMPUTER", "groupType": "STATIC", "memberCount": 3,
		})
	})

	group, err := c.GetDeviceGroup(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "g1" || group.GroupType != "STATIC" {
		t.Errorf("got %+v", group)
	}
}

func TestCreateDeviceGroup(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body DeviceGroupCreateRepresentationV1
		readJSON(t, r, &body)
		if body.Name != "New Group" {
			t.Errorf("Name = %q, want New Group", body.Name)
		}
		writeJSON(t, w, http.StatusCreated, map[string]string{"id": "g-new", "href": "/device-groups/g-new"})
	})

	resp, err := c.CreateDeviceGroup(context.Background(), &DeviceGroupCreateRepresentationV1{
		Name:       "New Group",
		DeviceType: "COMPUTER",
		GroupType:  "STATIC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "g-new" {
		t.Errorf("ID = %q, want g-new", resp.ID)
	}
}

func TestUpdateDeviceGroup(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/g1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.UpdateDeviceGroup(context.Background(), "g1", &DeviceGroupUpdateRepresentationV1{Name: "Updated"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteDeviceGroup(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/g1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.DeleteDeviceGroup(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListDeviceGroupMembers(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/g1/members", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results":  []string{"dev-1", "dev-2"},
			"hasNext":  false,
			"page":     0,
			"pageSize": 100,
		})
	})

	members, err := c.ListDeviceGroupMembers(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("len = %d, want 2", len(members))
	}
}

func TestUpdateDeviceGroupMembers(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/g1/members", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		var body DeviceGroupMemberPatchRepresentationV1
		readJSON(t, r, &body)
		if len(body.Added) != 1 || body.Added[0] != "dev-3" {
			t.Errorf("Added = %v, want [dev-3]", body.Added)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.UpdateDeviceGroupMembers(context.Background(), "g1", &DeviceGroupMemberPatchRepresentationV1{
		Added: []string{"dev-3"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetDeviceGroup_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	_, err := c.GetDeviceGroup(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateDeviceGroup_APIError(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, map[string]any{
			"httpStatus": 400,
			"traceId":    "trace-bad",
			"errors":     []map[string]string{{"code": "INVALID_INPUT", "field": "name", "description": "required"}},
		})
	})

	_, err := c.CreateDeviceGroup(context.Background(), &DeviceGroupCreateRepresentationV1{
		DeviceType: "COMPUTER",
		GroupType:  "STATIC",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateDeviceGroup_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	err := c.UpdateDeviceGroup(context.Background(), "missing", &DeviceGroupUpdateRepresentationV1{Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteDeviceGroup_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/device-groups/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	err := c.DeleteDeviceGroup(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListDeviceGroupsForDevice(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("/api/device-groups/v1/tenant/t-test/devices/dev-1/device-groups", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]string{
				{"groupId": "g1", "groupName": "Group 1"},
			},
			"hasNext": false,
		})
	})

	groups, err := c.ListDeviceGroupsForDevice(context.Background(), "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].GroupID != "g1" {
		t.Errorf("got %+v", groups)
	}
}
