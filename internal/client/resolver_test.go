// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ResolveByNameFiltered
// ---------------------------------------------------------------------------

func TestResolveByNameFiltered_Match(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		// FormatArgument only quotes when the value contains spaces/punct;
		// "alpha" travels unquoted, which is still valid RSQL.
		filter := r.URL.Query().Get("filter")
		if filter != `name==alpha` {
			t.Errorf("filter = %q, want name==alpha", filter)
		}
		if ps := r.URL.Query().Get("page-size"); ps != "2" {
			t.Errorf("page-size = %q, want 2", ps)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalCount": 1,
			"results": []map[string]any{
				{"id": "bp-1", "name": "alpha"},
			},
		})
	})

	id, raw, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", "alpha")
	if err != nil {
		t.Fatalf("ResolveByNameFiltered: %v", err)
	}
	if id != "bp-1" {
		t.Errorf("id = %q, want bp-1", id)
	}
	var got struct{ ID, Name string }
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("decoded name = %q, want alpha", got.Name)
	}
}

func TestResolveByNameFiltered_NotFound(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalCount": 0,
			"results":    []map[string]any{},
		})
	})

	_, _, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", "missing")
	if err == nil {
		t.Fatalf("expected error for missing resource")
	}
	apiErr := AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("expected *APIResponseError, got %T: %v", err, err)
	}
	if !apiErr.HasStatus(http.StatusNotFound) {
		t.Errorf("status = %d, want 404", apiErr.StatusCode)
	}
}

func TestResolveByNameFiltered_Ambiguous(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalCount": 2,
			"results": []map[string]any{
				{"id": "bp-1", "name": "dup"},
				{"id": "bp-2", "name": "dup"},
			},
		})
	})

	_, _, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", "dup")
	if err == nil {
		t.Fatalf("expected error for duplicate match")
	}
	var amErr *AmbiguousMatchError
	if !errors.As(err, &amErr) {
		t.Fatalf("expected *AmbiguousMatchError, got %T: %v", err, err)
	}
	if amErr.Name != "dup" {
		t.Errorf("Name = %q, want dup", amErr.Name)
	}
	if len(amErr.Matches) != 2 || amErr.Matches[0] != "bp-1" || amErr.Matches[1] != "bp-2" {
		t.Errorf("Matches = %v, want [bp-1 bp-2]", amErr.Matches)
	}
}

// Server returns the same resource twice (same ID) — should resolve
// successfully, not report ambiguity.
func TestResolveByNameFiltered_DuplicateSameID(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalCount": 2,
			"results": []map[string]any{
				{"id": "bp-1", "name": "dup"},
				{"id": "bp-1", "name": "dup"},
			},
		})
	})

	id, _, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", "dup")
	if err != nil {
		t.Fatalf("expected success for same-ID duplicates, got: %v", err)
	}
	if id != "bp-1" {
		t.Errorf("id = %q, want bp-1", id)
	}
}

func TestResolveByNameFiltered_ServerError(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"code":"OOPS","description":"server broke"}]}`))
	})

	_, _, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", "alpha")
	if err == nil {
		t.Fatalf("expected error for 500")
	}
	apiErr := AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(http.StatusInternalServerError) {
		t.Fatalf("expected APIResponseError(500), got %v", err)
	}
}

func TestResolveByNameFiltered_EmptyName(t *testing.T) {
	c, _, _ := newTestClient(t)
	_, _, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", "")
	if err == nil {
		t.Fatalf("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name must not be empty") {
		t.Errorf("error = %v, want contains 'name must not be empty'", err)
	}
}

func TestResolveByNameFiltered_NameWithQuotes(t *testing.T) {
	c, _, mux := newTestClient(t)
	var gotFilter string
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		gotFilter = r.URL.Query().Get("filter")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "bp-1", "name": `has"quote`},
			},
		})
	})

	_, _, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", `has"quote`)
	if err != nil {
		t.Fatalf("ResolveByNameFiltered: %v", err)
	}
	// FormatArgument escapes " as \" and wraps the argument when it contains
	// punctuation. The server-side filter string should round-trip through
	// url.QueryEscape correctly.
	if !strings.Contains(gotFilter, `\"`) {
		t.Errorf("filter = %q, want escaped quote", gotFilter)
	}
}

