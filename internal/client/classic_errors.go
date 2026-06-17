// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"mime"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// classicStatusPageMarker is the <title> unique to the Jamf Classic API's
// custom HTML error template ("Status page"). Gating the paragraph scrape on
// this marker keeps it confined to that known, long-stable template and
// prevents it running against other text/html bodies — WAF/CDN block pages,
// gateway error pages, login redirects — whose unrelated markup would
// otherwise yield a misleading "message". Anything without the marker falls
// through to the raw response body, unchanged.
const classicStatusPageMarker = "<title>Status page</title>"

// parseClassicErrorMessage extracts a concise, human-readable message from a
// Jamf Classic API HTML "Status page" error body.
//
// The Classic API does not return structured (JSON) errors. For 4xx/5xx it
// serves a custom — not stock Tomcat — HTML page that is undocumented but
// stable across Jamf Pro versions and uniform across every classic resource.
// Its <p> elements are, in order: the HTTP reason phrase ("Conflict"), a
// detail line (either "Error: <message>" or a free-form sentence), then
// boilerplate ("You can get technical details …"). The detail line is the only
// part not already conveyed by the HTTP status, so it is preferred; the reason
// phrase is the fallback when no detail is present.
//
// Returns ("", false) unless the body is text/html carrying the recognised
// template, so callers retain the raw body for every other shape: JSON errors
// are handled upstream, and text/xml entity echoes returned under a misleading
// 4xx by quirk endpoints (e.g. the classic /ebooks DELETE, which echoes
// <ebook><id>N</id></ebook> on an accepted async delete) are left untouched.
func parseClassicErrorMessage(header http.Header, body []byte) (string, bool) {
	if !isHTMLContentType(header.Get("Content-Type")) {
		return "", false
	}
	if !bytes.Contains(body, []byte(classicStatusPageMarker)) {
		return "", false
	}

	paragraphs := extractParagraphs(body)
	if len(paragraphs) == 0 {
		return "", false
	}

	// paragraphs[0] is the HTTP reason phrase (already in the status line);
	// paragraphs[1], when present, carries the meaningful detail.
	msg := paragraphs[0]
	if len(paragraphs) > 1 {
		msg = paragraphs[1]
	}
	// Strip the "Error:" prefix the template uses for application messages.
	// The colon is intentional — free-form sentences like "Error in XML file"
	// have no colon and must be left intact.
	msg = strings.TrimSpace(strings.TrimPrefix(msg, "Error:"))
	if msg == "" {
		return "", false
	}
	return msg, true
}

// isHTMLContentType reports whether a Content-Type header value names an HTML
// body, tolerating parameters such as "; charset=utf-8".
func isHTMLContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "text/html"
}

// extractParagraphs returns the trimmed, entity-decoded text of each <p>
// element in body, in document order, dropping the template's trailing "You
// can get technical details …" boilerplate and any empty paragraph. It uses
// the x/net/html tokenizer rather than a regex so attribute ordering, nested
// inline tags (<a>, <br>), and HTML entities are handled correctly.
func extractParagraphs(body []byte) []string {
	z := html.NewTokenizer(bytes.NewReader(body))
	var (
		out   []string
		depth int // >0 while inside a <p> element
		buf   strings.Builder
	)
	flush := func() {
		text := strings.Join(strings.Fields(buf.String()), " ")
		buf.Reset()
		if text == "" || strings.HasPrefix(text, "You can get technical details") {
			return
		}
		out = append(out, text)
	}
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken:
			if name, _ := z.TagName(); string(name) == "p" {
				depth++
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "p" && depth > 0 {
				depth--
				if depth == 0 {
					flush()
				}
			}
		case html.TextToken:
			if depth > 0 {
				buf.Write(z.Text()) // z.Text() returns entity-decoded text
			}
		}
	}
}
