// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// runSuffix computes a unique suffix (epoch timestamp) once for the entire test run.
var runSuffix = sync.OnceValue(func() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
})

// accTraceOpts returns the diagnostic client options every acceptance client
// constructor spreads, so one variable turns each on for the whole suite:
//
//	JAMFPLATFORM_ACC_TRACE=1      print every request and response
//	JAMFPLATFORM_ACC_FAST_RETRY=1 shorten the retry backoff (see accRetryOpts)
//	JAMFPLATFORM_ACC_TRACE_MAX=n  per-body byte cap; 0 for none
//
//	JAMFPLATFORM_ACC_TRACE=1 JAMFPLATFORM_ACC_FAST_RETRY=1 \
//	  go test -v -tags acceptance -run TestAcceptance_Account ./jamfplatform/
//
// Off by default and free when unset: with no logger installed the transport
// skips the calls entirely rather than formatting and discarding.
//
// Output goes to stderr rather than t.Logf because the logger has no *testing.T
// to attach to, and because stderr streams as the requests happen — the point of
// tracing is watching a hang or a retry storm unfold, which buffered per-test
// output cannot show.
//
// -v is REQUIRED for that streaming: without it `go test` buffers the test
// binary's output and shows it only for a failing test, so a trace of a passing
// test is swallowed and a trace of a hanging one appears only once it ends.
func accTraceOpts() []jamfplatform.Option {
	var opts []jamfplatform.Option
	if os.Getenv("JAMFPLATFORM_ACC_TRACE") != "" {
		opts = append(opts, jamfplatform.WithLogger(&accTracer{max: traceBodyLimit()}))
	}
	opts = append(opts, accRetryOpts()...)
	return opts
}

// accRetryOpts shortens the retry backoff when JAMFPLATFORM_ACC_FAST_RETRY is
// set. It is opt-in rather than the default because the production timing is
// part of what the suite exercises.
//
// Why it is worth having: the production policy is RetryMax=4 with
// RetryWaitMin=1s and RetryWaitMax=60s, and retryablehttp's
// RateLimitLinearJitterBackoff treats those two as the *jitter range*, not as
// (initial, cap) — the wait is (1s + rand*59s) * attemptNum, capped at 60s. So
// the first retry alone waits a median of ~30s and four retries total a median
// of ~184s, up to 240s. An endpoint that returns a persistent 502 in 1.5s (which
// GET /api/sso/v1/connections currently does) therefore takes about three
// minutes to surface, with no output in between: retries happen inside the
// retryablehttp client, below the SDK's Logger, so a trace shows one request line
// and then silence. That reads as a hang, and it was reported as one.
//
// 50ms/500ms/2 keeps a retry in the loop — so a genuinely transient failure is
// still absorbed — while bounding the wait to about a second.
func accRetryOpts() []jamfplatform.Option {
	if os.Getenv("JAMFPLATFORM_ACC_FAST_RETRY") == "" {
		return nil
	}
	return []jamfplatform.Option{jamfplatform.WithRetryPolicy(50*time.Millisecond, 500*time.Millisecond, 2)}
}

