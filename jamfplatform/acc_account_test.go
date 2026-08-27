// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/account"
)

// The Jamf Account APIs are organization-scoped: no scope header is sent and the
// gateway derives the organization from the token. They are also served only from
// the US gateway — see initOrgAcceptanceClient.

func TestAcceptance_AccountReads(t *testing.T) {
	ac := account.New(accOrgClient(t))
	ctx := context.Background()

	licences, err := ac.ListLicenses(ctx)
	if err != nil {
		t.Fatalf("ListLicenses: %v", err)
	}
	t.Logf("organization holds %d licences", len(licences))
	if len(licences) > 0 {
		l := licences[0]
		t.Logf("  first: title=%q type=%q parent=%q seats=%v", l.Title, l.LicenseType, l.ProductParent, l.PurchasedSeats)
	}

	domains, err := ac.ListDomains(ctx)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	t.Logf("organization has %d domains", len(domains))
	for _, d := range domains {
		// Domain.ID is json.Number: the spec declares it a string and the server
		// sends a bare number. Asserting it converts is the point — a consumer
		// has to pass it back as a string to DeleteDomain/VerifyDomain.
		if domainID(d) == "" {
			t.Errorf("domain %q has an empty ID", d.Domain)
		}
		t.Logf("  %s id=%s status=%v shared=%v", d.Domain, domainID(d), d.DomainStatus, d.SharedDomain)
	}

	deals, err := ac.ListDealRegistrations(ctx)
	if err != nil {
		t.Fatalf("ListDealRegistrations: %v", err)
	}
	t.Logf("organization has %d deal registrations", len(deals))

	cfg, err := ac.GetDistributorConfiguration(ctx)
	if err != nil {
		// A non-distributor organization legitimately has no configuration.
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
			t.Log("GetDistributorConfiguration: 404 — this organization is not a distributor")
		} else {
			t.Fatalf("GetDistributorConfiguration: %v", err)
		}
	} else {
		t.Logf("distributor configuration: poSubmissionPermission=%v", cfg.PoSubmissionPermission)
	}
}

// TestAcceptance_AccountListConnections is separated from the other reads because
// the endpoint currently answers 502 from an upstream service. It is a real fault
// rather than a shape problem — every sibling endpoint on the same credential
// returns 200 — and it fails here rather than skipping so the suite reports the
// day it is fixed. Note the 502 is retryable, so this test spends the client's
// full retry budget before returning.
func TestAcceptance_AccountListConnections(t *testing.T) {
	ac := account.New(accOrgClient(t))

	conns, err := ac.ListConnections(context.Background())
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(502) {
			t.Fatalf("ListConnections still returns 502 from upstream — report to Jamf, the URL is confirmed correct: %v", err)
		}
		t.Fatalf("ListConnections: %v", err)
	}
	t.Logf("organization has %d SSO connections", len(conns))
}

