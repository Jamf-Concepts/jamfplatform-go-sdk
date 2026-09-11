// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// A non-success response that is not JSON comes from one of three places, and
// they need different treatment, so each is detected separately rather than
// lumped together as "not JSON":
//
//   - Jamf Pro's own HTML "Status page" — a real application message, lifted by
//     parseClassicErrorMessage in classic_errors.go.
//   - An edge or gateway HTML error page (CloudFront, the gateway's own styled
//     503) — no Jamf message at all, condensed here and marked with
//     ErrUnexpectedResponse.
//   - A plain-text gateway refusal ("Authentication failed", "404 page not
//     found") — short enough to read as-is, but "Authentication failed" is
//     ambiguous in a way worth naming. See nonJSONAuthGuidance.
//
// Deliberately NOT detected: which layer refused. The body's JSON formatting
// (Jamf Pro pretty-prints, the gateway emits compact) separates the
// authorization service from Jamf Pro, and docs/WIRE-FACTS.md records that as a
// serialiser tell rather than a contract, not to be encoded in the transport.
// Distinguishing "this is a web page, not an API response" is a weaker claim
// than layer attribution and is one the SDK already makes on the token exchange
// (see annotateTokenError) — this brings the API path in line with it.

// edgeRequestIDHeaders lists response headers carrying an edge-assigned request
// identifier. An edge block never carries a Jamf traceId, so there is nothing in
// Jamf's logs to correlate and this is the only handle for a CloudFront-side
// lookup — which is why it is worth lifting out of a body that is otherwise
// discarded.
var edgeRequestIDHeaders = []string{
	"X-Amz-Cf-Id", // CloudFront; equals the "Request ID" its error page prints
}

// maxErrorHeadings caps how many headings summarizeNonJSONError keeps. Three is
// enough for both observed templates (CloudFront uses title/h1/h2, the gateway's
// 503 page uses title plus two h2s) while bounding what an unknown page can
// contribute.
const maxErrorHeadings = 3

// requestIDPattern matches the "Request ID: <id>" line CloudFront prints inside
// the <pre> block of its error page, used only when the header is absent.
var requestIDPattern = regexp.MustCompile(`(?i)request id:\s*(\S+)`)

// isHTMLErrorBody reports whether a response body is an HTML page rather than an
// API response.
//
// Both the declared content type and the body's own first bytes are consulted:
// the header is authoritative when present, and the sniff covers a page served
// without one. The sniff requires a doctype or an <html> root specifically, so a
// Classic XML body (which begins "<?xml") and the XML entity echoes some Classic
// endpoints return under a 4xx are not mistaken for pages.
func isHTMLErrorBody(header http.Header, body []byte) bool {
	if isHTMLContentType(header.Get("Content-Type")) {
		return true
	}
	t := bytes.TrimLeft(body, " \t\r\n")
	if len(t) == 0 || t[0] != '<' {
		return false
	}
	head := bytes.ToLower(t[:min(len(t), 64)])
	return bytes.HasPrefix(head, []byte("<!doctype html")) || bytes.HasPrefix(head, []byte("<html"))
}

