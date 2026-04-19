// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Batch 9 — security + auth surface.
//
// Most settings endpoints (cloud-azure, cloud-ldap, cloud-idp, adcs-
// settings, SSO config) require real external infrastructure to create
// — Azure AD tenants, LDAPS keystores, ADCS servers, SSO IdP metadata.
// Those CRUDs are probe-only against bogus ids or skip without a
// fixture env var. api-integrations + api-roles are self-contained
// and run full CRUD.

// --- api-authentication ------------------------------------------------

// GetAuthorizationV1 is the user-authorization-session probe. OAuth
// client-credential callers don't have a user identity and get 403
// BAD_PERMISSIONS — documented server behaviour, not an SDK fault.
func TestAcceptance_Pro_Security_GetAuthorizationV1(t *testing.T) {
	c := accClient(t)

	auth, err := pro.New(c).GetAuthorizationV1(context.Background())
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
			t.Skip("GetAuthorizationV1: 403 — endpoint requires a user-scoped token, not an OAuth client credential")
		}
		skipOnServerError(t, err)
		t.Fatalf("GetAuthorizationV1: %v", err)
	}
	t.Logf("Authorization account: %+v", auth)
}

// --- api-integrations --------------------------------------------------

func TestAcceptance_Pro_Security_ApiIntegrationCRUDV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Pick a known role. If none exist, skip — we can't attach a scope.
	roles, err := p.ListApiRolesV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListApiRolesV1: %v", err)
	}
	if len(roles) == 0 {
		t.Skip("tenant has no API roles — skipping integration CRUD")
	}
	roleName := roles[0].DisplayName

	name := "sdk-acc-api-integration-" + runSuffix()
	enabled := true
	created, err := p.CreateApiIntegrationV1(ctx, &pro.ApiIntegrationRequest{
		DisplayName:         name,
		Enabled:             &enabled,
		AuthorizationScopes: []string{roleName},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateApiIntegrationV1: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("CreateApiIntegrationV1 returned no id")
	}
	id := fmt.Sprintf("%d", created.ID)
	cleanupDelete(t, "DeleteApiIntegrationV1", func() error { return p.DeleteApiIntegrationV1(ctx, id) })
	t.Logf("Created api integration %s", id)

	got, err := p.GetApiIntegrationV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetApiIntegrationV1(%s): %v", id, err)
	}
	if got.DisplayName != name {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, name)
	}

	newName := name + "-updated"
	if _, err := p.UpdateApiIntegrationV1(ctx, id, &pro.ApiIntegrationRequest{
		DisplayName:         newName,
		Enabled:             &enabled,
		AuthorizationScopes: []string{roleName},
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateApiIntegrationV1(%s): %v", id, err)
	}

	// Rotate client credentials — generates a new secret.
	creds, err := p.RotateApiIntegrationClientCredentialsV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("RotateApiIntegrationClientCredentialsV1(%s): %v", id, err)
	}
	if creds.ClientID == "" {
		t.Error("RotateApiIntegrationClientCredentialsV1 returned no clientID")
	}

	if err := p.DeleteApiIntegrationV1(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteApiIntegrationV1(%s): %v", id, err)
	}

	_, err = p.GetApiIntegrationV1(ctx, id)
	if err == nil {
		t.Fatalf("GetApiIntegrationV1(%s) after delete should 404", id)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetApiIntegrationV1(%s) after delete: want 404, got %v", id, err)
	}
}

func TestAcceptance_Pro_Security_ListApiIntegrationsV1(t *testing.T) {
	c := accClient(t)

	items, err := pro.New(c).ListApiIntegrationsV1(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListApiIntegrationsV1: %v", err)
	}
	t.Logf("API integrations: %d", len(items))
}

// --- api-roles + api-role-privileges -----------------------------------

