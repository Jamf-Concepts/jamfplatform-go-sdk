---
name: ingest-jsc-specs
description: Ingest a new Jamf Platform GitOps spec bundle (zip) into testing/, evaluate what changed for Jamf Security Cloud, wire-probe the changes against a live tenant, regenerate, and live-test. Use when the user supplies a jamf-platform-apis-gitops-*.zip and asks whether the Security Cloud specs changed, or asks to ingest/refresh them.
---

# Ingest a GitOps spec bundle (Security Cloud)

The Security Cloud specs move fast and the bundle's own layout is not stable.
Work through the phases in order and **do not skip the wire probe** — every
Security Cloud fact in `CLAUDE.md` was established by probing, and the specs
have been wrong in both directions.

Take the zip path from the user. If they did not give one, ask for it.
Unpack into the scratchpad, never into the repo.

## 1. Locate the specs — by content, not by directory name

The bundle has three environment trees (`internal/dev`, `internal/stage`,
`external`). Per `CLAUDE.md`: **`-beta` variants only**; `external/*-beta`
where the spec is published to prod, otherwise `internal/stage/*-beta`.
Never fall back to `public-apis-oas/redocly-implementation/teams/`.

Directory names get renamed between builds (v1401 renamed
`securitycloud-dns` → `jsc-dns` and friends), so **map source → destination by
`info.title` plus the path set**, not by name. A name-keyed match silently
finds nothing and looks like "no changes".

The current mapping table lives in `CLAUDE.md` under **Spec provenance**.
Verify each row still holds; if a directory moved, fix the table in the same
change. Two specific traps that table records:

- `securitycloud-device-groups-api.yaml` is the **Security Cloud Devices API**
  (`external/securitycloud-devices-beta`). `internal/stage/device-groups-beta`
  is the unrelated *Platform* Device Groups API.
- Check whether any of the four stage-sourced specs has appeared in
  `external/` this build. If so it is now prod-published and should be sourced
  from there.

Sanity-check the variant you picked: the right one has a non-zero
`x-required-privileges` count and `/api/securitycloud` servers. The non-beta
variants have zero privileges and a per-service namespace the gateway lacks.

## 2. Diff structurally, not textually

A textual diff is mostly prose reflow, tag re-casing and `servers` host churn.
Compare parsed YAML instead, and report these separately:

- **path set** added/removed (per method)
- **schema set** added/removed
- **per-schema semantic diff** with `description`/`example`/`title`/`summary`
  stripped, so `required`, `enum`, `type`, `minLength`, `pattern`, `format`
  and `default` changes stand out
- **per-operation semantic diff** with `tags` stripped too

Then read the *remaining* prose changes for the interesting ones only — new
constraints are usually described in the property description as well.

Things that look alarming and are not:
- OpenAPI tag re-casing (`Shared Gateways` → `shared-gateways`) — the
  generator lowercases and maps `-`/`_`/space alike, so filenames don't move.
- `X-B3-TraceId` response-header declarations appearing or vanishing.
- `servers` / `tokenUrl` host churn between builds and environments.

Things that matter: any `required` list change (it flips Go pointer-ness), any
new `enum`, any path prefix change, any schema rename.

**Check the path prefixes.** As of v1416 all five specs carry their own `/vN/`
prefix and **none** sets the spec-level `"version"` key in
`tools/generate/config.json` — dns and ztna were the last to convert. If a spec
changes its prefix, the `"op"` strings must gain or lose it **and** the
spec-level `"version"` key must be deleted or added in the same change —
otherwise you get `/v1/v1/…`, or a hard `path %s not found in spec`.

The URL the SDK builds is unaffected by which of the two carries the version:
the generator derives it from the path when the key is absent, so
`TenantPrefix("securitycloud", "v1")` comes out either way. That makes a prefix
migration pure spec hygiene — and also means a *correct* migration produces no
diff in the generated methods or tests. Do not read an empty method diff as
proof you missed something.

## 3. Wire-probe every behavioural change before ingesting

Credentials: the Security Cloud sandbox (`JAMFPLATFORM_JSC_*`). Tenant segment
must be the tenant **UUID**. Path shape is
`/api/securitycloud/tenant/{uuid}/{version}/…` (tenant-first — see
`tenantFirstNamespaces`).

For each new constraint, establish on the wire:
- Is it actually enforced, and with what status and `code`?
- Is the error attributed to a `field`, or unattributed (enum coercion
  failures usually are not)?
- Does it apply on create, on PATCH, or both? Note that PATCH checks existence
  first, so a bogus ID returns 404 before any field validation — you need a
  real resource to probe PATCH.
- Exploit validation ordering to probe safely: field validation runs before
  business rules, so a request that is guaranteed to fail a business rule can
  still exercise field validation without creating anything.

Reversed direction too: check whether the server is *looser* than the new spec
(v1401's `GatewayContact.email` pattern rejects `user@localhost`, which the
server accepts).