// ---------------------------------------------------------------------------
// ResolveByNameClient
// ---------------------------------------------------------------------------

func TestResolveByNameClient_WithSearchParam(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		// Blueprints uses ?search= (not filter).
		if got := r.URL.Query().Get("search"); got != "alpha" {
			t.Errorf("search = %q, want alpha", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "bp-1", "name": "alpha"},
				{"id": "bp-2", "name": "alpha-beta"}, // full-text hit, not exact — must be filtered client-side
			},
		})
	})

	id, _, err := c.ResolveByNameClient(context.Background(), "/api/blueprints/v1/blueprints", "search", "", "name", "id", "alpha")
	if err != nil {
		t.Fatalf("ResolveByNameClient: %v", err)
	}
	if id != "bp-1" {
		t.Errorf("id = %q, want bp-1 (exact match only)", id)
	}
}

func TestResolveByNameClient_NoSearchParam(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/v2/benchmarks", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("unexpected query string: %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"benchmarks": []map[string]any{}, // unknown shape — should fall back to single-object envelope handling
			"results": []map[string]any{
				{"id": "b-1", "title": "CIS-1"},
				{"id": "b-2", "title": "CIS-2"},
			},
		})
	})

	id, _, err := c.ResolveByNameClient(context.Background(), "/api/v2/benchmarks", "", "", "title", "id", "CIS-2")
	if err != nil {
		t.Fatalf("ResolveByNameClient: %v", err)
	}
	if id != "b-2" {
		t.Errorf("id = %q, want b-2", id)
	}
}

func TestResolveByNameClient_RawArrayResponse(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/things", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 42, "name": "thing"},
		})
	})

	id, _, err := c.ResolveByNameClient(context.Background(), "/api/things", "", "", "name", "id", "thing")
	if err != nil {
		t.Fatalf("ResolveByNameClient: %v", err)
	}
	if id != "42" {
		t.Errorf("id = %q, want 42 (numeric coerced to string)", id)
	}
}

func TestResolveByNameClient_NotFound(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
	})

	_, _, err := c.ResolveByNameClient(context.Background(), "/api/blueprints/v1/blueprints", "search", "", "name", "id", "nope")
	apiErr := AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(http.StatusNotFound) {
		t.Fatalf("expected APIResponseError(404), got %v", err)
	}
}

func TestResolveByNameClient_Ambiguous(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "a", "name": "same"},
				{"id": "b", "name": "same"},
			},
		})
	})

	_, _, err := c.ResolveByNameClient(context.Background(), "/api/blueprints/v1/blueprints", "", "", "name", "id", "same")
	var amErr *AmbiguousMatchError
	if !errors.As(err, &amErr) {
		t.Fatalf("expected *AmbiguousMatchError, got %v", err)
	}
	if len(amErr.Matches) != 2 {
		t.Errorf("Matches = %v, want 2 ids", amErr.Matches)
	}
}

func TestResolveByNameClient_EmptyName(t *testing.T) {
	c, _, _ := newTestClient(t)
	_, _, err := c.ResolveByNameClient(context.Background(), "/api/x", "search", "", "name", "id", "")
	if err == nil {
		t.Fatalf("expected error for empty name")
	}
}

func TestResolveByNameClient_PreservesExistingQuery(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/x", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("sort"); got != "name" {
			t.Errorf("sort = %q, want name", got)
		}
		if got := q.Get("search"); got != "alpha" {
			t.Errorf("search = %q, want alpha", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "1", "name": "alpha"}},
		})
	})

	_, _, err := c.ResolveByNameClient(context.Background(), "/api/x?sort=name", "search", "", "name", "id", "alpha")
	if err != nil {
		t.Fatalf("ResolveByNameClient: %v", err)
	}
}

// ---------------------------------------------------------------------------
// extractField
// ---------------------------------------------------------------------------