// traceBodyLimit is the per-body byte cap, overridable with
// JAMFPLATFORM_ACC_TRACE_MAX. It is deliberately generous: truncation can hide
// the exact field a trace was turned on to find, so the default only guards
// against a genuinely huge list body (the account licence list is ~245 rows)
// rather than trying to keep output tidy. Set it to 0 for no cap.
func traceBodyLimit() int {
	if v := os.Getenv("JAMFPLATFORM_ACC_TRACE_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 16384
}

// accTracer prints each request and response as it happens.
//
// Two precautions, because a trace is routinely pasted into a ticket or a chat.
// Headers are printed from a fixed allowlist rather than filtered, so
// Authorization — which LogResponse receives in full, carrying a bearer token
// live for the next 900 seconds — can never be printed by accident, and a header
// added upstream tomorrow cannot leak either. Request and response bodies have
// credential-shaped members replaced by name, which is best-effort by nature: a
// secret under an unrecognised key still prints, so treat a trace as sensitive
// regardless.
type accTracer struct {
	max int
	mu  sync.Mutex // one request's two lines must not interleave with another's
}

// secretBodyKeys are JSON members whose values are replaced before printing.
// Matched case-insensitively on the key. clientSecret and password are the ones
// the suite actually sends — UEM Connect connector creation and the Pro
// credential minting it does — but the list covers the obvious neighbours so a
// new endpoint does not quietly start logging a secret.
var secretBodyKeys = []string{
	"clientsecret", "client_secret", "password", "secret", "privatekey",
	"private_key", "token", "accesstoken", "access_token", "refreshtoken",
	"refresh_token", "apikey", "api_key", "passphrase", "psk",
}

func (l *accTracer) LogRequest(_ context.Context, method, url string, body []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(os.Stderr, "\n--> %s %s\n", method, url)
	if len(body) > 0 {
		fmt.Fprintf(os.Stderr, "    body: %s\n", l.render(redactSecrets(body)))
	}
}

func (l *accTracer) LogResponse(_ context.Context, statusCode int, headers http.Header, body []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(os.Stderr, "<-- %d\n", statusCode)

	// Response headers carry the facts that explain a failure a body cannot:
	// the traceId to quote to Jamf Support, the Deprecation notice, and the
	// Content-Encoding that determines whether href survives (see CLAUDE.md).
	for _, h := range []string{"X-Tyk-Trace-Id", "X-B3-Traceid", "Deprecation", "Sunset", "Link", "Content-Type", "Content-Encoding", "Retry-After"} {
		if v := headers.Get(h); v != "" {
			fmt.Fprintf(os.Stderr, "    %s: %s\n", h, v)
		}
	}
	if len(body) > 0 {
		fmt.Fprintf(os.Stderr, "    body: %s\n", l.render(redactSecrets(body)))
	}
}

// render indents a JSON body so it is readable, falls back to the raw bytes when
// it is not JSON (Classic is XML, and a gateway or WAF error page is HTML), and
// applies the byte cap last so the cap is measured against what is printed.
func (l *accTracer) render(body []byte) string {
	out := body
	var buf bytes.Buffer
	if json.Indent(&buf, body, "    ", "  ") == nil {
		out = buf.Bytes()
	}
	if l.max > 0 && len(out) > l.max {
		return fmt.Sprintf("%s\n    … truncated at %d of %d bytes (raise JAMFPLATFORM_ACC_TRACE_MAX, or set it to 0)", out[:l.max], l.max, len(out))
	}
	return string(out)
}

// redactSecrets replaces the values of credential-shaped JSON members. It
// re-encodes through a generic map, so a body that is not JSON object-shaped is
// returned untouched — including Classic's XML, which the suite does send.
//
// Returning the original on any failure is deliberate: a redaction pass that
// silently dropped a body would make tracing useless exactly when it matters.
func redactSecrets(body []byte) []byte {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return body
	}
	if !redactInto(v) {
		return body
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// redactInto walks maps and slices in place, reporting whether it changed
// anything so the caller can skip a pointless re-encode.
func redactInto(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			lk := strings.ToLower(k)
			if slices.Contains(secretBodyKeys, lk) {
				// A null stays null: replacing it would make a trace claim the
				// request carried a secret it did not send.
				if inner != nil {
					t[k] = "***REDACTED***"
					changed = true
				}
				continue
			}
			if redactInto(inner) {
				changed = true
			}
		}
	case []any:
		for _, inner := range t {
			if redactInto(inner) {
				changed = true
			}
		}
	}
	return changed
}

// errAccCredsUnset marks the one condition under which skipping the whole suite
// is legitimate: no tenant was configured, as on a local run or a fork PR.
//
// Every other error from initAcceptanceClient means credentials WERE supplied and
// the tenant refused them, which must fail. Conflating the two is how a total
// auth outage reports success: on 2026-08-04 a WAF block made all 146 scoped
// tests skip, the package still printed `ok`, and the acceptance check went green
// having executed zero assertions against the tenant.
var errAccCredsUnset = errors.New("acceptance credentials not configured")