If a probe needs a resource that does not exist on the tenant, mint the
cheapest possible one and delete it. For ZTNA member gateways that is
`{"enabled": false, "dedicatedIps": {"enabled": true}}` — no subnets, vendor
or PSK needed. **Track everything you create and delete it before finishing;
re-list at the end and state the tenant is clean.**

## 4. Ingest and regenerate

Copy the chosen files over `testing/*.yaml` — verbatim, no edits. `testing/` is
gitignored and the generator falls back to `api/`, so a wrong copy regenerates
the tree backwards. Then `make generate` and read the whole diff.

Expect the diff to be scoped to `api/securitycloud_*.json` and
`jamfplatform/securitycloud/`. Anything outside that is a signal: either a
generator change you made, or a spec you copied to the wrong destination.

Review pointer-ness changes explicitly. A field moving `*T` → `T` is a
breaking change for consumers; grep the repo *and* the downstream providers
(`terraform-provider-jamfplatform`, `terraform-provider-jamfprotect`) for the
field before accepting it.

## 5. Test

```
go build ./... && go test -count=1 ./...        # generated unit tests
cd tools/generate && go test -count=1 ./...
go vet -tags acceptance ./jamfplatform/          # the acceptance file is build-tagged out of CI
make lint
go fmt ./... && go fix ./...
```

`go vet -tags acceptance` is not optional: a return-type or pointer-ness change
breaks the acceptance file silently, since `make test` never compiles it.

Then run the live suite:

```
JAMFPLATFORM_ACC=1 JAMFPLATFORM_JSC_* … \
  go test -v -tags acceptance -count=1 -run TestAcceptance_SecurityCloud ./jamfplatform/
```

Read the SKIPs, don't just count PASSes — and treat a skip as a defect in the
test, not a fact about the tenant. A test that skips for want of a fixture has
never verified anything, and it skips hardest on the clean tenant a CI run
starts from. If the fixture is mintable, **make the test mint it** rather than
minting it by hand: `jscEnsureGateways` is the worked example, and it turned
four permanent skips into four passes. Creating by hand fixes one run; making
the test self-provision fixes every run.

Keep the safety gate when the fixture provisions real infrastructure — the
point is not to remove `JAMFPLATFORM_JSC_GATEWAY_WRITE_OK`, it is that on a
tenant reserved for the suite nothing skips, and where a skip remains it names
the variable that would fix it instead of an absent fixture the reader cannot
act on. One skip is expected and correct: UEM Connect writes need live
credentials for a *separate* Jamf Pro or Intune tenant.

Gateway writes additionally need `JAMFPLATFORM_JSC_GATEWAY_WRITE_OK`.

**Add an acceptance test for each behavioural change you probed**, pinning the
observed status, `code` and field attribution. Prefer a test that cannot
provision anything (see the validation-ordering trick above) so it runs
unconditionally. If a test pins a current *limitation*, say so in a comment —
it should fail the day the limitation lifts.

**When a limitation does lift, flip the assertion; never delete it.** v1416
landed `GatewayIpSecPatchRequest` and made the partial `ipsec` merge-patch
reachable, so the test that pinned the resulting 400 became a test that the
partial patch applies *and* that every unpatched sibling survives the deep
merge. The negative test earned its keep by failing at the right moment; the
positive one that replaces it must assert the new capability at least as
precisely, or the coverage silently shrinks.

## 5b. Check the local spec repairs are still needed

`tools/generate/config.json` carries `schemaCreations` / `schemaPatches` entries
that substitute for things these specs omit. Each is a liability once upstream
fixes the spec: a creation shadows the real schema, a patch shadows the real
property and discards its enum/format/required-ness.

Both are guarded, so the check is mostly automatic — `schemaCreations` panics
when the name reappears, and every stand-in patch is listed in
`schemaPatchesRequireAbsent`, which panics when the path resolves. **A generate
that panics with either message is not a problem to work around: it is the spec
being repaired.** Delete the config entry (and the `schemaPatchesRequireAbsent`
line, and any `schemaCreations` it depended on), regenerate, and check whether
the upstream declaration is richer than the local one was — an enum you were
missing usually means new generated constants.

Currently outstanding for Security Cloud: `ConnectorCreateRequest.authStrategy`
and `.deviceSyncAuth`, plus the `DeviceSyncAuth` schema they need. Without them
`CreateUemConnectorV1` can only return 500.

If you add a *new* stand-in patch, list it in `schemaPatchesRequireAbsent` in
the same change. A patch that supplies a missing property without that line is
the one failure mode nothing else catches.

## 6. Record what was learned

Update `CLAUDE.md`'s Security Cloud section: the provenance table if directories
moved, the new wire facts with their date, and strike through anything the
probe disproved. Record the reasoning, not just the conclusion — the value is
in why a thing is the way it is.

Report to the user: what changed in the specs, what the wire said, what the
generated diff was, what now passes that did not, what is still pending
upstream, and anything worth reporting back to Jamf.
