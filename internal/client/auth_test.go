// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2/clientcredentials"
)

// nginxForbidden is the verbatim body observed when an edge proxy blocked the
// SDK's token request from a GitHub-hosted runner on 2026-08-04.
const nginxForbidden = `<html>
<head><title>403 Forbidden</title></head>
<body>
<center><h1>403 Forbidden</h1></center>
</body>
</html>`

// spaShell stands in for the other common block shape: a 200 OK carrying the
// tenant's HTML app shell in place of the token JSON.
const spaShell = `<!doctype html>
<html lang="en"><head><title>Jamf</title></head><body><div id="root"></div></body></html>`

// tokenServer serves one canned response on the token endpoint and returns a
// config plus base client wired to it.
func tokenServer(t *testing.T, status int, contentType, body string) (*clientcredentials.Config, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("writing token response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	return &clientcredentials.Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		TokenURL:     srv.URL + "/token",
	}, srv.Client()
}

// The guidance must appear whatever status the proxy chose, since a WAF block is
// not tied to one status code.
func TestValidateCredentials_NonJSONBodyIsDiagnosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantInError []string
	}{
		{
			name:        "nginx 403",
			status:      http.StatusForbidden,
			contentType: "text/html",
			body:        nginxForbidden,
			// The body is echoed on this path, so the proxy's own words survive.
			wantInError: []string{"403", "text/html", "403 Forbidden"},
		},
		{
			name:        "200 with app shell",
			status:      http.StatusOK,
			contentType: "text/html",
			body:        spaShell,
			// x/oauth2 drops the body on the 2xx path; only the guidance remains.
			wantInError: []string{"success status with a non-JSON body"},
		},
		{
			name:        "503 with empty body",
			status:      http.StatusServiceUnavailable,
			contentType: "",
			body:        "",
			wantInError: []string{"503"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config, base := tokenServer(t, tc.status, tc.contentType, tc.body)
			err := validateCredentials(context.Background(), config, base)
			if err == nil {
				t.Fatal("expected an error from a non-JSON token response, got nil")
			}
			// The sentinel is the contract consumers branch on; the wording is
			// only for humans. Assert both, but this one is the load-bearing half.
			if !errors.Is(err, ErrUnexpectedResponse) {
				t.Errorf("expected ErrUnexpectedResponse in the chain, got: %v", err)
			}
			if !strings.Contains(err.Error(), "security policy (WAF or IP allowlist)") {
				t.Errorf("expected the WAF guidance in the error, got: %v", err)
			}
			for _, want := range tc.wantInError {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("expected %q in the error, got: %v", want, err)
				}
			}
			// Run with -v to review the wording a blocked caller actually sees.
			t.Logf("\n--- verbatim error ---\n%s\n---", err.Error())
		})
	}
}

// A genuine credential rejection is JSON and must be left alone — mislabelling it
// as a network block would send the caller hunting for a firewall that is fine.
func TestValidateCredentials_JSONRejectionIsNotDiagnosedAsWAF(t *testing.T) {
	t.Parallel()

	config, base := tokenServer(t, http.StatusUnauthorized, "application/json",
		`{"error":"invalid_client","error_description":"Client authentication failed"}`)

	err := validateCredentials(context.Background(), config, base)
	if err == nil {
		t.Fatal("expected an error from a 401 token response, got nil")
	}
	// The sentinel must NOT be set: a consumer branching on it would look up an
	// egress IP and send the user chasing a firewall over a wrong secret.
	if errors.Is(err, ErrUnexpectedResponse) {
		t.Errorf("a JSON credential rejection must not carry ErrUnexpectedResponse, got: %v", err)
	}
	if strings.Contains(err.Error(), "security policy (WAF or IP allowlist)") {
		t.Errorf("a JSON credential rejection must not be reported as a network block, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("expected the server's own error code to survive, got: %v", err)
	}
}

// The annotation must not disturb the success path.
func TestValidateCredentials_SucceedsOnJSONToken(t *testing.T) {
	t.Parallel()

	config, base := tokenServer(t, http.StatusOK, "application/json",
		`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`)

	if err := validateCredentials(context.Background(), config, base); err != nil {
		t.Fatalf("expected a valid token exchange to succeed, got: %v", err)
	}
}

func TestLooksLikeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want bool
	}{
		{"object", `{"a":1}`, true},
		{"array", `[1,2]`, true},
		{"leading whitespace", "\n\t {\"a\":1}", true},
		{"html", "<html></html>", false},
		{"empty", "", false},
		{"whitespace only", "  \n ", false},
		{"bare string", `"a"`, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeJSON([]byte(tc.body)); got != tc.want {
				t.Errorf("looksLikeJSON(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
