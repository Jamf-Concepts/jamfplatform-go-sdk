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