// TestAcceptance_AccountDomainLifecycle exercises the only account resource with
// a complete create/read/delete surface.
//
// The domain is under `.invalid`, which RFC 2606 reserves and guarantees can
// never be registered or resolved. That matters twice: nobody can own it, so the
// claim cannot collide with a real customer's domain, and it can never satisfy
// DNS verification, so a claim left behind by a failed cleanup is inert.
func TestAcceptance_AccountDomainLifecycle(t *testing.T) {
	requireWriteOptIn(t, "JAMFPLATFORM_ORG_WRITE_OK",
		"Creating a domain claims it for this organization — harmless for a .invalid domain, but it writes to a real customer record.")

	ac := account.New(accOrgClient(t))
	ctx := context.Background()

	name := fmt.Sprintf("sdk-acc-%d.invalid", rand.IntN(1_000_000))
	created, err := ac.CreateDomain(ctx, &account.DomainRequest{Domain: name})
	if err != nil {
		t.Fatalf("CreateDomain(%s): %v", name, err)
	}
	id := domainID(*created)
	if id == "" {
		t.Fatal("CreateDomain returned an empty ID")
	}
	cleanupDelete(t, "domain "+name, func() error { return ac.DeleteDomain(ctx, id) })
	t.Logf("created domain %s id=%s status=%v", created.Domain, id, created.DomainStatus)

	if created.VerificationKey == "" {
		t.Error("CreateDomain returned no verificationKey — a caller has nothing to publish as a TXT record")
	}

	// Round-trip through the list, which is the only way to confirm the claim
	// actually landed: there is no GET /v1/domains/{id}.
	domains, err := ac.ListDomains(ctx)
	if err != nil {
		t.Fatalf("ListDomains after create: %v", err)
	}
	var found bool
	for _, d := range domains {
		if domainID(d) == id {
			found = true
			if d.Domain != name {
				t.Errorf("round-trip domain = %q, want %q", d.Domain, name)
			}
		}
	}
	if !found {
		t.Errorf("created domain id=%s absent from ListDomains", id)
	}

	alloc, err := ac.GetDomainAllocation(ctx, name)
	if err != nil {
		t.Logf("GetDomainAllocation(%s): %v", name, err)
	} else {
		t.Logf("allocation for %s: %+v", name, *alloc)
	}

	// VerifyDomain must fail: .invalid can never carry the TXT record. Pinning
	// the failure is what proves verification is really checked server-side
	// rather than being a no-op that returns the domain unchanged.
	verified, err := ac.VerifyDomain(ctx, id)
	if err == nil {
		t.Errorf("VerifyDomain succeeded for an unresolvable .invalid domain (status=%v) — verification is not being enforced", verified.DomainStatus)
	} else {
		t.Logf("VerifyDomain correctly rejected %s: %v", name, err)
	}
}

// TestAcceptance_AccountDistributorConfigRoundTrip exercises the PATCH by writing
// the configuration back exactly as read. That is the only safe way to cover it:
// the resource is a singleton holding a real distributor's settings, there is no
// create or delete, and PATCH is documented as a full replacement — so any test
// that changed a value would have to restore it from the same read it is trying
// to verify, and would corrupt the record if it failed in between.
func TestAcceptance_AccountDistributorConfigRoundTrip(t *testing.T) {
	requireWriteOptIn(t, "JAMFPLATFORM_ORG_WRITE_OK",
		"PATCHes a real distributor's configuration — written back unchanged, but it is a live customer record.")

	ac := account.New(accOrgClient(t))
	ctx := context.Background()

	before, err := ac.GetDistributorConfiguration(ctx)
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
			t.Skip("Skipping: this organization is not a distributor, so there is no configuration to round-trip")
		}
		t.Fatalf("GetDistributorConfiguration: %v", err)
	}

	if err := ac.UpdateDistributorConfiguration(ctx, before); err != nil {
		t.Fatalf("UpdateDistributorConfiguration (unchanged write-back): %v", err)
	}

	after, err := ac.GetDistributorConfiguration(ctx)
	if err != nil {
		t.Fatalf("GetDistributorConfiguration after PATCH: %v", err)
	}
	if fmt.Sprintf("%+v", *after) != fmt.Sprintf("%+v", *before) {
		t.Errorf("configuration changed across an unchanged write-back:\n before: %+v\n after:  %+v", *before, *after)
	}
}

