// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Jamf Security Cloud acceptance coverage.
//
// Every test here runs against the Security Cloud credential set
// (JAMFPLATFORM_JSC_*), never the Jamf Pro one — the two products have separate
// tenants and separate API clients, and neither credential reaches the other's
// surface (probed 2026-08-17: 403 BAD_PERMISSIONS in both directions).
//
// Two groups of endpoints are deliberately not exercised, each for a reason
// named at the test that skips it: paths the gateway does not route yet, and
// writes that would provision or reconfigure real infrastructure on a shared
// tenant.
//
// Activation profiles are absent from this suite because they are absent from
// the SDK: their spec is not published in any environment of the GitOps build,
// and the SDK only ingests published specs.

// jscName namespaces every artefact this suite creates, so a leftover from a
// crashed run is identifiable and safe to remove by hand.
func jscName(kind string) string {
	return "sdk-acc-" + kind + "-" + runSuffix()
}

// jscCleanupDelete registers a safety-net delete that stays quiet when the test
// already deleted the resource on its happy path. cleanupDelete would log the
// resulting 404 as a cleanup failure on every passing run, training the reader
// to ignore exactly the line that reports a real leak.
func jscCleanupDelete(t *testing.T, label string, fn func() error) {
	t.Helper()
	cleanupDelete(t, label, func() error {
		if err := fn(); err != nil && !isSecurityCloudNotFound(err) {
			return err
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// DNS — zones
// ---------------------------------------------------------------------------

// assertCreateHrefEmpty pins a server bug, not a capability: every Security
// Cloud create returns `href` (and a Location header) when the response is
// uncompressed, and returns `"href": null` with no Location header when it is
// gzipped. Wire-verified 2026-08-24, 12/12 deterministic either way — content
// encoding is mutating the response body.
//
// Go's net/http adds `Accept-Encoding: gzip` to every request and transparently
// decompresses, so **no Go caller can ever see href**. That is why the
// long-standing note "the server sends only id, never href" looked true: it was
// only ever observed through this SDK.
//
// So the ID is the only usable member, and this asserts that emptiness on
// purpose. It will fail the day the gateway stops varying the body by encoding,
// which is the signal to flip it into a real href assertion and to drop the
// caveat from CLAUDE.md.
func assertCreateHrefEmpty(t *testing.T, href, what string) {
	t.Helper()
	if href != "" {
		t.Errorf("%s href = %q, want empty: Go always sends Accept-Encoding: gzip and the gateway nulls href on the compressed path. A value here means that bug is fixed — assert the canonical path instead and update CLAUDE.md.", what, href)
	}
}

// jscEnsureGateways returns at least n dedicated ZTNA gateways, creating any
// shortfall itself. Four tests need a real gateway ID and no synthetic value
// works: DNS zones must point their name servers at one, and a grouped gateway
// needs two dedicated members (shared ones are refused with 422
// SHARED_GATEWAY_MEMBER). Those tests used to skip whenever the tenant happened
// to have none, which meant their create paths went unexercised on exactly the
// clean tenant a CI run starts from.
//
// Gateways it creates are `enabled: false` with `dedicatedIps` and no `ipsec`,
// which is the cheapest form the server accepts — no subnets, vendor or PSK —
// and they carry no traffic. Each is registered for deletion via
// jscCleanupDelete, so a run leaves the tenant as it found it.
//
// Creating one still provisions real infrastructure, so it stays behind
// JAMFPLATFORM_JSC_GATEWAY_WRITE_OK. The difference from a bare skip is that on
// a tenant reserved for this suite nothing skips, and on a shared tenant the
// skip names the variable that would fix it rather than an absent fixture the
// reader cannot act on. Pre-existing gateways are used as-is and never deleted.
func jscEnsureGateways(t *testing.T, sc *securitycloud.Client, n int) []securitycloud.Gateway {
	t.Helper()
	ctx := context.Background()

	page, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
	existing := page.Results
	if len(existing) >= n {
		return existing
	}
	if os.Getenv("JAMFPLATFORM_JSC_GATEWAY_WRITE_OK") == "" {
		t.Skipf("need %d dedicated ZTNA gateways, tenant has %d, and creating one provisions real network egress — set JAMFPLATFORM_JSC_GATEWAY_WRITE_OK to let this test create and delete its own on a tenant reserved for it", n, len(existing))
	}

	enabled := false
	for len(existing) < n {
		created, err := sc.CreateZtnaGatewayV1(ctx, &securitycloud.GatewayCreateRequest{
			Name:       jscName("fixture-gateway"),
			Datacenter: securitycloud.GatewayCreateRequestDatacenterEuWest2,
			Enabled:    &enabled,
			TenantIds:  []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
			Contact: securitycloud.GatewayContact{
				Email: "sdk-acc@example.invalid",
				Name:  "SDK acceptance fixture",
			},
			DedicatedIps: &securitycloud.DedicatedIps{Enabled: true},
		})
		if err != nil {
			t.Fatalf("creating fixture gateway %d/%d failed: %v", len(existing)+1, n, err)
		}
		id := created.ID
		jscCleanupDelete(t, "fixture gateway "+id, func() error {
			return sc.DeleteZtnaGatewayV1(context.Background(), id)
		})
		// Create answers {id, href} (see the lifecycle test), so the full
		// Gateway has to be read back — callers here want its Name as well as
		// its ID, and the resolver test matches on Name.
		full, err := sc.GetZtnaGatewayV1(ctx, id)
		if err != nil {
			t.Fatalf("GetZtnaGatewayV1(%s) after creating a fixture gateway failed: %v", id, err)
		}
		existing = append(existing, *full)
	}
	return existing
}

func TestAcceptance_SecurityCloudDnsZoneLifecycle(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	// A zone's name servers must point at a real gateway ID; there is no
	// synthetic value the server accepts.
	gateways := jscEnsureGateways(t, sc, 1)

	created, err := sc.CreateDnsZoneV1(ctx, &securitycloud.ZoneWrite{
		Name:        jscName("zone"),
		Domains:     []string{"sdk-acc.invalid"},
		NameServers: []securitycloud.NameServer{{IP: "203.0.113.53", GatewayID: gateways[0].ID}},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDnsZoneV1 failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateDnsZoneV1 returned an empty ID")
	}
	assertCreateHrefEmpty(t, created.Href, "dns zone")
	jscCleanupDelete(t, "dns zone "+created.ID, func() error {
		return sc.DeleteDnsZoneV1(context.Background(), created.ID)
	})
	// Wire-verified 2026-08-17: the 201 body's href is null even though the
	// spec marks it required, and the Location header carries a bare ID rather
	// than the documented canonical URL. Only ID is usable.
	t.Logf("created zone id=%s href=%q", created.ID, created.Href)

	got, err := sc.GetDnsZoneV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDnsZoneV1(%s) failed: %v", created.ID, err)
	}
	if got.Name != jscName("zone") {
		t.Errorf("zone name = %q, want %q", got.Name, jscName("zone"))
	}

	renamed := jscName("zone") + "-renamed"
	if err := sc.UpdateDnsZoneV1(ctx, created.ID, &securitycloud.ZonePatch{Name: &renamed}); err != nil {
		t.Fatalf("UpdateDnsZoneV1(%s) failed: %v", created.ID, err)
	}
	got, err = sc.GetDnsZoneV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDnsZoneV1 after patch failed: %v", err)
	}
	if got.Name != renamed {
		t.Errorf("after merge-patch, name = %q, want %q", got.Name, renamed)
	}
	if len(got.Domains) != 1 || got.Domains[0] != "sdk-acc.invalid" {
		t.Errorf("merge-patch of name should leave domains untouched, got %v", got.Domains)
	}

	zones, err := sc.ListDnsZonesV1(ctx, "name:asc")
	if err != nil {
		t.Fatalf("ListDnsZonesV1 failed: %v", err)
	}
	t.Logf("tenant has %d DNS zones (totalCount=%d)", len(zones.Results), zones.TotalCount)
	var found bool
	for _, z := range zones.Results {
		if z.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("created zone %s missing from ListDnsZonesV1", created.ID)
	}

	if err := sc.DeleteDnsZoneV1(ctx, created.ID); err != nil {
		t.Fatalf("DeleteDnsZoneV1(%s) failed: %v", created.ID, err)
	}
	_, err = sc.GetDnsZoneV1(ctx, created.ID)
	if err == nil {
		t.Fatal("GetDnsZoneV1 succeeded after delete")
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(http.StatusNotFound) {
		t.Errorf("after delete, want 404 APIResponseError, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DNS — search domain singleton
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudSearchDomain(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	// The search domain is a tenant-wide singleton that affects name
	// resolution for enrolled devices, so the original value is captured and
	// restored rather than left at the probe value. An unset domain answers
	// 404 SEARCH_DOMAIN_NOT_SET, which is state, not failure.
	original, err := sc.GetDnsSearchDomainV1(ctx)
	switch {
	case err == nil:
		t.Logf("tenant search domain is currently %q — it will be restored", original.Suffix)
		defer func() {
			if err := sc.SetDnsSearchDomainV1(context.Background(), original); err != nil {
				t.Errorf("failed to restore the original search domain %q: %v", original.Suffix, err)
			}
		}()
	case isSecurityCloudNotFound(err):
		t.Log("tenant has no search domain configured — it will be cleared again after the test")
		defer func() {
			if err := sc.ClearDnsSearchDomainV1(context.Background()); err != nil {
				t.Errorf("failed to restore the unset search domain: %v", err)
			}
		}()
	default:
		skipOnServerError(t, err)
		t.Fatalf("GetDnsSearchDomainV1 failed: %v", err)
	}

	probe := "sdk-acc.invalid"
	if err := sc.SetDnsSearchDomainV1(ctx, &securitycloud.SearchDomain{Suffix: probe}); err != nil {
		t.Fatalf("SetDnsSearchDomainV1 failed: %v", err)
	}
	got, err := sc.GetDnsSearchDomainV1(ctx)
	if err != nil {
		t.Fatalf("GetDnsSearchDomainV1 after set failed: %v", err)
	}
	if got.Suffix != probe {
		t.Errorf("search domain = %q, want %q", got.Suffix, probe)
	}

	if err := sc.ClearDnsSearchDomainV1(ctx); err != nil {
		t.Fatalf("ClearDnsSearchDomainV1 failed: %v", err)
	}
	if _, err := sc.GetDnsSearchDomainV1(ctx); !isSecurityCloudNotFound(err) {
		t.Errorf("after clear, want 404 SEARCH_DOMAIN_NOT_SET, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DNS — custom hostname mappings
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudCustomHostnameMappings(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	// PUT replaces the whole list, so the tenant's existing mappings are
	// captured and written back on the way out.
	original, err := sc.GetDnsCustomHostnameMappingsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetDnsCustomHostnameMappingsV1 failed: %v", err)
	}
	t.Logf("tenant has %d custom hostname mappings", original.TotalCount)
	defer func() {
		restore := original.Results
		if len(restore) == 0 {
			// Clearing is the exact restore of an empty list, and is the only
			// safe way to exercise ClearDnsCustomHostnameMappingsV1 — on a
			// tenant that owns real mappings it would delete them.
			if err := sc.ClearDnsCustomHostnameMappingsV1(context.Background()); err != nil {
				t.Errorf("failed to clear the probe mapping: %v", err)
			}
			return
		}
		if err := sc.ReplaceDnsCustomHostnameMappingsV1(context.Background(), &restore); err != nil {
			t.Errorf("failed to restore the tenant's %d original mappings: %v", len(restore), err)
		}
	}()

	probe := append(append([]securitycloud.Mapping{}, original.Results...), securitycloud.Mapping{
		Hostname: "sdk-acc.invalid",
		ARecords: &[]string{"203.0.113.10"},
	})
	if err := sc.ReplaceDnsCustomHostnameMappingsV1(ctx, &probe); err != nil {
		t.Fatalf("ReplaceDnsCustomHostnameMappingsV1 failed: %v", err)
	}

	got, err := sc.GetDnsCustomHostnameMappingsV1(ctx)
	if err != nil {
		t.Fatalf("GetDnsCustomHostnameMappingsV1 after replace failed: %v", err)
	}
	var found bool
	for _, m := range got.Results {
		if m.Hostname == "sdk-acc.invalid" {
			found = true
			var aRecords []string
			if m.ARecords != nil {
				aRecords = *m.ARecords
			}
			t.Logf("probe mapping round-tripped: aRecords=%v secureDns=%v ztna=%v",
				aRecords, boolValue(m.SecureDns), boolValue(m.Ztna))
		}
	}
	if !found {
		t.Error("probe mapping missing after ReplaceDnsCustomHostnameMappingsV1")
	}
}

// ---------------------------------------------------------------------------
// ZTNA — reads
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudZtnaReads(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	gatewayPage, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
	gateways := gatewayPage.Results
	t.Logf("%d ZTNA gateways", len(gateways))
	if len(gateways) > 0 {
		g, err := sc.GetZtnaGatewayV1(ctx, gateways[0].ID)
		if err != nil {
			t.Fatalf("GetZtnaGatewayV1(%s) failed: %v", gateways[0].ID, err)
		}
		t.Logf("gateway %s: name=%q datacenter=%q enabled=%v", g.ID, g.Name, g.Datacenter, g.Enabled)
	}

	shared, err := sc.ListZtnaSharedGatewaysV1(ctx)
	if err != nil {
		t.Fatalf("ListZtnaSharedGatewaysV1 failed: %v", err)
	}
	t.Logf("%d shared gateways (totalCount=%d)", len(shared.Results), shared.TotalCount)

	grouped, err := sc.ListZtnaGroupedGatewaysV1(ctx)
	if err != nil {
		t.Fatalf("ListZtnaGroupedGatewaysV1 failed: %v", err)
	}
	t.Logf("%d grouped gateways", len(grouped.Results))
	if len(grouped.Results) > 0 {
		gg, err := sc.GetZtnaGroupedGatewayV1(ctx, grouped.Results[0].ID)
		if err != nil {
			t.Fatalf("GetZtnaGroupedGatewayV1(%s) failed: %v", grouped.Results[0].ID, err)
		}
		t.Logf("grouped gateway %s: strategy=%s members=%v", gg.ID, gg.RoutingStrategy, gg.GatewayIds)
	}

	apps, err := sc.ListZtnaAppsV1(ctx)
	if err != nil {
		t.Fatalf("ListZtnaAppsV1 failed: %v", err)
	}
	t.Logf("%d ZTNA apps", len(apps))
	if len(apps) > 0 {
		a, err := sc.GetZtnaAppV1(ctx, apps[0].ID)
		if err != nil {
			t.Fatalf("GetZtnaAppV1(%s) failed: %v", apps[0].ID, err)
		}
		t.Logf("app %s: name=%v category=%q hostnames=%v", a.ID, a.Name, a.CategoryName, a.Hostnames)
	}

	predefined, err := sc.ListZtnaPredefinedAppsV1(ctx)
	if err != nil {
		t.Fatalf("ListZtnaPredefinedAppsV1 failed: %v", err)
	}
	t.Logf("%d predefined app templates", len(predefined.Results))
}

// ---------------------------------------------------------------------------
// ZTNA — access policy (app) lifecycle
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudZtnaAppLifecycle(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	// categoryName is validated against the *displayName* of the tenant's own
	// category list, not a fixed enum, so it has to be read first.
	cats, err := sc.ListContentCategoriesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListContentCategoriesV1 failed: %v", err)
	}
	if len(cats.Results) == 0 {
		t.Skip("tenant exposes no content categories — an app needs a categoryName")
	}
	// Prefer the neutral catch-all when the tenant has it; the categories are a
	// content-filtering taxonomy and the first entry alphabetically is "Adult".
	category := cats.Results[0].DisplayName
	for _, c := range cats.Results {
		if c.DisplayName == "Uncategorized" {
			category = c.DisplayName
			break
		}
	}

	name := jscName("app")
	// routing.type DIRECT keeps the policy off any gateway, so creating it
	// changes no network path.
	created, err := sc.CreateZtnaAppV1(ctx, &securitycloud.AppCreateRequest{
		Name:         &name,
		CategoryName: category,
		Hostnames:    &[]string{"sdk-acc-app.invalid"},
		Assignments:  securitycloud.Assignments{Inclusions: securitycloud.AssignmentsInclusions{AllUsers: true}},
		Routing:      securitycloud.Routing{Type: "DIRECT"},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateZtnaAppV1 failed: %v", err)
	}
	jscCleanupDelete(t, "ztna app "+created.ID, func() error {
		return sc.DeleteZtnaAppV1(context.Background(), created.ID)
	})
	// 201 with the spec-declared {id, href}. The server used to answer with the
	// whole App, which config.json encoded as a responseType override; build
	// v1439 brought it in line with the spec and the override is gone.
	if created.ID == "" {
		t.Fatal("CreateZtnaAppV1 returned no ID")
	}
	assertCreateHrefEmpty(t, created.Href, "ztna app")
	t.Logf("created app %s", created.ID)

	renamed := name + "-renamed"
	if err := sc.UpdateZtnaAppV1(ctx, created.ID, &securitycloud.AppPatchRequest{Name: &renamed}); err != nil {
		t.Fatalf("UpdateZtnaAppV1(%s) failed: %v", created.ID, err)
	}
	got, err := sc.GetZtnaAppV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaAppV1 after patch failed: %v", err)
	}
	// App.name became *string in build v1582: the spec now declares it nullable
	// (and required), matching the wire — a predefined-template app returns
	// name:null. A custom app like this one must still carry a real name, so a
	// nil here is a regression, not the nullable case.
	if got.Name == nil {
		t.Fatal("app name is nil after a name patch; only predefined-template apps return name null")
	}
	if *got.Name != renamed {
		t.Errorf("app name = %q, want %q", *got.Name, renamed)
	}
	if len(got.Hostnames) != 1 || got.Hostnames[0] != "sdk-acc-app.invalid" {
		t.Errorf("merge-patch of name should leave hostnames untouched, got %v", got.Hostnames)
	}

	if err := sc.DeleteZtnaAppV1(ctx, created.ID); err != nil {
		t.Fatalf("DeleteZtnaAppV1(%s) failed: %v", created.ID, err)
	}
	if _, err := sc.GetZtnaAppV1(ctx, created.ID); !isSecurityCloudNotFound(err) {
		t.Errorf("after delete, want 404, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ZTNA — grouped gateway lifecycle
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudZtnaGroupedGatewayLifecycle(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	gateways := jscEnsureGateways(t, sc, 2)

	name := jscName("grouped")
	created, err := sc.CreateZtnaGroupedGatewayV1(ctx, &securitycloud.GroupedGatewayCreateRequest{
		Name:            name,
		GatewayIds:      []string{gateways[0].ID, gateways[1].ID},
		TenantIds:       []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		RoutingStrategy: "NEAREST",
		// Required on create even for NEAREST, where the server ignores it
		// (spec v1401, wire-confirmed). Leaving the Go zero value in place
		// earns a 400 — see the RecoveryDelay validation test below.
		RecoveryDelayInSec: 3600,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateZtnaGroupedGatewayV1 failed: %v", err)
	}
	jscCleanupDelete(t, "grouped gateway "+created.ID, func() error {
		return sc.DeleteZtnaGroupedGatewayV1(context.Background(), created.ID)
	})
	if created.ID == "" {
		t.Fatal("CreateZtnaGroupedGatewayV1 returned no ID")
	}
	assertCreateHrefEmpty(t, created.Href, "ztna grouped gateway")

	renamed := name + "-renamed"
	if err := sc.UpdateZtnaGroupedGatewayV1(ctx, created.ID, &securitycloud.GroupedGatewayPatchRequest{Name: &renamed}); err != nil {
		t.Fatalf("UpdateZtnaGroupedGatewayV1(%s) failed: %v", created.ID, err)
	}
	got, err := sc.GetZtnaGroupedGatewayV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaGroupedGatewayV1 after patch failed: %v", err)
	}
	if got.Name != renamed {
		t.Errorf("grouped gateway name = %q, want %q", got.Name, renamed)
	}
	if len(got.GatewayIds) != 2 {
		t.Errorf("merge-patch of name should leave gatewayIds untouched, got %v", got.GatewayIds)
	}

	if err := sc.DeleteZtnaGroupedGatewayV1(ctx, created.ID); err != nil {
		t.Fatalf("DeleteZtnaGroupedGatewayV1(%s) failed: %v", created.ID, err)
	}
}

// TestAcceptance_SecurityCloudZtnaGroupedGatewayRecoveryDelay pins the
// recoveryDelayInSec constraint the ztna spec gained in GitOps build v1401 and
// that was wire-confirmed on 2026-08-21: the field is required on create for
// every routing strategy, and its value must be one of five discrete durations.
//
// It runs unconditionally because it cannot provision anything. gatewayIds are
// deliberately *shared* gateways, which the server refuses with 422
// SHARED_GATEWAY_MEMBER — so the valid-value case proves the request cleared
// field validation without ever creating a grouped gateway. Field validation
// runs before that business rule, which is what makes the whole matrix
// reachable from an empty tenant.
func TestAcceptance_SecurityCloudZtnaGroupedGatewayRecoveryDelay(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	shared, err := sc.ListZtnaSharedGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaSharedGatewaysV1 failed: %v", err)
	}
	if len(shared.Results) < 2 {
		t.Skipf("need 2 shared gateways to build a rejectable member list, tenant has %d", len(shared.Results))
	}
	members := []string{shared.Results[0].ID, shared.Results[1].ID}

	req := func(delay int) *securitycloud.GroupedGatewayCreateRequest {
		return &securitycloud.GroupedGatewayCreateRequest{
			Name:               jscName("grouped-delay"),
			GatewayIds:         members,
			TenantIds:          []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
			RoutingStrategy:    "ACTIVE_STANDBY",
			RecoveryDelayInSec: delay,
		}
	}

	// The Go zero value is not a legal duration. Before v1401 the spec declared
	// `default: 0` and the field was optional, so a caller who left it unset got
	// a working grouped gateway; now they get a 400. Asserted explicitly because
	// that is the whole breaking change.
	t.Run("zero value rejected", func(t *testing.T) {
		_, err := sc.CreateZtnaGroupedGatewayV1(ctx, req(0))
		assertRecoveryDelayRejected(t, err)
	})

	t.Run("non-enum value rejected", func(t *testing.T) {
		_, err := sc.CreateZtnaGroupedGatewayV1(ctx, req(60))
		assertRecoveryDelayRejected(t, err)
	})

	// Every legal duration must clear field validation and fall through to the
	// shared-member business rule. A 400 here means the server's accepted set
	// has drifted from the spec's enum.
	for _, delay := range []int{300, 1800, 3600, 10800, 28800} {
		t.Run(fmt.Sprintf("%d accepted", delay), func(t *testing.T) {
			created, err := sc.CreateZtnaGroupedGatewayV1(ctx, req(delay))
			if err == nil {
				jscCleanupDelete(t, "grouped gateway "+created.ID, func() error {
					return sc.DeleteZtnaGroupedGatewayV1(context.Background(), created.ID)
				})
				t.Fatalf("create with shared gateway members unexpectedly succeeded (id %s) — SHARED_GATEWAY_MEMBER is no longer enforced, so this test can now provision real grouped gateways and must be redesigned", created.ID)
			}
			apiErr := jamfplatform.AsAPIError(err)
			if apiErr == nil || !apiErr.HasStatus(422) {
				t.Fatalf("recoveryDelayInSec=%d: want 422 SHARED_GATEWAY_MEMBER (field validation passed), got %v", delay, err)
			}
			if fields := apiErr.FieldErrors(); len(fields["recoveryDelayInSec"]) > 0 {
				t.Errorf("recoveryDelayInSec=%d rejected on the field itself: %v", delay, fields["recoveryDelayInSec"])
			}
			t.Logf("recoveryDelayInSec=%d -> %s", delay, apiErr.Summary())
		})
	}
}

// assertRecoveryDelayRejected asserts a grouped-gateway create failed on
// recoveryDelayInSec specifically, not on some other field. The distinction
// matters: a 400 attributed elsewhere would mean the probe stopped testing
// what it claims to.
func assertRecoveryDelayRejected(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("CreateZtnaGroupedGatewayV1 unexpectedly succeeded — an illegal recoveryDelayInSec was accepted, and a grouped gateway may exist on the tenant")
	}
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(400) {
		t.Fatalf("want 400, got %v", err)
	}
	if got := apiErr.FieldErrors()["recoveryDelayInSec"]; len(got) == 0 {
		t.Fatalf("400 did not attribute the failure to recoveryDelayInSec; FieldErrors = %v", apiErr.FieldErrors())
	}
	t.Logf("rejected: %s", apiErr.Summary())
}

// jscGatewayWriteOK gates the two gateway write tests. Creating a gateway
// provisions real network egress — a datacenter allocation, an IPSec tunnel
// endpoint — and deleting one severs traffic for every access policy routed
// through it, so this must never run against a tenant anyone depends on. The
// full lifecycle was wire-verified on the wisconsam sandbox on 2026-08-20
// (create with ipsec, GET, four PATCHes, delete, 404 after) and the tenant was
// left exactly as found, which is what makes an opt-in test worth having here
// rather than a permanent skip.
func jscGatewayWriteOK(t *testing.T) {
	t.Helper()
	if os.Getenv("JAMFPLATFORM_JSC_GATEWAY_WRITE_OK") == "" {
		t.Skip("gated behind JAMFPLATFORM_JSC_GATEWAY_WRITE_OK — gateway writes provision real network egress and deleting one severs traffic for every access policy routed through it; opt in only on a tenant reserved for it")
	}
}

// TestAcceptance_SecurityCloudZtnaGatewayLifecycle exercises the gateway
// create/get/patch/delete cycle with an IPSec configuration, which is the only
// way to reach ConnectionConfigLeftResponse / ConnectionConfigRightResponse —
// no gateway carries an ipsec block unless this test made it.
//
// The gateway is created with enabled:false so it never carries traffic, and
// its subnets sit in documentation ranges.
func TestAcceptance_SecurityCloudZtnaGatewayLifecycle(t *testing.T) {
	jscGatewayWriteOK(t)
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	// lifetimeInSec became int64 in build v1424 (the spec added format: int64).
	suite := func(lifetime int64) securitycloud.CipherSuiteConfig {
		return securitycloud.CipherSuiteConfig{
			Encryption:    []string{"aes256"},
			Integrity:     []string{"sha256"},
			DhGroups:      []string{"modp2048"},
			LifetimeInSec: lifetime,
		}
	}

	name := jscName("gateway")
	enabled := false
	created, err := sc.CreateZtnaGatewayV1(ctx, &securitycloud.GatewayCreateRequest{
		Name:       name,
		Datacenter: securitycloud.GatewayCreateRequestDatacenterEuWest2,
		Enabled:    &enabled,
		TenantIds:  []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		Contact: securitycloud.GatewayContact{
			Email: "sdk-acc@example.invalid",
			Name:  "SDK acceptance",
		},
		Ipsec: &securitycloud.GatewayIpSecRequest{
			KeyExchange: "ikev2",
			Ike:         suite(28800),
			Esp:         suite(3600),
			Left: securitycloud.ConnectionConfigLeftRequest{
				ID:      "sdk-acc.example.invalid",
				Host:    "%any",
				Subnets: []string{"10.99.0.0/24"},
				Secret:  &[]string{"SdkAcceptancePsk1234"}[0],
			},
			// Two right subnets, deliberately. Build v1424 dropped maxItems:1
			// from right (and only right), and the server agrees — see the
			// cardinality assertions below. Creating with two is what proves the
			// relaxation is real rather than spec-only.
			Right: securitycloud.ConnectionConfigRightRequest{
				ID:      "peer.sdk-acc.example.invalid",
				Host:    "203.0.113.10",
				Subnets: []string{"203.0.113.0/24", "198.51.100.0/24"},
				Vendor:  securitycloud.ConnectionConfigRightRequestVendorCisco,
			},
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateZtnaGatewayV1 failed: %v", err)
	}
	jscCleanupDelete(t, "ztna gateway "+created.ID, func() error {
		return sc.DeleteZtnaGatewayV1(context.Background(), created.ID)
	})

	// The 201 body is the spec-declared CreateResponse. This reverses what the
	// server did until build v1439, when it answered with the whole Gateway —
	// config.json carried a responseType override for that, now removed. The
	// shape change is independent of content encoding (wire-verified on both
	// the gzip and identity paths); only href's population varies, and see
	// assertCreateHrefEmpty for why Go never sees it.
	if created.ID == "" {
		t.Fatal("CreateZtnaGatewayV1 returned an empty ID")
	}
	assertCreateHrefEmpty(t, created.Href, "ztna gateway")

	got, err := sc.GetZtnaGatewayV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaGatewayV1(%s) failed: %v", created.ID, err)
	}
	if got.Ipsec == nil {
		t.Fatal("GetZtnaGatewayV1 returned no ipsec block")
	}
	// The secret is write-only and must never come back on a read.
	t.Logf("ipsec left:  %+v", got.Ipsec.Left)
	t.Logf("ipsec right: %+v", got.Ipsec.Right)
	if got.Ipsec.Right.Vendor != securitycloud.ConnectionConfigRightResponseVendorCisco {
		t.Errorf("right.vendor = %q, want Cisco", got.Ipsec.Right.Vendor)
	}
	// Both right subnets must survive the round trip. Wire-verified 2026-08-21:
	// right accepts many, left is still capped at one, and build v1424's schema
	// change removed maxItems from right alone — an asymmetry worth pinning
	// because nothing in the spec explains it.
	if len(got.Ipsec.Right.Subnets) != 2 {
		t.Errorf("right.subnets = %v, want both subnets preserved — v1424 dropped maxItems:1 from right, so a single-element result means the server still caps it", got.Ipsec.Right.Subnets)
	}
	if len(got.Ipsec.Left.Subnets) != 1 {
		t.Errorf("left.subnets = %v, want exactly 1", got.Ipsec.Left.Subnets)
	}

	// The other half of the asymmetry: two *left* subnets are refused. This
	// needs an otherwise-valid request, because the deep IPSec check does not
	// run while a required top-level field is missing — which is why it lives
	// here rather than in the ungated rejections test, whose cases all fail
	// earlier. The rejection means nothing is provisioned.
	tooManyLeft := &securitycloud.GatewayCreateRequest{
		Name:       jscName("gateway-leftpair"),
		Datacenter: securitycloud.GatewayCreateRequestDatacenterEuWest2,
		Enabled:    &enabled,
		TenantIds:  []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		Contact:    securitycloud.GatewayContact{Email: "sdk-acc@example.invalid", Name: "SDK acceptance"},
		Ipsec: &securitycloud.GatewayIpSecRequest{
			KeyExchange: "ikev2",
			Ike:         suite(28800),
			Esp:         suite(3600),
			Left: securitycloud.ConnectionConfigLeftRequest{
				ID:      "sdk-acc.example.invalid",
				Host:    "%any",
				Subnets: []string{"10.99.0.0/24", "10.98.0.0/24"},
				Secret:  &[]string{"SdkAcceptancePsk1234"}[0],
			},
			Right: securitycloud.ConnectionConfigRightRequest{
				ID:      "peer.sdk-acc.example.invalid",
				Host:    "203.0.113.10",
				Subnets: []string{"203.0.113.0/24"},
				Vendor:  securitycloud.ConnectionConfigRightRequestVendorCisco,
			},
		},
	}
	if extra, err := sc.CreateZtnaGatewayV1(ctx, tooManyLeft); err == nil {
		jscCleanupDelete(t, "ztna gateway "+extra.ID, func() error {
			return sc.DeleteZtnaGatewayV1(context.Background(), extra.ID)
		})
		t.Error("two left subnets were accepted — left's maxItems:1 is no longer enforced, so the spec should drop it there too")
	} else if apiErr := jamfplatform.AsAPIError(err); apiErr == nil || !apiErr.HasStatus(400) {
		t.Errorf("two left subnets: want 400, got %v", err)
	} else {
		// Reported against `ipsec`, not `ipsec.left.subnets` — the array-level
		// checks surface at the block, so a caller gets no path to the field.
		t.Logf("two left subnets -> %s", apiErr.Summary())
	}

	// A whole-block ipsec patch still works, and is the shape a caller uses to
	// replace cipher suites and endpoints together. Omitting left.secret
	// preserves the existing PSK — the secret can be rotated, never cleared.
	full := func(espLifetime int64) *securitycloud.GatewayIpSecPatchRequest {
		return &securitycloud.GatewayIpSecPatchRequest{
			KeyExchange: &[]string{"ikev2"}[0],
			Ike:         &[]securitycloud.CipherSuiteConfig{suite(28800)}[0],
			Esp:         &[]securitycloud.CipherSuiteConfig{suite(espLifetime)}[0],
			Left: &securitycloud.ConnectionConfigPatchLeftRequest{
				ID:      &[]string{"sdk-acc.example.invalid"}[0],
				Host:    &[]string{"%any"}[0],
				Subnets: &[]string{"10.99.0.0/24"},
			},
			Right: &securitycloud.ConnectionConfigPatchRightRequest{
				ID:      &[]string{"peer.sdk-acc.example.invalid"}[0],
				Host:    &[]string{"203.0.113.10"}[0],
				Subnets: &[]string{"10.98.0.0/16"},
				Vendor:  &[]string{securitycloud.ConnectionConfigPatchRightRequestVendorCisco}[0],
			},
		}
	}
	if err := sc.UpdateZtnaGatewayV1(ctx, created.ID, &securitycloud.GatewayPatchRequest{
		Ipsec: full(7200),
	}); err != nil {
		t.Fatalf("UpdateZtnaGatewayV1 full ipsec patch failed: %v", err)
	}
	after, err := sc.GetZtnaGatewayV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaGatewayV1 after patch failed: %v", err)
	}
	if after.Ipsec == nil || after.Ipsec.Esp.LifetimeInSec != 7200 {
		t.Errorf("after ipsec patch, esp.lifetimeInSec = %v, want 7200", after.Ipsec)
	}
	if after.Ipsec.Ike.LifetimeInSec != 28800 {
		t.Errorf("ipsec patch clobbered ike: lifetimeInSec = %d, want 28800", after.Ipsec.Ike.LifetimeInSec)
	}

	// The partial ipsec merge-patch. The server has always deep-merged this
	// correctly (curl-verified 2026-08-20) but it was unreachable from Go until
	// build v1416 gave PATCH its own all-optional GatewayIpSecPatchRequest:
	// GatewayPatchRequest.ipsec previously reused the POST-shaped
	// GatewayIpSecRequest, whose required fields are non-pointer, so a partial
	// value marshalled keyExchange:"" plus empty sub-objects and earned a 400
	// "Request body is missing or malformed." Where an earlier revision of this
	// test pinned that 400 so it would fail the day the shape landed, this
	// asserts the capability instead — esp is replaced and every sibling
	// survives, which is the whole point of the new schema.
	if err := sc.UpdateZtnaGatewayV1(ctx, created.ID, &securitycloud.GatewayPatchRequest{
		Ipsec: &securitycloud.GatewayIpSecPatchRequest{
			Esp: &[]securitycloud.CipherSuiteConfig{suite(3600)}[0],
		},
	}); err != nil {
		t.Fatalf("partial ipsec merge-patch failed — the all-optional PATCH shape landed in v1416 and should make this reachable: %v", err)
	}
	merged, err := sc.GetZtnaGatewayV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaGatewayV1 after partial patch failed: %v", err)
	}
	if merged.Ipsec == nil {
		t.Fatal("partial ipsec patch dropped the whole ipsec block")
	}
	if merged.Ipsec.Esp.LifetimeInSec != 3600 {
		t.Errorf("partial patch did not apply: esp.lifetimeInSec = %d, want 3600", merged.Ipsec.Esp.LifetimeInSec)
	}
	// Everything not named in the patch must survive the deep merge. These are
	// the assertions that would have caught a server-side replace-not-merge.
	if merged.Ipsec.Ike.LifetimeInSec != 28800 {
		t.Errorf("partial patch clobbered ike: lifetimeInSec = %d, want 28800", merged.Ipsec.Ike.LifetimeInSec)
	}
	if merged.Ipsec.KeyExchange != "ikev2" {
		t.Errorf("partial patch clobbered keyExchange: %q, want ikev2", merged.Ipsec.KeyExchange)
	}
	if merged.Ipsec.Left == nil || merged.Ipsec.Left.ID != "sdk-acc.example.invalid" {
		t.Errorf("partial patch clobbered left: %+v", merged.Ipsec.Left)
	}
	if merged.Ipsec.Right == nil || merged.Ipsec.Right.Vendor != securitycloud.ConnectionConfigRightResponseVendorCisco {
		t.Errorf("partial patch clobbered right: %+v", merged.Ipsec.Right)
	}

	// Rotating the PSK alone is the use case the new PATCH schema exists for —
	// its own description says "supply only `left.secret` to rotate the
	// pre-shared key". The secret is write-only so the new value cannot be read
	// back; what is checkable is that the call is accepted and clobbers nothing,
	// which is what would break if `left` were replaced rather than merged.
	if err := sc.UpdateZtnaGatewayV1(ctx, created.ID, &securitycloud.GatewayPatchRequest{
		Ipsec: &securitycloud.GatewayIpSecPatchRequest{
			Left: &securitycloud.ConnectionConfigPatchLeftRequest{
				Secret: &[]string{"SdkAcceptancePskRotated9876"}[0],
			},
		},
	}); err != nil {
		t.Fatalf("secret-only ipsec rotation failed: %v", err)
	}
	rotated, err := sc.GetZtnaGatewayV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaGatewayV1 after secret rotation failed: %v", err)
	}
	if rotated.Ipsec == nil || rotated.Ipsec.Left == nil {
		t.Fatal("secret-only rotation dropped the ipsec left block")
	}
	if rotated.Ipsec.Left.ID != "sdk-acc.example.invalid" || len(rotated.Ipsec.Left.Subnets) != 1 {
		t.Errorf("secret-only rotation clobbered the rest of left: %+v", rotated.Ipsec.Left)
	}
	if rotated.Ipsec.Esp.LifetimeInSec != 3600 {
		t.Errorf("secret-only rotation clobbered esp: lifetimeInSec = %d, want 3600", rotated.Ipsec.Esp.LifetimeInSec)
	}

	// A merge-patch that touches nothing but the name leaves ipsec intact.
	if err := sc.UpdateZtnaGatewayV1(ctx, created.ID, &securitycloud.GatewayPatchRequest{
		Name: &[]string{name + "-renamed"}[0],
	}); err != nil {
		t.Fatalf("UpdateZtnaGatewayV1 name-only patch failed: %v", err)
	}
	renamed, err := sc.GetZtnaGatewayV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaGatewayV1 after name-only patch failed: %v", err)
	}
	if renamed.Ipsec == nil {
		t.Error("a name-only patch dropped the ipsec configuration")
	}

	// Removing ipsec and clearing the PSK are both refused (wire-verified
	// 2026-08-20). IPSEC_REMOVAL_NOT_SUPPORTED is not in the published spec
	// yet; the pending restructure adds it.
	if err := sc.UpdateZtnaGatewayV1(ctx, created.ID, &securitycloud.GatewayPatchRequest{}); err != nil {
		t.Fatalf("empty merge-patch failed: %v", err)
	}

	if err := sc.DeleteZtnaGatewayV1(ctx, created.ID); err != nil {
		t.Fatalf("DeleteZtnaGatewayV1(%s) failed: %v", created.ID, err)
	}
	if _, err := sc.GetZtnaGatewayV1(ctx, created.ID); !isSecurityCloudNotFound(err) {
		t.Errorf("after delete, want 404, got %v", err)
	}
}

// TestAcceptance_SecurityCloudZtnaGatewayIpSecRejections pins the server-side
// IPSec rules that no spec states, all wire-verified 2026-08-20. Every case
// here is a rejection, so nothing is provisioned and the test is safe to run
// ungated — the required top-level fields are deliberately omitted so a
// gateway can never be created even if a rule stops being enforced.
func TestAcceptance_SecurityCloudZtnaGatewayIpSecRejections(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	suite := securitycloud.CipherSuiteConfig{
		Encryption:    []string{"aes256"},
		Integrity:     []string{"sha256"},
		DhGroups:      []string{"modp2048"},
		LifetimeInSec: 3600,
	}
	ipsec := func(leftSubnet, vendor string) *securitycloud.GatewayIpSecRequest {
		return &securitycloud.GatewayIpSecRequest{
			KeyExchange: "ikev2",
			Ike:         suite,
			Esp:         suite,
			Left: securitycloud.ConnectionConfigLeftRequest{
				ID: "sdk-acc.example.invalid", Host: "%any", Subnets: []string{leftSubnet},
			},
			Right: securitycloud.ConnectionConfigRightRequest{
				ID: "peer.sdk-acc.example.invalid", Host: "203.0.113.10",
				Subnets: []string{"10.98.0.0/16"}, Vendor: vendor,
			},
		}
	}

	// No name / datacenter / tenantIds / contact: creation is impossible.
	for _, tc := range []struct {
		name    string
		ipsec   *securitycloud.GatewayIpSecRequest
		wantMsg string
	}{
		{"public left subnet", ipsec("8.8.8.0/24", securitycloud.ConnectionConfigRightRequestVendorCisco), "private range"},
		// Deliberate literals: these are the values a caller would type without
		// the constants, and the point is that the server refuses them.
		{"lowercase vendor", ipsec("10.99.0.0/24", "cisco"), "malformed"},
		{"unknown vendor", ipsec("10.99.0.0/24", "NotAVendor"), "malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sc.CreateZtnaGatewayV1(ctx, &securitycloud.GatewayCreateRequest{Ipsec: tc.ipsec})
			if err == nil {
				t.Fatal("CreateZtnaGatewayV1 unexpectedly succeeded — a gateway may have been provisioned, check the tenant")
			}
			apiErr := jamfplatform.AsAPIError(err)
			if apiErr == nil || !apiErr.HasStatus(400) {
				t.Fatalf("want 400, got %v", err)
			}
			if !strings.Contains(strings.ToLower(apiErr.Summary()), tc.wantMsg) {
				t.Errorf("400 summary = %q, want it to mention %q", apiErr.Summary(), tc.wantMsg)
			}
			t.Logf("%s -> %s", tc.name, apiErr.Summary())
		})
	}
}

// ---------------------------------------------------------------------------
// Content categories
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudContentCategories(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	cats, err := sc.ListContentCategoriesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListContentCategoriesV1 failed: %v", err)
	}
	if len(cats.Results) == 0 {
		t.Fatal("ListContentCategoriesV1 returned no categories — the catalogue is a per-tenant system list and should never be empty")
	}
	t.Logf("%d content categories (totalCount=%d), first: id=%s name=%q displayName=%q",
		len(cats.Results), cats.TotalCount, cats.Results[0].ID, cats.Results[0].Name, cats.Results[0].DisplayName)
}

// ---------------------------------------------------------------------------
// Device groups
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudDeviceGroupLifecycle(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	created, err := sc.CreateDeviceGroupV1(ctx, &securitycloud.CreateGroupRequest{Name: jscName("group")})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateDeviceGroupV1 failed: %v", err)
	}
	jscCleanupDelete(t, "device group "+created.ID, func() error {
		return sc.DeleteDeviceGroupV1(context.Background(), created.ID)
	})
	if created.ID == "" {
		t.Fatal("CreateDeviceGroupV1 returned an empty ID")
	}

	got, err := sc.GetDeviceGroupV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDeviceGroupV1(%s) failed: %v", created.ID, err)
	}
	if got.Name != jscName("group") {
		t.Errorf("group name = %q, want %q", got.Name, jscName("group"))
	}

	// ListDeviceGroupsV1 is deprecated (2026-08-12) and v2 is routed as of
	// 2026-08-20, but v1 stays in the SDK — and stays covered — until Jamf
	// removes the path, so consumers get a real migration window. Both
	// versions' resolvers and Apply methods exist side by side for the same
	// reason (see the pro packages, where V1/V2/V3 coexist).
	groups, err := sc.ListDeviceGroupsV1(ctx)
	if err != nil {
		t.Fatalf("ListDeviceGroupsV1 failed: %v", err)
	}
	// GroupListResponse is an alias for []Group, so the method returns a
	// pointer to a slice — the shape any bare-array response takes.
	var found bool
	for _, g := range *groups {
		if g.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("created group %s missing from ListDeviceGroupsV1 (%d groups)", created.ID, len(*groups))
	}

	// Wire-verified: the update answers 200 with the updated group, though the
	// spec declares 204 with no content. An empty name back means the
	// config-level expectedStatus/responseType override regressed.
	updated, err := sc.UpdateDeviceGroupV1(ctx, created.ID, &securitycloud.UpdateGroupRequest{Name: jscName("group") + "-renamed"})
	if err != nil {
		t.Fatalf("UpdateDeviceGroupV1(%s) failed: %v", created.ID, err)
	}
	if updated.Name != jscName("group")+"-renamed" {
		t.Errorf("UpdateDeviceGroupV1 returned name %q, want %q", updated.Name, jscName("group")+"-renamed")
	}

	if err := sc.DeleteDeviceGroupV1(ctx, created.ID); err != nil {
		t.Fatalf("DeleteDeviceGroupV1(%s) failed: %v", created.ID, err)
	}
	if _, err := sc.GetDeviceGroupV1(ctx, created.ID); !isSecurityCloudNotFound(err) {
		t.Errorf("after delete, want 404, got %v", err)
	}
}

// TestAcceptance_SecurityCloudDeviceGroupsV2 no longer tolerates a 403: the
// gateway routes /v2/.../groups as of 2026-08-20 (wire-verified on eu, tenant
// wisconsam, where it returns the {groups: []} envelope). It was 403
// BAD_PERMISSIONS when the surface was first generated, so a 403 resurfacing
// here means routing regressed or the region in use lags eu — either way that
// is worth a failure rather than a skip.
func TestAcceptance_SecurityCloudDeviceGroupsV2(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	groups, err := sc.ListDeviceGroupsV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListDeviceGroupsV2 failed: %v", err)
	}
	t.Logf("v2 returned %d device groups", len(groups.Groups))
}

// ---------------------------------------------------------------------------
// UEM Connect
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudUemConnectReads(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	connectorPage, err := sc.ListUemConnectorsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListUemConnectorsV1 failed: %v", err)
	}
	connectors := connectorPage.Results
	t.Logf("%d UEM connectors", len(connectors))
	if len(connectors) == 0 {
		t.Skip("tenant has no UEM connector configured")
	}
	id := connectors[0].ID

	got, err := sc.GetUemConnectorV1(ctx, id)
	if err != nil {
		t.Fatalf("GetUemConnectorV1(%s) failed: %v", id, err)
	}
	t.Logf("connector %s: vendor=%s url=%s connected=%v enabled=%v scheduled=%v",
		got.ID, got.Vendor, got.URL, got.Connected, got.Enabled, got.Scheduled)

	settings, err := sc.GetUemConnectorSyncSettingsV1(ctx, id)
	if err != nil {
		t.Fatalf("GetUemConnectorSyncSettingsV1(%s) failed: %v", id, err)
	}
	t.Logf("sync settings: refreshRateMinutes=%d concurrentSyncEnabled=%v deviceRiskTagging=%v",
		settings.RefreshRateMinutes, settings.ConcurrentSyncEnabled, settings.DeviceRiskTagging)

	runs, err := sc.ListUemConnectorSyncRunsV1(ctx, id)
	if err != nil {
		t.Fatalf("ListUemConnectorSyncRunsV1(%s) failed: %v", id, err)
	}
	t.Logf("%d sync runs", len(runs))
	for _, r := range runs {
		t.Logf("  run %s: status=%s type=%s synced=%d errored=%d deleted=%d", r.TransactionID, r.Status, r.RefreshType, r.Synced, r.Errored, r.Deleted)
	}
}

