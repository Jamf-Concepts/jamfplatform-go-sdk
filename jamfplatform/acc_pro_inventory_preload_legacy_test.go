// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// The deprecated inventory-preload surface: the /v1/ operations and their
// unversioned aliases. Both are superseded by /v2/inventory-preload/records,
// covered in acc_pro_inventory_test.go, and both remain live — wire-verified
// 2026-08-31 against eu.api.jamfcloud.com.
//
// The two forms are the same handler, with one difference the SDK has to encode:
// the unversioned list honours `pagesize` and *ignores* `page-size`, while the
// /v1/ list honours both. That is why ListInventoryPreloadRecords carries
// pageSizeParam "pagesize" in config — without it ListAllPages would multiply an
// offset by a page size the server never applied. Probed with four seeded
// records: `pagesize=2` returned 2, `page-size=2` returned all 4.
//
// DeleteAllInventoryPreloadRecords{,V1} wipe every preload record on the tenant,
// so they are gated behind an explicit opt-in rather than run by default.

// preloadLegacySerial builds a serial unique to this run and variant, so the v1
// and unversioned tests can run in parallel without colliding.
func preloadLegacySerial(variant string) string {
	return "sdkacc" + variant + runSuffix()
}

// --- /v1/inventory-preload ---------------------------------------------

func TestAcceptance_Pro_InventoryPreload_RecordCRUDV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	serial := preloadLegacySerial("v1")
	dept := "SDK Acceptance"

	created, err := p.CreateInventoryPreloadRecordV1(ctx, &pro.InventoryPreloadRecord{
		SerialNumber: serial,
		DeviceType:   pro.InventoryPreloadRecordDeviceTypeComputer,
		Department:   &dept,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateInventoryPreloadRecordV1: %v", err)
	}
	if created.ID == nil {
		t.Fatal("CreateInventoryPreloadRecordV1 returned no ID")
	}
	id := strconv.Itoa(*created.ID)
	cleanupDelete(t, "DeleteInventoryPreloadRecordV1", func() error { return p.DeleteInventoryPreloadRecordV1(ctx, id) })
	t.Logf("Created preload record %s (%s)", id, serial)

	got, err := p.GetInventoryPreloadRecordV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetInventoryPreloadRecordV1(%s): %v", id, err)
	}
	if got.SerialNumber != serial {
		t.Errorf("SerialNumber = %q, want %q", got.SerialNumber, serial)
	}

	newDept := dept + " (updated)"
	got.Department = &newDept
	updated, err := p.UpdateInventoryPreloadRecordV1(ctx, id, got)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateInventoryPreloadRecordV1(%s): %v", id, err)
	}
	if updated.Department == nil || *updated.Department != newDept {
		t.Errorf("Department = %v, want %q", updated.Department, newDept)
	}

	// The list must contain the record, and — this is the part the response
	// shape depends on — its elements must be records, not search envelopes.
	records, err := p.ListInventoryPreloadRecordsV1(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListInventoryPreloadRecordsV1: %v", err)
	}
	if !containsPreloadSerial(records, serial) {
		t.Errorf("ListInventoryPreloadRecordsV1 (%d records) does not contain %q", len(records), serial)
	}
	t.Logf("ListInventoryPreloadRecordsV1: %d records", len(records))

	if err := p.DeleteInventoryPreloadRecordV1(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteInventoryPreloadRecordV1(%s): %v", id, err)
	}

	_, err = p.GetInventoryPreloadRecordV1(ctx, id)
	if err == nil {
		t.Fatalf("GetInventoryPreloadRecordV1(%s) after delete should 404", id)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetInventoryPreloadRecordV1(%s) after delete: want 404, got %v", id, err)
	}
}

func TestAcceptance_Pro_InventoryPreload_HistoryV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	entries, err := p.ListInventoryPreloadHistoryV1(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListInventoryPreloadHistoryV1: %v", err)
	}
	t.Logf("Inventory preload history (v1): %d entries", len(entries))

	note := "sdk-acc inventory-preload history note " + runSuffix()
	created, err := p.CreateInventoryPreloadHistoryNoteV1(ctx, &pro.ObjectHistoryNote{Note: note})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateInventoryPreloadHistoryNoteV1: %v", err)
	}
	t.Logf("Created history note %d", created.ID)
}

func TestAcceptance_Pro_InventoryPreload_ValidateCsvV1(t *testing.T) {
	c := accClient(t)

	// A single valid row must validate cleanly. The header names are the
	// human-readable ones the server's own template uses, not the JSON field
	// names — a body built from the latter comes back 412 INVALID_DEVICE_TYPE.
	csv := "Serial Number,Device Type\n" + preloadLegacySerial("csv") + ",Computer\n"
	res, err := pro.New(c).ValidateInventoryPreloadCsvV1(context.Background(), []byte(csv))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ValidateInventoryPreloadCsvV1: %v", err)
	}
	if res.RecordCount != 1 {
		t.Errorf("recordCount = %d, want 1", res.RecordCount)
	}
	t.Logf("ValidateInventoryPreloadCsvV1 accepted %d record(s)", res.RecordCount)
}