// TestAcceptance_AccountPurchaseOrderValidation covers both distributor POSTs
// without ever creating an order.
//
// CreateDistributorPurchaseOrder has no delete and writes a real business record
// against a Salesforce-backed organization, so a successful create is not
// something an acceptance run may leave behind. It is exercised through its
// validation, using the ordering trick documented in CLAUDE.md: field validation
// runs before the record is written, so a request that is guaranteed to be
// rejected still proves the payload reaches the endpoint and is understood.
//
// ValidateDistributorPurchaseOrder is the endpoint's own dry run — a POST that
// deliberately changes nothing — so it is called with the same body.
func TestAcceptance_AccountPurchaseOrderValidation(t *testing.T) {
	ac := account.New(accOrgClient(t))
	ctx := context.Background()

	// Deliberately references a quote that cannot exist.
	quote := "SDK-ACC-NO-SUCH-QUOTE"
	po := fmt.Sprintf("SDK-ACC-%d", rand.IntN(1_000_000))
	currency := "USD"
	order := &account.DistributorPurchaseOrder{
		QuoteNumber:  &quote,
		PoNumber:     &po,
		CurrencyCode: &currency,
	}

	result, err := ac.ValidateDistributorPurchaseOrder(ctx, order)
	switch {
	case err == nil:
		t.Logf("ValidateDistributorPurchaseOrder returned a result for a bogus quote: %+v", *result)
	default:
		var apiErr *jamfplatform.APIResponseError
		if !errors.As(err, &apiErr) {
			t.Fatalf("ValidateDistributorPurchaseOrder: non-API error, the request did not reach the endpoint: %v", err)
		}
		if apiErr.StatusCode >= 500 {
			t.Fatalf("ValidateDistributorPurchaseOrder: server error, not a validation verdict: %v", err)
		}
		t.Logf("ValidateDistributorPurchaseOrder rejected the bogus order (%d): %s", apiErr.StatusCode, apiErr.Summary())
		for field, msgs := range apiErr.FieldErrors() {
			t.Logf("  field %q: %v", field, msgs)
		}
	}

	// The create must NOT succeed. If it ever does, this organization just grew
	// an undeletable purchase order and the test needs redesigning, not relaxing.
	err = ac.CreateDistributorPurchaseOrder(ctx, order)
	if err == nil {
		t.Fatalf("CreateDistributorPurchaseOrder ACCEPTED an order referencing quote %q — an unremovable record may have been created; investigate before re-running", quote)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreateDistributorPurchaseOrder: non-API error, the request did not reach the endpoint: %v", err)
	}
	if apiErr.StatusCode >= 500 {
		t.Fatalf("CreateDistributorPurchaseOrder: server error rather than a validation rejection: %v", err)
	}
	t.Logf("CreateDistributorPurchaseOrder correctly rejected the bogus order (%d): %s", apiErr.StatusCode, apiErr.Summary())
}

// TestAcceptance_AccountSsoConnectionWrites is deliberately not implemented.
//
// CreateConnection/UpdateConnection/DeleteConnection configure the identity
// provider a real organization's users authenticate through: a bad or abandoned
// connection can lock people out of Jamf Account, and the blast radius is the
// whole organization rather than one record. Two things would have to be true
// before this is safe, and neither is today: ListConnections has to work (it
// answers 502, so a leaked connection cannot even be found to clean up), and the
// organization has to be one reserved for the suite rather than a live UAT tenant.
//
// The request shape is still partly covered — CreateConnection with an invalid
// body is exercised below, which reaches validation without creating anything.
func TestAcceptance_AccountSsoConnectionWrites(t *testing.T) {
	t.Skip("SSO connection writes reconfigure how a real organization's users log in, and ListConnections currently 502s so a leaked connection could not be found to delete. Needs an organization reserved for the suite.")
}

func TestAcceptance_AccountCreateConnectionRejected(t *testing.T) {
	ac := account.New(accOrgClient(t))

	// Every required member omitted: this cannot create anything, but it proves
	// the endpoint is reachable and validating rather than silently accepting.
	_, err := ac.CreateConnection(context.Background(), &account.ConnectionRequest{})
	if err == nil {
		t.Fatal("CreateConnection accepted an empty request — an SSO connection may have been created; investigate immediately")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreateConnection: non-API error, the request did not reach the endpoint: %v", err)
	}
	if apiErr.StatusCode >= 500 {
		t.Skipf("Skipping: CreateConnection returns %d, the same upstream fault ListConnections hits: %v", apiErr.StatusCode, err)
	}
	t.Logf("CreateConnection correctly rejected an empty request (%d): %s", apiErr.StatusCode, apiErr.Summary())
	for field, msgs := range apiErr.FieldErrors() {
		t.Logf("  field %q: %v", field, msgs)
	}
}

// domainID renders Domain.ID as the opaque string the delete and verify path
// params take. It is *json.Number because the spec declares the field a string
// while the server sends a bare number — see the fieldTypeOverride in
// tools/generate/config.json — and optional, so nil is possible on the wire.
func domainID(d account.Domain) string {
	if d.ID == nil {
		return ""
	}
	return d.ID.String()
}