// initAcceptanceClient creates and validates the singleton acceptance client
// once, preferring environment scope over tenant scope.
//
// Environment is becoming the predominant Jamf scope, with tenant the fallback,
// so the suite follows the same order: when a complete environment credential
// set is configured it is used, otherwise the tenant set is. A credential is
// minted against one scope and the header must match it, so this is a choice
// between two integrations rather than two IDs for one — see
// WithEnvironmentID.
//
// The consequence worth knowing before configuring the environment secrets:
// every test using accClient switches scope at that moment, and an environment
// credential generally reaches a different Jamf Pro instance with different
// data. Expect fixture-dependent tests to change behaviour, and read
// accScopeInUse in the log before concluding that the secrets broke CI.
var initAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	envBase, envID := os.Getenv("JAMFPLATFORM_ENV_BASE_URL"), os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID")
	envClient, envSecret := os.Getenv("JAMFPLATFORM_ENV_CLIENT_ID"), os.Getenv("JAMFPLATFORM_ENV_CLIENT_SECRET")
	if envBase == "" {
		envBase = os.Getenv("JAMFPLATFORM_BASE_URL")
	}
	if envBase != "" && envClient != "" && envSecret != "" && envID != "" {
		c := jamfplatform.NewClient(envBase, envClient, envSecret,
			append(accTraceOpts(), jamfplatform.WithEnvironmentID(envID))...)
		if err := c.ValidateCredentials(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to validate environment-scoped credentials: %w", err)
		}
		accScopeInUse = "environment " + envID
		return c, nil
	}

	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	clientID := os.Getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_CLIENT_SECRET")
	tenantID := os.Getenv("JAMFPLATFORM_TENANT_ID")

	if baseURL == "" || clientID == "" || clientSecret == "" || tenantID == "" {
		return nil, fmt.Errorf("%w: set JAMFPLATFORM_BASE_URL, JAMFPLATFORM_CLIENT_ID, JAMFPLATFORM_CLIENT_SECRET, JAMFPLATFORM_TENANT_ID (or the JAMFPLATFORM_ENV_* set for environment scope)", errAccCredsUnset)
	}

	c := jamfplatform.NewClient(baseURL, clientID, clientSecret,
		append(accTraceOpts(), jamfplatform.WithTenantID(tenantID))...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}
	accScopeInUse = "tenant " + tenantID
	return c, nil
})

// accScopeInUse records which scope initAcceptanceClient settled on, so a run
// says so out loud. A silent switch between scopes would be indistinguishable
// from the tenant's data changing underneath the suite.
var accScopeInUse = "(client not yet built)"

// errAccJSCCredsUnset marks a Security Cloud acceptance run with no Security
// Cloud credentials configured. It is deliberately separate from
// errAccCredsUnset: Security Cloud is a different product with its own tenant
// ID, and a Jamf Pro API client cannot reach it at all — probed 2026-08-17, a
// Security Cloud client answers 403 BAD_PERMISSIONS on /api/pro and
// /api/devices, and the reverse holds. So the suite needs a second credential
// set, not just a second tenant ID, and a tenant configured for Jamf Pro only
// must skip these tests rather than fail them.
var errAccJSCCredsUnset = errors.New("Security Cloud acceptance credentials not configured")

// initJSCAcceptanceClient creates and validates the singleton Security Cloud
// acceptance client. JAMFPLATFORM_JSC_BASE_URL is optional and defaults to the
// Jamf Pro base URL, since both products are served by the same regional
// gateway; the credentials and tenant are not optional.
var initJSCAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_JSC_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("JAMFPLATFORM_BASE_URL")
	}
	clientID := os.Getenv("JAMFPLATFORM_JSC_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_JSC_CLIENT_SECRET")
	tenantID := os.Getenv("JAMFPLATFORM_JSC_TENANT_ID")

	if baseURL == "" || clientID == "" || clientSecret == "" || tenantID == "" {
		return nil, fmt.Errorf("%w: set JAMFPLATFORM_JSC_CLIENT_ID, JAMFPLATFORM_JSC_CLIENT_SECRET, JAMFPLATFORM_JSC_TENANT_ID (and JAMFPLATFORM_JSC_BASE_URL when it differs from JAMFPLATFORM_BASE_URL)", errAccJSCCredsUnset)
	}

	// Plain WithTenantID: this client is built from Security Cloud credentials
	// and only ever reaches /api/securitycloud, so there is nothing to scope
	// per-namespace. The per-namespace override this used to exercise went away
	// with header scoping — one credential set reaches one product (a Security
	// Cloud client answers 403 on /api/pro and vice versa), so a single Client
	// could never have served both regardless of how many tenant IDs it held.
	c := jamfplatform.NewClient(baseURL, clientID, clientSecret,
		append(accTraceOpts(), jamfplatform.WithTenantID(tenantID))...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate Security Cloud credentials: %w", err)
	}

	return c, nil
})

// accSecurityCloudClient returns a live client for the securitycloud package.
// It skips when no Security Cloud credentials are configured and FAILS when
// they are configured but rejected — the same distinction accClient draws, for
// the same reason (see errAccCredsUnset).
func accSecurityCloudClient(t *testing.T) *securitycloud.Client {
	t.Helper()
	c, err := initJSCAcceptanceClient()
	switch {
	case errors.Is(err, errAccJSCCredsUnset):
		t.Skipf("Skipping Security Cloud acceptance test: %v", err)
	case err != nil:
		t.Fatal(credentialRejectedMessage(err))
	}
	return securitycloud.New(c)
}

