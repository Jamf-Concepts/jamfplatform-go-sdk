// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/audit"
)

// The Audit API is read-only — all four operations are GETs, so there is nothing
// here to mutate.
//
// It is also unreachable by any external M2M credential today, and the cause is
// gateway configuration rather than anything in this SDK. The authz plugin
// resolves an organization from an em2m token only when the api-product declares
// exactly one request-context type and that type is "organization"; audit
// declares [environment, organization], so the fallback never fires and nothing
// resolves a context. Its allowed sources are [token, path] — no header — so
// X-Environment-Id is refused too. Full reasoning and the probe matrix are in
// CLAUDE.md.
//
// The tests below therefore pin the *limitation*, and are written to fail the day
// it lifts. When that happens, replace each with the real assertion rather than
// deleting it — a negative test that stops failing has done its job and its
// replacement must assert the capability at least as precisely.

// auditUnreachable reports whether err is the gateway refusing to resolve a
// request context, which is the documented current state of the audit product.
func auditUnreachable(t *testing.T, method string, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("%s: non-API error, the request did not reach the gateway: %v", method, err)
	}
	for _, d := range apiErr.Details() {
		if d.Code == "REQUEST_CONTEXT_NOT_PROVIDED" {
			t.Logf("%s: still blocked by the gateway's request-context config (%d %s)", method, apiErr.StatusCode, d.Code)
			return true
		}
	}
	if apiErr.HasStatus(403) {
		t.Logf("%s: context resolved but authorization denied (403) — the credential lacks read:org:audit / read:env:audit", method)
		return true
	}
	t.Fatalf("%s: unexpected failure, neither the known context block nor a 403: %v", method, err)
	return false
}

// TestAcceptance_AuditBlockedByGatewayContextConfig is the single test that
// asserts the block is still present. It fails if audit starts answering, which
// is the signal to write the real coverage below it.
func TestAcceptance_AuditBlockedByGatewayContextConfig(t *testing.T) {
	a := audit.New(accEnvClient(t))

	_, err := a.ListAuditSources(context.Background(), "")
	if err == nil {
		t.Fatal("ListAuditSources now succeeds — the gateway's audit request-context configuration has been fixed. " +
			"Replace this test and TestAcceptance_AuditReads' skips with real assertions, and re-check whether the " +
			"header form works or the path form is still required (see CLAUDE.md).")
	}
	if !auditUnreachable(t, "ListAuditSources", err) {
		t.Fatalf("ListAuditSources failed for an unexpected reason: %v", err)
	}
}

// TestAcceptance_AuditReads is the real coverage, held behind the block above. It
// exercises every operation and reports what each one says, so that the moment
// audit is reachable the suite already covers it — including the cursor-paginated
// walk, which is the part most likely to be wrong on first contact with real data.
func TestAcceptance_AuditReads(t *testing.T) {
	a := audit.New(accEnvClient(t))
	ctx := context.Background()

	// A wide but bounded window: `since` is the one required query param.
	since := "2026-08-01T00:00:00Z"

	sources, err := a.ListAuditSources(ctx, since)
	if err != nil {
		if auditUnreachable(t, "ListAuditSources", err) {
			t.Skip("Skipping the rest: audit is unreachable, see TestAcceptance_AuditBlockedByGatewayContextConfig")
		}
		t.Fatalf("ListAuditSources: %v", err)
	}
	t.Logf("%d audit sources", len(sources))
	for _, s := range sources {
		t.Logf("  %+v", s)
	}

	// ListAuditEvents walks nextCursor to exhaustion. On a busy environment that
	// is a lot of pages, so the window above is what bounds it — not a page cap,
	// which the cursor walker deliberately does not have.
	events, err := a.ListAuditEvents(ctx, since, "", "", nil, nil, nil)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	t.Logf("%d audit events since %s (cursor-paginated walk)", len(events), since)

	if len(events) == 0 {
		t.Log("no events in the window — nothing to drive the lineage and transaction probes from")
		return
	}

	// AuditEnvelope is a merged structural union: a gateway event carries Actor
	// and RequestContext, a service event carries Data, and the two never mix.
	// Asserting that is the only check that proves mergeOneOfVariants produced a
	// type that matches the wire rather than merely compiling.
	var gateway, service int
	for _, e := range events {
		isGateway := e.Actor != nil || e.RequestContext != nil
		isService := e.Data != nil
		if isGateway && isService {
			t.Errorf("event %s carries both actor/requestContext and data — the two variants are documented never to mix", e.AuditID)
		}
		switch {
		case isGateway:
			gateway++
		case isService:
			service++
		}
		if e.AuditID == "" || e.TxID == "" || e.AuditSource == "" {
			t.Errorf("event has an empty required base field: %+v", e)
		}
	}
	t.Logf("  %d gateway events, %d service events, %d neither", gateway, service, len(events)-gateway-service)

	first := events[0]
	timeline, err := a.GetTransactionTimeline(ctx, first.TxID)
	if err != nil {
		t.Errorf("GetTransactionTimeline(%s): %v", first.TxID, err)
	} else {
		t.Logf("transaction %s spans %d events", first.TxID, len(timeline))
	}

	if first.ResourceID != nil && *first.ResourceID != "" {
		lineage, err := a.GetResourceLineage(ctx, *first.ResourceID, "", since, "")
		if err != nil {
			t.Errorf("GetResourceLineage(%s): %v", *first.ResourceID, err)
		} else {
			t.Logf("resource %s has %d transactions in its lineage", *first.ResourceID, len(lineage))
		}
	} else {
		t.Log("first event has no resourceId — GetResourceLineage needs one, so it is uncovered on this data")
	}
}