// jscProUemCredentials mints a throwaway API role, API integration and client
// credential set on the *Jamf Pro* tenant and returns the instance URL plus the
// credentials, all through the SDK. This is what a real JAMF_PRO UEM connector
// authenticates with, so it is the only way to build a genuine connector create
// request without hand-carrying a secret in an env var.
//
// It needs the Jamf Pro credential set (JAMFPLATFORM_*), which is a different
// product and a different tenant from the Security Cloud one — see
// errAccJSCCredsUnset. Both are required, and the Pro side is what skips when
// absent.
//
// Everything it creates is registered for deletion. The role carries every
// privilege the tenant offers rather than a guessed subset: UEM Connect's
// required Pro privileges are not documented anywhere, and this is a throwaway
// integration whose credentials never leave the test.
func jscProUemCredentials(t *testing.T) (instanceURL, clientID, clientSecret string) {
	t.Helper()
	c := accClient(t)
	p := pro.New(c)
	ctx := context.Background()

	serverURL, err := p.GetJamfProServerURLV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetJamfProServerURLV1 failed: %v", err)
	}
	if serverURL.URL == "" {
		t.Fatal("GetJamfProServerURLV1 returned an empty URL — a UEM connector has nothing to point at")
	}

	privs, err := p.ListApiRolePrivilegesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListApiRolePrivilegesV1 failed: %v", err)
	}
	if len(privs.Privileges) == 0 {
		t.Fatal("ListApiRolePrivilegesV1 returned no privileges")
	}

	roleName := jscName("uem-role")
	role, err := p.CreateApiRoleV1(ctx, &pro.ApiRoleRequest{
		DisplayName: roleName,
		Privileges:  privs.Privileges,
	})
	if err != nil {
		t.Fatalf("CreateApiRoleV1 failed: %v", err)
	}
	roleID := role.ID
	cleanupDelete(t, "pro api role "+roleID, func() error {
		return p.DeleteApiRoleV1(context.Background(), roleID)
	})

	integration, err := p.CreateApiIntegrationV1(ctx, &pro.ApiIntegrationRequest{
		DisplayName:                jscName("uem-integration"),
		AuthorizationScopes:        []string{roleName},
		Enabled:                    &[]bool{true}[0],
		AccessTokenLifetimeSeconds: &[]int{600}[0],
	})
	if err != nil {
		t.Fatalf("CreateApiIntegrationV1 failed: %v", err)
	}
	// ApiIntegrationResponse.ID is an int; every path parameter is a string.
	integrationID := strconv.Itoa(integration.ID)
	cleanupDelete(t, "pro api integration "+integrationID, func() error {
		return p.DeleteApiIntegrationV1(context.Background(), integrationID)
	})

	creds, err := p.RotateApiIntegrationClientCredentialsV1(ctx, integrationID)
	if err != nil {
		t.Fatalf("RotateApiIntegrationClientCredentialsV1 failed: %v", err)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		t.Fatal("client-credentials returned an empty pair")
	}
	return serverURL.URL, creds.ClientID, creds.ClientSecret
}

