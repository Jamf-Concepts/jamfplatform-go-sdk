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

// Batch 20 — accounts + user-preferences + notifications + dashboard
// + account-preferences v3. Accounts exercise full CRUD against an
// ephemeral user. Preferences round-trip under a sdk-acc-* key so
// real user state is untouched. Dashboard toggle is destructive —
// probed with bogus objectId, tolerating 4xx.

// --- accounts --------------------------------------------------------

// POST /v1/accounts needs ldapServerId=-1 + distinguishedName="" as
// sentinels even for non-LDAP accounts — omit them and the server
// 500s with an empty errors array (null-deref in the LDAP lookup
// path). Same for phone + changePasswordOnNextLogin; the schema
// marks them optional but the create handler deref's them.
func TestAcceptance_Pro_AccountsV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	existing, err := p.ListAccountsV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAccountsV1: %v", err)
	}
	t.Logf("Accounts: %d existing", len(existing))

	uname := "sdk-acc-user-" + runSuffix()
	realname := "SDK Acc Test " + runSuffix()
	email := uname + "@example.invalid"
	password := "SDKAccTestPwd!" + runSuffix()
	// Enum values from the spec — accessLevel is PascalCase, the
	// others are SCREAMING_SNAKE. UI labels like "Full Access" /
	// "Custom" / "Standard" will 400.
	accessLevel := "FullAccess"
	privilegeLevel := "ADMINISTRATOR"
	accountStatus := "Enabled"
	phone := "000-000-0000"
	distinguishedName := ""
	siteID := -1
	ldapServerID := -1
	changePassword := false

	created, err := p.CreateAccountV1(ctx, &pro.UserAccount{
		Username:                  &uname,
		Realname:                  &realname,
		Email:                     &email,
		Phone:                     &phone,
		PlainPassword:             &password,
		LdapServerID:              &ldapServerID,
		DistinguishedName:         &distinguishedName,
		SiteID:                    &siteID,
		AccessLevel:               &accessLevel,
		PrivilegeLevel:            &privilegeLevel,
		AccountStatus:             &accountStatus,
		ChangePasswordOnNextLogin: &changePassword,
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateAccountV1: %v", err)
	}
	if created.ID == nil {
		t.Fatalf("CreateAccountV1: nil ID on response")
	}
	id := *created.ID
	t.Logf("Created account id=%s username=%s", id, uname)
	cleanupDelete(t, "Account "+id, func() error { return p.DeleteAccountV1(ctx, id) })

	got, err := p.GetAccountV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAccountV1: %v", err)
	}
	if got.Username == nil || *got.Username != uname {
		t.Errorf("username round-trip mismatch: got %v, want %q", got.Username, uname)
	}

	// Update the realname and round-trip. The PUT body mirrors the
	// create body — server rejects partial PUTs that omit the LDAP
	// sentinels even though the schema marks them optional.
	updatedRealname := realname + " (updated)"
	got.Realname = &updatedRealname
	updated, err := p.UpdateAccountV1(ctx, id, got)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateAccountV1: %v", err)
	}
	if updated.Realname == nil || *updated.Realname != updatedRealname {
		t.Errorf("realname update mismatch: got %v, want %q", updated.Realname, updatedRealname)
	}
}

// --- account-groups --------------------------------------------------

// New in 11.28.0. Read-only — list, then GET first by id if any exist.
// Exercises the filtered resolver when a name is available.
func TestAcceptance_Pro_AccountGroupsV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	groups, err := p.ListAccountGroupsV1(ctx, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListAccountGroupsV1: %v", err)
	}
	t.Logf("AccountGroupsV1: %d existing", len(groups))

	if len(groups) == 0 {
		t.Skip("no account groups present — skip detail + resolver checks")
	}

	first := groups[0]
	if first.ID == "" {
		t.Fatalf("ListAccountGroupsV1: first group has empty ID")
	}
	id := first.ID
	got, err := p.GetAccountGroupV1(ctx, id)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAccountGroupV1(%s): %v", id, err)
	}
	if got.ID != id {
		t.Errorf("GetAccountGroupV1: ID mismatch got=%s want=%s", got.ID, id)
	}

	if first.Name == "" {
		return
	}
	name := first.Name
	resolvedID, err := p.ResolveAccountGroupV1IDByName(ctx, name)
	if err != nil {
		skipOnServerError(t, err)
		t.Logf("ResolveAccountGroupV1IDByName(%q) failed — likely the list endpoint doesn't support RSQL on name; switch resolver to clientFilter if reproducible: %v", name, err)
		return
	}
	if resolvedID != id {
		t.Errorf("ResolveAccountGroupV1IDByName(%q): resolved %s, want %s", name, resolvedID, id)
	}
}