// TestAcceptance_Pro_InventoryPreload_ValidateCsvV1RejectsBadRow is the negative
// half: the endpoint must reject a row whose device type is absent, and it must
// do so with its own documented code rather than a generic failure.
func TestAcceptance_Pro_InventoryPreload_ValidateCsvV1RejectsBadRow(t *testing.T) {
	c := accClient(t)

	csv := "Serial Number,Device Type\n" + preloadLegacySerial("bad") + ",\n"
	_, err := pro.New(c).ValidateInventoryPreloadCsvV1(context.Background(), []byte(csv))
	if err == nil {
		t.Fatal("ValidateInventoryPreloadCsvV1 accepted a row with no device type")
	}
	skipOnServerError(t, err)
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ValidateInventoryPreloadCsvV1: non-API error: %v", err)
	}
	if !apiErr.HasStatus(412) {
		t.Fatalf("ValidateInventoryPreloadCsvV1: want 412, got status %d: %v", apiErr.StatusCode, err)
	}
	for _, d := range apiErr.Details() {
		if d.Code == "INVALID_DEVICE_TYPE" {
			t.Logf("ValidateInventoryPreloadCsvV1 rejected the bad row with INVALID_DEVICE_TYPE on field %q", d.Field)
			return
		}
	}
	t.Fatalf("ValidateInventoryPreloadCsvV1: 412 but not INVALID_DEVICE_TYPE: %v", err)
}

// TestAcceptance_Pro_InventoryPreload_CsvTemplate covers both deprecated
// template downloads. Both answer 200 text/csv with the same human-readable
// header row the V2 template uses.
//
// The header the SDK does *not* send is what makes them work. Jamf Pro maps both
// /inventory-preload/{id} (produces JSON) and /inventory-preload/csv-template
// (produces text/csv) under the same prefix, so a request carrying
// `Accept: application/json` is content-negotiated onto the {id} handler and
// comes back 400 INVALID_REQUEST_PARAMETER_TYPE trying to parse the literal
// "csv-template" as a record id. The SDK sends no Accept on these and gets the
// template; a curl probe that adds one sees a failure that is an artefact of the
// probe. Established 2026-08-31 with both header forms against the same tenant.
func TestAcceptance_Pro_InventoryPreload_CsvTemplate(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	for _, tc := range []struct {
		name string
		call func() ([]byte, error)
	}{
		{"DownloadInventoryPreloadCsvTemplateV1", func() ([]byte, error) { return p.DownloadInventoryPreloadCsvTemplateV1(ctx) }},
		{"DownloadInventoryPreloadCsvTemplate", func() ([]byte, error) { return p.DownloadInventoryPreloadCsvTemplate(ctx) }},
	} {
		body, err := tc.call()
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(body) == 0 {
			t.Errorf("%s: CSV template body empty", tc.name)
			continue
		}
		header := string(body)
		if nl := strings.IndexByte(header, '\n'); nl >= 0 {
			header = header[:nl]
		}
		if !strings.Contains(strings.ToLower(header), "serial") {
			t.Errorf("%s: CSV template header %q does not contain 'serial'", tc.name, header)
		}
		t.Logf("%s: %d bytes; header: %s", tc.name, len(body), header)
	}
}

// --- unversioned /inventory-preload aliases ---------------------------

func TestAcceptance_Pro_InventoryPreload_RecordCRUDUnversioned(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	serial := preloadLegacySerial("unv")
	dept := "SDK Acceptance"

	created, err := p.CreateInventoryPreloadRecord(ctx, &pro.InventoryPreloadRecord{
		SerialNumber: serial,
		DeviceType:   pro.InventoryPreloadRecordDeviceTypeComputer,
		Department:   &dept,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateInventoryPreloadRecord: %v", err)
	}
	if created.ID == nil {
		t.Fatal("CreateInventoryPreloadRecord returned no ID")
	}
	id := strconv.Itoa(*created.ID)
	cleanupDelete(t, "DeleteInventoryPreloadRecord", func() error { return p.DeleteInventoryPreloadRecord(ctx, id) })
	t.Logf("Created preload record %s (%s)", id, serial)

	got, err := p.GetInventoryPreloadRecord(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetInventoryPreloadRecord(%s): %v", id, err)
	}
	if got.SerialNumber != serial {
		t.Errorf("SerialNumber = %q, want %q", got.SerialNumber, serial)
	}

	newDept := dept + " (updated)"
	got.Department = &newDept
	updated, err := p.UpdateInventoryPreloadRecord(ctx, id, got)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateInventoryPreloadRecord(%s): %v", id, err)
	}
	if updated.Department == nil || *updated.Department != newDept {
		t.Errorf("Department = %v, want %q", updated.Department, newDept)
	}

	// The unversioned list is where the spec disagrees with the server: it
	// declares application/json as an *array of* the search-results envelope,
	// while the wire sends the bare envelope (confirmed 2026-08-31). A
	// regression in that override shows up here as an element type that has no
	// serial number to match.
	records, err := p.ListInventoryPreloadRecords(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListInventoryPreloadRecords: %v", err)
	}
	if !containsPreloadSerial(records, serial) {
		t.Errorf("ListInventoryPreloadRecords (%d records) does not contain %q", len(records), serial)
	}
	t.Logf("ListInventoryPreloadRecords: %d records", len(records))

	if err := p.DeleteInventoryPreloadRecord(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteInventoryPreloadRecord(%s): %v", id, err)
	}

	_, err = p.GetInventoryPreloadRecord(ctx, id)
	if err == nil {
		t.Fatalf("GetInventoryPreloadRecord(%s) after delete should 404", id)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetInventoryPreloadRecord(%s) after delete: want 404, got %v", id, err)
	}
}

