// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type unwrapItem struct {
	ID string `json:"id"`
}

// The account list endpoints flipped from the bare array to the envelope on
// 2026-09-01 with no spec change, breaking every caller. Both shapes must
// decode, in both directions, or the next flip breaks them again.
func TestUnwrapResultList_AcceptsBothShapes(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		resultsField string
		want         int
		wantErr      bool
	}{
		{name: "envelope", body: `{"totalCount":2,"results":[{"id":"a"},{"id":"b"}]}`, want: 2},
		{name: "bare array", body: `[{"id":"a"},{"id":"b"}]`, want: 2},
		{name: "empty envelope", body: `{"totalCount":0,"results":[]}`, want: 0},
		{name: "empty array", body: `[]`, want: 0},
		{name: "envelope without the key", body: `{"totalCount":0}`, want: 0},
		{name: "null", body: `null`, want: 0},
		{name: "empty body", body: ``, want: 0},
		{name: "whitespace-padded envelope", body: "  \n{\"results\":[{\"id\":\"a\"}]}\n", want: 1},
		{name: "custom results field", body: `{"items":[{"id":"a"}]}`, resultsField: "items", want: 1},
		{name: "custom field absent from envelope", body: `{"results":[{"id":"a"}]}`, resultsField: "items", want: 0},
		{name: "results is not an array", body: `{"results":{"id":"a"}}`, wantErr: true},
		{name: "malformed", body: `{"results":`, wantErr: true},
		{name: "element type mismatch", body: `[1,2]`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := unwrapResultList[unwrapItem]([]byte(tc.body), tc.resultsField)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Fatalf("len = %d, want %d", len(got), tc.want)
			}
		})
	}
}

// []string is the other element type in use (device-group members), and it
// shares no decode path with a struct element.
func TestUnwrapResultList_StringElements(t *testing.T) {
	for _, body := range []string{`{"totalCount":2,"results":["a","b"]}`, `["a","b"]`} {
		got, err := unwrapResultList[string]([]byte(body), "")
		if err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if len(got) != 2 || got[0] != "a" {
			t.Fatalf("%s: got %#v", body, got)
		}
	}
}

func TestUnwrapResults_OverTransport(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "envelope", body: `{"totalCount":1,"results":[{"id":"a"}]}`},
		{name: "bare array", body: `[{"id":"a"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, mux := newTestClient(t)
			mux.HandleFunc("/things", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})

			got, err := UnwrapResults[unwrapItem](context.Background(), c, http.MethodGet, "/things", "")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].ID != "a" {
				t.Fatalf("got %#v", got)
			}
		})
	}
}

func TestUnwrapResults_PropagatesRequestError(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/things", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := UnwrapResults[unwrapItem](context.Background(), c, http.MethodGet, "/things", ""); err == nil {
		t.Fatal("expected error")
	}
}

// UnwrapResults goes through Transport.Do, so it inherits the whole request
// path — OAuth2, the scope header, throttling, logging, error mapping and the
// automatic 5xx retry. Nothing about it is bypassed by decoding into a
// json.RawMessage first, and this is the assertion that says so: the endpoint
// fails twice before answering, and the call still returns rows.
func TestUnwrapResults_RetriesThroughTransport(t *testing.T) {
	c, _, mux := newTestClient(t)
	c.throttle.setInterval(0)
	shrinkRetryWaits(c, 1*time.Millisecond, 2*time.Millisecond)

	var calls atomic.Int32
	mux.HandleFunc("/things-flaky", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalCount":1,"results":[{"id":"a"}]}`))
	})

	got, err := UnwrapResults[unwrapItem](context.Background(), c, http.MethodGet, "/things-flaky", "")
	if err != nil {
		t.Fatalf("UnwrapResults: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 attempts (500, 500, 200), got %d", n)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("got %#v", got)
	}
}

// The scope header is stamped in execute(), below the decode, so it travels on
// an unwrap read exactly as on any other. Pinned because UnwrapResults is the
// only generated call site that hands Do a json.RawMessage, and a codec-shaped
// change is the kind that quietly skips request setup.
func TestUnwrapResults_SendsScopeHeader(t *testing.T) {
	c, _, mux := newTestClient(t)
	WithTenantID("t-unwrap")(c)

	var seen string
	mux.HandleFunc("/things-scoped", func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Tenant-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"a"}]`))
	})

	if _, err := UnwrapResults[unwrapItem](context.Background(), c, http.MethodGet, "/things-scoped", ""); err != nil {
		t.Fatal(err)
	}
	if seen != "t-unwrap" {
		t.Errorf("X-Tenant-Id = %q, want %q", seen, "t-unwrap")
	}
}
