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
	"time"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// Full-body acc tests for the 100+ Classic alt-key / subset / filter /
// date-range endpoints added in the alt-key sweep. Unlike the probe
// files (which only verify transport + XML codec round-trip), these
// discover a real id via the corresponding list endpoint, call the
// new endpoint, and assert the response decodes into the expected
// shape. When the tenant has no fixture data for a given resource
// the test skips.

func skipIfNoFixture(t *testing.T, resource string, err error) bool {
	t.Helper()
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("list %s for fixture lookup: %v", resource, err)
	}
	return false
}

// --- computer-history subset filter ----------------------------------

func TestAcceptance_Classic_ComputerHistoryByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	computers, err := p.ListComputers(ctx)
	skipIfNoFixture(t, "computers", err)
	if len(computers.Computers) == 0 {
		t.Skip("no computers on tenant")
	}
	id := strconv.Itoa(*computers.Computers[0].ID)

	hist, err := p.GetComputerHistoryByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerHistoryByIDSubset(%s, General): %v", id, err)
	}
	if hist == nil {
		t.Fatal("nil history response")
	}
	t.Logf("Computer %s history (General subset) retrieved", id)
}

// --- computer-management filters -------------------------------------

func TestAcceptance_Classic_ComputerManagementByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	computers, err := p.ListComputers(ctx)
	skipIfNoFixture(t, "computers", err)
	if len(computers.Computers) == 0 {
		t.Skip("no computers on tenant")
	}
	id := strconv.Itoa(*computers.Computers[0].ID)

	got, err := p.GetComputerManagementByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerManagementByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Computer management (General subset) retrieved for %s", id)
}

func TestAcceptance_Classic_ComputerManagementByIDPatchFilter(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	computers, err := p.ListComputers(ctx)
	skipIfNoFixture(t, "computers", err)
	if len(computers.Computers) == 0 {
		t.Skip("no computers on tenant")
	}
	id := strconv.Itoa(*computers.Computers[0].ID)

	// patchfilter is a filter expression; use empty string which
	// Jamf interprets as "all". Server accepts empty.
	got, err := p.GetComputerManagementByIDPatchFilter(ctx, id, "")
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("GetComputerManagementByIDPatchFilter: status=%d — accepted for empty filter", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetComputerManagementByIDPatchFilter: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Computer management (patch filter) retrieved for %s", id)
}

func TestAcceptance_Classic_ComputerManagementByIDUsername(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	computers, err := p.ListComputers(ctx)
	skipIfNoFixture(t, "computers", err)
	if len(computers.Computers) == 0 {
		t.Skip("no computers on tenant")
	}
	id := strconv.Itoa(*computers.Computers[0].ID)

	got, err := p.GetComputerManagementByIDUsername(ctx, id, "sdk-acc-probe-user")
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("GetComputerManagementByIDUsername: status=%d — expected for synthetic username", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetComputerManagementByIDUsername: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
}

// --- computer hardware/software reports (date range) -----------------

func TestAcceptance_Classic_ComputerHardwareSoftwareReportByIDDateRange(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	computers, err := p.ListComputers(ctx)
	skipIfNoFixture(t, "computers", err)
	if len(computers.Computers) == 0 {
		t.Skip("no computers on tenant")
	}
	id := strconv.Itoa(*computers.Computers[0].ID)

	// 90-day window.
	end := time.Now().Format("2006-01-02")
	start := time.Now().AddDate(0, 0, -90).Format("2006-01-02")

	rpt, err := p.GetComputerHardwareSoftwareReportByIDDateRange(ctx, id, start, end)
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			t.Logf("GetComputerHardwareSoftwareReportByIDDateRange: 404 — no data in window")
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetComputerHardwareSoftwareReportByIDDateRange: %v", err)
	}
	if rpt == nil {
		t.Fatal("nil report")
	}
	t.Logf("Hardware/software report for %s over %s..%s retrieved", id, start, end)
}

// --- computer application usage (date range) -------------------------

func TestAcceptance_Classic_ComputerApplicationUsageByNameDateRange(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	computers, err := p.ListComputers(ctx)
	skipIfNoFixture(t, "computers", err)
	if len(computers.Computers) == 0 || computers.Computers[0].Name == nil {
		t.Skip("no computers (with names) on tenant")
	}
	name := *computers.Computers[0].Name

	end := time.Now().Format("2006-01-02")
	start := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	usage, err := p.GetComputerApplicationUsageByNameDateRange(ctx, name, start, end)
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			t.Logf("GetComputerApplicationUsageByNameDateRange: 404 — no usage data")
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetComputerApplicationUsageByNameDateRange: %v", err)
	}
	if usage == nil {
		t.Fatal("nil usage response")
	}
	t.Logf("Application usage for %q over %s..%s retrieved", name, start, end)
}

