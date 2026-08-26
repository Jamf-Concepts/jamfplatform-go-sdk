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
// Keys are "METHOD /suffix", matched against the tail of the request path.
var deniedPathSuffixes = map[string]string{
	"POST /auth/token":            "authentication is managed by the platform gateway; the SDK handles OAuth2 client credentials automatically",
	"POST /auth/keep-alive":       "token lifecycle is managed by the platform gateway",
	"POST /auth/invalidate-token": "token lifecycle is managed by the platform gateway",
	"GET /auth/current":           "not exposed via the platform gateway",
	"POST /oauth/token":           "authentication is managed by the platform gateway; the SDK handles OAuth2 client credentials automatically",
}

// checkDeniedPath returns a formatted error if method+fullURL targets a denied
// API path. Matching is on the tail of the path, so it applies uniformly across
// namespace and version segments.
//
// This used to key on the URL tail after /tenant/{id}, which silently stopped
// matching anything when scoping moved from the path to a request header: no URL
// the SDK builds contains /tenant/ any more, so every denied path was allowed
// through. The guard failed open, and its tests kept passing because their
// fixture URLs still carried the old segment. Matching the suffix directly has
// no such dependency on the surrounding shape, and still catches the legacy
// path form that the gateway accepts during the transition.
//
// Each key's suffix begins with "/", which is what stops a path ending in
// "/xauth/token" matching "/auth/token".
func checkDeniedPath(method, fullURL string) error {
	path := fullURL
	if q := strings.Index(path, "?"); q >= 0 {
		path = path[:q]
	}
	for key, reason := range deniedPathSuffixes {
		keyMethod, suffix, ok := strings.Cut(key, " ")
		if !ok || keyMethod != method || !strings.HasSuffix(path, suffix) {
			continue
		}
		return fmt.Errorf("jamfplatform: path not supported by SDK: %s %s — %s", method, suffix, reason)
	}
	return nil
}
