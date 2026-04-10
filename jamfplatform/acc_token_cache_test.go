// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"context"
	"os"
	"testing"
)

func TestAcceptance_FileTokenCache(t *testing.T) {
	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	clientID := os.Getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_CLIENT_SECRET")
	tenantID := os.Getenv("JAMFPLATFORM_TENANT_ID")

	if baseURL == "" || clientID == "" || clientSecret == "" || tenantID == "" {
		t.Skip("missing required environment variables")
	}

	cacheDir := t.TempDir()
	ctx := context.Background()

	client1 := NewClient(baseURL, clientID, clientSecret,
		WithTenantID(tenantID),
		WithFileTokenCache(cacheDir),
	)

	tok1, err := client1.AccessToken(ctx)
	if err != nil {
		t.Fatalf("first AccessToken call: %v", err)
	}
	if tok1.AccessToken == "" {
		t.Fatal("first AccessToken: expected non-empty token")
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("reading cache dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 cache file, got %d", len(entries))
	}
	t.Logf("Cache file: %s", entries[0].Name())

	client2 := NewClient(baseURL, clientID, clientSecret,
		WithTenantID(tenantID),
		WithFileTokenCache(cacheDir),
	)

	tok2, err := client2.AccessToken(ctx)
	if err != nil {
		t.Fatalf("second AccessToken call: %v", err)
	}
	if tok2.AccessToken != tok1.AccessToken {
		t.Error("expected second client to return the same cached token")
	}

	devices, err := client2.ListDevices(ctx, nil, "")
	if err != nil {
		t.Fatalf("ListDevices with cached token: %v", err)
	}
	t.Logf("Listed %d devices using cached token", len(devices))
}
