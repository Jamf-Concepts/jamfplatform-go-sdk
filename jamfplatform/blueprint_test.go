// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListBlueprints(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		if got := r.URL.Query().Get("search"); got != "test" {
			t.Errorf("search = %q, want test", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"id": "bp-1", "name": "Blueprint 1"},
			},
			"totalCount": 1,
		})
	})

	bps, err := c.ListBlueprints(context.Background(), nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(bps) != 1 || bps[0].ID != "bp-1" {
		t.Errorf("got %+v", bps)
	}
}

func TestGetBlueprint(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/bp-1", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		writeJSON(t, w, http.StatusOK, BlueprintDetailV1{
			ID:   "bp-1",
			Name: "Test Blueprint",
			Scope: BlueprintUpdateScopeV1{
				DeviceGroups: []string{"g1"},
			},
			Steps: []BlueprintStepV1{
				{Name: "Step 1"},
			},
		})
	})

	bp, err := c.GetBlueprint(context.Background(), "bp-1")
	if err != nil {
		t.Fatal(err)
	}
	if bp.Name != "Test Blueprint" {
		t.Errorf("Name = %q, want Test Blueprint", bp.Name)
	}
	if len(bp.Scope.DeviceGroups) != 1 {
		t.Errorf("DeviceGroups len = %d, want 1", len(bp.Scope.DeviceGroups))
	}
}

func TestGetBlueprintByName(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"id": "bp-1", "name": "My Blueprint"},
				{"id": "bp-2", "name": "Other Blueprint"},
			},
			"totalCount": 2,
		})
	})
	mux.HandleFunc("/api/blueprints/v1/blueprints/bp-1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, BlueprintDetailV1{
			ID:   "bp-1",
			Name: "My Blueprint",
		})
	})

	bp, err := c.GetBlueprintByName(context.Background(), "My Blueprint")
	if err != nil {
		t.Fatal(err)
	}
	if bp.ID != "bp-1" {
		t.Errorf("ID = %q, want bp-1", bp.ID)
	}
}

func TestGetBlueprintByName_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results":    []any{},
			"totalCount": 0,
		})
	})

	_, err := c.GetBlueprintByName(context.Background(), "Nonexistent")
	if err == nil {
		t.Fatal("expected error for missing blueprint")
	}
}

func TestGetBlueprintByName_EmptyName(t *testing.T) {
	c, _ := testServerWithOpts(t, WithTenantID("t-abc-123"))
	_, err := c.GetBlueprintByName(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreateBlueprint(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		var body BlueprintCreateRequestV1
		readJSON(t, r, &body)
		if body.Name != "New BP" {
			t.Errorf("Name = %q, want New BP", body.Name)
		}
		writeJSON(t, w, http.StatusCreated, map[string]string{"id": "bp-new", "href": "/blueprints/bp-new"})
	})

	resp, err := c.CreateBlueprint(context.Background(), &BlueprintCreateRequestV1{
		Name: "New BP",
		Scope: BlueprintCreateScopeV1{
			DeviceGroups: []string{"g1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "bp-new" {
		t.Errorf("ID = %q, want bp-new", resp.ID)
	}
}

func TestUpdateBlueprint(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/bp-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.UpdateBlueprint(context.Background(), "bp-1", &BlueprintUpdateRequestV1{Name: "Updated"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteBlueprint(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/bp-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.DeleteBlueprint(context.Background(), "bp-1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeployBlueprint(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/bp-1/deploy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		w.WriteHeader(http.StatusAccepted)
	})

	err := c.DeployBlueprint(context.Background(), "bp-1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUndeployBlueprint(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/bp-1/undeploy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		w.WriteHeader(http.StatusAccepted)
	})

	err := c.UndeployBlueprint(context.Background(), "bp-1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListBlueprintComponents(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprint-components", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"identifier": "com.jamf.passcode", "name": "Passcode", "meta": map[string]any{"supportedOs": map[string]any{}}},
			},
			"totalCount": 1,
		})
	})

	comps, err := c.ListBlueprintComponents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(comps) != 1 || comps[0].Identifier != "com.jamf.passcode" {
		t.Errorf("got %+v", comps)
	}
}

func TestGetBlueprintComponent(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprint-components/com.jamf.passcode", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Tenant-Id"); got != "t-abc-123" {
			t.Errorf("X-Tenant-Id = %q, want t-abc-123", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"identifier":  "com.jamf.passcode",
			"name":        "Passcode",
			"description": "Passcode policy",
			"meta":        map[string]any{"supportedOs": map[string]any{}},
		})
	})

	comp, err := c.GetBlueprintComponent(context.Background(), "com.jamf.passcode")
	if err != nil {
		t.Fatal(err)
	}
	if comp.Name != "Passcode" {
		t.Errorf("Name = %q, want Passcode", comp.Name)
	}
}

func TestGetBlueprint_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	_, err := c.GetBlueprint(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateBlueprint_APIError(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, map[string]any{
			"httpStatus": 400,
			"traceId":    "trace-bad",
			"errors":     []map[string]string{{"code": "INVALID_INPUT", "field": "name", "description": "required"}},
		})
	})

	_, err := c.CreateBlueprint(context.Background(), &BlueprintCreateRequestV1{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeployBlueprint_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/missing/deploy", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	err := c.DeployBlueprint(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUndeployBlueprint_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprints/missing/undeploy", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
	})

	err := c.UndeployBlueprint(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetBlueprintComponent_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-abc-123"))
	mux.HandleFunc("/api/blueprints/v1/blueprint-components/nonexistent", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "identifier", "description": "not found"}},
		})
	})

	_, err := c.GetBlueprintComponent(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBlueprintComponentV1_Configuration(t *testing.T) {
	raw := json.RawMessage(`{"key":"value"}`)
	comp := BlueprintComponentV1{
		Identifier:    "test",
		Configuration: raw,
	}
	data, err := json.Marshal(comp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded BlueprintComponentV1
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if string(decoded.Configuration) != `{"key":"value"}` {
		t.Errorf("Configuration = %s", decoded.Configuration)
	}
}