func TestExtractField(t *testing.T) {
	obj := map[string]any{
		"id":   float64(42),
		"name": "alpha",
		"general": map[string]any{
			"name":   "nested",
			"id":     float64(7),
			"active": true,
		},
		"empty": nil,
	}

	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"name", "alpha", true},
		{"id", "42", true},
		{"general.name", "nested", true},
		{"general.id", "7", true},
		{"general.active", "true", true},
		{"general.missing", "", false},
		{"missing", "", false},
		{"missing.subpath", "", false},
		{"empty", "", false}, // nil coerces to ("", false)
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, ok := extractField(obj, tc.path)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("extractField(%q) = (%q, %v), want (%q, %v)", tc.path, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractListElements
// ---------------------------------------------------------------------------

func TestExtractListElements(t *testing.T) {
	t.Run("paginated envelope with default results field", func(t *testing.T) {
		body := []byte(`{"totalCount": 2, "results": [{"id":"a"},{"id":"b"}]}`)
		elems, err := extractListElements(body, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(elems) != 2 {
			t.Fatalf("got %d elements, want 2", len(elems))
		}
	})
	t.Run("custom results field", func(t *testing.T) {
		body := []byte(`{"benchmarks": [{"id":"a"},{"id":"b"}]}`)
		elems, err := extractListElements(body, "benchmarks")
		if err != nil {
			t.Fatal(err)
		}
		if len(elems) != 2 {
			t.Fatalf("got %d elements, want 2", len(elems))
		}
	})
	t.Run("raw array", func(t *testing.T) {
		body := []byte(`[{"id":"a"},{"id":"b"},{"id":"c"}]`)
		elems, err := extractListElements(body, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(elems) != 3 {
			t.Fatalf("got %d elements, want 3", len(elems))
		}
	})
	t.Run("single object falls through to one-element slice", func(t *testing.T) {
		body := []byte(`{"id":"solo","name":"x"}`)
		elems, err := extractListElements(body, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(elems) != 1 {
			t.Fatalf("got %d elements, want 1", len(elems))
		}
	})
	t.Run("empty body", func(t *testing.T) {
		elems, err := extractListElements([]byte(""), "")
		if err != nil {
			t.Fatal(err)
		}
		if elems != nil {
			t.Errorf("got %v, want nil", elems)
		}
	})
}

// ---------------------------------------------------------------------------
// AmbiguousMatchError.Error formatting
// ---------------------------------------------------------------------------

func TestAmbiguousMatchError_Error(t *testing.T) {
	e := &AmbiguousMatchError{Name: "dup", Matches: []string{"1", "2", "3"}}
	msg := e.Error()
	want := `ambiguous match for name "dup": 3 resources (ids: 1, 2, 3)`
	if msg != want {
		t.Errorf("Error() = %q\nwant %q", msg, want)
	}
}

// Sanity: the primitive really does URL-encode the RSQL filter end-to-end so
// the server receives a parsable query string.
func TestResolveByNameFiltered_QueryEscaping(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/blueprints/v1/blueprints", func(w http.ResponseWriter, r *http.Request) {
		// Round-trip: decode the received filter and confirm it still equals
		// the RSQL expression we built.
		decoded, err := url.QueryUnescape(r.URL.RawQuery)
		if err != nil {
			t.Fatalf("query decode: %v", err)
		}
		if !strings.Contains(decoded, `name=="has space"`) {
			t.Errorf("decoded query %q missing expected filter fragment", decoded)
		}
		if _, err := fmt.Fprintln(w, `{"results":[{"id":"1","name":"has space"}]}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})

	_, _, err := c.ResolveByNameFiltered(context.Background(), "/api/blueprints/v1/blueprints", "", "name", "name", "id", "has space")
	if err != nil {
		t.Fatalf("ResolveByNameFiltered: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ResolveByNameClientPaged
// ---------------------------------------------------------------------------

func TestResolveByNameClientPaged_MatchFirstPage(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/pro/v1/prestages", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "0" {
			t.Errorf("expected page=0, got %s", r.URL.Query().Get("page"))
		}
		_, _ = fmt.Fprintln(w, `{"results":[{"id":"1","displayName":"alpha"},{"id":"2","displayName":"beta"}]}`)
	})

	id, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/prestages", "", "", "displayName", "id", "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "1" {
		t.Errorf("id = %q, want 1", id)
	}
}

func TestResolveByNameClientPaged_MatchSecondPage(t *testing.T) {
	c, _, mux := newTestClient(t)
	callCount := 0
	mux.HandleFunc("/api/pro/v1/items", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		callCount++
		switch page {
		case "0":
			// Return exactly pageSize (100) items so paging continues.
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("%d", i), "name": fmt.Sprintf("item-%d", i)}
			}
			resp := map[string]any{"results": items}
			_ = json.NewEncoder(w).Encode(resp)
		case "1":
			_, _ = fmt.Fprintln(w, `{"results":[{"id":"200","name":"target"}]}`)
		default:
			t.Errorf("unexpected page %s", page)
		}
	})

	id, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/items", "", "", "name", "id", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "200" {
		t.Errorf("id = %q, want 200", id)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestResolveByNameClientPaged_NotFound(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/pro/v1/items", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"results":[{"id":"1","name":"other"}]}`)
	})

	_, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/items", "", "", "name", "id", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIResponseError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("err = %v, want 404 APIResponseError", err)
	}
}

func TestResolveByNameClientPaged_AmbiguousSamePage(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/pro/v1/items", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"results":[{"id":"1","name":"dup"},{"id":"2","name":"dup"}]}`)
	})

	_, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/items", "", "", "name", "id", "dup")
	if err == nil {
		t.Fatal("expected error")
	}
	var ambig *AmbiguousMatchError
	if !errors.As(err, &ambig) {
		t.Errorf("err type = %T, want *AmbiguousMatchError", err)
	}
}

