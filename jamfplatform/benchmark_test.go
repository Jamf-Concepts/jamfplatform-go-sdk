// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"net/http"
	"testing"
)

func TestListBaselines(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v1/baselines", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, CBEngineBaselinesResponseV1{
			Baselines: []CBEngineBaselineInfoV1{
				{ID: "bl-1", Name: "CIS macOS 15", Title: "CIS Benchmark for macOS 15"},
			},
		})
	})

	resp, err := c.ListBaselines(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Baselines) != 1 || resp.Baselines[0].ID != "bl-1" {
		t.Errorf("got %+v", resp.Baselines)
	}
}

func TestListBenchmarks(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v2/benchmarks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, CBEngineBenchmarksResponseV2{
			Benchmarks: []CBEngineBenchmarkV2{
				{ID: "bm-1", Title: "My Benchmark", SyncState: "SYNCED"},
			},
		})
	})

	resp, err := c.ListBenchmarks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Benchmarks) != 1 || resp.Benchmarks[0].Title != "My Benchmark" {
		t.Errorf("got %+v", resp.Benchmarks)
	}
}

func TestGetBenchmark(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v2/benchmarks/bm-1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, CBEngineBenchmarkResponseV2{
			BenchmarkID:     "bm-1",
			Title:           "Test Benchmark",
			EnforcementMode: "AUDIT_ONLY",
		})
	})

	bm, err := c.GetBenchmark(context.Background(), "bm-1")
	if err != nil {
		t.Fatal(err)
	}
	if bm.BenchmarkID != "bm-1" {
		t.Errorf("BenchmarkID = %q, want bm-1", bm.BenchmarkID)
	}
	if bm.EnforcementMode != "AUDIT_ONLY" {
		t.Errorf("EnforcementMode = %q, want AUDIT_ONLY", bm.EnforcementMode)
	}
}

func TestCreateBenchmark(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v2/benchmarks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body CBEngineBenchmarkRequestV2
		readJSON(t, r, &body)
		if body.Title != "New Benchmark" {
			t.Errorf("Title = %q, want New Benchmark", body.Title)
		}
		writeJSON(t, w, http.StatusAccepted, CBEngineBenchmarkResponseV2{
			BenchmarkID: "bm-new",
			Title:       body.Title,
		})
	})

	resp, err := c.CreateBenchmark(context.Background(), &CBEngineBenchmarkRequestV2{
		Title:            "New Benchmark",
		SourceBaselineID: "bl-1",
		Target:           CBEngineTargetV2{DeviceGroups: []string{"g1"}},
		EnforcementMode:  "AUDIT_ONLY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.BenchmarkID != "bm-new" {
		t.Errorf("BenchmarkID = %q, want bm-new", resp.BenchmarkID)
	}
}

func TestDeleteBenchmark(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v1/benchmarks/bm-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.DeleteBenchmark(context.Background(), "bm-1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetBenchmarkByTitle(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v2/benchmarks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/compliance-benchmarks/preview/v2/benchmarks" {
			writeJSON(t, w, http.StatusOK, CBEngineBenchmarksResponseV2{
				Benchmarks: []CBEngineBenchmarkV2{
					{ID: "bm-1", Title: "Target"},
					{ID: "bm-2", Title: "Other"},
				},
			})
		}
	})
	mux.HandleFunc("/api/compliance-benchmarks/preview/v2/benchmarks/bm-1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, CBEngineBenchmarkResponseV2{
			BenchmarkID: "bm-1",
			Title:       "Target",
		})
	})

	bm, err := c.GetBenchmarkByTitle(context.Background(), "Target")
	if err != nil {
		t.Fatal(err)
	}
	if bm.BenchmarkID != "bm-1" {
		t.Errorf("BenchmarkID = %q, want bm-1", bm.BenchmarkID)
	}
}

func TestGetBenchmarkByTitle_NotFound(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v2/benchmarks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, CBEngineBenchmarksResponseV2{
			Benchmarks: []CBEngineBenchmarkV2{},
		})
	})

	_, err := c.GetBenchmarkByTitle(context.Background(), "Missing")
	if err == nil {
		t.Fatal("expected error for missing benchmark")
	}
}

func TestGetBaselineRules(t *testing.T) {
	c, mux := testServer(t)
	mux.HandleFunc("/api/compliance-benchmarks/preview/v1/rules", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("baselineId"); got != "bl-1" {
			t.Errorf("baselineId = %q, want bl-1", got)
		}
		writeJSON(t, w, http.StatusOK, CBEngineSourcedRulesV1{
			Sources: []CBEngineSourceV1{{Branch: "main", Revision: "abc123"}},
			Rules: []CBEngineRuleInfoV1{
				{ID: "rule-1", Title: "Test Rule", Enabled: true},
			},
		})
	})

	rules, err := c.GetBaselineRules(context.Background(), "bl-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 1 || rules.Rules[0].ID != "rule-1" {
		t.Errorf("got %+v", rules)
	}
}
