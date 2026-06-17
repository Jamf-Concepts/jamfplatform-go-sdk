// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"testing"
)

// statusPage renders the Jamf Classic API "Status page" error template with
// the given bold reason phrase and detail paragraph, matching the markup
// captured from live proclassic responses.
func statusPage(reason, detail string) string {
	return `<html>
<head>
   <title>Status page</title>
</head>
<body style="font-family: sans-serif;">
<p style="font-size: 1.2em;font-weight: bold;margin: 1em 0px;">` + reason + `</p>
<p>` + detail + `</p>
<p>You can get technical details <a href="http://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.10">here</a>.<br>
Please continue your visit at our <a href="/">home page</a>.
</p>
</body>
</html>`
}

func htmlHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "text/html;charset=utf-8")
	return h
}

func TestParseClassicErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		header  http.Header
		body    string
		wantMsg string
		wantOK  bool
	}{
		// --- real wire captures (proclassic, platform gateway) ---
		{
			name:    "409 duplicate extension (Error: prefix stripped)",
			header:  htmlHeader(),
			body:    statusPage("Conflict", "Error: Duplicate extension"),
			wantMsg: "Duplicate extension",
			wantOK:  true,
		},
		{
			name:    "409 duplicate name",
			header:  htmlHeader(),
			body:    statusPage("Conflict", "Error: Duplicate name"),
			wantMsg: "Duplicate name",
			wantOK:  true,
		},
		{
			name:    "validation error with nested colon in message",
			header:  htmlHeader(),
			body:    statusPage("Conflict", "Error: Allowed File Extension: extension is required"),
			wantMsg: "Allowed File Extension: extension is required",
			wantOK:  true,
		},
		{
			name:    "404 generic sentence (no Error: prefix)",
			header:  htmlHeader(),
			body:    statusPage("Not Found", "The server has not found anything matching the request URI"),
			wantMsg: "The server has not found anything matching the request URI",
			wantOK:  true,
		},
		{
			// Verbatim 400 body: note the double space after "file." which
			// strings.Fields collapses; "Error in" must NOT be stripped because
			// there is no "Error:" colon prefix.
			name:    "400 XML parse error, double space collapsed, no colon strip",
			header:  htmlHeader(),
			body:    statusPage("Bad Request", "Error in XML file.  Possible mismatch between resource specified in the URL and XML file"),
			wantMsg: "Error in XML file. Possible mismatch between resource specified in the URL and XML file",
			wantOK:  true,
		},
		{
			// Hand-written fixture for the duplicate-profile-UUID conflict;
			// identical template shape, so it flows through the same path.
			name:    "duplicate profile UUID conflict",
			header:  htmlHeader(),
			body:    statusPage("Conflict", "Error: Duplicate UUID"),
			wantMsg: "Duplicate UUID",
			wantOK:  true,
		},
		{
			name:    "HTML entities decoded in message",
			header:  htmlHeader(),
			body:    statusPage("Conflict", "Error: Name &amp; label already in use"),
			wantMsg: "Name & label already in use",
			wantOK:  true,
		},

		// --- negative cases: must fall through to the raw body ---
		{
			name:    "ebook DELETE text/xml entity echo is left untouched",
			header:  xmlHeader(),
			body:    `<?xml version="1.0" encoding="UTF-8"?><ebook><id>120</id></ebook>`,
			wantMsg: "",
			wantOK:  false,
		},
		{
			name:    "JSON error body is not treated as HTML",
			header:  jsonHeader(),
			body:    `{"httpStatus":500,"errors":[]}`,
			wantMsg: "",
			wantOK:  false,
		},
		{
			name:   "WAF-style HTML without the Status page marker is ignored",
			header: htmlHeader(),
			body: `<html><head><title>Access Denied</title></head><body>` +
				`<p>Request blocked. Reference #18.2bcd: see your administrator.</p></body></html>`,
			wantMsg: "",
			wantOK:  false,
		},
		{
			name:    "text/html with no paragraphs",
			header:  htmlHeader(),
			body:    `<html><head><title>Status page</title></head><body></body></html>`,
			wantMsg: "",
			wantOK:  false,
		},
		{
			name:    "empty body",
			header:  htmlHeader(),
			body:    "",
			wantMsg: "",
			wantOK:  false,
		},
		{
			name:    "no content-type header",
			header:  http.Header{},
			body:    statusPage("Conflict", "Error: Duplicate name"),
			wantMsg: "",
			wantOK:  false,
		},
		{
			name:    "text/plain is not HTML",
			header:  textHeader(),
			body:    "404 page not found",
			wantMsg: "",
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMsg, gotOK := parseClassicErrorMessage(tc.header, []byte(tc.body))
			if gotOK != tc.wantOK {
				t.Fatalf("ok = %v, want %v (msg=%q)", gotOK, tc.wantOK, gotMsg)
			}
			if gotMsg != tc.wantMsg {
				t.Errorf("msg = %q, want %q", gotMsg, tc.wantMsg)
			}
		})
	}
}

func xmlHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "text/xml;charset=utf-8")
	return h
}

func jsonHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return h
}

func textHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "text/plain; charset=utf-8")
	return h
}

// TestHandleResponse_ClassicHTMLError verifies end-to-end that a classic HTML
// "Status page" 409 surfaces a structured APIResponseError: the message reaches
// Error(), Summary(), and Details(), the trace id from the header is rendered,
// and the raw HTML is not dumped.
func TestHandleResponse_ClassicHTMLError(t *testing.T) {
	c, _, mux := newTestClient(t)
	mux.HandleFunc("/api/classic-conflict", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html;charset=utf-8")
		w.Header().Set("X-Tyk-Trace-Id", "tyk-trace-xyz")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(statusPage("Conflict", "Error: Duplicate extension")))
	})

	err := c.Do(context.Background(), http.MethodPost, "/api/classic-conflict", nil, nil)
	apiErr := AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("AsAPIError returned nil for err=%v", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("StatusCode = %d, want 409", apiErr.StatusCode)
	}
	if apiErr.TraceID != "tyk-trace-xyz" {
		t.Errorf("TraceID = %q, want tyk-trace-xyz", apiErr.TraceID)
	}
	for _, want := range []string{"409", "tyk-trace-xyz", "Duplicate extension"} {
		if !contains(apiErr.Error(), want) {
			t.Errorf("Error() = %q, missing %q", apiErr.Error(), want)
		}
	}
	if contains(apiErr.Error(), "<html>") || contains(apiErr.Error(), "Status page") {
		t.Errorf("Error() leaked raw HTML: %q", apiErr.Error())
	}
	if !contains(apiErr.Summary(), "Duplicate extension") {
		t.Errorf("Summary() = %q, missing message", apiErr.Summary())
	}
	if d := apiErr.Details(); len(d) != 1 || d[0].Description != "Duplicate extension" {
		t.Errorf("Details() = %+v, want one detail with the message", d)
	}
}
