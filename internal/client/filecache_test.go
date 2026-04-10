// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTokenCache_StoreAndLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := NewFileTokenCache(dir)

	expiresAt := time.Now().Add(1 * time.Hour).Truncate(time.Millisecond)
	if err := cache.Store("key1", "test-token", expiresAt); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	token, exp, ok := cache.Load("key1")
	if !ok {
		t.Fatal("expected Load to return ok=true")
	}
	if token != "test-token" {
		t.Errorf("expected token %q, got %q", "test-token", token)
	}
	if !exp.Equal(expiresAt) {
		t.Errorf("expected expiresAt %v, got %v", expiresAt, exp)
	}
}

func TestFileTokenCache_Load_MissingFile(t *testing.T) {
	t.Parallel()

	cache := NewFileTokenCache(t.TempDir())

	_, _, ok := cache.Load("nonexistent")
	if ok {
		t.Fatal("expected Load to return ok=false for missing file")
	}
}

func TestFileTokenCache_Load_CorruptedJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := NewFileTokenCache(dir)

	path := filepath.Join(dir, "jamfplatform-token-corrupt")
	if err := os.WriteFile(path, []byte("{invalid"), 0600); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}

	_, _, ok := cache.Load("corrupt")
	if ok {
		t.Fatal("expected Load to return ok=false for corrupted JSON")
	}
}

func TestFileTokenCache_Store_CreatesDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "cache")
	cache := NewFileTokenCache(dir)

	if err := cache.Store("key1", "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected path to be a directory")
	}
}

func TestFileTokenCache_Store_FilePermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := NewFileTokenCache(dir)

	if err := cache.Store("key1", "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "jamfplatform-token-key1"))
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if got := info.Mode().Perm(); got != fs.FileMode(0600) {
		t.Errorf("expected file mode 0600, got %o", got)
	}
}

func TestFileTokenCache_Overwrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := NewFileTokenCache(dir)

	exp1 := time.Now().Add(1 * time.Hour).Truncate(time.Millisecond)
	if err := cache.Store("key1", "first", exp1); err != nil {
		t.Fatalf("first Store failed: %v", err)
	}

	exp2 := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)
	if err := cache.Store("key1", "second", exp2); err != nil {
		t.Fatalf("second Store failed: %v", err)
	}

	token, exp, ok := cache.Load("key1")
	if !ok {
		t.Fatal("expected Load to return ok=true")
	}
	if token != "second" {
		t.Errorf("expected token %q, got %q", "second", token)
	}
	if !exp.Equal(exp2) {
		t.Errorf("expected expiresAt %v, got %v", exp2, exp)
	}
}

func TestCacheKey_Deterministic(t *testing.T) {
	t.Parallel()

	key1 := CacheKey("https://example.com", "client-123")
	key2 := CacheKey("https://example.com", "client-123")
	if key1 != key2 {
		t.Errorf("expected identical keys, got %q and %q", key1, key2)
	}
}

func TestCacheKey_DifferentInputs(t *testing.T) {
	t.Parallel()

	key1 := CacheKey("https://a.com", "id1")
	key2 := CacheKey("https://b.com", "id1")
	key3 := CacheKey("https://a.com", "id2")
	if key1 == key2 {
		t.Error("expected different keys for different base URLs")
	}
	if key1 == key3 {
		t.Error("expected different keys for different client IDs")
	}
}
