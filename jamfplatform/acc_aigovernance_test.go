// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/aigovernance"
)

// AI Governance is environment-scoped: X-Environment-Id is required: true and the
// gateway accepts the header for this product. Note the namespace is
// ai/governance/policies with slashes — every published spec says
// `ai-governance`, which the gateway 404s. See CLAUDE.md.

func TestAcceptance_AiGovernanceReads(t *testing.T) {
	g := aigovernance.New(accEnvClient(t))
	ctx := context.Background()

	tools, err := g.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	t.Logf("%d tools (totalCount=%d)", len(tools.Results), tools.TotalCount)
	for _, tool := range tools.Results {
		t.Logf("  %s (%s) schemaVersions=%v", tool.ID, tool.DisplayName, tool.SchemaVersions)
	}
	if len(tools.Results) == 0 {
		t.Fatal("no tools returned — every policy needs a toolId, so an empty catalogue makes the write tests impossible")
	}

	// GetTool / GetToolSchema against a real tool and one of its own versions,
	// rather than a hardcoded id: the catalogue is Jamf-managed and moves.
	first := tools.Results[0]
	tool, err := g.GetTool(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetTool(%s): %v", first.ID, err)
	}
	t.Logf("GetTool(%s): displayName=%q", tool.ID, tool.DisplayName)

	if len(first.SchemaVersions) > 0 {
		v := first.SchemaVersions[0]
		schema, err := g.GetToolSchema(ctx, first.ID, v)
		if err != nil {
			t.Fatalf("GetToolSchema(%s, %s): %v", first.ID, v, err)
		}
		t.Logf("GetToolSchema(%s, %s): %d bytes of vendor JSON Schema", first.ID, v, len(schema.Schema))
	}

	policies, err := g.ListPolicies(ctx, nil, false)
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	t.Logf("%d policies in this environment", len(policies))

	for _, p := range policies {
		t.Logf("  %s tool=%s status=%v drift=%v version=%v", p.Name, p.ToolID, p.Status, p.SchemaDrift, p.CurrentVersionNumber)

		detail, err := g.GetPolicy(ctx, p.ID)
		if err != nil {
			t.Errorf("GetPolicy(%s): %v", p.ID, err)
			continue
		}
		if detail.Name != p.Name {
			t.Errorf("GetPolicy(%s).Name = %q, list said %q", p.ID, detail.Name, p.Name)
		}

		versions, err := g.ListPolicyVersions(ctx, p.ID)
		if err != nil {
			t.Errorf("ListPolicyVersions(%s): %v", p.ID, err)
			continue
		}
		t.Logf("    %d versions", len(versions))
		if len(versions) > 0 {
			n := fmt.Sprintf("%v", versions[0].VersionNumber)
			if _, err := g.GetPolicyVersion(ctx, p.ID, n); err != nil {
				t.Errorf("GetPolicyVersion(%s, %s): %v", p.ID, n, err)
			}
		}

		// A policy that has never been published has no deployment; 404 is the
		// correct answer there, not a failure.
		dep, err := g.GetPolicyDeployment(ctx, p.ID)
		var apiErr *jamfplatform.APIResponseError
		switch {
		case err == nil:
			t.Logf("    deployment: %+v", *dep)
		case errors.As(err, &apiErr) && apiErr.HasStatus(404):
			t.Logf("    no deployment (404) — policy has not been published")
		default:
			t.Errorf("GetPolicyDeployment(%s): %v", p.ID, err)
		}
	}

	// schemaDrift=true narrows to policies authored against a superseded vendor
	// schema. Asserting it is a subset is the only check available without
	// mutating anything, and it catches the param being dropped on the wire.
	drifted, err := g.ListPolicies(ctx, nil, true)
	if err != nil {
		t.Fatalf("ListPolicies(schemaDrift=true): %v", err)
	}
	if len(drifted) > len(policies) {
		t.Errorf("schemaDrift=true returned %d policies, more than the unfiltered %d — the filter is not being applied", len(drifted), len(policies))
	}
	t.Logf("%d of %d policies have schema drift", len(drifted), len(policies))
}

