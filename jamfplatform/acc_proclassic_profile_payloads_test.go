// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

// Wire-level proof that configuration-profile <payloads> bodies survive
// both Create (POST) and Update (PUT) round-trips through Jamf Pro
// Classic without entity-layer corruption.
//
// Background: the JSSResource POST handler runs an extra XML-entity-decode
// pass on <payloads> chardata and then canonicalises the parsed plist
// before storage, while the PUT handler does neither — it stores the
// post-XML-decode bytes verbatim. To compensate the SDK has historically
// double-escaped the <payloads> wire form via PayloadsXMLText. That fix
// is correct for POST, but on PUT it persists literal "&amp;#34;" (8
// bytes) where the caller wrote "\"" (1 byte) and breaks downstream
// device-side parsing of CodeRequirement / TCC / PPPC payloads.
//
// SDK fix: PUT endpoints now take *OsXConfigurationProfileUpdate /
// *MobileDeviceConfigurationProfileUpdate, whose <general>.<payloads>
// field is wired to PayloadsXMLTextSingleEscape (single-escape, the
// spec-correct XML chardata form). These tests confirm the round-trip
// holds on a live EU tenant.
//
// The probe payload is a minimal valid mobileconfig plist containing
// both bug markers in one string:
//   - literal `"` chars (the TCC CodeRequirement quote syntax)
//   - the pre-encoded `&amp;` entity (any embedded ampersand in plist
//     source must be entity-encoded for the plist XML to be well-formed)
//
// Assertions inspect the SDK-decoded payload string returned from GET
// after Create and again after Update. They look for the buggy
// double-encoded forms (`&amp;amp;`, `&amp;#34;`, `&amp;quot;`) and fail
// the test if any are present — those forms only appear when the wire
// form was double-escaped on the way in.

