// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Batch 19 — dock-items + log-flushing + scheduler + ebooks + slasa.
// Grab-bag of read-heavy endpoints plus dock-items CRUD and a
// log-flushing task probe. Slasa accept is skipped — it's a
// tenant-wide EULA click that shouldn't auto-accept from a smoke test.

// --- dock-items CRUD -------------------------------------------------

func TestAcceptance_Pro_DockItemV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-dockitem-" + runSuffix()
	created, err := p.CreateDockItemV1(ctx, &pro.DockItem{
		Name: name,
		Type: "APP",
		Path: "file:///Applications/Safari.app/",
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDockItemV1: %v", err)
	}
	id := created.ID
	t.Logf("Created dock item id=%s name=%s", id, name)
	cleanupDelete(t, "DockItem "+id, func() error { return p.DeleteDockItemV1(ctx, id) })

	got, err := p.GetDockItemV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetDockItemV1: %v", err)
	}
	if got.Name != name {
		t.Errorf("name round-trip mismatch: got %q, want %q", got.Name, name)
	}

	got.Name = name + "-upd"
	if _, err := p.UpdateDockItemV1(ctx, id, got); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateDockItemV1: %v", err)
	}
}

// --- log-flushing ----------------------------------------------------

func TestAcceptance_Pro_LogFlushingV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	cfg, err := p.GetLogFlushingV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetLogFlushingV1: %v", err)
	}
	t.Logf("Log-flushing: hourOfDay=%d policies=%d", cfg.HourOfDay, len(cfg.RetentionPolicies))

	tasks, err := p.ListLogFlushingTasksV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListLogFlushingTasksV1: %v", err)
	}
	t.Logf("Log-flushing tasks: %d existing", len(tasks))
}

// Creating a flush task against a live tenant will actually prune log
// data, so attempt only with a clearly-invalid qualifier and tolerate
// 4xx. Deleting a task we don't own could race with server-side task
// lifecycle; we're just probing reachability here.
func TestAcceptance_Pro_LogFlushingTaskProbeV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Deliberately-invalid qualifier — the server rejects before
	// executing any flush. Confirms the endpoint is reachable.
	_, err := p.CreateLogFlushingTaskV1(ctx, &pro.LogFlushingTaskV1{
		Qualifier:           "sdk-acc-nonexistent-qualifier",
		RetentionPeriod:     1,
		RetentionPeriodUnit: "DAYS",
	})
	if err == nil {
		t.Log("CreateLogFlushingTaskV1 unexpectedly succeeded — qualifier may have matched a real log stream")
		return
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("CreateLogFlushingTaskV1 rejected: status=%d — expected for invalid qualifier", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Fatalf("CreateLogFlushingTaskV1: %v", err)
}

// --- scheduler -------------------------------------------------------

func TestAcceptance_Pro_SchedulerV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	summary, err := p.GetSchedulerSummaryV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSchedulerSummaryV1: %v", err)
	}
	t.Logf("Scheduler summary: %+v", summary)

	jobs, err := p.GetSchedulerJobsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSchedulerJobsV1: %v", err)
	}
	t.Logf("Scheduler jobs retrieved")

	// Triggers lookup needs a real jobKey; probe with a bogus one and
	// tolerate 4xx.
	if _, err := p.GetSchedulerJobTriggersV1(ctx, "sdk-acc-fake-job", nil, ""); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("GetSchedulerJobTriggersV1 probe: status=%d — expected for bogus jobKey", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetSchedulerJobTriggersV1: %v", err)
		}
	}
	_ = jobs
}

// --- ebooks ----------------------------------------------------------

func TestAcceptance_Pro_EbooksV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	list, err := p.ListEbooksV1(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListEbooksV1: %v", err)
	}
	t.Logf("Ebooks: %d", len(list))

	// Probe GET by-id and scope with a bogus id — tolerate 404.
	if _, err := p.GetEbookV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			t.Logf("GetEbookV1(-1): 404 — expected")
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetEbookV1: %v", err)
		}
	}
	if _, err := p.GetEbookScopeV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			t.Logf("GetEbookScopeV1(-1): 404 — expected")
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetEbookScopeV1: %v", err)
		}
	}
}

// --- slasa -----------------------------------------------------------

// POST /v1/slasa accepts the Software License + Service Agreement for
// the whole tenant — not something a smoke test should trigger. Only
// exercise the read.
func TestAcceptance_Pro_SlasaReadV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	slasa, err := pro.New(c).GetSlasaAcceptanceV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSlasaAcceptanceV1: %v", err)
	}
	t.Logf("SLASA acceptance: %+v", slasa)
}