// TestAcceptance_SecurityCloudUemConnectCreate drives CreateUemConnectorV1 with
// a real, fully-populated request: a Jamf Pro instance URL and OAuth credentials
// minted on the Pro tenant by jscProUemCredentials. It replaces a blanket skip
// that claimed the whole family was unreachable — most of it is, but the create
// path is not, and it was worth finding out where exactly it stops.
//
// Two things this pins that no spec states, both wire-verified 2026-08-21:
//
//   - `authStrategy` is required, and omitting it is a **500 INTERNAL_ERROR**
//     rather than a validation error. That matters because the published
//     ConnectorCreateRequest declares only vendor/url/isoCountry, so a request
//     built from the spec alone could only ever 500. authStrategy and
//     deviceSyncAuth are restored via schemaCreations/schemaPatches, and the
//     500 subtest is the regression guard: if it starts returning 4xx the
//     server bug is fixed and the note in CLAUDE.md should say so.
//   - A tenant may hold **one connector, full stop**. A complete, correct
//     request answers 409 CONNECTOR_CONFIG_ALREADY_EXISTS whenever any
//     connector exists, whatever its vendor — the message says "incompatible
//     UEM vendor" but INTUNE and JAMF_PRO are refused identically, and the
//     check fires before credential validation (bogus credentials give the
//     same 409).
//
// So on a tenant that already has a connector this exercises the create path
// right up to the singleton pre-check without provisioning anything. On a tenant
// with none it really does create one, which is why the success branch deletes
// it immediately and then fails: that tenant can support the full lifecycle and
// this test should be widened rather than silently starting to mutate a
// connector other tests read.
func TestAcceptance_SecurityCloudUemConnectCreate(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	instanceURL, clientID, clientSecret := jscProUemCredentials(t)
	t.Logf("minted Pro OAuth credentials for %s (clientId %s)", instanceURL, clientID)

	req := func() *securitycloud.ConnectorCreateRequest {
		return &securitycloud.ConnectorCreateRequest{
			Vendor:       securitycloud.ConnectorCreateRequestVendorJamfPro,
			URL:          instanceURL,
			AuthStrategy: &[]string{"JAMF_PRO_OAUTH"}[0],
			DeviceSyncAuth: &securitycloud.DeviceSyncAuth{
				ClientID:     &clientID,
				ClientSecret: &clientSecret,
			},
		}
	}

	// Ordered deliberately: prove the spec-shaped request is unusable, then that
	// authStrategy alone promotes the failure to real validation, then that a
	// complete request clears validation entirely.
	t.Run("authStrategy omitted is a 500, not a validation error", func(t *testing.T) {
		bare := req()
		bare.AuthStrategy = nil
		bare.DeviceSyncAuth = nil
		_, err := sc.CreateUemConnectorV1(ctx, bare)
		if err == nil {
			t.Fatal("a vendor+url-only create succeeded — the server bug is fixed and a connector may now exist on the tenant")
		}
		apiErr := jamfplatform.AsAPIError(err)
		if apiErr == nil {
			t.Fatalf("want an API error, got %v", err)
		}
		if !apiErr.HasStatus(500) {
			t.Fatalf("want 500 INTERNAL_ERROR for a spec-shaped request, got %d (%s) — if this is now a 4xx the server bug is fixed; update CLAUDE.md and this assertion", apiErr.StatusCode, apiErr.Summary())
		}
		t.Logf("spec-shaped request -> %s", apiErr.Summary())
	})

	t.Run("authStrategy without credentials is a 422", func(t *testing.T) {
		noAuth := req()
		noAuth.DeviceSyncAuth = nil
		_, err := sc.CreateUemConnectorV1(ctx, noAuth)
		if err == nil {
			t.Fatal("a create with no deviceSyncAuth succeeded — an unauthenticated connector may now exist on the tenant")
		}
		apiErr := jamfplatform.AsAPIError(err)
		if apiErr == nil || !apiErr.HasStatus(422) {
			t.Fatalf("want 422 VALIDATION_FAILED once authStrategy is present, got %v", err)
		}
		t.Logf("authStrategy without deviceSyncAuth -> %s", apiErr.Summary())
	})

	t.Run("a complete request clears validation", func(t *testing.T) {
		created, err := sc.CreateUemConnectorV1(ctx, req())
		if err == nil {
			// The tenant had no connector, so this made one. Remove it before
			// anything else observes it, then fail: the full lifecycle is now
			// reachable here and deserves real coverage.
			id := created.ID
			jscCleanupDelete(t, "uem connector "+id, func() error {
				return sc.DeleteUemConnectorV1(context.Background(), id)
			})
			t.Fatalf("CreateUemConnectorV1 succeeded (id %s) — this tenant holds no other connector, so enablement, sync-settings and sync runs are all exercisable now; widen this test rather than leaving the create unasserted", id)
		}
		apiErr := jamfplatform.AsAPIError(err)
		if apiErr == nil || !apiErr.HasStatus(409) {
			t.Fatalf("want 409 CONNECTOR_CONFIG_ALREADY_EXISTS (validation passed, singleton refused), got %v", err)
		}
		// A 409 here and a 422 above together prove the credential fields were
		// structurally accepted: the request got past field validation to a
		// state check, which is the furthest a shared tenant allows.
		t.Logf("complete request -> %s", apiErr.Summary())
	})
}

