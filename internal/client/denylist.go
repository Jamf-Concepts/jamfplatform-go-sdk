// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"fmt"
	"strings"
)

// deniedPathSuffixes lists API paths the SDK refuses to call. The platform
// gateway forwards these requests upstream where they are rejected (403/404),
// but calling them would also bypass the gateway-managed OAuth2 authentication
// this SDK handles automatically — so the SDK fails closed with a clear error
// instead of letting the misuse hit the wire.
//
// Keys are "METHOD /suffix" where /suffix is the URL tail after /tenant/{id}.
var deniedPathSuffixes = map[string]string{
	"POST /auth/token":            "authentication is managed by the platform gateway; the SDK handles OAuth2 client credentials automatically",
	"POST /auth/keep-alive":       "token lifecycle is managed by the platform gateway",
	"POST /auth/invalidate-token": "token lifecycle is managed by the platform gateway",
	"GET /auth/current":           "not exposed via the platform gateway",
	"POST /oauth/token":           "authentication is managed by the platform gateway; the SDK handles OAuth2 client credentials automatically",
}

// checkDeniedPath returns ErrPathNotSupported if method+fullURL targets a
// denied API path. Matching uses the URL tail after /tenant/{id}, so it
// applies uniformly across namespace and version path segments.
func checkDeniedPath(method, fullURL string) error {
	_, after, ok := strings.Cut(fullURL, "/tenant/")
	if !ok {
		return nil
	}
	slash := strings.Index(after, "/")
	if slash < 0 {
		return nil
	}
	suffix := after[slash:]
	if q := strings.Index(suffix, "?"); q >= 0 {
		suffix = suffix[:q]
	}
	key := method + " " + suffix
	if reason, ok := deniedPathSuffixes[key]; ok {
		return fmt.Errorf("%w: %s %s — %s", ErrPathNotSupported, method, suffix, reason)
	}
	return nil
}
