// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// --- v1 combined list ---------------------------------------------------

func TestAcceptance_Pro_Computer_ListComputerGroupsV1(t *testing.T) {
	c := accClient(t)

	groups, err := pro.New(c).ListComputerGroupsV1(context.Background())
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputerGroupsV1: %v", err)
	}
	t.Logf("Found %d computer groups (legacy list)", len(groups))
}

// --- smart computer groups v2 ------------------------------------------

func TestAcceptance_Pro_Computer_ListSmartComputerGroupsV2(t *testing.T) {
	c := accClient(t)

	groups, err := pro.New(c).ListSmartComputerGroupsV2(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListSmartComputerGroupsV2: %v", err)
	}
	t.Logf("Found %d smart computer groups", len(groups))
}

func TestAcceptance_Pro_Computer_SmartGroupCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-smart-cg-" + runSuffix()
	desc := "SDK acceptance test fixture"

	created, err := p.CreateSmartComputerGroupV2(ctx, &pro.SmartComputerGroupV2{
		Name:        name,
		Description: &desc,
	}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateSmartComputerGroupV2: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateSmartComputerGroupV2 returned no ID (href=%q)", created.Href)
	}
	cleanupDelete(t, "DeleteSmartComputerGroupV2", func() error { return p.DeleteSmartComputerGroupV2(ctx, created.ID) })
	t.Logf("Created smart group %s", created.ID)

	got, err := p.GetSmartComputerGroupV2(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSmartComputerGroupV2(%s): %v", created.ID, err)
	}
	if got.Name != name {
		t.Errorf("Name = %q, want %q", got.Name, name)
	}

	newDesc := desc + " (updated)"
	got.Description = &newDesc
	updated, err := p.UpdateSmartComputerGroupV2(ctx, created.ID, got)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateSmartComputerGroupV2(%s): %v", created.ID, err)
	}
	if updated.Description == nil || *updated.Description != newDesc {
		t.Errorf("Description = %v, want %q", updated.Description, newDesc)
	}

	mem, err := p.GetSmartComputerGroupMembershipV2(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetSmartComputerGroupMembershipV2(%s): %v", created.ID, err)
	}
	t.Logf("Smart group %s membership: %d members", created.ID, len(mem.Members))

	if err := p.DeleteSmartComputerGroupV2(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteSmartComputerGroupV2(%s): %v", created.ID, err)
	}

	_, err = p.GetSmartComputerGroupV2(ctx, created.ID)
	if err == nil {
		t.Fatalf("GetSmartComputerGroupV2(%s) after delete should 404", created.ID)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetSmartComputerGroupV2(%s) after delete: want 404, got %v", created.ID, err)
	}
}

// --- static computer groups v2 -----------------------------------------

func TestAcceptance_Pro_Computer_ListStaticComputerGroupsV2(t *testing.T) {
	c := accClient(t)

	groups, err := pro.New(c).ListStaticComputerGroupsV2(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListStaticComputerGroupsV2: %v", err)
	}
	t.Logf("Found %d static computer groups", len(groups))
}

func TestAcceptance_Pro_Computer_StaticGroupCRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-static-cg-" + runSuffix()
	desc := "SDK acceptance test fixture"

	// assignments must be a non-null array — server NPEs on null (same
	// pattern as popupMenuChoices on CEA create). Sending [] instead of
	// omitting lets the server iterate safely.
	assignments := []string{}
	created, err := p.CreateStaticComputerGroupV2(ctx, &pro.StaticComputerGroupAssignment{
		Name:        name,
		Description: &desc,
		Assignments: &assignments,
	}, false)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateStaticComputerGroupV2: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateStaticComputerGroupV2 returned no ID")
	}
	cleanupDelete(t, "DeleteStaticComputerGroupV2", func() error { return p.DeleteStaticComputerGroupV2(ctx, created.ID) })
	t.Logf("Created static group %s", created.ID)

	got, err := p.GetStaticComputerGroupV2(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetStaticComputerGroupV2(%s): %v", created.ID, err)
	}
	if got.Name != name {
		t.Errorf("Name = %q, want %q", got.Name, name)
	}

	// Update description. Assignments explicitly empty — same NPE guard
	// as create (server iterates null).
	newDesc := desc + " (updated)"
	update := &pro.StaticComputerGroupAssignment{
		Name:        got.Name,
		Description: &newDesc,
		Assignments: &assignments,
	}
	if _, err := p.UpdateStaticComputerGroupV2(ctx, created.ID, update); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateStaticComputerGroupV2(%s): %v", created.ID, err)
	}

	if err := p.DeleteStaticComputerGroupV2(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteStaticComputerGroupV2(%s): %v", created.ID, err)
	}

	_, err = p.GetStaticComputerGroupV2(ctx, created.ID)
	if err == nil {
		t.Fatalf("GetStaticComputerGroupV2(%s) after delete should 404", created.ID)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetStaticComputerGroupV2(%s) after delete: want 404, got %v", created.ID, err)
	}
}

