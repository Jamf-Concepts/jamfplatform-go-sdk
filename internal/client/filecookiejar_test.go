// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
)

func TestFileCookieJar_PersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")

	first, err := NewFileCookieJar(path)
	if err != nil {
		t.Fatalf("NewFileCookieJar (first): %v", err)
	}
	u, _ := url.Parse("https://eu.apigw.jamf.com/")
	first.SetCookies(u, []*http.Cookie{
		{Name: "JSESSIONID", Value: "node-a-1234", Path: "/"},
	})

	second, err := NewFileCookieJar(path)
	if err != nil {
		t.Fatalf("NewFileCookieJar (second): %v", err)
	}
	got := second.Cookies(u)
	if len(got) != 1 {
		t.Fatalf("reloaded jar Cookies = %d, want 1", len(got))
	}
	if got[0].Name != "JSESSIONID" || got[0].Value != "node-a-1234" {
		t.Fatalf("reloaded cookie = %+v, want JSESSIONID=node-a-1234", got[0])
	}
}

func TestFileCookieJar_MissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	j, err := NewFileCookieJar(path)
	if err != nil {
		t.Fatalf("NewFileCookieJar: %v", err)
	}
	u, _ := url.Parse("https://example.com/")
	if got := j.Cookies(u); len(got) != 0 {
		t.Fatalf("empty jar Cookies = %d, want 0", len(got))
	}
}