// summarizeNonJSONError condenses an HTML error page into one line: its distinct
// headings, plus an edge request id when one is available.
//
// The page's headings are the only part that varies with the failure; everything
// else is markup, styling and boilerplate. The gateway's 503 page is 149 lines
// whose entire content is "503 Error" and "Service currently unavailable, try
// your request again shortly", and CloudFront's is 39 lines around three
// headings — so the raw body costs a consumer's log a screen per failure and
// tells it nothing the summary does not. The full page stays on
// APIResponseError.Body for anyone who wants it.
//
// Returns "" when the page yields nothing, rather than inventing a message;
// describeNonJSONError is what the transport calls, and it names the page in
// that case instead of falling back to the body.
func summarizeNonJSONError(header http.Header, body []byte) string {
	var kept []string
	for _, t := range collectTagText(body, "title", "h1", "h2", "h3") {
		// Templates repeat themselves across title and headings — CloudFront's
		// <h2> restates its <title> almost verbatim. Drop a heading already
		// contained in one kept, so the summary carries each distinct statement
		// once.
		norm := normalizeHeading(t.text)
		if norm == "" {
			continue
		}
		seen := false
		for _, k := range kept {
			if strings.Contains(normalizeHeading(k), norm) {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		kept = append(kept, t.text)
		if len(kept) == maxErrorHeadings {
			break
		}
	}

	msg := strings.Join(kept, "; ")
	if id := edgeRequestID(header, body); id != "" {
		if msg == "" {
			msg = "non-JSON error page"
		}
		msg += " (edge request id: " + id + ")"
	}
	return msg
}

// describeNonJSONError is summarizeNonJSONError with a guaranteed non-empty
// answer, for the caller that has already decided the body is a page.
//
// An unrecognised template yields no headings and no request id, and falling
// back to the raw body there puts the whole page into Error() — 42 lines for a
// heading-less page, which is the wall of text this classification exists to
// remove, reintroduced on precisely the input nobody has seen before. Naming the
// page and its size is less than the summary and more than nothing; the body
// itself stays on APIResponseError.Body.
func describeNonJSONError(header http.Header, body []byte) string {
	if msg := summarizeNonJSONError(header, body); msg != "" {
		return msg
	}
	return fmt.Sprintf("unrecognised HTML error page, %d bytes (see APIResponseError.Body)", len(body))
}

// normalizeHeading reduces a heading to lowercase alphanumerics and single
// spaces, so near-duplicate headings differing only in punctuation or case
// compare equal.
func normalizeHeading(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// edgeRequestID returns an edge-assigned request identifier, preferring the
// response header over the copy printed in the page body. The header is the same
// value and needs no entity decoding — CloudFront HTML-escapes the trailing "=="
// of the body copy as "&#x3D;&#x3D;".
func edgeRequestID(header http.Header, body []byte) string {
	for _, name := range edgeRequestIDHeaders {
		if v := header.Get(name); v != "" {
			return v
		}
	}
	for _, t := range collectTagText(body, "pre", "address") {
		if m := requestIDPattern.FindStringSubmatch(t.text); m != nil {
			return m[1]
		}
	}
	return ""
}

// apiProductGuidance names the ambiguity in a plain-text 401 from the gateway.
//
// The gateway answers a rejected token and a credential holding no policy for
// the requested api-product with the identical two-word body, so the response
// cannot distinguish them — and they need opposite remedies: rotate the secret
// versus get the api-product granted. Unannotated it reads like the first while
// being, in every occurrence observed in CI, the second: thirteen of them in one
// run, all on PKI, patch-software-title and JSON-web-token paths, which is a
// coherent capability set rather than a credential going bad.
//
// Deliberately carries no sentinel. ErrUnexpectedResponse means "something in
// front of Jamf answered", and a consumer branching on it reports the host's
// egress IP — the wrong remedy for a missing grant, and worse than no branch at
// all.
const apiProductGuidance = "the gateway returns this same plain-text 401 both for a rejected token and for a credential with no policy for the requested api-product, so this response does not distinguish them; check whether another path in the same namespace answers for this client before rotating the secret"

// nonJSONAuthGuidance returns guidance to append to a plain-text 401, or "" when
// the response is not that case.
//
// Confined to 401: a plain-text body on any other status ("404 page not found"
// from the gateway for an unknown namespace, say) has an unambiguous cause and
// needs no annotation.
//
// The status is the whole test, deliberately: the body is NOT matched against
// "Authentication failed". Three reasons, in order of weight.
//
// The failure modes are not symmetric. A body this does not recognise loses the
// annotation permanently and silently — the same silence that had thirteen CI
// failures reading as bad credentials — whereas a body it recognises too eagerly
// gets appended prose that is sound advice for any unexplained 401 anyway
// ("check whether another path in the same namespace answers for this client
// before rotating the secret"). Unlike the sentinel, which sends a consumer
// after an egress IP, being wrong here costs a sentence and no wrong action,
// which is why this deliberately carries no sentinel.
//
// It is also the rule annotateTokenError already follows, for the reason stated
// there: an upstream message is prose the SDK does not own, and enrichment must
// never be conditional on it, because this only ever adds to an error that is
// already being returned.
//
// And the gateway gives the exact match nothing to discriminate. Probed
// 2026-09-11 against eu.api.jamfcloud.com with a 200 control in the same
// invocation: a garbage bearer, a bare "Bearer" carrying no token and a "Basic"
// scheme all answer the byte-identical 22-byte body, as the ungranted
// api-product does in WIRE-FACTS, while a *missing* Authorization header answers
// JSON ({"httpStatus":401,"message":"unauthorized access"}) and is excluded by
// looksLikeJSON. Four distinct faults, one wording, and no second plain-text 401
// wording observed on this gateway — so narrowing to the string would change
// nothing today and lose the annotation the day the wording moves.
func nonJSONAuthGuidance(status int, header http.Header, body []byte) string {
	if status != http.StatusUnauthorized {
		return ""
	}
	if looksLikeJSON(body) || isHTMLErrorBody(header, body) {
		return ""
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return ""
	}
	return apiProductGuidance
}

// tagText pairs an element's tag name with its text content.
type tagText struct {
	tag  string
	text string
}

// collectTagText returns the trimmed, entity-decoded text of every element whose
// tag name is in want, in document order.
//
// Uses the x/net/html tokenizer rather than a regex so attribute ordering,
// nested inline tags and HTML entities are handled correctly, and so a truncated
// or malformed page yields what it has rather than failing. Shared by the
// Classic "Status page" paragraph scrape and the edge-page heading summary.
//
// Only one wanted element is tracked at a time, which is sufficient for both
// (neither template nests wanted tags), and a stray unclosed tag is flushed
// rather than allowed to swallow the rest of the document: the tokenizer never
// emits the missing end tag, so without that an unclosed heading consumes every
// later one and the element never flushes at all. A page whose headings are all
// unclosed would then summarize to nothing, and an empty summary is the one
// input that puts the whole page back into the error message.
func collectTagText(body []byte, want ...string) []tagText {
	wanted := make(map[string]bool, len(want))
	for _, w := range want {
		wanted[w] = true
	}

	z := html.NewTokenizer(bytes.NewReader(body))
	var (
		out    []tagText
		active string
		buf    strings.Builder
	)
	flush := func() {
		text := strings.Join(strings.Fields(buf.String()), " ")
		buf.Reset()
		if text != "" {
			out = append(out, tagText{tag: active, text: text})
		}
		active = ""
	}
	for {
		switch z.Next() {
		case html.ErrorToken:
			// EOF, or a page cut off mid-element. Flush the pending text rather
			// than discarding it: a truncated body is exactly what a proxy
			// timeout produces, and its last element is often the only one.
			if active != "" {
				flush()
			}
			return out
		case html.StartTagToken:
			name, _ := z.TagName()
			switch n := string(name); {
			case !wanted[n]:
			case active == "":
				active = n
			default:
				// A wanted element opened while one is still active — the same
				// name included, since neither template nests them. Flush what
				// is buffered and track the new one, so the two texts neither
				// merge nor wait on an end tag that is never coming.
				flush()
				active = n
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if active == "" || string(name) != active {
				continue
			}
			flush()
		case html.TextToken:
			if active != "" {
				buf.Write(z.Text()) // z.Text() returns entity-decoded text
			}
		}
	}
}
