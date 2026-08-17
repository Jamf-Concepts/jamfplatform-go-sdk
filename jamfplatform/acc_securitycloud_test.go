// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"net/http"
	"os"
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
// Three groups of endpoints are deliberately not exercised, each for a reason
// named at the test that skips it: paths the gateway does not route yet, writes
// that would provision or reconfigure real infrastructure on a shared tenant,
// and one endpoint whose server-side delete is a confirmed no-op.

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
	gateways, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
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

	gateways, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
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

	gateways, err := sc.ListZtnaGatewaysV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListZtnaGatewaysV1 failed: %v", err)
	}
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

// TestAcceptance_SecurityCloudZtnaGatewayWrites documents why gateway
// create/update/delete are not exercised.
func TestAcceptance_SecurityCloudZtnaGatewayWrites(t *testing.T) {
	accSecurityCloudClient(t)
	t.Skip("CreateZtnaGatewayV1 / UpdateZtnaGatewayV1 / DeleteZtnaGatewayV1 provision real network egress — a gateway carries a datacenter allocation, dedicated public IPs and IPsec tunnel configuration, and deleting one severs traffic for every access policy routed through it. Exercising them needs a tenant reserved for it, not the shared acceptance tenant. Note that CreateZtnaGatewayV1's 201 response body is therefore also unverified: the spec declares no content, and unlike apps and grouped gateways (where the server was observed returning the full object) the SDK keeps the spec's shape here rather than guessing.")
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
// Activation profiles
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudActivationProfileReads(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	all, err := sc.ListActivationProfilesV1(ctx, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListActivationProfilesV1 failed: %v", err)
	}
	t.Logf("%d activation profiles", len(all.ActivationProfiles))

	// The spec marks `origin` required; the server does not (both calls above
	// and below return 200). It filters when supplied.
	apiOnly, err := sc.ListActivationProfilesV1(ctx, "PUBLIC_API")
	if err != nil {
		t.Fatalf("ListActivationProfilesV1(origin=PUBLIC_API) failed: %v", err)
	}
	t.Logf("%d of them were created via the public API", len(apiOnly.ActivationProfiles))
	if len(apiOnly.ActivationProfiles) > len(all.ActivationProfiles) {
		t.Errorf("origin-filtered list (%d) is larger than the unfiltered list (%d)",
			len(apiOnly.ActivationProfiles), len(all.ActivationProfiles))
	}

	if len(all.ActivationProfiles) == 0 {
		t.Skip("tenant has no activation profiles to fetch")
	}
	code := all.ActivationProfiles[0].Code
	got, err := sc.GetActivationProfileV1(ctx, code)
	if err != nil {
		t.Fatalf("GetActivationProfileV1(%s) failed: %v", code, err)
	}
	if got.Code != code {
		t.Errorf("GetActivationProfileV1 returned code %q, want %q", got.Code, code)
	}
}

// TestAcceptance_SecurityCloudActivationProfileLifecycle is opt-in because it
// cannot clean up after itself: see the skip message.
func TestAcceptance_SecurityCloudActivationProfileLifecycle(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	if os.Getenv("JAMFPLATFORM_JSC_ALLOW_ACTIVATION_PROFILE_CREATE") == "" {
		t.Skip("CreateActivationProfileV1 leaks an undeletable enrollment profile: POST /activation-profiles/delete-multiple answers 204 but deletes nothing — wire-verified 2026-08-17 against both a real code (still readable and still listed after two calls and a 10s wait) and a bogus one (also 204). Until that server bug is fixed, every run of this test would add a permanent activation code to the tenant. Set JAMFPLATFORM_JSC_ALLOW_ACTIVATION_PROFILE_CREATE=1 to run it anyway and clean up by hand.")
	}

	enabled := true
	// Server-side business rule, not in the spec: networkSecurity and
	// vulnerabilityManagement must both be enabled or both disabled (400
	// INVALID_FIELD on `capabilities` otherwise).
	created, err := sc.CreateActivationProfileV1(ctx, &securitycloud.PublicApiCreateActivationProfileRequest{
		Origin:    "PUBLIC_API",
		Name:      jscName("profile"),
		Platforms: []string{"iOS"},
		Capabilities: securitycloud.PublicApiCapabilities{
			NetworkSecurity:         &enabled,
			VulnerabilityManagement: &enabled,
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateActivationProfileV1 failed: %v", err)
	}
	// The spec declares {id, href}; the server returns {code}. The SDK's type
	// is repaired to match, so an empty Code means that repair regressed.
	if created.Code == "" {
		t.Fatal("CreateActivationProfileV1 returned an empty code — the ActivationProfileResponse repair has regressed")
	}
	t.Logf("created activation profile %s", created.Code)

	got, err := sc.GetActivationProfileV1(ctx, created.Code)
	if err != nil {
		t.Fatalf("GetActivationProfileV1(%s) failed: %v", created.Code, err)
	}
	if got.Code != created.Code {
		t.Errorf("GetActivationProfileV1 returned %q, want %q", got.Code, created.Code)
	}

	// Pause/resume are only ever called against a profile this test created,
	// never a pre-existing one — pausing a live enrollment profile would stop
	// real devices enrolling.
	if err := sc.PauseActivationProfileV1(ctx, created.Code); err != nil {
		skipOnGatewayUnrouted(t, err, "POST /v1/activation-profiles/{code}/pause")
		t.Fatalf("PauseActivationProfileV1(%s) failed: %v", created.Code, err)
	}
	if err := sc.ResumeActivationProfileV1(ctx, created.Code); err != nil {
		skipOnGatewayUnrouted(t, err, "POST /v1/activation-profiles/{code}/resume")
		t.Fatalf("ResumeActivationProfileV1(%s) failed: %v", created.Code, err)
	}

	// Best-effort: this is the call that answers 204 and does nothing. It is
	// still exercised so the request shape stays covered, and the leak is
	// reported rather than hidden.
	if err := sc.DeleteActivationProfilesV1(ctx, &securitycloud.BulkDeleteActivationProfilesRequest{Codes: []string{created.Code}}); err != nil {
		t.Fatalf("DeleteActivationProfilesV1 failed: %v", err)
	}
	if _, err := sc.GetActivationProfileV1(ctx, created.Code); err == nil {
		t.Logf("activation profile %s still exists after delete-multiple returned success — the known server-side no-op; remove it by hand", created.Code)
	}
}

// ---------------------------------------------------------------------------
// UEM Connect
// ---------------------------------------------------------------------------

func TestAcceptance_SecurityCloudUemConnectReads(t *testing.T) {
	sc := accSecurityCloudClient(t)
	ctx := context.Background()

	connectors, err := sc.ListUemConnectorsV1(ctx)
	if err != nil {
		skipOnGatewayUnrouted(t, err, "GET /tenant/{id}/uem-connect/v1/connectors")
		skipOnServerError(t, err)
		t.Fatalf("ListUemConnectorsV1 failed: %v", err)
	}
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

// isSecurityCloudNotFound reports whether err is a 404 from any Security Cloud
// service. It exists because the family does not speak one error dialect: DNS,
// ZTNA and UEM Connect answer the gateway's {httpStatus, traceId, errors[]}
// envelope, activation profiles answer the same shape minus traceId, and device
// groups answer an entirely different one ({message, error, logref,
// statusCode}) that leaves Details() and FieldErrors() empty. The status code is
// the only field every one of them populates.
func isSecurityCloudNotFound(err error) bool {
	var apiErr *jamfplatform.APIResponseError
	return errors.As(err, &apiErr) && apiErr.HasStatus(http.StatusNotFound)
}

// boolValue renders an optional bool for a log line without printing a pointer.
func boolValue(b *bool) bool {
	return b != nil && *b
}
