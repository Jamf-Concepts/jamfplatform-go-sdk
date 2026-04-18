// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "no exclude paths passes",
			cfg: Config{Specs: []SpecDef{{
				File:       "a.yaml",
				Operations: []OperationDef{{Op: "GET /v1/x", Name: "GetX"}},
			}}},
		},
		{
			name: "non-overlapping exclude passes",
			cfg: Config{Specs: []SpecDef{{
				File:         "a.yaml",
				Operations:   []OperationDef{{Op: "GET /v1/x", Name: "GetX"}},
				ExcludePaths: []string{"POST /v1/auth/token"},
			}}},
		},
		{
			name: "exact overlap fails",
			cfg: Config{Specs: []SpecDef{{
				File:         "a.yaml",
				Operations:   []OperationDef{{Op: "GET /v1/x", Name: "GetX"}},
				ExcludePaths: []string{"GET /v1/x"},
			}}},
			wantErr: "both operations and excludePaths",
		},
		{
			name: "case-insensitive method overlap fails",
			cfg: Config{Specs: []SpecDef{{
				File:         "a.yaml",
				Operations:   []OperationDef{{Op: "GET /v1/x", Name: "GetX"}},
				ExcludePaths: []string{"get /v1/x"},
			}}},
			wantErr: "both operations and excludePaths",
		},
		{
			name: "duplicate exclude entry fails",
			cfg: Config{Specs: []SpecDef{{
				File:         "a.yaml",
				ExcludePaths: []string{"POST /v1/auth/token", "POST /v1/auth/token"},
			}}},
			wantErr: "duplicate entry in excludePaths",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfig(tc.cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNormalizeOpKey(t *testing.T) {
	cases := map[string]string{
		"GET /v1/x":    "GET /v1/x",
		"get /v1/x":    "GET /v1/x",
		"  GET  /v1/x": "GET /v1/x",
		"POST  /a/b":   "POST /a/b",
	}
	for in, want := range cases {
		if got := normalizeOpKey(in); got != want {
			t.Errorf("normalizeOpKey(%q) = %q, want %q", in, got, want)
		}
	}
}