func TestAcceptance_Pro_Security_ApiRoleCRUDV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// Pick two real privileges — the server rejects unknown values.
	privs, err := p.ListApiRolePrivilegesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListApiRolePrivilegesV1: %v", err)
	}
	if len(privs.Privileges) < 1 {
		t.Skip("no API privileges available — skipping role CRUD")
	}
	picked := []string{privs.Privileges[0]}
	if len(privs.Privileges) > 1 {
		picked = append(picked, privs.Privileges[1])
	}

	name := "sdk-acc-api-role-" + runSuffix()
	created, err := p.CreateApiRoleV1(ctx, &pro.ApiRoleRequest{
		DisplayName: name,
		Privileges:  picked,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateApiRoleV1: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateApiRoleV1 returned no id")
	}
	id := created.ID
	cleanupDelete(t, "DeleteApiRoleV1", func() error { return p.DeleteApiRoleV1(ctx, id) })
	t.Logf("Created api role %s", id)

	got, err := p.GetApiRoleV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetApiRoleV1(%s): %v", id, err)
	}
	if got.DisplayName != name {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, name)
	}

	newName := name + "-updated"
	if _, err := p.UpdateApiRoleV1(ctx, id, &pro.ApiRoleRequest{
		DisplayName: newName,
		Privileges:  picked,
	}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateApiRoleV1(%s): %v", id, err)
	}

	if err := p.DeleteApiRoleV1(ctx, id); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteApiRoleV1(%s): %v", id, err)
	}

	_, err = p.GetApiRoleV1(ctx, id)
	if err == nil {
		t.Fatalf("GetApiRoleV1(%s) after delete should 404", id)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetApiRoleV1(%s) after delete: want 404, got %v", id, err)
	}
}

func TestAcceptance_Pro_Security_ApiRolePrivilegesV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	all, err := p.ListApiRolePrivilegesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListApiRolePrivilegesV1: %v", err)
	}
	t.Logf("API role privileges: %d", len(all.Privileges))

	sr, err := p.SearchApiRolePrivilegesV1(ctx, "Read", "5")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("SearchApiRolePrivilegesV1: %v", err)
	}
	t.Logf("API role privileges matching 'Read' (limit 5): %d", len(sr.Privileges))
}

// --- certificate-authority --------------------------------------------

func TestAcceptance_Pro_Security_CertificateAuthorityActiveV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	record, err := p.GetActiveCertificateAuthorityV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetActiveCertificateAuthorityV1: %v", err)
	}
	t.Logf("Active CA: subject=%q serial=%q", record.SubjectX500Principal, record.SerialNumber)

	der, err := p.DownloadActiveCertificateAuthorityDerV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DownloadActiveCertificateAuthorityDerV1: %v", err)
	}
	if len(der) < 100 {
		t.Errorf("DER body too small: %d bytes", len(der))
	}

	pem, err := p.DownloadActiveCertificateAuthorityPemV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DownloadActiveCertificateAuthorityPemV1: %v", err)
	}
	if !strings.Contains(string(pem), "BEGIN CERTIFICATE") {
		t.Errorf("PEM body does not look like PEM: %q", pem[:min(64, len(pem))])
	}
	t.Logf("Active CA DER %d bytes, PEM %d bytes", len(der), len(pem))
}

// --- adcs-settings (probe-only; needs real ADCS server) ---------------

func TestAcceptance_Pro_Security_AdcsSettingsProbeV1(t *testing.T) {
	c := accClient(t)

	probeID := "99999999"
	_, err := pro.New(c).GetAdcsSettingsV1(context.Background(), probeID)
	if err == nil {
		t.Logf("GetAdcsSettingsV1(%s) unexpectedly succeeded", probeID)
		return
	}
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		t.Logf("GetAdcsSettingsV1(%s): %d — plumbing OK", probeID, apiErr.StatusCode)
		return
	}
	skipOnServerError(t, err)
	t.Fatalf("GetAdcsSettingsV1(%s): %v", probeID, err)
}

// --- classic-ldap / ldap preview ---------------------------------------