// TestAcceptance_SecurityCloudUemConnectWrites documents why the remaining UEM
// Connect writes stay unexercised. Create is covered by
// TestAcceptance_SecurityCloudUemConnectCreate; these are the ones that act on
// whichever connector the tenant already owns.
func TestAcceptance_SecurityCloudUemConnectWrites(t *testing.T) {
	accSecurityCloudClient(t)
	t.Skip("every remaining UEM Connect write mutates the tenant's existing connector, which is a live link to a real UEM instance whose credentials are write-only and therefore unrestorable: DeleteUemConnectorV1 would destroy it permanently (the clientSecret cannot be read back to recreate it); EnableUemConnectorV1 / DisableUemConnectorV1 toggle whether it syncs a real device fleet; UpdateUemConnectorSyncSettingsV1 is a documented full replacement whose read shape (ConnectorConfig, syncConfig nested) differs from its write shape (SyncSettings, those fields top-level), so a round-trip that mis-maps one field silently resets the connector's configuration; TriggerUemConnectorSyncV1 and CancelUemConnectorSyncV1 start and abort a real inventory sync against the connected instance; DeployActivationProfileToUemV1 pushes an activation profile into that instance's device fleet. Exercising these needs a tenant whose connector is disposable — note a tenant may hold only one, so it cannot be a tenant that also has a connector worth keeping.")
}