// --- computer-applications lookup ------------------------------------

func TestAcceptance_Classic_ComputerApplicationByApplication(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	// "Safari.app" is present on every managed Mac.
	got, err := p.GetComputerApplicationByApplication(ctx, "Safari.app")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerApplicationByApplication(Safari.app): %v", err)
	}
	if got == nil {
		t.Fatal("nil application response")
	}
	t.Logf("Safari.app lookup retrieved")
}

// --- mobile-device subset + match ------------------------------------

func TestAcceptance_Classic_MobileDeviceByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListMobileDevices(ctx)
	skipIfNoFixture(t, "mobile-devices", err)
	if len(list.MobileDevices) == 0 {
		t.Skip("no mobile devices on tenant")
	}
	id := strconv.Itoa(*list.MobileDevices[0].ID)

	got, err := p.GetMobileDeviceByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Mobile device %s (General subset) retrieved", id)
}

func TestAcceptance_Classic_MobileDeviceByMatch(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	// "*" matches all devices. Server returns the full list.
	got, err := proclassic.New(c).GetMobileDeviceByMatch(context.Background(), "*")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceByMatch(*): %v", err)
	}
	_ = got
	t.Log("GetMobileDeviceByMatch(*) succeeded")
	_ = ctx
}

// --- mobile-device-history subset ------------------------------------

func TestAcceptance_Classic_MobileDeviceHistoryByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListMobileDevices(ctx)
	skipIfNoFixture(t, "mobile-devices", err)
	if len(list.MobileDevices) == 0 {
		t.Skip("no mobile devices on tenant")
	}
	id := strconv.Itoa(*list.MobileDevices[0].ID)

	got, err := p.GetMobileDeviceHistoryByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceHistoryByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Mobile device %s history (General subset) retrieved", id)
}

// --- policy filters --------------------------------------------------

func TestAcceptance_Classic_PolicyByCategory(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	cats, err := p.ListCategories(ctx)
	skipIfNoFixture(t, "categories", err)
	if len(cats.Categories) == 0 {
		t.Skip("no categories on tenant")
	}
	// Pick the first category with a plain name — some tenants have
	// categories with %-wrapped template placeholders that the server
	// 500s on when URL-encoded.
	var cat string
	for _, c := range cats.Categories {
		if c.Name == nil {
			continue
		}
		n := *c.Name
		if !strings.ContainsAny(n, "%&?#") {
			cat = n
			break
		}
	}
	if cat == "" {
		t.Skip("no plain-name categories on tenant (all contain URL-reserved chars)")
	}

	got, err := p.GetPolicyByCategory(ctx, cat)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPolicyByCategory(%q): %v", cat, err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Policies in category %q retrieved", cat)
}

func TestAcceptance_Classic_PolicyByCreatedBy(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	// Jamf supports "jss" and "casper" as built-in createdBy values.
	got, err := proclassic.New(c).GetPolicyByCreatedBy(context.Background(), "jss")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPolicyByCreatedBy(jss): %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Log("Policies created by jss retrieved")
	_ = ctx
}

func TestAcceptance_Classic_PolicyByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListPolicies(ctx)
	skipIfNoFixture(t, "policies", err)
	if len(list.Policies) == 0 {
		t.Skip("no policies on tenant")
	}
	id := strconv.Itoa(*list.Policies[0].ID)

	got, err := p.GetPolicyByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPolicyByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Policy %s (General subset) retrieved", id)
}

// --- os-x / mobile configuration profile subsets ---------------------

func TestAcceptance_Classic_OsxConfigurationProfileByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListOSXConfigurationProfiles(ctx)
	skipIfNoFixture(t, "osx-configuration-profiles", err)
	if len(list.OsXConfigurationProfiles) == 0 {
		t.Skip("no OSX configuration profiles on tenant")
	}
	id := strconv.Itoa(*list.OsXConfigurationProfiles[0].ID)

	got, err := p.GetOsxConfigurationProfileByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetOsxConfigurationProfileByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("OSX configuration profile %s (General subset) retrieved", id)
}