func TestAcceptance_Pro_Security_LdapReadsV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	if servers, err := p.ListLdapServersV1(ctx); err == nil {
		t.Logf("LDAP servers (v1/servers): %d", len(servers))
	} else {
		skipOnServerError(t, err)
		t.Errorf("ListLdapServersV1: %v", err)
	}
	if servers, err := p.ListLdapLdapServersV1(ctx); err == nil {
		t.Logf("LDAP servers (v1/ldap-servers): %d", len(servers))
	} else {
		skipOnServerError(t, err)
		t.Errorf("ListLdapLdapServersV1: %v", err)
	}
	if servers, err := p.ListLdapServersPreview(ctx); err == nil {
		t.Logf("LDAP servers (preview): %d", len(servers))
	} else {
		skipOnServerError(t, err)
		t.Errorf("ListLdapServersPreview: %v", err)
	}
	// group search — empty query returns all; if no servers, server 4xxs.
	if _, err := p.SearchLdapGroupsV1(ctx, ""); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if !errors.As(err, &apiErr) || apiErr.StatusCode >= 500 {
			skipOnServerError(t, err)
			t.Errorf("SearchLdapGroupsV1: %v", err)
		} else {
			t.Logf("SearchLdapGroupsV1 rejected (no LDAP configured): status=%d", apiErr.StatusCode)
		}
	}
}

// --- cloud providers (probe-only; need real Azure/LDAP/IdP) -----------

func TestAcceptance_Pro_Security_CloudAzureDefaultsV1(t *testing.T) {
	c := accClient(t)

	cfg, err := pro.New(c).GetCloudAzureDefaultServerConfigurationV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetCloudAzureDefaultServerConfigurationV1: %v", err)
	}
	t.Logf("Cloud Azure defaults: %+v", cfg)
}

func TestAcceptance_Pro_Security_ListCloudIdpV1(t *testing.T) {
	c := accClient(t)

	items, err := pro.New(c).ListCloudIdpV1(context.Background(), nil)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListCloudIdpV1: %v", err)
	}
	t.Logf("Cloud IdPs: %d", len(items))
}

func TestAcceptance_Pro_Security_CloudLdapDefaultsV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	for _, provider := range []string{"GOOGLE", "AZURE"} {
		if cfg, err := p.GetCloudLdapDefaultServerConfigurationV2(ctx, provider); err == nil {
			t.Logf("Cloud LDAP defaults for %s: %+v", provider, cfg)
		} else {
			var apiErr *jamfplatform.APIResponseError
			if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
				t.Logf("GetCloudLdapDefaultServerConfigurationV2(%s): %d", provider, apiErr.StatusCode)
			} else {
				skipOnServerError(t, err)
				t.Errorf("GetCloudLdapDefaultServerConfigurationV2(%s): %v", provider, err)
			}
		}
	}
}

// --- conditional access ------------------------------------------------

func TestAcceptance_Pro_Security_ConditionalAccessFeatureToggleV1(t *testing.T) {
	c := accClient(t)

	toggle, err := pro.New(c).GetConditionalAccessFeatureToggleV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetConditionalAccessFeatureToggleV1: %v", err)
	}
	t.Logf("Conditional access feature toggle: %+v", toggle)
}

// --- csa ---------------------------------------------------------------

func TestAcceptance_Pro_Security_CsaTenantIdV1(t *testing.T) {
	c := accClient(t)

	tenant, err := pro.New(c).GetCsaTenantIdV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetCsaTenantIdV1: %v", err)
	}
	t.Logf("CSA tenant id info: %+v", tenant)
}

func TestAcceptance_Pro_Security_CsaTokenV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	_, err := p.GetCsaTokenV1(ctx)
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
			t.Logf("GetCsaTokenV1: 404 — no CSA token registered, plumbing OK")
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetCsaTokenV1: %v", err)
	}
	t.Log("CSA token registered (not logged)")
}

// --- oidc --------------------------------------------------------------

func TestAcceptance_Pro_Security_OidcPublicV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	features, err := p.GetOidcPublicFeaturesV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetOidcPublicFeaturesV1: %v", err)
	}
	t.Logf("OIDC features: %+v", features)

	// public-key may 404 when OIDC isn't enabled
	if _, err := p.GetOidcPublicKeyV1(ctx); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
			t.Logf("GetOidcPublicKeyV1: 404 — OIDC not fully configured, plumbing OK")
		} else {
			skipOnServerError(t, err)
			t.Errorf("GetOidcPublicKeyV1: %v", err)
		}
	}

	// direct-idp-login-url: 404 when not set
	if _, err := p.GetOidcDirectIdpLoginUrlV1(ctx); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
			t.Logf("GetOidcDirectIdpLoginUrlV1: 404 — not configured, plumbing OK")
		} else {
			skipOnServerError(t, err)
			t.Errorf("GetOidcDirectIdpLoginUrlV1: %v", err)
		}
	}
}