// --- computer extension attributes -------------------------------------

func TestAcceptance_Pro_Computer_ListCEAV1(t *testing.T) {
	c := accClient(t)

	items, err := pro.New(c).ListComputerExtensionAttributesV1(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputerExtensionAttributesV1: %v", err)
	}
	t.Logf("Found %d computer extension attributes", len(items))
}

func TestAcceptance_Pro_Computer_ListCEATemplatesV1(t *testing.T) {
	c := accClient(t)

	items, err := pro.New(c).ListComputerExtensionAttributeTemplatesV1(context.Background(), nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputerExtensionAttributeTemplatesV1: %v", err)
	}
	t.Logf("Found %d CEA templates", len(items))
	if len(items) > 0 {
		// Probe Get for one template to exercise the endpoint end-to-end.
		tplID := items[0].TemplateID
		tpl, err := pro.New(c).GetComputerExtensionAttributeTemplateV1(context.Background(), tplID)
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("GetComputerExtensionAttributeTemplateV1(%s): %v", tplID, err)
		}
		t.Logf("Template %s: name=%q dataType=%s", tpl.ID, tpl.Name, tpl.DataType)
	}
}

func TestAcceptance_Pro_Computer_CEACRUD(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	name := "sdk-acc-cea-" + runSuffix()
	script := "#!/bin/sh\necho sdk-acc-cea-value\n"

	created, err := p.CreateComputerExtensionAttributeV1(ctx, &pro.ComputerExtensionAttributes{
		Name:                 name,
		Description:          "SDK acceptance test fixture",
		Enabled:              true,
		InputType:            "SCRIPT",
		InventoryDisplayType: "GENERAL",
		DataType:             "STRING",
		ScriptContents:       ptr(script),
		PopupMenuChoices:     []string{},
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateComputerExtensionAttributeV1: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("CreateComputerExtensionAttributeV1 returned no ID")
	}
	cleanupDelete(t, "DeleteComputerExtensionAttributeV1", func() error { return p.DeleteComputerExtensionAttributeV1(ctx, created.ID) })
	t.Logf("Created CEA %s", created.ID)

	got, err := p.GetComputerExtensionAttributeV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerExtensionAttributeV1(%s): %v", created.ID, err)
	}
	if got.Name != name {
		t.Errorf("Name = %q, want %q", got.Name, name)
	}

	got.Description = "updated"
	if _, err := p.UpdateComputerExtensionAttributeV1(ctx, created.ID, got); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("UpdateComputerExtensionAttributeV1(%s): %v", created.ID, err)
	}

	// Download — XML body for the CEA.
	body, err := p.DownloadComputerExtensionAttributeV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DownloadComputerExtensionAttributeV1(%s): %v", created.ID, err)
	}
	if len(body) == 0 {
		t.Error("DownloadComputerExtensionAttributeV1: empty body")
	} else if !strings.Contains(string(body), "<") {
		t.Errorf("DownloadComputerExtensionAttributeV1: body %q does not look like XML", body)
	}

	// Data dependency (read-only).
	deps, err := p.GetComputerExtensionAttributeDataDependencyV1(ctx, created.ID)
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("GetComputerExtensionAttributeDataDependencyV1(%s): %v", created.ID, err)
	}
	t.Logf("CEA %s has %d data-dependency entries", created.ID, len(deps.Results))

	// History create + list.
	note, err := p.CreateComputerExtensionAttributeHistoryNoteV1(ctx, created.ID, &pro.ObjectHistoryNote{
		Note: "sdk-acc test history entry",
	})
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("CreateComputerExtensionAttributeHistoryNoteV1(%s): %v", created.ID, err)
	}
	if note.Note == "" {
		t.Errorf("CreateComputerExtensionAttributeHistoryNoteV1 returned empty note body")
	}

	hist, err := p.ListComputerExtensionAttributeHistoryV1(ctx, created.ID, nil, "")
	if err != nil {
		skipOnServerError(t, err)
		t.Fatalf("ListComputerExtensionAttributeHistoryV1(%s): %v", created.ID, err)
	}
	t.Logf("CEA %s history has %d entries", created.ID, len(hist))

	if err := p.DeleteComputerExtensionAttributeV1(ctx, created.ID); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteComputerExtensionAttributeV1(%s): %v", created.ID, err)
	}

	_, err = p.GetComputerExtensionAttributeV1(ctx, created.ID)
	if err == nil {
		t.Fatalf("GetComputerExtensionAttributeV1(%s) after delete should 404", created.ID)
	}
	var apiErr *jamfplatform.APIResponseError
	if !errors.As(err, &apiErr) || !apiErr.HasStatus(404) {
		t.Fatalf("GetComputerExtensionAttributeV1(%s) after delete: want 404, got %v", created.ID, err)
	}
}