func TestResolveByNameClientPaged_AmbiguousAcrossPages(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/pro/v1/items", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			// Return exactly 100 items; one matches.
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("%d", i), "name": fmt.Sprintf("item-%d", i)}
			}
			items[50] = map[string]any{"id": "50", "name": "dup"}
			resp := map[string]any{"results": items}
			_ = json.NewEncoder(w).Encode(resp)
		case "1":
			_, _ = fmt.Fprintln(w, `{"results":[{"id":"200","name":"dup"}]}`)
		default:
			t.Errorf("unexpected page %s", page)
		}
	})

	_, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/items", "", "", "name", "id", "dup")
	if err == nil {
		t.Fatal("expected error")
	}
	var ambig *AmbiguousMatchError
	if !errors.As(err, &ambig) {
		t.Errorf("err type = %T, want *AmbiguousMatchError", err)
	}
	if len(ambig.Matches) != 2 {
		t.Errorf("matches = %d, want 2", len(ambig.Matches))
	}
}

// Server returns same resource twice on the same page — should resolve
// successfully, not report ambiguity.
func TestResolveByNameClientPaged_DuplicateSameIDSamePage(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/pro/v1/items", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"results":[{"id":"1","name":"dup"},{"id":"1","name":"dup"}]}`)
	})

	id, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/items", "", "", "name", "id", "dup")
	if err != nil {
		t.Fatalf("expected success for same-ID duplicates, got: %v", err)
	}
	if id != "1" {
		t.Errorf("id = %q, want 1", id)
	}
}

// Server returns same resource on different pages — should resolve
// successfully, not report ambiguity.
func TestResolveByNameClientPaged_DuplicateSameIDAcrossPages(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/pro/v1/items", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "0":
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("%d", i), "name": fmt.Sprintf("item-%d", i)}
			}
			items[50] = map[string]any{"id": "42", "name": "dup"}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": items})
		case "1":
			_, _ = fmt.Fprintln(w, `{"results":[{"id":"42","name":"dup"}]}`)
		default:
			t.Errorf("unexpected page %s", page)
		}
	})

	id, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/items", "", "", "name", "id", "dup")
	if err != nil {
		t.Fatalf("expected success for same-ID duplicates across pages, got: %v", err)
	}
	if id != "42" {
		t.Errorf("id = %q, want 42", id)
	}
}

func TestResolveByNameClientPaged_EmptyName(t *testing.T) {
	c, _, _ := newTestClient(t)
	_, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/x", "", "", "name", "id", "")
	if err == nil {
		t.Fatalf("expected error for empty name")
	}
}

func TestResolveByNameClientPaged_WithSearchParam(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/pro/v1/items", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search"); got != "target" {
			t.Errorf("search = %q, want target", got)
		}
		_, _ = fmt.Fprintln(w, `{"results":[{"id":"1","name":"target"}]}`)
	})

	id, _, err := c.ResolveByNameClientPaged(context.Background(), "/api/pro/v1/items", "search", "", "name", "id", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "1" {
		t.Errorf("id = %q, want 1", id)
	}
}