// TestAcceptance_AiGovernancePolicyLifecycle covers create, read, update and
// archive. It adds a policy rather than touching an existing one, so the
// environment's real policies are never modified — but the policy it creates is
// visible to admins in that environment until archived, hence the opt-in.
//
// PublishPolicy is deliberately excluded: publishing deploys the policy to the
// environment's real devices. It has its own test and its own gate.
func TestAcceptance_AiGovernancePolicyLifecycle(t *testing.T) {
	requireWriteOptIn(t, "JAMFPLATFORM_AIGOV_WRITE_OK",
		"Creates a policy in a real environment. It is archived on cleanup and never published, so it is not deployed to any device.")

	g := aigovernance.New(accEnvClient(t))
	ctx := context.Background()

	tools, err := g.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Results) == 0 || len(tools.Results[0].SchemaVersions) == 0 {
		t.Skip("Skipping: no tool with a schema version is available to author a policy against")
	}
	tool := tools.Results[0]
	schemaVersion := tool.SchemaVersions[0]

	// Settings are validated server-side against the vendor's JSON Schema for
	// this schemaVersion, and the shape is tool-specific, so an empty object is
	// the only body that is valid for every tool. If a tool ever requires a
	// member, this fails with a 422 naming it, which is the right outcome — it
	// tells the next reader what to send rather than silently skipping.
	name := fmt.Sprintf("sdk-acc-policy-%d", rand.IntN(1_000_000))
	desc := "Created by the jamfplatform-go-sdk acceptance suite. Safe to delete."
	created, err := g.CreatePolicy(ctx, &aigovernance.CreatePolicyRequest{
		Name:          name,
		Description:   &desc,
		ToolID:        tool.ID,
		SchemaVersion: schemaVersion,
		Settings:      json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CreatePolicy(tool=%s schemaVersion=%s): %v", tool.ID, schemaVersion, err)
	}
	if created.ID == "" {
		t.Fatal("CreatePolicy returned an empty ID")
	}
	cleanupDelete(t, "policy "+name, func() error { return g.ArchivePolicy(ctx, created.ID) })
	t.Logf("created policy %s id=%s href=%q", name, created.ID, created.Href)

	detail, err := g.GetPolicy(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPolicy(%s): %v", created.ID, err)
	}
	if detail.Name != name {
		t.Errorf("round-trip Name = %q, want %q", detail.Name, name)
	}
	if detail.ToolID != tool.ID {
		t.Errorf("round-trip ToolID = %q, want %q", detail.ToolID, tool.ID)
	}
	if detail.SchemaVersion != schemaVersion {
		t.Errorf("round-trip SchemaVersion = %q, want %q", detail.SchemaVersion, schemaVersion)
	}

	renamed := name + "-updated"
	if err := g.UpdatePolicy(ctx, created.ID, &aigovernance.UpdatePolicyRequest{
		Name:          &renamed,
		SchemaVersion: schemaVersion,
		Settings:      json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("UpdatePolicy(%s): %v", created.ID, err)
	}

	after, err := g.GetPolicy(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPolicy after update: %v", err)
	}
	if after.Name != renamed {
		t.Errorf("after UpdatePolicy, Name = %q, want %q", after.Name, renamed)
	}

	// An unpublished edit is a draft rather than a new version, so hasDraft is
	// the field that should have moved. Logged rather than asserted because the
	// draft/version model is not documented well enough to pin.
	t.Logf("after update: hasDraft=%v currentVersionNumber=%v status=%v", after.HasDraft, after.CurrentVersionNumber, after.Status)
}

// TestAcceptance_AiGovernancePublish is gated separately from the rest of the
// lifecycle because publishing is the step with real-world effect: it deploys the
// policy to the environment's managed devices. Everything else in the lifecycle
// test is invisible outside the admin console.
func TestAcceptance_AiGovernancePublish(t *testing.T) {
	requireWriteOptIn(t, "JAMFPLATFORM_AIGOV_PUBLISH_OK",
		"Publishing DEPLOYS an AI policy to the environment's real devices. Only opt in on an environment reserved for the suite.")
	requireWriteOptIn(t, "JAMFPLATFORM_AIGOV_WRITE_OK",
		"Publishing needs a policy to publish, which means creating one first.")

	g := aigovernance.New(accEnvClient(t))
	ctx := context.Background()

	tools, err := g.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Results) == 0 || len(tools.Results[0].SchemaVersions) == 0 {
		t.Skip("Skipping: no tool with a schema version is available to author a policy against")
	}
	tool := tools.Results[0]

	name := fmt.Sprintf("sdk-acc-publish-%d", rand.IntN(1_000_000))
	created, err := g.CreatePolicy(ctx, &aigovernance.CreatePolicyRequest{
		Name:          name,
		ToolID:        tool.ID,
		SchemaVersion: tool.SchemaVersions[0],
		Settings:      json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	cleanupDelete(t, "policy "+name, func() error { return g.ArchivePolicy(ctx, created.ID) })

	published, err := g.PublishPolicy(ctx, created.ID)
	if err != nil {
		t.Fatalf("PublishPolicy(%s): %v", created.ID, err)
	}
	t.Logf("published %s: %+v", name, *published)

	// Publishing is what creates a deployment, so the 404 the reads test sees on
	// an unpublished policy must now be gone. That is the assertion that proves
	// publish did something rather than returning 201 and no-oping.
	dep, err := g.GetPolicyDeployment(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPolicyDeployment after publish — publish reported success but produced no deployment: %v", err)
	}
	t.Logf("deployment after publish: %+v", *dep)

	// Publishing twice with no intervening change should be refused (409 in the
	// spec). Pinning it documents that publish is not idempotent.
	if _, err := g.PublishPolicy(ctx, created.ID); err == nil {
		t.Log("republishing an unchanged policy succeeded — publish appears idempotent, which the spec's 409 does not describe")
	} else {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) {
			t.Logf("republishing an unchanged policy returned %d: %s", apiErr.StatusCode, apiErr.Summary())
		}
	}
}