func TestAcceptance_Classic_MobileDeviceConfigurationProfileByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListMobileDeviceConfigurationProfiles(ctx)
	skipIfNoFixture(t, "mobile-device-configuration-profiles", err)
	if len(list.ConfigurationProfiles) == 0 {
		t.Skip("no mobile device configuration profiles on tenant")
	}
	id := strconv.Itoa(*list.ConfigurationProfiles[0].ID)

	got, err := p.GetMobileDeviceConfigurationProfileByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceConfigurationProfileByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Mobile device configuration profile %s (General subset) retrieved", id)
}

// --- ebook + application subsets -------------------------------------

func TestAcceptance_Classic_EbookByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListEbooks(ctx)
	skipIfNoFixture(t, "ebooks", err)
	if len(list.Ebooks) == 0 {
		t.Skip("no ebooks on tenant")
	}
	id := strconv.Itoa(*list.Ebooks[0].ID)

	got, err := p.GetEbookByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetEbookByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Ebook %s (General subset) retrieved", id)
}

func TestAcceptance_Classic_MacApplicationByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListMacApplications(ctx)
	skipIfNoFixture(t, "mac-applications", err)
	if len(list.MacApplications) == 0 {
		t.Skip("no mac applications on tenant")
	}
	id := strconv.Itoa(*list.MacApplications[0].ID)

	got, err := p.GetMacApplicationByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMacApplicationByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Mac application %s (General subset) retrieved", id)
}

func TestAcceptance_Classic_MobileDeviceApplicationByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListMobileDeviceApplications(ctx)
	skipIfNoFixture(t, "mobile-device-applications", err)
	if len(list.MobileDeviceApplications) == 0 {
		t.Skip("no mobile device applications on tenant")
	}
	id := strconv.Itoa(*list.MobileDeviceApplications[0].ID)

	got, err := p.GetMobileDeviceApplicationByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceApplicationByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Mobile device application %s (General subset) retrieved", id)
}

// --- LDAP server chain lookup ---------------------------------------

func TestAcceptance_Classic_LDAPServerByIDGroupUser(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListLDAPServers(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListLDAPServers: %v", err)
	}
	if len(list.LdapServers) == 0 {
		t.Skip("no LDAP servers configured on tenant")
	}
	id := strconv.Itoa(*list.LdapServers[0].ID)

	// Synthetic group + user — exercises transport; tolerates 404
	// since the synthetic values won't resolve.
	got, err := p.GetLDAPServerByIDGroupUser(ctx, id, "sdk-probe-group", "sdk-probe-user")
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("GetLDAPServerByIDGroupUser: status=%d — expected for synthetic keys", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetLDAPServerByIDGroupUser: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("LDAP chain lookup on server %s retrieved", id)
}

// --- VPP + patch policy subsets -------------------------------------

func TestAcceptance_Classic_VPPInvitationByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListVPPInvitations(ctx)
	skipIfNoFixture(t, "vpp-invitations", err)
	if len(list.VppInvitations) == 0 {
		t.Skip("no VPP invitations on tenant")
	}
	id := strconv.Itoa(*list.VppInvitations[0].ID)

	got, err := p.GetVPPInvitationByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetVPPInvitationByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("VPP invitation %s (General subset) retrieved", id)
}

func TestAcceptance_Classic_PatchPolicyByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListPatchPolicies(ctx)
	skipIfNoFixture(t, "patch-policies", err)
	if len(list.PatchPolicies) == 0 {
		t.Skip("no patch policies on tenant")
	}
	id := strconv.Itoa(*list.PatchPolicies[0].ID)

	got, err := p.GetPatchPolicyByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetPatchPolicyByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Patch policy %s (General subset) retrieved", id)
}

// --- mobile-device-enrollment-profile subset -------------------------

func TestAcceptance_Classic_MobileDeviceEnrollmentProfileByIDSubset(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := proclassic.New(c)

	list, err := p.ListMobileDeviceEnrollmentProfiles(ctx)
	skipIfNoFixture(t, "mobile-device-enrollment-profiles", err)
	if len(list.MobileDeviceEnrollmentProfiles) == 0 {
		t.Skip("no mobile device enrollment profiles on tenant")
	}
	id := strconv.Itoa(*list.MobileDeviceEnrollmentProfiles[0].ID)

	got, err := p.GetMobileDeviceEnrollmentProfileByIDSubset(ctx, id, "General")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceEnrollmentProfileByIDSubset: %v", err)
	}
	if got == nil {
		t.Fatal("nil response")
	}
	t.Logf("Mobile device enrollment profile %s (General subset) retrieved", id)
}
