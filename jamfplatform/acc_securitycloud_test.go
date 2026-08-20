// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
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

func TestAcceptance_SecurityCloudDnsZoneLifecycle(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	// A zone's name servers must point at a real gateway ID; there is no
	// synthetic value the server accepts.
	gatewayPage, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
	gateways := gatewayPage.Results
	if len(gateways) == 0 {
		t.Skip("tenant has no ZTNA gateways — a DNS zone needs a gateway ID for its name servers")
	}

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
		t.Logf("app %s: name=%q category=%q hostnames=%v", a.ID, a.Name, a.CategoryName, a.Hostnames)
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
	// The spec declares 201 with no content; the server returns the full App.
	// The SDK carries a config-level responseType override for exactly this,
	// so an empty ID here means the override regressed.
	if created.ID == "" {
		t.Fatal("CreateZtnaAppV1 returned no ID — the 201 response body override has regressed")
	}
	t.Logf("created app %s name=%q category=%q", created.ID, created.Name, created.CategoryName)

	renamed := name + "-renamed"
	if err := sc.UpdateZtnaAppV1(ctx, created.ID, &securitycloud.AppPatchRequest{Name: &renamed}); err != nil {
		t.Fatalf("UpdateZtnaAppV1(%s) failed: %v", created.ID, err)
	}
	got, err := sc.GetZtnaAppV1(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetZtnaAppV1 after patch failed: %v", err)
	}
	if got.Name != renamed {
		t.Errorf("app name = %q, want %q", got.Name, renamed)
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

	gatewayPage, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
	gateways := gatewayPage.Results
	if len(gateways) < 2 {
		t.Skipf("grouped gateways require at least 2 member gateways, tenant has %d", len(gateways))
	}

	name := jscName("grouped")
	created, err := sc.CreateZtnaGroupedGatewayV1(ctx, &securitycloud.GroupedGatewayCreateRequest{
		Name:            name,
		GatewayIds:      []string{gateways[0].ID, gateways[1].ID},
		TenantIds:       []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		RoutingStrategy: "NEAREST",
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateZtnaGroupedGatewayV1 failed: %v", err)
	}
	jscCleanupDelete(t, "grouped gateway "+created.ID, func() error {
		return sc.DeleteZtnaGroupedGatewayV1(context.Background(), created.ID)
	})
	if created.ID == "" {
		t.Fatal("CreateZtnaGroupedGatewayV1 returned no ID — the 201 response body override has regressed")
	}

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

	suite := func(lifetime int) securitycloud.CypherSuiteConfig {
		return securitycloud.CypherSuiteConfig{
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
		Datacenter: "eu-west-2",
		Enabled:    &enabled,
		TenantIds:  []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		Contact: &securitycloud.GatewayContact{
			Email: "sdk-acc@example.invalid",
			Name:  "SDK acceptance",
		},
		Ipsec: &securitycloud.GatewayIpSecRequest{
			KeyExchange: "ikev2",
			Ike:         suite(28800),
			Esp:         suite(3600),
			Left: securitycloud.ConnectionConfigRequest{
				ID:      "sdk-acc.example.invalid",
				Host:    "%any",
				Subnets: []string{"10.99.0.0/24"},
				Secret:  &[]string{"SdkAcceptancePsk1234"}[0],
			},
			Right: securitycloud.ConnectionConfigRequestNoSecret{
				ID:      "peer.sdk-acc.example.invalid",
				Host:    "203.0.113.10",
				Subnets: []string{"10.98.0.0/16"},
				Vendor:  securitycloud.ConnectionConfigRequestNoSecretVendorCisco,
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

	// Wire-verified 2026-08-20: POST answers 201 with the whole Gateway, not
	// the CreateResponse the spec declares, and sends no Location header.
	// config.json carries the responseType override that encodes this.
	if created.ID == "" {
		t.Fatal("CreateZtnaGatewayV1 returned an empty ID")
	}
	if created.Name != name {
		t.Errorf("create response name = %q, want %q — the 201 body is not the full Gateway", created.Name, name)
	}
	if created.Ipsec == nil {
		t.Fatal("create response carries no ipsec block")
	}

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

	// A partial ipsec merge-patch — {"ipsec":{"esp":{...}}} — is accepted by the
	// server and deep-merges correctly (wire-verified with curl on 2026-08-20:
	// esp is replaced, keyExchange/ike/left/right are preserved). It is
	// unreachable through this SDK: GatewayPatchRequest.ipsec reuses the
	// POST-shaped GatewayIpSecRequest, whose KeyExchange/Ike/Esp/Left/Right are
	// all non-pointer because the spec marks them required, so a partial value
	// marshals keyExchange:"" and empty ike/left/right and the server answers
	// 400 "Request body is missing or malformed." The pending spec restructure
	// (GatewayIpSecPatchRequest, all-optional) is what fixes this; until it
	// lands, a caller must resend the whole ipsec block, which is what this
	// asserts. Omitting left.secret on the resend preserves the existing PSK.
	full := func(espLifetime int) *securitycloud.GatewayIpSecRequest {
		return &securitycloud.GatewayIpSecRequest{
			KeyExchange: "ikev2",
			Ike:         suite(28800),
			Esp:         suite(espLifetime),
			Left: securitycloud.ConnectionConfigRequest{
				ID: "sdk-acc.example.invalid", Host: "%any", Subnets: []string{"10.99.0.0/24"},
			},
			Right: securitycloud.ConnectionConfigRequestNoSecret{
				ID: "peer.sdk-acc.example.invalid", Host: "203.0.113.10",
				Subnets: []string{"10.98.0.0/16"},
				Vendor:  securitycloud.ConnectionConfigRequestNoSecretVendorCisco,
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

	// Pin the limitation itself, so it fails loudly the day the spec's PATCH
	// shape lands and this stops being a 400.
	partialErr := sc.UpdateZtnaGatewayV1(ctx, created.ID, &securitycloud.GatewayPatchRequest{
		Ipsec: &securitycloud.GatewayIpSecRequest{Esp: suite(3600)},
	})
	if partialErr == nil {
		t.Error("a partial GatewayIpSecRequest patch now succeeds — the spec's all-optional PATCH shape has landed, drop the full-resend workaround above")
	} else if apiErr := jamfplatform.AsAPIError(partialErr); apiErr == nil || !apiErr.HasStatus(400) {
		t.Errorf("partial ipsec patch: want 400 malformed, got %v", partialErr)
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

	suite := securitycloud.CypherSuiteConfig{
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
			Left: securitycloud.ConnectionConfigRequest{
				ID: "sdk-acc.example.invalid", Host: "%any", Subnets: []string{leftSubnet},
			},
			Right: securitycloud.ConnectionConfigRequestNoSecret{
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
		{"public left subnet", ipsec("8.8.8.0/24", securitycloud.ConnectionConfigRequestNoSecretVendorCisco), "private range"},
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

	// ListDeviceGroupsV1 is deprecated (2026-08-12) in favour of v2, but v2 is
	// not routed by the gateway yet (see TestAcceptance_SecurityCloudDeviceGroupsV2),
	// so v1 is the only version a consumer can actually call today.
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

func TestAcceptance_SecurityCloudDeviceGroupsV2(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	groups, err := sc.ListDeviceGroupsV2(ctx)
	if err != nil {
		skipOnGatewayUnrouted(t, err, "GET /v2/tenant/{id}/groups")
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

// TestAcceptance_SecurityCloudUemConnectWrites documents why every UEM Connect
// write stays unexercised.
func TestAcceptance_SecurityCloudUemConnectWrites(t *testing.T) {
	accSecurityCloudClient(t)
	t.Skip("UEM Connect writes all act on a live link to a real UEM instance and cannot be made safe on a shared tenant: CreateUemConnectorV1 needs working credentials for a separate Jamf Pro or Intune tenant (and would then start syncing its devices); DeleteUemConnectorV1 and DisableUemConnectorV1 / EnableUemConnectorV1 would tear down or toggle the tenant's existing connector; UpdateUemConnectorSyncSettingsV1 rewrites its sync configuration; TriggerUemConnectorSyncV1 and CancelUemConnectorSyncV1 start and abort a real inventory sync against the connected instance; DeployActivationProfileToUemV1 pushes an activation profile into that instance's device fleet. Exercising these needs a tenant with a disposable connector.")
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
		if apps[i].Name != "" {
			namedApp = &apps[i]
			break
		}
	}
	if namedApp == nil {
		t.Logf("no ZTNA app has a name (all %d are predefined-template apps, which return name null) — ResolveZtnaAppV1ByName is covered by the app lifecycle test instead", len(apps))
	} else {
		id, err := sc.ResolveZtnaAppV1IDByName(ctx, namedApp.Name)
		if err != nil {
			t.Fatalf("ResolveZtnaAppV1IDByName(%q) failed: %v", namedApp.Name, err)
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
	if _, err := sc.ResolveDeviceGroupV1IDByName(ctx, absent); !isSecurityCloudNotFound(err) {
		t.Errorf("resolving an absent device group: want 404 APIResponseError, got %v", err)
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
func TestAcceptance_SecurityCloudApplyDeviceGroup(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	name := jscName("apply-group")
	id, created, err := sc.ApplyDeviceGroupV1(ctx, &securitycloud.CreateGroupRequest{Name: name})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ApplyDeviceGroupV1 (create branch) failed: %v", err)
	}
	jscCleanupDelete(t, "device group "+id, func() error {
		return sc.DeleteDeviceGroupV1(context.Background(), id)
	})
	if !created {
		t.Errorf("first Apply of %q reported created=false; nothing owned that name", name)
	}
	if id == "" {
		t.Fatal("ApplyDeviceGroupV1 returned an empty ID on the create branch")
	}

	// Second Apply with the same name must resolve to the same resource and
	// take the update path — a create here would leave a duplicate behind,
	// which is the failure mode Apply exists to prevent.
	sameID, created, err := sc.ApplyDeviceGroupV1(ctx, &securitycloud.CreateGroupRequest{Name: name})
	if err != nil {
		t.Fatalf("ApplyDeviceGroupV1 (update branch) failed: %v", err)
	}
	if created {
		t.Error("second Apply reported created=true; it should have resolved the existing group")
	}
	if sameID != id {
		t.Errorf("second Apply returned ID %q, want %q", sameID, id)
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

	gatewayPage, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
	gateways := gatewayPage.Results
	if len(gateways) == 0 {
		t.Skip("tenant has no ZTNA gateways — a DNS zone needs a gateway ID for its name servers")
	}

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
	if got.Name == "" {
		t.Error("after Apply update, app name is empty; the pointer name field was lost")
	}
}

// TestAcceptance_SecurityCloudApplyZtnaGroupedGateway covers the last Apply
// whose writes are safe on a shared tenant — a grouped gateway is metadata over
// existing gateways, so creating and deleting one moves no traffic by itself.
func TestAcceptance_SecurityCloudApplyZtnaGroupedGateway(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	gatewayPage, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
	gateways := gatewayPage.Results
	if len(gateways) < 2 {
		t.Skipf("grouped gateways require at least 2 member gateways, tenant has %d", len(gateways))
	}

	name := jscName("apply-grouped")
	req := &securitycloud.GroupedGatewayCreateRequest{
		Name:            name,
		GatewayIds:      []string{gateways[0].ID, gateways[1].ID},
		TenantIds:       []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		RoutingStrategy: "NEAREST",
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
		Datacenter: "eu-west-2",
		Enabled:    &enabled,
		TenantIds:  []string{os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")},
		Contact: &securitycloud.GatewayContact{
			Email: "sdk-acc@example.invalid",
			Name:  "SDK acceptance",
		},
		Ipsec: &securitycloud.GatewayIpSecRequest{
			KeyExchange: "ikev2",
			Ike: securitycloud.CypherSuiteConfig{
				Encryption: []string{"aes256"}, Integrity: []string{"sha256"},
				DhGroups: []string{"modp2048"}, LifetimeInSec: 28800,
			},
			Esp: securitycloud.CypherSuiteConfig{
				Encryption: []string{"aes256"}, Integrity: []string{"sha256"},
				DhGroups: []string{"modp2048"}, LifetimeInSec: 3600,
			},
			Left: securitycloud.ConnectionConfigRequest{
				ID: "sdk-acc.example.invalid", Host: "%any",
				Subnets: []string{"10.99.0.0/24"},
				Secret:  &[]string{"SdkAcceptancePsk1234"}[0],
			},
			Right: securitycloud.ConnectionConfigRequestNoSecret{
				ID: "peer.sdk-acc.example.invalid", Host: "203.0.113.10",
				Subnets: []string{"10.98.0.0/16"},
				Vendor:  securitycloud.ConnectionConfigRequestNoSecretVendorCisco,
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
	// The ID comes from the 201 body, which is the full Gateway (wire-verified
	// 2026-08-20) rather than the CreateResponse the spec declares.
	if id == "" {
		t.Fatal("ApplyZtnaGatewayV1 returned an empty ID on create")
	}

	req.Datacenter = "eu-west-1"
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
	if got.Datacenter != "eu-west-1" {
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
