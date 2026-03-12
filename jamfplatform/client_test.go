// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"testing"
)

func TestNewClient_DefaultUserAgent(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if got := c.BaseURL(); got != "https://example.com" {
		t.Errorf("BaseURL() = %q, want %q", got, "https://example.com")
	}
}

func TestNewClient_WithUserAgent(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret", WithUserAgent("custom/1.0"))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestNewClient_EmptyUserAgent(t *testing.T) {
	// Empty user agent should keep the default
	c := NewClient("https://example.com", "id", "secret", WithUserAgent(""))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	c := NewClient("https://example.com", "id", "secret", WithHTTPClient(nil))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestNullableString_Marshal(t *testing.T) {
	tests := []struct {
		name     string
		ns       NullableString
		expected string
	}{
		{"value", NullableString{Value: "hello"}, `"hello"`},
		{"null", NullableString{IsNull: true}, `null`},
		{"empty value", NullableString{Value: ""}, `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ns.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.expected {
				t.Errorf("MarshalJSON() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestNewNullableString(t *testing.T) {
	ns := NewNullableString("test")
	if ns.Value != "test" || ns.IsNull {
		t.Errorf("NewNullableString = %+v, want Value=test IsNull=false", ns)
	}
}

func TestNewNullableStringNull(t *testing.T) {
	ns := NewNullableStringNull()
	if !ns.IsNull {
		t.Error("NewNullableStringNull().IsNull = false, want true")
	}
}
