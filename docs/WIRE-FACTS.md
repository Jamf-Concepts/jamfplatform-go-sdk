# Wire facts

Behaviour established by probing the live API, per package. Everything here
disagrees with — or is absent from — the published specs, which is why it is
written down: the spec is not the authority, the gateway and the server are.

The standard for anything in this file: a probe against a live tenant with a
**known-good control in the same invocation**. A bare 401 or 403 proves nothing
on its own (see [Diagnosing a refusal](#diagnosing-a-refusal)), and a single
probe of a routing question is not evidence — the unrouted tell is a *repeated*
`403 BAD_PERMISSIONS`.

Rules derived from this file live in [CLAUDE.md](../CLAUDE.md); this is the
evidence behind them.

---

## Diagnosing a refusal

Four layers can refuse a request and three of them look alike. The response
*body formatting* separates them:

| probe | status | body | layer |
|---|---|---|---|
| `not-a-namespace/v1/…` | 404 | `404 page not found`, plain text | gateway, unknown namespace |
| `pro/v2/…/no-such-endpoint` | 403 | compact JSON, `BAD_PERMISSIONS`, top-level `traceId` | authorization service, no rule for the path |
| `pro/v1/…/buildings/nonsense` | 400 | pretty-printed (`"code" : "INVALID_ID"`) | reached Jamf Pro |
| anything, ~30 ms, HTML | 403 | CloudFront page, `x-cache: Error from cloudfront`, `x-amz-cf-id`, **no Jamf `traceId`** | edge WAF |

Jamf Pro pretty-prints its JSON; the gateway emits compact JSON. So a compact
`BAD_PERMISSIONS` means the request never reached Pro and no scope grant will fix
it. An edge block carries no Jamf `traceId` at all, so there is nothing in Jamf's
logs to correlate and the `x-amz-cf-id` is the only handle for an AWS-side WAF
lookup. Do not encode any of this in the transport — it is a serialiser tell, not
a contract.

**A 403 that varies by credential is a capability grant. A 403 that is constant
across credentials is a missing authorization rule.** Telling them apart needs
two credentials, not two paths. Probed 2026-08-29:

| path | EU tenant | EU environment | US tenant |
|---|---|---|---|
| `GET /pro/v1/api-roles` | 200 | 403 | 200 |
| `GET /pro/v2/smtp-server/allowed-auth-types` | 403 | 200 | 403 |
| `GET /pro/v1/pki/digicert/…/privilege-check` | 403 | 400 `NOT_FOUND` (reached Pro) | 403 |
| `GET /pro/v2/environment-type` | 403 | 403 | 403 |

`environment-type` was the only structurally-refused row, and it has no rule in
`jamf/authorization-policies` on any branch — which is what made its 403
permanent and assertable. **A retraction worth not repeating:** an earlier pass
dated the deployed policy bundle by arguing that two newly-added rules were
"still refused". They were refused because those *tenant* credentials lacked the
capability; the same paths answered 200 for an environment credential against the
same bundle. Re-probe with a second credential before concluding anything about a
rollout.

**Tokens live 900 seconds, and a rejected token gives the same plain-text
`401 Authentication failed` as a credential with no policy for the api-product.**
Two rounds of scope probing were reported wrongly — once from a cached expired
token, once from a broken shell helper — before a known-good positive control in
the *same* invocation showed the harness, not the API, was at fault.

**A caller cannot check its own grants.** The token endpoint returns an opaque
token (one segment, not a JWT) and the token response's `scope` comes back empty.
With `GET /v1/api-role-privileges` withdrawn at GA there is no introspection path
at all: whether a credential holds a capability is visible only in Jamf Account.

### Where the per-path allowlist lives

Not in `tyk-gateway-management`. pro, proclassic and securitycloud are all
**catch-all proxies** (`listen_path: /api/pro/` etc., `strip_listen_path: true`,
no `white_list`, product-level scopes), and grepping the whole repo for any
withdrawn path name returns nothing. The `audit.rules` globs in the securitycloud
definition are *audit* rules, not routing.

The decision is made by an external authorization service, called from
`plugins/authz-plugin/plugin/handler.go`; its policies are readable in
**`jamf/authorization-policies`** — OPA/Rego under `policies/tyk_external/<domain>/`,
each domain's `_default.rego` carrying `default allow := false`, so **a path with
no `allow` rule is a 403 by construction**. Two things about reading it:

- **Path arrays begin with `"api"`** (`["api","securitycloud","v2","groups"]`)
  even though the GA edge 404s anything under `/api`. The edge re-prefixes before
  tyk, so these are internal paths and must not be used to reason about caller URLs.
- **`main` is not what is deployed.** The bundle is tagged `git rev-parse --short HEAD`
  and rolled out from `jamf/authorization-service`; `tyk-gateway-management` names
  the bundle but never a version. Nothing local can read the deployed digest — the
  wire is the only oracle. As of 2026-08-29 the deployed bundle sat between 18 and
  28 August in both regions, dated from *deletions* (credential-independent), not
  additions.

It is also genuinely independent evidence, unlike `_permissions/routes.yaml`,
which is **generated from the specs' own `x-required-privileges`** and therefore
cannot corroborate them. Proven, not assumed: twelve pro GETs absent from
`routes.yaml` (`/startup-status`, `/v1/dashboard`, `/v1/health-check`, …) all
answer 200, so a missing route entry is not the mechanism behind any 403.

A stale local checkout of either repo shows old config and looks authoritative.
Use `git show origin/master:<path>` after a fetch, not the working tree.

### The whole registry cross-checked against the allowlist (2026-08-31)

Every generated `Privileges` entry was matched against every `allow` rule in
`jamf/authorization-policies` — 1483 SDK operations against 1861 rules in 275
non-test `.rego` files, keyed on HTTP method plus path template with the
package's namespace prefixed under `"api"`. **1475 matched a policy route and
1454 of those agree with it exactly**, so the specs' `x-required-privileges` are
corroborated by an oracle that is not derived from them. The 29 that do not
agree, and the 8 with no matching rule, are each one of five things:

- **18 `account` operations: the spec declares nothing and the policy requires a
  capability.** `account_api.rego` gates them on `licensing:read`,
  `deal-registration:read`, `distributor-actions:{create,read,update}`,
  `sso-connections:{c,r,u,d}` and `sso-domains:{c,r,u,d}` — exactly the five
  capabilities the permissions-map article lists under organization scope. The
  published licensing, partners and sso specs carry no `x-required-privileges` at
  all, so the registry reports an empty set and **the SDK is the one
  under-reporting**. Do not hand-supply the names — a local privilege table is
  the same shadowing liability `schemaPatches` is, with nothing to panic when
  upstream fixes it — and do not read an empty `Scoped` slice as "no privilege
  required"; the godoc says so.

  **The cause is a bundle-pipeline coupling, not missing authorship** (traced
  2026-08-31, `public-apis-oas` at `959591e`, the commit v1877 was built from).
  All 18 privileges *are* declared upstream, twice: inline in
  `teams/account-{licensing,partners,sso}/openapi.yaml` (1 + 7 + 10) and again in
  each `config.yaml` under `requiredPrivileges.operations`. The build injects
  them into the published artifact only when `betaApiConfig.enabled: true`, and
  the correlation is exact — those three specs are the only ones without that
  block and the only three whose privileges are stripped; all 19 others have it
  and publish theirs intact. `teams/account-sso/config.yaml` documents the
  consequence in its own comment. The account APIs opt out of `betaApiConfig`
  correctly: it drives the beta `/{scopeType}/{scopeId}` path transform, which
  cannot apply to an organization-scoped API that resolves the org from the
  token. Privilege injection and path-prefix transformation simply share one
  switch.

  So the values are corroborated three ways — source spec, source config, and
  `account_api.rego`, which the config cites by name — and the published artifact
  is the only dissenter. **The upstream ask is to decouple `requiredPrivileges`
  injection from `betaApiConfig`**, not to add the privileges: any future API
  that opts out of the beta transform loses its privilege metadata the same way,
  silently. Re-check this when the account holds are lifted; if the coupling is
  fixed, the 18 empty sets fill in with no SDK change beyond the ingest.
- **3 `/logflush` DELETEs: the policy keeps an alternative the spec dropped.**
  `has_any_of_permissions([…policies:delete, flush-policy-logs:execute])`, so
  either capability still admits the caller. This is the platform's **only**
  cross-capability OR — every other multi-capability route uses
  `has_all_permissions` — and because the spec declares only
  `flush-policy-logs:execute`, no registry entry is an alternatives set. That is
  what licenses the godoc's "all of them are required".
- **4 `audit` operations, 1 `proclassic` file upload: matched by hand, agree.**
  Not parser gaps in the policy. `audit_service_api.rego` gates on a path
  *predicate* (`path[0..3] == api/audit/v1/audit`) rather than a literal array,
  requiring `audit:read` on `environmentPermissions` — consistent with the
  package's known blocker and with environment scope.
  `jamf_proclassic_file_uploads.rego` enumerates the resource segment as
  literals (`computers`, `mobiledevices`, `policies`, …), so the SDK's one
  templated `POST //fileuploads/{resource}/{idType}/{id}` maps to several rules,
  all `file-uploads:create`.
- **1 `securitycloud` `PUT /v2/groups/{groupId}`: no rule.** Already recorded
  below — the v2 rule was never authored.
- **2 `proclassic` parameterised mobile-device-command forms: no rule at any
  arity.** `POST //mobiledevicecommands/command/{command}/{parameter}/id/{id_list}`
  and its `/{version}/` sibling. The policy authors
  `command/{command}/id/{id_list}` plus hand-written rules for the two commands
  that actually take a parameter (`DeviceName`, `ScheduleOSUpdate`), so the
  generic parameterised shape the SDK emits has no `allow` rule and is a 403 by
  construction for any other command. **Unproven on the wire**, and honestly so:
  the probe credential 403s on the parameterised form *and* on the
  `command/{command}/id/{id_list}` form that does have a rule, and on the
  `DeviceName` rule too, so it holds neither `device-actions:execute` nor
  `destructive-device-actions:execute` and cannot discriminate policy-absence
  from a missing grant. Needs a credential holding both.

Two probes worth keeping from the same invocation (EU, 2026-08-31): the control
`GET /pro/v1/jamf-pro-version` → 200 (`11.31.1-t1787060595569`), and
`GET /pro/v1/gsx-connection` → 200, which is the two-capability AND
(`gsx-connection:read` + `push-certificates:read`) actually being satisfied
rather than inferred. Twelve requests total, per the edge-block volume rule.

The permissions-map article's capability reference is now transcribed into
`gaCapabilityActions` in `tools/generate/privileges_test.go` and asserted against
every generated identifier, so a capability or action arriving from an ingest
cannot land in a consumer's table unchecked. One deliberate difference: the
article's table lists `devices:{c,r,u,d}` while **no route uses `devices:delete`**
— device deletes are `destructive-device-actions:execute`. The table keeps the
article's superset; the absence is the fact.

---

## Jamf Pro (`pro`) and Classic (`proclassic`)

### The GA withdrawal (ingested at v1872)

29 whitelisted methods went with paths Jamf removed — 18 pro (api-roles,
api-integrations, api-role-privileges, `/v2/account-preferences`,
`/v1/oidc/dispatch`, `/v2/environment-type`, `POST /v2/mdm/commands`) and 11
Classic (peripherals, peripheraltypes). **28 of the 29 were wire-confirmed live
on 2026-08-29**, each probed individually with bogus IDs and invalid bodies so
routing was exercised without mutating anything; `GET /v2/environment-type` was
the sole exception and had never been routed.

The withdrawal is deliberate and coordinated across three repos: the same
JSC-73265 / API-364 tickets that removed the operations from the specs
(`public-apis-oas#395`, `#397`) deleted the matching hand-written OPA rules in
`jamf/authorization-policies` (`d5facb3`, `4a3adf6`, `944085e`, `ebb2253`).
`ebb2253`'s body names the mechanism outright: each removed operation "is marked
deprecated in the jss source spec and has a non-deprecated successor whose policy
rule already exists here." Credential management moved to Jamf Account.
`4a3adf6` groups peripherals under *"deprecated or not exposed because of
security"*, so that half is Wiki-driven cleanup rather than a rider on the
credential change.

**Malformed XML is the right probe for a Classic write** — rejected at parse time,
before anything can be created, so a POST's routing is provable without mutating
the tenant. Classic's two dialects also make the layer unmistakable: XML for a
success, a Tomcat HTML status page for an error, neither confusable with the
gateway's compact JSON.

**Both landed 2026-08-31, and the SDK dropped them:** `POST /v1/system/initialize`
and `POST /v1/system/platform-initialize` (`public-apis-oas#400`,
`authorization-policies#260`, API-364) — they bootstrap an on-prem server from an
activation code and neither applies to cloud. v1897 removed both paths and their
`InitializeV1` / `PlatformInitializeV1` schemas from `external/jpapi`; `cdd734b`
deleted both allow blocks from `jamf_pro_m2m_only.rego`, returning them to the
package default `allow := false`. `InitializeSystemV1` and
`PlatformInitializeSystemV1` are gone from the SDK — a breaking change with no
downstream consumer in either provider.

**The spec and the policy are ahead of the deployed gateway, and a 400 is not a
reason to re-add them.** Re-probed 2026-08-31 with `GET /pro/v1/jamf-pro-version`
→ 200 as the control in the same invocation: `{}` to `/pro/v1/system/initialize`
answers `400 INVALID_FIELD` on `jssUrl` and `password`, and to
`/pro/v1/system/platform-initialize` `400 INVALID_FIELD` on `eulaAccepted` and
`email` — Jamf Pro's own field validation, so both were still routed and unrefused
hours after the policy merged. `main` is not `deployed`; the deny arrives with the
next bundle, and until then a probe only dates the rollout.

`cdd734b` also names why they were reachable at all: every rule in
`jamf_pro_m2m_only.rego` ends in `lib.has_access(...)`, which `lib.rego` defines
as `permissions != null` — **no permission check whatsoever**. It cites the same
decision that dropped `/v1/system/initialize-database-connection` and
`/v2/environment-type` in `public-apis-oas#395`; neither was ever whitelisted here.

**`ebb2253` (`#259`) removed the policies for four further deprecated jpapi
operations, and the SDK was already clear of all four**:
`GET /v1/macos-managed-software-updates/available-updates`,
`GET`+`PATCH /v2/account-preferences` and `POST /v1/oidc/dispatch`. `config.json`
whitelists only their non-deprecated successors
(`/v1/managed-software-updates/available-updates`, `/v3/account-preferences`,
`/v2/oidc/dispatch`), so the deprecated four had already left the spec in an
earlier build with nothing to migrate. Worth re-checking per bundle: a policy
deletion for a path the SDK *does* whitelist is a coming 403, not a diff.

### Privileges

pro and Classic converted to the GA `{capability}:{action}` form in v1824 and
were ingested at v1872: 746 pro ops, 606 Classic ops, both `securitySchemes`
scope lists, zero `{action}:pro:{resource}` strings left. Cross-checked against
the bundle's own `_permissions/` oracle — all 193 pro and 188 Classic privilege
strings declared in `scopes.yaml`, `routes.yaml` agreeing on every shared route
with zero disagreements.

The Classic conversion consolidates rather than renames. The computer/mobile
split collapses into single capabilities (`devices`, `device-groups`,
`extension-attributes`, `configuration-profiles`, `applications`,
`advanced-device-searches`, `enrollment-invitations`, `device-actions` /
`destructive-device-actions`), the three vpp resources fold into
`volume-purchasing-locations`, and `read:pro:computers` fans out three ways by
context — `device-history:read` (29 ops), `devices:read` (10),
`advanced-device-searches:read` (4), with `device-history` being new.

Eight Classic ops changed what a caller must hold: the three `/logflush` DELETEs
dropped the policy-delete requirement, the five `/mobiledevicecommands` POSTs
split across `destructive-device-actions:execute` and `device-actions:execute`,
device DELETEs became `destructive-device-actions:execute` (**there is no
`devices:delete` capability**), and `computer-inventory-collection` gained a
`-settings` suffix.

Two quirks that survived the conversion and look like bugs but are not:
`DELETE`/`POST .../patch-software-title-configurations/{id}/dashboard` require
`patch-management-software-titles:**read**`, and `DELETE`/`POST
/v2/patch-policies/{id}/dashboard` require `patch-policies:**read**` — dashboard
membership is modelled as a read on the title, not a delete on it.

### Enum behaviour

- **Unrecognised enum values are silently dropped, not rejected.** Pro
  deserialises an unknown member to null and proceeds; only a *recognised* value
  reaches business-rule validation. A typo in a caller's string therefore changes
  behaviour with no error. This is the whole argument for the generated constants.
- **The server is an oracle for value sets.** An invalid value on a validated Pro
  field returns `possible values can be [ A, B, C ]`. Six fields (computer +
  mobile-device extension attributes × `dataType`, `inputType`,
  `inventoryDisplayType`) were confirmed byte-identical to what we emit.
  `ListSmtpServerAllowedAuthTypesV2` returned all four spec values through the
  gateway on 2026-08-29, confirming that enum complete.
- **Classic validates neither `subset` nor `createdBy`** — garbage returns 2xx
  with the full record, and Classic's Tomcat HTML errors never list valid values.
  Classic has no oracle at all; its constants are documentation.
- **Read and write representations can disagree on casing.**
  `ComputerGeneral.platform` (read) is a bare `string` and returns `"Mac"`;
  `ComputerGeneralCreate.platform` (write) declares `[WINDOWS, MAC, NONE]`. Both
  are faithfully generated, but a consumer comparing a read value against
  `ComputerGeneralCreatePlatformMac` never matches. Do not assume a round-trip.
- **`manageExistingData` carries an unencoded precondition:** writable only on
  update, and only when `inputType` is `SCRIPT` with `enabled` false.
  `RETAIN`/`DELETE` are correct but rejected outside that state.
- `MDMCommandType` is now **unverifiable**: its oracle was the withdrawn
  `POST /v2/mdm/commands` type-resolution error (`known type ids = [...]`). The
  enum still generates from `MDMCommand` on the read side.

### Server bugs worth reporting

- `POST .../pro/v3/computer-groups/static-groups` 500s if `assignments` is
  omitted despite being optional in the spec; pass `assignments: []`.
- `POST /v2/mdm/commands` with `{}` answered **500 with an empty `errors` array**,
  while `{"clientData":[],"commandData":{}}` correctly answered `400 INVALID_FIELD`.
  (Recorded before the path was withdrawn.)
- `DownloadIconV1` answers **500** for an icon that uploaded successfully and
  returned a live CDN URL (traceId `3b39c7b12ddad6175c18030c5240c501`, id 2191).
  `TestAcceptance_Pro_IconV1` swallows it through a skip-on-server-error branch —
  contrary to the never-tolerate-real-errors rule — and because a 500 on a GET is
  retryable the test spends ~153 s in the retry loop first.

### The remaining 38 jpapi paths (whitelisted 2026-08-31)

The `pro` whitelist covered all 790 operations in `openapi-jpapi.json` as of that
date; v1897 withdrew two, leaving 788. The
last 38 were probed against `eu.api.jamfcloud.com` with a control
(`GET /pro/v1/jamf-pro-version`) in every invocation. Five disagreements with the
spec, all recorded in `config.json`:

| finding | evidence |
|---|---|
| `PUT /settings/obj/policyProperties` answers **200**, spec declares 201 | write-back of the tenant's own values returned 200 |
| `POST /v1/inventory-preload/history` and `POST /inventory-preload/history/notes` answer **200**, spec declares 201 | same call against `/v2/inventory-preload/history` and `/v1/self-service/settings/history` returns 201, so this is per-endpoint, not a dialect |
| unversioned `GET /inventory-preload` honours **`pagesize` only** — `page-size` is ignored | with four seeded records, `pagesize=2` → 2 rows, `page-size=2` → all 4. `/v1/inventory-preload` honours both |
| unversioned `GET /inventory-preload` sends the bare `{totalCount, results}` envelope, not the *array of* envelopes its `application/json` schema declares | identical body from the `/v1/` twin, which declares it correctly |
| `text/csv` request bodies are schemaless in the spec (`{"type":"object"}`) | `application/json` on `validate-csv` → **415** `Content-Type 'application/json' is not supported`; raw CSV → 412 with per-row codes |

The `expectedStatus` correction on `CreateInventoryPreloadHistoryNoteV1` fixes a
**method that was already shipping broken** — it has expected 201 since it was
whitelisted, so every call failed with "unexpected status code" while the write
itself succeeded.

Two paths are **unrouted at the gateway** while being present in
`_permissions/routes.yaml`. Both are generated and both are pinned by a test
that fails when the block lifts (`acc_pro_gateway_and_hosted_limits_test.go`):

- `GET /v1/dss-declarations/{declarationId}`. Declared with `declarations:read`,
  and the credential holds it — the two `ddmreport` operations requiring the same
  string (`ListDeclarationReportClients`, `GetDeviceDeclarationReport`) both
  answer 200 for it.
- `POST /v1/jamf-pro-server-url/history`, while **GET on the same path is routed
  and answers 200** — a method-level gap. Declared with `jss-url:update`, and the
  credential holds it: `PUT /pro/v1/jamf-pro-server-url` reaches Jamf Pro and
  answers 415 for an unsupported media type, which the gateway would have
  pre-empted.

The tell for both is response shape: the gateway emits compact JSON with a
`traceId` and `errors[].code == BAD_PERMISSIONS`, byte-identical to a
deliberately bogus path (`GET /pro/v1/zzz-not-a-real-endpoint`), whereas Jamf
Pro's own responses are pretty-printed. **That is a cheaper discriminator than a
second credential** for separating an unrouted path from a denied privilege —
but only in one direction: a compact 403 rules out Jamf Pro having answered, it
does not distinguish an unmapped path from an ungranted gateway capability.

Two more are routed and structurally refused, each pinned the same way:

- `PUT /v1/cache-settings` → **403 `HOSTED_ENVIRONMENT`**, "PUT command is not
  available in hosted environments". Pretty-printed, so Jamf Pro's own refusal.
  The GET is fine. Permanently unusable on Jamf Cloud.
- `POST /v1/macos-managed-software-updates/send-updates` → **503**, "This
  endpoint cannot be used if the Managed Software Update Plans toggle is on."
  The toggle check runs **before** field validation, which is what makes an
  empty-body probe safe on either kind of tenant.

**`GET /inventory-preload/csv-template` is not broken — an `Accept` header breaks
it.** An earlier pass here recorded it as server-broken on the strength of a 400
`INVALID_REQUEST_PARAMETER_TYPE` complaining that "csv-template" is not an
integer. The cause was the probe: Jamf Pro maps both
`/inventory-preload/{id}` (produces JSON) and `/inventory-preload/csv-template`
(produces `text/csv`) under one prefix, so `Accept: application/json` is
content-negotiated onto the `{id}` handler. The SDK sends no `Accept` on these
and gets a 200 `text/csv` template. Both header forms were run against the same
tenant to establish it. This is the "SDK-mediated observation is not wire truth"
rule inverted: **curl was the misleading observer**, and the disagreement was
still resolved by diffing the requests.

TeamViewer Remote Administration path parameters are typed `string` in the spec
and the server requires **a positive integer or `-1`** — a UUID gives
`400 INVALID_ID` naming `pathId`, `integrationID` or `configurationId` depending
on the operation. The generated `string` parameters are right; the constraint is
on the value.

### v1942 dropped 146 deprecated operations from the specs; every one is live (2026-09-01)

Build v1942 removed, from both `external/` and `internal/stage/`, every deprecated
operation that had a surviving successor version — 122 from `jpapi`, 20 from
`capi`, 2 from `declaration-reporting`, 2 from `securitycloud-devices`. Nothing
was added, no schema changed, and the 666 surviving `jpapi` operations and 675
schemas were byte-equal to v1897's. The SDK **took** the removals, as a deliberate decision to follow the specs —
so it is now stricter than the gateway on all 146. The evidence that the server
has withdrawn none of them, gathered 2026-09-01 against Jamf Pro
`11.31.1-t1787060595569`:

Probed with a `pro` credential on `eu`, `X-Tenant-Id` header form, with
`GET /pro/v1/jamf-pro-version` → `200 {"version":"11.31.1-…"}` as the control in
the same invocation. Every path below is one v1942 deleted:

| path | status | payload |
|---|---|---|
| `GET /pro/v1/computers-inventory?page-size=1` | 200 | `totalCount: 4`, first row `id 5` |
| `GET /pro/v2/computers-inventory?page-size=1` | 200 | identical body to v1 |
| `GET /pro/v3/computers-inventory?page-size=1` | 200 | identical body to v1 |
| `GET /pro/v4/computers-inventory?page-size=1` | 200 | identical body — the successor, as a second control |
| `GET /pro/v1/groups?page-size=1` | 200 | `totalCount: 382` |
| `GET /pro/v2/groups?page-size=1` | 200 | identical body — successor |
| `GET /pro/v1/inventory-preload?page-size=1` | 200 | `totalCount: 0` |
| `GET /pro/v2/inventory-preload/records?page-size=1` | 200 | identical — successor |
| `GET /pro/v1/computer-inventory-collection-settings` | 200 | preferences object |
| `GET /pro/v2/computer-inventory-collection-settings` | 200 | identical — successor |
| `GET /pro/v1/mobile-device-groups?page-size=1` | 200 | bare array, 2+ groups |
| `GET /pro/v2/patch-software-title-configurations` | 200 | `[ ]` |
| `GET /pro/v2/mobile-device-prestages?page-size=1` | 200 | `totalCount: 6` |
| `GET /pro/v1/mdm/commands?page-size=1` | 400 | `{"httpStatus":400,"errors":[]}` — its required-filter validation, i.e. routed and unrefused |
| `GET /proclassic/computers` | 200 | 4 computers |
| `GET /proclassic/computers/subset/basic` | 200 | 4 computers, basic subset |
| `GET /proclassic/patches` | 200 | `{"patch_management_software_titles":[]}` |
| `GET /proclassic/patchsoftwaretitles` | 200 | `{"patch_software_titles":[]}` |
| `GET /proclassic/patchpolicies` | 200 | `{"patch_policies":[]}` |

The deprecated version and its successor return **byte-identical bodies** wherever
both were sampled, so these are aliases at the gateway, not a legacy shape being
wound down.

Two further checks, each independent of the specs:

- **`authorization-policies`** `origin/main` is still `cdd734b` — v1897's
  `/v1/system/initialize` deny — with no successor commit. The hand-written OPA
  allow blocks for `v1`, `v2` and `v3` `computers-inventory` (including the
  capability and wrong-token-guard variants) are all present.
- **The bundle's own `_permissions/routes.yaml`** is byte-different from v1897 and
  `sort`-identical: a pure reordering. 1474 route-method entries on both sides,
  zero removed, zero added, zero permission changes — `/computers`, `/patches`,
  `/patchpolicies` and `/v1/computers-inventory` all still declared with their
  capabilities. Since `routes.yaml` is generated from the specs'
  `x-required-privileges`, its generation must precede the filter, which places
  the deletion at the publish stage.

The only derivative change: `capi`'s OAuth scope list went 188 → 185, losing
`patch-management-software-titles:{create,update,delete}` — referenced solely by
the removed operations. `patch-management-software-titles:read` survives, on
`GET /patches/name/{name}`.

A fourth signal came out of the ingest itself: the withdrawn Security Cloud v1
group operations carried **`x-sunset-date: 2027-08-25`** next to
`x-successor-endpoint`, in the very build that deleted them — a spec deleting a
path a year before its own declared sunset.

**Report upstream, and re-diff every subsequent bundle.** The removal is a
publish-stage filter, not a withdrawal, and the SDK following it is a decision
rather than a corroboration.

### What v1942's removal cost, probed the same day

Three capability gaps opened, each now pinned by a test that fails when it
closes. All probed 2026-09-01 with `GET /pro/v1/jamf-pro-version` → 200 as the
control in the same invocation.

**Security Cloud has no working device-group update.**
`PUT /v1/groups/{groupId}` is gone and its declared successor
`PUT /v2/groups/{groupId}` is unrouted — 403 `BAD_PERMISSIONS`, 7/7 on
2026-08-29, unchanged. `ApplyDeviceGroupV2` used the v1 write for its update
branch and now uses the v2 one, so Apply cannot update. The standalone pin
`TestAcceptance_SecurityCloudUpdateDeviceGroupV2` survives but its control had
to weaken from a v1 *write* to a v1 read: no working write remains to control
with.

**Patch software titles are unseedable.** The only path that minted an id the
Pro v3 configuration endpoints address was Classic's
`POST /patchsoftwaretitles/id/0`, now withdrawn. The alternatives do not work:

| probe | result |
|---|---|
| `GET /proclassic/patchavailabletitles/sourceid/1` | 200, `size: 1551`, entries carry `name_id`, `app_name`, `publisher` |
| `POST /pro/v3/patch-software-title-configurations` with `softwareTitleId` = a catalogue `name_id` (`"376"`) | `400` `SOFTWARE_TITLE_ID_NOT_FOUND` attributed to `softwareTitleId` — the catalogue's `name_id` is **not** a `softwareTitleId` |
| `GET /pro/v1/patch-software-titles` | `403 BAD_PERMISSIONS` (unrouted) |
| `GET /pro/v2/patch-software-titles` | `403 BAD_PERMISSIONS` (unrouted) |
| `GET /pro/v3/patch-software-title-configurations` | 200 `[ ]` — nothing pre-existing to borrow, and borrowing would mean mutating a tenant's own configuration |

So `seedPatchSoftwareTitleFixture` skips, and with it the V3 lifecycle, Apply
and resolve-by-name tests. Restore real seeding when either the Classic
operation returns to the spec or a routed Pro catalogue endpoint is whitelisted.

**`GET /patches/name/{name}` answers 500, not 404.** One of the few patch
operations to survive, and it is broken: `500` for an obviously-absent name 3/3,
and `500` again for a plausible one (`Firefox`), on a tenant whose
`/patchsoftwaretitles` list is empty. Server defect, pinned by
`TestAcceptance_Classic_GetPatchByName`, which fails when it starts answering
404.

**Classic computer enumeration and reads moved keys.** `GET` and
`PUT /computers/id/{id}` went while `POST` and `DELETE` on the same path
survived. `GET /proclassic/computers/match/*` answers 200 with `id`, `name`,
`udid`, `serial_number` and `mac_address` per row, and `MatchComputers` returns
the same `*Computers` type `ListComputers` did — so it is a drop-in enumeration
replacement. Reads move to `GetComputerByName` / `…BySerialNumber`;
`GET /computers/subset/basic` and `/computers/id/{id}/subset/{subset}` have no
surviving equivalent at all.
`GET /proclassic/patchpolicies/id/99999999/subset/General` → 404, as expected.

### The gateway-bypass technique

When a gateway 403 blocks verification, separate "Jamf Pro can't do this" from
"the gateway won't forward it": provision an all-privileges API role + client
*through* the gateway, read the instance host from `jamf-pro-server-url`, then
authenticate directly at `POST https://<instance-host>/api/oauth/token` and call
the **tenant-less** Pro paths (`/api/v2/environment-type`, not
`/api/pro/v2/...`). This proved all three 11.31 endpoints worked in Jamf Pro
while the gateway refused them.

**This technique is now broken** and cannot be restored from the SDK: it depended
on `POST /v1/api-roles` + `api-integrations`, both withdrawn. Mint the credential
in Jamf Account instead.

### Resolved: the edge WAF blocked every multipart upload (2026-08-28)

Recorded because the diagnosis is reusable. `POST /pro/v1/packages/{id}/upload`
with a real signed installer answered **403 with a CloudFront HTML page** in
~30 ms. Root cause, confirmed by the API team: the IP had been added to a WAF
blocklist under **XSS attack detection**, and "including plist/XML content within
request body can trigger the detection".

How it was localised, in order, because the first two hypotheses were wrong:
not size (10 MiB of filler passed); not the SDK's request shape (replaying the
exact bytes through curl succeeded whenever the payload was benign); not the
protocol version (blocked on both); **it was the file's own bytes** — truncating
the installer put the boundary between 512 B and 2 KiB, exactly the span of the
xar zlib-compressed table of contents whose uncompressed form is XML carrying an
RSA `<signature>`. The decisive control was the *retired* gateway: byte-identical
content, same credential, same invocation, `apigw.jamf.com` reached Jamf and
`api.jamfcloud.com` blocked.

**Two failure modes were in play at once and are indistinguishable from the
response**: a payload rule blocks one request, and enough of them blocklist the
*IP*, with the same status, HTML and `x-amz-cf-id`. ~150 probe requests during
the investigation triggered the second, so **the investigation caused a second,
broader outage**. When re-probing an edge block: keep volume low, always
interleave a benign control in the same invocation, stop as soon as the answer is
in.

Verified fixed across widening scope — a real 9.8 MiB signed `.pkg` (OK in 3.4 s),
both branding uploads, Classic plain-XML writes, Classic XML carrying an embedded
mobileconfig, and all eight Classic profile-payload roundtrips (quote, ampersand,
line-break and reserved-character matrices). That last row mattered most: Classic
is XML end-to-end across 617 operations, so a live rule would have taken out most
of the SDK's write surface. **No SDK change was needed and none should be added** —
do not reshape a multipart body or escape plist content to placate a WAF.

---

## Pagination

`ListAllPages` computes each page's offset by multiplying the *requested* page
size, so if the true cap is lower than requested, page N+1 skips the untransferred
tail of page N **with no error at any layer**. That is why the default is scoped
as narrowly as the evidence.

- **Every `pro`-package, `totalCount`-style list operation clamps `page-size` at
  exactly 2000, silently, with zero exceptions found.** Verified by seeding real
  data past 2000 rows and comparing a `page-size=5000` request's returned count
  against the seeded total, across **33 operations** of wildly different resource
  shapes: `ListGroupsV1`/`V2`, `ListAccountsV1`, `ListEnrollmentAccessGroupsV3`,
  and 29 History operations. No skip or overlap at any 2000/2001 boundary. Hence
  `defaultMaxPageSize` bakes 2000 in for the whole pro+totalCount combination
  (110 of 112 pro list operations).
  - Re-confirmed 2026-08-31 on `/inventory-preload/history` (both the `/v1/` and
    unversioned forms) against a tenant holding 2052 rows: `page-size=5000`
    returned exactly 2000, and pages 0 and 1 at `page-size=2000` had **zero
    overlap and a union of exactly 2052**. `TestAcceptance_Pro_InventoryPreload_HistoryUnversioned`
    keeps a duplicate check on the walk while the tenant stays over 2000 rows.
  - **The page-size parameter name is not uniform.** Unversioned
    `GET /inventory-preload` reads `pagesize` and ignores `page-size`; every other
    pro list reads `page-size`. Configured via `pageSizeParam`, and load-bearing:
    a list whose page size is silently dropped returns the server's own default
    while `ListAllPages` multiplies offsets by 2000.
- **Deliberately excluded:** `ListUsersV1` (`hasNext`) and `ListSiteObjectsV1`
  (`rawArray`) were not in the verified sample and fail differently under a wrong
  cap — `hasNext` can skip a chunk between pages, `sizeCheck` can stop early and
  truncate the whole remainder. Both stay at 100.
- **`devices/v1` hard-rejects rather than clamping**: `page-size>1000` returns a
  structured 400 (`"Page size must not exceed 1000"`), so `ListDevices`,
  `ListDeviceApplications` and `ListDevicesForUser` set `maxPageSize: 1000`
  explicitly. Same SDK, same generator, genuinely different backend — which is
  exactly why the pro default is not extended to other packages.
- **No other package's cap has been probed.** Classic, DeviceGroups, Blueprints,
  DDMReport and ComplianceBenchmarks stay at 100.

**Security Cloud: only two of four `totalCount` ops actually paginate.**

| op | honours `page`/`page-size`? | wire cap |
|---|---|---|
| `ListZtnaAppsV1` | yes | **1000** (undocumented; `page-size=99999` → 400) |
| `ListUemConnectorSyncRunsV1` | yes | **100** (spec-documented, wire-confirmed) |
| `ListZtnaGatewaysV1` | **no** — params ignored entirely | n/a |
| `ListUemConnectorsV1` | **no** | n/a |

The last two declared a `{totalCount, results}` envelope but always return the
complete list in one shot (`page=99&page-size=1` still returns everything). With
`pagination: totalCount` configured, `hasNext = (page+1)*pageSize < totalCount`
would have looped and **concatenated the same full list repeatedly** on any tenant
with more than 100 — a duplication bug, the mirror image of the truncation risk.
Fixed by dropping the `pagination` key from both.

**AI Governance:** `page`/`page-size` honoured on `/policies` and
`/policies/{id}/versions` (max 500, enforced with a 400); **`GET /v1/tools`
ignores them** and returns the whole catalogue.

**Genuinely untestable, not merely unverified** — listed so nobody re-litigates
the same wall: `compliance-benchmarks/*` (backend 500s tenant-wide),
`blueprint-components` and `enrollment/languages` (fixed catalogs of a few dozen
rows), `pki/adcs-settings/{id}/history` (create needs real X.509 material),
`patch-software-title-configurations/{id}/history` (needs a `softwareTitleId`
from Jamf's external catalog), `cloud-idp/{id}/history` (**no create endpoint
exists at all**), `mdm/commands` (needs a device with a live push channel).

---

## Jamf Security Cloud (`securitycloud`)

### URL shape

The gateway is a catch-all proxy; the path globs in the api-product definition
are **audit** rules covering mutating methods only, and must not be read as a
routing allowlist.

| form | result |
|---|---|
| `/api/securitycloud/tenant/{t}/v1/dns/zones` | 200 — what the SDK builds, and the only form the audit globs match |
| `/api/securitycloud/v1/tenant/{t}/dns/zones` | 200 — also routed; what the SDK sent before 2026-08-21 |
| `/api/securitycloud/tenant/{t}/dns/zones` | **403** — the versionless form the published spec declares |
| `/api/securitycloud/tenant/{t}/uem-connect/v1/connectors` | 200 — UEM Connect carries its own in-path version |

**Routing is indifferent to the order; auditing is not.** The matcher is
`audit-plugin`: `CompileGlob` renders `**` as `.+` (one-or-more, never matching
empty) and matching happens on the raw inbound path with `/api/securitycloud`
removed, before rewriting. Every glob is `/vN/<svc>/…` or `/**/vN/<svc>/…` —
version *after* the wildcard — so a stripped path only matches when the tenant
segment precedes the version. Sending version-first meant **19 of 27 mutating
Security Cloud operations executed but were never audited**. Fixed in the SDK via
`tenantFirstNamespaces`, not upstream: one allowlist entry beats five region
files of new globs. Recompute coverage after any change to either side — the
answer is not guessable.

The tenant segment must be the tenant **UUID**: a tenant name returns
`400 REQUEST_CONTEXT_NOT_PROVIDED` and another organisation's tenant ID returns
`403 OWNERSHIP_FORBIDDEN`, so a failing tenant segment is distinguishable from
both an unrouted path and a privilege failure.

**Not routed** (403 with credentials that reach every other Security Cloud path):
the `jsc-api-gateway` ping, `securitycloud-devices`' `GET /v1/…/devices`, and
**every `/v2/groups/{id}` method** — see below.

### Device groups `/v2/{id}`: the rule has not been authored

`securitycloud_api_devices.rego` carries exactly six rules — `POST`/`GET /v1/groups`,
`GET /v2/groups`, and `GET`/`PUT`/`DELETE /v1/groups/{groupId}`. There is **no
`/v2/groups/{groupId}` rule of any method**, and grepping all 60+ remote branches
and all open PRs finds none pending. So this is not awaiting a rollout; it has not
been written. Watch that repo, not the specs and not tyk.

Wire-confirmed 2026-08-29 with an environment-scoped EU credential, v1 control in
the same invocation:

| request | result |
|---|---|
| `PUT`/`GET`/`DELETE /securitycloud/v2/groups/{real-id}` | **403**, 4/4 attempts |
| `PUT /securitycloud/v2/groups/{bogus-uuid}` | **403**, 3/3 |
| `PUT /securitycloud/v1/groups/{real-id}` (control) | **200** with the `Group` body |
| `PUT /securitycloud/v1/groups/{bogus-uuid}` | `404 GROUP_NOT_FOUND`, `field: groupId` |

`TestAcceptance_SecurityCloudUpdateDeviceGroupV2` asserts the 403 and **fails when
routing lands** — invert it then, and do not weaken it to a skip.

**Re-probed 2026-09-01 with the JSC sandbox tenant `928260f5…` on eu. Nothing has
moved, and the v1 write still works.** Full sequence in one invocation:

| request | result |
|---|---|
| `GET /securitycloud/v2/groups` (list, control) | **200**, the `{groups: []}` envelope |
| `POST /securitycloud/v1/groups` | **201** `{href, id, name}` |
| `GET /securitycloud/v1/groups/{real-id}` (control) | **200** |
| `PUT /securitycloud/v2/groups/{real-id}` | **403** `BAD_PERMISSIONS`, 3/3 |
| `PUT /securitycloud/v2/groups/{bogus-uuid}` | **403** `BAD_PERMISSIONS` |
| `GET /securitycloud/v2/groups/{real-id}` | **403** `BAD_PERMISSIONS` |
| `PUT /securitycloud/v1/groups/{real-id}` | **200**, body carries the new name |
| `GET /securitycloud/v1/groups/{real-id}` (read-back) | **200**, rename persisted |
| `DELETE /securitycloud/v1/groups/{real-id}` | **204**, then GET → 404 |

Two things this pins down. The 403 is **not** id-shaped or permission-shaped — a
bogus uuid gets the same answer as a real one, and the same credential drives the
v1 write to 200 in the same invocation, so it is the unrouted-path tell, not a
capability gap. And **no `/v2/groups/{groupId}` verb is routed**: the `GET` fails
the same way the `PUT` does, matching `securitycloud_api_devices.rego` having no
rule for that path at all.

**v1942 therefore removed the only working device-group update and kept the broken
one.** It withdrew `PUT /v1/groups/{groupId}` from the spec — the call that answers
200 above — leaving the unrouted v2 PUT as the package's sole update method.
`ApplyDeviceGroupV2` was repointed onto it, so its update branch can no longer
succeed (`TestAcceptance_SecurityCloudApplyDeviceGroup` pins that 403 too), and
this probe's control had to weaken from a v1 *write* to a v1 read because the SDK
no longer generates the write. Restore `UpdateDeviceGroupV1` if the gap needs
closing before Jamf authors the v2 rule — the wire still serves it.

Two methodological warnings from this probe. The first attempt returned **500 on
both v1 and v2**, which reads as "v2 is routed and merely faulting" — the opposite
of the truth; repeating each request 3–4 times showed the steady state. And a
**pro** credential answers 403 on `/securitycloud/v1/groups` *and* `/v2/groups`
alike, so a routing probe on the wrong credential set concludes "still unrouted"
no matter what the gateway is doing.

### Report upstream: the DNS zone operations carry `ztna:*`

The v1559 split gave `search-domains` and `custom-hostname-mappings` — in the same
spec — their own privilege families, so `/v1/dns/zones` carrying `ztna:read`,
`ztna:create`, `ztna:update` and `ztna:delete` reads like a copy-paste rather than
a decision. It may be deliberate (DNS zones being part of the ZTNA capability) but
nothing in the spec says so, and a customer granting `ztna:*` to manage DNS zones
is surprising either way. **The SDK reports what the spec says; do not "correct" it
locally.**

Note these are not gateway scopes: neither the old `*:jsc:all` names nor the new
ones appear anywhere in `tyk-gateway-management`, which carries only a
product-level `securitycloud-product` scope. Per-privilege checks live in the
authorization service and the Jamf Account permissions model, so the tyk repo
cannot answer "is this live" for a privilege rename. The wire can.

### `href` is nulled by response compression

`ZoneRef.href` and `CreateResponse.href` are `required` and *are* sent — but only
when the response is uncompressed. Ask for `Accept-Encoding: gzip` and the same
request returns `"href": null` **and drops the `Location` header**; identity, `br`
and no-header all return both. Deterministic 12/12 each way.

Root cause: the backend never sends `href` at all; the gateway's
`href-injection` plugin synthesises it, and bails out early when the response
carries a `Content-Encoding` other than `identity`, logging *"unsupported
Content-Encoding %q on POST 201; leaving response unchanged"*. Go's `net/http`
adds `Accept-Encoding: gzip` to every request, so **no Go caller can ever see
`href`** and only `id` is usable. Report it as "the injection plugin cannot
rewrite a compressed body", not as "the gateway nulls href".

`assertCreateHrefEmpty` pins the emptiness so the suite **fails when this is
fixed**, rather than silently continuing to assume it.

This is also the cautionary tale for treating SDK-mediated observation as wire
truth: the long-standing note that "the server sends only `id`" was only ever
observed through this SDK. AI Governance is **not** affected — it emits `href`
itself, verified with both `--compressed` and `identity`.

### ZTNA

- **`vendor` is enum-validated and case-sensitive**, and an invalid value returns
  `400 INVALID_FIELD` with body `"Request body is missing or malformed."` and
  **no `field`** — enum coercion fails during deserialisation, before per-field
  validation. An invalid `keyExchange` behaves identically. This is the strongest
  case in the SDK for generated constants: the failure is unattributed. Unknown
  *properties*, by contrast, are silently ignored at every level.
- `ConnectionConfigRightRequest`'s own description contradicts its enum, naming
  lowercase `cisco`, `strongSwan`, `juniper`; the enum says `Cisco`/`Juniper` and
  lowercase gets the unattributed 400 above. **Follow the constants, not the prose.**
- **`left` and `right` differ in both range and cardinality.** `ipsec.left.subnets`
  must be a private range (`10/8` /8–/30, `172.16/12` /12–/30, `192.168/16`
  /16–/30) — the spec's own `0.0.0.0/0` examples are rejected — and takes
  **exactly one** subnet; `right.subnets` accepts public ranges and **many**.
  Nothing explains why. Note the deep IPSec checks do not run while a required
  top-level field is missing, so a left-cardinality probe needs an otherwise-valid
  request.
- `dedicatedIps.enabled: true` is mutually exclusive with `ipsec`, and a gateway
  must be one or the other. `dedicatedIps: {enabled: true}` with no `ipsec` is the
  cheapest throwaway gateway — no subnets, vendor or PSK needed.
- `auth` and `left.vendor` are accepted and silently ignored on write, reading
  back as `"psk"` and `null`.
- **`ipsec: null` on PATCH returns `400 IPSEC_REMOVAL_NOT_SUPPORTED`**, and is
  unreachable from Go either way (`*GatewayIpSecPatchRequest` with `,omitempty`).
  `IPSEC_SECRET_CLEAR_NOT_SUPPORTED` fires for `left.secret: null` and does carry
  a `field`.
- Omitting `left.secret` on PATCH preserves the existing PSK — the documented way
  to patch anything else about the tunnel.
- **Two error field paths map to nothing in the schema:** a missing `tenantIds` is
  reported as `field: "customerIds"`, and an array violation as
  `field: "ipsec.left.subnets[].<iterable element>"`. Both are internal Bean
  Validation names leaking. Any consumer doing attribute-level diagnostics off
  `FieldErrors()` needs a documented fall-through for unmappable keys, and this is
  the concrete case proving it is not hypothetical.
- **`recoveryDelayInSec` is required on create for every routing strategy**, even
  `RANDOM`/`NEAREST` whose prose says it is *ignored* — "ignored" describes the
  semantic, not whether you must supply it. Value must be one of
  `300, 1800, 3600, 10800, 28800`; `0` — the Go zero value a caller gets by
  forgetting the field — is rejected. Unlike `vendor`, this rejection *is*
  attributed. PATCH enforces the same set and rejects `null`; omitting the field
  leaves the stored value untouched.
- **Field validation runs before business rules.** A create carrying *shared*
  gateway IDs and a legal delay reaches `422 SHARED_GATEWAY_MEMBER`; the same
  request with an illegal delay stops at `400` first.
  `TestAcceptance_SecurityCloudZtnaGroupedGatewayRecoveryDelay` exploits exactly
  this to pin the whole matrix without provisioning anything.
- `GatewayContact.email`: the spec's regex requires a dot in the domain; the
  server uses Bean Validation's `@Email`, which does not, so `user@localhost` is
  accepted on the wire and rejected by the spec. **The spec is stricter than the
  server** — do not trust the pattern.
- `App.name` is `required` *and* `nullable`: an app created from a
  `predefinedAppId` inherits its name and returns `name: null`. This is why
  `ResolveZtnaAppV1ByName` cannot reach such apps.
- `App.routing.type` is `CUSTOM` or `DIRECT` only. `categoryName` validates
  against the **`displayName`** of the tenant's own category list, not a fixed
  enum, and an unknown value is `409 MISSING_CATEGORY_NAME` — a state conflict,
  not a malformed request.

### UEM Connect

- **`authStrategy` is required and its absence is a 500** (`500 INTERNAL_ERROR`,
  stable across repeats, re-verified 2026-08-31), not a validation error. Supply
  it and a request without credentials returns a proper `422 VALIDATION_FAILED`.
  A request built from the published spec alone could only ever fail, which is
  why `authStrategy`, `tenantId` and `deviceSyncAuth` were patched in — **until
  v1882, which declares all three** on its new `JamfProConnectorCreateRequest`
  and let the request-side patches go.
- **The 500 is no longer reachable through the typed request, and that is the
  point.** v1882 makes `authStrategy` required, so it generates as a non-pointer
  `string` and absence cannot be expressed. What a forgetful caller now sends is
  the empty string, which the enum coercion refuses properly:
  `422 "Cannot coerce empty String (\"\") to JamfProAuthStrategy"`, field
  unattributed (2026-08-31). Required-ness converts an unactionable 500 into a
  diagnosable 422.
- **Deserialization and field validation run ahead of the singleton pre-check**
  (2026-08-31). An unknown `vendor` is `422` where a well-formed request is
  `409`. This is what makes field-level probing possible at all on a tenant that
  already holds a connector, and every UEM Connect create subtest depends on it.
- **`url` is not required for `M2M`, though v1882 lists it in the variant's
  `required` set.** Omitted *and* empty-string both clear field validation and
  reach the 409; a connector created with `url` absent reads back carrying the
  URL the server derived from the named tenant (`https://nmartin.jamfcloud.com`
  from tenant `ff584e5b…`, 2026-08-31). That is what makes the generated
  non-pointer `URL string` safe — an M2M caller who never sets it sends
  `"url": ""` and is accepted. For the credential strategies `url` genuinely is
  required: `JAMF_PRO_OAUTH` without one is `422 ": invalid auth configuration
  for Jamf PRO"`, unattributed. **Report upstream:** the flat
  `required: [vendor, url, authStrategy]` cannot express a per-strategy
  requirement and is wrong for one of the three.
- **v1882 claims `tenantId` is never echoed back; the wire echoes it.** The
  schema prose says write-only fields "are never echoed back in any response",
  but a GET on an `M2M`-created connector returns
  `"tenantId": "ff584e5b-d9f8-4c1c-8752-449d8c5e45d5"` (2026-08-31, re-confirming
  2026-08-28). The marker itself is on the *request* schema and so is harmless,
  but the prose is false and `ConnectorConfig.tenantId` stays patched — it is the
  only field distinguishing an M2M connector from an OAuth one, since
  `authStrategy` reads back as `JAMF_PRO_OAUTH` either way. **Report upstream.**
- **`additionalProperties: false` is not enforced.** An unknown property on a
  `JAMF_PRO` create reaches the 409 (2026-08-31). The spec is stricter than the
  server; harmless for the SDK, which sends no unknown fields.
- **Three response fields are still undeclared anywhere**, observed on an
  `M2M`-created Jamf Pro connector (2026-08-31): `tokenAuthenticationSupported`
  (`true`), `uemRiskStateExtAttrs` (`null`) and `deployedProfiles` (`null`). Only
  the first is typeable from the observation — a null column proves nothing about
  its type — so none is patched in yet. **Report upstream.**
- **Two usable strategies for a Jamf Pro connector, needing different fields.**
  `JAMF_PRO_OAUTH` takes `deviceSyncAuth.clientId`/`.clientSecret` for an API
  integration the caller created themselves, plus the instance `url`. **`M2M`
  takes `tenantId` and no credentials at all** — Jamf Security Cloud provisions
  its own role and integration ("JSC Connector") on that tenant, and `url` is
  ignored. Missing `tenantId` under M2M is `422 "tenantId: must not be null"`.
  This is what keeps the create path testable now that the SDK can no longer mint
  a Pro credential. Both strategies, and `BASIC`, are declared upstream as of
  v1882 and agree with what was probed.
- **`authStrategy` is a provisioning instruction, not stored state:** a connector
  created with `M2M` reads back as `JAMF_PRO_OAUTH`. It does not round-trip.
- **A tenant may hold exactly one connector, whatever its vendor.** A complete,
  correct request answers `409 CONNECTOR_CONFIG_ALREADY_EXISTS` — the message says
  "incompatible UEM vendor" but INTUNE and JAMF_PRO are refused identically, and
  the check fires *before* credential validation, so bogus credentials give the
  same 409. Consequence: the create path is exercisable on a tenant that already
  has a connector, reaching the singleton pre-check without provisioning anything.
- **This service has a server-side oracle for its vendor enum**, unlike ZTNA. A
  recognised vendor reaches field validation (`emmPassword: must not be null`)
  while an unrecognised one fails Jackson subtype resolution first *and prints the
  whole accepted set*: `known type ids = [AIRWATCH, GOOGLE, INTUNE, JAMF_PRO,
  JAMF_SCHOOL, MAAS360, MOBILEIRONCLOUD, MOBILEIRONCORE, WIZY, XENMOBILE,
  EmmServerConfig]` — less `EmmServerConfig`, which is Jackson's base-class name
  leaking. Do not generalise ZTNA's unhelpfulness to the rest of Security Cloud.
- Secrets are genuinely write-only: a read returns only `clientId`, `username`
  and an `empty` flag. Combined with the singleton rule, that is why a
  credential-bearing connector cannot be deleted and recreated, and why every
  other write in the family needs a tenant whose *only* connector is disposable.
- **An `M2M` connector is the disposable one, and that unblocks the write
  suite.** It is recreatable from `tenantId` alone — no secret to lose — so
  unlike a credential-bearing connector it can be deleted and remade at will.
  The JSC sandbox tenant `928260f5…` now holds exactly such a connector
  (`6a95614fa7d64061069424cf`, `JAMF_PRO`/`M2M` against
  `nmartin.jamfcloud.com`), created 2026-08-31 in place of the MaaS360 one that
  was there before. So `TestAcceptance_SecurityCloudUemConnectWrites`, whose skip
  reason is that the tenant's connector is unrestorable, is now stale: enablement,
  the sync-settings full-replacement round-trip, trigger/cancel sync and the
  activation-profile deploy are all reachable. Note the deletion must also clear
  the "JSC Connector" API role and integration `M2M` provisions on the Jamf Pro
  side, which the SDK cannot do.
- `EmailMapping`'s `CUSTOM` type returns an extra `fieldName` naming the UEM
  attribute to read, **deliberately undocumented** because ADG-125 requires every
  documented field to be present in every response. The transport decodes
  leniently so nothing breaks, but `fieldName` is unreachable from Go. Worth
  reporting: a documentation policy is costing generated clients a real field.
- **v1942's `uemGroups` documentation is upstream's claim, not wire truth
  (2026-09-01).** It is the only substantive v1942 change the SDK took, and it is
  doc-only, but it asserts four behaviours nothing here has probed: that
  `GroupMapping.emmGroupId` **requires** the `computer_<id>` / `mobile_<id>`
  prefix for Jamf Pro and rejects a bare ID; that
  `ActivationProfileDeployRequest.uemGroups` accepts a bare numeric ID as well;
  that a present prefix must match the device type the `platform` deploys to; and
  that scoping is **additive** — the IDs merge into the configuration profile's
  existing scope, so a deploy can widen but never narrow or clear it, and an
  omitted or empty array leaves the existing scope untouched rather than meaning
  "all groups". The last is the one that would bite a caller who read the old
  wording ("If omitted or empty, deploys to all UEM groups"), so it is worth
  confirming against the sandbox connector above before a consumer relies on it.
  The same text sends callers to `GET /v1/mobile-device-groups`, which v1942
  deleted from `jpapi` in the same build.

### PUT response shapes

`PUT /v1/…/groups/{id}` answers **200 with the updated `Group`**, not 204 — and it
is the only Security Cloud PUT that disagrees with its spec. `PUT dns/search-domains`,
`PUT dns/custom-hostname-mappings`, `PUT uem-connect/…/enablement` and
`PUT uem-connect/…/sync-settings` were each probed by idempotent round-trip (GET
current state, PUT it back unchanged, confirm nothing moved) and all four genuinely
return 204.

**A create's response shape is only ever knowable by probing it.** `POST /ztna/apps`,
`/ztna/grouped-gateways` and `/ztna/gateways` returned the full resource object for
months, then reverted to the spec-declared `CreateResponse` on or before v1439.
Nothing in the spec ever described the old behaviour, so no ingest could have
predicted either direction; an acceptance assertion on response *shape* caught it.

### Device groups v1 deprecation headers

Wire-verified header-by-header across all five v1 group operations:

| op | `Deprecation` header | spec `deprecated` |
|---|---|---|
| `GET /v1/groups` | `@1786492800` (2026-08-12) | **true** |
| `PUT /v1/groups/{id}` | `@1787616000` (2026-08-25) | absent at the time |
| `POST /v1/groups`, `GET`/`DELETE /v1/groups/{id}` | none | absent |

Two things follow. A wire-only deprecation is still surfaced, because
`logDeprecation` logs any runtime `Deprecation` header once per method+path —
nothing needed building for it. And **the successor the header names does not
exist**: `/v2/customers/{tenantId}/groups` is a third URL shape and answers 403,
as does every `/v2/groups/{id}` form. The server is telling callers to migrate a
write to a path that is neither published nor routed.

`ListDeviceGroupsV1` returned a **bare JSON array** and `ListDeviceGroupsV2` wraps
it in `{groups: []}`. Both resolvers and both Applies existed side by side, sharing
the v1 create/update/delete ops. **v1942 withdrew `GET /v1/groups` and
`PUT /v1/groups/{groupId}`**, so only the v2 resolver and Apply remain — and the
`{groups: []}` unwrap is now the SDK's *only* `resultsField` user with no
bare-array sibling to contrast against, which is exactly the shape most likely to
rot silently. `POST /v1/groups`, `GET /v1/groups/{groupId}` and
`DELETE /v1/groups/{groupId}` all survived, so Apply's create branch and the
lifecycle test still work.

`Default Group` comes back with **no `id`** on both v1 and v2 — the reason
`GroupListItem` requires only `name`, and the reason `ResolveDeviceGroupV1ByName`
yields an empty ID for it.

### Enrollment (not covered — no published spec)

`securitycloud-enrollment` (activation profiles) is `api_maturity: ga` with
`prod: true`, answers in production, and appears in **no** environment of any
bundle. Three things learned while it was briefly generated, worth having when the
spec lands: `POST /v1/activation-profiles` returns `{"code"}` rather than the
declared `{id, href}`; `capabilities.networkSecurity` and
`capabilities.vulnerabilityManagement` must both be enabled or both disabled
(400 `INVALID_FIELD`); and **`POST /v1/activation-profiles/delete-multiple`
answers 204 and deletes nothing** — for a real code and a bogus one alike — so
creating a profile leaks an undeletable enrollment code.

---

## Jamf Account (`account`) — organization scope

Wire-verified 2026-08-27 with a real organization credential, URLs exactly as
generated:

| endpoint | result |
|---|---|
| `GET /api/licensing/v1/licenses` | 200 — 16 real licence rows |
| `GET /api/sso/v1/domains` | 200 — 5 real domain rows |
| `GET /api/partners/v1/deal-registrations` | 200 `[]` |
| `GET /api/sso/v1/connections` | **502** `An upstream service returned an error` |

Three things only the wire could tell us:

- **US only.** The `account` tyk product ships *only* `use1` api-definitions in
  every environment — no euc1, no apne1. An EU credential cannot reach these and
  the failure will not look like a region problem. Use `https://us.api.jamfcloud.com`.
- **The list endpoints return bare JSON arrays, not the spec's
  `{totalCount, results}` envelope.** All four are declared as envelopes with both
  members `required`; the server sends `[…]`. Following the spec produced methods
  that fail every decode. Fixed with `"responseType": "[]License"` etc.
- **`Domain.id` and `Domain.verifiedTldId` are JSON numbers declared as strings.**
  The spec's own description is self-aware — *"Treat it as an opaque string, even
  though it is currently a decimal number"* — but it declares `type: string` and
  the server sends `1552`, unquoted. `json.Number` is the honest target: it
  decodes the number, preserves the text, and converts to the string the
  `domainId` path params want.

**A sweep that skips null values will miss these, and did.** The first pass
reported `Domain.id` as the only mismatch because it compared only non-null
values and `verifiedTldId` was null on all five sampled domains. A larger
organization populated it and the decode failed on row 31. **Treat "no
mismatches" from a small sample as unproven, not as clearance.** The reliable
predictor was in the spec: a `type: string` field whose `example` is a bare
decimal. Those two are the only such fields across the three account specs —
every other string-typed `*Id` carries a UUID, a Salesforce key, an `org_` handle
or a hex trace ID.

**The whole distributor surface is blocked by a half-finished Skyway scope
migration.** `GET /api/partners/v1/distributor/configuration` answers `400` with
an OAuth body on an API path: `{"error":"invalid_scope","error_description":
"Invalid scopes: skyway-use2-product"}`. The partners backend calls Skyway and in
**prod** asks for a scope that exists only in **dev**; prod declares
`skyway-use1-product` and, since `tyk-gateway-management` `e2f54c1c`, a
region-independent `skyway-product` added for exactly this purpose. Nothing to fix
in the SDK — the URL is confirmed correct, every non-distributor endpoint on the
same credential returns 200, and `account_api.rego` carries the full distributor
surface, so authorization passes. **Note the tell: an `invalid_scope` OAuth error
arriving on a resource path means a *backend* service failed its own token
exchange, not that the caller's credential is wrong.** `isSkywayScopeFault`
matches on the scope name rather than the status code, because a 400 from these
endpoints could equally be a genuine validation verdict.

The 502 on `/sso/v1/connections` is an upstream fault, but note its body is the
account service's own envelope (`{classification, fields, message}`) — a fourth
dialect none of the shapes under [Error handling](#error-dialects) cover.
`Details()`/`FieldErrors()` parse nothing from it. It is also retryable, so a
client with the default policy sits on it until the context deadline.

**No `x-required-privileges` anywhere in these specs** — 0 privileged ops in every
variant, and no `-beta` variant to take them from. So `account/permissions.go` is a
registry of `Scoped: nil`. That is honest reporting, not a gap.

### Held back from v1872

Two breaking changes in the account specs are **ahead of the server** and were not
ingested. Probed 2026-08-29 with the US organization credential, control in the
same invocation:

| probe | result |
|---|---|
| `GET /sso/v1/domains/allocation/{domain}` × all 5 | 200 — **`authZeroRegion` on 5/5, `authRegion` on 0/5** |
| `GET /licensing/v1/licenses` | 200, 16 rows — **`type` populated on 16/16** |

So `DomainAllocationConnection.authZeroRegion` → `authRegion` and the removal of
`License.type` would both be **silent** regressions: nothing sets
`DisallowUnknownFields`, so a renamed field the server does not send decodes to
nil and a removed field the server still populates simply becomes unreachable.
Neither fails a test.

**`License.type` is not recoverable by a single substitution when it does land.**
It mixes two taxonomies: on the 5 BETA/NFR rows it duplicates `licenseType`
exactly; on the other 11 it names a product while `licenseType` is `NFR` or null.
Nothing loses its only identification, but there is no one rule — the Jamf Trust
row is `type: JAMF_SECURITY_CLOUD` with `addOnType: JAMF_TRUST`,
`productTopLine: TRUST` and `productParent: SECURITY_CLOUD`, all four different.

**Report upstream: the `Region` enum is incomplete.** It declares `[US, EU, AU, JP]`
and the wire returned **`RAMP`**. Nothing breaks today because the field is `any`,
but `RegionValues()` exists to feed `stringvalidator.OneOf` and would refuse a
region the server itself returns.

---

## AI Governance (`aigovernance`) — environment scope

**The published path was wrong in every variant and the gateway was the
authority — ~~fixed upstream at v1877~~.** The spec declared
`servers: …/api/ai-governance/policies` while the tyk listen_path is
`/api/ai/governance/policies`, with slashes: the generated form returned 200 with
real data and the spec's hyphenated form `404 page not found`. Reported upstream
and corrected in v1877 (`external/ai-governance`, and `_permissions/routes.yaml`'s
domain key with it) — the whole build was that one line. **Nothing in the SDK
changed**, because the transport never reads `servers`; the only generated diff
was the URL string inside the published `api/ai_governance_policies_api.json`.
The lesson stands even though the instance is closed: the gateway is the
authority, and a `servers` block is not evidence.

**The package is `aigovernance`, not `aigovernancepolicies`, and the gateway
settles that too:** `…/ai/governance/visibility/v1/policies` answers **403**
(routed namespace, no policy rule) while the bare `…/ai/governance/v1/policies`
answers **404 page not found**. Those two answers distinguish "routed but
ungranted" from "no such namespace", and they are the basis for expecting
Visibility to land as a second spec in this package. It still has no spec.

`X-Environment-Id` is `required: true`. **A malformed value is a 500, not a 400**:
a well-formed unknown UUID correctly gives `404 ENVIRONMENT_NOT_FOUND`,
`not-a-uuid` gives `500`, and sending `X-Tenant-Id` as well makes the tenant
header win and the request fail `403 OWNERSHIP_FORBIDDEN`.

### Three real defects (full-surface probe, 2026-08-30)

1. **`GET /v1/policies/{id}/deployment` is non-functional — it always returns
   `{"blueprints":[]}`.** Confirmed three ways, including a blueprint created for
   the probe referencing a freshly published policy. The blueprints service *does*
   resolve the link — referencing a nonexistent policy version is refused
   `400 POLICY_VERSION_NOT_FOUND` on
   `steps[0].components[0].configuration.policies[0].policyId` — so the reference
   exists and the AI-governance side is not reading it. This is the endpoint's
   entire purpose. `TestAcceptance_AiGovernanceDeploymentReportsReferencingBlueprints`
   cross-references the two APIs off existing state and **fails when the endpoint
   starts working**. Do not weaken it to a skip: an empty 200 is exactly the kind
   of answer that goes unnoticed.
2. **Archiving a blueprint-referenced policy succeeds (204)** and leaves the
   blueprint pointing at a policy nothing can read. No referential guard — the
   opposite of Security Cloud's `*_REFERENCED_BY_*` 409s.
3. **Archive is not a readable soft delete.** The spec calls it a soft delete
   retaining published versions "for audit trail integrity". On the wire the
   parent becomes **unreachable** — `GET`, `PATCH`, `publish` and a second
   `DELETE` all 404, and the policy leaves the list — while `/versions`,
   `/versions/{n}` and `/deployment` keep answering 200. So the parent 404s and
   its children 200, `status: ARCHIVED` is a value **no caller can ever read**
   (making `PolicySummaryStatusArchived` unobservable), the audit trail is
   reachable only via an id the caller must already hold, and `DELETE` is not
   idempotent. Every create therefore leaves a permanent row.

### Behaviours a consumer must know, neither in the spec

- **`PATCH` replaces `settings` wholesale — it does not merge.** A patch carrying
  only `permissions.defaultMode` wiped the stored `permissions.allow`/`deny` and
  `env`. Load-bearing for any read-modify-write caller.
- **`hasDraft` is a diff against the published version's `settings`, not a record
  that an update happened.** Publish `{}`, patch a key in → true; patch back to
  `{}` → **false again**, though both PATCHes returned 204 and both bumped the
  write counter. A reconciler that patches then publishes only when `hasDraft` is
  true will silently skip, and calling publish regardless is
  `409 NO_DRAFT_TO_PUBLISH` rather than a no-op.
- **Publishing does not deploy anything.** `POST /publish` freezes the draft into
  an immutable version row and nothing more; a policy reaches a device only by
  being referenced from a blueprint's `com.jamf.ai-governance` component *and*
  that blueprint being deployed. A separate `JAMFPLATFORM_AIGOV_PUBLISH_OK` gate,
  justified as "publishing DEPLOYS an AI policy", was therefore protecting against
  nothing and kept the publish path unexercised; it is folded into
  `JAMFPLATFORM_AIGOV_WRITE_OK`.
- Unknown `settings` keys are **accepted** (the vendor schemas do not set
  `additionalProperties: false` at the root) and persist into the published version.

### Params and errors

- **`schema-drift` is a switch, not a boolean filter.** `true` narrows to drifted
  policies; `false` returns **everything**, drifted included. So the only relation
  a test can assert is subset. `schema-drift=banana` is silently treated as false
  while `page=-1` correctly 400s.
- **Only the first `sort` criterion is validated or applied.** `sort=bogus:asc`
  alone is a 400, but `sort=name:asc,bogus:desc` is 200. `sort=name:sideways` is
  accepted too. The SDK comma-joins the slice, which is not the spec's
  `explode: true` form but is immaterial given only element zero is read.
- Errors report `field: "pageSize"` — the internal camelCase name, not the wire
  key `page-size`.
- **Status contract, worth relying on:** `400 VALIDATION_FAILED` for a missing or
  blank required member (one detail per field), `422` for a semantic failure
  (`TOOL_ID_UNKNOWN`, `SCHEMA_VERSION_UNKNOWN`, `SCHEMA_VALIDATION_FAILED`),
  `409 NO_DRAFT_TO_PUBLISH`, `404 POLICY_NOT_FOUND`/`VERSION_NOT_FOUND`.
  `GET /v1/tools/{id}/schemas/{ver}` answers **422** for an unknown version
  although the operation declares only 200/401/403/404. A non-UUID `policyId` is a
  **404**, not a 400, so status cannot detect a malformed id.
- `SCHEMA_VALIDATION_FAILED` sets `field` to a **JSON Pointer relative to
  `settings`** (`/permissions/defaultMode`), not the documented dot-path. A blank
  `name` yields the raw Bean Validation regex `must match ".*\S.*"`.
- `PolicyDetail` carries an **undeclared `version` integer** — 0 on create,
  incrementing on every write, `null` on UI-created rows. A write counter,
  unreachable from Go, probably meant to be internal.
- Two 401 dialects: the gateway's plain-text `Authentication failed` for a bad
  token, versus JSON `{"httpStatus":401,"message":"unauthorized access"}` for no
  header at all — the latter carrying no `errors` array, so `Details()` parses
  nothing.
- Actor IDs leak the internal issuer
  `https://eu.int.apigw.jamf.com/m2mex/realms/platform`, and the blueprints create
  `href` leaks `euc1.tyk-external.jprosvc.jamfapps.io`.

---

## Audit (`audit`) — environment scope, blocked on a grant

Generated, compiled and typed correctly; **every call is refused**, and the SDK is
not at fault.

Prod `tyk-gateway-management` `3e99c347` (2026-08-28, *"TRIVIAL Audit Service is
Environment scoped only"*, all three prod regions plus dev and stage) changed
`platform-audit-service`:

| key | before | after |
|---|---|---|
| `request-context-allowed-sources` | `[token, path]` | **`[token, path, header]`** |
| `request-context-types` | `[environment, organization]` | **`[environment]`** |

That fixed both halves of the original diagnosis. The original 400 came from
`plugins/authz-plugin/requestcontext/token.go`: an external-M2M token carries no
tenant, environment or organization claim, so `TokenProvider.Resolve` falls
through to a server-side organization lookup — but only when
`len(RequestContextTypes) == 1 && [0] == "organization"`. licensing/partners/sso
declare exactly `[organization]` and work; audit declared two entries, so the
fallback never fired.

Re-probed 2026-08-29 on **two separate** environment credentials, control
(`GET /blueprints/v1/blueprints` → 200) in the same invocation:

| credential | request | result |
|---|---|---|
| environment (eu) | `GET /audit/v1/audit` + `X-Environment-Id` | **403 BAD_PERMISSIONS** |
| environment (eu) | `GET /audit/v1/audit/sources` + `X-Environment-Id` | **403** |
| organization (us) | `GET /audit/v1/audit`, no header | `400 REQUEST_CONTEXT_NOT_PROVIDED` |
| organization (us) | `GET /audit/v1/audit` + `X-Organization-Id` | `400 INVALID_REQUEST_CONTEXT_TYPE` |

**The gateway now says what it wants, in as many words:** *"Request context type
'organization' is invalid. Expected any of 'environment'."* So the context half is
fixed and the remaining 403 is pure authorization. `audit_service_api.rego` (20
lines after `ee84e61`, *"TRIVIAL Remove organization scoping from Audit"*, same
day as the tyk change) confirms it independently: environment-only,
`environmentId != ""`, gated on `read:env:audit` or `audit:read`. It also confirms
the path prefix is `/v1/audit/…` — an earlier probe here guessed `/v1/events`.

**What audit needs is an environment-scoped credential granted `audit:read`.** No
SDK change, no gateway change. Two separate credentials now lack it, making it a
provisioning gap rather than a quirk of one integration — and since the token is
opaque with an empty `scope`, no caller can confirm this from the API at all.

**The path-scoping workaround this file once contemplated is confirmed
unnecessary. Do not add it.**

**Report upstream:** the audit spec's prose still says *"`audit:read` (environment
scope … or organization scope, when X-Environment-Id header not present)"*. The
organization half is dead as of 2026-08-28, and `scopes.yaml` in the same bundle
still lists `audit` under `organization` — two published artefacts advertising a
scope the gateway rejects.

`AuditEnvelope` is the SDK's first discriminator-less `oneOf` union. The spec says
it is discriminated *structurally* — a gateway event carrying
`actor` + `requestContext`, a service event carrying `data`, never mixed — so the
merged struct has the six shared base fields required and those three as pointers:
nil `Actor` means service event, nil `Data` means gateway event. **No discriminator
is synthesised**, deliberately: the nearest candidate is `auditSource`, an open
string whose own description says "e.g. `api-gateway`, `blueprints`, `ai-policy`",
so any mapping would rot the first time a new source appears.

---

## Error dialects

Per-family behaviour of `Details()` / `FieldErrors()` / `Summary()`. The probes
validating this are in `acc_api_errors_test.go`.

| family | structured details | `Field` populated |
|---|---|---|
| Pro (JSON) | yes | **yes** — `name`, `id`, … |
| Devices / DeviceGroups / Blueprints / DeviceActions | yes | usually empty, so everything buckets under `""` |
| Compliance Benchmarks 404s | empty body or `errors: []` | n/a — `Summary` falls back to status text |
| Classic | **no** — Tomcat HTML error page, preserved in `Body` | n/a; do not HTML-scrape in the transport |
| DDM Report | n/a — returns an empty report for unknown devices rather than an error | n/a |

**Security Cloud is not one dialect.** DNS, ZTNA and UEM Connect return the
standard `{httpStatus, traceId, errors[]}`, DNS being the only one that populates
`field`/`id` (as nulls). Activation profiles return the same shape minus
`traceId`. Device groups return a different envelope entirely —
`{message, messageKey, messageParams, error, logref, statusCode}` — which parses
to nothing. **The status code is the only thing every Security Cloud service
populates, so branch on `HasStatus`, not on details.**

Rule for consumers doing field-attributed diagnostics (Terraform
`AddAttributeError`, CLI per-field output): iterate `FieldErrors()` and fall
through to a generic diagnostic when the field key is empty. That works across
every family.

---

## Rate limiting

**No Jamf path rate-limits as of 2026-08-31. This is a fact with an expiry date
— rate limiting is planned — so treat it as a snapshot, not a standing
property, and re-probe before relying on it.**

Wire evidence, `us` prod, one `pro` GET plus a 30-request burst shaped like
Terraform's default parallelism (3 rounds x 10 concurrent, no client pacing):
all 30 returned 200, and every response carried

```
x-ratelimit-limit: 0
x-ratelimit-remaining: 0
x-ratelimit-reset: 0
```

Tyk reports the limit as `0` when rate limiting is disabled for the key. **No
response carried a `Retry-After` header.**

Corroborated in `jamf/tyk-gateway-management` @ `92c06467`: both prod plans
(`prod/plans/default.yaml`, `default-external.yaml`) carry `rate: -1`,
`per: -1`, `quota_max: -1`. The only `global_rate_limit` anywhere in `prod/` is
on the m2m Auth0 client-registration path (`rate: 2, per: 3`), which the SDK
never calls. `Retry-After` appears nowhere in the gateway config or in
`jamf/tyk-custom-plugins`.

Two consequences for the transport:

- **429 is currently unreachable on SDK paths**, so `jamfBackoff`'s
  `Retry-After` branch is untested against the real gateway. It is written to
  be correct when rate limiting arrives rather than tuned to the present
  absence — `retryablehttp.DefaultBackoff` supplies the parsing, and the SDK
  adds only a clamp to `retryWaitMax`.
- **When rate limiting does land, the shape matters.** If it emits
  `Retry-After`, the transport already honors it and nothing needs changing. If
  it does not, the exponential curve is the only backpressure response, and
  jittering it becomes worth adding — retries are routinely concurrent and a
  deterministic curve makes requests that failed together collide again. Worth
  asking the gateway team to emit `Retry-After` on 429 and to populate the
  `x-ratelimit-*` headers that currently return `0`; that removes the guessing
  entirely.

### Tyk enforced timeouts on smart-group writes

Not rate limiting, but found in the same config pass and shares the
retry-amplification concern. `prod/api-products/pro/api-definitions-*.yaml`
sets `hard_timeouts` on exactly two operations, in all three regions:

```yaml
hard_timeouts:
- method: POST
  path: "/v2/computer-groups/smart-groups"
  timeout: 180
- method: PUT
  path: "/v2/computer-groups/smart-groups/{id}"
  timeout: 180
```

These are *raised* ceilings — someone hit Tyk's default enforced timeout on
smart-group writes and pushed it to 180s, which is itself evidence the endpoint
struggles.

The PUT is in the transport's retryable set (idempotent method, see
`isRetryableWriteStatus`) and the POST is not. Retrying the PUT is **safe** —
`UpdateSmartComputerGroupV2`/`V3` carry no `VersionLock`, `If-Match` or ETag, so
there is no stale precondition to replay and the final state converges. But it
is expensive: the method expects `202 Accepted`, so the write is asynchronous
and a Tyk timeout means Tyk stopped waiting, not that the upstream declined the
work. A retry re-queues a recomputation that is probably still running, and
because each attempt can burn 180s of upstream work before Tyk cuts it, an
exhausted sequence is ~15 minutes of wall clock.

**This is the one endpoint the 2026-08-31 backoff change does not help.** That
change shortened the *waits* (1+2+4+8 = 15s); it did not shorten the attempts.
Not flagged `noRetry`, because the retry is correct and a genuine transient 504
on this path is worth recovering — and the transport cannot tell an enforced-timeout
504 from a transient one by status alone. Recorded so the latency is not a
surprise; revisit if the deterministic-timeout case turns out to be the common
one.

---

## Transport details established by probing

- **Credentials in the base URL's userinfo (`https://user:pass@host/path`) never
  reach the wire.** `net/http` applies `URL.User` as Basic only when
  `Authorization` is empty, and both the token exchange and API calls already
  carry one. Inline userinfo is reported to work against other Jamf SDKs; here it
  is silently dropped with no error. `TestURLUserinfoIsNotSentAsBasicAuth`
  documents it.
- **A caller-supplied `Authorization` flips the token exchange to
  `AuthStyleInParams`**, because the caller's header takes the slot the client
  credential would use. Wire-verified 2026-08-26 that
  `us.apigw.jamf.com/auth/token` accepts body-form client credentials — a full
  proxied round trip returned `11.31.1`. Without the flip, x/oauth2's
  auto-detection still gets there but only after a rejected header-style attempt
  per fetch.
- **Relocation matches on the `Bearer ` scheme, not the request phase.** Both
  phases put something in `Authorization`: `oauth2.Transport` writes
  `Bearer <token>` on API calls, `clientcredentials` writes `Basic <id:secret>` on
  the token request. Moving the latter sends the client credential to a header the
  token endpoint does not read — authentication fails while the relocation looks
  like it worked.
- **A base URL carrying a path prefix cannot work against Jamf's gateways, and the
  failure lands on authentication.** `TokenURL` is `baseURL + "/auth/token"` with
  no normalisation, so `https://host/api` sends the exchange to `/api/auth/token`
  → 404, and the call fails during auth rather than on the request the caller
  made. Verified across all four combinations of {new, retired} host × {bare,
  `/api`-suffixed}. Prefixes *do* work against a caller's own reverse proxy that
  mounts both the token endpoint and the namespaces beneath it — `fakeProxy` in
  `internal/client/headers_test.go` exercises exactly that. This is why
  `annotateTokenError` special-cases **404** on the token exchange and names the
  base URL rather than the WAF/IP-allowlist cause it reports for other statuses.
- **A wrong base URL presents as a total 404 with working authentication**, which
  reads as a routing regression in the SDK rather than a config error: `/auth/token`
  sits at the root on both the GA and the retired host, so the exchange still
  succeeds while every API call 404s. This is the symptom every consumer will report
  at GA, since all of them have to change the host.
- **Tokens are portable across both gateway hosts** and `/auth/token` sits at the
  root on both; `{base}/api/auth/token` is 404 on both.
- **Region isolation is real despite one shared CloudFront distribution.** All
  three regional names resolve to `d2jmnb3kwds4a0`; routing is by Host header. An
  EU credential gets 200 on `eu.` and **`401 Authentication failed`** on `us.` and
  `apac.` — the same body a wrong secret gives, so a wrong-region base URL is
  indistinguishable from a bad credential from the error alone.
- **One credential set reaches one product.** A Security Cloud client answers 403
  on `/api/pro` and a Jamf Pro client answers 403 on `/api/securitycloud`. A single
  `Client` cannot span products however many tenant IDs it holds — hence no
  per-namespace tenant override, and in Terraform a provider alias per credential.
- **Scope headers must match the credential.** An environment credential sending
  `X-Tenant-Id` gets `403 OWNERSHIP_FORBIDDEN` / *"Tenant 'x' is not part of your
  organization"*, and a tenant credential sending `X-Environment-Id` gets the
  mirror image, even when both IDs belong to the same customer.
  `TestAcceptance_EnvironmentScopeMismatch` asserts **both** directions — the
  mismatch is the half worth testing, because it is what stops a consumer treating
  the options as interchangeable spellings.
- Environment scope reaches `blueprints`, `devices`, `pro`, `proclassic`,
  `securitycloud`, `compliance-benchmarks` and `ai-governance`; read-only probes
  returned real data from all of them on 2026-08-25. `audit` is the **one** spec
  this SDK generates that declares `X-Environment-Id` on every operation; every
  other declares `X-Tenant-Id`.
- **`request-context-types` absent means unrestricted, not tenant-only.** It is
  absent for jamf-pro, securitycloud and blueprints, and present only on `account`
  (`[organization]`) and `audit` (`[environment]`). Two traps while establishing
  that: `prod/api-products/pro/` is the **`/ui/jamfpro/`** definition and has no
  `header` source — the external one is
  `prod/api-products/jamf-pro/api-definitions-external-*.yaml`, found by grepping
  for `listen_path: /api/pro/`, not by directory name.