// errAccEnvCredsUnset marks an absent environment-scoped credential set, the
// same way errAccCredsUnset marks an absent tenant one.
var errAccEnvCredsUnset = errors.New("environment-scoped acceptance credentials not configured")

// initEnvAcceptanceClient creates and validates the singleton environment-scoped
// acceptance client.
//
// This needs its own credential set, not just a second ID: a credential is
// minted against one scope, and the gateway refuses a header that disagrees with
// it — an environment-scoped integration sending X-Tenant-Id, or a tenant-scoped
// one sending X-Environment-Id, gets 403 OWNERSHIP_FORBIDDEN even when both IDs
// belong to the same customer. Wire-verified against securitycloud in prod on
// 2026-08-25, which is what TestAcceptance_EnvironmentScope pins.
var initEnvAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_ENV_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("JAMFPLATFORM_BASE_URL")
	}
	clientID := os.Getenv("JAMFPLATFORM_ENV_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_ENV_CLIENT_SECRET")
	environmentID := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID")

	if baseURL == "" || clientID == "" || clientSecret == "" || environmentID == "" {
		return nil, fmt.Errorf("%w: set JAMFPLATFORM_ENV_CLIENT_ID, JAMFPLATFORM_ENV_CLIENT_SECRET, JAMFPLATFORM_ENVIRONMENT_ID (and JAMFPLATFORM_ENV_BASE_URL when it differs from JAMFPLATFORM_BASE_URL)", errAccEnvCredsUnset)
	}

	c := jamfplatform.NewClient(baseURL, clientID, clientSecret,
		append(accTraceOpts(), jamfplatform.WithEnvironmentID(environmentID))...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate environment-scoped credentials: %w", err)
	}
	return c, nil
})

// accEnvClient returns a live environment-scoped client. Unset credentials skip;
// supplied-but-rejected credentials fail, the same distinction accClient draws.
func accEnvClient(t *testing.T) *jamfplatform.Client {
	t.Helper()
	c, err := initEnvAcceptanceClient()
	switch {
	case errors.Is(err, errAccEnvCredsUnset):
		t.Skipf("Skipping environment-scope acceptance test: %v", err)
	case err != nil:
		t.Fatal(credentialRejectedMessage(err))
	}
	return c
}

// errAccOrgCredsUnset marks an absent organization-scoped credential set.
var errAccOrgCredsUnset = errors.New("organization-scoped acceptance credentials not configured")

// initOrgAcceptanceClient creates and validates the singleton organization-scoped
// client used by the Jamf Account tests.
//
// Organization scope is the *absence* of a scope, not a header: the gateway
// derives the organization from the token itself, so no WithTenantID or
// WithEnvironmentID is applied and Client.Scope() reports the zero kind with an
// empty ID. Passing either option would be actively wrong — it would make the
// gateway resolve a different context type and refuse the request.
//
// JAMFPLATFORM_ORG_BASE_URL is separate from JAMFPLATFORM_BASE_URL because the
// Jamf Account api-product ships only use1 api-definitions: an EU base URL
// cannot reach these endpoints at all, and the failure does not look regional.
var initOrgAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_ORG_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("JAMFPLATFORM_BASE_URL")
	}
	clientID := os.Getenv("JAMFPLATFORM_ORG_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_ORG_CLIENT_SECRET")

	if baseURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("%w: set JAMFPLATFORM_ORG_CLIENT_ID, JAMFPLATFORM_ORG_CLIENT_SECRET (and JAMFPLATFORM_ORG_BASE_URL — must be the US gateway)", errAccOrgCredsUnset)
	}

	c := jamfplatform.NewClient(baseURL, clientID, clientSecret, accTraceOpts()...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate organization-scoped credentials: %w", err)
	}
	return c, nil
})

// accOrgClient returns a live organization-scoped client. Unset credentials skip;
// supplied-but-rejected credentials fail, the same distinction accClient draws.
func accOrgClient(t *testing.T) *jamfplatform.Client {
	t.Helper()
	c, err := initOrgAcceptanceClient()
	switch {
	case errors.Is(err, errAccOrgCredsUnset):
		t.Skipf("Skipping organization-scope acceptance test: %v", err)
	case err != nil:
		t.Fatal(credentialRejectedMessage(err))
	}
	return c
}