// ---------------------------------------------------------------------------
// Resolvers and Apply (upsert)
// ---------------------------------------------------------------------------

// TestAcceptance_SecurityCloudResolvers checks every read-only resolver against
// live data. Security Cloud offers no RSQL filter and no search parameter on any
// list endpoint, so all six resolvers run in clientFilter mode — the match
// happens in memory, which makes "does the list element really carry this name
// field" a wire question rather than a spec one.
func TestAcceptance_SecurityCloudResolvers(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	// Content categories are a per-tenant system catalogue, so this resolver is
	// always exercisable — and it is the one a caller needs before creating a
	// ZTNA app, whose categoryName validates against displayName.
	cats, err := sc.ListContentCategoriesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListContentCategoriesV1 failed: %v", err)
	}
	if len(cats.Results) > 0 {
		want := cats.Results[0]
		id, err := sc.ResolveContentCategoryV1IDByName(ctx, want.DisplayName)
		if err != nil {
			t.Fatalf("ResolveContentCategoryV1IDByName(%q) failed: %v", want.DisplayName, err)
		}
		if id != want.ID {
			t.Errorf("resolved category ID = %q, want %q", id, want.ID)
		}
		got, err := sc.ResolveContentCategoryV1ByName(ctx, want.DisplayName)
		if err != nil {
			t.Fatalf("ResolveContentCategoryV1ByName(%q) failed: %v", want.DisplayName, err)
		}
		if got.ID != want.ID {
			t.Errorf("resolved category = %q, want %q", got.ID, want.ID)
		}
	}

	// ZTNA gateways and apps resolve across pages; grouped gateways do not
	// (their list endpoint declares no page params), so both transport walks
	// get exercised.
	gatewayPage, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
	gateways := gatewayPage.Results
	if len(gateways) > 0 {
		id, err := sc.ResolveZtnaGatewayV1IDByName(ctx, gateways[0].Name)
		if err != nil {
			t.Fatalf("ResolveZtnaGatewayV1IDByName(%q) failed: %v", gateways[0].Name, err)
		}
		if id != gateways[0].ID {
			t.Errorf("resolved gateway ID = %q, want %q", id, gateways[0].ID)
		}
	}

	grouped, err := sc.ListZtnaGroupedGatewaysV1(ctx)
	if err != nil {
		t.Fatalf("ListZtnaGroupedGatewaysV1 failed: %v", err)
	}
	if len(grouped.Results) > 0 {
		got, err := sc.ResolveZtnaGroupedGatewayV1ByName(ctx, grouped.Results[0].Name)
		if err != nil {
			t.Fatalf("ResolveZtnaGroupedGatewayV1ByName(%q) failed: %v", grouped.Results[0].Name, err)
		}
		if got.ID != grouped.Results[0].ID {
			t.Errorf("resolved grouped gateway = %q, want %q", got.ID, grouped.Results[0].ID)
		}
	}

	// An app created from a predefinedAppId has name null on the wire, so only
	// a named app is resolvable — pick one rather than assuming the first has a
	// name.
	apps, err := sc.ListZtnaAppsV1(ctx)
	if err != nil {
		t.Fatalf("ListZtnaAppsV1 failed: %v", err)
	}
	var namedApp *securitycloud.App
	for i := range apps {
		if apps[i].Name != nil && *apps[i].Name != "" {
			namedApp = &apps[i]
			break
		}
	}
	if namedApp == nil {
		t.Logf("no ZTNA app has a name (all %d are predefined-template apps, which return name null) — ResolveZtnaAppV1ByName is covered by the app lifecycle test instead", len(apps))
	} else {
		id, err := sc.ResolveZtnaAppV1IDByName(ctx, *namedApp.Name)
		if err != nil {
			t.Fatalf("ResolveZtnaAppV1IDByName(%q) failed: %v", *namedApp.Name, err)
		}
		if id != namedApp.ID {
			t.Errorf("resolved app ID = %q, want %q", id, namedApp.ID)
		}
	}

	// A name nothing owns must be a 404, not an empty string — Apply branches
	// on exactly this to decide create-vs-update.
	absent := jscName("does-not-exist")
	if _, err := sc.ResolveDnsZoneV1IDByName(ctx, absent); !isSecurityCloudNotFound(err) {
		t.Errorf("resolving an absent DNS zone: want 404 APIResponseError, got %v", err)
	}
	// Both device-group resolver versions are live and hit different endpoints:
	// v1 walks the deprecated bare-array list, v2 the {groups: []} envelope.
	// Cover both — the envelope unwrap is the only resultsField in the package,
	// so a regression there would otherwise be invisible.
	if _, err := sc.ResolveDeviceGroupV1IDByName(ctx, absent); !isSecurityCloudNotFound(err) {
		t.Errorf("resolving an absent device group (v1): want 404 APIResponseError, got %v", err)
	}
	if _, err := sc.ResolveDeviceGroupV2IDByName(ctx, absent); !isSecurityCloudNotFound(err) {
		t.Errorf("resolving an absent device group (v2): want 404 APIResponseError, got %v", err)
	}

	// An empty name is a caller bug, and must not degrade into "match the first
	// element with no name" — which is exactly what a predefined ZTNA app would
	// be.
	if _, err := sc.ResolveZtnaAppV1IDByName(ctx, ""); err == nil {
		t.Error("resolving an empty app name succeeded; want an error")
	}
}

