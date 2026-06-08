// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestVppInvitation_DecodeWireFixture verifies the SDK decodes the fields that
// were previously broken by three spec/generator defects:
//
//  1. <invitation_usages> (plural) — was invitation_usage; whole block lost.
//  2. <last_action_date_epoch> inside each <usage> — was silently nil.
//  3. <auto_register_managed_users> in <general> — was absent from generated struct.
func TestVppInvitation_DecodeWireFixture(t *testing.T) {
	const wire = `<vpp_invitation>
  <general>
    <id>2</id>
    <name>sdk-acc-vpp-inv-ref</name>
    <auto_register_managed_users>true</auto_register_managed_users>
    <require_login>false</require_login>
    <distribution_method>Prompt Users to Install</distribution_method>
  </general>
  <invitation_usages>
    <size>2</size>
    <usage>
      <id>10</id>
      <name>alice@example.com</name>
      <email_address>alice@example.com</email_address>
      <status>Accepted</status>
      <last_action_date_utc>2026-03-01T00:00:00.000+0000</last_action_date_utc>
      <last_action_date_epoch>1778575794328</last_action_date_epoch>
      <vpp_account>sdk-acc-vpp</vpp_account>
    </usage>
    <usage>
      <id>11</id>
      <name>bob@example.com</name>
      <email_address>bob@example.com</email_address>
      <status>Invited</status>
      <last_action_date_utc>2026-03-02T00:00:00.000+0000</last_action_date_utc>
      <last_action_date_epoch>1778662194328</last_action_date_epoch>
      <vpp_account>sdk-acc-vpp</vpp_account>
    </usage>
  </invitation_usages>
  <scope>
    <exclusions>
      <user_groups>
        <user_group>
          <id>5</id>
          <name>All Users</name>
        </user_group>
      </user_groups>
    </exclusions>
  </scope>
</vpp_invitation>`

	var got proclassic.VppInvitation
	if err := xml.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Defect #1 fix: InvitationUsages block must decode.
	if got.InvitationUsages == nil {
		t.Fatal("InvitationUsages nil — wrapper element not decoded (invitation_usage vs invitation_usages mismatch)")
	}
	if got.InvitationUsages.Size == nil || *got.InvitationUsages.Size != 2 {
		t.Errorf("InvitationUsages.Size = %v, want 2", got.InvitationUsages.Size)
	}
	if got.InvitationUsages.Usage == nil || len(*got.InvitationUsages.Usage) != 2 {
		t.Fatalf("InvitationUsages.Usage len = %d, want 2", len(*got.InvitationUsages.Usage))
	}
	item0 := (*got.InvitationUsages.Usage)[0]

	// Defect #2 fix: last_action_date_epoch must be populated.
	if item0.LastActionDateEpoch == nil || *item0.LastActionDateEpoch != 1778575794328 {
		t.Errorf("Usage[0].LastActionDateEpoch = %v, want 1778575794328", item0.LastActionDateEpoch)
	}

	// Defect #3 fix: auto_register_managed_users must be decoded.
	if got.General == nil {
		t.Fatal("General nil")
	}
	if got.General.AutoRegisterManagedUsers == nil || !*got.General.AutoRegisterManagedUsers {
		t.Errorf("General.AutoRegisterManagedUsers = %v, want true", got.General.AutoRegisterManagedUsers)
	}

	// Defect #4: exclusions.user_groups.user_group must carry both id and name.
	if got.Scope == nil || got.Scope.Exclusions == nil || got.Scope.Exclusions.UserGroups == nil {
		t.Fatal("Scope.Exclusions.UserGroups nil")
	}
	ugs := got.Scope.Exclusions.UserGroups.UserGroup
	if ugs == nil || len(*ugs) != 1 {
		t.Fatalf("exclusions user_groups count = %d, want 1", len(*ugs))
	}
	ug := (*ugs)[0]
	if ug.ID == nil || *ug.ID != 5 {
		t.Errorf("exclusions UserGroup.ID = %v, want 5 (id field lost — name-only struct)", ug.ID)
	}
	if ug.Name == nil || *ug.Name != "All Users" {
		t.Errorf("exclusions UserGroup.Name = %v, want \"All Users\"", ug.Name)
	}
}

// TestVppInvitation_MarshalElementNames verifies that MarshalXML emits the
// correct wire element names for the renamed wrapper.  The top-level element
// must be <vpp_invitation> and the usage wrapper must be <invitation_usages>
// (plural), not the old <invitation_usage> (singular).
func TestVppInvitation_MarshalElementNames(t *testing.T) {
	epoch := 1778575794328
	size := 1
	status := "Accepted"
	usage := []proclassic.VppInvitationInvitationUsagesUsageItem{
		{Status: &status, LastActionDateEpoch: &epoch},
	}
	autoReg := true
	in := proclassic.VppInvitation{
		General: &proclassic.VppInvitationGeneral{
			AutoRegisterManagedUsers: &autoReg,
		},
		InvitationUsages: &proclassic.VppInvitationInvitationUsages{
			Size:  &size,
			Usage: &usage,
		},
	}

	buf, err := xml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(buf)

	if !strings.Contains(out, "<invitation_usages>") {
		t.Errorf("marshal output missing <invitation_usages>; got:\n%s", out)
	}
	if strings.Contains(out, "<invitation_usage>") {
		// the per-item element is <usage>, not <invitation_usage>; the old wrapper
		// emitted <invitation_usage> — confirm the stale singular form is gone.
		t.Errorf("marshal output still contains stale singular <invitation_usage>:\n%s", out)
	}
	if !strings.Contains(out, "<auto_register_managed_users>true</auto_register_managed_users>") {
		t.Errorf("marshal output missing <auto_register_managed_users>; got:\n%s", out)
	}
	if !strings.Contains(out, "<last_action_date_epoch>") {
		t.Errorf("marshal output missing <last_action_date_epoch>; got:\n%s", out)
	}
}
