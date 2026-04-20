// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// FileCookieJar is a cookie jar backed by a JSON file so cookies persist
// across process invocations. Layered on top of the stdlib in-memory
// cookiejar.Jar; every SetCookies call mutates the jar and flushes to disk.
//
// Used primarily for CLI-style consumers that make each API call in a
// separate process — long-running callers don't need persistence since
// the in-memory jar already handles sticky-session cookies within the run.
type FileCookieJar struct {
	inner    *cookiejar.Jar
	path     string
	mu       sync.Mutex
	seenURLs map[string]*url.URL
}

type persistedCookies struct {
	URL     string         `json:"url"`
	Cookies []*http.Cookie `json:"cookies"`
}

// NewFileCookieJar opens (or creates) a persistent jar backed by path. A
// missing or unreadable file is treated as empty — the caller starts with a
// fresh jar rather than failing.
func NewFileCookieJar(path string) (*FileCookieJar, error) {
	inner, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	j := &FileCookieJar{
		inner:    inner,
		path:     path,
		seenURLs: make(map[string]*url.URL),
	}
	j.loadFromDisk()
	return j, nil
}

// SetCookies implements http.CookieJar. After forwarding to the inner jar it
// rewrites the on-disk state. Errors writing to disk are swallowed — the
// in-memory jar is authoritative for the current process regardless.
func (j *FileCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.inner.SetCookies(u, cookies)
	j.trackURLLocked(u)
	j.persistLocked()
}

// Cookies implements http.CookieJar. The inner jar is goroutine-safe on its
// own, but we take the same lock SetCookies holds so reads are consistent
// with in-flight persists (no observing a half-updated jar state).
func (j *FileCookieJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.inner.Cookies(u)
}

func (j *FileCookieJar) trackURLLocked(u *url.URL) {
	if u == nil {
		return
	}
	key := u.Scheme + "://" + u.Host
	if _, ok := j.seenURLs[key]; !ok {
		j.seenURLs[key] = &url.URL{Scheme: u.Scheme, Host: u.Host}
	}
}

func (j *FileCookieJar) loadFromDisk() {
	j.mu.Lock()
	defer j.mu.Unlock()
	data, err := os.ReadFile(j.path)
	if err != nil {
		return
	}
	var entries []persistedCookies
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	for _, e := range entries {
		u, err := url.Parse(e.URL)
		if err != nil {
			continue
		}
		j.inner.SetCookies(u, e.Cookies)
		j.trackURLLocked(u)
	}
}

func (j *FileCookieJar) persistLocked() {
	entries := make([]persistedCookies, 0, len(j.seenURLs))
	for _, u := range j.seenURLs {
		cookies := j.inner.Cookies(u)
		if len(cookies) == 0 {
			continue
		}
		entries = append(entries, persistedCookies{URL: u.String(), Cookies: cookies})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return
	}
	if dir := filepath.Dir(j.path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0700)
	}
	_ = os.WriteFile(j.path, data, 0600)
}