func TestAcceptance_Pro_InventoryPreload_HistoryUnversioned(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	entries, err := p.ListInventoryPreloadHistory(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListInventoryPreloadHistory: %v", err)
	}
	t.Logf("Inventory preload history (unversioned): %d entries", len(entries))

	// This tenant carries 2052 history rows, past the 2000 page-size clamp
	// every pro totalCount list applies, so the walk is a real multi-page walk
	// and a truncating or duplicating pagination bug would show up as a count
	// that disagrees with the sum of its pages.
	if len(entries) > 2000 {
		seen := make(map[int]struct{}, len(entries))
		for _, e := range entries {
			if _, dup := seen[e.ID]; dup {
				t.Fatalf("ListInventoryPreloadHistory returned entry %d twice — the walk is concatenating a repeated page", e.ID)
			}
			seen[e.ID] = struct{}{}
		}
		t.Logf("Walked %d entries across multiple pages with no repeats", len(entries))
	}

	note := "sdk-acc inventory-preload history note " + runSuffix()
	created, err := p.CreateInventoryPreloadHistoryNote(ctx, &pro.ObjectHistoryNote{Note: note})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateInventoryPreloadHistoryNote: %v", err)
	}
	t.Logf("Created history note %d", created.ID)
}

func TestAcceptance_Pro_InventoryPreload_ValidateCsvUnversioned(t *testing.T) {
	c := accClient(t)

	csv := "Serial Number,Device Type\n" + preloadLegacySerial("csvu") + ",Computer\n"
	res, err := pro.New(c).ValidateInventoryPreloadCsv(context.Background(), []byte(csv))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ValidateInventoryPreloadCsv: %v", err)
	}
	if res.RecordCount != 1 {
		t.Errorf("recordCount = %d, want 1", res.RecordCount)
	}
	t.Logf("ValidateInventoryPreloadCsv accepted %d record(s)", res.RecordCount)
}

// --- destructive: wipes every preload record on the tenant --------------

// TestAcceptance_Pro_InventoryPreload_DeleteAll covers both
// DeleteAllInventoryPreloadRecordsV1 and its unversioned alias. It removes every
// inventory-preload record on the tenant, which is why it is opt-in: on a tenant
// with real preload data this is not recoverable through the API.
func TestAcceptance_Pro_InventoryPreload_DeleteAll(t *testing.T) {
	requireWriteOptIn(t, "JAMFPLATFORM_ACC_PRELOAD_DELETE_ALL",
		"DeleteAllInventoryPreloadRecords{,V1} removes EVERY inventory-preload record on the tenant.")

	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Seed one record per variant so each delete-all has something to remove
	// and the post-condition is not vacuously true.
	for _, variant := range []string{"delv1", "delunv"} {
		if _, err := p.CreateInventoryPreloadRecordV1(ctx, &pro.InventoryPreloadRecord{
			SerialNumber: preloadLegacySerial(variant),
			DeviceType:   pro.InventoryPreloadRecordDeviceTypeComputer,
		}); err != nil {
			skipOnServerError(t, err)
			t.Fatalf("CreateInventoryPreloadRecordV1(%s): %v", variant, err)
		}
	}

	if err := p.DeleteAllInventoryPreloadRecordsV1(ctx); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteAllInventoryPreloadRecordsV1: %v", err)
	}
	remaining, err := p.ListInventoryPreloadRecordsV1(ctx, nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListInventoryPreloadRecordsV1 after delete-all: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("DeleteAllInventoryPreloadRecordsV1 left %d records", len(remaining))
	}

	// The unversioned alias must behave identically, including on an already
	// empty tenant.
	if err := p.DeleteAllInventoryPreloadRecords(ctx); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteAllInventoryPreloadRecords: %v", err)
	}
	remaining, err = p.ListInventoryPreloadRecords(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListInventoryPreloadRecords after delete-all: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("DeleteAllInventoryPreloadRecords left %d records", len(remaining))
	}
	t.Log("Both delete-all variants emptied the preload table")
}

// containsPreloadSerial reports whether records holds one with the given serial.
// Its signature is the assertion that matters: it only compiles if the list
// operations return []InventoryPreloadRecord rather than the search envelope.
func containsPreloadSerial(records []pro.InventoryPreloadRecord, serial string) bool {
	for _, r := range records {
		if strings.EqualFold(r.SerialNumber, serial) {
			return true
		}
	}
	return false
}