// TestAcceptance_SecurityCloudApplyDeviceGroup exercises both Apply branches on
// the cheapest resource in the family: create-when-absent, then
// update-when-present with the same name.
//
// Run once per endpoint version. Both Apply methods share the v1 create,
// update and delete ops — only the resolve step differs, and it differs in the
// way most likely to break silently: v1 walks a bare JSON array, v2 unwraps a
// {groups: []} envelope. Covering only one version would leave the other's
// resolve path untested while it stayed exported and callable, which is the
// cost of the SDK's additive-versioning rule and the reason to pay it here.
func TestAcceptance_SecurityCloudApplyDeviceGroup(t *testing.T) {
	sc := accSecurityCloudClient(t)

	for _, tc := range []struct {
		version string
		apply   func(context.Context, *securitycloud.CreateGroupRequest) (string, bool, error)
	}{
		{"v1", sc.ApplyDeviceGroupV1},
		{"v2", sc.ApplyDeviceGroupV2},
	} {
		t.Run(tc.version, func(t *testing.T) {
			ctx := context.Background()
			name := jscName("apply-group-" + tc.version)

			id, created, err := tc.apply(ctx, &securitycloud.CreateGroupRequest{Name: name})
			if err != nil {
				skipOnServerError(t, err)
				t.Fatalf("ApplyDeviceGroup%s (create branch) failed: %v", tc.version, err)
			}
			jscCleanupDelete(t, "device group "+id, func() error {
				return sc.DeleteDeviceGroupV1(context.Background(), id)
			})
			if !created {
				t.Errorf("first Apply of %q reported created=false; nothing owned that name", name)
			}
			if id == "" {
				t.Fatalf("ApplyDeviceGroup%s returned an empty ID on the create branch", tc.version)
			}

			// Second Apply with the same name must resolve to the same resource
			// and take the update path — a create here would leave a duplicate
			// behind, which is the failure mode Apply exists to prevent.
			sameID, created, err := tc.apply(ctx, &securitycloud.CreateGroupRequest{Name: name})
			if err != nil {
				t.Fatalf("ApplyDeviceGroup%s (update branch) failed: %v", tc.version, err)
			}
			if created {
				t.Error("second Apply reported created=true; it should have resolved the existing group")
			}
			if sameID != id {
				t.Errorf("second Apply returned ID %q, want %q", sameID, id)
			}
		})
	}
}