// --- user session ----------------------------------------------------

func TestAcceptance_Pro_UserSessionV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	// GetUserSessionV1 returns the list of accounts bound to the
	// current token's identity.
	accts, err := p.GetUserSessionV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetUserSessionV1: %v", err)
	}
	t.Logf("User session: %d bound accounts", len(accts))

	// Round-trip updateSession with an empty Session body — servers
	// accept this as a no-op that re-stamps the session timestamp.
	if _, err := p.UpdateUserSessionV1(ctx, &pro.Session{}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateUserSessionV1: %v", err)
	}
}

// --- user preferences ------------------------------------------------

func TestAcceptance_Pro_UserPreferencesV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	key := "sdk-acc-pref-" + runSuffix()

	// Settings endpoint returns schema/metadata for a key.
	if _, err := p.GetUserPreferencesSettingsV1(ctx, key); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("GetUserPreferencesSettingsV1(%s): status=%d — expected for unknown key", key, apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("GetUserPreferencesSettingsV1: %v", err)
		}
	}

	// Write + read + delete cycle. Request type is map[string]any —
	// store an arbitrary JSON object against our sdk-acc key.
	payload := map[string]any{"sdkAccTest": true, "ts": runSuffix()}
	if _, err := p.UpdateUserPreferenceV1(ctx, key, &payload); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateUserPreferenceV1: %v", err)
	}
	cleanupDelete(t, "UserPreference "+key, func() error { return p.DeleteUserPreferenceV1(ctx, key) })

	got, err := p.GetUserPreferenceV1(ctx, key)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetUserPreferenceV1: %v", err)
	}
	t.Logf("User preference %q round-tripped: %+v", key, got)
}

// --- notifications ---------------------------------------------------

// Notifications list returns system-posted items; we don't mutate
// them here to avoid wiping legitimate notices. The DELETE endpoint
// is probed with clearly-synthetic values and tolerates 4xx.
func TestAcceptance_Pro_NotificationsV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	items, err := p.ListNotificationsV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListNotificationsV1: %v", err)
	}
	t.Logf("Notifications: %d", len(items))

	if err := p.DeleteNotificationV1(ctx, "sdk-acc-fake-type", "-1"); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("DeleteNotificationV1 probe: status=%d — expected for bogus id/type", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("DeleteNotificationV1: %v", err)
		}
	}
}

// --- dashboard -------------------------------------------------------

func TestAcceptance_Pro_DashboardV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	setup, err := p.GetDashboardV1(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetDashboardV1: %v", err)
	}
	t.Logf("Dashboard setup retrieved: %+v", setup)

	// Toggle needs a real objectId — probe with bogus values and
	// tolerate 4xx so we don't flip live widgets.
	if _, err := p.ToggleDashboardObjectV1(ctx, &pro.DashboardObject{
		Enabled:    false,
		ObjectID:   "sdk-acc-fake",
		ObjectType: "sdk-acc-fake",
	}); err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			t.Logf("ToggleDashboardObjectV1 probe: status=%d — expected for bogus objectId", apiErr.StatusCode)
		} else {
			skipOnServerError(t, err)
			t.Fatalf("ToggleDashboardObjectV1: %v", err)
		}
	}
}

// --- account preferences v3 -----------------------------------------

func TestAcceptance_Pro_AccountPreferencesV3(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	current, err := p.GetAccountPreferencesV3(ctx)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetAccountPreferencesV3: %v", err)
	}
	t.Logf("Account preferences v3 retrieved")

	if err := p.UpdateAccountPreferencesV3(ctx, current); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateAccountPreferencesV3 round-trip: %v", err)
	}
}
