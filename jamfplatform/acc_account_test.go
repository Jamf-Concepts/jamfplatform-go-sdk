// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
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
		// LicenseType/ProductParent are enum aliases of string, so the fields are
		// *string once the single-member-allOf wrapper collapses; deref rather
		// than printing a pointer.
		t.Logf("  first: title=%q type=%q parent=%q seats=%v", l.Title, derefStr(l.LicenseType), derefStr(l.ProductParent), l.PurchasedSeats)
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
		// verifiedTldId is the second numeric-ID-declared-as-string field on this
		// struct. Logging it exercises the decode on every row, which is how the
		// first one was missed: it is null on most domains, so a small sample
		// never populates it.
		var tld string
		if d.VerifiedTldID != nil {
			tld = d.VerifiedTldID.String()
		}
		t.Logf("  %s id=%s verifiedTldId=%q status=%q shared=%v", d.Domain, domainID(d), tld, derefStr(d.DomainStatus), d.SharedDomain)
	}

	deals, err := ac.ListDealRegistrations(ctx)
	if err != nil {
		t.Fatalf("ListDealRegistrations: %v", err)
	}
	t.Logf("organization has %d deal registrations", len(deals))

	// The distributor surface is covered separately: it currently fails for an
	// upstream reason that has nothing to do with these three, and folding it in
	// here would mask them. See TestAcceptance_AccountDistributorReads.
}

// isSkywayScopeFault reports whether err is the known upstream fault behind every
// distributor endpoint: the Jamf Account partners backend calls Skyway with the
// scope `skyway-use2-product`, which exists only in the dev environment. Prod
// declares `skyway-use1-product` and the region-independent `skyway-product`
// (added by tyk-gateway-management e2f54c1c, EAI-4327) — so prod's auth server
// rejects the request and the 400 surfaces to us as an OAuth error on an API path.
//
// Matched on the scope name rather than the status code: a 400 from these
// endpoints could equally be a real validation verdict, which must not be
// swallowed.
func isSkywayScopeFault(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.HasStatus(400) && strings.Contains(apiErr.Body, "skyway-use2-product")
}

const skywayFaultReport = "the Jamf Account partners backend requests the Skyway scope %q, which exists only in dev — " +
	"prod declares skyway-use1-product and the region-independent skyway-product. " +
	"Report to Jamf: the account service needs repointing at skyway-product (tyk-gateway-management e2f54c1c, EAI-4327). " +
	"The SDK URL is confirmed correct; every non-distributor endpoint on the same credential returns 200"

// TestAcceptance_AccountDistributorReads is separated from the other account
// reads because the whole distributor surface is currently unreachable for a
// server-side reason. It fails rather than skips, so the suite reports the day it
// is fixed — see isSkywayScopeFault for the diagnosis.
func TestAcceptance_AccountDistributorReads(t *testing.T) {
	ac := account.New(accOrgClient(t))
	ctx := context.Background()

	cfg, err := ac.GetDistributorConfiguration(ctx)
	switch {
	case isSkywayScopeFault(err):
		t.Fatalf("GetDistributorConfiguration: "+skywayFaultReport, "skyway-use2-product")
	case err != nil:
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
			t.Skip("Skipping: this organization is not a distributor, so it has no configuration")
		}
		t.Fatalf("GetDistributorConfiguration: %v", err)
	default:
		t.Logf("distributor configuration: poSubmissionPermission=%v", cfg.PoSubmissionPermission)
	}

	// GetDistributorQuote and GetDistributorPurchaseOrder need identifiers only a
	// real order can supply, so they are probed with values that cannot exist.
	// A 404 is the pass here: it proves the endpoint is routed and looking the
	// identifier up rather than refusing the request outright.
	if _, err := ac.GetDistributorQuote(ctx, "SDK-ACC-NO-SUCH-QUOTE"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		switch {
		case isSkywayScopeFault(err):
			t.Errorf("GetDistributorQuote: "+skywayFaultReport, "skyway-use2-product")
		case errors.As(err, &apiErr) && apiErr.HasStatus(404):
			t.Log("GetDistributorQuote correctly 404s for a quote that does not exist")
		default:
			t.Errorf("GetDistributorQuote: unexpected failure: %v", err)
		}
	} else {
		t.Error("GetDistributorQuote returned a quote for SDK-ACC-NO-SUCH-QUOTE")
	}

	if _, err := ac.GetDistributorPurchaseOrder(ctx, "SDK-ACC-NO-SUCH-PO"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		switch {
		case isSkywayScopeFault(err):
			t.Errorf("GetDistributorPurchaseOrder: "+skywayFaultReport, "skyway-use2-product")
		case errors.As(err, &apiErr) && apiErr.HasStatus(404):
			t.Log("GetDistributorPurchaseOrder correctly 404s for a PO that does not exist")
		default:
			t.Errorf("GetDistributorPurchaseOrder: unexpected failure: %v", err)
		}
	} else {
		t.Error("GetDistributorPurchaseOrder returned an order for SDK-ACC-NO-SUCH-PO")
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
	t.Logf("created domain %s id=%s status=%q", created.Domain, id, derefStr(created.DomainStatus))

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
		t.Errorf("VerifyDomain succeeded for an unresolvable .invalid domain (status=%q) — verification is not being enforced", derefStr(verified.DomainStatus))
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
		// Skipping rather than failing only because the PATCH is genuinely
		// unreachable: there is nothing to write back when the read cannot
		// happen. TestAcceptance_AccountDistributorReads is what reports the
		// fault, so it is not being hidden.
		if isSkywayScopeFault(err) {
			t.Skip("Skipping: the distributor surface is blocked upstream, so there is no configuration to round-trip. See TestAcceptance_AccountDistributorReads")
		}
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
	case isSkywayScopeFault(err):
		t.Fatalf("ValidateDistributorPurchaseOrder: "+skywayFaultReport, "skyway-use2-product")
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
	if isSkywayScopeFault(err) {
		t.Fatalf("CreateDistributorPurchaseOrder: "+skywayFaultReport, "skyway-use2-product")
	}
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