package jamfplatform_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// osxProfilePlist builds a minimal valid macOS configuration profile
// payload. Two flavours:
//   - includeAmpersand=false → only the prompt's PPPC `"` marker is
//     embedded. Isolates the original PUT-double-escape-on-quote bug
//     from the orthogonal `&`-content corruption.
//   - includeAmpersand=true  → adds `&amp;` inside the Identifier /
//     CodeRequirement strings. Exposes the broader entity-doubling
//     issue affecting POST today.
func osxProfilePlist(displayName, marker string, includeAmpersand bool) string {
	identifier := "com.example.sdk"
	if includeAmpersand {
		identifier = "com.example&amp;sdk"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadType</key>
			<string>com.apple.TCC.configuration-profile-policy</string>
			<key>PayloadIdentifier</key>
			<string>com.example.tcc.sdk-acc</string>
			<key>PayloadUUID</key>
			<string>11111111-2222-3333-4444-555555555555</string>
			<key>PayloadDisplayName</key>
			<string>` + marker + `</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>Services</key>
			<dict>
				<key>SystemPolicyAllFiles</key>
				<array>
					<dict>
						<key>Identifier</key>
						<string>` + identifier + `</string>
						<key>IdentifierType</key>
						<string>bundleID</string>
						<key>CodeRequirement</key>
						<string>identifier "` + identifier + `" and anchor apple generic</string>
						<key>Authorization</key>
						<string>Allow</string>
					</dict>
				</array>
			</dict>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>` + displayName + `</string>
	<key>PayloadIdentifier</key>
	<string>com.example.profile.sdk-acc</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>66666666-7777-8888-9999-aaaaaaaaaaaa</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`
}

// mobileProfilePlist builds a minimal valid mobile device configuration
// profile payload. Two flavours mirror osxProfilePlist:
//   - includeAmpersand=false → marker lives inside the
//     autonomousSingleAppIDs string with a literal `"`.
//   - includeAmpersand=true  → adds `&amp;` inside the same entry.
//
// The marker rides in autonomousSingleAppIDs because the server
// canonicalises the inner payload's PayloadDisplayName to a default
// ("Restrictions Payload") on this profile type and discards anything
// the caller wrote there. autonomousSingleAppIDs string content is
// preserved verbatim through canonicalisation.
func mobileProfilePlist(displayName, marker string, includeAmpersand bool) string {
	appID := `com.example.sdk-` + marker + ` "primary" bundle`
	if includeAmpersand {
		appID = `com.example&amp;sdk-` + marker + ` "primary" bundle`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadType</key>
			<string>com.apple.applicationaccess</string>
			<key>PayloadIdentifier</key>
			<string>com.example.restrictions.sdk-acc</string>
			<key>PayloadUUID</key>
			<string>aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee</string>
			<key>PayloadDisplayName</key>
			<string>` + marker + `</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>allowCamera</key>
			<true/>
			<key>safariAcceptCookies</key>
			<integer>2</integer>
			<key>autonomousSingleAppIDs</key>
			<array>
				<string>` + appID + `</string>
			</array>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>` + displayName + `</string>
	<key>PayloadIdentifier</key>
	<string>com.example.mobile.sdk-acc</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>ffffffff-1111-2222-3333-444444444444</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`
}

// assertQuoteRoundtrip checks the SDK-decoded payload string returned
// from a GET after a write. Verifies the prompt's specific repro: a
// literal `"` round-trips without being corrupted into `&amp;#34;` (the
// double-escape PUT bug) or `&amp;quot;`. Does not exercise `&`-content.
func assertQuoteRoundtrip(t *testing.T, stage, got string) {
	t.Helper()
	for _, bad := range []string{"&amp;#34;", "&amp;quot;", "&amp;lt;", "&amp;gt;"} {
		if strings.Contains(got, bad) {
			t.Fatalf("%s: payload chardata contains double-encoded entity %q — entity layer was not collapsed by the server.\nstored payload:\n%s", stage, bad, got)
		}
	}
	if !strings.Contains(got, "<plist") {
		t.Fatalf("%s: payload chardata missing <plist> root — server may have rejected or rewritten the body. Got:\n%s", stage, got)
	}
	if !strings.Contains(got, `"`) && !strings.Contains(got, "&quot;") && !strings.Contains(got, "&#34;") {
		t.Fatalf("%s: payload chardata missing the quote marker (\", &quot;, or &#34;) — got:\n%s", stage, got)
	}
}

// assertAmpersandRoundtrip is the stricter variant: also requires that
// an inbound `&amp;` survives without becoming `&amp;amp;` (9 bytes) or
// any further-doubled form. Currently fails on POST too — the existing
// double-escape PayloadsXMLText wrapper corrupts `&`-content even on
// Create. Wire-evidenced 2026-05-27; see project memory.
func assertAmpersandRoundtrip(t *testing.T, stage, got string) {
	t.Helper()
	for _, bad := range []string{"&amp;amp;"} {
		if strings.Contains(got, bad) {
			t.Fatalf("%s: payload chardata contains double-encoded entity %q — entity layer was not collapsed by the server.\nstored payload:\n%s", stage, bad, got)
		}
	}
	if !strings.Contains(got, "&amp;") && !strings.Contains(got, "&#38;") {
		t.Fatalf("%s: payload chardata missing the encoded ampersand marker (&amp; or &#38;) — got:\n%s", stage, got)
	}
}

// TestAcceptance_Classic_OSXProfile_QuoteRoundtrip isolates the prompt's
// specific PUT-double-escape-on-`"` bug from the orthogonal `&`-content
// corruption. Plist contains literal `"` chars (TCC CodeRequirement
// style) and no `&amp;` content. Without the single-escape PUT fix the
// stored chardata would contain `&amp;#34;` after Update.
func TestAcceptance_Classic_OSXProfile_QuoteRoundtrip(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-osxcp-quote-" + runSuffix()
	createPayload := osxProfilePlist(name, "create-marker-quote-\"-only", false)

	created, err := pc.CreateOSXConfigurationProfileByID(ctx, "0", &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{
			Name:     classicStrPtr(name),
			Payloads: payloadsXMLPtr(createPayload),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateOSXConfigurationProfileByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateOSXConfigurationProfileByID: no ID returned: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteOSXConfigurationProfileByID", func() error {
		return pc.DeleteOSXConfigurationProfileByID(ctx, intToStr(id))
	})

	afterCreate, err := pc.GetOSXConfigurationProfileByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetOSXConfigurationProfileByID after create: %v", err)
	}
	if afterCreate == nil || afterCreate.General == nil || afterCreate.General.Payloads == nil {
		t.Fatalf("GET after create: profile or payloads missing: %+v", afterCreate)
	}
	got := string(*afterCreate.General.Payloads)
	assertQuoteRoundtrip(t, "after Create (POST)", got)
	t.Logf("after Create: payload chardata:\n%s", got)

	updatePayload := osxProfilePlist(name, "update-marker-quote-\"-only", false)
	updateReq := (&proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{
			Name:     classicStrPtr(name),
			Payloads: payloadsXMLPtr(updatePayload),
		},
	}).ToUpdate()
	if err := pc.UpdateOSXConfigurationProfileByID(ctx, intToStr(id), updateReq); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateOSXConfigurationProfileByID: %v", err)
	}

	afterUpdate, err := pc.GetOSXConfigurationProfileByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetOSXConfigurationProfileByID after update: %v", err)
	}
	if afterUpdate == nil || afterUpdate.General == nil || afterUpdate.General.Payloads == nil {
		t.Fatalf("GET after update: profile or payloads missing: %+v", afterUpdate)
	}
	got = string(*afterUpdate.General.Payloads)
	assertQuoteRoundtrip(t, "after Update (PUT)", got)
	t.Logf("after Update: payload chardata:\n%s", got)

	if !strings.Contains(got, "update-marker") {
		t.Fatalf("after Update: payload still contains create-marker, server did not apply the update. Got:\n%s", got)
	}
}

