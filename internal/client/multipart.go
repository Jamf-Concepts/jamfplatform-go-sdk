// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

// multipartCopyBuf is the buffer size for io.CopyBuffer when streaming
// file parts through the multipart body pipe. 1 MiB is larger than the
// default 32 KiB — fewer syscalls when pushing multi-GB package uploads.
const multipartCopyBuf = 1 << 20

// MultipartField represents one part of a multipart/form-data request body.
// Exactly one of Filename (file upload) or Value (text field) must be set.
//
// For file parts, Content is consumed once and streamed directly to the
// network — no in-memory buffering of the whole body. The body is always sent
// over HTTP/1.1 with chunked transfer encoding and no declared Content-Length,
// whatever Content is; an io.Seeker only makes the upload retryable, because it
// can be rewound and streamed again. See sendMultipart for why.
type MultipartField struct {
	Name     string    // form field name
	Filename string    // if non-empty, part is a file upload with this filename
	Content  io.Reader // file content; read to EOF
	Value    string    // text value when Filename is empty
}

// DoMultipart performs an authenticated API request with a multipart/form-data
// body. The body is streamed via io.Pipe — memory usage is O(buffer), not
// O(file). result follows the same rules as Do — either a JSON-unmarshal
// target or *[]byte for raw responses.
//
// Retries a transient failure (429, 503, or 500/502/504 on an idempotent
// method — see isRetryableWriteStatus) up to retryMax times, with the same
// backoff policy as the JSON/XML transport (jamfBackoff), but ONLY when
// every file part's Content is an io.Seeker (rewindable): sendMultipart
// streams the body through an io.Pipe consumed exactly once, so a retry can
// only resend it by seeking each part back to the start and re-streaming.
// Otherwise the failure surfaces as an APIResponseError/transport error
// immediately and the caller is expected to re-invoke with fresh Content
// readers.
//
// This is a separate, manual retry loop rather than a ride on c.httpClient's
// automatic one deliberately: retryablehttp makes a request body replayable
// by buffering it wholesale (see FromRequest/getBodyReaderAndContentLength),
// which for a multi-GB package upload would defeat the entire point of
// streaming it — see sendMultipart's use of c.uploadClient instead of
// c.httpClient.
func (c *Transport) DoMultipart(ctx context.Context, method, path string, fields []MultipartField, expectedStatus int, result any) error {
	fullURL := c.buildURL(path)
	if err := checkDeniedPath(method, fullURL); err != nil {
		return err
	}

	var resp *http.Response
	var err error
	for attempt := 0; ; attempt++ {
		attemptCtx, sent := withSendState(ctx, method)
		resp, err = c.sendMultipart(attemptCtx, method, fullURL, fields)

		retryable := false
		switch {
		case err != nil:
			// Same rule as jamfCheckRetry's resp == nil branch: free to retry
			// while the request never finished being written, so a connection
			// dropped mid-upload starts over, but not after — a POST whose body
			// was fully sent and then lost its connection may have been applied.
			retryable = retryableTransportError(sent, err)
		case isRetryableWriteStatus(method, resp.StatusCode):
			retryable = true
		}
		if !retryable || attempt >= retryMax || !multipartRewindable(fields) {
			break
		}

		wait := jamfBackoff(retryWaitMin, retryWaitMax, attempt, resp)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if rerr := rewindMultipart(fields); rerr != nil {
			return rerr
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			if err != nil {
				return err
			}
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	return c.handleMultipartResponse(ctx, resp, expectedStatus, result)
}

// sendMultipart builds the request body as a streaming pipe and dispatches it.
// Callers handle status codes; this function only returns transport-level
// errors. The caller is responsible for closing resp.Body.
func (c *Transport) sendMultipart(ctx context.Context, method, fullURL string, fields []MultipartField) (*http.Response, error) {
	boundary := randomBoundary()

	// Every multipart request goes out over HTTP/1.1 and chunked, with no
	// declared Content-Length, on purpose and with no threshold or opt-out.
	// Measured against the GA gateway, both choices help a large upload:
	//
	//   - HTTP/1.1: CloudFront advertises a 64 KiB per-stream HTTP/2 window, so a
	//     single h2 upload tops out at 64 KiB per round trip — about 4 MiB/s at
	//     13 ms — where HTTP/1.1 runs at the uplink, about 10 MiB/s here.
	//   - No Content-Length: the same 1.43 GiB file over HTTP/1.1 on the same
	//     tenant was refused with a 502 at about 1.04 GiB when the length was
	//     declared (curl and Go agreed) and uploaded whole, hashes matching, when
	//     it was not. A declared length also caps uploads near 375 MiB over h2.
	//
	// The gateway still refuses some very large bodies whatever the framing, so
	// this raises the ceiling and does not remove it. It is unconditional rather
	// than size-gated so that it stays correct if the gateway is fixed.
	// Marking the context is what routes this request to the HTTP/1.1 transport;
	// a client supplied through WithHTTPClient keeps its own transport and so
	// its own protocol, and only the framing applies to it.
	ctx = withHTTP1(ctx)

	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(writeMultipart(pw, fields, boundary))
	}()

	req, err := http.NewRequestWithContext(ctx, method, fullURL, pr)
	if err != nil {
		_ = pr.Close()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.setScopeHeader(req)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	// -1 declares the length unknown, so net/http chunks the body without
	// probing it first (a zero length with a non-nil body makes it wait up to
	// 200ms for a first byte to decide).
	req.ContentLength = -1

	if c.logger != nil {
		c.logger.LogRequest(ctx, method, fullURL, []byte("<multipart body>"))
	}

	// Deliberately c.uploadClient, not c.httpClient: req's body is a live
	// io.Pipe, consumed exactly once. Routing it through the retry-wrapped
	// client would make retryablehttp buffer the entire body into memory up
	// front (its only fallback for a non-seekable io.Reader) just to make it
	// theoretically replayable — for a multi-GB package upload that defeats
	// the whole point of streaming. Retrying multipart requests is handled
	// one layer up, in DoMultipart, by re-streaming from a rewound source
	// instead.
	resp, err := c.uploadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	return resp, nil
}

// handleMultipartResponse reads the response body, logs it, and either
// unmarshals into result or builds an APIResponseError on status mismatch.
func (c *Transport) handleMultipartResponse(ctx context.Context, resp *http.Response, expectedStatus int, result any) error {
	defer func() { _ = resp.Body.Close() }()

	c.logDeprecation(resp)

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read response body: %w", readErr)
	}

	if c.logger != nil {
		c.logger.LogResponse(ctx, resp.StatusCode, resp.Header, body)
	}

	if resp.StatusCode != expectedStatus {
		respErr := &APIResponseError{
			StatusCode: resp.StatusCode,
			Method:     resp.Request.Method,
			URL:        resp.Request.URL.String(),
			Body:       string(body),
		}
		var apiErr ApiError
		_ = json.Unmarshal(body, &apiErr) // best-effort; non-JSON bodies leave apiErr zero
		switch {
		case len(apiErr.Errors) > 0:
			if apiErr.HTTPStatus > 0 {
				respErr.StatusCode = apiErr.HTTPStatus
			}
			respErr.Errors = apiErr.Errors
		default:
			// Classic API errors are an HTML "Status page", not JSON — see
			// handleResponse for the rationale. Lift the message into a
			// synthetic structured detail for consistent rendering.
			if msg, ok := parseClassicErrorMessage(resp.Header, body); ok {
				respErr.Errors = []Error{{Description: msg}}
			}
		}
		respErr.TraceID = pickTraceID(apiErr.TraceID, resp.Header)
		return respErr
	}

	if result != nil {
		if bp, ok := result.(*[]byte); ok {
			*bp = append((*bp)[:0], body...)
		} else if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

// writeMultipart writes the fields to w as a multipart/form-data body using
// the supplied boundary. File content is streamed via io.CopyBuffer with a
// 1 MiB buffer.
func writeMultipart(w io.Writer, fields []MultipartField, boundary string) error {
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return fmt.Errorf("multipart SetBoundary: %w", err)
	}
	buf := make([]byte, multipartCopyBuf)
	for _, f := range fields {
		if f.Filename != "" {
			part, err := mw.CreatePart(filePartHeader(f.Name, f.Filename))
			if err != nil {
				return fmt.Errorf("multipart CreatePart(%q): %w", f.Name, err)
			}
			if f.Content != nil {
				if _, err := io.CopyBuffer(part, f.Content, buf); err != nil {
					return fmt.Errorf("multipart copy(%q): %w", f.Name, err)
				}
			}
		} else {
			if err := mw.WriteField(f.Name, f.Value); err != nil {
				return fmt.Errorf("multipart WriteField(%q): %w", f.Name, err)
			}
		}
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("multipart close: %w", err)
	}
	return nil
}

// filePartHeader produces the MIME headers for a file part. The server's
// image-upload endpoints (enrollment-customization, icon) reject the stdlib
// default Content-Type: application/octet-stream for PNG uploads, so we
// sniff from the extension and strip charset parameters that mime attaches
// to text/* types.
func filePartHeader(name, filename string) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, name, filename))
	ct := mime.TypeByExtension(filepath.Ext(filename))
	if ct == "" {
		ct = "application/octet-stream"
	}
	if semi := strings.Index(ct, ";"); semi >= 0 {
		ct = strings.TrimSpace(ct[:semi])
	}
	h.Set("Content-Type", ct)
	return h
}

// multipartRewindable reports whether every file part's Content can be
// seeked back to its start — required for a safe 429 retry.
func multipartRewindable(fields []MultipartField) bool {
	for _, f := range fields {
		if f.Filename == "" {
			continue
		}
		if f.Content == nil {
			continue
		}
		if _, ok := f.Content.(io.Seeker); !ok {
			return false
		}
	}
	return true
}

// rewindMultipart seeks every file part's Content back to the start.
// Caller must have already confirmed rewindability via multipartRewindable.
func rewindMultipart(fields []MultipartField) error {
	for _, f := range fields {
		if f.Filename == "" || f.Content == nil {
			continue
		}
		s := f.Content.(io.Seeker)
		if _, err := s.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("rewind part %q: %w", f.Name, err)
		}
	}
	return nil
}

// randomBoundary matches stdlib multipart.Writer's default boundary style
// (30 hex chars). Kept fixed-length so both sizing and writing passes use
// identical bytes.
func randomBoundary() string {
	// Reuse the stdlib's own generator by constructing a throwaway writer.
	return multipart.NewWriter(io.Discard).Boundary()
}
