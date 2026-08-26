// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"
)

// svcCredential stands in for the vaulted service-account credential a reverse
// proxy demands for itself, distinct from the Jamf client credential.
const svcCredential = "Basic " + "c3ZjOnMzY3JldA==" // svc:s3cret

// capturedRequest records the headers one phase of a request pair arrived with.
type capturedRequest struct {
	path   string
	header http.Header
	form   url.Values
}

// fakeProxy stands up an httptest server that impersonates a reverse proxy
// fronting the Jamf gateway on a path prefix: it serves the OAuth2 token
// endpoint and one API endpoint, recording the headers each arrived with.
//
// This is the shape a corporate proxy deployment actually takes. Such a "proxy"
// is not a CONNECT proxy — it is a reverse proxy on a different host under a path
// prefix, which is exactly a base URL the SDK can be pointed at, so the whole
// feature is testable in-process with no network and no build tag.
func fakeProxy(t *testing.T) (baseURL string, seen func() map[string]capturedRequest) {
	t.Helper()

	var mu sync.Mutex
	got := map[string]capturedRequest{}
	record := func(phase string, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		got[phase] = capturedRequest{path: r.URL.Path, header: r.Header.Clone(), form: r.PostForm}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxypath/auth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing token request form: %v", err)
		}
		record("token", r)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-abc123",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}); err != nil {
			t.Errorf("writing token response: %v", err)
		}
	})
	mux.HandleFunc("/proxypath/api/pro/v1/buildings", func(w http.ResponseWriter, r *http.Request) {
		record("api", r)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"totalCount":0,"results":[]}`)); err != nil {
			t.Errorf("writing api response: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + "/proxypath", func() map[string]capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

// callBoth drives one token exchange and one API call through the transport.
func callBoth(t *testing.T, c *Transport) {
	t.Helper()
	var out map[string]any
	if err := c.Do(context.Background(), http.MethodGet, "/api/pro/v1/buildings", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

// The headers must reach the token exchange as well as the API call: their proxy
// demands its credential on every request, and the token fetch is the first one
// made. Wrapping the wrong client — the OAuth2 one rather than its base — passes
// an API-only assertion while leaving authentication broken.
func TestWithHeaders_AppliedToTokenExchangeAndAPICalls(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithTenantID("tenant-uuid"),
		WithHeaders(http.Header{
			"Authorization":  {svcCredential},
			"X-Routing-Tag":  {"corp-egress"},
			"X-Multi-Valued": {"one", "two"},
		}),
	)
	callBoth(t, c)

	got := seen()
	for _, phase := range []string{"token", "api"} {
		req, ok := got[phase]
		if !ok {
			t.Fatalf("%s phase never reached the proxy", phase)
		}
		if v := req.header.Get("X-Routing-Tag"); v != "corp-egress" {
			t.Errorf("%s phase: X-Routing-Tag = %q, want %q", phase, v, "corp-egress")
		}
		if v := req.header.Values("X-Multi-Valued"); len(v) != 2 || v[0] != "one" || v[1] != "two" {
			t.Errorf("%s phase: X-Multi-Valued = %v, want [one two]", phase, v)
		}
		if v := req.header.Get("Authorization"); v != svcCredential {
			t.Errorf("%s phase: Authorization = %q, want the caller's %q", phase, v, svcCredential)
		}
	}
}

// The full proxy arrangement end to end: the proxy's credential in
// Authorization, Jamf's bearer under a name the proxy chose, and the client
// credential in the token request body. Each piece is covered on its own above;
// this asserts they compose, which is what a consumer actually configures.
func TestWithHeaders_ProxyArrangementEndToEnd(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithTenantID("tenant-uuid"),
		WithHeaders(http.Header{"Authorization": {svcCredential}}),
		WithAuthorizationHeaderName("custom-header-name"),
	)
	callBoth(t, c)

	got := seen()
	tok, ok := got["token"]
	if !ok {
		t.Fatal("token phase never reached the proxy")
	}
	if v := tok.header.Get("Authorization"); v != svcCredential {
		t.Errorf("token phase: Authorization = %q, want the proxy credential %q", v, svcCredential)
	}
	if v := tok.form.Get("client_secret"); v != "csecret" {
		t.Errorf("token phase: form client_secret = %q, want the Jamf credential in the body", v)
	}

	api, ok := got["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.header.Get("Authorization"); v != svcCredential {
		t.Errorf("api phase: Authorization = %q, want the proxy credential %q", v, svcCredential)
	}
	if v := api.header.Get("Custom-Header-Name"); v != "Bearer tok-abc123" {
		t.Errorf("api phase: Custom-Header-Name = %q, want the relocated bearer", v)
	}
	if v := api.header.Get("X-Tenant-Id"); v != "tenant-uuid" {
		t.Errorf("api phase: X-Tenant-Id = %q, want the configured scope", v)
	}
}

// The bearer must move to the named header while Authorization carries the
// caller's credential — both on the same request, which is the whole point.
func TestWithAuthorizationHeaderName_RelocatesBearer(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithHeaders(http.Header{"Authorization": {svcCredential}}),
		WithAuthorizationHeaderName("custom-header-name"),
	)
	callBoth(t, c)

	api, ok := seen()["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.header.Get("Custom-Header-Name"); v != "Bearer tok-abc123" {
		t.Errorf("relocated bearer = %q, want %q", v, "Bearer tok-abc123")
	}
	if v := api.header.Get("Authorization"); v != svcCredential {
		t.Errorf("Authorization = %q, want the caller's %q", v, svcCredential)
	}
}

// Relocation must not touch the token exchange. Both phases put something in
// Authorization and only one is the bearer: oauth2.Transport writes
// "Bearer <token>" on API calls, clientcredentials writes
// "Basic <client_id:client_secret>" on the token request. Moving the latter
// sends the client credential to a header the token endpoint does not read, so
// authentication fails while the relocation looks like it worked — which is why
// headerTransport matches on the scheme rather than the phase.
func TestWithAuthorizationHeaderName_LeavesClientCredentialAlone(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithAuthorizationHeaderName("custom-header-name"),
	)
	callBoth(t, c)

	tok, ok := seen()["token"]
	if !ok {
		t.Fatal("token phase never reached the proxy")
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:csecret"))
	if v := tok.header.Get("Authorization"); v != want {
		t.Errorf("token phase: Authorization = %q, want the client credential left in place as %q", v, want)
	}
	if v := tok.header.Get("Custom-Header-Name"); v != "" {
		t.Errorf("token phase: Custom-Header-Name = %q, want empty — only a Bearer credential may be relocated", v)
	}
}

// A caller-supplied Authorization takes the header slot the client credential
// would have used, so the client credential has to move into the request body
// (RFC 6749 §2.3.1 permits both). Without this the caller's header silently
// destroys the credential and every token fetch fails — or, under x/oauth2's
// auto-detection, costs a wasted 401 round trip per fetch. This is the
// arrangement a proxy that authenticates callers itself expects.
func TestWithHeaders_AuthorizationMovesClientCredentialToBody(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithHeaders(http.Header{"Authorization": {svcCredential}}),
	)
	callBoth(t, c)

	tok, ok := seen()["token"]
	if !ok {
		t.Fatal("token phase never reached the proxy")
	}
	if v := tok.header.Get("Authorization"); v != svcCredential {
		t.Errorf("token phase: Authorization = %q, want the caller's %q", v, svcCredential)
	}
	if v := tok.form.Get("client_id"); v != "cid" {
		t.Errorf("token phase: form client_id = %q, want %q — the credential must ride in the body", v, "cid")
	}
	if v := tok.form.Get("client_secret"); v != "csecret" {
		t.Errorf("token phase: form client_secret = %q, want %q", v, "csecret")
	}
}

// Without a caller-supplied Authorization the client credential stays in the
// header, which is the form x/oauth2 and the RFC both prefer. Switching every
// client to body-style credentials would be an unnecessary change in what the
// SDK sends by default.
func TestWithHeaders_LeavesAuthStyleAloneWhenNoAuthorizationGiven(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
	)
	callBoth(t, c)

	tok, ok := seen()["token"]
	if !ok {
		t.Fatal("token phase never reached the proxy")
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:csecret"))
	if v := tok.header.Get("Authorization"); v != want {
		t.Errorf("token phase: Authorization = %q, want the default header-style credential %q", v, want)
	}
	if v := tok.form.Get("client_secret"); v != "" {
		t.Errorf("token phase: form client_secret = %q, want empty — the default must stay header-style", v)
	}
}

// A caller must not be able to override the scope headers. The gateway resolves
// the request context from them and refuses a wrong value with 403
// OWNERSHIP_FORBIDDEN — the same answer a mismatched credential gives, so a
// silent override is close to undiagnosable from the error.
func TestWithHeaders_RejectsScopeHeaders(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithTenantID("real-tenant"),
		WithHeaders(http.Header{
			"X-Tenant-Id":      {"hijacked"},
			"X-Environment-Id": {"also-hijacked"},
			"X-Allowed":        {"yes"},
		}),
	)
	callBoth(t, c)

	api, ok := seen()["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.header.Get("X-Tenant-Id"); v != "real-tenant" {
		t.Errorf("X-Tenant-Id = %q, want the configured scope %q", v, "real-tenant")
	}
	if v := api.header.Get("X-Environment-Id"); v != "" {
		t.Errorf("X-Environment-Id = %q, want empty — a tenant-scoped client must not send it", v)
	}
	if v := api.header.Get("X-Allowed"); v != "yes" {
		t.Errorf("X-Allowed = %q, want the non-reserved header to still apply", v)
	}
}

// unwrapTuned walks the RoundTripper chain to the underlying *http.Transport,
// so a test can assert the SDK's tuning survived being wrapped.
func unwrapTuned(t *testing.T, rt http.RoundTripper) *http.Transport {
	t.Helper()
	for range 10 {
		switch v := rt.(type) {
		case *headerTransport:
			rt = v.base
		case *userAgentTransport:
			rt = v.base
		case *throttleTransport:
			rt = v.base
		case *http.Transport:
			return v
		default:
			t.Fatalf("unexpected transport %T in chain", rt)
		}
	}
	t.Fatal("transport chain too deep")
	return nil
}

// The reason this option exists rather than pushing callers through
// WithHTTPClient: the tuned transport must survive. A caller-supplied client
// discards proxy-from-environment, the phase timeouts, the pool ceiling matched
// to Terraform's parallelism, and the write buffer package upload depends on —
// all silently, since nothing fails, throughput and proxy support just go.
func TestWithHeaders_PreservesTunedTransport(t *testing.T) {
	t.Parallel()

	base, _ := fakeProxy(t)
	plain := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1")
	withHeaders := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
	)

	if _, ok := withHeaders.baseClient.Transport.(*headerTransport); !ok {
		t.Fatalf("base transport = %T, want the header wrapper outermost so it runs below oauth2", withHeaders.baseClient.Transport)
	}

	want := unwrapTuned(t, plain.baseClient.Transport)
	got := unwrapTuned(t, withHeaders.baseClient.Transport)

	if got.MaxIdleConnsPerHost != want.MaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", got.MaxIdleConnsPerHost, want.MaxIdleConnsPerHost)
	}
	if got.WriteBufferSize != want.WriteBufferSize {
		t.Errorf("WriteBufferSize = %d, want %d", got.WriteBufferSize, want.WriteBufferSize)
	}
	if got.ResponseHeaderTimeout != want.ResponseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", got.ResponseHeaderTimeout, want.ResponseHeaderTimeout)
	}
	// Asserting the function identity rather than exercising it: net/http caches
	// the parsed environment in a sync.Once, so an env-var test would depend on
	// which test in the process ran first.
	if got.Proxy == nil {
		t.Fatal("Proxy is nil — HTTPS_PROXY support was dropped")
	}
	if reflect.ValueOf(got.Proxy).Pointer() != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() {
		t.Error("Proxy is not http.ProxyFromEnvironment — environment proxy support was replaced")
	}
}

// The throttle must not be applied twice. installHeaderTransport rebuilds the
// OAuth2 wrapper, and passing the throttle to that rebuild would stack a second
// gate on the chain, doubling every request's pacing interval.
func TestWithHeaders_DoesNotDoubleThrottle(t *testing.T) {
	t.Parallel()

	base, _ := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
	)

	var throttles int
	rt := c.baseClient.Transport
	for range 10 {
		switch v := rt.(type) {
		case *headerTransport:
			rt = v.base
		case *userAgentTransport:
			rt = v.base
		case *throttleTransport:
			throttles++
			rt = v.base
		default:
			if throttles != 1 {
				t.Errorf("throttle wrappers = %d, want exactly 1", throttles)
			}
			return
		}
	}
	t.Fatal("transport chain too deep")
}

// Composing with WithHTTPClient must still work: a caller may supply a client
// for an unrelated reason (a custom CA pool, a recording transport) and still
// need proxy headers, without hand-rolling the wrapper themselves.
func TestWithHeaders_ComposesWithHTTPClient(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithHTTPClient(&http.Client{Transport: http.DefaultTransport}),
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
	)
	callBoth(t, c)

	for _, phase := range []string{"token", "api"} {
		req, ok := seen()[phase]
		if !ok {
			t.Fatalf("%s phase never reached the proxy", phase)
		}
		if v := req.header.Get("X-Routing-Tag"); v != "corp-egress" {
			t.Errorf("%s phase: X-Routing-Tag = %q, want it applied over the caller's client", phase, v)
		}
	}
}

// A token cache rebuilds the OAuth2 client from the base transport, so it has to
// pick the header wrapper up rather than rebuild past it.
func TestWithHeaders_SurvivesTokenCache(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithTokenCache(&mockTokenCache{
			loadFn:  func(string) (string, time.Time, bool) { return "", time.Time{}, false },
			storeFn: func(string, string, time.Time) error { return nil },
		}, "key"),
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
	)
	callBoth(t, c)

	api, ok := seen()["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.header.Get("X-Routing-Tag"); v != "corp-egress" {
		t.Errorf("X-Routing-Tag = %q, want it to survive the token-cache rebuild", v)
	}
}

// SetUserAgent rebuilds both clients from scratch, which would drop the wrapper.
func TestWithHeaders_SurvivesSetUserAgent(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
	)
	c.SetUserAgent("ua/2")
	callBoth(t, c)

	api, ok := seen()["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.header.Get("X-Routing-Tag"); v != "corp-egress" {
		t.Errorf("X-Routing-Tag = %q, want it to survive SetUserAgent", v)
	}
	if v := api.header.Get("User-Agent"); v != "ua/2" {
		t.Errorf("User-Agent = %q, want the new value %q", v, "ua/2")
	}
}

// Documents what the SDK actually does with credentials in the base URL, because
// inline userinfo is reported to have "just worked" for the token request on a
// different SDK and the claim should not be inherited untested. net/http applies
// URL.User as Basic only when Authorization is empty, and both phases here
// already carry an Authorization — so the userinfo is silently dropped and a
// caller relying on it gets no credential and no error.
func TestURLUserinfoIsNotSentAsBasicAuth(t *testing.T) {
	t.Parallel()

	rawBase, seen := fakeProxy(t)
	u, err := url.Parse(rawBase)
	if err != nil {
		t.Fatalf("parsing base URL: %v", err)
	}
	u.User = url.UserPassword("svc", "s3cret")

	c := NewTransportWithUserAgent(u.String(), "cid", "csecret", "ua/1", WithMinRequestInterval(0))
	callBoth(t, c)

	inline := "Basic " + base64.StdEncoding.EncodeToString([]byte("svc:s3cret"))
	for _, phase := range []string{"token", "api"} {
		req, ok := seen()[phase]
		if !ok {
			t.Fatalf("%s phase never reached the proxy", phase)
		}
		if got := req.header.Get("Authorization"); got == inline {
			t.Errorf("%s phase: Authorization = %q — userinfo reached the wire; this test documented the opposite and the docs on WithHeaders need revisiting", phase, got)
		}
	}
}

// A real forward proxy, to prove absolute-URI proxying still works with the
// wrapper installed. The proxy is addressed explicitly rather than through
// HTTP_PROXY because net/http caches the environment in a sync.Once, which would
// make an env-var test depend on test ordering within the process.
func TestWithHeaders_TraversesForwardProxy(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)

	var mu sync.Mutex
	var proxied []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxied = append(proxied, r.URL.String())
		mu.Unlock()

		if !r.URL.IsAbs() {
			t.Errorf("proxy received a non-absolute URI %q, so it was not addressed as a proxy", r.URL)
			http.Error(w, "not proxied", http.StatusBadGateway)
			return
		}
		out, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		out.Header = r.Header.Clone()
		resp, err := http.DefaultTransport.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			t.Errorf("copying proxied body: %v", err)
		}
	}))
	t.Cleanup(proxy.Close)

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parsing proxy URL: %v", err)
	}
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithHTTPClient(&http.Client{Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) { return proxyURL, nil },
		}}),
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
	)
	callBoth(t, c)

	mu.Lock()
	hits := len(proxied)
	mu.Unlock()
	if hits < 2 {
		t.Fatalf("forward proxy saw %d requests, want both the token exchange and the API call", hits)
	}
	for _, phase := range []string{"token", "api"} {
		req, ok := seen()[phase]
		if !ok {
			t.Fatalf("%s phase never reached the origin through the proxy", phase)
		}
		if v := req.header.Get("X-Routing-Tag"); v != "corp-egress" {
			t.Errorf("%s phase: X-Routing-Tag = %q, want it to survive proxying", phase, v)
		}
	}
}

// Relocating the bearer onto Authorization is refused rather than obeyed. The
// header is both the source and the target, and RoundTrip sets the target before
// deleting the source, so honouring the request would delete the credential and
// leave every API call unauthenticated — a 401 that reads exactly like a wrong
// client secret. Omitting the option is how a caller says "leave it there".
func TestWithAuthorizationHeaderName_RefusesAuthorization(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithAuthorizationHeaderName("authorization"),
	)
	callBoth(t, c)

	api, ok := seen()["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.header.Get("Authorization"); v != "Bearer tok-abc123" {
		t.Errorf("Authorization = %q, want the bearer left in place as %q", v, "Bearer tok-abc123")
	}
}

// The scope headers are refused as a relocation target for the reason WithHeaders
// refuses them as a value, reached the other way round: headerTransport runs
// below setScopeHeader, so the bearer would overwrite the scope the SDK just
// stamped and the gateway would answer 403 OWNERSHIP_FORBIDDEN — the same answer
// a mismatched credential gives, naming the wrong cause.
func TestWithAuthorizationHeaderName_RefusesScopeHeaders(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"X-Tenant-Id", "x-environment-id"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			base, seen := fakeProxy(t)
			c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
				WithMinRequestInterval(0),
				WithTenantID("real-tenant"),
				WithAuthorizationHeaderName(name),
			)
			callBoth(t, c)

			api, ok := seen()["api"]
			if !ok {
				t.Fatal("api phase never reached the proxy")
			}
			if v := api.header.Get("X-Tenant-Id"); v != "real-tenant" {
				t.Errorf("X-Tenant-Id = %q, want the configured scope %q", v, "real-tenant")
			}
			if v := api.header.Get("Authorization"); v != "Bearer tok-abc123" {
				t.Errorf("Authorization = %q, want the bearer left in place as %q", v, "Bearer tok-abc123")
			}
		})
	}
}

// Every scope kind that travels in a header must be in reservedHeaders. Adding a
// third ScopeKind without extending the guard would leave its header
// caller-writable, and the lookup is canonicalised so a renamed header whose
// spelling Go canonicalises differently fails here rather than failing open.
func TestReservedHeadersCoversEveryScopeHeader(t *testing.T) {
	t.Parallel()

	for kind := range ScopeKind(8) {
		h := kind.ScopeHeader()
		if h == "" {
			continue
		}
		if !reservedHeaders[http.CanonicalHeaderKey(h)] {
			t.Errorf("ScopeKind(%d).ScopeHeader() = %q is not in reservedHeaders — WithHeaders would let a caller override it", kind, h)
		}
	}
}

// SetHTTPClient replaces baseClient outright, so it has to re-install the header
// transport for the same reason SetUserAgent does. Without that a caller's
// headers and the bearer relocation silently stop being applied from the call
// onwards, with no error anywhere.
func TestWithHeaders_SurvivesSetHTTPClient(t *testing.T) {
	t.Parallel()

	base, seen := fakeProxy(t)
	c := NewTransportWithUserAgent(base, "cid", "csecret", "ua/1",
		WithMinRequestInterval(0),
		WithHeaders(http.Header{"X-Routing-Tag": {"corp-egress"}}),
		WithAuthorizationHeaderName("custom-header-name"),
	)
	c.SetHTTPClient(&http.Client{Transport: newTunedTransport(), Timeout: 10 * time.Second})
	callBoth(t, c)

	api, ok := seen()["api"]
	if !ok {
		t.Fatal("api phase never reached the proxy")
	}
	if v := api.header.Get("X-Routing-Tag"); v != "corp-egress" {
		t.Errorf("X-Routing-Tag = %q, want %q — SetHTTPClient dropped the header transport", v, "corp-egress")
	}
	if v := api.header.Get("Custom-Header-Name"); v != "Bearer tok-abc123" {
		t.Errorf("relocated bearer = %q, want %q", v, "Bearer tok-abc123")
	}
}
