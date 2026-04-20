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

// Batch 14 — PKI integrations (digicert + venafi). Both require external
// CA infrastructure (DigiCert TLM or Venafi TPP with valid
// client-certs/refresh-tokens). Tenants without a fixture will 404 on
// GET-by-id and 400 on create without valid credentials. Tests probe
// the happy path, fall back to 4xx-tolerance, and fail only on 5xx or
// client bugs.

// --- digicert ---------------------------------------------------------

func TestAcceptance_Pro_PKI_DigicertTLMProbe(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// The API exposes no list endpoint — callers must know the id.
	// Probing with a known-bad id proves routing works; a real fixture
	// would be needed to exercise the full lifecycle.
	_, err := p.GetDigicertTrustLifecycleManagerV1(ctx, "-1")
	if err == nil {
		t.Log("GetDigicertTrustLifecycleManagerV1(-1) unexpectedly succeeded")
		return
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 404 {
			t.Logf("DigiCert TLM not configured on this tenant (404) — expected")
			return
		}
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("DigiCert TLM probe rejected: status=%d", apiErr.StatusCode)
			return
		}
	}
	skipOnServerError(t, err)
	t.Fatalf("GetDigicertTrustLifecycleManagerV1: %v", err)
}

func TestAcceptance_Pro_PKI_DigicertValidateCertificate(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Endpoint returns 204 when the payload's certificate is a valid
	// DigiCert client cert. With an empty payload the server will
	// reject with 4xx — that's expected; we only guard against 5xx and
	// transport failures.
	err := p.ValidateDigicertClientCertificateV1(ctx, &pro.Certificate{
		Filename: "probe.p12",
		Data:     [][]byte{},
	})
	if err == nil {
		t.Log("ValidateDigicertClientCertificateV1 accepted empty payload")
		return
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("ValidateDigicertClientCertificateV1 rejected: status=%d — expected without a real DigiCert fixture", apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Fatalf("ValidateDigicertClientCertificateV1: %v", err)
}

// --- venafi -----------------------------------------------------------

func TestAcceptance_Pro_PKI_VenafiProbe(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// No list endpoint. Probe with a known-bad id to verify routing.
	if _, err := p.GetVenafiV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("Venafi probe: status=%d — expected on tenants without a Venafi CA", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetVenafiV1(-1): %v", err)
		}
	}

	// Connection-status and dependent-profiles follow the same probe
	// pattern.
	if _, err := p.GetVenafiConnectionStatusV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("Venafi connection-status probe: status=%d", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetVenafiConnectionStatusV1: %v", err)
		}
	}

	if _, err := p.GetVenafiDependentProfilesV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("Venafi dependent-profiles probe: status=%d", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetVenafiDependentProfilesV1: %v", err)
		}
	}

	if _, err := p.GetVenafiJamfPublicKeyV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("Venafi jamf-public-key probe: status=%d", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetVenafiJamfPublicKeyV1: %v", err)
		}
	}

	if _, err := p.GetVenafiProxyTrustStoreV1(ctx, "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("Venafi proxy-trust-store probe: status=%d", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetVenafiProxyTrustStoreV1: %v", err)
		}
	}
}

func TestAcceptance_Pro_PKI_VenafiLifecycle(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// The server accepts a minimal name-only payload — refresh-token
	// validation is deferred to actual TPP calls. Exercise the full
	// CRUD lifecycle against a placeholder record; cleanup deletes it.
	name := "sdk-acc-venafi-" + runSuffix()
	created, err := p.CreateVenafiV1(ctx, &pro.VenafiCaRecord{Name: name})
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("CreateVenafiV1 rejected: status=%d — tenant may require richer payload", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("CreateVenafiV1: %v", err)
	}
	id := created.ID
	t.Logf("Created Venafi CA record id=%s", id)
	cleanupDelete(t, "Venafi "+id, func() error { return p.DeleteVenafiV1(ctx, id) })

	got, err := p.GetVenafiV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetVenafiV1(%s): %v", id, err)
	}
	if got.Name != name {
		t.Errorf("name round-trip mismatch: got %q, want %q", got.Name, name)
	}

	// PATCH rejects response-only fields (refreshTokenConfigured).
	// Send the minimal writable subset instead of echoing the full GET.
	if _, err := p.UpdateVenafiV1(ctx, id, &pro.VenafiCaRecord{Name: name + "-upd"}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateVenafiV1: %v", err)
	}

	if _, err := p.CreateVenafiHistoryNoteV1(ctx, id, &pro.ObjectHistoryNote{
		Note: "sdk-acc test venafi history entry",
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateVenafiHistoryNoteV1: %v", err)
	}

	hist, err := p.ListVenafiHistoryV1(ctx, id, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListVenafiHistoryV1: %v", err)
	}
	t.Logf("Venafi history: %d entries", len(hist))
}