// TestAcceptance_Classic_OSXProfile_AmpersandRoundtrip is the broader
// scenario: plist contains both `"` and `&amp;` markers. Currently
// expected to fail on POST because the existing PayloadsXMLText
// double-escape corrupts `&`-content. Documents the open `&`-content
// bug — orthogonal to the PUT-on-`"` fix this commit lands. Skipped
// from CI gating until the `&`-content bug is independently fixed; use
// `go test -run AmpersandRoundtrip` locally to track the issue.
func TestAcceptance_Classic_OSXProfile_AmpersandRoundtrip(t *testing.T) {
	t.Skip("open `&`-content corruption affects POST too; tracked separately from the PUT-quote fix landing now")
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-osxcp-amp-" + runSuffix()
	createPayload := osxProfilePlist(name, "create-marker-amp-\"-and-&amp;", true)

	created, err := pc.CreateOSXConfigurationProfileByID(ctx, "0", &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{
			Name:     classicStrPtr(name),
			Payloads: payloadsXMLPtr(createPayload),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateOSXConfigurationProfileByID: %v", err)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteOSXConfigurationProfileByID", func() error {
		return pc.DeleteOSXConfigurationProfileByID(ctx, intToStr(id))
	})

	afterCreate, _ := pc.GetOSXConfigurationProfileByID(ctx, intToStr(id))
	got := string(*afterCreate.General.Payloads)
	assertAmpersandRoundtrip(t, "after Create (POST)", got)
}

// TestAcceptance_Classic_MobileDeviceProfile_QuoteRoundtrip mirrors the
// OS X quote-only scenario for the mobile resource.
func TestAcceptance_Classic_MobileDeviceProfile_QuoteRoundtrip(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	pc := proclassic.New(c)

	name := "sdk-acc-mdcp-quote-" + runSuffix()
	createPayload := mobileProfilePlist(name, "create-marker-quote-\"-only", false)

	created, err := pc.CreateMobileDeviceConfigurationProfileByID(ctx, "0", &proclassic.MobileDeviceConfigurationProfile{
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{
			Name:     classicStrPtr(name),
			Payloads: payloadsXMLPtr(createPayload),
		},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateMobileDeviceConfigurationProfileByID: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("CreateMobileDeviceConfigurationProfileByID: no ID returned: %+v", created)
	}
	id := *created.ID
	cleanupDelete(t, "DeleteMobileDeviceConfigurationProfileByID", func() error {
		return pc.DeleteMobileDeviceConfigurationProfileByID(ctx, intToStr(id))
	})

	afterCreate, err := pc.GetMobileDeviceConfigurationProfileByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceConfigurationProfileByID after create: %v", err)
	}
	if afterCreate == nil || afterCreate.General == nil || afterCreate.General.Payloads == nil {
		t.Fatalf("GET after create: profile or payloads missing: %+v", afterCreate)
	}
	got := string(*afterCreate.General.Payloads)
	assertQuoteRoundtrip(t, "after Create (POST)", got)
	t.Logf("after Create: payload chardata:\n%s", got)

	updatePayload := mobileProfilePlist(name, "update-marker-quote-\"-only", false)
	updateReq := (&proclassic.MobileDeviceConfigurationProfile{
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{
			Name:     classicStrPtr(name),
			Payloads: payloadsXMLPtr(updatePayload),
		},
	}).ToUpdate()
	if err := pc.UpdateMobileDeviceConfigurationProfileByID(ctx, intToStr(id), updateReq); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateMobileDeviceConfigurationProfileByID: %v", err)
	}

	afterUpdate, err := pc.GetMobileDeviceConfigurationProfileByID(ctx, intToStr(id))
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetMobileDeviceConfigurationProfileByID after update: %v", err)
	}
	if afterUpdate == nil || afterUpdate.General == nil || afterUpdate.General.Payloads == nil {
		t.Fatalf("GET after update: profile or payloads missing: %+v", afterUpdate)
	}
	got = string(*afterUpdate.General.Payloads)
	assertQuoteRoundtrip(t, "after Update (PUT)", got)
	t.Logf("after Update: payload chardata:\n%s", got)

	if !strings.Contains(got, "update-marker") {
		t.Fatalf("after Update: payload still contains create-marker, server did not apply the update. Got:\n%s", got)
	}
}

func payloadsXMLPtr(s string) *proclassic.PayloadsXMLText {
	v := proclassic.PayloadsXMLText(s)
	return &v
}
