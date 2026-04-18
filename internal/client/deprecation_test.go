// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strings"
	"testing"
)

func TestDeprecationHeader_LogsOncePerPath(t *testing.T) {
	c, srv, mux := newTestClient(t)

	mux.HandleFunc("/api/old", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Deprecation", `date="2026-06-01"`)
		w.WriteHeader(http.StatusOK)
	})

	var buf bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	})

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := c.Do(ctx, http.MethodGet, "/api/old", nil, nil); err != nil {
			t.Fatalf("Do %d: %v", i, err)
		}
	}

	lines := strings.Count(buf.String(), "Deprecation header")
	if lines != 1 {
		t.Errorf("expected 1 deprecation log across 3 calls, got %d — output:\n%s", lines, buf.String())
	}
	if !strings.Contains(buf.String(), `date="2026-06-01"`) {
		t.Errorf("log missing deprecation date, got:\n%s", buf.String())
	}
	_ = srv
}