// TestAcceptance_SecurityCloudApplyDnsZone covers the Apply variant that
// converts the create request into a different update type (ZoneWrite →
// ZonePatch) by JSON round-trip. A field the patch type does not carry would be
// dropped silently, so the update branch checks the zone still holds what was
// sent.
func TestAcceptance_SecurityCloudApplyDnsZone(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	gateways := jscEnsureGateways(t, sc, 1)

	name := jscName("apply-zone")
	req := &securitycloud.ZoneWrite{
		Name:        name,
		Domains:     []string{"sdk-acc-apply.invalid"},
		NameServers: []securitycloud.NameServer{{IP: "203.0.113.53", GatewayID: gateways[0].ID}},
	}

	id, created, err := sc.ApplyDnsZoneV1(ctx, req)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ApplyDnsZoneV1 (create branch) failed: %v", err)
	}
	jscCleanupDelete(t, "dns zone "+id, func() error {
		return sc.DeleteDnsZoneV1(context.Background(), id)
	})
	if !created {
		t.Errorf("first Apply of %q reported created=false", name)
	}

	// Same name, changed domains: must update in place, and the round-trip
	// through ZonePatch must carry the new domain list.
	req.Domains = []string{"sdk-acc-apply.invalid", "sdk-acc-apply-2.invalid"}
	sameID, created, err := sc.ApplyDnsZoneV1(ctx, req)
	if err != nil {
		t.Fatalf("ApplyDnsZoneV1 (update branch) failed: %v", err)
	}
	if created {
		t.Error("second Apply reported created=true; it should have resolved the existing zone")
	}
	if sameID != id {
		t.Errorf("second Apply returned ID %q, want %q", sameID, id)
	}
	got, err := sc.GetDnsZoneV1(ctx, id)
	if err != nil {
		t.Fatalf("GetDnsZoneV1 after Apply update failed: %v", err)
	}
	if len(got.Domains) != 2 {
		t.Errorf("after Apply update, zone has domains %v; the ZoneWrite→ZonePatch round-trip dropped them", got.Domains)
	}
	if len(got.NameServers) != 1 {
		t.Errorf("after Apply update, zone has name servers %+v; want the one that was sent", got.NameServers)
	}
}

