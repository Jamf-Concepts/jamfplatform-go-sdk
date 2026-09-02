// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// unwrapResults generates a call to client.UnwrapResults, which json.Unmarshals
// the body. The transport picks XML for any /proclassic/ path and a
// format=xml spec generates XML tags throughout, neither of which looks at the
// Go type — so the combination compiles, emits a passing httptest stub, and
// fails on every real call. Generation refuses it instead.
func TestValidateUnwrapCodecRejectsNonJSON(t *testing.T) {
	tests := []struct {
		name    string
		method  GoMethod
		wantErr string
	}{
		{
			name:    "classic namespace",
			method:  GoMethod{Name: "ListThings", Namespace: "proclassic", UnwrapResults: "[]Thing"},
			wantErr: "decoded as XML by the transport",
		},
		{
			name:    "xml format",
			method:  GoMethod{Name: "ListThings", Namespace: "pro", Format: "xml", UnwrapResults: "[]Thing"},
			wantErr: `"format": "xml"`,
		},
		{
			name:   "json is fine",
			method: GoMethod{Name: "ListThings", Namespace: "licensing", UnwrapResults: "[]Thing"},
		},
		{
			name:   "classic without unwrapResults is fine",
			method: GoMethod{Name: "GetThing", Namespace: "proclassic", ResponseType: "Thing"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUnwrapCodec("spec test.json", []GoMethod{tc.method})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
			// The message has to name the operation and the way out, or it
			// sends whoever hits it back into the generator to find out why.
			if !strings.Contains(err.Error(), "ListThings") || !strings.Contains(err.Error(), "unwrapResults is JSON-only") {
				t.Errorf("error is not actionable: %v", err)
			}
		})
	}
}
