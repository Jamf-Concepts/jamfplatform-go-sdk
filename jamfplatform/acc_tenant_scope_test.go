// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

// Tenant-scope coverage — the mirror of acc_environment_scope_test.go.
//
// # Why this exists
//
// accClient prefers environment scope whenever that credential set is complete,
// which is correct: environment is the scope Jamf intends new integrations to
// use, and tenant is legacy. But the consequence is that once the environment
// secrets are configured, NOTHING in the suite sends X-Tenant-Id any more.
// WithTenantID stays public, stays supported, and is what
// terraform-provider-jamfplatform uses — so it would be a live consumer surface
// with no CI coverage, which is exactly how a legacy path breaks unnoticed.
//
// So this lane pins the scope rather than the data: accTenantClient forces
// tenant scope with no environment fallback, and the assertions below check that
// the gateway accepts X-Tenant-Id across the namespaces the SDK reaches with it.
//
// Everything here is read-only. Tenant scope is about which credential reaches
// an API, not what it may do, so nothing is gained by mutating — and this lane
// generally points at a different tenant from the pro lane, where a write would
// leave litter nobody is looking after.

import (
	"context"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAcceptance_TenantScope drives read-only operations with a credential
// pinned to tenant scope.
//
// Three of the four assertions are strict, because a tenant credential that
// cannot reach pro, Classic or devices is not an entitlement quirk — it means
// X-Tenant-Id stopped working, which is the whole point of the lane. Entitlement
// differences between products are logged instead, the same way the
// environment-scope test treats them: a 403 there says this credential is not
// granted that product, which is a permissions question rather than a scoping
// one.
func TestAcceptance_TenantScope(t *testing.T) {
	c := accTenantClient(t)
	ctx := context.Background()

	// Assert the scope the client actually settled on. Without this the test
	// could pass having silently been built from some other credential set —
	// the failure mode this lane was created to rule out.
	kind, id := c.Scope()
	if kind != jamfplatform.ScopeTenant {
		t.Fatalf("client scope is %v (id %q), want ScopeTenant — accTenantClient must never fall back to another scope", kind, id)
	}
	if id == "" {
		t.Fatal("tenant scope reported an empty ID, so no X-Tenant-Id would be sent")
	}
	t.Logf("tenant scope in use: %s", id)

	t.Run("pro reads", func(t *testing.T) {
		p := pro.New(c)
		v, err := p.GetJamfProVersionV1(ctx)
		if err != nil {
			t.Fatalf("GetJamfProVersionV1 under tenant scope: %v", err)
		}
		t.Logf("jamf-pro-version = %q", v.Version)

		if cats, err := p.ListCategoriesV1(ctx, nil, ""); err != nil {
			t.Errorf("ListCategoriesV1 under tenant scope: %v", err)
		} else {
			t.Logf("pro categories: %d", len(cats))
		}
	})

	// Classic shares the client and the header. It is included because
	// proclassic is XML end to end and reaches the gateway on a different
	// namespace, so a scoping regression could plausibly hit one and not the
	// other.
	t.Run("proclassic reads", func(t *testing.T) {
		if cats, err := proclassic.New(c).ListCategories(ctx); err != nil {
			t.Errorf("ListCategories under tenant scope: %v", err)
		} else {
			t.Logf("classic categories: %d", len(cats.Categories))
		}
	})

	// A Platform namespace rather than a Jamf Pro one: tenant scope has to work
	// across both, and devices is the one this credential is reliably granted.
	t.Run("devices reads", func(t *testing.T) {
		if d, err := devices.New(c).ListDevices(ctx, nil, ""); err != nil {
			t.Errorf("ListDevices under tenant scope: %v", err)
		} else {
			t.Logf("devices: %d", len(d))
		}
	})
}