// TestAcceptance_SecurityCloudApplyZtnaApp covers Apply on a resource whose
// name field is an optional pointer, which the generator has to unwrap before
// it can resolve.
func TestAcceptance_SecurityCloudApplyZtnaApp(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	cats, err := sc.ListContentCategoriesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListContentCategoriesV1 failed: %v", err)
	}
	if len(cats.Results) == 0 {
		t.Skip("tenant exposes no content categories — an app needs a categoryName")
	}
	category := cats.Results[0].DisplayName
	for _, c := range cats.Results {
		if c.DisplayName == "Uncategorized" {
			category = c.DisplayName
			break
		}
	}

	name := jscName("apply-app")
	req := &securitycloud.AppCreateRequest{
		Name:         &name,
		CategoryName: category,
		Hostnames:    &[]string{"sdk-acc-apply-app.invalid"},
		Assignments:  securitycloud.Assignments{Inclusions: securitycloud.AssignmentsInclusions{AllUsers: true}},
		Routing:      securitycloud.Routing{Type: "DIRECT"},
	}

	id, created, err := sc.ApplyZtnaAppV1(ctx, req)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ApplyZtnaAppV1 (create branch) failed: %v", err)
	}
	jscCleanupDelete(t, "ztna app "+id, func() error {
		return sc.DeleteZtnaAppV1(context.Background(), id)
	})
	if !created {
		t.Errorf("first Apply of %q reported created=false", name)
	}

	req.Hostnames = &[]string{"sdk-acc-apply-app.invalid", "sdk-acc-apply-app-2.invalid"}
	sameID, created, err := sc.ApplyZtnaAppV1(ctx, req)
	if err != nil {
		t.Fatalf("ApplyZtnaAppV1 (update branch) failed: %v", err)
	}
	if created {
		t.Error("second Apply reported created=true; it should have resolved the existing app")
	}
	if sameID != id {
		t.Errorf("second Apply returned ID %q, want %q", sameID, id)
	}
	got, err := sc.GetZtnaAppV1(ctx, id)
	if err != nil {
		t.Fatalf("GetZtnaAppV1 after Apply update failed: %v", err)
	}
	if len(got.Hostnames) != 2 {
		t.Errorf("after Apply update, app has hostnames %v; the AppCreateRequest→AppPatchRequest round-trip dropped them", got.Hostnames)
	}
	if got.Name == nil || *got.Name == "" {
		t.Error("after Apply update, app name is nil or empty; the pointer name field was lost")
	}
}

// TestAcceptance_SecurityCloudApplyZtnaGroupedGateway covers the last Apply
// whose writes are safe on a shared tenant — a grouped gateway is metadata over
// existing gateways, so creating and deleting one moves no traffic by itself.
func TestAcceptance_SecurityCloudApplyZtnaGroupedGateway(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	gateways := jscEnsureGateways(t, sc, 2)

	name := jscName("apply-grouped")
	req := &securitycloud.GroupedGatewayCreateRequest{
		Name:               name,
		GatewayIds:         []string{gateways[0].ID, gateways[1].ID},
		TenantIds:          []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		RoutingStrategy:    "NEAREST",
		RecoveryDelayInSec: 3600,
	}

	id, created, err := sc.ApplyZtnaGroupedGatewayV1(ctx, req)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ApplyZtnaGroupedGatewayV1 (create branch) failed: %v", err)
	}
	jscCleanupDelete(t, "grouped gateway "+id, func() error {
		return sc.DeleteZtnaGroupedGatewayV1(context.Background(), id)
	})
	if !created {
		t.Errorf("first Apply of %q reported created=false", name)
	}

	req.RoutingStrategy = "RANDOM"
	sameID, created, err := sc.ApplyZtnaGroupedGatewayV1(ctx, req)
	if err != nil {
		t.Fatalf("ApplyZtnaGroupedGatewayV1 (update branch) failed: %v", err)
	}
	if created {
		t.Error("second Apply reported created=true; it should have resolved the existing grouped gateway")
	}
	if sameID != id {
		t.Errorf("second Apply returned ID %q, want %q", sameID, id)
	}
	got, err := sc.GetZtnaGroupedGatewayV1(ctx, id)
	if err != nil {
		t.Fatalf("GetZtnaGroupedGatewayV1 after Apply update failed: %v", err)
	}
	if got.RoutingStrategy != "RANDOM" {
		t.Errorf("after Apply update, routingStrategy = %q, want RANDOM", got.RoutingStrategy)
	}
}

// TestAcceptance_SecurityCloudApplyZtnaGateway exercises both Apply branches.
// Gated for the same reason as the lifecycle test: the create branch provisions
// real network egress.
func TestAcceptance_SecurityCloudApplyZtnaGateway(t *testing.T) {
	jscGatewayWriteOK(t)
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	enabled := false
	req := &securitycloud.GatewayCreateRequest{
		Name:       jscName("apply-gateway"),
		Datacenter: securitycloud.GatewayCreateRequestDatacenterEuWest2,
		Enabled:    &enabled,
		TenantIds:  []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		Contact: securitycloud.GatewayContact{
			Email: "sdk-acc@example.invalid",
			Name:  "SDK acceptance",
		},
		Ipsec: &securitycloud.GatewayIpSecRequest{
			KeyExchange: "ikev2",
			Ike: securitycloud.CipherSuiteConfig{
				Encryption: []string{"aes256"}, Integrity: []string{"sha256"},
				DhGroups: []string{"modp2048"}, LifetimeInSec: 28800,
			},
			Esp: securitycloud.CipherSuiteConfig{
				Encryption: []string{"aes256"}, Integrity: []string{"sha256"},
				DhGroups: []string{"modp2048"}, LifetimeInSec: 3600,
			},
			Left: securitycloud.ConnectionConfigLeftRequest{
				ID: "sdk-acc.example.invalid", Host: "%any",
				Subnets: []string{"10.99.0.0/24"},
				Secret:  &[]string{"SdkAcceptancePsk1234"}[0],
			},
			Right: securitycloud.ConnectionConfigRightRequest{
				ID: "peer.sdk-acc.example.invalid", Host: "203.0.113.10",
				Subnets: []string{"10.98.0.0/16"},
				Vendor:  securitycloud.ConnectionConfigRightRequestVendorCisco,
			},
		},
	}

	id, created, err := sc.ApplyZtnaGatewayV1(ctx, req)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ApplyZtnaGatewayV1 create failed: %v", err)
	}
	jscCleanupDelete(t, "ztna gateway "+id, func() error {
		return sc.DeleteZtnaGatewayV1(context.Background(), id)
	})
	if !created {
		t.Errorf("first Apply reported created=false for a fresh name")
	}
	// The ID comes from the 201 body, which is the spec-declared CreateResponse
	// since build v1439 (it used to be the whole Gateway).
	if id == "" {
		t.Fatal("ApplyZtnaGatewayV1 returned an empty ID on create")
	}

	req.Datacenter = securitycloud.GatewayCreateRequestDatacenterEuWest1
	sameID, createdAgain, err := sc.ApplyZtnaGatewayV1(ctx, req)
	if err != nil {
		t.Fatalf("ApplyZtnaGatewayV1 update failed: %v", err)
	}
	if createdAgain {
		t.Errorf("second Apply reported created=true for an existing name")
	}
	if sameID != id {
		t.Errorf("second Apply returned ID %q, want %q", sameID, id)
	}
	got, err := sc.GetZtnaGatewayV1(ctx, id)
	if err != nil {
		t.Fatalf("GetZtnaGatewayV1 after Apply update failed: %v", err)
	}
	if got.Datacenter != securitycloud.GatewayDatacenterEuWest1 {
		t.Errorf("after Apply update, datacenter = %q, want eu-west-1", got.Datacenter)
	}
}

// isSecurityCloudNotFound reports whether err is a 404 from any Security Cloud
// service. It exists because the family does not speak one error dialect: DNS,
// ZTNA and UEM Connect answer the gateway's {httpStatus, traceId, errors[]}
// envelope, while device groups answer an entirely different one ({message,
// error, logref, statusCode}) that leaves Details() and FieldErrors() empty. The
// status code is the only field both of them populate.
func isSecurityCloudNotFound(err error) bool {
	var apiErr *jamfplatform.APIResponseError
	return errors.As(err, &apiErr) && apiErr.HasStatus(http.StatusNotFound)
}

// boolValue renders an optional bool for a log line without printing a pointer.
func boolValue(b *bool) bool {
	return b != nil && *b
}