// requireWriteOptIn skips unless the named environment variable is set. Used for
// mutations that touch shared organization or environment state, where the
// blast radius is a real customer record rather than a throwaway tenant object.
// The message names the variable so the skip is actionable rather than a dead end.
func requireWriteOptIn(t *testing.T, envVar, why string) {
	t.Helper()
	if os.Getenv(envVar) == "" {
		t.Skipf("Skipping: set %s to opt in. %s", envVar, why)
	}
}

// egressIP reports this host's public egress IP, resolved once per run because a
// rejected credential fails every test in the suite and a per-test lookup would
// add three seconds each. An empty result is itself a signal: a host that cannot
// reach checkip.amazonaws.com probably cannot reach Jamf either.
var egressIP = sync.OnceValue(func() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://checkip.amazonaws.com", nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
})

// credentialRejectedMessage builds the report to hand Jamf Support when the tenant
// refuses credentials that were supplied, mirroring what
// terraform-provider-jamfprotect emits for the same failure. The first question
// support asks about a blocked request is always "from which IP, and when", and on
// a CI runner nobody can go and check afterwards — the egress IP is gone with the
// runner. Capturing it in the failure output is the only chance to have it.
//
// The base URL is read from the environment rather than the client, which was
// never successfully constructed. In CI it renders as *** because it is a secret;
// that is fine, whoever opens the ticket has it.
func credentialRejectedMessage(err error) string {
	ip := egressIP()
	if ip == "" {
		ip = "(unable to determine — run `curl -s https://checkip.amazonaws.com` on this host)"
	}
	return fmt.Sprintf(`acceptance credentials were supplied but the tenant rejected them.
Failing rather than skipping: a skipped suite reports success while verifying nothing.

If this is an edge or WAF block rather than a bad secret, give Jamf Support:
  - Timestamp:    %s
  - Instance URL: %s
  - Egress IP:    %s

Technical details: %v`,
		time.Now().UTC().Format(time.RFC3339),
		os.Getenv("JAMFPLATFORM_BASE_URL"),
		ip,
		err)
}

// accClient returns a live Jamf Platform API client. It skips the test when no
// credentials are configured, and FAILS when credentials are configured but the
// tenant rejected them — see errAccCredsUnset for why that distinction matters.
func accClient(t *testing.T) *jamfplatform.Client {
	t.Helper()
	c, err := initAcceptanceClient()
	switch {
	case errors.Is(err, errAccCredsUnset):
		t.Skipf("Skipping acceptance test: %v", err)
	case err != nil:
		t.Fatal(credentialRejectedMessage(err))
	}
	logAccScopeOnce()
	return c
}

// logAccScopeOnce announces the scope the suite is running under, once per run.
// Which scope is active changes what data every accClient test sees, so a run
// that does not say so leaves a scope switch looking like the tenant's data
// having changed.
var logAccScopeOnce = sync.OnceFunc(func() {
	fmt.Fprintf(os.Stderr, "acceptance: running under %s scope\n", accScopeInUse)
})

// skipOnServerError skips the test if err is an API 5xx response.
// Use instead of t.Fatalf for API calls that may hit transient server bugs.
func skipOnServerError(t *testing.T, err error) {
	t.Helper()
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 500 {
		t.Skipf("Skipping due to server error: %v", err)
	}
}

// cleanupDelete registers a best-effort delete in t.Cleanup. Unlike
// `_ = pc.DeleteXxx(...)`, it surfaces delete failures via t.Logf so a
// server-side 5xx on cleanup doesn't silently leak a tenant record.
// Errors are intentionally non-fatal: one failed cleanup must not
// prevent other registered Cleanup hooks from running.
func cleanupDelete(t *testing.T, label string, fn func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := fn(); err != nil {
			t.Logf("cleanup %s: %v", label, err)
		}
	})
}

// Smart group fixture — shared across all tests that need a device group scope.

var smartGroupFixtureOnce sync.Once
var smartGroupID string
var smartGroupErr error

func smartGroupFixtureName() string {
	return "sdk-acc-fixture-" + runSuffix()
}

