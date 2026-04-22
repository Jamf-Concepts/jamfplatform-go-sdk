// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDoMultipart_FileUpload(t *testing.T) {
	c, srv, mux := newTestClient(t)

	const fieldName = "file"
	const filename = "icon.png"
	const contents = "\x89PNG\r\n\x1a\n-fake-png-body"

	var seenName, seenFilename, seenBody string
	mux.HandleFunc("/api/icons", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("Content-Type = %q, want multipart/form-data", ct)
		}
		_, params, err := strings.Cut(ct, "boundary=")
		if !err {
			t.Fatalf("no boundary in Content-Type: %q", ct)
		}
		mr := multipart.NewReader(r.Body, params)
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("reading part: %v", err)
			}
			seenName = p.FormName()
			seenFilename = p.FileName()
			buf, _ := io.ReadAll(p)
			seenBody = string(buf)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"icon-42"}`))
	})

	var result struct{ ID string }
	err := c.DoMultipart(context.Background(), http.MethodPost, "/api/icons", []MultipartField{
		{Name: fieldName, Filename: filename, Content: bytes.NewBufferString(contents)},
	}, http.StatusCreated, &result)
	if err != nil {
		t.Fatalf("DoMultipart: %v", err)
	}
	if seenName != fieldName {
		t.Errorf("field name = %q, want %q", seenName, fieldName)
	}
	if seenFilename != filename {
		t.Errorf("filename = %q, want %q", seenFilename, filename)
	}
	if seenBody != contents {
		t.Errorf("body = %q, want %q", seenBody, contents)
	}
	if result.ID != "icon-42" {
		t.Errorf("result.ID = %q, want icon-42", result.ID)
	}
	_ = srv
}

// TestDoMultipart_PrecomputedContentLength verifies that when Content is an
// io.Seeker (here: bytes.Reader) the transport sets Content-Length exactly
// and avoids chunked transfer encoding — the primary perf/compat property
// for big package uploads.
func TestDoMultipart_PrecomputedContentLength(t *testing.T) {
	c, _, mux := newTestClient(t)

	var gotCL int64
	var gotTE string
	var gotBodyLen int64
	mux.HandleFunc("/api/up", func(w http.ResponseWriter, r *http.Request) {
		gotCL = r.ContentLength
		gotTE = strings.Join(r.TransferEncoding, ",")
		n, _ := io.Copy(io.Discard, r.Body)
		gotBodyLen = n
		w.WriteHeader(http.StatusCreated)
	})

	payload := strings.Repeat("A", 4096)
	err := c.DoMultipart(context.Background(), http.MethodPost, "/api/up", []MultipartField{
		{Name: "file", Filename: "x.bin", Content: bytes.NewReader([]byte(payload))},
	}, http.StatusCreated, nil)
	if err != nil {
		t.Fatalf("DoMultipart: %v", err)
	}
	if gotCL <= 0 {
		t.Errorf("ContentLength = %d, want > 0 (precomputed for seekable reader)", gotCL)
	}
	if gotTE == "chunked" {
		t.Errorf("TransferEncoding = chunked, want identity (Content-Length should be set)")
	}
	if gotCL != gotBodyLen {
		t.Errorf("ContentLength header %d != actual body bytes %d", gotCL, gotBodyLen)
	}
}

// unseekableReader is an io.Reader that deliberately does not implement
// io.Seeker — used to assert the chunked-fallback path.
type unseekableReader struct{ r io.Reader }

func (u unseekableReader) Read(p []byte) (int, error) { return u.r.Read(p) }

func TestDoMultipart_ChunkedFallbackWhenSizeUnknown(t *testing.T) {
	c, _, mux := newTestClient(t)

	var gotCL int64
	var gotChunked bool
	var gotBodyLen int64
	mux.HandleFunc("/api/up2", func(w http.ResponseWriter, r *http.Request) {
		gotCL = r.ContentLength
		for _, te := range r.TransferEncoding {
			if te == "chunked" {
				gotChunked = true
			}
		}
		n, _ := io.Copy(io.Discard, r.Body)
		gotBodyLen = n
		w.WriteHeader(http.StatusCreated)
	})

	payload := strings.Repeat("B", 1024)
	err := c.DoMultipart(context.Background(), http.MethodPost, "/api/up2", []MultipartField{
		{Name: "file", Filename: "y.bin", Content: unseekableReader{r: strings.NewReader(payload)}},
	}, http.StatusCreated, nil)
	if err != nil {
		t.Fatalf("DoMultipart: %v", err)
	}
	if gotCL != -1 && gotCL != 0 {
		t.Errorf("ContentLength = %d, want unset (non-seekable reader → chunked)", gotCL)
	}
	if !gotChunked {
		t.Errorf("expected Transfer-Encoding: chunked when size is unknown")
	}
	if gotBodyLen == 0 {
		t.Errorf("server read 0 body bytes, expected streamed content")
	}
}

// TestDoMultipart_RewindOn429 verifies the transport seeks the file Content
// back to 0 and retries once when the server responds 429 with a short
// Retry-After, provided all file parts are io.Seeker.
func TestDoMultipart_RewindOn429(t *testing.T) {
	c, _, mux := newTestClient(t)

	var calls atomic.Int32
	mux.HandleFunc("/api/retry", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		// Drain the body so the client can proceed.
		_, _ = io.Copy(io.Discard, r.Body)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	payload := strings.Repeat("C", 2048)
	rd := bytes.NewReader([]byte(payload))
	err := c.DoMultipart(context.Background(), http.MethodPost, "/api/retry", []MultipartField{
		{Name: "file", Filename: "z.bin", Content: rd},
	}, http.StatusCreated, nil)
	if err != nil {
		t.Fatalf("DoMultipart: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("server saw %d calls, want 2 (one 429, one retry)", got)
	}
}