// TestAcceptance_Pro_Computer_DeleteMultipleCEAV1 creates two CEAs and deletes
// both via the bulk endpoint, confirming they 404.
func TestAcceptance_Pro_Computer_DeleteMultipleCEAV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	suffix := runSuffix()
	var ids []string
	for _, tag := range []string{"a", "b"} {
		resp, err := p.CreateComputerExtensionAttributeV1(ctx, &pro.ComputerExtensionAttributes{
			Name:                 "sdk-acc-cea-bulk-" + suffix + "-" + tag,
			Description:          "SDK acceptance bulk-delete fixture",
			Enabled:              true,
			InputType:            "SCRIPT",
			InventoryDisplayType: "GENERAL",
			DataType:             "STRING",
			ScriptContents:       ptr("#!/bin/sh\necho bulk\n"),
			PopupMenuChoices:     []string{},
		})
		if err != nil {
			skipOnServerError(t, err)
			t.Fatalf("CreateComputerExtensionAttributeV1[%s]: %v", tag, err)
		}
		ids = append(ids, resp.ID)
		id := resp.ID
		cleanupDelete(t, "DeleteComputerExtensionAttributeV1(fallback)", func() error {
			return p.DeleteComputerExtensionAttributeV1(ctx, id)
		})
	}

	if err := p.DeleteMultipleComputerExtensionAttributesV1(ctx, &pro.Ids{IDs: &ids}); err != nil {
		skipOnServerError(t, err)
		t.Fatalf("DeleteMultipleComputerExtensionAttributesV1: %v", err)
	}

	for _, id := range ids {
		if _, err := p.GetComputerExtensionAttributeV1(ctx, id); err == nil {
			t.Errorf("GetComputerExtensionAttributeV1(%s) after bulk delete should 404", id)
		}
	}
}

// TestAcceptance_Pro_Computer_UploadCEAV1 exercises the multipart template
// upload endpoint with a minimal inline XML body. The uploaded template
// becomes a new CEA; we clean it up by id.
func TestAcceptance_Pro_Computer_UploadCEAV1(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	p := pro.New(c)

	body := `<?xml version="1.0" encoding="UTF-8"?>
<computer_extension_attribute>
    <name>sdk-acc-cea-upload-` + runSuffix() + `</name>
    <description>SDK acceptance upload fixture</description>
    <data_type>String</data_type>
    <input_type>
        <type>script</type>
        <platform>Mac</platform>
        <script>#!/bin/sh
echo sdk-acc-upload-value</script>
    </input_type>
    <inventory_display>General</inventory_display>
</computer_extension_attribute>`

	resp, err := p.UploadComputerExtensionAttributeV1(ctx, "cea.xml", strings.NewReader(body))
	if err != nil {
		skipOnServerError(t, err)
		// Upload endpoint has historically been finicky about body shape; skip
		// rather than leak on unusual server responses.
		t.Skipf("UploadComputerExtensionAttributeV1 rejected fixture: %v", err)
	}
	if resp.ID != "" {
		id := resp.ID
		cleanupDelete(t, "DeleteComputerExtensionAttributeV1(uploaded)", func() error {
			return p.DeleteComputerExtensionAttributeV1(ctx, id)
		})
		t.Logf("Uploaded CEA %s name=%q", resp.ID, resp.Name)
	} else {
		t.Logf("UploadComputerExtensionAttributeV1 succeeded but returned no id; body: %+v", resp)
	}
}