func TestAcceptance_Pro_Security_GenerateOidcCertificateV1(t *testing.T) {
	t.Skip("generates a new OIDC certificate on the tenant — skip to avoid rotating live SSO")
}

func TestAcceptance_Pro_Security_DispatchOidcLoginV2(t *testing.T) {
	t.Skip("requires a real registered email + origin URL — plumbing only if you wire those up")
}

// --- sso settings ------------------------------------------------------

func TestAcceptance_Pro_Security_SsoSettingsV3(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	settings, err := p.GetSsoSettingsV3(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSsoSettingsV3: %v", err)
	}
	t.Logf("SSO configurationType=%s", settings.ConfigurationType)

	deps, err := p.GetSsoDependenciesV3(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("GetSsoDependenciesV3: %v", err)
	} else {
		t.Logf("SSO dependencies: %+v", deps)
	}

	body, err := p.DownloadSsoMetadataV3(ctx)
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(404) {
			t.Logf("DownloadSsoMetadataV3: 404 — no metadata configured, plumbing OK")
		} else {
			t.Errorf("DownloadSsoMetadataV3: %v", err)
		}
	} else {
		t.Logf("SSO metadata: %d bytes", len(body))
	}

	hist, err := p.ListSsoHistoryV3(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("ListSsoHistoryV3: %v", err)
	} else {
		t.Logf("SSO history: %d entries", len(hist))
	}
}

func TestAcceptance_Pro_Security_UpdateSsoSettingsV3(t *testing.T) {
	t.Skip("mutating SSO settings can break login for all users — skip, round-trip in a disposable tenant only")
}

func TestAcceptance_Pro_Security_DisableSsoV3(t *testing.T) {
	t.Skip("disables SSO on the tenant — destructive, manual only")
}

// --- sso-certificate ---------------------------------------------------

func TestAcceptance_Pro_Security_SsoCertificateV2(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	_, err := p.GetSsoCertificateV2(ctx)
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && (apiErr.HasStatus(404) || apiErr.HasStatus(400)) {
			t.Logf("GetSsoCertificateV2: %d — no SSO cert configured, plumbing OK", apiErr.StatusCode)
			return
		}
		skipOnServerError(t, err)
		t.Fatalf("GetSsoCertificateV2: %v", err)
	}
	t.Log("SSO certificate present (details not logged)")

	body, err := p.DownloadSsoCertificateV2(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Errorf("DownloadSsoCertificateV2: %v", err)
	} else {
		t.Logf("SSO cert download: %d bytes", len(body))
	}
}

func TestAcceptance_Pro_Security_MutateSsoCertificateV2(t *testing.T) {
	t.Skip("PUT/POST/DELETE on SSO cert mutate live SAML flows — skip")
}

// --- sso-failover ------------------------------------------------------

func TestAcceptance_Pro_Security_SsoFailoverV1(t *testing.T) {
	c := accClient(t)

	data, err := pro.New(c).GetSsoFailoverV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 400 {
			t.Logf("GetSsoFailoverV1: 400 — no failover configured, plumbing OK")
			return
		}
		t.Fatalf("GetSsoFailoverV1: %v", err)
	}
	t.Logf("SSO failover data: %+v", data)
}

func TestAcceptance_Pro_Security_GenerateSsoFailoverV1(t *testing.T) {
	t.Skip("generates new SSO failover URL — rotates live SSO state, manual curl only")
}

// --- sso-oauth-session-tokens -----------------------------------------

func TestAcceptance_Pro_Security_ListOauthSessionTokensV1(t *testing.T) {
	c := accClient(t)

	tokens, err := pro.New(c).ListSsoOauthSessionTokensV1(context.Background())
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
			t.Skipf("ListSsoOauthSessionTokensV1: 403 — OAuth client lacks privilege for this endpoint (needs a user-scoped token)")
		}
		skipOnServerError(t, err)
		t.Fatalf("ListSsoOauthSessionTokensV1: %v", err)
	}
	t.Logf("OAuth2 session tokens: %+v", tokens)
}

// min is a tiny helper used in PEM slicing above.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
