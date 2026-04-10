// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TokenCache persists OAuth2 tokens across process restarts.
type TokenCache interface {
	Load(key string) (token string, expiresAt time.Time, ok bool)
	Store(key string, token string, expiresAt time.Time) error
}

// FileTokenCache persists tokens to disk as JSON files.
type FileTokenCache struct {
	dir string
}

// NewFileTokenCache creates a FileTokenCache that stores tokens in the given directory.
func NewFileTokenCache(dir string) *FileTokenCache {
	return &FileTokenCache{dir: dir}
}

type fileCacheEntry struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Load reads a cached token from disk.
func (c *FileTokenCache) Load(key string) (string, time.Time, bool) {
	data, err := os.ReadFile(c.path(key))
	if err != nil {
		return "", time.Time{}, false
	}
	var entry fileCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", time.Time{}, false
	}
	return entry.AccessToken, entry.ExpiresAt, true
}

// Store writes a token to disk.
func (c *FileTokenCache) Store(key string, token string, expiresAt time.Time) error {
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return fmt.Errorf("creating token cache directory: %w", err)
	}
	data, err := json.Marshal(fileCacheEntry{AccessToken: token, ExpiresAt: expiresAt})
	if err != nil {
		return fmt.Errorf("marshalling cached token: %w", err)
	}
	if err := os.WriteFile(c.path(key), data, 0600); err != nil {
		return fmt.Errorf("writing cached token: %w", err)
	}
	return nil
}

// CacheKey computes a deterministic cache key from a base URL and client ID.
func CacheKey(baseURL, clientID string) string {
	h := sha256.Sum256([]byte(baseURL + "\x00" + clientID))
	return fmt.Sprintf("%x", h)
}

func (c *FileTokenCache) path(key string) string {
	return filepath.Join(c.dir, "jamfplatform-token-"+key)
}