func requireSmartGroupFixture(t *testing.T) string {
	t.Helper()

	smartGroupFixtureOnce.Do(func() {
		// If a device group ID is provided via env var, use it directly.
		// This is useful when the device groups API is not available with the
		// current credentials (e.g. tenant-scoped credentials for blueprints/benchmarks).
		if id := os.Getenv("JAMFPLATFORM_DEVICE_GROUP_ID"); id != "" {
			smartGroupID = id
			return
		}

		c := accClient(t)
		ctx := context.Background()
		dg := devicegroups.New(c)

		groups, err := dg.ListDeviceGroups(ctx, nil, fmt.Sprintf("name==%q", smartGroupFixtureName()))
		if err != nil {
			smartGroupErr = fmt.Errorf("failed to look up fixture smart group: %w", err)
			return
		}
		for _, g := range groups {
			if g.Name == smartGroupFixtureName() {
				smartGroupID = g.ID
				return
			}
		}

		fixtureDesc := "SDK acceptance test fixture — safe to delete"
		resp, err := dg.CreateDeviceGroup(ctx, &devicegroups.DeviceGroupCreateRepresentationV1{
			Name:        smartGroupFixtureName(),
			Description: &fixtureDesc,
			DeviceType:  "COMPUTER",
			GroupType:   "SMART",
			Criteria: &[]devicegroups.DeviceGroupCriteriaRepresentationV1{
				{
					Order:          0,
					AttributeName:  "Serial Number",
					Operator:       "LIKE",
					AttributeValue: "",
					JoinType:       "AND",
				},
			},
		})
		if err != nil {
			smartGroupErr = fmt.Errorf("failed to create fixture smart group: %w", err)
			return
		}
		smartGroupID = resp.ID
	})

	if smartGroupErr != nil {
		t.Fatalf("Smart group fixture failed: %v", smartGroupErr)
	}
	return smartGroupID
}

// cleanupSmartGroupFixture deletes the shared fixture. Call from TestMain.
// Skips cleanup when the group was provided via JAMFPLATFORM_DEVICE_GROUP_ID
// since we don't own that resource.
func cleanupSmartGroupFixture() {
	if smartGroupID == "" || os.Getenv("JAMFPLATFORM_DEVICE_GROUP_ID") != "" {
		return
	}
	if c, err := initAcceptanceClient(); err == nil {
		_ = devicegroups.New(c).DeleteDeviceGroup(context.Background(), smartGroupID)
	}
}

// Benchmark cleanup helpers — handle async sync states and stuck DELETING.

func ensureBenchmarkDeletedByID(t *testing.T, c *jamfplatform.Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	cb := compliancebenchmarks.New(c)
	waitForBenchmarkSyncState(t, c, ctx, benchmarkID)

	if err := cb.DeleteBenchmark(ctx, benchmarkID); err != nil {
		t.Logf("Warning: failed to delete benchmark %s: %v", benchmarkID, err)
		return
	}
	t.Logf("Delete issued for benchmark %s", benchmarkID)

	deleteCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	lastDelete := time.Now()
	err := jamfplatform.PollUntil(deleteCtx, 2*time.Second, func(_ context.Context) (bool, error) {
		if _, found := benchmarkSyncState(c, ctx, benchmarkID); !found {
			t.Logf("Benchmark %s fully deleted", benchmarkID)
			return true, nil
		}
		if time.Since(lastDelete) > 20*time.Second {
			lastDelete = time.Now()
			t.Logf("Retrying delete for stuck benchmark %s", benchmarkID)
			_ = cb.DeleteBenchmark(ctx, benchmarkID)
		}
		return false, nil
	})
	if err != nil {
		t.Logf("Warning: benchmark %s still present after 2m", benchmarkID)
	}
}

func waitForBenchmarkSyncState(t *testing.T, c *jamfplatform.Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	err := jamfplatform.PollUntil(syncCtx, 3*time.Second, func(_ context.Context) (bool, error) {
		state, found := benchmarkSyncState(c, ctx, benchmarkID)
		if !found {
			t.Logf("Benchmark %s not found, may already be deleted", benchmarkID)
			return true, nil
		}
		if state == "SYNCED" || state == "FAILED" {
			t.Logf("Benchmark %s reached state %s", benchmarkID, state)
			return true, nil
		}
		t.Logf("Benchmark %s in state %q, waiting for SYNCED", benchmarkID, state)
		return false, nil
	})
	if err != nil {
		t.Logf("Warning: benchmark %s did not reach SYNCED after 2m", benchmarkID)
	}
}

func benchmarkSyncState(c *jamfplatform.Client, ctx context.Context, benchmarkID string) (string, bool) {
	cb := compliancebenchmarks.New(c)
	benchmarks, err := cb.ListBenchmarks(ctx)
	if err != nil {
		return "", false
	}
	for _, b := range benchmarks.Benchmarks {
		if b.ID == benchmarkID {
			return b.SyncState, true
		}
	}
	return "", false
}
